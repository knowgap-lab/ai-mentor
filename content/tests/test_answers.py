"""Ключи, опции и пул вариантов: AR-R16–AR-R20, VR-R1, VR-R2, VR-R4, VR-R10.

Импорт ``catalog_ci`` выполняется внутри вызова проверки, во время теста.
"""

from __future__ import annotations

import copy
from pathlib import Path

import pytest

from conftest import mutate_yaml


PACKAGE = "ct-demo"
RULES = f"{PACKAGE}/answer-rules.yaml"
TOPIC_D1 = f"{PACKAGE}/topics/D1.yaml"
LESSON = f"{PACKAGE}/lessons/L01.yaml"
INTEGER = f"{PACKAGE}/variants/D1/T01.yaml"
CHOICE = f"{PACKAGE}/variants/D1/T02.yaml"
OTHER_INTEGER = f"{PACKAGE}/variants/D2/T01.yaml"
OTHER_CHOICE = f"{PACKAGE}/variants/D2/T02.yaml"
S2 = f"{PACKAGE}:D1.T01.S2"
RECIPE_TYPE = f"{PACKAGE}:D1.T02"
VALID_DIGITS = "124"
VALID_MATCH = "\u04101\u04111\u04124"  # А1Б1В4


def check(root: Path, *, mode: str = "pr") -> list[object]:
    """Вызывает публичный API; импорт намеренно происходит только во время теста."""
    from catalog_ci import Problem, check_catalog

    problems = check_catalog(root, mode=mode)
    assert all(isinstance(problem, Problem) for problem in problems)
    return problems


def problem_codes(problems: list[object]) -> set[str]:
    """Возвращает коды без привязки теста к числу остальных проблем."""
    return {problem.code for problem in problems}


def expect(root: Path, code: str) -> list[object]:
    """Код есть, и на том же указателе нет второго ``schema_invalid``."""
    problems = check(root)
    hits = [problem for problem in problems if problem.code == code]
    assert hits, code
    places = {(problem.file, problem.pointer) for problem in hits}
    overlap = [
        problem
        for problem in problems
        if problem.code == "schema_invalid" and (problem.file, problem.pointer) in places
    ]
    assert overlap == [], (code, overlap)
    return problems


def reject(root: Path, code: str) -> set[str]:
    """Этого кода в отчёте нет."""
    found = problem_codes(check(root))
    assert code not in found, found
    return found


def allow_format(
    root: Path,
    format_id: str,
    *,
    title: str,
    applies_to: str,
    max_points: int,
) -> None:
    """Добавляет формат в таблицу пакета, иначе тип получит answer_format_unknown."""

    def mutate(doc: dict) -> None:
        doc["answer_formats"].append(
            {
                "id": format_id,
                "title": title,
                "applies_to": applies_to,
                "input": title,
                "max_points": max_points,
            }
        )

    mutate_yaml(root, RULES, mutate)


def set_type_format(root: Path, topic: str, index: int, format_id: str) -> None:
    """Меняет формат ответа у типа. Проверка берёт формат оттуда."""
    mutate_yaml(
        root,
        f"{PACKAGE}/topics/{topic}.yaml",
        lambda doc: doc["types"][index].__setitem__("answer_format", format_id),
    )


def set_keys(root: Path, relative: str, keys: list[object]) -> None:
    """Подставляет ключ каждому варианту файла по порядку."""

    def mutate(doc: dict) -> None:
        for variant, key in zip(doc["variants"], keys, strict=True):
            variant["key"] = key

    mutate_yaml(root, relative, mutate)


def use_digits(root: Path) -> None:
    """Переводит незадействованный в уроке тип D2.T01 на ct.digits."""
    allow_format(root, "ct.digits", title="цифры", applies_to="часть В", max_points=2)
    set_type_format(root, "D2", 0, "ct.digits")


def use_match(root: Path) -> None:
    """Переводит незадействованный в уроке тип D2.T01 на ct.match."""
    allow_format(
        root,
        "ct.match",
        title="соответствие",
        applies_to="часть В",
        max_points=2,
    )
    set_type_format(root, "D2", 0, "ct.match")


def use_choice_n(root: Path) -> None:
    """Переводит незадействованный в уроке тип D2.T02 на ct.choiceN."""
    allow_format(
        root,
        "ct.choiceN",
        title="несколько верных",
        applies_to="часть А",
        max_points=1,
    )
    set_type_format(root, "D2", 1, "ct.choiceN")


def match_key(kind: str) -> str:
    """Собирает ключ ct.match из явных кодов букв, без визуальных двойников."""
    keys = {
        "valid": "\u04101\u04111\u04124",  # А1Б1В4
        "missing-letter": "\u04101\u04111",  # А1Б1
        "latin": "\u00411\u04111\u04124",  # латинская A, дальше Б и В
        "repeated-letter": "\u04101\u04102\u04111\u04124",  # А1А2Б1В4
    }
    return keys[kind]


def move_tail_to_s2(root: Path, keep: int) -> None:
    """Оставляет первые варианты на S1, хвост переносит на новый подвариант S2."""
    mutate_yaml(
        root,
        TOPIC_D1,
        lambda doc: doc["types"][0]["subvariants"].append(
            {"id": S2, "given": "два других числа"}
        ),
    )

    def mutate(doc: dict) -> None:
        for index, variant in enumerate(doc["variants"][keep:], start=keep):
            number = index + 1
            variant["subvariant"] = S2
            variant["id"] = f"{PACKAGE}:D1.T01.S2.V{number:03d}"

    mutate_yaml(root, INTEGER, mutate)


def append_retired(doc: dict, number: int = 6) -> None:
    """Добавляет retired-копию первого варианта, не меняя активные."""
    extra = copy.deepcopy(doc["variants"][0])
    extra["id"] = f"{PACKAGE}:D1.T01.S1.V{number:03d}"
    extra["status"] = "retired"
    extra["retired_reason"] = "синтетический вывод"
    doc["variants"].append(extra)


def point_recipe_at_other_type(doc: dict) -> None:
    """Оставляет тип D1.T01 только в практике: ряд рецепта смотрит на D1.T02."""
    doc["homework"]["items"][0]["series"][0]["type"] = RECIPE_TYPE


def clear_practice(doc: dict) -> None:
    """Убирает тип из практики урока."""
    doc["stages"][1]["practice"] = []


@pytest.mark.parametrize(
    "key",
    ["2.5", 2.5, "+5", " 12 "],
    ids=["two-point-five", "float", "plus", "spaces"],
)
def test_ar_r16_key_not_integer(catalog_root: Path, key: object) -> None:
    """AR-R16: ct.integer, ключ не int и не строка целого, фразы масштаба нет."""
    mutate_yaml(
        catalog_root,
        INTEGER,
        lambda doc: doc["variants"][0].__setitem__("key", key),
    )
    expect(catalog_root, "key_not_integer")


def test_ar_r16_key_not_integer_absent_with_scale_phrase(catalog_root: Path) -> None:
    """AR-R16: фраза из scale_phrases пакета есть в условии как подстрока."""
    phrase = "ответ умножьте на 10"

    def scale(doc: dict) -> None:
        doc["scale_phrases"] = ["нет в условии", phrase]

    def variant(doc: dict) -> None:
        doc["variants"][0]["key"] = "2.5"
        doc["variants"][0]["statement"] = f"Начало {phrase}, дальше текст."

    mutate_yaml(catalog_root, RULES, scale)
    mutate_yaml(catalog_root, INTEGER, variant)
    reject(catalog_root, "key_not_integer")


def test_ar_r16_key_not_integer_without_listed_phrase(catalog_root: Path) -> None:
    """AR-R16: текст фразы в условии не считается, пока его нет в scale_phrases."""

    def mutate(doc: dict) -> None:
        doc["variants"][0]["key"] = "2.5"
        doc["variants"][0]["statement"] += " ответ умножьте на 10"

    mutate_yaml(catalog_root, INTEGER, mutate)
    expect(catalog_root, "key_not_integer")


@pytest.mark.parametrize("key", ["12", "-3", "007", 0, -4])
def test_ar_r16_key_not_integer_absent_for_integer_key(
    catalog_root: Path, key: object
) -> None:
    """AR-R16: int и строка ^-?[0-9]+$ — целый ключ, в том числе с ведущими нулями."""
    mutate_yaml(
        catalog_root,
        INTEGER,
        lambda doc: doc["variants"][0].__setitem__("key", key),
    )
    reject(catalog_root, "key_not_integer")


@pytest.mark.parametrize(
    "key",
    ["421", "112", "12a", 124],
    ids=["unsorted", "repeat", "not-digits", "int"],
)
def test_ar_r17_key_digits_order(catalog_root: Path, key: object) -> None:
    """AR-R17: ключ ct.digits не строка цифр строго по возрастанию без повторов."""
    use_digits(catalog_root)
    keys = [VALID_DIGITS] * 5
    keys[0] = key
    set_keys(catalog_root, OTHER_INTEGER, keys)
    expect(catalog_root, "key_digits_order")


def test_ar_r17_key_digits_order_absent_when_sorted(catalog_root: Path) -> None:
    """AR-R17: строка цифр по возрастанию без повторов валидна."""
    use_digits(catalog_root)
    set_keys(catalog_root, OTHER_INTEGER, [VALID_DIGITS] * 5)
    reject(catalog_root, "key_digits_order")


@pytest.mark.parametrize(
    "kind",
    ["missing-letter", "latin", "repeated-letter"],
)
def test_ar_r18_key_match_incomplete(catalog_root: Path, kind: str) -> None:
    """AR-R18: у ct.match нет ровно одной пары на каждую из букв А, Б, В."""
    use_match(catalog_root)
    key = match_key(kind)
    if kind == "latin":
        assert key[0] == "\u0041"
        assert "\u0410" not in key
    keys = [VALID_MATCH] * 5
    keys[0] = key
    set_keys(catalog_root, OTHER_INTEGER, keys)
    expect(catalog_root, "key_match_incomplete")


def test_ar_r18_key_match_incomplete_absent_for_valid_key(catalog_root: Path) -> None:
    """AR-R18: А1Б1В4 валиден, одна и та же цифра у разных букв допустима."""
    assert VALID_MATCH == "\u0410" + "1" + "\u0411" + "1" + "\u0412" + "4"
    use_match(catalog_root)
    set_keys(catalog_root, OTHER_INTEGER, [VALID_MATCH] * 5)
    reject(catalog_root, "key_match_incomplete")


@pytest.mark.parametrize(
    ("left", "right"),
    [("7", "7"), (" 1 3 ", "13")],
    ids=["same-text", "spaces"],
)
def test_ar_r19_options_not_distinct(
    catalog_root: Path, left: str, right: str
) -> None:
    """AR-R19: нормализованные тексты двух неверных опций совпали."""

    def mutate(doc: dict) -> None:
        options = doc["variants"][0]["options"]
        options[0]["text"] = left
        options[1]["text"] = right

    mutate_yaml(catalog_root, CHOICE, mutate)
    found = problem_codes(expect(catalog_root, "options_not_distinct"))
    assert "distractor_equals_key" not in found


def test_ar_r19_options_not_distinct_numbers(catalog_root: Path) -> None:
    """AR-R19: номера опций не составляют множество {1, 2, 3, 4, 5}."""

    def mutate(doc: dict) -> None:
        doc["variants"][0]["options"][4]["n"] = 4

    mutate_yaml(catalog_root, CHOICE, mutate)
    expect(catalog_root, "options_not_distinct")


def test_ar_r19_options_not_distinct_count(catalog_root: Path) -> None:
    """AR-R19: у ct.choice1 не пять опций."""
    mutate_yaml(
        catalog_root,
        CHOICE,
        lambda doc: doc["variants"][0]["options"].pop(),
    )
    expect(catalog_root, "options_not_distinct")


def test_ar_r19_distractor_equals_key(catalog_root: Path) -> None:
    """AR-R19: текст неверной опции после снятия пробелов равен тексту верной."""

    def mutate(doc: dict) -> None:
        options = doc["variants"][0]["options"]
        options[0]["text"] = " 5 "
        options[3]["text"] = "5"

    mutate_yaml(catalog_root, CHOICE, mutate)
    expect(catalog_root, "distractor_equals_key")


@pytest.mark.parametrize("key", [[1, 2], [1, 2, 3]], ids=["two", "three"])
def test_ar_r19_correct_count_mismatch(catalog_root: Path, key: list[int]) -> None:
    """AR-R19: у ct.choice1 в ключе не ровно один номер."""

    def mutate(doc: dict) -> None:
        variant = doc["variants"][0]
        variant["key"] = key
        variant["options"][3]["trap_id"] = "off_by_one"

    mutate_yaml(catalog_root, CHOICE, mutate)
    expect(catalog_root, "correct_count_mismatch")


def test_ar_r19_correct_count_mismatch_absent_for_choice1_integer(
    catalog_root: Path,
) -> None:
    """AR-R19: целое в ключе choice1 считается одним номером."""
    mutate_yaml(
        catalog_root,
        CHOICE,
        lambda doc: doc["variants"][0].__setitem__("key", 4),
    )
    reject(catalog_root, "correct_count_mismatch")


@pytest.mark.parametrize("key", [[1], 2], ids=["one-number", "integer"])
def test_ar_r19_correct_count_mismatch_choice_n(
    catalog_root: Path, key: object
) -> None:
    """AR-R19: у ct.choiceN в ключе меньше двух номеров, целое считается одним."""
    use_choice_n(catalog_root)
    keys: list[object] = [[1, 2]] * 5
    keys[0] = key
    set_keys(catalog_root, OTHER_CHOICE, keys)
    expect(catalog_root, "correct_count_mismatch")


def test_ar_r19_correct_count_mismatch_absent_when_choice_n_has_two(
    catalog_root: Path,
) -> None:
    """AR-R19: два номера в ключе ct.choiceN — не correct_count_mismatch."""
    use_choice_n(catalog_root)
    set_keys(catalog_root, OTHER_CHOICE, [[2, 4]] * 5)
    reject(catalog_root, "correct_count_mismatch")


@pytest.mark.parametrize("case", ["no-trap-id", "unknown-trap", "empty-description"])
def test_ar_r20_trap_missing(catalog_root: Path, case: str) -> None:
    """AR-R20: у неверной опции нет ловушки, id чужой или description пустое."""

    def mutate(doc: dict) -> None:
        variant = doc["variants"][0]
        if case == "no-trap-id":
            del variant["options"][0]["trap_id"]
        elif case == "unknown-trap":
            variant["options"][0]["trap_id"] = "not_in_variant"
        else:
            variant["traps"][0]["description"] = ""

    mutate_yaml(catalog_root, CHOICE, mutate)
    expect(catalog_root, "trap_missing")


def test_ar_r20_trap_missing_absent_on_correct_option(catalog_root: Path) -> None:
    """AR-R20: у верной опции ловушка не требуется."""
    reject(catalog_root, "trap_missing")


@pytest.mark.parametrize(
    "minutes",
    [0, 61, -1, 1.5],
    ids=["zero", "above-60", "negative", "fraction"],
)
def test_vr_r10_minutes_out_of_range(catalog_root: Path, minutes: object) -> None:
    """VR-R10: minutes варианта не целое от 1 до 60 включительно."""
    mutate_yaml(
        catalog_root,
        INTEGER,
        lambda doc: doc["variants"][0].__setitem__("minutes", minutes),
    )
    expect(catalog_root, "minutes_out_of_range")


@pytest.mark.parametrize("minutes", [1, 60], ids=["lower", "upper"])
def test_vr_r10_minutes_out_of_range_absent_at_bounds(
    catalog_root: Path, minutes: int
) -> None:
    """VR-R10: границы 1 и 60 входят в диапазон."""
    mutate_yaml(
        catalog_root,
        INTEGER,
        lambda doc: doc["variants"][0].__setitem__("minutes", minutes),
    )
    reject(catalog_root, "minutes_out_of_range")


def test_vr_r1_pool_too_small(catalog_root: Path) -> None:
    """VR-R1: тип упомянут в шаблоне урока, активных вариантов четыре."""
    mutate_yaml(catalog_root, INTEGER, lambda doc: doc["variants"].pop())
    found = problem_codes(expect(catalog_root, "pool_too_small"))
    assert "reinforcement_impossible" not in found


def test_vr_r1_pool_too_small_retired_not_counted(catalog_root: Path) -> None:
    """VR-R1: retired не считается, пять записей с одним retired — это четыре активных."""

    def mutate(doc: dict) -> None:
        doc["variants"][0]["status"] = "retired"
        doc["variants"][0]["retired_reason"] = "синтетический вывод"

    mutate_yaml(catalog_root, INTEGER, mutate)
    found = problem_codes(expect(catalog_root, "pool_too_small"))
    assert "reinforcement_impossible" not in found


def test_vr_r1_pool_too_small_absent_with_five_active_and_retired(
    catalog_root: Path,
) -> None:
    """VR-R1: пять активных и один retired — норма."""
    mutate_yaml(catalog_root, INTEGER, append_retired)
    found = reject(catalog_root, "pool_too_small")
    assert "reinforcement_impossible" not in found


def test_vr_r1_pool_too_small_absent_outside_lesson(catalog_root: Path) -> None:
    """VR-R1: тип вне практики и ряда рецепта не проверяется."""
    mutate_yaml(catalog_root, OTHER_INTEGER, lambda doc: doc["variants"].pop())
    found = reject(catalog_root, "pool_too_small")
    assert "reinforcement_impossible" not in found


def test_vr_r1_pool_too_small_when_type_only_in_practice(catalog_root: Path) -> None:
    """VR-R1: тип только в practice шаблона урока, активных меньше пяти."""
    mutate_yaml(catalog_root, LESSON, point_recipe_at_other_type)
    mutate_yaml(catalog_root, INTEGER, lambda doc: doc["variants"].pop())
    expect(catalog_root, "pool_too_small")


def test_vr_r1_pool_too_small_when_type_only_in_recipe(catalog_root: Path) -> None:
    """VR-R1: тип только в ряде рецепта, активных меньше пяти."""
    mutate_yaml(catalog_root, LESSON, clear_practice)
    mutate_yaml(catalog_root, INTEGER, lambda doc: doc["variants"].pop())
    expect(catalog_root, "pool_too_small")


def test_vr_r1_pool_too_small_absent_when_type_not_in_practice_or_recipe(
    catalog_root: Path,
) -> None:
    """VR-R1: тип убран и из практики, и из ряда рецепта."""
    mutate_yaml(catalog_root, LESSON, point_recipe_at_other_type)
    mutate_yaml(catalog_root, LESSON, clear_practice)
    mutate_yaml(catalog_root, INTEGER, lambda doc: doc["variants"].pop())
    reject(catalog_root, "pool_too_small")


def test_vr_r2_reinforcement_impossible(catalog_root: Path) -> None:
    """VR-R2: подвариант из practice имеет один активный вариант."""
    move_tail_to_s2(catalog_root, keep=1)
    found = problem_codes(expect(catalog_root, "reinforcement_impossible"))
    assert "pool_too_small" not in found


def test_vr_r2_reinforcement_impossible_absent_when_two_active(
    catalog_root: Path,
) -> None:
    """VR-R2: два активных варианта подварианта из practice достаточны."""
    move_tail_to_s2(catalog_root, keep=2)
    found = reject(catalog_root, "reinforcement_impossible")
    assert "pool_too_small" not in found


def test_vr_r2_reinforcement_impossible_absent_outside_practice(
    catalog_root: Path,
) -> None:
    """VR-R2: подвариант, не упомянутый в practice, не проверяется."""
    mutate_yaml(
        catalog_root,
        OTHER_INTEGER,
        lambda doc: doc.__setitem__("variants", doc["variants"][:1]),
    )
    found = reject(catalog_root, "reinforcement_impossible")
    assert "pool_too_small" not in found


def test_vr_r2_reinforcement_impossible_retired_not_counted(
    catalog_root: Path,
) -> None:
    """VR-R2: retired-вариант подварианта не добирает второй активный."""
    move_tail_to_s2(catalog_root, keep=1)
    mutate_yaml(catalog_root, INTEGER, append_retired)
    found = problem_codes(expect(catalog_root, "reinforcement_impossible"))
    assert "pool_too_small" not in found
