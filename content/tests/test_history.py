"""Проверки истории каталога: сравнение базы и результата PR.

Коды этого модуля — id_removed, id_reused, transfer_map_missing (CS-R4, CS-R5,
CS-R23, CS-R25). Импорт catalog_ci выполняется внутри вызова, чтобы сбор тестов
не зависел от новой сигнатуры.
"""

from __future__ import annotations

from pathlib import Path
import json
import shutil
import subprocess
import sys

import pytest
import yaml

from conftest import mutate_frontmatter, mutate_yaml


PACKAGE = "ct-demo"
CONTENT_ROOT = Path(__file__).resolve().parents[1]
_HISTORY = frozenset({"id_removed", "id_reused", "transfer_map_missing"})
_OMIT = object()

_TOPIC_D1 = f"{PACKAGE}/topics/D1.yaml"
_TOPIC_D2 = f"{PACKAGE}/topics/D2.yaml"
_LESSON = f"{PACKAGE}/lessons/L01.yaml"
_TEST = f"{PACKAGE}/tests/TEST.2025-CT-V1.yaml"
_BLOCK_D2_B02 = f"{PACKAGE}/blocks/D2/B02.md"
_VARIANTS_D1_T01 = f"{PACKAGE}/variants/D1/T01.yaml"
_VARIANTS_D2_T01 = f"{PACKAGE}/variants/D2/T01.yaml"

_TYPE_D1_T01 = f"{PACKAGE}:D1.T01"
_TYPE_D1_T02 = f"{PACKAGE}:D1.T02"
_TYPE_D1_T03 = f"{PACKAGE}:D1.T03"
_TYPE_D1_T04 = f"{PACKAGE}:D1.T04"
_TYPE_D2_T01 = f"{PACKAGE}:D2.T01"
_TYPE_D2_T02 = f"{PACKAGE}:D2.T02"
_ELEMENT_D1 = f"{PACKAGE}:D1.E1"
_ELEMENT_D2 = f"{PACKAGE}:D2.E1"
_SUB_D1_T01 = f"{PACKAGE}:D1.T01.S1"


def _files(root: Path) -> dict[str, bytes]:
    """Снимок байтов каталога: проверка не переписывает исходники."""
    return {
        path.relative_to(root).as_posix(): path.read_bytes()
        for path in root.rglob("*")
        if path.is_file()
    }


def pair(catalog_root: Path) -> tuple[Path, Path]:
    """Две независимые копии демо-каталога: база и результат."""
    base = catalog_root.parent / "base"
    result = catalog_root.parent / "result"
    shutil.copytree(catalog_root, base)
    shutil.copytree(catalog_root, result)
    return base, result


def check(root: Path, mode: str = "pr", base: object = _OMIT) -> list[object]:
    """Вызывает публичный API. base=_OMIT — прежний вызов из двух аргументов."""
    from catalog_ci import Problem, check_catalog

    before_root = _files(root)
    before_base = _files(base) if isinstance(base, Path) else None
    if base is _OMIT:
        problems = check_catalog(root, mode)
    else:
        problems = check_catalog(root, mode=mode, base=base)
    assert _files(root) == before_root
    if isinstance(base, Path):
        assert _files(base) == before_base
    assert all(isinstance(problem, Problem) for problem in problems)
    return problems


def _codes(problems: list[object]) -> set[str]:
    return {problem.code for problem in problems}


def _history(problems: list[object]) -> set[str]:
    return _codes(problems) & set(_HISTORY)


def _locations(problems: list[object], code: str) -> set[tuple[str, str]]:
    return {
        (problem.file, problem.pointer) for problem in problems if problem.code == code
    }


def _retire(node: dict) -> None:
    node["status"] = "retired"
    node["retired_reason"] = "выведен из использования"


def _write_map(root: Path, entries: list[dict]) -> None:
    path = root / PACKAGE / "transfer-map.yaml"
    path.write_text(
        yaml.safe_dump({"entries": entries}, allow_unicode=True, sort_keys=False),
        encoding="utf-8",
    )


def _apply(root: Path, relative: str, mutate, *, frontmatter: bool = False) -> None:
    if frontmatter:
        mutate_frontmatter(root, relative, mutate)
    else:
        mutate_yaml(root, relative, mutate)


def _type_ids(root: Path) -> set[str]:
    found: set[str] = set()
    for path in (root / PACKAGE / "topics").glob("*.yaml"):
        document = yaml.safe_load(path.read_text(encoding="utf-8"))
        for task_type in document["types"]:
            found.add(task_type["id"])
    return found


def _subvariants_by_type(root: Path) -> dict[str, set[str]]:
    grouped: dict[str, set[str]] = {}
    for path in (root / PACKAGE / "topics").glob("*.yaml"):
        document = yaml.safe_load(path.read_text(encoding="utf-8"))
        for task_type in document["types"]:
            grouped.setdefault(task_type["id"], set())
            for subvariant in task_type["subvariants"]:
                grouped[task_type["id"]].add(subvariant["id"])
    return grouped


def _variant_ids(root: Path) -> set[str]:
    found: set[str] = set()
    for path in (root / PACKAGE / "variants").glob("*/*.yaml"):
        document = yaml.safe_load(path.read_text(encoding="utf-8"))
        for variant in document["variants"]:
            found.add(variant["id"])
    return found


def _exam_type(type_id: str, title: str, subvariants: list[dict]) -> dict:
    return {
        "id": type_id,
        "title": title,
        "kind": "exam",
        "primary_element": _ELEMENT_D1,
        "prerequisites": [],
        "answer_format": "ct.integer",
        "difficulty": 1,
        "levels": {"base": {"hours": 0.5}, "automatism": {"hours": 1.0}},
        "citations": [{"doc": "topic-card", "ref": "D1"}],
        "subvariants": subvariants,
    }


def run_cli(*arguments: str) -> subprocess.CompletedProcess[str]:
    """Запускает CLI из каталога content."""
    return subprocess.run(
        [sys.executable, "-m", "catalog_ci", *arguments],
        cwd=CONTENT_ROOT,
        capture_output=True,
        text=True,
        check=False,
    )


@pytest.mark.parametrize("mode", ["pr", "main"])
def test_cs_20_id_removed(catalog_root: Path, mode: str) -> None:
    """CS-R4, cs-20: из результата удалён id типа, который был в базе.

    Вместе с типом исчезает его подвариант: сравнение идёт по каждому id.
    file и pointer этого случая не проверяются: файл темы в результате остаётся,
    а бриф задаёт путь только когда файла в результате нет.
    """
    base, result = pair(catalog_root)
    mutate_yaml(
        result,
        _TOPIC_D1,
        lambda document: document.__setitem__(
            "types",
            [item for item in document["types"] if item["id"] != _TYPE_D1_T02],
        ),
    )
    problems = check(result, mode, base=base)
    removed = [problem for problem in problems if problem.code == "id_removed"]
    assert len(removed) == 2
    assert _history(problems) == {"id_removed"}


def test_id_removed_uses_base_location_when_file_is_gone(catalog_root: Path) -> None:
    """CS-R4: удалённый вместе с файлом id сообщает путь и указатель в базе.

    Покрыты тема, элемент, тип, подвариант, вариант, блок, тест, шаблон урока
    и этап. Файл вариантов D2/T02 и блок D2.B01 остаются, их id не удалены.
    """
    base, result = pair(catalog_root)
    for relative in (
        _TOPIC_D2,
        _VARIANTS_D2_T01,
        _BLOCK_D2_B02,
        _TEST,
        _LESSON,
    ):
        (result / relative).unlink()

    problems = check(result, base=base)
    expected = {
        (_TOPIC_D2, "/id"),
        (_TOPIC_D2, "/elements/0/id"),
        (_TOPIC_D2, "/types/0/id"),
        (_TOPIC_D2, "/types/0/subvariants/0/id"),
        (_TOPIC_D2, "/types/1/id"),
        (_TOPIC_D2, "/types/1/subvariants/0/id"),
        (_VARIANTS_D2_T01, "/variants/0/id"),
        (_VARIANTS_D2_T01, "/variants/1/id"),
        (_VARIANTS_D2_T01, "/variants/2/id"),
        (_VARIANTS_D2_T01, "/variants/3/id"),
        (_VARIANTS_D2_T01, "/variants/4/id"),
        (_BLOCK_D2_B02, "/id"),
        (_TEST, "/id"),
        (_LESSON, "/id"),
        (_LESSON, "/stages/0/id"),
        (_LESSON, "/stages/1/id"),
    }
    assert _locations(problems, "id_removed") == expected
    assert "id_reused" not in _history(problems)
    assert "transfer_map_missing" not in _history(problems)


def test_id_removed_absent_when_status_retired(catalog_root: Path) -> None:
    """CS-R23: сущность со status retired остаётся в результате, id_removed нет."""
    base, result = pair(catalog_root)
    _apply(result, _BLOCK_D2_B02, _retire, frontmatter=True)
    problems = check(result, base=base)
    assert _history(problems) == set()


@pytest.mark.parametrize("mode", ["pr", "main"])
@pytest.mark.parametrize(
    ("entity", "relative", "pointer", "frontmatter"),
    [
        ("topic", _TOPIC_D1, "/id", False),
        ("element", _TOPIC_D1, "/elements/0/id", False),
        ("type", _TOPIC_D1, "/types/1/id", False),
        ("subvariant", _TOPIC_D1, "/types/0/subvariants/0/id", False),
        ("variant", _VARIANTS_D1_T01, "/variants/0/id", False),
        ("block", _BLOCK_D2_B02, "/id", True),
        ("test", _TEST, "/id", False),
    ],
    ids=["topic", "element", "type", "subvariant", "variant", "block", "test"],
)
def test_cs_21_id_reused(
    catalog_root: Path,
    mode: str,
    entity: str,
    relative: str,
    pointer: str,
    frontmatter: bool,
) -> None:
    """CS-R4, cs-21: в базе id retired, в результате тот же id и статус не retired.

    Строка type — пример cs-21. Нет поля status, сущность считается active.
    У темы в демо статус active записан явно. Один id даёт id_reused и не даёт
    id_removed.
    """
    base, result = pair(catalog_root)

    def retire_entity(document: dict) -> None:
        if entity == "topic" or entity == "block" or entity == "test":
            _retire(document)
        elif entity == "element":
            _retire(document["elements"][0])
        elif entity == "type":
            _retire(document["types"][1])
        elif entity == "subvariant":
            _retire(document["types"][0]["subvariants"][0])
        elif entity == "variant":
            _retire(document["variants"][0])
        else:
            raise AssertionError(entity)

    _apply(base, relative, retire_entity, frontmatter=frontmatter)
    problems = check(result, mode, base=base)
    assert _locations(problems, "id_reused") == {(relative, pointer)}
    assert "id_removed" not in _history(problems)
    assert "transfer_map_missing" not in _history(problems)


def test_id_reused_absent_when_retired_stays_retired(catalog_root: Path) -> None:
    """CS-R4: retired в базе и retired в результате не дают id_reused."""
    base, result = pair(catalog_root)

    def retire_subvariant(document: dict) -> None:
        _retire(document["types"][0]["subvariants"][0])

    mutate_yaml(base, _TOPIC_D1, retire_subvariant)
    mutate_yaml(result, _TOPIC_D1, retire_subvariant)
    problems = check(result, base=base)
    assert _history(problems) == set()


def test_cs_22_no_history_codes_when_type_changes_topic(catalog_root: Path) -> None:
    """CS-R5, cs-22: тип перенесён в другую тему, id прежний и не retired.

    Трёх кодов истории нет. Проверка не меняет id в копиях каталога.
    """
    base, result = pair(catalog_root)
    holder: dict[str, dict] = {}

    def take(document: dict) -> None:
        for index, item in enumerate(document["types"]):
            if item["id"] == _TYPE_D1_T01:
                holder["type"] = document["types"].pop(index)
                return
        raise AssertionError(_TYPE_D1_T01)

    def put(document: dict) -> None:
        document["types"].append(holder["type"])

    mutate_yaml(result, _TOPIC_D1, take)
    mutate_yaml(result, _TOPIC_D2, put)
    assert holder["type"]["id"] == _TYPE_D1_T01
    assert holder["type"].get("status", "active") != "retired"
    assert _TYPE_D1_T01 in _type_ids(base)
    assert _TYPE_D1_T01 in _type_ids(result)

    problems = check(result, base=base)
    assert _history(problems) == set()
    assert _TYPE_D1_T01 in _type_ids(result)


@pytest.mark.parametrize("mode", ["pr", "main"])
def test_cs_30_transfer_map_missing(catalog_root: Path, mode: str) -> None:
    """CS-R25, cs-30: тип retired, в базе он был в тесте, карты результата нет."""
    base, result = pair(catalog_root)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["types"][0]))
    problems = check(result, mode, base=base)
    assert _locations(problems, "transfer_map_missing") == {(_TOPIC_D1, "/types/0/id")}
    assert "id_removed" not in _history(problems)
    assert "id_reused" not in _history(problems)


def test_transfer_map_missing_from_base_test_appearance(catalog_root: Path) -> None:
    """CS-R25: появления типа — его id в файле теста базы, даже без файла вариантов."""
    base, result = pair(catalog_root)

    def retarget(document: dict) -> None:
        document["type"] = _TYPE_D1_T02

    def drop_appearance(document: dict) -> None:
        document["appearances"] = [
            item for item in document["appearances"] if item["type"] != _TYPE_D1_T01
        ]

    mutate_yaml(base, _VARIANTS_D1_T01, retarget)
    mutate_yaml(result, _VARIANTS_D1_T01, retarget)
    mutate_yaml(result, _TEST, drop_appearance)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["types"][0]))
    problems = check(result, base=base)
    assert _locations(problems, "transfer_map_missing") == {(_TOPIC_D1, "/types/0/id")}


def test_transfer_map_missing_from_base_variants_file(catalog_root: Path) -> None:
    """CS-R25: варианты типа — файл вариантов базы с полем type, равным этому id."""
    base, result = pair(catalog_root)
    mutate_yaml(
        result,
        _VARIANTS_D2_T01,
        lambda document: document.__setitem__("type", _TYPE_D2_T02),
    )
    mutate_yaml(result, _TOPIC_D2, lambda document: _retire(document["types"][0]))
    problems = check(result, base=base)
    assert _locations(problems, "transfer_map_missing") == {(_TOPIC_D2, "/types/0/id")}


def test_transfer_map_missing_for_primary_element(catalog_root: Path) -> None:
    """CS-R25: у элемента были появления, если в базе он primary_element такого типа."""
    base, result = pair(catalog_root)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["elements"][0]))
    problems = check(result, base=base)
    assert _locations(problems, "transfer_map_missing") == {
        (_TOPIC_D1, "/elements/0/id")
    }


def test_transfer_map_missing_absent_without_base_use(catalog_root: Path) -> None:
    """CS-R25: retired-тип без появлений в тесте базы и без файла вариантов.

    Упоминание id в шаблоне урока базы появлением не считается.
    """
    base, result = pair(catalog_root)
    mutate_yaml(
        base,
        _VARIANTS_D2_T01,
        lambda document: document.__setitem__("type", _TYPE_D2_T02),
    )

    def mention(document: dict) -> None:
        document["stages"][1]["practice"].append(
            {
                "type": _TYPE_D2_T01,
                "subvariant": f"{PACKAGE}:D2.T01.S1",
                "role": "solve_alone",
                "theory_block": f"{PACKAGE}:D2.B01",
            }
        )

    mutate_yaml(base, _LESSON, mention)
    mutate_yaml(result, _TOPIC_D2, lambda document: _retire(document["types"][0]))
    problems = check(result, base=base)
    assert "transfer_map_missing" not in _history(problems)


def test_transfer_map_missing_absent_when_element_was_not_primary(
    catalog_root: Path,
) -> None:
    """CS-R25: элемент, который в базе не был primary_element занятого типа."""
    base, result = pair(catalog_root)

    def retarget_primary(document: dict) -> None:
        for task_type in document["types"]:
            task_type["primary_element"] = _ELEMENT_D2

    mutate_yaml(base, _TOPIC_D1, retarget_primary)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["elements"][0]))
    problems = check(result, base=base)
    assert _history(problems) == set()


@pytest.mark.parametrize("field", ["from", "to", "subvariants_to"])
def test_transfer_map_missing_absent_when_id_is_in_map(
    catalog_root: Path, field: str
) -> None:
    """CS-R25: retired-id есть в from, to или subvariants_to карты результата."""
    base, result = pair(catalog_root)
    entry = {"from": [_TYPE_D1_T02], "to": [_TYPE_D1_T02], "reason": "деление"}
    if field == "subvariants_to":
        entry["subvariants_to"] = _TYPE_D1_T01
    else:
        entry[field] = [_TYPE_D1_T01]
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["types"][0]))
    _write_map(result, [entry])
    problems = check(result, base=base)
    assert _history(problems) == set()


def test_transfer_map_missing_absent_when_element_id_is_in_map(
    catalog_root: Path,
) -> None:
    """CS-R25: retired-элемент записан в from карты результата."""
    base, result = pair(catalog_root)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["elements"][0]))
    _write_map(
        result,
        [{"from": [_ELEMENT_D1], "to": [_ELEMENT_D2], "reason": "слияние"}],
    )
    problems = check(result, base=base)
    assert "transfer_map_missing" not in _history(problems)


def test_transfer_map_missing_when_map_omits_retired_id(catalog_root: Path) -> None:
    """CS-R25: файл карты есть, но retired-id не входит в from, to и subvariants_to."""
    base, result = pair(catalog_root)
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["types"][0]))
    _write_map(
        result,
        [{"from": [_TYPE_D1_T02], "to": [_TYPE_D2_T01], "reason": "другое"}],
    )
    problems = check(result, base=base)
    assert (_TOPIC_D1, "/types/0/id") in _locations(problems, "transfer_map_missing")


def test_transfer_map_missing_when_only_base_has_map(catalog_root: Path) -> None:
    """CS-R25: карта базы не заменяет карту результата."""
    base, result = pair(catalog_root)
    _write_map(
        base,
        [{"from": [_TYPE_D1_T01], "to": [_TYPE_D1_T02], "reason": "деление"}],
    )
    mutate_yaml(result, _TOPIC_D1, lambda document: _retire(document["types"][0]))
    problems = check(result, base=base)
    assert (_TOPIC_D1, "/types/0/id") in _locations(problems, "transfer_map_missing")


def test_transfer_map_missing_joins_citation_missing(catalog_root: Path) -> None:
    """CS-R25: transfer_map_missing добавляется к другому коду того же файла."""
    base, result = pair(catalog_root)

    def retire_without_citations(document: dict) -> None:
        task_type = document["types"][0]
        _retire(task_type)
        task_type["citations"] = []

    mutate_yaml(result, _TOPIC_D1, retire_without_citations)
    problems = check(result, base=base)
    assert _locations(problems, "transfer_map_missing") == {(_TOPIC_D1, "/types/0/id")}
    citation_files = {
        problem.file for problem in problems if problem.code == "citation_missing"
    }
    assert citation_files == {_TOPIC_D1}


def test_cs_31_same_subvariant_ids_have_no_history_codes(catalog_root: Path) -> None:
    """CS-R25, cs-31: деление записано, subvariants_to указывает новый тип.

    Id подварианта и вариантов в результате те же, что в базе. Проверка сама
    id не меняет: тест сравнивает id в двух копиях.
    """
    base, result = pair(catalog_root)
    holder: dict[str, dict] = {}

    def split(document: dict) -> None:
        task_type = next(item for item in document["types"] if item["id"] == _TYPE_D1_T01)
        holder["subvariant"] = task_type["subvariants"].pop(0)
        _retire(task_type)
        task_type["subvariants"] = [
            {"id": f"{PACKAGE}:D1.T01.S2", "given": "два числа"},
        ]
        document["types"].append(
            _exam_type(_TYPE_D1_T03, "Сложение: первая часть", [holder["subvariant"]])
        )
        document["types"].append(
            _exam_type(
                _TYPE_D1_T04,
                "Сложение: вторая часть",
                [{"id": f"{PACKAGE}:D1.T04.S1", "given": "два числа"}],
            )
        )

    mutate_yaml(result, _TOPIC_D1, split)
    mutate_yaml(
        result,
        _VARIANTS_D1_T01,
        lambda document: document.__setitem__("type", _TYPE_D1_T03),
    )
    _write_map(
        result,
        [
            {
                "from": [_TYPE_D1_T01],
                "to": [_TYPE_D1_T03, _TYPE_D1_T04],
                "reason": "деление",
                "subvariants_to": _TYPE_D1_T03,
            }
        ],
    )
    base_subs = _subvariants_by_type(base)
    result_subs = _subvariants_by_type(result)
    assert _SUB_D1_T01 in base_subs[_TYPE_D1_T01]
    assert _SUB_D1_T01 in result_subs[_TYPE_D1_T03]
    assert _variant_ids(base) == _variant_ids(result)

    problems = check(result, base=base)
    assert _history(problems) == set()
    assert _subvariants_by_type(result)[_TYPE_D1_T03] == {_SUB_D1_T01}
    assert _variant_ids(base) == _variant_ids(result)


def test_id_removed_not_reported_for_identical_catalogs(catalog_root: Path) -> None:
    """CS-R4: одинаковые база и результат не дают кодов истории."""
    base, result = pair(catalog_root)
    problems = check(result, base=base)
    assert _history(problems) == set()


def test_id_removed_not_returned_by_two_argument_call(catalog_root: Path) -> None:
    """CS-R4: прежний вызов из двух аргументов не возвращает коды истории."""
    _base, result = pair(catalog_root)
    (result / _BLOCK_D2_B02).unlink()
    problems = check(result, "pr")
    assert _history(problems) == set()


def test_id_removed_not_returned_when_base_is_none(catalog_root: Path) -> None:
    """CS-R4: base is None — коды id_removed, id_reused и transfer_map_missing не возвращаются."""
    _base, result = pair(catalog_root)
    (result / _BLOCK_D2_B02).unlink()
    problems = check(result, base=None)
    assert _history(problems) == set()


def test_id_removed_cli_omits_code_without_base(catalog_root: Path) -> None:
    """CS-R4: без --base CLI не сообщает коды истории."""
    _base, result = pair(catalog_root)
    (result / _BLOCK_D2_B02).unlink()
    before = _files(result)
    completed = run_cli("check", str(result))
    assert _files(result) == before
    assert completed.returncode == 0
    for code in _HISTORY:
        assert code not in completed.stdout
        assert code not in completed.stderr


def test_id_removed_cli_exits_2_when_base_directory_is_missing(catalog_root: Path) -> None:
    """CS-R4: нет каталога базы — код выхода 2, флаг --base принят."""
    missing = catalog_root.parent / "missing-base"
    completed = run_cli("check", str(catalog_root), "--base", str(missing))
    assert completed.returncode == 2
    assert "unrecognized arguments" not in completed.stderr


def test_id_removed_cli_reports_code_when_base_is_passed(catalog_root: Path) -> None:
    """CS-R4, cs-20: --base сравнивает каталоги и --json показывает id_removed."""
    base, result = pair(catalog_root)
    (result / _BLOCK_D2_B02).unlink()
    before_result = _files(result)
    before_base = _files(base)
    completed = run_cli(
        "check",
        str(result),
        "--mode",
        "pr",
        "--base",
        str(base),
        "--json",
    )
    assert _files(result) == before_result
    assert _files(base) == before_base
    assert completed.returncode == 1
    payload = json.loads(completed.stdout)
    assert any(item["code"] == "id_removed" for item in payload)
