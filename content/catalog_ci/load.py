"""Загрузка раскладки каталога."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from .problem import Problem


@dataclass(frozen=True)
class Document:
    """Загруженный предусмотренный раскладкой файл."""

    file: str
    package_dir: Path
    definition: str
    data: Any


_PACKAGE_DEFINITIONS = {
    "package": "package",
    "topic": "topic",
    "variants": "variants_file",
    "block": "block_frontmatter",
    "test": "test",
    "lesson": "lesson_template",
    "transfer_map": "transfer_map",
    "note": "note",
}


def load_documents(root: Path) -> tuple[list[Document], list[Problem]]:
    """Load every file that belongs to the declared package layout."""

    documents: list[Document] = []
    problems: list[Problem] = []

    package_dirs = sorted(
        (
            candidate
            for candidate in root.iterdir()
            if candidate.is_dir() and (candidate / "package.yaml").is_file()
        ),
        key=lambda candidate: candidate.name,
    )

    for package_dir in package_dirs:
        package_file = package_dir / "package.yaml"
        package_document = _load_yaml_document(
            root, package_dir, package_file, _PACKAGE_DEFINITIONS["package"], problems
        )
        if package_document is not None:
            documents.append(package_document)

        answer_rules = _answer_rules_path(package_document)
        if _has_unsafe_answer_rules_path(package_document):
            problems.append(
                Problem(
                    code="schema_invalid",
                    file=_relative(root, package_file),
                    pointer="/answer_rules",
                    message="Путь к правилам ответа должен находиться внутри пакета.",
                )
            )
        elif answer_rules is not None:
            answer_file = package_dir / answer_rules
            if answer_file.is_file() and answer_file.is_relative_to(package_dir):
                answer_document = _load_yaml_document(
                    root, package_dir, answer_file, "subject_package", problems
                )
                if answer_document is not None:
                    documents.append(answer_document)
            else:
                problems.append(
                    Problem(
                        code="schema_invalid",
                        file=_relative(root, package_file),
                        pointer="/answer_rules",
                        message="Файл правил ответа не найден внутри пакета.",
                    )
                )

        for relative_glob, definition, loader in (
            ("topics/*.yaml", _PACKAGE_DEFINITIONS["topic"], _load_yaml_document),
            ("variants/*/*.yaml", _PACKAGE_DEFINITIONS["variants"], _load_yaml_document),
            ("blocks/*/*.md", _PACKAGE_DEFINITIONS["block"], _load_block_document),
            ("tests/*.yaml", _PACKAGE_DEFINITIONS["test"], _load_yaml_document),
            ("lessons/L*.yaml", _PACKAGE_DEFINITIONS["lesson"], _load_yaml_document),
            ("notes/*.yaml", _PACKAGE_DEFINITIONS["note"], _load_yaml_document),
        ):
            for source_file in sorted(package_dir.glob(relative_glob)):
                document = loader(root, package_dir, source_file, definition, problems)
                if document is not None:
                    documents.append(document)

        transfer_map = package_dir / "transfer-map.yaml"
        if transfer_map.is_file():
            document = _load_yaml_document(
                root,
                package_dir,
                transfer_map,
                _PACKAGE_DEFINITIONS["transfer_map"],
                problems,
            )
            if document is not None:
                documents.append(document)

    return documents, problems


def _relative(root: Path, file: Path) -> str:
    return file.relative_to(root).as_posix()


def _answer_rules_path(document: Document | None) -> Path | None:
    if document is None or not isinstance(document.data, dict):
        return None
    raw_path = document.data.get("answer_rules")
    if not isinstance(raw_path, str):
        return None
    candidate = Path(raw_path)
    if candidate.is_absolute() or ".." in candidate.parts:
        return None
    return candidate


def _has_unsafe_answer_rules_path(document: Document | None) -> bool:
    if document is None or not isinstance(document.data, dict):
        return False
    raw_path = document.data.get("answer_rules")
    if not isinstance(raw_path, str):
        return False
    candidate = Path(raw_path)
    return candidate.is_absolute() or ".." in candidate.parts


def _load_yaml_document(
    root: Path,
    package_dir: Path,
    source_file: Path,
    definition: str,
    problems: list[Problem],
) -> Document | None:
    try:
        data = yaml.safe_load(source_file.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as error:
        problems.append(
            Problem(
                code="schema_invalid",
                file=_relative(root, source_file),
                pointer="",
                message=f"Не удалось прочитать YAML: {error}.",
            )
        )
        return None
    return Document(_relative(root, source_file), package_dir, definition, data)


def _load_block_document(
    root: Path,
    package_dir: Path,
    source_file: Path,
    definition: str,
    problems: list[Problem],
) -> Document | None:
    try:
        text = source_file.read_text(encoding="utf-8")
        if not text.startswith("---\n"):
            raise ValueError("frontmatter должен начинаться с разделителя '---'")
        _, frontmatter, _ = text.split("---\n", 2)
        data = yaml.safe_load(frontmatter)
    except (OSError, ValueError, yaml.YAMLError) as error:
        problems.append(
            Problem(
                code="schema_invalid",
                file=_relative(root, source_file),
                pointer="",
                message=f"Не удалось прочитать frontmatter блока: {error}.",
            )
        )
        return None
    return Document(_relative(root, source_file), package_dir, definition, data)
