"""Command-line interface for ``catalog_ci``."""

from __future__ import annotations

import argparse
from dataclasses import asdict
import json
from pathlib import Path

from .checker import check_catalog


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m catalog_ci")
    commands = parser.add_subparsers(dest="command", required=True)
    check = commands.add_parser("check", help="проверить каталог")
    check.add_argument("path", type=Path)
    check.add_argument("--mode", choices=("pr", "main"), default="pr")
    check.add_argument("--json", action="store_true", dest="as_json")
    return parser


def main() -> int:
    parser = _parser()
    args = parser.parse_args()
    if not args.path.is_dir():
        parser.error("path должен быть существующим каталогом")

    problems = check_catalog(args.path, mode=args.mode)
    if args.as_json:
        print(
            json.dumps(
                [asdict(problem) for problem in problems],
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
    else:
        for problem in problems:
            print(f"{problem.file}: {problem.code}: {problem.message}")
    return 1 if problems else 0


if __name__ == "__main__":
    raise SystemExit(main())
