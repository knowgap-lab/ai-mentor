#!/usr/bin/env python3
"""Прогоняет проверочные примеры контрактов (docs/contracts/fixtures/*.cases.yaml).

Использование: python3 docs/contracts/check.py [файл.cases.yaml ...]
Зависимости: jsonschema, pyyaml, openapi-spec-validator.
Каждый случай: {name, schema, def, valid: bool, doc}. Для OpenAPI — {openapi: файл}.
"""
import glob, json, sys, pathlib
import yaml
from jsonschema import Draft202012Validator
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012

root = pathlib.Path(__file__).parent
registry = Registry()
for f in root.glob("*.schema.json"):
    data = json.loads(f.read_text(encoding="utf8"))
    registry = registry.with_resource(data["$id"], Resource.from_contents(data, DRAFT202012))

def validator(schema_file, definition):
    data = json.loads((root / schema_file).read_text(encoding="utf8"))
    sub = {"$ref": data["$id"] + ("#/$defs/" + definition if definition else "")}
    return Draft202012Validator(sub, registry=registry)

failed = 0
files = sys.argv[1:] or sorted(glob.glob(str(root / "fixtures" / "*.cases.yaml")))
for path in files:
    cases = yaml.safe_load(open(path, encoding="utf8"))
    for case in cases:
        if "openapi" in case:
            from openapi_spec_validator import validate
            try:
                validate(yaml.safe_load(open(root / case["openapi"], encoding="utf8")))
                ok = True
            except Exception as e:  # noqa: BLE001
                ok = False
                err = str(e)[:200]
            want = case.get("valid", True)
            name = case["openapi"]
        else:
            v = validator(case["schema"], case.get("def"))
            errors = list(v.iter_errors(case["doc"]))
            ok = not errors
            err = errors[0].message[:200] if errors else ""
            want = case["valid"]
            name = case["name"]
        if ok != want:
            failed += 1
            print(f"FAIL {path.split('/')[-1]}: {name} (ожидалось {'valid' if want else 'invalid'}) {err}")

# Политика WB-R4: поля с ключом ответа допустимы только в разрешённых схемах core-api.yaml.
ALLOWED_KEY_HOLDERS = {"Preparation", "PreparationItem", "AnswerResponse", "SubmitResult"}
api = yaml.safe_load(open(root / "core-api.yaml", encoding="utf8"))
def walk(node, holder, found):
    if isinstance(node, dict):
        props = node.get("properties", {})
        for k in props:
            if k in {"key", "correct", "solution"} and holder not in ALLOWED_KEY_HOLDERS:
                found.append(f"{holder}.{k}")
        for k, v in node.items():
            walk(v, holder, found)
    elif isinstance(node, list):
        for v in node:
            walk(v, holder, found)
leaks = []
for name, sch in api["components"]["schemas"].items():
    walk(sch, name, leaks)
for leak in leaks:
    failed += 1
    print(f"FAIL политика WB-R4: поле ключа в {leak}")
print(f"{'FAILED' if failed else 'OK'}: {failed} ошибок")
sys.exit(1 if failed else 0)
