#!/usr/bin/env python3
"""Опись выпуска 1: что из контрактов и web.md есть в работающем приложении.

Операции: каждая операция core-api.yaml и internal-api.yaml должна быть объявлена
маршрутом в пакете Go, который входит в зависимости сервера (`go list -deps ./cmd/server`).
Экраны: каждый экран из таблиц «Ученик: экраны» и «Репетитор: экраны» в web.md должен
быть в реестре web/screens.json, а его маршрут — в `path="…"` одного из файлов web/src.

Код выхода 0 — всё на месте, 1 — чего-то нет. `--json` печатает машиночитаемый итог.
"""

import json
import re
import subprocess
import sys
from pathlib import Path

import yaml

root = Path(__file__).resolve().parent.parent

# Операции вне выпуска 1. Ключ — operationId, значение — причина со ссылкой.
EXCLUDED = {
    "putRtImport": "импорт РТ — выпуск 2+ (docs/platform/overview.md)",
}

METHODS = ("get", "post", "put", "patch", "delete")


def contract_ops():
    ops = []
    for name in ("core-api.yaml", "internal-api.yaml"):
        spec = yaml.safe_load(
            (root / "docs/contracts" / name).read_text(encoding="utf-8")
        )
        for path, item in spec["paths"].items():
            for method, op in item.items():
                if method in METHODS and isinstance(op, dict) and "operationId" in op:
                    ops.append((op["operationId"], method.upper(), path, name))
    return ops


def server_packages():
    out = subprocess.run(
        ["go", "list", "-deps", "-f", "{{.Dir}}", "./cmd/server"],
        cwd=root / "core",
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    return {Path(line) for line in out.splitlines() if line.startswith(str(root))}


ROUTE = re.compile(r'"(GET|POST|PUT|PATCH|DELETE) (/[^"\s]*)"')
MANUAL = re.compile(
    r'r\.Method == http\.Method(Get|Post|Put|Patch|Delete) && r\.URL\.Path == "(/[^"]*)"'
)


def declared_routes(packages):
    routes = {}
    for go in (root / "core").rglob("*.go"):
        if go.name.endswith("_test.go") or go.parent not in packages:
            continue
        text = go.read_text(encoding="utf-8")
        for method, path in ROUTE.findall(text):
            routes.setdefault((method, path), go.relative_to(root))
        for method, path in MANUAL.findall(text):
            routes.setdefault((method.upper(), path), go.relative_to(root))
    return routes


def spec_screens():
    text = (root / "docs/platform/web.md").read_text(encoding="utf-8")
    screens = []
    for role, heading in (
        ("Ученик", "### Ученик: экраны"),
        ("Репетитор", "### Репетитор: экраны"),
    ):
        start = text.index(heading)
        end = text.index("\n### ", start + len(heading))
        for line in text[start:end].splitlines():
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            if (
                len(cells) >= 2
                and cells[0] not in ("Экран", "")
                and not set(cells[0]) <= set("-: ")
            ):
                screens.append(f"{role}/{cells[0]}")
    return screens


def web_routes():
    found = set()
    for tsx in (root / "web/src").rglob("*.tsx"):
        if ".test." in tsx.name:
            continue
        found.update(re.findall(r'path="([^"]+)"', tsx.read_text(encoding="utf-8")))
    return found


def main():
    ops = contract_ops()
    routes = declared_routes(server_packages())
    missing_ops = [
        {"operationId": op, "method": m, "path": p, "contract": c}
        for op, m, p, c in ops
        if op not in EXCLUDED and (m, p) not in routes
    ]

    registry_file = root / "web/screens.json"
    registry = (
        json.loads(registry_file.read_text(encoding="utf-8"))
        if registry_file.exists()
        else {}
    )
    paths = web_routes()
    missing_screens = []
    for screen in spec_screens():
        route = registry.get(screen)
        if route is None:
            missing_screens.append({"screen": screen, "why": "нет в web/screens.json"})
        elif route not in paths:
            missing_screens.append(
                {"screen": screen, "why": f"маршрута {route} нет в web/src"}
            )

    result = {
        "operations_total": len(ops) - len(EXCLUDED),
        "operations_missing": missing_ops,
        "screens_total": len(spec_screens()),
        "screens_missing": missing_screens,
        "excluded": EXCLUDED,
    }
    if "--json" in sys.argv:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(
            f"Операции: {result['operations_total'] - len(missing_ops)} из {result['operations_total']} подключены к серверу"
        )
        for o in missing_ops:
            print(
                f"  нет: {o['operationId']} {o['method']} {o['path']} ({o['contract']})"
            )
        print(
            f"Экраны: {result['screens_total'] - len(missing_screens)} из {result['screens_total']} есть в вебе"
        )
        for s in missing_screens:
            print(f"  нет: {s['screen']} — {s['why']}")
    return 1 if missing_ops or missing_screens else 0


if __name__ == "__main__":
    sys.exit(main())
