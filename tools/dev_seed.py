"""Сид локальной базы: два синтетических человека и демо-каталог.

Повторный запуск не добавляет строки и не переписывает уже записанные,
включая ``loaded_at``. Паролей и настоящих имён нет.
"""

from __future__ import annotations

import os
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
CONTENT = REPO / "content"
DEMO_CATALOG = CONTENT / "tests" / "fixtures" / "demo-catalog"
DEFAULT_DSN = "postgres://postgres@127.0.0.1:5432/ai_mentor?sslmode=disable"

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


def main() -> None:
    dsn = os.environ.get("DATABASE_URL") or DEFAULT_DSN
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
    print(dsn)
    print(f"POST /auth/dev telegram_id={TUTOR_TELEGRAM}")
    print(f"POST /auth/dev telegram_id={STUDENT_TELEGRAM}")


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


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(error, file=sys.stderr)
        raise SystemExit(1) from error
