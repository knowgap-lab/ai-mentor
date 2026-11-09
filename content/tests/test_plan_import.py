"""Тесты импорта плана, LP-R1…LP-R20.

Импорт ``plan_import`` стоит внутри вызова, чтобы ``pytest --collect-only``
проходил без реализации. Ученик в вызовах только ``student-demo``.

Случаи ниже специально не зафиксированы: в спецификации и брифе нет одного
ответа, выбирать его нельзя.

ВОПРОС: пишет ли ``dry_run=True`` скелет в ``write_dir``; варианты: пишет, потому
что это не блокирующая ошибка; не пишет, потому что сухой прогон только строит
отчёт расхождений.
ВОПРОС: какие ``file`` и ``line`` у ``already_imported`` и ``import_failed``;
варианты: пустой файл и строка 1; путь ``write_dir`` или источника; в брифе не
заданы.
ВОПРОС: два файла с одним номером урока; варианты: ``lesson_gap``;
``frontmatter_invalid``; поведение не задано.
ВОПРОС: пустой список ``topics``; варианты: ``frontmatter_invalid``; урок
собирается с пустым списком, хотя в контракте ``minItems: 1``.
ВОПРОС: номер урока меньше 1; варианты: ``frontmatter_invalid``; ``lesson_gap``;
отказ без кода спецификации.
ВОПРОС: ``subject`` во frontmatter урока не совпадает с аргументом ``subject``;
варианты: ошибка; в урок пишется значение из frontmatter как есть.
ВОПРОС: в источнике нет каталога предмета; варианты: ``ValueError``;
блокирующая ошибка спецификации; пустой успешный результат.
ВОПРОС: номер строки, если обязательного поля или строки ``# `` нет; варианты:
строка 1; начало frontmatter; конец frontmatter.
ВОПРОС: ``week`` не является датой; варианты: только ``frontmatter_invalid``;
только ``week_not_monday``; оба кода.
ВОПРОС: чужие права HTTP, CSRF, Origin и заголовок Idempotency-Key; варианты:
это граница core, у ``import_plan`` нет актёра и токена; пакет сам отклоняет
чужой вызов. Тесты ниже проверяют только границу пакета: чужой ученик и чужой
предмет не попадают в результат, повтор тех же байтов даёт тот же хеш, ключ
ответа из текста урока не попадает в результат.
"""

from __future__ import annotations

import ast
import copy
import dataclasses
import datetime as dt
import hashlib
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest
import yaml


STUDENT = "student-demo"
TODAY = dt.date(2026, 11, 9)
FIXTURES = Path(__file__).resolve().parent / "fixtures" / "plan-import"
CONTENT = Path(__file__).resolve().parents[1]
EXAMPLES = Path(__file__).resolve().parents[2] / "docs" / "catalog" / "plan-import.examples.yaml"
PACKAGE = {"math": "ct-math", "physics": "ct-phys"}
KINDS = ("diagnostic", "new", "consolidation", "review", "mock", "analysis")
LESSON_KEYS = {
    "subject",
    "lesson",
    "planned_week",
    "phase",
    "kind",
    "topics",
    "review",
    "title",
    "source_path",
    "source_hash",
}
TOPIC_KEYS = {
    "id",
    "title",
    "section",
    "foundation_level",
    "topic_hours",
    "ct_positions",
    "prerequisite_elements",
    "citations",
    "elements",
}
REPORT_KEYS = {"created_topics", "created_lessons", "held_lessons", "differences"}
RESULT_FIELDS = {"request", "skeleton", "errors", "warnings", "report"}
BLOCKING = {
    "frontmatter_invalid",
    "kind_unknown",
    "topic_unknown",
    "review_before_intro",
    "lesson_gap",
    "week_not_monday",
    "week_not_increasing",
    "already_imported",
    "import_failed",
}
WARNINGS = {"generated_mismatch"}
SECRET = "SECRET-KEY-BEFORE-ATTEMPT"
FOREIGN_STUDENT = "student-other-9f3c"
FOREIGN_KEY = "FOREIGN-ANSWER-KEY"


def as_date(value: object) -> dt.date:
    """Приводит значение frontmatter к дате."""
    if isinstance(value, dt.datetime):
        return value.date()
    if isinstance(value, dt.date):
        return value
    return dt.date.fromisoformat(str(value))


def split_frontmatter(text: str) -> tuple[dict, str]:
    """Отделяет YAML между первыми двумя маркерами ``---``."""
    if not text.startswith("---\n"):
        raise AssertionError("нет frontmatter")
    _marker, rest = text.split("---\n", 1)
    raw, body = rest.split("\n---\n", 1)
    loaded = yaml.safe_load(raw)
    assert isinstance(loaded, dict)
    return loaded, body


def markdown_title(text: str) -> str:
    """Берёт первую строку ``# `` и срезает решётку и крайние пробелы."""
    for line in text.splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    raise AssertionError("нет заголовка")


def file_hash(path: Path) -> str:
    """Считает SHA-256 сырых байтов, как в брифе."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree_bytes(root: Path) -> dict[str, bytes]:
    """Читает все файлы дерева; каталоги без файлов не учитываются."""
    return {
        path.relative_to(root).as_posix(): path.read_bytes()
        for path in root.rglob("*")
        if path.is_file()
    }


def line_containing(path: Path, needle: str, occurrence: int = 1) -> int:
    """Возвращает номер строки с нужным вхождением, считая с 1."""
    seen = 0
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if needle in line:
            seen += 1
            if seen == occurrence:
                return number
    raise AssertionError(needle)


def write_files(root: Path, files: dict[str, str]) -> Path:
    """Записывает синтетическое дерево точно теми байтами, что в словаре."""
    for relative, text in files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(text.encode("utf-8"))
    return root


def card(
    topic_id: str,
    title: str,
    *,
    section: str = "Алгебра",
    level: str = "ядро",
    hours_lesson: int = 2,
    hours_home: int = 3,
    prereqs: list[str] | None = None,
    positions: list[str] | None = None,
    tier: str | None = None,
    explicit_empty: bool = False,
) -> str:
    """Собирает карточку темы. ``None`` значит «ключа в файле нет»."""
    lines = [
        "---",
        f"id: {topic_id}",
        f"title: {title}",
        f"section: {section}",
        f"level: {level}",
    ]
    if tier is not None:
        lines.append(f"tier: {tier}")
    if prereqs is not None:
        if prereqs:
            lines.append("prereqs:")
            lines.extend(f"  - {item}" for item in prereqs)
        elif explicit_empty:
            lines.append("prereqs: []")
    if positions is not None:
        if positions:
            lines.append("ct_positions:")
            lines.extend(f"  - {item}" for item in positions)
        elif explicit_empty:
            lines.append("ct_positions: []")
    lines.extend(
        [
            f"hours_lesson: {hours_lesson}",
            f"hours_home: {hours_home}",
            "---",
            "",
            f"Карточка {topic_id}.",
            "",
        ]
    )
    return "\n".join(lines)


def lesson(
    subject: str,
    number: int,
    week: str,
    topics: list[str],
    review: list[str] | None = None,
    *,
    kind: str = "new",
    phase: str = "основной",
    heading: str | None = None,
    body: str = "",
) -> str:
    """Собирает файл урока. Заголовок по умолчанию ``# Урок N``."""
    review = [] if review is None else review
    lines = [
        "---",
        f"subject: {subject}",
        f"lesson: {number}",
        f"week: {week}",
        f"phase: {phase}",
        f"kind: {kind}",
        "topics:",
    ]
    lines.extend(f"  - {item}" for item in topics)
    if review:
        lines.append("review:")
        lines.extend(f"  - {item}" for item in review)
    else:
        lines.append("review: []")
    lines.append("---")
    lines.append(heading if heading is not None else f"# Урок {number}")
    if body:
        lines.append(body.rstrip("\n"))
    lines.append("")
    return "\n".join(lines)


def copy_example(tmp_path: Path, example_id: str, dest: str | None = None) -> Path:
    """Копирует пример в каталог теста, исходную фикстуру не меняет."""
    target = tmp_path / (dest or example_id)
    shutil.copytree(FIXTURES / example_id, target)
    return target


def assert_fixture_ready(source: Path, subject: str) -> None:
    """Проверяет, что синтетический успех сам не нарушает LP-R7, LP-R9 и LP-R10."""
    cards: dict[str, dict] = {}
    for path in sorted((source / subject / "topics").glob("*.md")):
        meta, _body = split_frontmatter(path.read_text(encoding="utf-8"))
        for key in ("id", "title", "section", "level", "hours_lesson", "hours_home"):
            assert key in meta, path
        assert meta["hours_lesson"] + meta["hours_home"] > 0
        cards[str(meta["id"])] = meta
    assert cards
    for meta in cards.values():
        for code in meta.get("prereqs") or []:
            assert code in cards
    lessons: dict[int, tuple[Path, dict]] = {}
    for path in sorted((source / subject / "plan").glob("*.md")):
        meta, _body = split_frontmatter(path.read_text(encoding="utf-8"))
        number = int(meta["lesson"])
        assert number not in lessons
        lessons[number] = (path, meta)
    numbers = sorted(lessons)
    assert numbers == list(range(1, len(numbers) + 1))
    introduced: set[str] = set()
    previous: dt.date | None = None
    for number in numbers:
        path, meta = lessons[number]
        assert meta["kind"] in KINDS
        assert meta["subject"] == subject
        assert meta["topics"]
        week = as_date(meta["week"])
        assert week.weekday() == 0, path
        if previous is not None:
            assert previous < week
        previous = week
        for code in meta.get("review") or []:
            assert code in cards
            assert code in introduced
        for code in meta["topics"]:
            assert code in cards
            introduced.add(str(code))
        assert any(line.startswith("# ") for line in path.read_text(encoding="utf-8").splitlines())


def assert_problem(item: dict, *, allowed: set[str]) -> None:
    """Проверяет форму ошибки или предупреждения из брифа."""
    assert set(item) == {"code", "file", "line", "message"}
    assert item["code"] in allowed
    assert isinstance(item["file"], str)
    assert isinstance(item["line"], int) and item["line"] >= 1
    assert isinstance(item["message"], str) and re.search(r"[А-Яа-яЁё]", item["message"])


def assert_sorted(items: tuple) -> None:
    """Порядок брифа: файл, код, строка."""
    assert list(items) == sorted(items, key=lambda item: (item["file"], item["code"], item["line"]))


def assert_plan(
    result: object,
    *,
    source: Path,
    subject: str,
    shift_weeks: int = 0,
    today: dt.date = TODAY,
    clean: bool = True,
    differences: set[str] | None = None,
) -> None:
    """Сверяет успешный результат с карточками, уроками и контрактом."""
    assert result.errors == ()  # type: ignore[attr-defined]
    assert result.request is not None  # type: ignore[attr-defined]
    assert result.skeleton is not None  # type: ignore[attr-defined]
    request = result.request  # type: ignore[attr-defined]
    skeleton = result.skeleton  # type: ignore[attr-defined]
    report = result.report  # type: ignore[attr-defined]
    warnings = result.warnings  # type: ignore[attr-defined]
    assert isinstance(warnings, tuple)
    assert_sorted(result.errors)  # type: ignore[attr-defined]
    assert_sorted(warnings)
    for item in warnings:
        assert_problem(item, allowed=WARNINGS)
    if clean:
        assert warnings == ()
    else:
        assert warnings
    assert set(request) <= {"student_id", "subject", "shift_weeks", "lessons", "snapshot"}
    assert {"student_id", "subject", "lessons", "snapshot"} <= set(request)
    assert request["student_id"] == STUDENT
    assert request["subject"] == subject
    if "shift_weeks" in request:
        assert request["shift_weeks"] == shift_weeks
    assert set(request["snapshot"]) == {"source_hashes"}
    package = PACKAGE[subject]
    card_meta: dict[str, tuple[Path, dict]] = {}
    for path in sorted((source / subject / "topics").glob("*.md")):
        meta, _body = split_frontmatter(path.read_text(encoding="utf-8"))
        card_meta[str(meta["id"])] = (path, meta)
    assert set(skeleton) == {f"{package}/topics/{code}.yaml" for code in card_meta}
    for code, (path, meta) in card_meta.items():
        topic = skeleton[f"{package}/topics/{code}.yaml"]
        assert set(topic) == TOPIC_KEYS
        assert topic["id"] == f"{package}:{code}"
        assert topic["title"] == meta["title"]
        assert topic["section"] == meta["section"]
        assert topic["foundation_level"] == meta["level"]
        assert topic["topic_hours"] == meta["hours_lesson"] + meta["hours_home"]
        assert isinstance(topic["topic_hours"], (int, float)) and not isinstance(topic["topic_hours"], bool)
        assert topic["topic_hours"] > 0
        assert topic["ct_positions"] == list(meta.get("ct_positions") or [])
        assert topic["prerequisite_elements"] == [
            f"{package}:{item}.E1" for item in (meta.get("prereqs") or [])
        ]
        relative = path.relative_to(source).as_posix()
        citation = {"doc": "topic-card", "ref": relative}
        assert topic["citations"] == [citation]
        assert topic["elements"] == [
            {"id": f"{package}:{code}.E1", "title": "тема целиком", "citations": [citation]}
        ]
    lesson_files: dict[int, Path] = {}
    for path in (source / subject / "plan").glob("*.md"):
        meta, _body = split_frontmatter(path.read_text(encoding="utf-8"))
        lesson_files[int(meta["lesson"])] = path
    lessons = request["lessons"]
    numbers = list(range(1, len(lesson_files) + 1))
    assert [item["lesson"] for item in lessons] == numbers
    expected_hashes: dict[str, str] = {}
    for number in numbers:
        path = lesson_files[number]
        text = path.read_text(encoding="utf-8")
        meta, _body = split_frontmatter(text)
        item = lessons[number - 1]
        relative = path.relative_to(source).as_posix()
        expected_hashes[relative] = file_hash(path)
        assert set(item) == LESSON_KEYS
        assert item["subject"] == meta["subject"]
        assert item["lesson"] == number
        assert isinstance(item["lesson"], int) and not isinstance(item["lesson"], bool)
        planned = as_date(meta["week"]) + dt.timedelta(days=shift_weeks * 7)
        assert item["planned_week"] == planned.isoformat()
        assert item["phase"] == meta["phase"]
        assert item["kind"] == meta["kind"]
        assert item["topics"] == [f"{package}:{code}" for code in meta["topics"]]
        assert item["review"] == [f"{package}:{code}" for code in (meta.get("review") or [])]
        assert item["title"] == markdown_title(text)
        assert item["source_path"] == relative
        assert item["source_hash"] == expected_hashes[relative]
        assert re.fullmatch(r"[0-9a-f]{64}", item["source_hash"])
    for path in (source / subject / "topics").glob("*.md"):
        relative = path.relative_to(source).as_posix()
        expected_hashes[relative] = file_hash(path)
    assert request["snapshot"]["source_hashes"] == expected_hashes
    assert set(report) == REPORT_KEYS
    for key in REPORT_KEYS:
        assert isinstance(report[key], list)
    assert len(report["created_topics"]) == len(skeleton)
    assert len(report["created_lessons"]) == len(lessons)
    monday = today - dt.timedelta(days=today.weekday())
    expected_held = {
        item["lesson"]: {
            "lesson": item["lesson"],
            "state": "held",
            "snapshot": None,
            "marks": None,
        }
        for item in lessons
        if dt.date.fromisoformat(item["planned_week"]) < monday
    }
    got_held = {item["lesson"]: item for item in report["held_lessons"]}
    assert len(got_held) == len(report["held_lessons"])
    assert got_held == expected_held
    assert all(isinstance(item, str) for item in report["differences"])
    if differences is None:
        assert report["differences"] == []
    else:
        assert set(report["differences"]) == set(differences)
        assert len(report["differences"]) == len(set(report["differences"]))


def run(source: Path, subject: str, **kwargs: object) -> object:
    """Вызывает импорт. Импорт пакета не выполняется при сборе тестов."""
    from plan_import import ImportResult, import_plan

    today = kwargs.pop("today", TODAY)
    result = import_plan(source=source, student_id=STUDENT, subject=subject, today=today, **kwargs)
    assert isinstance(result, ImportResult)
    assert dataclasses.is_dataclass(result) and result.__dataclass_params__.frozen is True
    assert {field.name for field in dataclasses.fields(result)} == RESULT_FIELDS
    return result


def run_preserving(source: Path, subject: str, **kwargs: object) -> object:
    """Импорт не имеет права менять прочитанный источник."""
    before = tree_bytes(source)
    result = run(source, subject, **kwargs)
    assert tree_bytes(source) == before
    return result


def import_ok(
    source: Path,
    subject: str,
    *,
    shift_weeks: int | None = None,
    today: dt.date = TODAY,
    clean: bool = True,
    differences: set[str] | None = None,
    **kwargs: object,
) -> object:
    """Ждёт успешный разбор и сверяет его с оракулом."""
    assert_fixture_ready(source, subject)
    if shift_weeks is not None:
        kwargs["shift_weeks"] = shift_weeks
    result = run_preserving(source, subject, today=today, **kwargs)
    assert_plan(
        result,
        source=source,
        subject=subject,
        shift_weeks=0 if shift_weeks is None else shift_weeks,
        today=today,
        clean=clean,
        differences=differences,
    )
    return result


def assert_blocked(
    result: object,
    code: str,
    *,
    file: str | None = None,
    line: int | None = None,
    only: bool = True,
    clear_report: bool = True,
) -> None:
    """Блокирующая ошибка обнуляет набор и скелет и не пишет чужой код."""
    assert result.request is None  # type: ignore[attr-defined]
    assert result.skeleton is None  # type: ignore[attr-defined]
    assert isinstance(result.errors, tuple)  # type: ignore[attr-defined]
    assert isinstance(result.warnings, tuple)  # type: ignore[attr-defined]
    assert result.warnings == ()  # type: ignore[attr-defined]
    assert_sorted(result.errors)  # type: ignore[attr-defined]
    report = result.report  # type: ignore[attr-defined]
    assert set(report) == REPORT_KEYS
    assert report["created_topics"] == []
    assert report["created_lessons"] == []
    if clear_report:
        assert report["held_lessons"] == []
        assert report["differences"] == []
    codes = [item["code"] for item in result.errors]  # type: ignore[attr-defined]
    if only:
        assert codes == [code]
    else:
        assert code in codes
    for item in result.errors:  # type: ignore[attr-defined]
        assert_problem(item, allowed=BLOCKING)
    matched = [item for item in result.errors if item["code"] == code]  # type: ignore[attr-defined]
    if file is not None:
        assert matched[0]["file"] == file
    if line is not None:
        assert matched[0]["line"] == line


def payload(result: object) -> str:
    """Сериализует результат: в этой строке не должно быть чужих секретов."""
    return json.dumps(
        {
            "request": result.request,  # type: ignore[attr-defined]
            "skeleton": result.skeleton,  # type: ignore[attr-defined]
            "errors": result.errors,  # type: ignore[attr-defined]
            "warnings": result.warnings,  # type: ignore[attr-defined]
            "report": result.report,  # type: ignore[attr-defined]
        },
        ensure_ascii=False,
    )


def run_cli(*arguments: str) -> subprocess.CompletedProcess[str]:
    """Запускает модульный CLI из каталога content."""
    return subprocess.run(
        [sys.executable, "-m", "plan_import", *arguments],
        cwd=CONTENT,
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )


def test_examples_are_bound_to_named_rule_tests() -> None:
    """Каждый id таблицы примеров и каждый номер LP-R1…LP-R20 есть в тестах."""
    rows = yaml.safe_load(EXAMPLES.read_text(encoding="utf-8"))
    source = Path(__file__).read_text(encoding="utf-8")
    tree = ast.parse(source)
    names: list[str] = []
    docs: list[str] = []
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name.startswith("test_"):
            names.append(node.name)
            docs.append(ast.get_docstring(node) or "")
    joined = "\n".join(docs)
    assert [row["id"] for row in rows] == [f"plan-import-{number:02d}" for number in range(1, 15)]
    for row in rows:
        assert row["id"] in joined
        assert row["rule"] in joined
    for number in range(1, 21):
        prefix = f"test_lp_r{number}_"
        assert any(name.startswith(prefix) for name in names)


def test_lp_r1_plan_import_01_math_topic_and_element(tmp_path: Path) -> None:
    """LP-R1, plan-import-01: карточка M14 и предмет math дают тему и элемент E1."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    topic = result.skeleton["ct-math/topics/M14.yaml"]
    assert topic["id"] == "ct-math:M14"
    assert [element["id"] for element in topic["elements"]] == ["ct-math:M14.E1"]
    assert topic["elements"][0]["title"] == "тема целиком"
    assert topic["title"] == "Дроби"


def test_lp_r1_plan_import_02_physics_topic_and_element(tmp_path: Path) -> None:
    """LP-R1, plan-import-02: карточка P05 и предмет physics дают ct-phys:P05.E1."""
    source = copy_example(tmp_path, "plan-import-02")
    result = import_ok(source, "physics")
    topic = result.skeleton["ct-phys/topics/P05.yaml"]
    assert topic["id"] == "ct-phys:P05"
    assert topic["elements"][0]["id"] == "ct-phys:P05.E1"
    assert topic["elements"][0]["title"] == "тема целиком"
    assert topic["foundation_level"] == "фундамент"


def test_lp_r2_code_is_not_rewritten(tmp_path: Path) -> None:
    """LP-R2: таблица исключений пуста, код карточки входит в id как есть."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/AB12.md": card("AB12", "Код без таблицы"),
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["AB12", "M14"]),
        },
    )
    result = import_ok(source, "math")
    assert result.skeleton["ct-math/topics/AB12.yaml"]["id"] == "ct-math:AB12"
    assert result.skeleton["ct-math/topics/AB12.yaml"]["elements"][0]["id"] == "ct-math:AB12.E1"
    assert result.skeleton["ct-math/topics/M14.yaml"]["id"] == "ct-math:M14"
    assert result.request["lessons"][0]["topics"] == ["ct-math:AB12", "ct-math:M14"]


def test_lp_r3_plan_import_03_prereq_on_topic_until_types_exist(tmp_path: Path) -> None:
    """LP-R3, plan-import-03: prereqs M13 лежит на теме как ct-math:M13.E1, типов нет."""
    source = copy_example(tmp_path, "plan-import-03")
    result = import_ok(source, "math")
    topic = result.skeleton["ct-math/topics/M14.yaml"]
    assert topic["prerequisite_elements"] == ["ct-math:M13.E1"]
    assert "types" not in topic
    assert "prerequisite_elements" not in topic["elements"][0]
    assert result.skeleton["ct-math/topics/M13.yaml"]["elements"][0]["id"] == "ct-math:M13.E1"


def test_lp_r3_prereq_order_and_omitted_list(tmp_path: Path) -> None:
    """LP-R3: нет ключа prereqs или он пуст — пустой список; порядок кодов сохраняется."""
    omitted = copy_example(tmp_path, "plan-import-01", "omitted")
    omitted_result = import_ok(omitted, "math")
    assert omitted_result.skeleton["ct-math/topics/M14.yaml"]["prerequisite_elements"] == []
    explicit = write_files(
        tmp_path / "explicit",
        {
            "math/topics/M04.md": card("M04", "Тема M04"),
            "math/topics/M13.md": card("M13", "Предпосылка"),
            "math/topics/M14.md": card("M14", "Дроби", prereqs=["M13", "M04"]),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"]),
        },
    )
    ordered = import_ok(explicit, "math")
    assert ordered.skeleton["ct-math/topics/M14.yaml"]["prerequisite_elements"] == [
        "ct-math:M13.E1",
        "ct-math:M04.E1",
    ]
    empty = write_files(
        tmp_path / "empty",
        {
            "math/topics/M14.md": card("M14", "Дроби", prereqs=[], explicit_empty=True),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"]),
        },
    )
    assert import_ok(empty, "math").skeleton["ct-math/topics/M14.yaml"]["prerequisite_elements"] == []


def test_lp_r4_plan_import_04_tier_dropped_and_fields_mapped(tmp_path: Path) -> None:
    """LP-R4, plan-import-04: tier не попадает в каталог, остальные поля переносятся."""
    source = copy_example(tmp_path, "plan-import-04")
    result = import_ok(source, "math")
    topic = result.skeleton["ct-math/topics/M14.yaml"]
    assert topic["title"] == "Дроби"
    assert topic["section"] == "Алгебра"
    assert topic["foundation_level"] == "ядро"
    assert topic["topic_hours"] == 5
    assert topic["ct_positions"] == ["A1", "B2"]
    assert "tier" not in topic
    assert "level" not in topic
    assert "hours_lesson" not in topic
    assert "hours_home" not in topic
    written = tmp_path / "catalog"
    written.mkdir()
    import_ok(source, "math", write_dir=written)
    text = (written / "ct-math/topics/M14.yaml").read_text(encoding="utf-8")
    assert "tier" not in text


def test_lp_r4_omitted_ct_positions_are_empty(tmp_path: Path) -> None:
    """LP-R4: нет ct_positions — пустой список, это не появления."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    topic = result.skeleton["ct-math/topics/M14.yaml"]
    assert topic["ct_positions"] == []
    assert "positions" not in topic
    assert "appearances" not in topic


def test_lp_r5_citation_on_topic_and_element(tmp_path: Path) -> None:
    """LP-R5: у темы и элемента одна цитата topic-card с путём карточки."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    topic = result.skeleton["ct-math/topics/M14.yaml"]
    citation = {"doc": "topic-card", "ref": "math/topics/M14.md"}
    assert topic["citations"] == [citation]
    assert topic["elements"][0]["citations"] == [citation]


def test_lp_r6_plan_import_05_lesson_seven(tmp_path: Path) -> None:
    """LP-R6, plan-import-05: урок 7 сохраняет поля, planned_week равен 2026-11-23."""
    source = copy_example(tmp_path, "plan-import-05")
    result = import_ok(source, "math", shift_weeks=0)
    item = result.request["lessons"][6]
    assert item["lesson"] == 7
    assert item["subject"] == "math"
    assert item["planned_week"] == "2026-11-23"
    assert item["phase"] == "ядро"
    assert item["kind"] == "new"
    assert item["topics"] == ["ct-math:M10"]
    assert item["review"] == ["ct-math:M04", "ct-math:M08", "ct-math:M06", "ct-math:M09"]
    assert item["title"] == "Показательные уравнения"
    assert item["source_path"] == "math/plan/L07.md"
    assert "M10" not in item["topics"]


def test_lp_r6_title_strips_hash_and_keeps_first_heading(tmp_path: Path) -> None:
    """LP-R6: название — первая строка ``# `` без решётки и крайних пробелов."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    assert result.request["lessons"][0]["title"] == "Урок про дроби"


def test_lp_r6_lessons_sorted_by_number_not_filename(tmp_path: Path) -> None:
    """LP-R6: уроки в запросе идут по номеру, а не по имени файла."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/b-second.md": lesson("math", 1, "2026-11-09", ["M14"]),
            "math/plan/a-first.md": lesson("math", 2, "2026-11-16", ["M14"]),
        },
    )
    result = import_ok(source, "math")
    assert [item["source_path"] for item in result.request["lessons"]] == [
        "math/plan/b-second.md",
        "math/plan/a-first.md",
    ]


def test_lp_r7_plan_import_06_kind_unknown_blocks(tmp_path: Path) -> None:
    """LP-R7, plan-import-06: kind seminar — kind_unknown, импорт заблокирован."""
    source = copy_example(tmp_path, "plan-import-06")
    result = run_preserving(source, "math")
    lesson_path = source / "math/plan/L01.md"
    assert_blocked(
        result,
        "kind_unknown",
        file="math/plan/L01.md",
        line=line_containing(lesson_path, "kind: seminar"),
    )


@pytest.mark.parametrize("kind", KINDS)
def test_lp_r7_allowed_kind_imports(tmp_path: Path, kind: str) -> None:
    """LP-R7: каждый вид из списка импортируется как обычный урок."""
    source = write_files(
        tmp_path / kind,
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"], kind=kind),
        },
    )
    result = import_ok(source, "math")
    assert result.request["lessons"][0]["kind"] == kind


@pytest.mark.parametrize("kind", KINDS)
def test_lp_r8_answer_key_not_leaked_before_attempt(tmp_path: Path, kind: str) -> None:
    """LP-R8: ключ ответа из текста урока не попадает в результат до попытки."""
    source = write_files(
        tmp_path / kind,
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson(
                "math",
                1,
                "2026-11-16",
                ["M14"],
                kind=kind,
                body=f"Ключ ответа: {SECRET}",
            ),
        },
    )
    written = tmp_path / f"out-{kind}"
    written.mkdir()
    result = import_ok(source, "math", write_dir=written)
    assert SECRET in (source / "math/plan/L01.md").read_text(encoding="utf-8")
    assert SECRET not in payload(result)
    assert SECRET.encode("utf-8") not in b"".join(tree_bytes(written).values())
    assert set(result.request["lessons"][0]) == LESSON_KEYS


def test_lp_r9_plan_import_07_review_before_intro(tmp_path: Path) -> None:
    """LP-R9, plan-import-07: повтор в уроке 3 темы, введённой в уроке 5, блокируется."""
    source = copy_example(tmp_path, "plan-import-07")
    result = run_preserving(source, "math")
    lesson_path = source / "math/plan/L03.md"
    assert_blocked(
        result,
        "review_before_intro",
        file="math/plan/L03.md",
        line=line_containing(lesson_path, "- M13"),
    )


def test_lp_r9_review_of_same_lesson_is_too_early(tmp_path: Path) -> None:
    """LP-R9: повтор темы того же урока не считается введённым раньше."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"], ["M14"]),
        },
    )
    result = run_preserving(source, "math")
    lesson_path = source / "math/plan/L01.md"
    assert_blocked(
        result,
        "review_before_intro",
        file="math/plan/L01.md",
        line=line_containing(lesson_path, "- M14", occurrence=2),
    )


def test_lp_r9_known_topic_not_yet_introduced(tmp_path: Path) -> None:
    """LP-R9: существующая карточка, которой не было в прошлых уроках, — не topic_unknown."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M13.md": card("M13", "Предпосылка"),
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"], ["M13"]),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "review_before_intro", file="math/plan/L01.md")


def test_lp_r9_unknown_topic_in_lesson(tmp_path: Path) -> None:
    """LP-R9: неизвестный код в topics — topic_unknown и блокировка."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/plan/L01.md"
    path.write_text(path.read_text(encoding="utf-8").replace("  - M14", "  - M99"), encoding="utf-8")
    result = run_preserving(source, "math")
    assert_blocked(
        result,
        "topic_unknown",
        file="math/plan/L01.md",
        line=line_containing(path, "M99"),
    )


def test_lp_r10_plan_import_08_lesson_gap(tmp_path: Path) -> None:
    """LP-R10, plan-import-08: уроки 1, 2, 4 — lesson_gap, в сообщении пропущенный номер."""
    source = copy_example(tmp_path, "plan-import-08")
    result = run_preserving(source, "math")
    assert_blocked(result, "lesson_gap", file="", line=1)
    assert re.search(r"(?<!\d)3(?!\d)", result.errors[0]["message"])


def test_lp_r10_plan_import_09_week_not_monday(tmp_path: Path) -> None:
    """LP-R10, plan-import-09: week 2026-11-24 — вторник, ошибка week_not_monday."""
    source = copy_example(tmp_path, "plan-import-09")
    result = run_preserving(source, "math")
    lesson_path = source / "math/plan/L01.md"
    assert_blocked(
        result,
        "week_not_monday",
        file="math/plan/L01.md",
        line=line_containing(lesson_path, "week: 2026-11-24"),
    )


def test_lp_r10_week_not_increasing(tmp_path: Path) -> None:
    """LP-R10: по номерам уроков week строго возрастает, иначе week_not_increasing."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-16", ["M14"]),
            "math/plan/L02.md": lesson("math", 2, "2026-11-09", ["M14"]),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(
        result,
        "week_not_increasing",
        file="math/plan/L02.md",
        line=line_containing(source / "math/plan/L02.md", "week: 2026-11-09"),
    )


def test_lp_r10_equal_weeks_are_not_increasing(tmp_path: Path) -> None:
    """LP-R10: одинаковые понедельники не строго возрастают."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-09", ["M14"]),
            "math/plan/L02.md": lesson("math", 2, "2026-11-09", ["M14"]),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "week_not_increasing", file="math/plan/L02.md")


def test_lp_r10_numbers_start_at_one(tmp_path: Path) -> None:
    """LP-R10: нумерация начинается с 1, пропуск первого номера — lesson_gap."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L02.md": lesson("math", 2, "2026-11-09", ["M14"]),
            "math/plan/L03.md": lesson("math", 3, "2026-11-16", ["M14"]),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "lesson_gap", file="", line=1)
    assert re.search(r"(?<!\d)1(?!\d)", result.errors[0]["message"])


def test_lp_r10_order_follows_lesson_number_not_filename(tmp_path: Path) -> None:
    """LP-R10: нарушение роста week определяется по номеру урока, не по имени файла."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/later.md": lesson("math", 1, "2026-11-16", ["M14"]),
            "math/plan/earlier.md": lesson("math", 2, "2026-11-09", ["M14"]),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "week_not_increasing", file="math/plan/earlier.md")


def test_lp_r11_plan_import_10_shift_weeks(tmp_path: Path) -> None:
    """LP-R11, plan-import-10: shift_weeks 4 переносит 2026-10-12 на 2026-11-09, номер тот же."""
    source = copy_example(tmp_path, "plan-import-10")
    result = import_ok(source, "math", shift_weeks=4)
    item = result.request["lessons"][0]
    assert item["lesson"] == 1
    assert item["planned_week"] == "2026-11-09"
    assert item["source_hash"] == file_hash(source / "math/plan/L01.md")


def test_lp_r11_default_shift_is_zero(tmp_path: Path) -> None:
    """LP-R11: без shift_weeks planned_week совпадает с week."""
    source = copy_example(tmp_path, "plan-import-10")
    assert_fixture_ready(source, "math")
    result = run_preserving(source, "math")
    assert_plan(result, source=source, subject="math", shift_weeks=0)
    assert result.request["lessons"][0]["planned_week"] == "2026-10-12"
    assert result.request["lessons"][0]["lesson"] == 1


def test_lp_r12_lesson_has_only_contract_fields(tmp_path: Path) -> None:
    """LP-R12: в уроке нет даты занятия, времени, длительности, отметок и снимка урока."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    item = result.request["lessons"][0]
    assert set(item) == LESSON_KEYS
    assert {
        "date",
        "time",
        "duration",
        "starts_at",
        "ends_at",
        "marks",
        "snapshot",
        "state",
        "answers",
        "answer",
        "key",
    }.isdisjoint(item)


def test_lp_r13_plan_import_11_second_import_rejected(tmp_path: Path) -> None:
    """LP-R13, plan-import-11: второй запуск той же пары — только already_imported."""
    source = copy_example(tmp_path, "plan-import-11")
    first = import_ok(source, "math")
    assert first.errors == ()
    written = tmp_path / "out"
    written.mkdir()
    (written / "sentinel.txt").write_text("keep\n", encoding="utf-8")
    before = tree_bytes(written)
    second = run_preserving(source, "math", already_imported=True, dry_run=False, write_dir=written)
    assert_blocked(second, "already_imported")
    assert tree_bytes(written) == before


def test_lp_r13_plan_import_12_dry_run_differences(tmp_path: Path) -> None:
    """LP-R13, plan-import-12: сухой прогон при уже импортированной паре строит расхождения и не блокирует."""
    source = copy_example(tmp_path, "plan-import-12")
    unchanged = "math/topics/M14.md"
    changed = "math/plan/L01.md"
    missing = "math/topics/M13.md"
    snapshot = {
        "source_hashes": {
            unchanged: file_hash(source / unchanged),
            changed: "0" * 64,
            "math/topics/M99.md": "ab" * 32,
        }
    }
    result = import_ok(
        source,
        "math",
        already_imported=True,
        dry_run=True,
        previous_snapshot=snapshot,
        differences={changed, missing},
    )
    assert result.request is not None
    assert "already_imported" not in [item["code"] for item in result.errors]


def test_lp_r13_dry_run_without_snapshot_differences_empty(tmp_path: Path) -> None:
    """LP-R13: сухой прогон без снимка даёт пустой список расхождений и не отвергает пару."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math", already_imported=True, dry_run=True)
    assert result.report["differences"] == []
    assert result.request["student_id"] == STUDENT


def test_lp_r14_file_edits_do_not_change_imported_plan(tmp_path: Path) -> None:
    """LP-R14: правка файла после импорта не меняет уже возвращённый набор."""
    source = copy_example(tmp_path, "plan-import-11")
    first = import_ok(source, "math")
    frozen = copy.deepcopy(first.request)
    lesson_path = source / "math/plan/L01.md"
    lesson_path.write_bytes(lesson_path.read_bytes() + "Новая строка урока.\n".encode("utf-8"))
    second = run_preserving(source, "math", already_imported=True, dry_run=False)
    assert_blocked(second, "already_imported")
    assert first.request == frozen


def test_lp_r15_snapshot_excludes_generated_files(tmp_path: Path) -> None:
    """LP-R15: снимок — хеши карточек и уроков, без graph.json и PLAN.md."""
    source = copy_example(tmp_path, "plan-import-14")
    result = import_ok(source, "math", clean=False)
    hashes = result.request["snapshot"]["source_hashes"]
    assert set(hashes) == {"math/topics/M14.md", "math/plan/L01.md"}
    assert "graph/graph.json" not in hashes
    for relative, digest in hashes.items():
        assert digest == file_hash(source / relative)
        assert digest == digest.lower()


def test_lp_r15_repeat_request_same_hash_key(tmp_path: Path) -> None:
    """LP-R15: повтор запроса на тех же байтах возвращает тот же хеш и не меняет прошлый снимок."""
    source = copy_example(tmp_path, "plan-import-01")
    written = tmp_path / "catalog"
    written.mkdir()
    first = import_ok(source, "math", write_dir=written)
    frozen = copy.deepcopy(first.request["snapshot"])
    first_files = tree_bytes(written)
    second = import_ok(source, "math", write_dir=written)
    assert first.request["snapshot"] == frozen
    assert second.request["snapshot"]["source_hashes"] == frozen["source_hashes"]
    assert tree_bytes(written) == first_files
    assert set(first_files) == set(second.skeleton)
    lesson_path = source / "math/plan/L01.md"
    lesson_path.write_bytes(lesson_path.read_bytes() + b"\n")
    third = import_ok(source, "math")
    assert first.request["snapshot"] == frozen
    assert third.request["lessons"][0]["source_hash"] != frozen["source_hashes"]["math/plan/L01.md"]
    with pytest.raises(dataclasses.FrozenInstanceError):
        first.errors = ()


def test_lp_r16_blocking_error_does_not_write(tmp_path: Path) -> None:
    """LP-R16: блокирующая ошибка не оставляет частично записанный каталог."""
    source = copy_example(tmp_path, "plan-import-06")
    written = tmp_path / "catalog"
    written.mkdir()
    (written / "sentinel.txt").write_text("keep\n", encoding="utf-8")
    before = tree_bytes(written)
    result = run_preserving(source, "math", write_dir=written)
    assert_blocked(result, "kind_unknown")
    assert tree_bytes(written) == before
    assert "import_failed" not in [item["code"] for item in result.errors]


def test_lp_r16_import_failed_leaves_directory_unchanged(tmp_path: Path) -> None:
    """LP-R16: сбой записи после успешного разбора — import_failed, каталог как до вызова."""
    source = copy_example(tmp_path, "plan-import-01")
    written = tmp_path / "catalog"
    written.mkdir()
    (written / "ct-math").write_text("blocker\n", encoding="utf-8")
    (written / "sentinel.txt").write_text("keep\n", encoding="utf-8")
    before = tree_bytes(written)
    result = run_preserving(source, "math", write_dir=written)
    assert_blocked(result, "import_failed", clear_report=False)
    assert result.report["created_topics"] == []
    assert result.report["created_lessons"] == []
    assert tree_bytes(written) == before


def test_lp_r16_writes_all_skeleton_files(tmp_path: Path) -> None:
    """LP-R16: без блокировки в write_dir лежат все файлы скелета и нет package.yaml."""
    source = copy_example(tmp_path, "plan-import-03")
    written = tmp_path / "catalog"
    written.mkdir()
    result = import_ok(source, "math", write_dir=written)
    files = tree_bytes(written)
    assert set(files) == set(result.skeleton)
    for relative, topic in result.skeleton.items():
        assert yaml.safe_load(files[relative].decode("utf-8")) == topic
    assert not (written / "package.yaml").exists()


def test_lp_r17_plan_import_13_held_without_snapshot_or_marks(tmp_path: Path) -> None:
    """LP-R17, plan-import-13: урок 4 с неделей 2026-11-02 при сегодня 2026-11-09 — held без снимка и отметок."""
    source = copy_example(tmp_path, "plan-import-13")
    result = import_ok(source, "math", today=TODAY)
    held = {item["lesson"]: item for item in result.report["held_lessons"]}
    assert held[4] == {"lesson": 4, "state": "held", "snapshot": None, "marks": None}
    lesson = next(item for item in result.request["lessons"] if item["lesson"] == 4)
    assert "state" not in lesson
    assert "marks" not in lesson
    assert "snapshot" not in lesson
    assert result.request["snapshot"]["source_hashes"]["math/plan/L04.md"] == file_hash(
        source / "math/plan/L04.md"
    )


@pytest.mark.parametrize(
    ("week", "today", "held"),
    [
        ("2026-11-09", dt.date(2026, 11, 9), False),
        ("2026-11-09", dt.date(2026, 11, 11), False),
        ("2026-11-09", dt.date(2026, 11, 15), False),
        ("2026-11-09", dt.date(2026, 11, 16), True),
        ("2026-11-02", dt.date(2026, 11, 9), True),
        ("2026-11-16", dt.date(2026, 11, 9), False),
    ],
)
def test_lp_r17_held_before_monday_of_today(tmp_path: Path, week: str, today: dt.date, held: bool) -> None:
    """LP-R17: held только если planned_week раньше понедельника недели today."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, week, ["M14"]),
        },
    )
    result = import_ok(source, "math", today=today)
    if held:
        assert result.report["held_lessons"] == [
            {"lesson": 1, "state": "held", "snapshot": None, "marks": None}
        ]
    else:
        assert result.report["held_lessons"] == []


def test_lp_r17_held_uses_shifted_planned_week(tmp_path: Path) -> None:
    """LP-R17: граница held считается по planned_week уже после сдвига."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-10-26", ["M14"]),
            "math/plan/L02.md": lesson("math", 2, "2026-11-02", ["M14"]),
        },
    )
    result = import_ok(source, "math", shift_weeks=1, today=TODAY)
    assert [item["lesson"] for item in result.report["held_lessons"]] == [1]
    assert result.request["lessons"][0]["planned_week"] == "2026-11-02"
    assert result.request["lessons"][1]["planned_week"] == "2026-11-09"


def test_lp_r18_plan_import_14_missing_topic_warning(tmp_path: Path) -> None:
    """LP-R18, plan-import-14: в graph.json есть код, которого нет среди карточек; предупреждение с этим id."""
    source = copy_example(tmp_path, "plan-import-14")
    result = import_ok(source, "math", clean=False)
    assert result.errors == ()
    assert "ct-math:M81" not in {topic["id"] for topic in result.skeleton.values()}
    assert any(
        item["code"] == "generated_mismatch" and item["file"] == "graph/graph.json" and "M81" in item["message"]
        for item in result.warnings
    )


def test_lp_r18_extra_topic_warning(tmp_path: Path) -> None:
    """LP-R18: карточка, которой нет в graph.json, импортируется, и предупреждение называет её код."""
    source = copy_example(tmp_path, "plan-import-12")
    (source / "graph").mkdir()
    (source / "graph/graph.json").write_text('{"topics": ["M14"]}\n', encoding="utf-8")
    result = import_ok(source, "math", clean=False)
    assert "ct-math/topics/M13.yaml" in result.skeleton
    assert any(
        item["code"] == "generated_mismatch" and item["file"] == "graph/graph.json" and "M13" in item["message"]
        for item in result.warnings
    )


def test_lp_r18_plan_compares_weeks_before_shift(tmp_path: Path) -> None:
    """LP-R18: PLAN.md сверяется с неделями до сдвига; совпадение не даёт предупреждения."""
    matched = copy_example(tmp_path, "plan-import-10", "matched")
    (matched / "PLAN.md").write_text("| 1 | 2026-10-12 |\n", encoding="utf-8")
    ok = import_ok(matched, "math", shift_weeks=4)
    assert ok.request["lessons"][0]["planned_week"] == "2026-11-09"
    shifted = copy_example(tmp_path, "plan-import-10", "shifted")
    (shifted / "PLAN.md").write_text("| 1 | 2026-11-09 |\n", encoding="utf-8")
    warned = import_ok(shifted, "math", shift_weeks=4, clean=False)
    assert warned.request["lessons"][0]["planned_week"] == "2026-11-09"
    assert any(item["file"] == "PLAN.md" and item["code"] == "generated_mismatch" for item in warned.warnings)


def test_lp_r18_plan_number_order(tmp_path: Path) -> None:
    """LP-R18: порядок номеров в PLAN.md расходится — предупреждение, не блокировка."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-10-12", ["M14"]),
            "math/plan/L02.md": lesson("math", 2, "2026-10-19", ["M14"]),
            "PLAN.md": "| 2 | 2026-10-19 |\n| 1 | 2026-10-12 |\n",
        },
    )
    result = import_ok(source, "math", clean=False)
    assert [item["file"] for item in result.warnings] == ["PLAN.md"]


def test_lp_r18_matching_graph_and_plan_stay_silent(tmp_path: Path) -> None:
    """LP-R18: множество кодов graph.json и строки PLAN.md до сдвига совпадают — предупреждения нет."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M13.md": card("M13", "Предпосылка"),
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-10-12", ["M14"]),
            "math/plan/L02.md": lesson("math", 2, "2026-10-19", ["M13"], ["M14"]),
            "graph/graph.json": '{ "topics": ["M13", "M14"] }\n',
            "PLAN.md": "| 1 | 2026-10-12 |\n| 2 | 2026-10-19 |\n",
        },
    )
    result = import_ok(source, "math", shift_weeks=4)
    assert result.request["lessons"][0]["planned_week"] == "2026-11-09"
    assert result.request["lessons"][1]["planned_week"] == "2026-11-16"


def test_lp_r18_missing_generated_files_do_not_warn(tmp_path: Path) -> None:
    """LP-R18: нет graph.json и PLAN.md — сверки нет и предупреждения нет."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    assert result.warnings == ()


def test_lp_r18_unparseable_files_warn_and_sort(tmp_path: Path) -> None:
    """LP-R18: неразбираемый graph.json и PLAN.md — предупреждения, импорт не блокируется."""
    source = copy_example(tmp_path, "plan-import-01")
    (source / "graph").mkdir()
    (source / "graph/graph.json").write_text("не json\n", encoding="utf-8")
    (source / "PLAN.md").write_text("сверка не читается\n", encoding="utf-8")
    result = import_ok(source, "math", clean=False)
    assert [item["file"] for item in result.warnings] == ["PLAN.md", "graph/graph.json"]
    assert [item["code"] for item in result.warnings] == ["generated_mismatch", "generated_mismatch"]


def test_lp_r19_card_missing_required_field(tmp_path: Path) -> None:
    """LP-R19: нет обязательного поля карточки — frontmatter_invalid, указан файл."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/topics/M14.md"
    path.write_text(
        "".join(line for line in path.read_text(encoding="utf-8").splitlines(True) if not line.startswith("section:")),
        encoding="utf-8",
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "frontmatter_invalid", file="math/topics/M14.md")


def test_lp_r19_lesson_missing_required_field(tmp_path: Path) -> None:
    """LP-R19: нет обязательного поля урока — frontmatter_invalid."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/plan/L01.md"
    path.write_text(
        "".join(line for line in path.read_text(encoding="utf-8").splitlines(True) if not line.startswith("phase:")),
        encoding="utf-8",
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "frontmatter_invalid", file="math/plan/L01.md")


def test_lp_r19_card_bad_hours_type(tmp_path: Path) -> None:
    """LP-R19: неверный тип hours_lesson блокирует импорт и указывает строку значения."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/topics/M14.md"
    path.write_text(
        path.read_text(encoding="utf-8").replace("hours_lesson: 2", 'hours_lesson: "два"'),
        encoding="utf-8",
    )
    result = run_preserving(source, "math")
    assert_blocked(
        result,
        "frontmatter_invalid",
        file="math/topics/M14.md",
        line=line_containing(path, 'hours_lesson: "два"'),
    )


def test_lp_r19_lesson_without_heading(tmp_path: Path) -> None:
    """LP-R19: нет строки ``# `` — frontmatter_invalid."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/plan/L01.md"
    kept = "".join(
        line for line in path.read_text(encoding="utf-8").splitlines(True) if not line.startswith("#")
    )
    path.write_text(kept, encoding="utf-8")
    result = run_preserving(source, "math")
    assert_blocked(result, "frontmatter_invalid", file="math/plan/L01.md")


def test_lp_r19_heading_without_space_is_invalid(tmp_path: Path) -> None:
    """LP-R19: строка ``#Заголовок`` без пробела после решётки не считается названием."""
    source = copy_example(tmp_path, "plan-import-01")
    path = source / "math/plan/L01.md"
    text = path.read_text(encoding="utf-8")
    path.write_text(text.replace("#  Урок про дроби  ", "#Заголовок", 1), encoding="utf-8")
    path.write_text(
        "".join(line for line in path.read_text(encoding="utf-8").splitlines(True) if line != "# Не заголовок\n"),
        encoding="utf-8",
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "frontmatter_invalid", file="math/plan/L01.md")


def test_lp_r19_unknown_prereq_blocks(tmp_path: Path) -> None:
    """LP-R19: неизвестный код в prereqs — topic_unknown, указаны файл и строка."""
    source = copy_example(tmp_path, "plan-import-03")
    path = source / "math/topics/M14.md"
    path.write_text(path.read_text(encoding="utf-8").replace("  - M13", "  - M99"), encoding="utf-8")
    result = run_preserving(source, "math")
    assert_blocked(
        result,
        "topic_unknown",
        file="math/topics/M14.md",
        line=line_containing(path, "M99"),
    )


def test_lp_r19_errors_sorted_by_file_code_line(tmp_path: Path) -> None:
    """LP-R19: несколько блокирующих ошибок упорядочены по файлу, коду и строке."""
    source = write_files(
        tmp_path / "src",
        {
            "math/topics/M14.md": card("M14", "Дроби"),
            "math/plan/L01.md": lesson("math", 1, "2026-11-09", ["M14"], kind="seminar"),
            "math/plan/L02.md": lesson("math", 2, "2026-11-16", ["M14"], kind="seminar"),
        },
    )
    result = run_preserving(source, "math")
    assert_blocked(result, "kind_unknown", only=False)
    assert [(item["file"], item["code"]) for item in result.errors] == [
        ("math/plan/L01.md", "kind_unknown"),
        ("math/plan/L02.md", "kind_unknown"),
    ]


def test_lp_r20_lesson_body_is_not_parsed(tmp_path: Path) -> None:
    """LP-R20: текст урока ниже заголовка не разбирается, в результате путь и хеш."""
    source = copy_example(tmp_path, "plan-import-01")
    result = import_ok(source, "math")
    blob = payload(result)
    for phrase in (
        "Цель урока",
        "Перед уроком",
        "Ход урока",
        "Задачи на уроке",
        "Домашняя работа",
        "Заметки для генерации материалов",
        "Не заголовок",
        "Текст урока не разбирается.",
    ):
        assert phrase not in blob
    lesson_path = source / "math/plan/L01.md"
    assert result.request["lessons"][0]["source_hash"] == file_hash(lesson_path)
    lesson_path.write_bytes(lesson_path.read_bytes() + "Ход урока\n".encode("utf-8"))
    changed = import_ok(source, "math")
    assert changed.request["lessons"][0]["title"] == result.request["lessons"][0]["title"]
    assert changed.request["lessons"][0]["source_hash"] != result.request["lessons"][0]["source_hash"]
    assert "Ход урока" not in payload(changed)


def test_lp_r20_foreign_rights_other_student_and_subject(tmp_path: Path) -> None:
    """LP-R20: чужой ученик и дерево другого предмета не входят в результат math."""
    source = copy_example(tmp_path, "plan-import-01")
    math_lesson = source / "math/plan/L01.md"
    math_lesson.write_bytes(math_lesson.read_bytes() + f"Ученик {FOREIGN_STUDENT}.\n".encode("utf-8"))
    write_files(
        source,
        {
            "physics/topics/P05.md": card(
                "P05", "Кинематика", section="Механика", level="фундамент"
            ),
            "physics/plan/L01.md": lesson(
                "physics",
                1,
                "2026-11-16",
                ["P05"],
                heading="# Кинематика точки",
                body=f"Ключ ответа: {FOREIGN_KEY}",
            ),
        },
    )
    written = tmp_path / "catalog"
    written.mkdir()
    result = import_ok(source, "math", write_dir=written)
    blob = payload(result) + b"".join(tree_bytes(written).values()).decode("utf-8")
    assert FOREIGN_STUDENT in math_lesson.read_text(encoding="utf-8")
    assert FOREIGN_KEY in (source / "physics/plan/L01.md").read_text(encoding="utf-8")
    assert FOREIGN_STUDENT not in blob
    assert FOREIGN_KEY not in blob
    assert "ct-phys" not in blob
    assert "P05" not in blob
    assert result.request["student_id"] == STUDENT
    assert result.request["subject"] == "math"
    assert all(key.startswith("ct-math/") for key in result.skeleton)


@pytest.mark.parametrize("subject", ["biology", "Math", "ct-math", ""])
def test_interface_subject_must_be_math_or_physics(tmp_path: Path, subject: str) -> None:
    """Аргумент subject кроме math и physics — ValueError, файлы не пишутся."""
    source = copy_example(tmp_path, "plan-import-01", f"subject-{subject or 'empty'}")
    before = tree_bytes(source)
    written = tmp_path / f"out-{subject or 'empty'}"
    written.mkdir()
    (written / "sentinel.txt").write_text("keep\n", encoding="utf-8")
    out_before = tree_bytes(written)
    with pytest.raises(ValueError):
        run(source, subject, write_dir=written)
    assert tree_bytes(source) == before
    assert tree_bytes(written) == out_before


def test_lp_r11_cli_json_shift_weeks(tmp_path: Path) -> None:
    """LP-R11: CLI --shift-weeks и --json печатают один объект с planned_week примера."""
    source = copy_example(tmp_path, "plan-import-10")
    result = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--shift-weeks",
        "4",
        "--today",
        "2026-11-09",
        "--json",
    )
    assert result.returncode == 0
    payload_json = json.loads(result.stdout)
    assert payload_json["request"]["student_id"] == STUDENT
    assert payload_json["request"]["lessons"][0]["planned_week"] == "2026-11-09"
    assert payload_json["request"]["lessons"][0]["lesson"] == 1
    assert payload_json["errors"] == []


def test_lp_r1_cli_physics_json(tmp_path: Path) -> None:
    """LP-R1: CLI physics возвращает тему ct-phys:P05."""
    source = copy_example(tmp_path, "plan-import-02")
    result = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "physics",
        "--today",
        "2026-11-09",
        "--json",
    )
    assert result.returncode == 0
    payload_json = json.loads(result.stdout)
    assert "ct-phys/topics/P05.yaml" in payload_json["skeleton"]
    assert payload_json["skeleton"]["ct-phys/topics/P05.yaml"]["id"] == "ct-phys:P05"


def test_lp_r17_cli_held_exit_zero(tmp_path: Path) -> None:
    """LP-R17: held не является блокирующей ошибкой, CLI завершается нулём."""
    source = copy_example(tmp_path, "plan-import-13")
    result = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--today",
        "2026-11-09",
        "--json",
    )
    assert result.returncode == 0
    payload_json = json.loads(result.stdout)
    held = {item["lesson"]: item for item in payload_json["report"]["held_lessons"]}
    assert held[4]["state"] == "held"
    assert held[4]["snapshot"] is None
    assert held[4]["marks"] is None


def test_lp_r7_cli_plain_exit_1(tmp_path: Path) -> None:
    """LP-R7: без --json блокирующая ошибка печатается строкой и даёт код 1."""
    source = copy_example(tmp_path, "plan-import-06")
    result = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--today",
        "2026-11-09",
    )
    assert result.returncode == 1
    assert re.search(r"(?m)^math/plan/L01\.md: kind_unknown: \S", result.stdout)


def test_lp_r18_cli_warning_exit_zero(tmp_path: Path) -> None:
    """LP-R18: предупреждение не меняет код выхода и печатается с префиксом warning:."""
    source = copy_example(tmp_path, "plan-import-14")
    result = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--today",
        "2026-11-09",
    )
    assert result.returncode == 0
    assert re.search(r"(?m)^warning:.*generated_mismatch", result.stdout)
    assert "graph/graph.json" in result.stdout


def test_lp_r13_cli_repeat_and_dry_run(tmp_path: Path) -> None:
    """LP-R13: --already-imported даёт код 1, вместе с --dry-run код 0 и пустые расхождения."""
    source = copy_example(tmp_path, "plan-import-11")
    blocked = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--already-imported",
        "--today",
        "2026-11-09",
        "--json",
    )
    assert blocked.returncode == 1
    blocked_json = json.loads(blocked.stdout)
    assert blocked_json["request"] is None
    assert any(item["code"] == "already_imported" for item in blocked_json["errors"])
    dry = run_cli(
        str(source),
        "--student",
        STUDENT,
        "--subject",
        "math",
        "--already-imported",
        "--dry-run",
        "--today",
        "2026-11-09",
        "--json",
    )
    assert dry.returncode == 0
    dry_json = json.loads(dry.stdout)
    assert dry_json["request"] is not None
    assert dry_json["errors"] == []
    assert dry_json["report"]["differences"] == []


def test_interface_cli_usage_exit_2(tmp_path: Path) -> None:
    """Чужой subject и запуск без ученика — код выхода 2."""
    source = copy_example(tmp_path, "plan-import-01")
    foreign = run_cli(str(source), "--student", STUDENT, "--subject", "biology", "--today", "2026-11-09")
    assert foreign.returncode == 2
    missing = run_cli(str(source), "--subject", "math", "--today", "2026-11-09")
    assert missing.returncode == 2
