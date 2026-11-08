#!/usr/bin/env python3
"""Проверяет текстовые *.examples.yaml (given/expect): число строк = строкам таблицы спецификации,
все правила существуют в спецификации, id уникальны, ячейки не пусты."""
import pathlib, re, sys
import yaml

root = pathlib.Path(__file__).resolve().parents[2]
bad = 0
checked = 0
for ex in sorted((root / "docs").rglob("*.examples.yaml")):
    cases = yaml.safe_load(ex.read_text(encoding="utf8"))
    if not cases or not isinstance(cases[0].get("expect"), str):
        continue  # сценарные файлы проверяются другими скриптами
    checked += 1
    md = ex.with_name(ex.name.replace(".examples.yaml", ".md"))
    spec = md.read_text(encoding="utf8")
    sec = spec[spec.index("## 4. Таблицы примеров"):spec.index("## 5.")]
    table_rows = [l for l in sec.splitlines() if l.startswith("|")][2:]
    if len(table_rows) != len(cases):
        bad += 1; print(f"FAIL {ex.name}: строк в таблице {len(table_rows)}, в YAML {len(cases)}")
    ids = [c["id"] for c in cases]
    if len(set(ids)) != len(ids):
        bad += 1; print(f"FAIL {ex.name}: повтор id")
    for c in cases:
        for f in ("rule", "given", "expect"):
            if not str(c.get(f, "")).strip():
                bad += 1; print(f"FAIL {c['id']}: пустое поле {f}")
        for rule in re.findall(r"[A-Z]{2}-R\d+[a-z]?", str(c["rule"])):
            if f"**{rule}.**" not in spec:
                bad += 1; print(f"FAIL {c['id']}: правило {rule} не найдено в {md.name}")
print(f"{'FAILED' if bad else 'OK'}: {bad} ошибок, файлов {checked}")
sys.exit(1 if bad else 0)
