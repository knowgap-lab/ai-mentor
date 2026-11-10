"""Файлы эксплуатации без Docker: compose, backup.sh, тестовый вход.

Красный результат из-за отсутствующего файла ожидаем: реализация лежит
в другой сессии. Секретов здесь нет, контейнеры не запускаются.

Имя ``AUTH_DEV_MODE`` взято из примера AU-R36 в ``docs/platform/auth.md``.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml


DEPLOY = Path(__file__).resolve().parent
COMPOSE_NAMES = ("compose.prod.yml", "compose.staging.yml")
REQUIRED_SERVICES = ("web", "core", "content", "db")
APP_SERVICES = ("core", "web")
DISABLED_DEV_MODE = {"false", "0"}

_HEX_RUN = re.compile(r"\b[0-9a-fA-F]{32,}\b")
_ENV_REFERENCE = re.compile(r"\$\{[A-Za-z_][A-Za-z0-9_]*\}")
_DEV_MODE = re.compile(r"(?i)\bAUTH_DEV_MODE\b\s*[:=]\s*([^\s#]+)")
_MIGRATION = re.compile(r"(?i)migrat|миграц")


def _compose_path(name: str) -> Path:
    return DEPLOY / name


def _read(path: Path) -> str:
    if not path.is_file():
        pytest.fail(f"нет файла {path.relative_to(DEPLOY.parent)}")
    return path.read_text(encoding="utf-8")


def _load_compose(name: str) -> dict:
    text = _read(_compose_path(name))
    try:
        document = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        pytest.fail(f"{name} не разбирается как YAML: {exc}")
    if not isinstance(document, dict):
        pytest.fail(f"{name} должен быть YAML-отображением")
    return document


def _code_lines(text: str) -> list[str]:
    return [line for line in text.splitlines() if not line.lstrip().startswith("#")]


def _unquote(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
        return value[1:-1].strip()
    return value


def _is_secret_value(value: str) -> bool:
    raw = _unquote(value)
    if raw == "":
        return False
    return _ENV_REFERENCE.fullmatch(raw) is None


def _secret_assignment(line: str) -> str | None:
    left, separator, right = line.partition(":")
    if separator == "":
        return None
    key = left.strip().lstrip("-").strip().casefold()
    if key.endswith(("password", "token")) and _is_secret_value(right):
        return key
    return None


def _service_header_at(text: str, service: str) -> int:
    match = re.search(
        rf"(?m)^[ \t]+{re.escape(service)}:[ \t]*(?:#.*)?$",
        text,
    )
    if match is None:
        pytest.fail(f"нет ключа сервиса {service}")
    return match.start()


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_compose_parses_as_yaml(name: str) -> None:
    _load_compose(name)


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_compose_services_present(name: str) -> None:
    services = _load_compose(name).get("services")
    if not isinstance(services, dict):
        pytest.fail(f"{name}: services должен быть отображением")
    missing = [service for service in REQUIRED_SERVICES if service not in services]
    if missing:
        pytest.fail(f"{name}: нет сервисов {', '.join(missing)}")


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_compose_has_no_latest_tag(name: str) -> None:
    text = _read(_compose_path(name))
    if "latest" in text:
        pytest.fail(f"{name}: тег latest запрещён")


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_compose_has_no_secret_values(name: str) -> None:
    text = _read(_compose_path(name))
    literals = []
    for line in _code_lines(text):
        key = _secret_assignment(line)
        if key is not None:
            literals.append(key)
    if literals:
        pytest.fail(f"{name}: литерал секрета у {', '.join(literals)}")
    if _HEX_RUN.search(text):
        pytest.fail(f"{name}: длинная hex-строка")


def test_backup_sh_does_not_compose_up() -> None:
    text = _read(DEPLOY / "backup.sh")
    if "docker compose up" in text:
        pytest.fail("backup.sh содержит docker compose up")


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_migration_mentioned_before_app(name: str) -> None:
    text = _read(_compose_path(name))
    mention = _MIGRATION.search(text)
    if mention is None:
        pytest.fail(f"{name}: миграция не упомянута")
    for service in APP_SERVICES:
        if mention.start() > _service_header_at(text, service):
            pytest.fail(f"{name}: миграция упомянута после сервиса {service}")


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_local_test_login_disabled(name: str) -> None:
    text = "\n".join(_code_lines(_read(_compose_path(name))))
    values = [_unquote(match).casefold() for match in _DEV_MODE.findall(text)]
    if not values:
        pytest.fail(f"{name}: AUTH_DEV_MODE не выключен")
    enabled = [value for value in values if value not in DISABLED_DEV_MODE]
    if enabled:
        pytest.fail(f"{name}: AUTH_DEV_MODE включён")
