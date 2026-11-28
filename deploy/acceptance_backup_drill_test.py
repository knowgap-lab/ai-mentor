"""Синтаксис и обязательные шаги acceptance-backup-drill.sh без Docker и без гостя."""

from __future__ import annotations

import subprocess
from pathlib import Path

DEPLOY = Path(__file__).resolve().parent
SCRIPT = DEPLOY / "acceptance-backup-drill.sh"


def test_acceptance_backup_drill_script_is_executable() -> None:
    if not SCRIPT.is_file():
        raise AssertionError("нет deploy/acceptance-backup-drill.sh")
    text = SCRIPT.read_text(encoding="utf-8")
    for needle in (
        "acceptance_backup_drill",
        "pg_dump",
        "pg_restore",
        "records_compared",
        "ai-mentor-acceptance-restore",
    ):
        if needle not in text:
            raise AssertionError(f"нет {needle}")


def test_acceptance_backup_drill_bash_syntax() -> None:
    proc = subprocess.run(
        ["bash", "-n", str(SCRIPT)],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise AssertionError(proc.stderr or proc.stdout)
