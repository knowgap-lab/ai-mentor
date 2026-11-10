#!/usr/bin/env python3
"""Проверяет id_check-строки docs/catalog/schema.examples.yaml по catalog.schema.json и наличие правил в schema.md."""

import json
import pathlib
import sys

import yaml
from jsonschema import Draft202012Validator

root = pathlib.Path(__file__).resolve().parents[2]
schema = json.loads(
    (root / "docs/contracts/catalog.schema.json").read_text(encoding="utf8")
)
spec = (root / "docs/catalog/schema.md").read_text(encoding="utf8")
bad = 0
cases = yaml.safe_load(
    (root / "docs/catalog/schema.examples.yaml").read_text(encoding="utf8")
)
ids = [c["id"] for c in cases]
if len(ids) != len(set(ids)):
    bad += 1
    print("FAIL повтор id сценария")
for c in cases:
    if f"**{c['rule']}.**" not in spec:
        bad += 1
        print(f"FAIL {c['id']}: правило {c['rule']} не найдено в schema.md")
    chk = c.get("id_check")
    if chk:
        v = Draft202012Validator(
            {"$defs": schema["$defs"], "$ref": "#/$defs/" + chk["def"]}
        )
        ok = not list(v.iter_errors(chk["value"]))
        if ok != chk["valid"]:
            bad += 1
            print(f"FAIL {c['id']}: {chk['value']} ожидалось valid={chk['valid']}")
print(f"{'FAILED' if bad else 'OK'}: {bad} ошибок, строк {len(cases)}")
sys.exit(1 if bad else 0)
