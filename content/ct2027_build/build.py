"""plan_import + compose для санитизированного ``content/sources/ct-2027``."""

from __future__ import annotations

import re
import shutil
from dataclasses import dataclass
from pathlib import Path

from composer import compose_lesson
from plan_import.importer import PACKAGES, _load_cards, _skeleton, _write_skeleton

DEFAULT_SOURCE = Path(__file__).resolve().parents[1] / "sources" / "ct-2027"
FORBIDDEN_PARTS = ("student", "handoff", "strategy", "research", "7-grade")


@dataclass(frozen=True)
class BuildResult:
    out_dir: Path
    lessons: tuple[str, ...]
    errors: tuple[str, ...]


def build_catalog(
    out_dir: Path,
    *,
    source: Path | None = None,
    lessons: tuple[tuple[str, int], ...] = (("math", 2), ("physics", 2)),
    student_id: str = "student-demo",
    today: str = "2026-11-09",
) -> BuildResult:
    source = Path(source or DEFAULT_SOURCE)
    out_dir = Path(out_dir)
    errors: list[str] = []
    _assert_source_safe(source, errors)
    if errors:
        return BuildResult(out_dir, (), tuple(errors))

    if out_dir.exists():
        shutil.rmtree(out_dir)
    out_dir.mkdir(parents=True)

    for subject in ("math", "physics"):
        card_errors: list[dict] = []
        cards, _known = _load_cards(source, subject, card_errors)
        if card_errors:
            errors.extend(
                f"topics {subject}: {item['file']}: {item['message']}"
                for item in card_errors[:5]
            )
            continue
        package = PACKAGES[subject]
        try:
            _write_skeleton(out_dir, _skeleton(package, cards))
        except OSError as exc:
            errors.append(f"не удалось записать скелет {subject}: {exc}")

    composed: list[str] = []
    for subject, lesson in lessons:
        write = out_dir / "_compose" / subject
        if write.exists():
            shutil.rmtree(write)
        write.mkdir(parents=True, exist_ok=True)
        result = compose_lesson(source, subject, lesson, write_dir=write)
        if result.errors:
            errors.extend(
                f"compose {subject} L{lesson:02d}: {item['message']}"
                for item in result.errors
            )
            continue
        package = "ct-math" if subject == "math" else "ct-phys"
        pkg_dir = out_dir / package
        pkg_dir.mkdir(parents=True, exist_ok=True)
        for relative, text in (result.files or {}).items():
            target = out_dir / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text, encoding="utf-8")
        composed.append(f"{package}:L{lesson:02d}")

    compose_scratch = out_dir / "_compose"
    if compose_scratch.exists():
        shutil.rmtree(compose_scratch, ignore_errors=True)

    return BuildResult(out_dir, tuple(composed), tuple(errors))


def _assert_source_safe(source: Path, errors: list[str]) -> None:
    if not source.is_dir():
        errors.append(f"нет каталога источника {source}")
        return
    for part in FORBIDDEN_PARTS:
        if (source / part).exists():
            errors.append(f"запрещённый каталог в источнике: {part}")