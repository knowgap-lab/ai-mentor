"""Сервис Gotenberg в compose: точная версия, только внутренняя сеть.

Красный результат ожидаем, пока сервиса нет в compose. Секретов здесь нет.
Имя переменной адреса тест не выбирает: ищет единственное уже записанное.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml


DEPLOY = Path(__file__).resolve().parent
COMPOSE_NAMES = ("compose.prod.yml", "compose.staging.yml")
_GOTENBERG_NAME = re.compile(r"\b([A-Z][A-Z0-9_]*GOTENBERG[A-Z0-9_]*)\b")
_NAME_FILES = ("secrets.md", "README.md", "compose.prod.yml", "compose.staging.yml")


def _read(path: Path) -> str:
    if not path.is_file():
        pytest.fail(f"нет файла {path.name}")
    return path.read_text(encoding="utf-8")


def _load_compose(name: str) -> dict:
    text = _read(DEPLOY / name)
    try:
        document = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        pytest.fail(f"{name} не разбирается как YAML: {exc}")
    if not isinstance(document, dict):
        pytest.fail(f"{name} должен быть YAML-отображением")
    return document


def _gotenberg(name: str) -> dict:
    services = _load_compose(name).get("services")
    if not isinstance(services, dict) or "gotenberg" not in services:
        pytest.fail(f"{name}: нет сервиса gotenberg")
    service = services["gotenberg"]
    if not isinstance(service, dict):
        pytest.fail(f"{name}: gotenberg должен быть отображением")
    return service


def _pinned(image: object) -> bool:
    if not isinstance(image, str) or "latest" in image.lower():
        return False
    tag = image.rsplit(":", 1)[-1].split("@", 1)[0]
    parts = tag.split(".")
    if len(parts) != 3:
        return False
    return all(part.isdigit() and part != "" for part in parts)


def _published(service: dict) -> bool:
    ports = service.get("ports")
    if isinstance(ports, list) and ports:
        return True
    return service.get("network_mode") == "host"


@pytest.mark.parametrize("name", COMPOSE_NAMES)
def test_s8_gotenberg_internal_pinned(name: str) -> None:
    service = _gotenberg(name)
    image = service.get("image")
    if not _pinned(image):
        pytest.fail(f"{name}: образ Gotenberg не закреплён точной версией: {image!r}")
    if _published(service):
        pytest.fail(f"{name}: gotenberg публикует порт наружу")
    prod = _gotenberg(COMPOSE_NAMES[0]).get("image")
    if image != prod:
        pytest.fail(f"{name}: образ {image!r} не совпадает с prod {prod!r}")


def test_s8_gotenberg_address_named() -> None:
    names: set[str] = set()
    for name in _NAME_FILES:
        names.update(_GOTENBERG_NAME.findall(_read(DEPLOY / name)))
    if not names:
        pytest.fail(
            "имя переменной адреса Gotenberg не записано в deploy/secrets.md и deploy/README.md"
        )
    if len(names) > 1:
        pytest.fail("несколько имён адреса Gotenberg: " + ", ".join(sorted(names)))
