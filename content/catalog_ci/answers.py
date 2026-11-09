"""Проверки ключей, опций и пула вариантов: AR-R16–AR-R20, VR-R1, VR-R2, VR-R4, VR-R10."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Iterable
import re
from typing import Any

from .load import Document
from .problem import Problem


_ANSWER_CODES = frozenset(
    {
        "key_not_integer",
        "key_digits_order",
        "key_match_incomplete",
        "options_not_distinct",
        "distractor_equals_key",
        "correct_count_mismatch",
        "trap_missing",
        "pool_too_small",
        "reinforcement_impossible",
        "minutes_out_of_range",
    }
)

_INTEGER_KEY = re.compile(r"^-?[0-9]+$")
_ASCII_DIGITS = set("0123456789")
_MATCH_LETTERS = frozenset({"\u0410", "\u0411", "\u0412"})  # А Б В
_CHOICE_FORMATS = frozenset({"ct.choice1", "ct.choiceN"})
_OPTION_NUMBERS = frozenset({1, 2, 3, 4, 5})
_POOL_MINIMUM = 5
_REINFORCEMENT_MINIMUM = 2


def answer_problems(documents: Iterable[Document]) -> list[Problem]:
    """Возвращает проблемы ключей, опций, минут и пула по загруженным файлам."""

    docs = list(documents)
    formats = _type_formats(docs)
    phrases = _scale_phrases(docs)
    problems: list[Problem] = []
    active_by_type: dict[str, int] = defaultdict(int)
    active_by_subvariant: dict[str, int] = defaultdict(int)
    file_by_type: dict[str, str] = {}

    for document in docs:
        if document.definition != "variants_file" or not isinstance(document.data, dict):
            continue
        type_id = document.data.get("type")
        type_name = type_id if isinstance(type_id, str) else None
        if type_name is not None and type_name not in file_by_type:
            file_by_type[type_name] = document.file
        format_id = formats.get(type_name) if type_name is not None else None
        variants = document.data.get("variants")
        if not isinstance(variants, list):
            continue
        package_phrases = phrases.get(document.package_dir, ())
        for index, variant in enumerate(variants):
            if not isinstance(variant, dict):
                continue
            problems.extend(
                _variant_problems(document, index, variant, format_id, package_phrases)
            )
            if not _is_active(variant):
                continue
            if type_name is not None:
                active_by_type[type_name] += 1
            subvariant = variant.get("subvariant")
            if isinstance(subvariant, str):
                active_by_subvariant[subvariant] += 1

    problems.extend(
        _pool_problems(docs, active_by_type, active_by_subvariant, file_by_type)
    )
    return problems


def drop_shadowed_schema_problems(problems: list[Problem]) -> list[Problem]:
    """Убирает schema_invalid с указателя, где уже есть код ключа, опций или пула."""

    owned = {
        (problem.file, problem.pointer)
        for problem in problems
        if problem.code in _ANSWER_CODES
    }
    if not owned:
        return problems
    return [
        problem
        for problem in problems
        if problem.code != "schema_invalid" or (problem.file, problem.pointer) not in owned
    ]


def _variant_problems(
    document: Document,
    index: int,
    variant: dict[str, Any],
    format_id: str | None,
    phrases: tuple[str, ...],
) -> list[Problem]:
    problems: list[Problem] = []
    if not _minutes_ok(variant.get("minutes")):
        problems.append(
            _problem(
                document,
                "minutes_out_of_range",
                f"/variants/{index}/minutes",
                "Оценка в минутах должна быть целым числом от 1 до 60.",
            )
        )
    if format_id == "ct.integer":
        problems.extend(_integer_key_problems(document, index, variant, phrases))
    elif format_id == "ct.digits":
        problems.extend(_digits_key_problems(document, index, variant))
    elif format_id == "ct.match":
        problems.extend(_match_key_problems(document, index, variant))
    elif format_id in _CHOICE_FORMATS:
        problems.extend(_choice_problems(document, index, variant, format_id))
    return problems


def _integer_key_problems(
    document: Document,
    index: int,
    variant: dict[str, Any],
    phrases: tuple[str, ...],
) -> list[Problem]:
    if _is_integer_key(variant.get("key")):
        return []
    if _statement_has_phrase(variant.get("statement"), phrases):
        return []
    return [
        _problem(
            document,
            "key_not_integer",
            f"/variants/{index}/key",
            "Ключ ct.integer не целый, и в условии нет фразы масштаба.",
        )
    ]


def _digits_key_problems(
    document: Document, index: int, variant: dict[str, Any]
) -> list[Problem]:
    if _digits_key_ok(variant.get("key")):
        return []
    return [
        _problem(
            document,
            "key_digits_order",
            f"/variants/{index}/key",
            "Ключ ct.digits должен быть строкой цифр по возрастанию без повторов.",
        )
    ]


def _match_key_problems(
    document: Document, index: int, variant: dict[str, Any]
) -> list[Problem]:
    if _match_key_ok(variant.get("key")):
        return []
    return [
        _problem(
            document,
            "key_match_incomplete",
            f"/variants/{index}/key",
            "Ключ ct.match должен задавать ровно одну цифру для каждой из букв А, Б и В.",
        )
    ]


def _choice_problems(
    document: Document,
    index: int,
    variant: dict[str, Any],
    format_id: str,
) -> list[Problem]:
    problems: list[Problem] = []
    key_numbers = _choice_numbers(variant.get("key"))
    if format_id == "ct.choice1":
        count_ok = len(key_numbers) == 1
    else:
        count_ok = len(key_numbers) >= 2
    if not count_ok:
        problems.append(
            _problem(
                document,
                "correct_count_mismatch",
                f"/variants/{index}/key",
                "Число номеров в ключе не совпадает с форматом ответа.",
            )
        )

    options = variant.get("options")
    items = _option_items(options)
    if not _options_distinct(options, items):
        problems.append(
            _problem(
                document,
                "options_not_distinct",
                f"/variants/{index}/options",
                "Опции части А должны быть пятью различными текстами с номерами 1–5.",
            )
        )

    correct_texts = {
        text
        for _, option in items
        if _option_in_key(option, key_numbers)
        for text in (_normalize_text(option.get("text")),)
        if text is not None
    }
    traps = _trap_descriptions(variant)
    for position, option in items:
        if _option_in_key(option, key_numbers):
            continue
        text = _normalize_text(option.get("text"))
        if text is not None and text in correct_texts:
            problems.append(
                _problem(
                    document,
                    "distractor_equals_key",
                    f"/variants/{index}/options/{position}/text",
                    "Текст неверной опции совпадает с текстом верной.",
                )
            )
        if not _trap_ok(option, traps):
            problems.append(
                _problem(
                    document,
                    "trap_missing",
                    f"/variants/{index}/options/{position}/trap_id",
                    "У неверной опции нет ловушки с непустым описанием.",
                )
            )
    return problems


def _pool_problems(
    documents: list[Document],
    active_by_type: dict[str, int],
    active_by_subvariant: dict[str, int],
    file_by_type: dict[str, str],
) -> list[Problem]:
    used_types, used_subvariants = _lesson_mentions(documents)
    problems: list[Problem] = []
    for type_id in used_types:
        if active_by_type.get(type_id, 0) >= _POOL_MINIMUM:
            continue
        lesson_file, lesson_pointer = used_types[type_id]
        file = file_by_type.get(type_id, lesson_file)
        pointer = "/variants" if type_id in file_by_type else lesson_pointer
        problems.append(
            Problem(
                "pool_too_small",
                file,
                pointer,
                f"У типа {type_id!r}, который используется в уроке, меньше 5 активных вариантов.",
            )
        )
    for subvariant_id, (file, pointer) in used_subvariants.items():
        if active_by_subvariant.get(subvariant_id, 0) >= _REINFORCEMENT_MINIMUM:
            continue
        problems.append(
            Problem(
                "reinforcement_impossible",
                file,
                pointer,
                (
                    f"У подварианта {subvariant_id!r} из практики урока "
                    "меньше 2 активных вариантов."
                ),
            )
        )
    return problems


def _lesson_mentions(
    documents: list[Document],
) -> tuple[dict[str, tuple[str, str]], dict[str, tuple[str, str]]]:
    used_types: dict[str, tuple[str, str]] = {}
    used_subvariants: dict[str, tuple[str, str]] = {}
    for document in documents:
        if document.definition != "lesson_template" or not isinstance(document.data, dict):
            continue
        _remember_practice(document, used_types, used_subvariants)
        _remember_recipe(document, used_types)
    return used_types, used_subvariants


def _remember_practice(
    document: Document,
    used_types: dict[str, tuple[str, str]],
    used_subvariants: dict[str, tuple[str, str]],
) -> None:
    stages = document.data.get("stages")
    if not isinstance(stages, list):
        return
    for stage_index, stage in enumerate(stages):
        if not isinstance(stage, dict):
            continue
        practice = stage.get("practice")
        if not isinstance(practice, list):
            continue
        for item_index, item in enumerate(practice):
            if not isinstance(item, dict):
                continue
            base = f"/stages/{stage_index}/practice/{item_index}"
            _remember(used_types, item.get("type"), document.file, f"{base}/type")
            _remember(
                used_subvariants,
                item.get("subvariant"),
                document.file,
                f"{base}/subvariant",
            )


def _remember_recipe(document: Document, used_types: dict[str, tuple[str, str]]) -> None:
    homework = document.data.get("homework")
    if not isinstance(homework, dict):
        return
    items = homework.get("items")
    if not isinstance(items, list):
        return
    for item_index, item in enumerate(items):
        if not isinstance(item, dict):
            continue
        series = item.get("series")
        if not isinstance(series, list):
            continue
        for series_index, row in enumerate(series):
            if not isinstance(row, dict):
                continue
            pointer = f"/homework/items/{item_index}/series/{series_index}/type"
            _remember(used_types, row.get("type"), document.file, pointer)


def _remember(
    store: dict[str, tuple[str, str]], value: Any, file: str, pointer: str
) -> None:
    if isinstance(value, str) and value not in store:
        store[value] = (file, pointer)


def _type_formats(documents: Iterable[Document]) -> dict[str, str]:
    formats: dict[str, str] = {}
    for document in documents:
        if document.definition != "topic" or not isinstance(document.data, dict):
            continue
        types = document.data.get("types")
        if not isinstance(types, list):
            continue
        for task_type in types:
            if not isinstance(task_type, dict):
                continue
            type_id = task_type.get("id")
            answer_format = task_type.get("answer_format")
            if (
                isinstance(type_id, str)
                and isinstance(answer_format, str)
                and type_id not in formats
            ):
                formats[type_id] = answer_format
    return formats


def _scale_phrases(documents: Iterable[Document]) -> dict[Any, tuple[str, ...]]:
    phrases: dict[Any, tuple[str, ...]] = {}
    for document in documents:
        if document.definition != "subject_package" or not isinstance(document.data, dict):
            continue
        raw = document.data.get("scale_phrases")
        if not isinstance(raw, list):
            phrases[document.package_dir] = ()
            continue
        kept = [
            item
            for item in raw
            if isinstance(item, str) and item.strip() != ""
        ]
        phrases[document.package_dir] = tuple(kept)
    return phrases


def _statement_has_phrase(statement: Any, phrases: tuple[str, ...]) -> bool:
    if not isinstance(statement, str) or not phrases:
        return False
    return any(phrase in statement for phrase in phrases)


def _is_strict_int(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _is_integer_key(value: Any) -> bool:
    if _is_strict_int(value):
        return True
    return isinstance(value, str) and _INTEGER_KEY.fullmatch(value) is not None


def _digits_key_ok(value: Any) -> bool:
    if not isinstance(value, str) or not value:
        return False
    if any(char not in _ASCII_DIGITS for char in value):
        return False
    return all(value[index] < value[index + 1] for index in range(len(value) - 1))


def _match_key_ok(value: Any) -> bool:
    if not isinstance(value, str) or not value:
        return False
    seen: set[str] = set()
    index = 0
    size = len(value)
    while index < size:
        letter = value[index]
        if letter not in _MATCH_LETTERS or letter in seen:
            return False
        digit_index = index + 1
        if digit_index >= size or value[digit_index] not in _ASCII_DIGITS:
            return False
        if digit_index + 1 < size and value[digit_index + 1] in _ASCII_DIGITS:
            return False
        seen.add(letter)
        index += 2
    return seen == _MATCH_LETTERS


def _choice_numbers(value: Any) -> set[int]:
    if _is_strict_int(value):
        return {value}
    if not isinstance(value, list):
        return set()
    return {item for item in value if _is_strict_int(item)}


def _minutes_ok(value: Any) -> bool:
    return _is_strict_int(value) and 1 <= value <= 60


def _is_active(variant: dict[str, Any]) -> bool:
    return variant.get("status", "active") == "active"


def _option_items(options: Any) -> list[tuple[int, dict[str, Any]]]:
    if not isinstance(options, list):
        return []
    return [
        (index, item)
        for index, item in enumerate(options)
        if isinstance(item, dict)
    ]


def _options_distinct(options: Any, items: list[tuple[int, dict[str, Any]]]) -> bool:
    if not isinstance(options, list) or len(options) != 5 or len(items) != 5:
        return False
    numbers: list[int] = []
    texts: list[str] = []
    for _, option in items:
        number = option.get("n")
        text = _normalize_text(option.get("text"))
        if not _is_strict_int(number) or text is None:
            return False
        numbers.append(number)
        texts.append(text)
    return set(numbers) == _OPTION_NUMBERS and len(set(texts)) == len(texts)


def _option_in_key(option: dict[str, Any], key_numbers: set[int]) -> bool:
    number = option.get("n")
    return _is_strict_int(number) and number in key_numbers


def _normalize_text(value: Any) -> str | None:
    if not isinstance(value, str):
        return None
    return "".join(value.split())


def _trap_descriptions(variant: dict[str, Any]) -> dict[str, bool]:
    """id ловушки → есть ли у неё непустое описание."""

    traps = variant.get("traps")
    found: dict[str, bool] = {}
    if not isinstance(traps, list):
        return found
    for trap in traps:
        if not isinstance(trap, dict):
            continue
        trap_id = trap.get("id")
        if not isinstance(trap_id, str):
            continue
        description = trap.get("description")
        filled = isinstance(description, str) and description.strip() != ""
        found[trap_id] = found.get(trap_id, False) or filled
    return found


def _trap_ok(option: dict[str, Any], traps: dict[str, bool]) -> bool:
    trap_id = option.get("trap_id")
    if not isinstance(trap_id, str):
        return False
    return traps.get(trap_id, False)


def _problem(document: Document, code: str, pointer: str, message: str) -> Problem:
    return Problem(code, document.file, pointer, message)
