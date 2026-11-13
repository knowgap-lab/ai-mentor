"""Приёмочные тесты интерфейса командной строки catalog_ci."""

from __future__ import annotations

import json
from pathlib import Path
import subprocess
import sys

from conftest import mutate_yaml


CONTENT_ROOT = Path(__file__).resolve().parents[1]
PACKAGE = "ct-demo"


def run_cli(*arguments: str) -> subprocess.CompletedProcess[str]:
    """Запускает обязательный модульный интерфейс из каталога content."""
    return subprocess.run(
        [sys.executable, "-m", "catalog_ci", *arguments],
        cwd=CONTENT_ROOT,
        capture_output=True,
        text=True,
        check=False,
    )


def add_disallowed_topic_field(catalog_root: Path) -> None:
    """Создаёт ровно одну проверяемую проблему для вывода CLI."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda document: document.__setitem__("tier", "B"),
    )


def test_cli_returns_zero_and_no_plain_output_for_valid_catalog(catalog_root: Path) -> None:
    """CS-R1–CS-R30: корректный каталог завершает CLI кодом 0 без вывода проблем."""
    result = run_cli("check", str(catalog_root))
    assert result.returncode == 0
    assert result.stdout == ""


def test_cli_returns_one_and_plain_problem_line(catalog_root: Path) -> None:
    """CS-R29: запрещённое поле печатается одной строкой обычного вывода."""
    add_disallowed_topic_field(catalog_root)
    result = run_cli("check", str(catalog_root))
    assert result.returncode == 1
    assert result.stdout.startswith(f"{PACKAGE}/topics/D1.yaml: field_not_allowed: ")
    assert result.stdout.endswith("\n")


def test_cli_returns_two_for_missing_path() -> None:
    """CS-R1–CS-R30: CLI возвращает код 2 при ошибке использования."""
    result = run_cli("check")
    assert result.returncode == 2


def test_cli_json_output_is_problem_objects(catalog_root: Path) -> None:
    """CS-R29: --json выдаёт массив объектов Problem в stdout."""
    add_disallowed_topic_field(catalog_root)
    result = run_cli("check", str(catalog_root), "--json")
    assert result.returncode == 1
    payload = json.loads(result.stdout)
    assert isinstance(payload, list)
    assert payload
    problem = next(item for item in payload if item["code"] == "field_not_allowed")
    assert set(problem) == {"code", "file", "pointer", "message"}
    assert problem["file"] == f"{PACKAGE}/topics/D1.yaml"
    assert isinstance(problem["pointer"], str)
    assert isinstance(problem["message"], str) and problem["message"]
