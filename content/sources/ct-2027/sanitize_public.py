"""Удаляет из снимка ct-2027 пути student/* и формулировки про одного ученика."""

from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent

# Целые формы «наш … ученик», без частичного совпадения по корню «ученик».
_OUR_PUPIL_PHRASE = re.compile(
    r"\bнаш(?:его|ему|ем|им|ей|ую|яя|и|их|ими)?\s+ученик(?:а|у|е|ом|и|ов|ам|ах|ами)?\b",
    re.IGNORECASE,
)

REPLACEMENTS: tuple[tuple[str, str], ...] = (
    (r"\(`student/mistakes\.md`[^)]*\)", "(`журнал ошибок`)"),
    (r"`student/mistakes\.md`", "`журнал ошибок`"),
    (r"`student/progress\.md`", "`журнал прогресса`"),
    (r"student/mistakes\.md", "журнал ошибок"),
    (r"student/progress\.md", "журнал прогресса"),
    (r" со слов репетитора", ""),
    (r"надёжнее для этого ученика", "надёжнее при слабой базе"),
    (r"для нашего ученика", "на базовом уровне"),
    (r"для этого ученика", "на базовом уровне"),
    (r"этот ученик ([а-яё]+) тогда не сдаёт", r"в этот день \1 не сдаём"),
    (r"нашему ученику не нужно", "в базовую программу не входит"),
    (r"нашему ученику", "в программе курса"),
    (r"ученику хватит", "достаточно"),
    (r"а у этого ученика", "при слабой базе"),
    (r"это пишет репетитор", "это фиксируется"),
    (r"пишет репетитор", "фиксируется"),
    (r"у репетитора заполнена", "в журнале заполнена"),
    (r"главный сигнал для репетитора", "главный сигнал в журнале"),
    (r"ЦТ:\s*30 по математике,\s*15 по физике", "слабая исходная база"),
    (r"Ученик очень слабый \([^)]+\)", "Базовый уровень подготовки"),
    (r"журнал ошибок \(`журнал ошибок`\)", "журнал ошибок"),
    (
        r"у ученика не закреплена",
        "ещё не закреплена в базовой программе",
    ),
    (r"времени у ученика нет", "на это в программе нет времени"),
)

FORBIDDEN = (
    re.compile(r"student/"),
    re.compile(r"ЦТ:\s*\d+\s*по\s*математике"),
    re.compile(r"^## Заметки для генерации материалов", re.MULTILINE),
    re.compile(r"у ученика не закреплена"),
    re.compile(r"времени у ученика нет"),
    re.compile(r"этот ученик"),
    re.compile(r"нашему ученику"),
    re.compile(_OUR_PUPIL_PHRASE.pattern),
    re.compile(r"ученику хватит"),
)


def sanitize_text(text: str) -> str:
    for pattern, repl in REPLACEMENTS:
        text = re.sub(pattern, repl, text)
    text = _OUR_PUPIL_PHRASE.sub("на базовом уровне", text)
    return text


def main() -> None:
    changed = 0
    for path in sorted(ROOT.rglob("*.md")):
        original = path.read_text(encoding="utf-8")
        updated = sanitize_text(original)
        if updated != original:
            path.write_text(updated, encoding="utf-8")
            changed += 1
    problems: list[str] = []
    skip = {"README.md", "sanitize_public.py"}
    for path in sorted(ROOT.rglob("*.md")):
        if path.name in skip:
            continue
        text = path.read_text(encoding="utf-8")
        rel = path.relative_to(ROOT)
        for rule in FORBIDDEN:
            if rule.search(text):
                problems.append(f"{rel}: {rule.pattern}")
    if problems:
        raise SystemExit("остались запрещённые фрагменты:\n" + "\n".join(problems[:30]))
    print(f"sanitized files: {changed}")


if __name__ == "__main__":
    main()
