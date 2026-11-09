"""Командная строка импорта плана."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from dataclasses import asdict
from pathlib import Path

from .importer import import_plan


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m plan_import")
    parser.add_argument("source", type=Path)
    parser.add_argument("--student", required=True)
    parser.add_argument("--subject", required=True)
    parser.add_argument("--shift-weeks", type=int, default=0)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--already-imported", action="store_true")
    parser.add_argument("--today")
    parser.add_argument("--json", action="store_true")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    if args.today is None:
        today = None
    else:
        try:
            today = dt.date.fromisoformat(args.today)
        except ValueError:
            print("дата --today должна быть YYYY-MM-DD", file=sys.stderr)
            return 2
    try:
        result = import_plan(
            source=args.source,
            student_id=args.student,
            subject=args.subject,
            shift_weeks=args.shift_weeks,
            already_imported=args.already_imported,
            dry_run=args.dry_run,
            today=today,
        )
    except ValueError as exc:
        print(exc, file=sys.stderr)
        return 2
    if args.json:
        print(json.dumps(asdict(result), ensure_ascii=False))
    else:
        for item in result.errors:
            print(f"{item['file']}: {item['code']}: {item['message']}")
        for item in result.warnings:
            print(f"warning: {item['file']}: {item['code']}: {item['message']}")
    return 1 if result.errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
