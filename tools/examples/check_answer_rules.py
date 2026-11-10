#!/usr/bin/env python3
"""Эталонная (независимая от Go) реализация функции оценки catalog/answer-rules.md
и проверка docs/catalog/answer-rules.examples.yaml (D99, D133, AR-R15).

Запуск: python3 tools/examples/check_answer_rules.py   (нужен pyyaml)
"""

import pathlib
import re
import sys

import yaml

MAX_POINTS = {
    "ct.choice1": 1,
    "ct.choiceN": 1,
    "ct.match": 2,
    "ct.digits": 2,
    "ct.integer": 2,
}
MINUS = "−–‐"
LATIN_TO_CYR = {
    "A": "А",
    "B": "В",
    "C": "С",
    "E": "Е",
    "H": "Н",
    "K": "К",
    "M": "М",
    "O": "О",
    "P": "Р",
    "T": "Т",
    "X": "Х",
}


def strip_ws(s):
    return re.sub(r"\s+", "", s)


def normalize(fmt, raw, key):
    """Возвращает нормализованный ввод или None (malformed)."""
    if fmt in ("ct.choice1", "ct.choiceN"):
        if (
            not isinstance(raw, list)
            or not raw
            or any(not isinstance(x, int) or not 1 <= x <= 5 for x in raw)
        ):
            return None
        if fmt == "ct.choice1" and len(raw) != 1:
            return None
        return sorted(set(raw))
    s = strip_ws(raw)
    for ch in MINUS:
        s = s.replace(ch, "-")
    if fmt == "ct.integer":
        s = s.removeprefix("+")
        if not re.fullmatch(r"-?[0-9]+", s):
            return None
        return str(int(s))
    if fmt == "ct.digits":
        if not re.fullmatch(r"[0-9]+", s) or len(set(s)) != len(s):
            return None
        return "".join(sorted(s))
    if fmt == "ct.match":
        s = "".join(LATIN_TO_CYR.get(c, c) for c in s.upper())
        if not re.fullmatch(r"([А-ЯЁ][0-9])+", s):
            return None
        pairs = re.findall(r"([А-ЯЁ])([0-9])", s)
        letters = [p[0] for p in pairs]
        key_letters = sorted(re.findall(r"[А-ЯЁ]", key))
        if sorted(letters) != key_letters:
            return None
        return "".join(l + d for l, d in sorted(pairs))
    raise ValueError(fmt)


def score(fmt, key, raw, traps=None):
    norm = normalize(fmt, raw, key)
    if norm is None:
        return {"status": "malformed"}
    traps = traps or {}
    if fmt in ("ct.choice1", "ct.choiceN"):
        shown = ",".join(map(str, norm))
        full = norm == sorted(key)
        verdict = "correct" if full else "wrong"
    elif fmt == "ct.integer":
        shown = norm
        verdict = "correct" if int(norm) == int(key) else "wrong"
    elif fmt == "ct.digits":
        shown = norm
        diff = len(set(norm) ^ set(str(key)))
        verdict = "correct" if diff == 0 else "partial" if diff == 1 else "wrong"
    else:  # ct.match
        shown = norm
        got = dict(re.findall(r"([А-ЯЁ])([0-9])", norm))
        want = dict(re.findall(r"([А-ЯЁ])([0-9])", key))
        wrong = sum(got[k] != want[k] for k in want)
        verdict = "correct" if wrong == 0 else "partial" if wrong == 1 else "wrong"
    maxp = MAX_POINTS[fmt]
    points = maxp if verdict == "correct" else 1 if verdict == "partial" else 0
    res = {
        "status": "scored",
        "normalized": shown,
        "verdict": verdict,
        "points": points,
    }
    if verdict == "wrong" and shown in traps:
        res["trap_id"] = traps[shown]
    return res


def main():
    path = (
        pathlib.Path(__file__).resolve().parents[2]
        / "docs/catalog/answer-rules.examples.yaml"
    )
    failed = 0
    for c in yaml.safe_load(path.read_text(encoding="utf8")):
        got = score(c["format"], c["key"], c["input"], c.get("traps"))
        if got != c["expect"]:
            failed += 1
            print(
                f"FAIL {c['id']} ({c['rule']}): ожидалось {c['expect']}, получено {got}"
            )
    print(f"{'FAILED' if failed else 'OK'}: {failed} ошибок")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
