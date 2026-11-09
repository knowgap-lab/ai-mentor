"""Разбор карточек тем и файлов уроков в набор импорта плана 0.

Пакет не ходит в сеть и в базу: только читает синтетическое дерево и,
если разбор успешен, записывает скелет каталога в ``write_dir``.
"""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


PACKAGES = {"math": "ct-math", "physics": "ct-phys"}
KINDS = {"diagnostic", "new", "consolidation", "review", "mock", "analysis"}
CARD_REQUIRED = ("id", "title", "section", "level", "hours_lesson", "hours_home")
LESSON_REQUIRED = ("subject", "lesson", "week", "phase", "kind", "topics", "review")
DATE_TEXT = re.compile(r"^\d{4}-\d{2}-\d{2}$")
PLAN_ROW = re.compile(r"^\|\s*(\d+)\s*\|\s*(\d{4}-\d{2}-\d{2})\s*\|")
KEY_LINE = re.compile(r"^([A-Za-z_][\w-]*)\s*:(.*)$")
ITEM_LINE = re.compile(r"^(\s+)-\s+(.*)$")


@dataclass(frozen=True)
class ImportResult:
    """Результат одного вызова импорта."""

    request: dict | None
    skeleton: dict[str, dict] | None
    errors: tuple[dict, ...]
    warnings: tuple[dict, ...]
    report: dict


@dataclass
class LoadedFile:
    """Прочитанный markdown с разобранным frontmatter."""

    relative: str
    text: str
    digest: str
    data: dict[str, Any]


@dataclass
class Card:
    """Карточка, у которой обязательные поля уже проверены."""

    relative: str
    text: str
    code: str
    title: str
    section: str
    level: str
    hours_lesson: int | float
    hours_home: int | float
    prereqs: list[str]
    positions: list[str]
    digest: str


@dataclass
class Lesson:
    """Урок с разобранным frontmatter."""

    relative: str
    text: str
    number: int
    week: dt.date
    subject: str
    phase: str
    kind: str
    topics: list[str]
    review: list[str]
    title: str
    digest: str


def import_plan(
    source: Path,
    student_id: str,
    subject: str,
    shift_weeks: int = 0,
    already_imported: bool = False,
    dry_run: bool = False,
    today: dt.date | None = None,
    previous_snapshot: dict | None = None,
    write_dir: Path | None = None,
) -> ImportResult:
    """Собирает набор плана 0 и скелет каталога из синтетического дерева."""
    source = Path(source)
    if write_dir is not None:
        write_dir = Path(write_dir)
    if subject not in PACKAGES:
        raise ValueError("предмет должен быть math или physics")
    if not (source / subject).is_dir():
        raise ValueError(f"нет каталога {subject}")
    if already_imported and not dry_run:
        return _blocked(
            [
                _problem(
                    "already_imported",
                    "",
                    1,
                    "план этого ученика и предмета уже импортирован",
                )
            ]
        )

    if today is None:
        today = dt.date.today()
    package = PACKAGES[subject]
    errors: list[dict[str, Any]] = []
    cards, known_ids = _load_cards(source, subject, errors)
    _check_prereqs(cards, known_ids, errors)
    lessons = _load_lessons(source, subject, known_ids, errors)
    _check_duplicates(lessons, errors)
    unique = _without_duplicate_numbers(lessons)
    _check_lesson_rules(unique, known_ids, errors)
    warnings = _cross_check(source, known_ids, unique)
    if errors:
        return _blocked(errors, warnings)

    skeleton = _skeleton(package, cards)
    request_lessons, held = _lessons_payload(package, unique, shift_weeks, today)
    snapshot = {
        "source_hashes": {card.relative: card.digest for card in cards}
        | {lesson.relative: lesson.digest for lesson in unique}
    }
    request = {
        "student_id": student_id,
        "subject": subject,
        "shift_weeks": shift_weeks,
        "lessons": request_lessons,
        "snapshot": snapshot,
    }
    report = {
        "created_topics": [topic["id"] for topic in skeleton.values()],
        "created_lessons": [item["lesson"] for item in request_lessons],
        "held_lessons": held,
        "differences": _differences(snapshot["source_hashes"], previous_snapshot, dry_run),
    }
    if write_dir is not None and not dry_run:
        try:
            _write_skeleton(write_dir, skeleton)
        except OSError:
            return _blocked(
                [
                    _problem(
                        "import_failed",
                        "",
                        1,
                        "не удалось записать скелет каталога",
                    )
                ]
            )
    return ImportResult(
        request=request,
        skeleton=skeleton,
        errors=(),
        warnings=_sorted(warnings),
        report=report,
    )


def _blocked(
    errors: list[dict[str, Any]],
    warnings: list[dict[str, Any]] | None = None,
) -> ImportResult:
    return ImportResult(
        request=None,
        skeleton=None,
        errors=_sorted(errors),
        warnings=_sorted(warnings or []),
        report={
            "created_topics": [],
            "created_lessons": [],
            "held_lessons": [],
            "differences": [],
        },
    )


def _problem(code: str, file: str, line: int, message: str) -> dict[str, Any]:
    return {"code": code, "file": file, "line": line, "message": message}


def _sorted(items: list[dict[str, Any]]) -> tuple[dict, ...]:
    return tuple(sorted(items, key=lambda item: (item["file"], item["code"], item["line"])))


def _load_cards(
    source: Path,
    subject: str,
    errors: list[dict[str, Any]],
) -> tuple[list[Card], set[str]]:
    cards: list[Card] = []
    known: set[str] = set()
    topics_dir = source / subject / "topics"
    if not topics_dir.is_dir():
        return cards, known
    for path in sorted(topics_dir.glob("*.md")):
        loaded = _read_markdown(source, path, errors)
        if loaded is None:
            continue
        code = loaded.data.get("id")
        if isinstance(code, str) and code:
            known.add(code)
        card = _parse_card(loaded, errors)
        if card is not None:
            cards.append(card)
    return cards, known


def _load_lessons(
    source: Path,
    subject: str,
    known_ids: set[str],
    errors: list[dict[str, Any]],
) -> list[Lesson]:
    lessons: list[Lesson] = []
    plan_dir = source / subject / "plan"
    if not plan_dir.is_dir():
        return lessons
    for path in sorted(plan_dir.glob("*.md")):
        loaded = _read_markdown(source, path, errors)
        if loaded is None:
            continue
        lesson = _parse_lesson(loaded, subject, known_ids, errors)
        if lesson is not None:
            lessons.append(lesson)
    return lessons


def _read_markdown(source: Path, path: Path, errors: list[dict[str, Any]]) -> LoadedFile | None:
    raw = path.read_bytes()
    relative = path.relative_to(source).as_posix()
    digest = hashlib.sha256(raw).hexdigest()
    try:
        text = raw.decode("utf-8").replace("\r\n", "\n").replace("\r", "\n")
    except UnicodeDecodeError:
        errors.append(_problem("frontmatter_invalid", relative, 1, "файл не в кодировке UTF-8"))
        return None
    data = _frontmatter(text, relative, errors)
    if data is None:
        return None
    return LoadedFile(relative=relative, text=text, digest=digest, data=data)


def _frontmatter(text: str, relative: str, errors: list[dict[str, Any]]) -> dict[str, Any] | None:
    if not text.startswith("---\n"):
        errors.append(_problem("frontmatter_invalid", relative, 1, "нет frontmatter"))
        return None
    rest = text[4:]
    closing = re.search(r"\n---[ \t]*(?:\n|$)", rest)
    if closing is None:
        errors.append(_problem("frontmatter_invalid", relative, 1, "frontmatter не закрыт"))
        return None
    raw = rest[: closing.start()]
    try:
        loaded = yaml.safe_load(raw) if raw.strip() else None
    except yaml.YAMLError as exc:
        line = 1
        mark = getattr(exc, "problem_mark", None)
        if mark is not None and getattr(mark, "line", None) is not None:
            line = mark.line + 2
        errors.append(_problem("frontmatter_invalid", relative, line, "frontmatter не разбирается"))
        return None
    if not isinstance(loaded, dict):
        errors.append(_problem("frontmatter_invalid", relative, 1, "frontmatter должен быть объектом"))
        return None
    return loaded


def _parse_card(loaded: LoadedFile, errors: list[dict[str, Any]]) -> Card | None:
    data = loaded.data
    before = len(errors)
    missing = [key for key in CARD_REQUIRED if key not in data]
    if missing:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                1,
                "нет обязательного поля: " + ", ".join(missing),
            )
        )
    code = _text_field(data, "id", loaded, errors)
    title = _text_field(data, "title", loaded, errors)
    section = _text_field(data, "section", loaded, errors)
    level = _text_field(data, "level", loaded, errors)
    hours_lesson = _number_field(data, "hours_lesson", loaded, errors)
    hours_home = _number_field(data, "hours_home", loaded, errors)
    prereqs = _code_list(data, "prereqs", loaded, errors, allow_empty=True, required=False)
    positions = _code_list(data, "ct_positions", loaded, errors, allow_empty=True, required=False)
    if hours_lesson is not None and hours_home is not None and hours_lesson + hours_home <= 0:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                _key_line(loaded.text, "hours_lesson"),
                "сумма часов темы не положительная",
            )
        )
    if len(errors) != before or None in (
        code,
        title,
        section,
        level,
        hours_lesson,
        hours_home,
        prereqs,
        positions,
    ):
        return None
    assert code and title is not None and section is not None and level is not None
    assert hours_lesson is not None and hours_home is not None
    assert prereqs is not None and positions is not None
    return Card(
        relative=loaded.relative,
        text=loaded.text,
        code=code,
        title=title,
        section=section,
        level=level,
        hours_lesson=hours_lesson,
        hours_home=hours_home,
        prereqs=prereqs,
        positions=positions,
        digest=loaded.digest,
    )


def _parse_lesson(
    loaded: LoadedFile,
    subject: str,
    known_ids: set[str],
    errors: list[dict[str, Any]],
) -> Lesson | None:
    data = loaded.data
    before = len(errors)
    missing = [key for key in LESSON_REQUIRED if key not in data]
    if missing:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                1,
                "нет обязательного поля: " + ", ".join(missing),
            )
        )
    lesson_subject = _text_field(data, "subject", loaded, errors)
    if isinstance(data.get("subject"), str) and data["subject"] != subject:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                _key_line(loaded.text, "subject"),
                "предмет во frontmatter не совпадает с аргументом",
            )
        )
        lesson_subject = None
    number = _lesson_number(data, loaded, errors)
    week = _week_field(data, loaded, errors)
    phase = _text_field(data, "phase", loaded, errors)
    kind = _kind_field(data, loaded, errors)
    topics = _code_list(data, "topics", loaded, errors, allow_empty=False, required=True)
    review = _code_list(data, "review", loaded, errors, allow_empty=True, required=True)
    title = _title(loaded.text)
    if title is None:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                1,
                "нет строки заголовка, которая начинается с «# »",
            )
        )
    if topics is not None:
        for code in dict.fromkeys(topics):
            if code not in known_ids:
                errors.append(
                    _problem(
                        "topic_unknown",
                        loaded.relative,
                        _item_line(loaded.text, "topics", code),
                        f"неизвестная тема {code}",
                    )
                )
    if len(errors) != before or None in (
        lesson_subject,
        number,
        week,
        phase,
        kind,
        topics,
        review,
        title,
    ):
        return None
    assert lesson_subject and number is not None and week is not None
    assert phase is not None and kind is not None and topics is not None and review is not None
    assert title is not None
    return Lesson(
        relative=loaded.relative,
        text=loaded.text,
        number=number,
        week=week,
        subject=lesson_subject,
        phase=phase,
        kind=kind,
        topics=topics,
        review=review,
        title=title,
        digest=loaded.digest,
    )


def _check_prereqs(cards: list[Card], known_ids: set[str], errors: list[dict[str, Any]]) -> None:
    for card in cards:
        for code in dict.fromkeys(card.prereqs):
            if code not in known_ids:
                errors.append(
                    _problem(
                        "topic_unknown",
                        card.relative,
                        _item_line(card.text, "prereqs", code),
                        f"неизвестная тема {code}",
                    )
                )


def _text_field(
    data: dict[str, Any],
    key: str,
    loaded: LoadedFile,
    errors: list[dict[str, Any]],
) -> str | None:
    if key not in data:
        return None
    value = data[key]
    if isinstance(value, str):
        return value
    errors.append(
        _problem(
            "frontmatter_invalid",
            loaded.relative,
            _key_line(loaded.text, key),
            f"неверный тип поля {key}",
        )
    )
    return None


def _number_field(
    data: dict[str, Any],
    key: str,
    loaded: LoadedFile,
    errors: list[dict[str, Any]],
) -> int | float | None:
    if key not in data:
        return None
    value = data[key]
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                _key_line(loaded.text, key),
                f"неверный тип поля {key}",
            )
        )
        return None
    return value


def _lesson_number(data: dict[str, Any], loaded: LoadedFile, errors: list[dict[str, Any]]) -> int | None:
    if "lesson" not in data:
        return None
    value = data["lesson"]
    line = _key_line(loaded.text, "lesson")
    if isinstance(value, bool) or not isinstance(value, int):
        errors.append(_problem("frontmatter_invalid", loaded.relative, line, "неверный тип поля lesson"))
        return None
    if value < 1:
        errors.append(_problem("frontmatter_invalid", loaded.relative, line, "номер урока меньше 1"))
        return None
    return value


def _week_field(data: dict[str, Any], loaded: LoadedFile, errors: list[dict[str, Any]]) -> dt.date | None:
    if "week" not in data:
        return None
    week = _as_date(data["week"])
    if week is None:
        errors.append(
            _problem(
                "frontmatter_invalid",
                loaded.relative,
                _key_line(loaded.text, "week"),
                "неделя не является датой",
            )
        )
        return None
    return week


def _as_date(value: object) -> dt.date | None:
    if type(value) is dt.date:
        return value
    if isinstance(value, str) and DATE_TEXT.fullmatch(value):
        try:
            return dt.date.fromisoformat(value)
        except ValueError:
            return None
    return None


def _kind_field(data: dict[str, Any], loaded: LoadedFile, errors: list[dict[str, Any]]) -> str | None:
    if "kind" not in data:
        return None
    value = data["kind"]
    line = _key_line(loaded.text, "kind")
    if not isinstance(value, str):
        errors.append(_problem("frontmatter_invalid", loaded.relative, line, "неверный тип поля kind"))
        return None
    if value not in KINDS:
        errors.append(_problem("kind_unknown", loaded.relative, line, f"неизвестный вид урока {value}"))
        return None
    return value


def _code_list(
    data: dict[str, Any],
    key: str,
    loaded: LoadedFile,
    errors: list[dict[str, Any]],
    *,
    allow_empty: bool,
    required: bool,
) -> list[str] | None:
    if key not in data:
        if required:
            return None
        return []
    value = data[key]
    line = _key_line(loaded.text, key)
    if not isinstance(value, list):
        errors.append(_problem("frontmatter_invalid", loaded.relative, line, f"неверный тип поля {key}"))
        return None
    if not value and not allow_empty:
        errors.append(_problem("frontmatter_invalid", loaded.relative, line, f"список {key} пуст"))
        return None
    codes: list[str] = []
    for item in value:
        if not isinstance(item, str) or not item:
            errors.append(
                _problem(
                    "frontmatter_invalid",
                    loaded.relative,
                    line,
                    f"неверный тип элемента в {key}",
                )
            )
            return None
        codes.append(item)
    return codes


def _title(text: str) -> str | None:
    for line in text.splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    return None


def _key_line(text: str, key: str) -> int:
    for number, line in enumerate(text.splitlines(), start=1):
        matched = KEY_LINE.match(line)
        if matched and matched.group(1) == key:
            return number
    return 1


def _item_line(text: str, key: str, code: str) -> int:
    lines = text.splitlines()
    for index, line in enumerate(lines):
        matched = KEY_LINE.match(line)
        if not matched or matched.group(1) != key:
            continue
        inline = _inline_items(matched.group(2))
        if inline is not None and code in inline:
            return index + 1
        for later, row in enumerate(lines[index + 1 :], start=index + 2):
            if not row.strip():
                continue
            item = ITEM_LINE.match(row)
            if item is None or _scalar(item.group(2)) != code:
                if item is None or KEY_LINE.match(row):
                    break
                continue
            return later
        return index + 1
    return 1


def _inline_items(raw: str) -> list[str] | None:
    text = raw.split("#", 1)[0].strip()
    if not text:
        return None
    if text.startswith("[") and text.endswith("]"):
        inner = text[1:-1].strip()
        if not inner:
            return []
        return [_scalar(part) for part in inner.split(",")]
    return None


def _scalar(raw: str) -> str:
    text = raw.split("#", 1)[0].strip()
    if len(text) >= 2 and text[0] == text[-1] and text[0] in {"'", '"'}:
        return text[1:-1]
    return text


def _check_duplicates(lessons: list[Lesson], errors: list[dict[str, Any]]) -> None:
    grouped: dict[int, list[Lesson]] = {}
    for lesson in lessons:
        grouped.setdefault(lesson.number, []).append(lesson)
    for number, group in grouped.items():
        if len(group) < 2:
            continue
        for lesson in group:
            errors.append(
                _problem(
                    "frontmatter_invalid",
                    lesson.relative,
                    _key_line(lesson.text, "lesson"),
                    f"номер урока {number} повторяется",
                )
            )


def _without_duplicate_numbers(lessons: list[Lesson]) -> list[Lesson]:
    counts: dict[int, int] = {}
    for lesson in lessons:
        counts[lesson.number] = counts.get(lesson.number, 0) + 1
    return [lesson for lesson in lessons if counts[lesson.number] == 1]


def _check_lesson_rules(
    lessons: list[Lesson],
    known_ids: set[str],
    errors: list[dict[str, Any]],
) -> None:
    ordered = sorted(lessons, key=lambda lesson: lesson.number)
    numbers = [lesson.number for lesson in ordered]
    expected = list(range(1, len(numbers) + 1))
    if numbers != expected:
        missing = [number for number in expected if number not in set(numbers)]
        if missing:
            listed = ", ".join(str(number) for number in missing)
            noun = "урока" if len(missing) == 1 else "уроков"
            errors.append(
                _problem(
                    "lesson_gap",
                    "",
                    1,
                    f"пропуск в нумерации уроков: нет {noun} {listed}",
                )
            )
    introduced: set[str] = set()
    previous: dt.date | None = None
    for lesson in ordered:
        week_line = _key_line(lesson.text, "week")
        if lesson.week.weekday() != 0:
            errors.append(
                _problem(
                    "week_not_monday",
                    lesson.relative,
                    week_line,
                    "неделя урока не понедельник",
                )
            )
        if previous is not None and not previous < lesson.week:
            errors.append(
                _problem(
                    "week_not_increasing",
                    lesson.relative,
                    week_line,
                    "недели уроков не строго возрастают",
                )
            )
        previous = lesson.week
        for code in dict.fromkeys(lesson.review):
            if code not in known_ids:
                errors.append(
                    _problem(
                        "topic_unknown",
                        lesson.relative,
                        _item_line(lesson.text, "review", code),
                        f"неизвестная тема {code}",
                    )
                )
            elif code not in introduced:
                errors.append(
                    _problem(
                        "review_before_intro",
                        lesson.relative,
                        _item_line(lesson.text, "review", code),
                        f"повторение темы {code} раньше введения",
                    )
                )
        for code in lesson.topics:
            if code in known_ids:
                introduced.add(code)


def _cross_check(source: Path, card_codes: set[str], lessons: list[Lesson]) -> list[dict[str, Any]]:
    warnings: list[dict[str, Any]] = []
    graph = source / "graph" / "graph.json"
    if graph.is_file():
        warnings.extend(_graph_warnings(graph, card_codes))
    plan = source / "PLAN.md"
    if plan.is_file():
        warnings.extend(_plan_warnings(plan, lessons))
    return warnings


def _graph_warnings(path: Path, card_codes: set[str]) -> list[dict[str, Any]]:
    try:
        loaded = json.loads(path.read_text(encoding="utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return [_mismatch("graph/graph.json", "файл не разбирается форматом сверки")]
    topics = loaded.get("topics") if isinstance(loaded, dict) else None
    if not isinstance(topics, list) or not all(isinstance(item, str) for item in topics):
        return [_mismatch("graph/graph.json", "файл не разбирается форматом сверки")]
    graph_codes = set(topics)
    missing = sorted(graph_codes - card_codes)
    extra = sorted(card_codes - graph_codes)
    if not missing and not extra:
        return []
    parts = [f"нет карточки {code}" for code in missing]
    parts.extend(f"лишняя карточка {code}" for code in extra)
    return [_mismatch("graph/graph.json", "расхождение со сгенерированным графом: " + "; ".join(parts))]


def _plan_warnings(path: Path, lessons: list[Lesson]) -> list[dict[str, Any]]:
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return [_mismatch("PLAN.md", "файл не разбирается форматом сверки")]
    rows: list[tuple[int, str]] = []
    for line in text.splitlines():
        matched = PLAN_ROW.match(line.strip())
        if matched:
            rows.append((int(matched.group(1)), matched.group(2)))
    if not rows:
        return [_mismatch("PLAN.md", "файл не разбирается форматом сверки")]
    expected = [
        (lesson.number, lesson.week.isoformat())
        for lesson in sorted(lessons, key=lambda item: item.number)
    ]
    if rows != expected:
        return [
            _mismatch(
                "PLAN.md",
                "сгенерированный план расходится с числом, порядком или неделями уроков",
            )
        ]
    return []


def _mismatch(file: str, message: str) -> dict[str, Any]:
    return _problem("generated_mismatch", file, 1, message)


def _skeleton(package: str, cards: list[Card]) -> dict[str, dict]:
    skeleton: dict[str, dict] = {}
    for card in cards:
        citation = {"doc": "topic-card", "ref": card.relative}
        skeleton[f"{package}/topics/{card.code}.yaml"] = {
            "id": f"{package}:{card.code}",
            "title": card.title,
            "section": card.section,
            "foundation_level": card.level,
            "topic_hours": card.hours_lesson + card.hours_home,
            "ct_positions": list(card.positions),
            "prerequisite_elements": [f"{package}:{code}.E1" for code in card.prereqs],
            "citations": [dict(citation)],
            "elements": [
                {
                    "id": f"{package}:{card.code}.E1",
                    "title": "тема целиком",
                    "citations": [dict(citation)],
                }
            ],
        }
    return skeleton


def _lessons_payload(
    package: str,
    lessons: list[Lesson],
    shift_weeks: int,
    today: dt.date,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    monday = today - dt.timedelta(days=today.weekday())
    payload: list[dict[str, Any]] = []
    held: list[dict[str, Any]] = []
    for lesson in sorted(lessons, key=lambda item: item.number):
        planned = lesson.week + dt.timedelta(days=shift_weeks * 7)
        payload.append(
            {
                "subject": lesson.subject,
                "lesson": lesson.number,
                "planned_week": planned.isoformat(),
                "phase": lesson.phase,
                "kind": lesson.kind,
                "topics": [f"{package}:{code}" for code in lesson.topics],
                "review": [f"{package}:{code}" for code in lesson.review],
                "title": lesson.title,
                "source_path": lesson.relative,
                "source_hash": lesson.digest,
            }
        )
        if planned < monday:
            held.append({"lesson": lesson.number, "state": "held", "snapshot": None, "marks": None})
    return payload, held


def _differences(current: dict[str, str], previous: dict | None, dry_run: bool) -> list[str]:
    if not dry_run or not isinstance(previous, dict):
        return []
    hashes = previous.get("source_hashes")
    if not isinstance(hashes, dict):
        hashes = {}
    return sorted(path for path, digest in current.items() if hashes.get(path) != digest)


def _write_skeleton(write_dir: Path, skeleton: dict[str, dict]) -> None:
    payloads: list[tuple[Path, bytes]] = []
    for relative, topic in skeleton.items():
        path = write_dir.joinpath(*relative.split("/"))
        text = yaml.safe_dump(topic, allow_unicode=True, sort_keys=False)
        payloads.append((path, text.encode("utf-8")))
    _ensure_writable(write_dir, [path for path, _data in payloads])
    created: list[Path] = []
    replaced: list[tuple[Path, bytes]] = []
    try:
        for path, data in payloads:
            path.parent.mkdir(parents=True, exist_ok=True)
            if path.exists():
                replaced.append((path, path.read_bytes()))
            else:
                created.append(path)
            path.write_bytes(data)
    except OSError:
        for path, previous in reversed(replaced):
            path.write_bytes(previous)
        for path in reversed(created):
            if path.is_file():
                path.unlink()
        raise


def _ensure_writable(write_dir: Path, paths: list[Path]) -> None:
    if write_dir.exists() and not write_dir.is_dir():
        raise OSError("каталог записи недоступен")
    for path in paths:
        cursor = write_dir
        for part in path.relative_to(write_dir).parent.parts:
            cursor = cursor / part
            if cursor.exists() and not cursor.is_dir():
                raise OSError("на месте каталога лежит файл")
        if path.exists() and not path.is_file():
            raise OSError("на месте файла лежит каталог")
