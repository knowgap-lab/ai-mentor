"""Сид локальной базы: два синтетических человека, демо-каталог и экраны выпуска 1.

Без ``DATABASE_URL`` поднимает чистую Postgres ``16.15`` (контейнер
``ai-mentor-l1``) и применяет миграции, если схемы ещё нет. С ``DATABASE_URL``
пишет в уже подготовленную базу и контейнер не трогает.

Повторный запуск не добавляет строки и не переписывает уже записанные,
включая ``loaded_at``. Паролей и настоящих имён нет.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
CONTENT = REPO / "content"
_CATALOG_OVERRIDE = os.environ.get("AI_MENTOR_DEV_CATALOG", "").strip()
DEMO_CATALOG = (
    Path(_CATALOG_OVERRIDE)
    if _CATALOG_OVERRIDE
    else CONTENT / "tests" / "fixtures" / "ct-2027-catalog"
)
SCENARIO = Path(__file__).resolve().parent / "dev_seed_scenario.sql"
MIGRATIONS = REPO / "migrations"
DEFAULT_DSN = "postgres://postgres@127.0.0.1:55432/ai_mentor?sslmode=disable"
POSTGRES_IMAGE = "postgres:16.15"
CONTAINER = "ai-mentor-l1"

TUTOR_ID = "tutor-demo"
STUDENT_ID = "student-demo"
TUTOR_TELEGRAM = "tg-demo-tutor"
STUDENT_TELEGRAM = "tg-demo-student"

# Версия фиксирована, чтобы повтор попал в уже загруженную голову
# и загрузчик не открыл вторую строку catalog.load.
CATALOG_VERSION = 1
CATALOG_COMMIT = "dev-seed"

_SEED_SQL = f"""
BEGIN;
INSERT INTO core.app_user (id, role, owner_tutor_id, status)
SELECT '{TUTOR_ID}', 'tutor', NULL, 'active'
WHERE NOT EXISTS (SELECT 1 FROM core.app_user WHERE id = '{TUTOR_ID}');
INSERT INTO core.app_user (id, role, owner_tutor_id, status)
SELECT '{STUDENT_ID}', 'student', '{TUTOR_ID}', 'active'
WHERE NOT EXISTS (SELECT 1 FROM core.app_user WHERE id = '{STUDENT_ID}');
INSERT INTO core.user_pii (user_id, telegram_id)
SELECT '{TUTOR_ID}', '{TUTOR_TELEGRAM}'
WHERE NOT EXISTS (SELECT 1 FROM core.user_pii WHERE user_id = '{TUTOR_ID}');
INSERT INTO core.user_pii (user_id, telegram_id)
SELECT '{STUDENT_ID}', '{STUDENT_TELEGRAM}'
WHERE NOT EXISTS (SELECT 1 FROM core.user_pii WHERE user_id = '{STUDENT_ID}');
COMMIT;
"""

_SUMMARY_SQL = """
SELECT json_build_object(
    'users', (SELECT count(*) FROM core.app_user),
    'lessons', (SELECT count(*) FROM core.lesson),
    'drafts', (SELECT count(*) FROM core.homework_draft),
    'assignments', (SELECT count(*) FROM core.assignment),
    'attempts', (SELECT count(*) FROM core.attempts),
    'journal', (SELECT count(*) FROM core.journal_event),
    'loads', (SELECT count(*) FROM catalog.load)
)::text
"""


def main() -> None:
    given = os.environ.get("DATABASE_URL", "").strip()
    dsn = given or ensure_database()
    sys.path.insert(0, str(CONTENT))
    import catalog_artifact
    from loader import load_artifact

    _psql(dsn, _SEED_SQL)
    with tempfile.TemporaryDirectory() as directory:
        artifact_path = Path(directory) / "demo-catalog.tar.zst"
        catalog_artifact.build_artifact(
            DEMO_CATALOG,
            ref="main",
            commit=CATALOG_COMMIT,
            catalog_version=CATALOG_VERSION,
            out=artifact_path,
        )
        load_artifact(dsn, artifact_path, environment="staging")
    version = _psql_value(
        dsn,
        "SELECT catalog_version::text FROM catalog.load "
        "WHERE environment = 'staging' ORDER BY catalog_version LIMIT 1",
    )
    if not version.isdigit():
        raise RuntimeError(f"нечисловая версия каталога: {version!r}")
    scenario = SCENARIO.read_text(encoding="utf-8")
    if scenario.count("__CATALOG_VERSION__") < 1:
        raise RuntimeError(f"в {SCENARIO} нет версии каталога")
    _psql(dsn, scenario.replace("__CATALOG_VERSION__", version))
    summary = _psql_value(dsn, _SUMMARY_SQL)
    print(dsn)
    print(f"POST /auth/dev telegram_id={TUTOR_TELEGRAM}")
    print(f"POST /auth/dev telegram_id={STUDENT_TELEGRAM}")
    print("lesson lesson-demo-held")
    print("draft lesson-demo-next")
    print("assignment asg-demo-submitted")
    print(f"seed {summary}")


def ensure_database() -> str:
    """Контейнер ai-mentor-l1. Повтор использует его, не создавая вторую базу."""
    if shutil.which("docker") is None:
        raise RuntimeError("docker недоступен, задайте DATABASE_URL")
    image = _docker(["image", "inspect", POSTGRES_IMAGE], timeout=20)
    if image.returncode != 0:
        raise RuntimeError(f"нет образа {POSTGRES_IMAGE}")
    state = _docker(["inspect", "-f", "{{.State.Running}}", CONTAINER], timeout=10)
    if state.returncode != 0:
        started = _docker(
            [
                "run",
                "-d",
                "--name",
                CONTAINER,
                "-v",
                f"{CONTAINER}-data:/var/lib/postgresql/data",
                "-e",
                "POSTGRES_HOST_AUTH_METHOD=trust",
                "-e",
                "POSTGRES_DB=ai_mentor",
                "-p",
                "127.0.0.1:55432:5432",
                "--pull",
                "never",
                POSTGRES_IMAGE,
            ],
            timeout=30,
        )
        if started.returncode != 0:
            detail = (started.stderr or started.stdout or "").strip()
            raise RuntimeError(detail or "docker run не поднял Postgres")
    elif state.stdout.strip() != "true":
        started = _docker(["start", CONTAINER], timeout=30)
        if started.returncode != 0:
            detail = (started.stderr or started.stdout or "").strip()
            raise RuntimeError(detail or "docker start не поднял Postgres")
    _wait_ready(DEFAULT_DSN)
    if _psql_value(DEFAULT_DSN, "SELECT to_regclass('core.app_user') IS NULL") == "t":
        _apply_migrations(DEFAULT_DSN)
    return DEFAULT_DSN


def _apply_migrations(dsn: str) -> None:
    files = sorted(MIGRATIONS.glob("*.sql"))
    if not files:
        raise RuntimeError(f"нет файлов в {MIGRATIONS}")
    with tempfile.TemporaryDirectory() as directory:
        folder = Path(directory)
        for index, path in enumerate(files):
            body = _up_sql(path.read_text(encoding="utf-8"), path)
            target = folder / f"{index:02d}-{path.name}"
            target.write_text(body, encoding="utf-8")
            _psql_file(dsn, target)


def _up_sql(text: str, source: Path) -> str:
    lines = text.splitlines()
    start = None
    for index, line in enumerate(lines):
        if line.strip() == "-- +goose Up":
            start = index + 1
            break
    if start is None:
        raise RuntimeError(f"{source} не содержит -- +goose Up")
    end = len(lines)
    for index in range(start, len(lines)):
        if lines[index].strip() == "-- +goose Down":
            end = index
            break
    body = "\n".join(lines[start:end]).strip()
    if not body:
        raise RuntimeError(f"{source}: пустая секция Up")
    return body + "\n"


def _wait_ready(dsn: str) -> None:
    deadline = time.monotonic() + 60
    last = "нет ответа"
    while time.monotonic() < deadline:
        try:
            _psql_value(dsn, "SELECT 1")
        except RuntimeError as error:
            last = str(error)
            time.sleep(0.3)
            continue
        return
    raise RuntimeError(f"Postgres не готова: {last}")


def _docker(args: list[str], timeout: float) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=timeout,
    )


def _psql_value(dsn: str, sql: str) -> str:
    completed = _run_psql(dsn, ["-t", "-A", "-c", sql])
    return completed.stdout.strip()


def _psql_file(dsn: str, path: Path) -> None:
    _run_psql(dsn, ["-f", str(path)])


def _psql(dsn: str, sql: str) -> None:
    env = os.environ.copy()
    env["PGCLIENTENCODING"] = "UTF8"
    env["PSQL_PAGER"] = "off"
    completed = subprocess.run(
        ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q"],
        input=sql,
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env,
        check=False,
    )
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise RuntimeError(detail or f"psql завершился с кодом {completed.returncode}")


def _run_psql(dsn: str, args: list[str]) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env["PGCLIENTENCODING"] = "UTF8"
    env["PSQL_PAGER"] = "off"
    completed = subprocess.run(
        ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", *args],
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env,
        check=False,
    )
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise RuntimeError(detail or f"psql завершился с кодом {completed.returncode}")
    return completed


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(error, file=sys.stderr)
        raise SystemExit(1) from error
