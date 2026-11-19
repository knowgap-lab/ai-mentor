"""Командная строка составителя одного урока."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .compose import compose_lesson


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m composer")
    parser.add_argument("source", type=Path)
    parser.add_argument("--subject", required=True)
    parser.add_argument("--lesson", type=int, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--json", action="store_true")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    try:
        result = compose_lesson(
            args.source,
            args.subject,
            args.lesson,
            write_dir=args.out,
        )
    except ValueError as exc:
        print(exc, file=sys.stderr)
        return 2
    if args.json:
        print(
            json.dumps(
                {
                    "errors": list(result.errors),
                    "warnings": list(result.warnings),
                    "report": result.report,
                    "files": sorted(result.files) if result.files else None,
                },
                ensure_ascii=False,
            )
        )
    else:
        for item in result.errors:
            print(f"{item['file']}: {item['code']}: {item['message']}")
        for item in result.warnings:
            print(f"warning: {item['file']}: {item['code']}: {item['message']}")
        if result.files and not result.errors:
            for relative in result.files:
                print(relative)
    return 1 if result.errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
