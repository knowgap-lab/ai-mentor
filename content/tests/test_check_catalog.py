"""Приёмочные тесты пакета 1 проверки структуры каталога."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

from conftest import mutate_frontmatter, mutate_yaml


PACKAGE = "ct-demo"


def check(root: Path, *, mode: str = "pr") -> list[object]:
    """Вызывает публичный API; импорт намеренно происходит только во время теста."""
    from catalog_ci import Problem, check_catalog

    problems = check_catalog(root, mode=mode)
    assert all(isinstance(problem, Problem) for problem in problems)
    return problems


def problem_codes(problems: list[object]) -> set[str]:
    """Возвращает коды без привязки теста к количеству сопутствующих проблем."""
    return {problem.code for problem in problems}


@pytest.mark.parametrize("mode", ["pr", "main"])
def test_demo_catalog_has_no_problems(catalog_root: Path, mode: str) -> None:
    """CS-R1–CS-R30: неизменённый демо-каталог корректен в обоих режимах."""
    assert check(catalog_root, mode=mode) == []


@pytest.mark.parametrize(
    ("entity", "mutate"),
    [
        (
            "topic",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/topics/D1.yaml",
                lambda doc: doc.__setitem__("id", f"{PACKAGE}:D1.T1"),
            ),
        ),
        (
            "element",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/topics/D1.yaml",
                lambda doc: doc["elements"][0].__setitem__("id", f"{PACKAGE}:D1.E01"),
            ),
        ),
        (
            "type",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/topics/D1.yaml",
                lambda doc: doc["types"][0].__setitem__("id", f"{PACKAGE}:D1.T1"),
            ),
        ),
        (
            "subvariant",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/topics/D1.yaml",
                lambda doc: doc["types"][0]["subvariants"][0].__setitem__(
                    "id", f"{PACKAGE}:D1.T01.S01"
                ),
            ),
        ),
        (
            "variant",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/variants/D1/T01.yaml",
                lambda doc: doc["variants"][0].__setitem__(
                    "id", f"{PACKAGE}:D1.T01.S1.V01"
                ),
            ),
        ),
        (
            "block",
            lambda root: mutate_frontmatter(
                root,
                f"{PACKAGE}/blocks/D1/B01.md",
                lambda doc: doc.__setitem__("id", f"{PACKAGE}:D1.B1"),
            ),
        ),
        (
            "test",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/tests/TEST.2025-CT-V1.yaml",
                lambda doc: doc.__setitem__("id", f"{PACKAGE}:TEST.2025-CT"),
            ),
        ),
        (
            "lesson_template",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/lessons/L01.yaml",
                lambda doc: doc.__setitem__("id", f"{PACKAGE}:L1"),
            ),
        ),
        (
            "stage",
            lambda root: mutate_yaml(
                root,
                f"{PACKAGE}/lessons/L01.yaml",
                lambda doc: doc["stages"][0].__setitem__("id", f"{PACKAGE}:L01.G0"),
            ),
        ),
    ],
    ids=[
        "topic",
        "element",
        "type",
        "subvariant",
        "variant",
        "block",
        "test",
        "lesson_template",
        "stage",
    ],
)
def test_invalid_id_of_each_entity_kind_is_reported(
    catalog_root: Path,
    entity: str,
    mutate: Callable[[Path], None],
) -> None:
    """CS-R1, CS-R3, CS-R30: id каждого вида проверяется по своей схеме."""
    mutate(catalog_root)
    assert "id_invalid" in problem_codes(check(catalog_root)), entity


def test_duplicate_id_is_reported(catalog_root: Path) -> None:
    """CS-R4: один id не встречается дважды в загруженных пакетах."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][1].__setitem__("id", f"{PACKAGE}:D1.T01"),
    )
    assert "duplicate_id" in problem_codes(check(catalog_root))


def test_field_not_allowed_is_reported(catalog_root: Path) -> None:
    """CS-R29: поле tier не допускается в теме каталога."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc.__setitem__("tier", "B"),
    )
    assert "field_not_allowed" in problem_codes(check(catalog_root))


def test_other_schema_violation_is_reported(catalog_root: Path) -> None:
    """CS-R18: нарушение перечисления difficulty получает schema_invalid."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("difficulty", 4),
    )
    assert "schema_invalid" in problem_codes(check(catalog_root))


def test_empty_citations_are_reported(catalog_root: Path) -> None:
    """CS-R21: у типа есть хотя бы одна цитата."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("citations", []),
    )
    assert "citation_missing" in problem_codes(check(catalog_root))


def test_missing_primary_element_is_reported(catalog_root: Path) -> None:
    """CS-R9: у типа ровно один primary_element."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].pop("primary_element"),
    )
    assert "primary_element_missing" in problem_codes(check(catalog_root))


def test_prerequisite_that_is_not_an_element_is_reported(catalog_root: Path) -> None:
    """CS-R10: предпосылка типа ссылается только на элемент."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D2.yaml",
        lambda doc: doc["types"][0].__setitem__("prerequisites", [f"{PACKAGE}:D1"]),
    )
    assert "prereq_not_element" in problem_codes(check(catalog_root))


def test_prerequisite_cycle_is_reported(catalog_root: Path) -> None:
    """CS-R11: граф предпосылок типов не содержит циклов."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__(
            "prerequisites", [f"{PACKAGE}:D2.E1"]
        ),
    )
    assert "prereq_cycle" in problem_codes(check(catalog_root))


def test_automatism_hours_cannot_be_less_than_base(catalog_root: Path) -> None:
    """CS-R17: часы automatism не меньше часов base."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0]["levels"]["automatism"].__setitem__("hours", 0.25),
    )
    assert "level_hours_order" in problem_codes(check(catalog_root))


def test_unknown_answer_format_is_reported(catalog_root: Path) -> None:
    """CS-R16: answer_format типа объявлен в правилах пакета."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("answer_format", "ct.unknown"),
    )
    assert "answer_format_unknown" in problem_codes(check(catalog_root))


def _mutate_primary_element(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("primary_element", f"{PACKAGE}:D9.E1"),
    )


def _mutate_type_prerequisite(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/topics/D2.yaml",
        lambda doc: doc["types"][0].__setitem__("prerequisites", [f"{PACKAGE}:D9.E1"]),
    )


def _mutate_topic_prerequisite(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/topics/D2.yaml",
        lambda doc: doc.__setitem__("prerequisite_elements", [f"{PACKAGE}:D9.E1"]),
    )


def _mutate_variant_subvariant(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/variants/D1/T01.yaml",
        lambda doc: doc["variants"][0].__setitem__("subvariant", f"{PACKAGE}:D1.T01.S9"),
    )


def _mutate_variant_theory_block(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/variants/D1/T01.yaml",
        lambda doc: doc["variants"][0].__setitem__("theory_block", f"{PACKAGE}:D1.B09"),
    )


def _mutate_variants_file_type(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/variants/D1/T01.yaml",
        lambda doc: doc.__setitem__("type", f"{PACKAGE}:D1.T09"),
    )


def _mutate_appearance_type(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/tests/TEST.2025-CT-V1.yaml",
        lambda doc: doc["appearances"][0].__setitem__("type", f"{PACKAGE}:D1.T09"),
    )


def _mutate_block_topic(root: Path) -> None:
    mutate_frontmatter(
        root,
        f"{PACKAGE}/blocks/D1/B01.md",
        lambda doc: doc.__setitem__("topic_id", f"{PACKAGE}:D9"),
    )


def _mutate_lesson_topics(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["topics"].__setitem__(0, f"{PACKAGE}:D9"),
    )


def _mutate_lesson_review(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc.__setitem__("review", [f"{PACKAGE}:D9"]),
    )


def _mutate_lesson_stage_block(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["stages"][0]["blocks"].__setitem__(0, f"{PACKAGE}:D1.B09"),
    )


def _mutate_lesson_practice_type(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["stages"][1]["practice"][0].__setitem__("type", f"{PACKAGE}:D1.T09"),
    )


def _mutate_lesson_practice_subvariant(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["stages"][1]["practice"][0].__setitem__(
            "subvariant", f"{PACKAGE}:D1.T01.S9"
        ),
    )


def _mutate_lesson_practice_block(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["stages"][1]["practice"][0].__setitem__(
            "theory_block", f"{PACKAGE}:D1.B09"
        ),
    )


def _mutate_homework_series_type(root: Path) -> None:
    mutate_yaml(
        root,
        f"{PACKAGE}/lessons/L01.yaml",
        lambda doc: doc["homework"]["items"][0]["series"][0].__setitem__(
            "type", f"{PACKAGE}:D1.T09"
        ),
    )


@pytest.mark.parametrize(
    ("reference_kind", "mutate"),
    [
        ("primary_element", _mutate_primary_element),
        ("type.prerequisites", _mutate_type_prerequisite),
        ("topic.prerequisite_elements", _mutate_topic_prerequisite),
        ("variant.subvariant", _mutate_variant_subvariant),
        ("variant.theory_block", _mutate_variant_theory_block),
        ("variants_file.type", _mutate_variants_file_type),
        ("appearance.type", _mutate_appearance_type),
        ("block.topic_id", _mutate_block_topic),
        ("lesson.topics", _mutate_lesson_topics),
        ("lesson.review", _mutate_lesson_review),
        ("lesson.stage.blocks", _mutate_lesson_stage_block),
        ("lesson.practice.type", _mutate_lesson_practice_type),
        ("lesson.practice.subvariant", _mutate_lesson_practice_subvariant),
        ("lesson.practice.theory_block", _mutate_lesson_practice_block),
        ("lesson.homework.series.type", _mutate_homework_series_type),
    ],
    ids=[
        "primary_element",
        "type_prerequisites",
        "topic_prerequisite_elements",
        "variant_subvariant",
        "variant_theory_block",
        "variants_file_type",
        "appearance_type",
        "block_topic_id",
        "lesson_topics",
        "lesson_review",
        "lesson_stage_blocks",
        "lesson_practice_type",
        "lesson_practice_subvariant",
        "lesson_practice_theory_block",
        "lesson_homework_series_type",
    ],
)
def test_each_reference_kind_reports_a_dangling_reference(
    catalog_root: Path,
    reference_kind: str,
    mutate: Callable[[Path], None],
) -> None:
    """CS-R7–CS-R20: каждая ссылка каталога ведёт на существующий id."""
    mutate(catalog_root)
    assert "dangling_ref" in problem_codes(check(catalog_root)), reference_kind


def test_skill_type_with_an_appearance_is_reported(catalog_root: Path) -> None:
    """CS-R15: тип skill не встречается в появлении теста."""
    mutate_yaml(
        catalog_root,
        f"{PACKAGE}/topics/D1.yaml",
        lambda doc: doc["types"][1].__setitem__("kind", "skill"),
    )
    assert "skill_has_appearance" in problem_codes(check(catalog_root))


def _make_element_temporary(root: Path) -> None:
    temporary_id = "tmp:pending-element"

    def mutate_d1(doc: dict) -> None:
        doc["elements"][0]["id"] = temporary_id
        for task_type in doc["types"]:
            task_type["primary_element"] = temporary_id

    def mutate_d2(doc: dict) -> None:
        for task_type in doc["types"]:
            task_type["prerequisites"] = [temporary_id]

    mutate_yaml(root, f"{PACKAGE}/topics/D1.yaml", mutate_d1)
    mutate_yaml(root, f"{PACKAGE}/topics/D2.yaml", mutate_d2)


def test_temporary_key_is_allowed_in_pr_mode(catalog_root: Path) -> None:
    """CS-R6: временные ключи разрешены в режиме pr."""
    _make_element_temporary(catalog_root)
    assert check(catalog_root, mode="pr") == []


def test_temporary_key_is_reported_in_main_mode(catalog_root: Path) -> None:
    """CS-R6: временные ключи запрещены в режиме main."""
    _make_element_temporary(catalog_root)
    assert "tmp_key_in_main" in problem_codes(check(catalog_root, mode="main"))
