#!/usr/bin/env python3
"""Проверяет имена в env-файле. Значения не печатает."""

import sys


def main() -> int:
    path = sys.argv[1]
    names = sys.argv[2:]
    present: dict[str, str] = {}
    with open(path, encoding="utf-8") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            present[key] = value.strip().strip('"').strip("'")
    missing = [name for name in names if not present.get(name)]
    if missing:
        print("missing secret names: " + ", ".join(missing), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
