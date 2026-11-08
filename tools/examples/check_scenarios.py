#!/usr/bin/env python3
"""Проверяет сценарные примеры (*.examples.yaml со шагами op/check) против контракта core-api.yaml (D133).

Для каждого шага с op: операция существует; запрос по схеме тела (кроме ожидаемых 422);
код ответа описан в операции; тело ответа проверяется по схеме; error.code присутствует;
правила (rule) названы в соответствующей спецификации.
Запуск: python3 tools/examples/check_scenarios.py   (нужны jsonschema, pyyaml)
"""
import re, sys, pathlib
import yaml
from jsonschema import Draft202012Validator

root = pathlib.Path(__file__).resolve().parents[2]
api = yaml.safe_load((root / "docs/contracts/core-api.yaml").read_text(encoding="utf8"))
ops = {}
for path, item in api["paths"].items():
    for method, op in item.items():
        ops[op["operationId"]] = op

def schema_for(node):
    """Схема JSON из описания тела/ответа с подставленными components."""
    return {**node, "components": api["components"]}

def resolve(ref_node):
    return schema_for(ref_node)

def expand(v):
    if isinstance(v, dict):
        if set(v) == {"$repeat"}:
            s, n = v["$repeat"]
            return s * n
        return {k: expand(x) for k, x in v.items()}
    if isinstance(v, list):
        return [expand(x) for x in v]
    return v

def body_schema(op, status, where):
    if where == "request":
        rb = op.get("requestBody")
        return rb["content"]["application/json"]["schema"] if rb else None
    resp = op["responses"].get(status)
    if resp and "$ref" in resp:
        resp = api["components"]["responses"][resp["$ref"].split("/")[-1]]
    if not resp:
        return None
    c = resp.get("content", {}).get("application/json")
    return c["schema"] if c else None

errors = []
def fail(sid, msg):
    errors.append(f"{sid}: {msg}")

def check_step(sid, step):
    if "parallel" in step:
        for s in step["parallel"]:
            check_step(sid, s)
        return
    if "op" not in step:
        return
    name = step["op"]
    op = ops.get(name)
    if not op:
        return fail(sid, f"операция {name} не найдена в core-api.yaml")
    exp = step.get("expect", {})
    status = str(exp.get("http", ""))
    if status and status not in op["responses"]:
        fail(sid, f"{name}: код {status} не описан в контракте")
    if "request" in step and status != "422":
        sch = body_schema(op, None, "request")
        if sch is None:
            fail(sid, f"{name}: у операции нет тела запроса")
        else:
            for e in Draft202012Validator(schema_for(sch)).iter_errors(expand(step["request"])):
                fail(sid, f"{name}: запрос не по схеме: {e.message[:120]}")
    if "body" in exp:
        sch = body_schema(op, status, "response")
        if sch is None:
            fail(sid, f"{name}: для {status} нет схемы тела")
        else:
            for e in Draft202012Validator(schema_for(sch)).iter_errors(exp["body"]):
                fail(sid, f"{name}: тело ответа не по схеме: {e.message[:120]}")
        for f in exp.get("no_fields", []):
            if f in exp["body"]:
                fail(sid, f"{name}: поле {f} должно отсутствовать")
    if "error" in exp:
        if not re.fullmatch(r"[a-z][a-z0-9_]*", exp["error"]["code"]):
            fail(sid, f"{name}: код ошибки {exp['error']['code']} не в формате snake_case")

SPECS = {"answer-check": "docs/platform/answer-check.md", "attempts": "docs/platform/attempts.md"}
total = 0
for stem, md in SPECS.items():
    ex = root / md.replace(".md", ".examples.yaml")
    spec_text = (root / md).read_text(encoding="utf8")
    seen = set()
    covered = set()
    for sc in yaml.safe_load(ex.read_text(encoding="utf8")):
        total += 1
        sid = sc["id"]
        if sid in seen:
            fail(sid, "повтор id сценария")
        seen.add(sid)
        for rule in re.split(r"\s*,\s*", sc["rule"]):
            if f"**{rule}.**" not in spec_text:
                fail(sid, f"правило {rule} не найдено в {md}")
        covered |= set(re.findall(r"[A-Z]{2}-R\d+[a-z]?", sc["rule"]))
        for st in sc.get("steps", []):
            check_step(sid, st)
    sec = spec_text[spec_text.index("## 4. Таблицы примеров"):spec_text.index("## 5.")]
    table_rules = set(re.findall(r"[A-Z]{2}-R\d+[a-z]?", "\n".join(l.split("|")[1] for l in sec.splitlines() if l.startswith("|") and not l.startswith("|--") and not l.startswith("| Правило"))))
    for r in sorted(table_rules - covered):
        fail(stem, f"правило {r} есть в таблице спецификации, но не покрыто сценарием")
for e in errors:
    print("FAIL", e)
print(f"{'FAILED' if errors else 'OK'}: {len(errors)} ошибок, сценариев {total}")
sys.exit(1 if errors else 0)
