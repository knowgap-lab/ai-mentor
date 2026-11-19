"""Составитель одного синтетического урока: карточка и план → каталог.

Команды из каталога ``content``:

    python -m composer tests/fixtures/composer/one-lesson --subject math --lesson 1 --out <каталог>
    python -m catalog_ci check <каталог> --mode main
"""

from __future__ import annotations

import json
import secrets
import shutil
import subprocess
import sys
import time
from collections.abc import Iterator
from decimal import Decimal
from pathlib import Path

import pytest
import yaml

import catalog_artifact
from catalog_ci import check_catalog
from composer import compose_lesson, level_hours, stage_kind


FIXTURE = Path(__file__).parent / "fixtures" / "composer" / "one-lesson"
CONTENT = Path(__file__).resolve().parents[1]
SCHEMA = Path(__file__).parent / "fixtures" / "loader" / "schema.sql"
POSTGRES_IMAGE = "postgres:16.15"
LESSON_ID = "ct-math:L01"


def copy_source(tmp_path: Path) -> Path:
    target = tmp_path / "source"
    shutil.copytree(FIXTURE, target)
    return target


def replace(path: Path, old: str, new: str) -> None:
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise AssertionError(f"{old!r} не найден")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")


def compose(tmp_path: Path):
    source = copy_source(tmp_path)
    out = tmp_path / "catalog"
    result = compose_lesson(source, "math", 1, write_dir=out)
    return source, out, result


def lesson_document(out: Path) -> dict:
    return yaml.safe_load((out / "ct-math" / "lessons" / "L01.yaml").read_text(encoding="utf-8"))


def test_stage_kind_follows_d302() -> None:
    assert stage_kind("Разминка-вспоминание") == "warmup"
    assert stage_kind("Разбор ДЗ") == "homework_review"
    assert stage_kind("Ошибки по типам") == "homework_review"
    assert stage_kind("Новая теория") == "theory"
    assert stage_kind("Новая тема") == "theory"
    assert stage_kind("Памятки") == "theory"
    assert stage_kind("Разбор: дроби") == "theory"
    assert stage_kind("Практика") == "practice"
    assert stage_kind("Перерешивание") == "practice"
    assert stage_kind("Выходной мини-тест") == "exit_test"
    assert stage_kind("Срез") == "exit_test"
    assert stage_kind("Диагностика") == "exit_test"
    assert stage_kind("ДЗ") == "homework_handout"
    assert stage_kind("Стратегия и ДЗ") == "homework_handout"
    assert stage_kind("Свободная работа") is None


def test_level_hours_match_examples() -> None:
    assert level_hours(Decimal("4.5"), 2, 6) == (0.5, 1)
    assert level_hours(Decimal("0.5"), 1, 1) == (0.25, 0.5)


def test_one_lesson_is_deterministic_and_passes_catalog_ci(tmp_path: Path) -> None:
    source, out, result = compose(tmp_path)
    assert result.errors == ()
    assert result.warnings == ()
    again = compose_lesson(source, "math", 1)
    assert again.files == result.files
    assert result.report["created_types"] == 1
    assert result.report["created_blocks"] == 1
    assert result.report["created_variants"] == 8
    assert result.report["created_stages"] == 2

    lesson = lesson_document(out)
    assert lesson["id"] == LESSON_ID
    assert lesson["topics"] == ["ct-math:D1"]
    assert lesson["review"] == []
    assert lesson["duration_min"] == 60
    assert lesson["generation_notes"] == "Числа целые."
    assert [stage["kind"] for stage in lesson["stages"]] == ["theory", "practice"]
    assert lesson["stages"][0]["minutes"] == [0, 30]
    assert lesson["stages"][0]["blocks"] == ["ct-math:D1.B01"]
    assert lesson["stages"][1]["minutes"] == [30, 60]
    assert lesson["stages"][1]["blocks"] == []
    practice = lesson["stages"][1]["practice"]
    assert practice == [
        {
            "type": "ct-math:D1.T01",
            "subvariant": "ct-math:D1.T01.S1",
            "role": "solve_alone",
            "theory_block": "ct-math:D1.B01",
        }
    ]
    assert lesson["homework"] == {
        "budget_hours": 1,
        "items": [
            {
                "source": "new_topic",
                "series": [{"type": "ct-math:D1.T01", "count": 2}],
                "difficulty": [1, 1],
            }
        ],
    }
    lesson_text = (out / "ct-math" / "lessons" / "L01.yaml").read_text(encoding="utf-8")
    assert "карточки" not in lesson_text
    assert "перечитать" not in lesson_text

    topic = yaml.safe_load((out / "ct-math" / "topics" / "D1.yaml").read_text(encoding="utf-8"))
    task_type = topic["types"][0]
    assert task_type["answer_format"] == "ct.integer"
    assert task_type["kind"] == "exam"
    assert task_type["levels"] == {"base": {"hours": 0.25}, "automatism": {"hours": 0.5}}
    assert task_type["subvariants"] == [
        {"id": "ct-math:D1.T01.S1", "given": "два целых числа / их сумму"}
    ]
    variants = yaml.safe_load(
        (out / "ct-math" / "variants" / "D1" / "T01.yaml").read_text(encoding="utf-8")
    )
    assert variants["variants"][0]["statement"] == "Найдите значение выражения $2 + 3$."
    assert variants["variants"][0]["key"] == 5
    assert len(variants["variants"]) == 8
    block = (out / "ct-math" / "blocks" / "D1" / "B01.md").read_text(encoding="utf-8")
    assert "Сумма $a + b$ находится по определению." in block

    problems = check_catalog(out, mode="main")
    assert problems == []


def test_conditional_practice_stays_linear(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    lesson = source / "math" / "plan" / "L01.md"
    text = lesson.read_text(encoding="utf-8")
    extra = (
        "- если шаг 1 не объясняли — Практика, сам. "
        "D1 §Банк 1 × 1. Опора: D1 §Теория шаг 1\n"
    )
    lesson.write_text(text.replace("## Домашняя работа", extra + "\n## Домашняя работа"), encoding="utf-8")
    result = compose_lesson(source, "math", 1, write_dir=tmp_path / "catalog")
    assert result.errors == ()
    practice = lesson_document(tmp_path / "catalog")["stages"][1]["practice"]
    assert len(practice) == 2
    assert practice[1]["role"] == "solve_alone"
    assert practice[1]["note"].startswith("если шаг 1 не объясняли")


def test_stages_gap_blocks_the_catalog(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    replace(source / "math" / "plan" / "L01.md", "30–60", "30–58")
    out = tmp_path / "catalog"
    result = compose_lesson(source, "math", 1, write_dir=out)
    assert any(item["code"] == "stages_gap" for item in result.errors)
    assert result.files is None
    assert not out.exists()


def test_unknown_stage_is_not_guessed(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    replace(source / "math" / "plan" / "L01.md", "| Практика |", "| Свободная работа |")
    result = compose_lesson(source, "math", 1)
    assert result.files is None
    assert any("Свободная работа" in item["message"] and item["code"] == "" for item in result.errors)


def test_unknown_bank_row_stops(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    replace(
        source / "math" / "plan" / "L01.md",
        "Практика, сам. D1 §Банк 1",
        "Практика, сам. D1 §Банк 9",
    )
    result = compose_lesson(source, "math", 1)
    assert result.files is None
    assert any(item["code"] == "bank_ref_unknown" for item in result.errors)


def test_missing_theory_block_stops(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    replace(
        source / "math" / "plan" / "L01.md",
        " × 1. Опора: D1 §Теория шаг 1",
        " × 1",
    )
    result = compose_lesson(source, "math", 1)
    assert result.files is None
    assert any(item["code"] == "theory_block_unknown" for item in result.errors)


def test_homework_budget_mismatch_warns_and_still_writes(tmp_path: Path) -> None:
    source = copy_source(tmp_path)
    replace(source / "math" / "plan" / "L01.md", "≈ 1 ч", "≈ 0.01 ч")
    out = tmp_path / "catalog"
    result = compose_lesson(source, "math", 1, write_dir=out)
    assert result.errors == ()
    assert any(item["code"] == "homework_budget_mismatch" for item in result.warnings)
    assert check_catalog(out, mode="main") == []


def test_cli_writes_a_catalog_ci_accepts(tmp_path: Path) -> None:
    out = tmp_path / "out"
    completed = subprocess.run(
        [
            sys.executable,
            "-m",
            "composer",
            str(FIXTURE),
            "--subject",
            "math",
            "--lesson",
            "1",
            "--out",
            str(out),
        ],
        cwd=CONTENT,
        check=False,
        capture_output=True,
        text=True,
    )
    assert completed.returncode == 0, completed.stderr
    checked = subprocess.run(
        [sys.executable, "-m", "catalog_ci", "check", str(out), "--mode", "main"],
        cwd=CONTENT,
        check=False,
        capture_output=True,
        text=True,
    )
    assert checked.returncode == 0, checked.stdout + checked.stderr
    assert (out / "ct-math" / "lessons" / "L01.yaml").is_file()


def _docker(args: list[str], timeout: float) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=False,
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def _trim(text: str) -> str:
    text = text.strip()
    return text if len(text) <= 400 else text[:400]


@pytest.fixture
def dsn() -> Iterator[str]:
    if shutil.which("docker") is None or shutil.which("psql") is None:
        pytest.skip("docker or psql unavailable")
    info = _docker(["info", "--format", "{{.ServerVersion}}"], timeout=20)
    if info.returncode != 0:
        pytest.skip(f"docker unavailable: {_trim(info.stderr)}")
    image = _docker(["image", "inspect", POSTGRES_IMAGE], timeout=20)
    if image.returncode != 0:
        pytest.skip(f"image {POSTGRES_IMAGE} unavailable")

    name = "ai-mentor-composer-" + secrets.token_hex(4)
    volume = name + "-data"
    container_id: str | None = None
    try:
        started = _docker(
            [
                "run",
                "-d",
                "--rm",
                "--pull",
                "never",
                "--name",
                name,
                "-v",
                f"{volume}:/var/lib/postgresql/data",
                "-e",
                "POSTGRES_HOST_AUTH_METHOD=trust",
                "-e",
                "POSTGRES_DB=ai_mentor_test",
                "-p",
                "127.0.0.1::5432",
                POSTGRES_IMAGE,
            ],
            timeout=30,
        )
        container_id = started.stdout.strip() or None
        if started.returncode != 0 or container_id is None:
            raise AssertionError(f"docker run: {_trim(started.stderr)}")
        port = ""
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            published = _docker(["port", container_id, "5432/tcp"], timeout=2)
            for line in published.stdout.splitlines():
                if line.strip().startswith("127.0.0.1:"):
                    port = line.strip().removeprefix("127.0.0.1:")
                    break
            if port:
                break
            time.sleep(0.2)
        if not port:
            raise AssertionError("postgres port was not published")
        admin = f"postgresql://postgres@127.0.0.1:{port}/ai_mentor_test?sslmode=disable&connect_timeout=2"
        ready = time.monotonic() + 60
        last_error = "not started"
        while time.monotonic() < ready:
            probe = subprocess.run(
                ["psql", admin, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-t", "-A", "-c", "SELECT 1"],
                check=False,
                capture_output=True,
                text=True,
            )
            if probe.returncode == 0:
                break
            last_error = _trim(probe.stderr or probe.stdout)
            time.sleep(0.3)
        else:
            raise AssertionError(f"postgres not ready: {last_error}")
        subprocess.run(
            ["psql", admin, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-f", str(SCHEMA)],
            check=True,
            capture_output=True,
            text=True,
        )
        yield f"postgresql://loader@127.0.0.1:{port}/ai_mentor_test?sslmode=disable&connect_timeout=2"
    finally:
        if container_id:
            _docker(["rm", "-f", container_id], timeout=30)
        _docker(["volume", "rm", volume], timeout=15)


def test_composed_lesson_loads_into_empty_postgres(dsn: str, tmp_path: Path) -> None:
    out = tmp_path / "catalog"
    result = compose_lesson(FIXTURE, "math", 1, write_dir=out)
    assert result.errors == ()
    assert check_catalog(out, mode="main") == []
    artifact_path = tmp_path / "catalog-1.tar.zst"
    artifact = catalog_artifact.build_artifact(
        out,
        ref="main",
        commit="composer-c1",
        catalog_version=1,
        out=artifact_path,
    )
    import loader

    loaded = loader.load_artifact(dsn, artifact_path, environment="staging")
    assert loaded.catalog_version == 1
    assert loaded.new is True
    stored = subprocess.run(
        [
            "psql",
            dsn,
            "-v",
            "ON_ERROR_STOP=1",
            "-X",
            "-q",
            "-t",
            "-A",
            "-c",
            "SELECT data::text FROM catalog.revision WHERE entity_id = 'ct-math:L01'",
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    row = json.loads(stored.stdout.strip())
    assert row["id"] == LESSON_ID
    assert row["duration_min"] == 60
    assert len(artifact.entities) > 0
