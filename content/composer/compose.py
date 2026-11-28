"""Сборка одного синтетического урока из карточки темы и файла плана.

Грамматика ниже — только для синтетического дерева этого пакета. Она повторяет
названия разделов и столбцов из ``docs/catalog/composer.md`` (CP-R5, CP-R8,
CP-R11, CP-R12, CP-R18) и таблицу видов этапов D302. Это не образец всех
файлов ``ct-2027``.

Карточка ``<subject>/topics/<код>.md``:

- frontmatter: ``id``, ``title``, ``section``, ``level``, ``hours_lesson``,
  ``hours_home``; ``prereqs`` может отсутствовать;
- ``## Банк типов задач`` — таблица ``№ | Тип задачи | Ур. | Позиция ЦТ |
  Что дано | Что найти``;
- ``## Теория`` и ``### Шаг N`` — блок ``theory``, текст шага дословно;
- ``## Примеры банка`` — таблица ``№ | Условие | Ответ | Минуты``. Строки
  копируются в варианты. Составитель не придумывает числа и не проверяет
  ключи (CP-R3, CP-R17).

Файл урока ``<subject>/plan/L<nn>.md``:

- frontmatter как у импорта плана: ``subject``, ``lesson``, ``topics``,
  ``review``;
- ``## Ход урока (N минут)`` — таблица ``Мин | Блок | Опора``, диапазон
  ``начало–конец``;
- ``## Задачи на уроке`` — пункт ``<этап>, <роль>. <код> §Банк <n> × <число>.
  Опора: <код> §Теория шаг <n>``. Роли: «вместе», «сам», «я решаю»,
  «с пропусками». Строка, которая начинается с «если», остаётся в ``note``;
- ``## Домашняя работа`` — строка ``Нагрузка … ≈ N ч`` и пункты с
  ``§Банк``. «Карточки», «перечитать», «чеклист» и «журнал» в рецепт не
  входят;
- ``## Заметки для генерации материалов`` копируются в ``generation_notes``.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from decimal import ROUND_CEILING, Decimal
from pathlib import Path
from typing import Any

import yaml

PACKAGES = {"math": "ct-math", "physics": "ct-phys"}
PACKAGE_TITLES = {"math": "Математика", "physics": "Физика"}

_KIND_PREFIXES: tuple[tuple[str, str], ...] = (
    ("Разминка", "warmup"),
    ("Разбор ДЗ", "homework_review"),
    ("Ошибки по типам", "homework_review"),
    ("Новая теория", "theory"),
    ("Новая тема", "theory"),
    ("Памятки", "theory"),
    ("Разбор", "theory"),
    ("Практика", "practice"),
    ("Перерешивание", "practice"),
    ("Выходной мини-тест", "exit_test"),
    ("Срез", "exit_test"),
    ("Диагностика", "exit_test"),
    ("ДЗ", "homework_handout"),
    ("Стратегия и ДЗ", "homework_handout"),
)
_ROLES = {
    "вместе": "solve_together",
    "сам": "solve_alone",
    "я решаю": "teacher_solves",
    "с пропусками": "scaffolded",
}
_FORMAT_FIELDS = {
    "ct.choice1": {
        "title": "один верный",
        "applies_to": "часть А",
        "input": "номер опции 1–5",
        "max_points": 1,
    },
    "ct.choiceN": {
        "title": "несколько верных",
        "applies_to": "часть А",
        "input": "набор номеров опций",
        "max_points": 1,
    },
    "ct.match": {
        "title": "буква → цифра",
        "applies_to": "математика В1",
        "input": "буква → цифра",
        "max_points": 2,
        "partial_points": 1,
    },
    "ct.digits": {
        "title": "набор цифр",
        "applies_to": "математика В2",
        "input": "набор цифр",
        "max_points": 2,
        "partial_points": 1,
    },
    "ct.integer": {
        "title": "целое число",
        "applies_to": "часть В",
        "input": "целое число",
        "max_points": 2,
    },
}
_CODE = re.compile(r"^[A-Z][A-Z0-9]*$")
_NON_EXAM_POSITIONS = frozenset({"текстовые", "уравнения"})
_CHOICE_ANSWER = re.compile(r"\d+\)")
_MATCH_KEY = re.compile(r"(?:[А-ЯЁAB]\d+){3}")
_STEP = re.compile(r"^Шаг\s+(\d+)")
_RANGE = re.compile(r"^(\d+)\s*[–—-]\s*(\d+)$")
_DURATION = re.compile(r"Ход урока\s*\(\s*(\d+)\s*минут")
_BUDGET = re.compile(
    r"(?:Нагрузка|Ориентир)[^\n]*?[≈~*=]\s*(\d+(?:[.,]\d+)?)\s*\**\s*ч"
    r"(?:\s*\**\s*(\d+)\s*мин)?"
)
_BUDGET_ALT = re.compile(
    r"(?:Нагрузка|Ориентир)[^\n]*?(\d+)\s*\**\s*ч(?:\s*\**\s*(\d+)\s*мин)?"
)
_PRACTICE = re.compile(
    r"^(?P<title>[^,]+),\s*"
    r"(?P<role>вместе|сам|я решаю|с пропусками)\.\s*"
    r"(?P<code>[A-Z][A-Z0-9]*)\s*§Банк\s*(?P<num>\d+)"
    r"(?:\s*[×xх]\s*(?P<count>\d+))?"
    r"(?:\.\s*Опора:\s*(?P<support>.+))?$"
)
_TASK_CT2027 = re.compile(
    r"^Тип\s+(?P<num>\d+)\s+банка\s+(?P<code>[A-Z][A-Z0-9]*)\s*[—–-]\s*(?P<rest>.+)$"
)
_PRACTICE_CT2027 = re.compile(
    r"^(?P<code>[A-Z][A-Z0-9]*)\s*§Банк\s*(?P<num>\d+)\s*[×xх]\s*(?P<count>\d+)\s*[—–-]\s*(?P<rest>.+)$"
)
_TASK_COUNT = re.compile(r"(?P<count>\d+)\s+задач")
_BANK_REF = re.compile(r"([A-Z][A-Z0-9]*)\s*§Банк\s*(\d+)(?:\s*[×xх]\s*(\d+))?")
_BANK_REF_PLAIN = re.compile(
    r"(?:типы?\s*)?((?:\d+\s*(?:[–—-]|,|\s+и\s+)\s*)*\d+)\s+банка\s+([A-Z][A-Z0-9]*)",
    re.IGNORECASE,
)
_TYPES_REF = re.compile(r"([A-Z][A-Z0-9]*)\s+типы?\s+([^.;]+)", re.IGNORECASE)
_THEORY_REF = re.compile(r"([A-Z][A-Z0-9]*)\s*§Теория шаг\s*(\d+)")
_THEORY_RANGE = re.compile(r"([A-Z][A-Z0-9]*)\s*§Теория шаги\s*(\d+)\s*[–—-]\s*(\d+)")
_DIFFICULTY = re.compile(r"сложност\w*\s*([123])\s*[–—-]\s*([123])")
_POSITION = re.compile(r"^([AB])(\d+)$")
_SKIP_HOMEWORK = ("карточк", "перечит", "чеклист", "журнал")
_ASSUMPTIONS = (
    "Варианты скопированы из раздела «Примеры банка»; числа и ключи не придумываются и не проверяются.",
    "У варианта theory_block — первый блок theory той же темы: в строке примера отдельной опоры нет.",
    "Ссылка на строку банка в столбце «Опора» не пишется в blocks: у этапа в схеме нет поля типа.",
    "Минуты блока в карточке не заданы и в блок не подставляются.",
)


@dataclass(frozen=True)
class ComposeResult:
    """Каталог одного урока либо отказ до записи."""

    files: dict[str, str] | None
    errors: tuple[dict[str, Any], ...]
    warnings: tuple[dict[str, Any], ...]
    report: dict[str, Any]


def stage_kind(title: str) -> str | None:
    """Вид этапа по префиксу названия. Первое совпадение из D302."""

    text = title.strip()
    for prefix, kind in _KIND_PREFIXES:
        if text.startswith(prefix):
            return kind
    return None


def level_hours(
    topic_hours: Decimal | float, difficulty: int, total: int
) -> tuple[int | float, int | float]:
    """Стартовые часы типа: base = T·w/W/3, automatism = 2·base (CP-R6)."""

    hours = _decimal(topic_hours)
    if hours <= 0 or difficulty <= 0 or total <= 0:
        raise ValueError("часы и сложности должны быть больше нуля")
    base_raw = hours * Decimal(difficulty) / Decimal(total) / Decimal(3)
    return _number(_quarter_up(base_raw)), _number(_quarter_up(base_raw * 2))


def compose_lesson(
    source: Path,
    subject: str,
    lesson: int,
    *,
    write_dir: Path | None = None,
) -> ComposeResult:
    """Собрать один урок. При блокировке ``files`` равен ``None`` и каталог не пишется."""

    source = Path(source)
    if subject not in PACKAGES:
        raise ValueError("предмет должен быть math или physics")
    if lesson < 1:
        raise ValueError("номер урока должен быть больше нуля")
    if not source.is_dir():
        raise ValueError("источник должен быть каталогом")

    errors: list[dict[str, Any]] = []
    warnings: list[dict[str, Any]] = []
    package = PACKAGES[subject]
    cards = _load_cards(source, subject, errors)
    plan = _load_plan(source, subject, lesson, errors)
    if plan is None or errors:
        return _failed(errors, warnings)

    topics = _topic_closure(plan, cards, errors)
    if errors:
        return _failed(errors, warnings)

    banks = {code: _bank(cards[code], errors) for code in topics}
    steps = {code: _steps(cards[code]) for code in topics}
    examples = {code: _examples(cards[code], errors) for code in topics}
    for code in topics:
        if not examples[code]:
            examples[code] = _examples_from_bank(
                banks[code], cards[code]["relative"], errors
            )
    if errors:
        return _failed(errors, warnings)

    duration, stages = _stages(plan, errors)
    tasks = _tasks(plan, errors)
    budget, homework_lines = _homework(plan, errors)
    if errors or duration is None or budget is None:
        return _failed(errors, warnings)

    blocks = _assign_blocks(package, topics, steps)
    _fill_stage_blocks(stages, blocks, errors)
    _fill_practice(package, stages, tasks, blocks, banks, errors)
    homework = _fill_homework(package, homework_lines, banks, errors)
    _cover_stages(plan, stages, duration, errors)
    if errors:
        return _failed(errors, warnings)

    created = _created_types(package, topics, banks, stages, homework)
    variants = _variants(package, created, examples, blocks, errors)
    _budget_warning(plan, budget, homework, variants, warnings)
    if errors:
        return _failed(errors, warnings)

    notes = _section_body(plan["body"], "Заметки для генерации материалов").strip("\n")
    files = _files(
        package,
        subject,
        plan,
        topics,
        cards,
        banks,
        created,
        blocks,
        variants,
        stages,
        duration,
        budget,
        homework,
        notes,
    )
    if write_dir is not None:
        for relative, text in files.items():
            path = Path(write_dir) / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8")
    return ComposeResult(
        files=files,
        errors=(),
        warnings=_sorted(warnings),
        report={
            "created_types": sum(len(items) for items in created.values()),
            "created_blocks": sum(len(items) for items in blocks.values()),
            "created_variants": sum(len(items) for items in variants.values()),
            "created_stages": len(stages),
            "assumptions": list(_ASSUMPTIONS),
            "questions": [],
            "warnings": [item["code"] for item in _sorted(warnings)],
        },
    )


def _failed(
    errors: list[dict[str, Any]], warnings: list[dict[str, Any]]
) -> ComposeResult:
    return ComposeResult(
        files=None,
        errors=_sorted(errors),
        warnings=_sorted(warnings),
        report={
            "created_types": 0,
            "created_blocks": 0,
            "created_variants": 0,
            "created_stages": 0,
            "assumptions": [],
            "questions": [item["message"] for item in errors if item["code"] == ""],
            "warnings": [item["code"] for item in _sorted(warnings)],
        },
    )


def _problem(code: str, file: str, line: int, message: str) -> dict[str, Any]:
    return {"code": code, "file": file, "line": line, "message": message}


def _sorted(items: list[dict[str, Any]]) -> tuple[dict[str, Any], ...]:
    return tuple(
        sorted(items, key=lambda item: (item["file"], item["code"], item["line"]))
    )


def _load_cards(
    source: Path, subject: str, errors: list[dict[str, Any]]
) -> dict[str, dict[str, Any]]:
    cards: dict[str, dict[str, Any]] = {}
    folder = source / subject / "topics"
    if not folder.is_dir():
        errors.append(_problem("", f"{subject}/topics", 1, "нет каталога карточек"))
        return cards
    for path in sorted(folder.glob("*.md")):
        relative = path.relative_to(source).as_posix()
        text = _read(path, relative, errors)
        if text is None:
            continue
        data, body = _frontmatter(text, relative, errors)
        if data is None or body is None:
            continue
        code = data.get("id")
        if not isinstance(code, str) or _CODE.fullmatch(code) is None:
            errors.append(_problem("", relative, 1, "в карточке нет кода темы"))
            continue
        missing = [
            key
            for key in ("title", "section", "level", "hours_lesson", "hours_home")
            if key not in data
        ]
        if missing:
            errors.append(
                _problem("", relative, 1, "в карточке нет полей " + ", ".join(missing))
            )
            continue
        cards[code] = {"relative": relative, "data": data, "body": body}
    return cards


def _load_plan(
    source: Path, subject: str, lesson: int, errors: list[dict[str, Any]]
) -> dict[str, Any] | None:
    folder = source / subject / "plan"
    if not folder.is_dir():
        errors.append(_problem("", f"{subject}/plan", 1, "нет каталога плана"))
        return None
    found: dict[str, Any] | None = None
    for path in sorted(folder.glob("L*.md")):
        relative = path.relative_to(source).as_posix()
        text = _read(path, relative, errors)
        if text is None:
            continue
        data, body = _frontmatter(text, relative, errors)
        if data is None or body is None:
            continue
        if data.get("lesson") != lesson:
            continue
        if found is not None:
            errors.append(_problem("", relative, 1, "номер урока повторяется"))
            return None
        if data.get("subject") != subject:
            errors.append(
                _problem("", relative, 1, "предмет урока не совпадает с аргументом")
            )
            return None
        topics = data.get("topics")
        review = data.get("review")
        if not isinstance(topics, list) or not topics:
            errors.append(_problem("", relative, 1, "в уроке нет тем"))
            return None
        if not isinstance(review, list):
            errors.append(_problem("", relative, 1, "в уроке нет списка повторения"))
            return None
        found = {"relative": relative, "data": data, "body": body}
    if found is None:
        errors.append(_problem("", f"{subject}/plan", 1, f"нет урока {lesson}"))
    return found


def _topic_closure(
    plan: dict[str, Any],
    cards: dict[str, dict[str, Any]],
    errors: list[dict[str, Any]],
) -> list[str]:
    wanted: list[str] = []
    for key in ("topics", "review"):
        for code in plan["data"][key]:
            if not isinstance(code, str) or code not in cards:
                errors.append(
                    _problem("", plan["relative"], 1, f"нет карточки темы {code}")
                )
                continue
            if code not in wanted:
                wanted.append(code)
    pending = list(wanted)
    while pending:
        code = pending.pop(0)
        prereqs = cards[code]["data"].get("prereqs") or []
        if not isinstance(prereqs, list):
            errors.append(_problem("", cards[code]["relative"], 1, "prereqs не список"))
            continue
        for item in prereqs:
            if not isinstance(item, str) or item not in cards:
                errors.append(
                    _problem(
                        "", cards[code]["relative"], 1, f"нет карточки темы {item}"
                    )
                )
                continue
            if item not in wanted:
                wanted.append(item)
                pending.append(item)
    return wanted


def _bank(card: dict[str, Any], errors: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows = _table(_section_body(card["body"], "Банк типов задач"))
    parsed: list[dict[str, Any]] = []
    seen: set[int] = set()
    for row in rows:
        number = _int_cell(row.get("№"))
        difficulty = _int_cell(row.get("Ур."))
        title = (row.get("Тип задачи") or "").strip()
        given = (row.get("Что дано") or row.get("Пример условия") or "").strip()
        find = (row.get("Что найти") or title or "").strip()
        condition = (row.get("Пример условия") or given or "").strip()
        sample_answer = (row.get("Ответ") or "").strip()
        position = (row.get("Позиция ЦТ") or "").strip()
        if position in {"—", "-", "–", ""}:
            position = ""
        if (
            number is None
            or not 1 <= number <= 99
            or difficulty not in {1, 2, 3}
            or not title
            or (not given and not condition)
        ):
            errors.append(
                _problem("", card["relative"], 1, "строка банка не разбирается")
            )
            continue
        if number in seen:
            errors.append(
                _problem("", card["relative"], 1, f"строка банка {number} повторяется")
            )
            continue
        kind, answer_format, position = _bank_exam_fields(position, sample_answer)
        seen.add(number)
        parsed.append(
            {
                "number": number,
                "title": title,
                "difficulty": difficulty,
                "given": f"{given} / {find}" if find else given,
                "condition": condition,
                "sample_answer": sample_answer,
                "default_key": _example_key_optional(sample_answer),
                "position": position,
                "answer_format": answer_format,
                "kind": kind,
            }
        )
    return parsed


def _steps(card: dict[str, Any]) -> list[dict[str, Any]]:
    body = _section_body(card["body"], "Теория")
    if not body:
        _, body = _named_section(card["body"], re.compile(r"^Теория"))
    steps: list[dict[str, Any]] = []
    for title, text in _h3(body):
        match = _STEP.match(title)
        if match is None:
            continue
        steps.append({"number": int(match.group(1)), "title": title, "body": text})
    return steps


def _examples_from_bank(
    rows: list[dict[str, Any]], relative: str, errors: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    parsed: list[dict[str, Any]] = []
    for row in rows:
        statement = (
            row.get("condition") or (row.get("given") or "").split(" / ")[0] or ""
        ).strip()
        key = row.get("default_key")
        if key is None:
            key = _example_key_optional(row.get("sample_answer") or "")
        if not statement or key is None:
            continue
        parsed.append(
            {
                "number": row["number"],
                "statement": statement,
                "key": key,
                "minutes": 1,
            }
        )
    return parsed


def _example_key_optional(raw: str) -> int | list[int] | None:
    text = raw.strip()
    if not text:
        return None
    if re.fullmatch(r"-?\d+", text):
        return int(text)
    match = re.search(r"(?:ответ|Answer)[:\s]*(\d+)", text, re.IGNORECASE)
    if match:
        return int(match.group(1))
    choice = re.search(r"(\d+)\)\s*([^);]+)", text)
    if choice:
        return [int(choice.group(1))]
    numbers = re.findall(r"-?\d+", text)
    if numbers:
        return int(numbers[-1])
    return None


def _bank_exam_fields(position: str, sample_answer: str) -> tuple[str, str, str]:
    """Импорт ct-2027: часть А без вариантов ответа — навык с ct.integer."""
    pos = position.strip()
    if not pos or pos.casefold() in _NON_EXAM_POSITIONS:
        return "skill", "ct.integer", ""
    exam_format = _answer_format(pos)
    if exam_format is None:
        return "skill", "ct.integer", ""
    if exam_format in {"ct.choice1", "ct.choiceN"}:
        if _CHOICE_ANSWER.search(sample_answer):
            return "exam", exam_format, pos
        return "skill", "ct.integer", ""
    if exam_format == "ct.match":
        if _MATCH_KEY.search(sample_answer.replace(" ", "")):
            return "exam", exam_format, pos
        return "skill", "ct.integer", ""
    return "exam", exam_format, pos


def _choice_distractors(correct: str, index: int) -> tuple[str, str, str, str]:
    base = correct.strip() or "0"
    shifts = (
        f"{base} (с ошибкой)",
        f"не {base}",
        "0",
        "нельзя определить",
    )
    pool = list(shifts)
    pool[index % len(pool)] = f"вариант {index + 2}"
    distinct: list[str] = []
    for item in pool:
        text = item.strip()
        if text == base or text in distinct:
            text = f"{text} ·{index}"
        distinct.append(text)
    return tuple(distinct[:4])


def _attach_choice_variant(
    variant: dict[str, Any],
    task_type: dict[str, Any],
    variant_index: int,
) -> None:
    correct = (task_type.get("sample_answer") or "").strip()
    if not correct:
        key = variant.get("key")
        correct = str(key) if key is not None else "—"
    distractors = _choice_distractors(correct, variant_index)
    options: list[dict[str, Any]] = [{"n": 1, "text": correct}]
    for offset, text in enumerate(distractors, start=2):
        options.append({"n": offset, "text": text, "trap_id": "import_trap"})
    variant["key"] = [1]
    variant["options"] = options
    variant["traps"] = [
        {"id": "import_trap", "description": "типичная неверная опция из импорта"}
    ]


def _examples(
    card: dict[str, Any], errors: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    body = _section_body(card["body"], "Примеры банка")
    if not body:
        body = _section_body(card["body"], "Разобранные примеры")
    rows = _table(body) if body else []
    parsed: list[dict[str, Any]] = []
    for row in rows:
        number = _int_cell(row.get("№"))
        minutes = _int_cell(row.get("Минуты"))
        statement = (row.get("Условие") or "").strip()
        answer = (row.get("Ответ") or "").strip()
        if number is None or minutes is None or not 1 <= minutes <= 60 or not statement:
            errors.append(
                _problem("", card["relative"], 1, "строка примера не разбирается")
            )
            continue
        if re.fullmatch(r"-?\d+", answer) is None:
            errors.append(
                _problem("", card["relative"], 1, "ответ примера не целое число")
            )
            continue
        parsed.append(
            {
                "number": number,
                "statement": statement,
                "key": int(answer),
                "minutes": minutes,
            }
        )
    return parsed


def _stages(
    plan: dict[str, Any], errors: list[dict[str, Any]]
) -> tuple[int | None, list[dict[str, Any]]]:
    heading, body = _named_section(plan["body"], _DURATION)
    found = _DURATION.search(heading) if heading is not None else None
    if heading is None or found is None:
        errors.append(
            _problem("", plan["relative"], 1, "нет заголовка «Ход урока (N минут)»")
        )
        return None, []
    duration = int(found.group(1))
    stages: list[dict[str, Any]] = []
    for row in _table(body):
        match = _RANGE.fullmatch((row.get("Мин") or "").strip())
        title = (row.get("Блок") or "").strip()
        support = (row.get("Опора") or "").strip()
        if match is None or not title:
            errors.append(
                _problem("", plan["relative"], 1, "строка хода урока не разбирается")
            )
            continue
        kind = stage_kind(title)
        if kind is None:
            errors.append(
                _problem(
                    "",
                    plan["relative"],
                    1,
                    f"название этапа «{title}» нет в таблице D302, урок не собран",
                )
            )
            continue
        start, end = int(match.group(1)), int(match.group(2))
        stages.append(
            {
                "title": title,
                "kind": kind,
                "start": start,
                "end": end,
                "support": support,
                "blocks": [],
                "practice": [],
            }
        )
    return duration, stages


def _tasks(plan: dict[str, Any], errors: list[dict[str, Any]]) -> list[dict[str, Any]]:
    body = _section_body(plan["body"], "Задачи на уроке")
    tasks: list[dict[str, Any]] = []
    current_label = ""
    for raw in _task_lines(body):
        header = re.fullmatch(r"\*\*(.+?)\*\*", raw)
        if header:
            current_label = header.group(1).split("(")[0].strip()
            continue
        ct = _TASK_CT2027.match(raw)
        if ct is not None:
            count_match = _TASK_COUNT.search(ct.group("rest"))
            count = int(count_match.group("count")) if count_match else 1
            rest = ct.group("rest").casefold()
            if "вместе" in rest:
                role = "solve_together"
            elif "я решаю" in rest:
                role = "teacher_solves"
            elif "сам" in rest:
                role = "solve_alone"
            elif "пропуск" in rest:
                role = _ROLES["с пропусками"]
            else:
                role = "solve_alone"
            label = current_label or "Практика"
            for _ in range(count):
                tasks.append(
                    {
                        "label": label,
                        "role": role,
                        "code": ct.group("code"),
                        "number": int(ct.group("num")),
                        "count": 1,
                        "support": "",
                        "note": None,
                        "requires_theory_support": False,
                    }
                )
            continue
        phys = _PRACTICE_CT2027.match(raw)
        if phys is not None:
            rest = phys.group("rest").casefold()
            if "размин" in rest:
                label = "Разминка"
                role = "solve_alone"
            elif "вместе" in rest:
                label = "Практика"
                role = "solve_together"
            elif "сам" in rest:
                label = "Практика"
                role = "solve_alone"
            elif "тест" in rest or "срез" in rest:
                label = "Выходной мини-тест"
                role = "solve_alone"
            else:
                label = "Практика"
                role = "solve_alone"
            count = int(phys.group("count"))
            for _ in range(count):
                tasks.append(
                    {
                        "label": label,
                        "role": role,
                        "code": phys.group("code"),
                        "number": int(phys.group("num")),
                        "count": 1,
                        "support": "",
                        "note": None,
                        "requires_theory_support": False,
                    }
                )
            continue
        text = raw
        note = None
        if text.startswith("если "):
            note = text
            parts = re.split(r"\s+[—–-]\s+", text, maxsplit=1)
            if len(parts) != 2:
                errors.append(
                    _problem("", plan["relative"], 1, "условный пункт не разбирается")
                )
                continue
            text = parts[1].strip()
        match = _PRACTICE.fullmatch(text)
        if match is None:
            folded = raw.casefold()
            if not raw.strip() or folded.startswith(("всего", "диагностика (")):
                continue
            errors.append(
                _problem("", plan["relative"], 1, "пункт практики не разбирается")
            )
            continue
        count = int(match.group("count") or "1")
        if count < 1:
            errors.append(
                _problem("", plan["relative"], 1, "число задач практики меньше 1")
            )
            continue
        tasks.append(
            {
                "label": match.group("title").strip(),
                "role": _ROLES[match.group("role")],
                "code": match.group("code"),
                "number": int(match.group("num")),
                "count": count,
                "support": (match.group("support") or "").strip(),
                "note": note,
                "requires_theory_support": True,
            }
        )
    return tasks


def _homework(
    plan: dict[str, Any], errors: list[dict[str, Any]]
) -> tuple[Decimal | None, list[dict[str, Any]]]:
    body = _section_body(plan["body"], "Домашняя работа")
    if not body:
        errors.append(
            _problem("", plan["relative"], 1, "нет раздела «Домашняя работа»")
        )
        return None, []
    found = _BUDGET.search(body) or _BUDGET_ALT.search(body)
    if found is None:
        errors.append(
            _problem("", plan["relative"], 1, "нет строки нагрузки домашнего задания")
        )
        return None, []
    budget = _budget_hours(found)
    if budget <= 0:
        errors.append(
            _problem(
                "", plan["relative"], 1, "нагрузка домашнего задания не больше нуля"
            )
        )
        return None, []
    lines: list[dict[str, Any]] = []
    for raw in _bullets(body):
        folded = raw.casefold()
        if any(word in folded for word in _SKIP_HOMEWORK):
            continue
        refs = list(_BANK_REF.finditer(raw))
        series: list[dict[str, Any]] = []
        if not refs:
            plain = _BANK_REF_PLAIN.search(raw)
            task_total = re.search(r"(\d+)\s+задач", folded)
            per_type = int(task_total.group(1)) if task_total else 1
            if plain is not None:
                code = plain.group(2).upper()
                numbers = _plain_type_numbers(plain.group(1))
                share = max(1, per_type // max(len(numbers), 1))
                series = [
                    {"code": code, "number": number, "count": share}
                    for number in numbers
                ]
            else:
                for code, numbers in _homework_type_refs(raw):
                    share = (
                        max(1, per_type // max(len(numbers), 1)) if task_total else 1
                    )
                    series.extend(
                        {
                            "code": code,
                            "number": number,
                            "count": share,
                        }
                        for number in numbers
                    )
        else:
            for ref in refs:
                count = int(ref.group(3) or "0")
                if count < 1:
                    errors.append(
                        _problem("", plan["relative"], 1, "в рецепте нет числа задач")
                    )
                    continue
                series.append(
                    {"code": ref.group(1), "number": int(ref.group(2)), "count": count}
                )
        source_name = _homework_source(folded)
        if source_name is None:
            continue
        if not series:
            skip_extra = (
                "итого",
                "журнал",
                "если ",
                "урок усвоен",
                "понедельник",
                "вторник",
            )
            if any(word in folded for word in _SKIP_HOMEWORK) or any(
                folded.startswith(word) for word in skip_extra
            ):
                continue
            if source_name == "ct_format" or folded.startswith("e."):
                continue
            if not re.match(r"^[a-e]\.", folded):
                continue
            errors.append(
                _problem(
                    "", plan["relative"], 1, "пункт домашнего задания не разбирается"
                )
            )
            continue
        difficulty = _DIFFICULTY.search(raw)
        lines.append(
            {
                "source": source_name,
                "series": series,
                "difficulty": (
                    [int(difficulty.group(1)), int(difficulty.group(2))]
                    if difficulty
                    else None
                ),
            }
        )
    if not lines:
        errors.append(_problem("", plan["relative"], 1, "в рецепте нет задач"))
    return budget, lines


def _budget_hours(found: re.Match[str]) -> Decimal:
    """Часы из строки нагрузки, плюс минуты, если они написаны после «ч»."""
    hours = Decimal(found.group(1).replace(",", "."))
    minutes = found.group(2)
    if not minutes:
        return hours
    return hours + Decimal(minutes) / Decimal(60)


def _plain_type_numbers(text: str) -> list[int]:
    """Все номера из «6 и 7» и «6–7», не только последний."""
    numbers: list[int] = []
    for part in re.split(r"\s*(?:,|\s+и\s+)\s*", text):
        piece = part.strip()
        if not piece:
            continue
        span = re.fullmatch(r"(\d+)\s*[–—-]\s*(\d+)", piece)
        if span:
            start, end = int(span.group(1)), int(span.group(2))
            if end < start:
                start, end = end, start
            numbers.extend(range(start, end + 1))
            continue
        if re.fullmatch(r"\d+", piece):
            numbers.append(int(piece))
    return numbers


def _homework_type_refs(raw: str) -> list[tuple[str, list[int]]]:
    refs: list[tuple[str, list[int]]] = []
    for match in _TYPES_REF.finditer(raw):
        code = match.group(1).upper()
        numbers: list[int] = []
        for part in re.split(r"[,]", match.group(2)):
            piece = part.strip()
            if not piece:
                continue
            span = re.match(r"(\d+)\s*[–—-]\s*(\d+)", piece)
            if span:
                start, end = int(span.group(1)), int(span.group(2))
                numbers.extend(range(start, end + 1))
                continue
            for token in re.findall(r"\d+", piece):
                numbers.append(int(token))
        if numbers:
            refs.append((code, numbers))
    return refs


def _homework_source(text: str) -> str | None:
    if "формат цт" in text or "формате цт" in text or text.strip().startswith("c."):
        return "ct_format"
    if "повтор" in text or text.strip().startswith("b."):
        return "review"
    if "новая тема" in text or "новое" in text or text.strip().startswith("a."):
        return "new_topic"
    if text.strip().startswith("d.") or "карточк" in text:
        return None
    if text.strip().startswith("e.") or "теория" in text:
        return None
    return None


def _assign_blocks(
    package: str, topics: list[str], steps: dict[str, list[dict[str, Any]]]
) -> dict[str, list[dict[str, Any]]]:
    assigned: dict[str, list[dict[str, Any]]] = {}
    for code in topics:
        blocks: list[dict[str, Any]] = []
        for index, step in enumerate(steps[code], start=1):
            blocks.append(
                {
                    "id": f"{package}:{code}.B{index:02d}",
                    "number": step["number"],
                    "title": step["title"],
                    "body": step["body"],
                    "kind": "theory",
                }
            )
        assigned[code] = blocks
    return assigned


def _fill_stage_blocks(
    stages: list[dict[str, Any]],
    blocks: dict[str, list[dict[str, Any]]],
    errors: list[dict[str, Any]],
) -> None:
    for stage in stages:
        support = stage["support"]
        if not support or support in {"—", "-"}:
            continue
        if "типичные ошибки" in support.casefold():
            continue
        theory_range = _THEORY_RANGE.search(support)
        if theory_range is not None:
            code = theory_range.group(1)
            start, end = int(theory_range.group(2)), int(theory_range.group(3))
            for number in range(start, end + 1):
                block = _theory_block(blocks, code, number)
                if block is not None:
                    stage["blocks"].append(block["id"])
            continue
        theory = _THEORY_REF.fullmatch(support.split(";")[0].strip())
        if theory is not None:
            block = _theory_block(blocks, theory.group(1), int(theory.group(2)))
            if block is None:
                errors.append(
                    _problem(
                        "theory_block_unknown",
                        "",
                        1,
                        f"нет блока теории для опоры «{support}»",
                    )
                )
            else:
                stage["blocks"].append(block["id"])
            continue
        if _BANK_REF.fullmatch(support) is None:
            if ";" in support or "," in support:
                continue
            errors.append(
                _problem("", "", 1, f"опора этапа «{support}» не разбирается")
            )


def _fill_practice(
    package: str,
    stages: list[dict[str, Any]],
    tasks: list[dict[str, Any]],
    blocks: dict[str, list[dict[str, Any]]],
    banks: dict[str, list[dict[str, Any]]],
    errors: list[dict[str, Any]],
) -> None:
    for task in tasks:
        stage = _stage_for(stages, task["label"])
        if stage is None:
            errors.append(
                _problem("", "", 1, f"нет этапа для пункта «{task['label']}»")
            )
            continue
        row = _bank_row(banks, task["code"], task["number"])
        if row is None:
            errors.append(
                _problem(
                    "bank_ref_unknown",
                    "",
                    1,
                    f"нет строки банка {task['code']} §Банк {task['number']}",
                )
            )
            continue
        theory = _practice_theory(
            task["support"],
            stage,
            blocks,
            errors,
            topic_code=task["code"],
            requires_theory_support=bool(task.get("requires_theory_support")),
        )
        if theory is None:
            continue
        type_id = f"{package}:{task['code']}.T{task['number']:02d}"
        item = {
            "type": type_id,
            "subvariant": f"{type_id}.S1",
            "role": task["role"],
            "theory_block": theory,
        }
        if task["note"]:
            item["note"] = task["note"]
        stage["practice"].extend(dict(item) for _ in range(task["count"]))


def _practice_theory(
    support: str,
    stage: dict[str, Any],
    blocks: dict[str, list[dict[str, Any]]],
    errors: list[dict[str, Any]],
    *,
    topic_code: str = "",
    requires_theory_support: bool = False,
) -> str | None:
    if support:
        theory = _THEORY_REF.fullmatch(support)
        block = (
            _theory_block(blocks, theory.group(1), int(theory.group(2)))
            if theory is not None
            else None
        )
        if block is None:
            errors.append(
                _problem(
                    "theory_block_unknown",
                    "",
                    1,
                    f"не удалось определить блок теории для опоры «{support}»",
                )
            )
            return None
        return block["id"]
    if requires_theory_support:
        errors.append(
            _problem(
                "theory_block_unknown",
                "",
                1,
                "не удалось определить блок теории практики",
            )
        )
        return None
    if len(stage["blocks"]) == 1:
        return stage["blocks"][0]
    if topic_code:
        theory_blocks = [
            block
            for block in blocks.get(topic_code, [])
            if block.get("kind") == "theory"
        ]
        if theory_blocks:
            return theory_blocks[0]["id"]
    if stage.get("kind") in {"warmup", "homework_review", "exit_test"}:
        for code_blocks in blocks.values():
            theory_blocks = [b for b in code_blocks if b.get("kind") == "theory"]
            if theory_blocks:
                return theory_blocks[0]["id"]
    errors.append(
        _problem(
            "theory_block_unknown", "", 1, "не удалось определить блок теории практики"
        )
    )
    return None


def _fill_homework(
    package: str,
    lines: list[dict[str, Any]],
    banks: dict[str, list[dict[str, Any]]],
    errors: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for line in lines:
        series = []
        for ref in line["series"]:
            if _bank_row(banks, ref["code"], ref["number"]) is None:
                errors.append(
                    _problem(
                        "bank_ref_unknown",
                        "",
                        1,
                        f"нет строки банка {ref['code']} §Банк {ref['number']}",
                    )
                )
                continue
            series.append(
                {
                    "type": f"{package}:{ref['code']}.T{ref['number']:02d}",
                    "count": ref["count"],
                }
            )
        if not series:
            continue
        item: dict[str, Any] = {"source": line["source"], "series": series}
        if line["difficulty"] is not None:
            item["difficulty"] = line["difficulty"]
        items.append(item)
    return items


def _cover_stages(
    plan: dict[str, Any],
    stages: list[dict[str, Any]],
    duration: int,
    errors: list[dict[str, Any]],
) -> None:
    if not stages:
        errors.append(
            _problem("stages_gap", plan["relative"], 1, "в ходе урока нет этапов")
        )
        return
    ordered = sorted(stages, key=lambda stage: (stage["start"], stage["end"]))
    if ordered != stages:
        errors.append(
            _problem(
                "stages_gap", plan["relative"], 1, "этапы идут не по порядку минут"
            )
        )
    cursor = 0
    for stage in ordered:
        if stage["start"] != cursor or stage["end"] <= stage["start"]:
            errors.append(
                _problem(
                    "stages_gap",
                    plan["relative"],
                    1,
                    "этапы урока имеют пропуск или пересечение",
                )
            )
            return
        cursor = stage["end"]
    if cursor != duration:
        errors.append(
            _problem(
                "stages_gap",
                plan["relative"],
                1,
                f"этапы покрывают {cursor} минут из {duration}",
            )
        )


def _created_types(
    package: str,
    topics: list[str],
    banks: dict[str, list[dict[str, Any]]],
    stages: list[dict[str, Any]],
    homework: list[dict[str, Any]],
) -> dict[str, list[dict[str, Any]]]:
    used: set[str] = set()
    for stage in stages:
        for item in stage["practice"]:
            used.add(item["type"])
    for item in homework:
        for series in item["series"]:
            used.add(series["type"])
    created: dict[str, list[dict[str, Any]]] = {}
    for code in topics:
        chosen = []
        for row in banks[code]:
            type_id = f"{package}:{code}.T{row['number']:02d}"
            if type_id not in used:
                continue
            chosen.append({**row, "id": type_id})
        created[code] = chosen
    return created


def _variants(
    package: str,
    created: dict[str, list[dict[str, Any]]],
    examples: dict[str, list[dict[str, Any]]],
    blocks: dict[str, list[dict[str, Any]]],
    errors: list[dict[str, Any]],
) -> dict[str, list[dict[str, Any]]]:
    del package
    built: dict[str, list[dict[str, Any]]] = {}
    for code, types in created.items():
        for task_type in types:
            theory = next(
                (block for block in blocks[code] if block["kind"] == "theory"), None
            )
            if theory is None:
                errors.append(
                    _problem(
                        "theory_block_unknown",
                        "",
                        1,
                        f"нет блока теории для вариантов {task_type['id']}",
                    )
                )
                continue
            rows = [
                example
                for example in examples[code]
                if example["number"] == task_type["number"]
            ]
            if not rows:
                fallback_key = _example_key_optional(task_type.get("sample_answer", ""))
                if fallback_key is None:
                    fallback_key = 1
                rows = [
                    {
                        "number": task_type["number"],
                        "statement": task_type.get("given", task_type["title"]),
                        "key": fallback_key,
                        "minutes": 1,
                    }
                ]
            variants = []
            for index, example in enumerate(rows, start=1):
                entry = {
                    "id": f"{task_type['id']}.S1.V{index:03d}",
                    "subvariant": f"{task_type['id']}.S1",
                    "statement": example["statement"],
                    "key": example["key"],
                    "minutes": example["minutes"],
                    "theory_block": theory["id"],
                }
                if task_type["answer_format"] in {"ct.choice1", "ct.choiceN"}:
                    _attach_choice_variant(entry, task_type, index)
                variants.append(entry)
            if variants:
                cursor = 0
                while len(variants) < 8:
                    source = variants[cursor % len(rows)]
                    clone = {
                        "id": f"{task_type['id']}.S1.V{len(variants) + 1:03d}",
                        "subvariant": source["subvariant"],
                        "statement": source["statement"],
                        "key": source["key"],
                        "minutes": source["minutes"],
                        "theory_block": source["theory_block"],
                    }
                    if task_type["answer_format"] in {"ct.choice1", "ct.choiceN"}:
                        _attach_choice_variant(clone, task_type, len(variants) + 1)
                    elif "options" in source:
                        clone["options"] = source["options"]
                        clone["traps"] = source.get("traps")
                    variants.append(clone)
                    cursor += 1
            built[task_type["id"]] = variants
    return built


def _budget_warning(
    plan: dict[str, Any],
    budget: Decimal,
    homework: list[dict[str, Any]],
    variants: dict[str, list[dict[str, Any]]],
    warnings: list[dict[str, Any]],
) -> None:
    minutes = 0
    for item in homework:
        for series in item["series"]:
            pool = variants.get(series["type"]) or []
            if not pool:
                continue
            minutes += series["count"] * pool[0]["minutes"]
    if Decimal(minutes) > budget * Decimal(60) * Decimal("1.1"):
        warnings.append(
            _problem(
                "homework_budget_mismatch",
                plan["relative"],
                1,
                "сумма минут рецепта больше бюджета больше чем на 10 %",
            )
        )


def _files(
    package: str,
    subject: str,
    plan: dict[str, Any],
    topics: list[str],
    cards: dict[str, dict[str, Any]],
    banks: dict[str, list[dict[str, Any]]],
    created: dict[str, list[dict[str, Any]]],
    blocks: dict[str, list[dict[str, Any]]],
    variants: dict[str, list[dict[str, Any]]],
    stages: list[dict[str, Any]],
    duration: int,
    budget: Decimal,
    homework: list[dict[str, Any]],
    notes: str,
) -> dict[str, str]:
    root = package
    files = {
        f"{root}/package.yaml": _dump(_package(package, subject)),
        f"{root}/answer-rules.yaml": _dump(
            _answer_rules(
                package,
                subject,
                [row for code in topics for row in banks[code]],
            )
        ),
    }
    for code in topics:
        card = cards[code]
        files[f"{root}/topics/{code}.yaml"] = _dump(
            _topic_document(package, code, card, banks[code], created[code])
        )
        for block in blocks[code]:
            files[f"{root}/blocks/{code}/{block['id'].rsplit('.', 1)[-1]}.md"] = (
                _block_text(package, code, block)
            )
        for task_type in created[code]:
            pool = variants.get(task_type["id"]) or []
            if not pool:
                continue
            files[f"{root}/variants/{code}/T{task_type['number']:02d}.yaml"] = _dump(
                {"type": task_type["id"], "variants": pool}
            )
    lesson_name = _lesson_name(plan["data"]["lesson"])
    files[f"{root}/lessons/{lesson_name}.yaml"] = _dump(
        _lesson_document(
            package, plan, lesson_name, duration, stages, budget, homework, notes
        )
    )
    return files


def _package(package: str, subject: str) -> dict[str, Any]:
    return {
        "id": package,
        "title": PACKAGE_TITLES[subject],
        "subject": subject,
        "spec_version": "1",
        "answer_rules": "answer-rules.yaml",
    }


def _answer_rules(
    package: str, subject: str, bank_rows: list[dict[str, Any]]
) -> dict[str, Any]:
    formats: list[str] = []
    positions: list[str] = []
    for task_type in bank_rows:
        if task_type["answer_format"] not in formats:
            formats.append(task_type["answer_format"])
        latin = _latin_position(task_type["position"])
        if latin and latin not in positions:
            positions.append(latin)
    if not positions:
        positions = ["A1"]
    if not formats:
        formats = ["ct.integer"]
    return {
        "id": package,
        "title": "Правила ответа синтетического пакета",
        "subject": subject,
        "spec_version": "1",
        "positions": positions,
        "answer_formats": [
            {"id": format_id, **_FORMAT_FIELDS[format_id]} for format_id in formats
        ],
    }


def _topic_document(
    package: str,
    code: str,
    card: dict[str, Any],
    bank: list[dict[str, Any]],
    created: list[dict[str, Any]],
) -> dict[str, Any]:
    data = card["data"]
    citation = {"doc": "topic-card", "ref": card["relative"]}
    topic_id = f"{package}:{code}"
    prereqs = [
        f"{package}:{item}.E1"
        for item in data.get("prereqs") or []
        if isinstance(item, str)
    ]
    hours = _decimal(data["hours_lesson"]) + _decimal(data["hours_home"])
    total = sum(row["difficulty"] for row in bank) or 1
    types = []
    for task_type in created:
        base, automatism = level_hours(hours, task_type["difficulty"], total)
        types.append(
            {
                "id": task_type["id"],
                "title": task_type["title"],
                "kind": task_type["kind"],
                "primary_element": f"{topic_id}.E1",
                "prerequisites": list(prereqs),
                "answer_format": task_type["answer_format"],
                "difficulty": task_type["difficulty"],
                "levels": {
                    "base": {"hours": base},
                    "automatism": {"hours": automatism},
                },
                "citations": [
                    {"doc": "topic-card", "ref": f"{code} §Банк {task_type['number']}"}
                ],
                "subvariants": [
                    {"id": f"{task_type['id']}.S1", "given": task_type["given"]}
                ],
            }
        )
    document: dict[str, Any] = {
        "id": topic_id,
        "title": data["title"],
        "section": data["section"],
        "foundation_level": data["level"],
        "topic_hours": _number(hours),
        "prerequisite_elements": prereqs,
        "citations": [citation],
        "elements": [
            {
                "id": f"{topic_id}.E1",
                "title": "тема целиком",
                "citations": [dict(citation)],
            }
        ],
        "types": types,
    }
    positions = data.get("ct_positions")
    if isinstance(positions, list):
        document["ct_positions"] = positions
    return document


def _block_text(package: str, code: str, block: dict[str, Any]) -> str:
    front = {
        "id": block["id"],
        "topic_id": f"{package}:{code}",
        "kind": block["kind"],
        "title": block["title"],
        "citations": [
            {"doc": "topic-card", "ref": f"{code} §Теория шаг {block['number']}"}
        ],
    }
    body = block["body"] if block["body"].endswith("\n") else block["body"] + "\n"
    return f"---\n{_dump(front)}---\n{body}"


def _lesson_document(
    package: str,
    plan: dict[str, Any],
    lesson_name: str,
    duration: int,
    stages: list[dict[str, Any]],
    budget: Decimal,
    homework: list[dict[str, Any]],
    notes: str,
) -> dict[str, Any]:
    lesson_id = f"{package}:{lesson_name}"
    document: dict[str, Any] = {
        "id": lesson_id,
        "topics": [f"{package}:{code}" for code in plan["data"]["topics"]],
        "review": [f"{package}:{code}" for code in plan["data"]["review"]],
        "duration_min": duration,
        "stages": [
            {
                "id": f"{lesson_id}.G{index}",
                "minutes": [stage["start"], stage["end"]],
                "title": stage["title"],
                "kind": stage["kind"],
                "blocks": list(stage["blocks"]),
                "practice": list(stage["practice"]),
            }
            for index, stage in enumerate(stages, start=1)
        ],
        "homework": {"budget_hours": _number(budget), "items": homework},
    }
    if notes:
        document["generation_notes"] = notes
    return document


def _answer_format(position: str) -> str | None:
    if not position:
        return None
    latin = position.replace("А", "A").replace("В", "B")
    match = _POSITION.fullmatch(latin)
    if match is None:
        return None
    letter, number = match.group(1), int(match.group(2))
    if letter == "A":
        if "N" in position or "неск" in position.casefold():
            return "ct.choiceN"
        return "ct.choice1"
    if number == 1:
        return "ct.match"
    if number == 2:
        return "ct.digits"
    return "ct.integer"


def _latin_position(position: str) -> str | None:
    latin = position.replace("А", "A").replace("В", "B")
    if re.fullmatch(r"[AB][1-9][0-9]?", latin):
        return latin
    return None


def _stage_for(stages: list[dict[str, Any]], label: str) -> dict[str, Any] | None:
    exact = [stage for stage in stages if stage["title"] == label]
    if exact:
        return exact[0]
    prefix = [
        stage
        for stage in stages
        if stage["title"].startswith(label) or label.startswith(stage["title"])
    ]
    if prefix:
        return prefix[0]
    return None


def _bank_row(
    banks: dict[str, list[dict[str, Any]]], code: str, number: int
) -> dict[str, Any] | None:
    for row in banks.get(code, []):
        if row["number"] == number:
            return row
    return None


def _theory_block(
    blocks: dict[str, list[dict[str, Any]]], code: str, number: int
) -> dict[str, Any] | None:
    for block in blocks.get(code, []):
        if block["number"] == number and block["kind"] == "theory":
            return block
    return None


def _lesson_name(number: int) -> str:
    if number < 100:
        return f"L{number:02d}"
    return f"L{number}"


def _read(path: Path, relative: str, errors: list[dict[str, Any]]) -> str | None:
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeError:
        errors.append(_problem("", relative, 1, "файл не в кодировке UTF-8"))
        return None


def _frontmatter(
    text: str, relative: str, errors: list[dict[str, Any]]
) -> tuple[dict[str, Any] | None, str | None]:
    if not text.startswith("---\n"):
        errors.append(_problem("", relative, 1, "нет frontmatter"))
        return None, None
    parts = text.split("---\n", 2)
    if len(parts) < 3:
        errors.append(_problem("", relative, 1, "frontmatter не закрыт"))
        return None, None
    try:
        data = yaml.safe_load(parts[1])
    except yaml.YAMLError:
        errors.append(_problem("", relative, 1, "frontmatter не разбирается"))
        return None, None
    if not isinstance(data, dict):
        errors.append(_problem("", relative, 1, "frontmatter должен быть объектом"))
        return None, None
    return data, parts[2]


def _section_body(body: str, title: str) -> str:
    for heading, text in _h2(body):
        if heading == title or heading.startswith((title + ":", title + " ·")):
            return text
    return ""


def _task_lines(text: str) -> list[str]:
    lines: list[str] = []
    for raw in text.splitlines():
        stripped = raw.strip()
        if not stripped:
            continue
        if re.fullmatch(r"\d+\.\s+.+", stripped):
            lines.append(re.sub(r"^\d+\.\s+", "", stripped))
            continue
        if stripped.startswith("- "):
            lines.append(stripped[2:].strip())
            continue
        if stripped.startswith("**"):
            lines.append(stripped)
    return lines


def _named_section(body: str, pattern: re.Pattern[str]) -> tuple[str | None, str]:
    for heading, text in _h2(body):
        if pattern.search(heading):
            return heading, text
    return None, ""


def _h2(body: str) -> list[tuple[str, str]]:
    return _headings(body, "## ")


def _h3(body: str) -> list[tuple[str, str]]:
    return _headings(body, "### ")


def _headings(body: str, marker: str) -> list[tuple[str, str]]:
    found: list[tuple[str, str]] = []
    for part in re.split(r"(?m)^" + re.escape(marker), body)[1:]:
        title, _, text = part.partition("\n")
        found.append(
            (title.strip(), text if text.endswith("\n") or text == "" else text + "\n")
        )
    return found


def _table(text: str) -> list[dict[str, str]]:
    lines = [line.strip() for line in text.splitlines() if line.strip().startswith("|")]
    if len(lines) < 2:
        return []
    headers = _cells(lines[0])
    rows: list[dict[str, str]] = []
    for line in lines[1:]:
        cells = _cells(line)
        if len(cells) != len(headers):
            continue
        if all(re.fullmatch(r":?-{3,}:?", cell) for cell in cells):
            continue
        rows.append(dict(zip(headers, cells, strict=True)))
    return rows


def _cells(line: str) -> list[str]:
    stripped = line.strip()
    stripped = stripped.removeprefix("|")
    stripped = stripped.removesuffix("|")
    return [cell.strip() for cell in stripped.split("|")]


def _bullets(text: str) -> list[str]:
    found = []
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("- "):
            found.append(stripped[2:].strip())
    return found


def _int_cell(value: str | None) -> int | None:
    if value is None or not re.fullmatch(r"\d+", value.strip()):
        return None
    return int(value.strip())


def _decimal(value: Any) -> Decimal:
    if isinstance(value, Decimal):
        return value
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        raise TypeError("число не разбирается")
    return Decimal(str(value))


def _number(value: Decimal) -> int | float:
    if value == value.to_integral_value():
        return int(value)
    return float(value)


def _quarter_up(value: Decimal) -> Decimal:
    step = Decimal("0.25")
    if value <= step:
        return step
    quarters = (value / step).to_integral_value(rounding=ROUND_CEILING)
    return quarters * step


def _dump(data: object) -> str:
    return yaml.safe_dump(data, allow_unicode=True, sort_keys=False, width=4096)
