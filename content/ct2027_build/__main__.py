"""CLI: python -m ct2027_build <out_dir>"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .build import build_catalog


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m ct2027_build")
    parser.add_argument("out_dir", type=Path)
    parser.add_argument("--source", type=Path, default=None)
    args = parser.parse_args(argv)
    result = build_catalog(args.out_dir, source=args.source)
    if result.errors:
        for line in result.errors:
            print(line, file=sys.stderr)
        return 1
    print(f"lessons: {', '.join(result.lessons)}")
    print(result.out_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
