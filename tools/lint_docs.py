#!/usr/bin/env python3
"""Целостность ссылок документации: markdown-ссылки на файлы, номера решений Dnn, номера правил XX-Rnn.
Запуск: python3 tools/lint_docs.py   (только стандартная библиотека)"""
import pathlib, re, sys

root = pathlib.Path(__file__).resolve().parents[1]
docs = root / "docs"
SKIP = ("docs/spec/", "docs/archive/", "docs/handoff/entries/")  # записи журнала только добавляются
FUTURE = {"diagnostics.md", "optimizer.md", "knowledge-model.md", "agent.md", "eval.md"}  # модули выпусков 2–4, ещё не написаны
files = [p for p in docs.rglob("*.md") if not any(s in str(p.relative_to(root)) + "/" for s in SKIP)]
log = (docs / "decisions/log.md").read_text(encoding="utf8")
decisions = set(re.findall(r"^\| D(\d+) \|", log, re.M))
rule_defs = set()
for p in files:
    rule_defs |= set(re.findall(r"\*\*([A-Z]{2}-R\d+[a-z]?)\.\*\*", p.read_text(encoding="utf8")))
bad = 0
def fail(msg):
    global bad; bad += 1; print("FAIL", msg)
for p in files:
    rel = p.relative_to(root)
    t = p.read_text(encoding="utf8")
    if rel.as_posix() == "docs/decisions/log.md":
        continue
    for m in re.finditer(r"\]\(([^)#\s]+)(#[^)]*)?\)", t):
        href = m.group(1)
        if href.startswith(("http", "mailto:", "#")):
            continue
        if pathlib.PurePosixPath(href).name in FUTURE:
            continue
        if not (p.parent / href).resolve().exists():
            fail(f"{rel}: ссылка на несуществующий файл {href}")
    for d in set(re.findall(r"\bD(\d{1,3})\b", t)):
        if d not in decisions:
            fail(f"{rel}: решение D{d} нет в журнале")
    for r in set(re.findall(r"\b([A-Z]{2}-R\d+[a-z]?)\b", t)):
        if r not in rule_defs:
            fail(f"{rel}: правило {r} нигде не определено")
print(f"{'FAILED' if bad else 'OK'}: {bad} замечаний, файлов {len(files)}")
sys.exit(1 if bad else 0)
