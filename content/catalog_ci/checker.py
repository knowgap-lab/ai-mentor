"""Сборка проверки каталога: загрузка, схема, связи, ключи и пул."""

from __future__ import annotations

from pathlib import Path

from .answers import answer_problems, drop_shadowed_schema_problems
from .cross import cross_file_problems
from .history import history_problems
from .load import load_documents
from .problem import Problem
from .schema_layer import schema_problems


def check_catalog(
    path: Path, mode: str = "pr", base: Path | None = None
) -> list[Problem]:
    """Return every catalog problem in packages below *path*.

    ``path`` is the directory that contains package directories, not a package
    directory itself.  When ``base`` is a catalog directory, entity ids are
    also compared with that base (CS-R4, CS-R23, CS-R25).  Without ``base``
    the history codes are not returned.

    The function deliberately does not inspect unrecognised files: they are
    outside the catalog layout.  It does not rewrite catalog files.
    """

    if mode not in {"pr", "main"}:
        raise ValueError("mode должен быть 'pr' или 'main'")

    root = Path(path)
    if not root.is_dir():
        raise ValueError("путь к каталогу должен быть существующим каталогом")

    base_root: Path | None = None
    if base is not None:
        base_root = Path(base)
        if not base_root.is_dir():
            raise ValueError("каталог базы должен существовать")

    documents, loading_problems = load_documents(root)
    problems = list(loading_problems)
    problems.extend(schema_problems(documents))
    problems.extend(cross_file_problems(documents, mode))
    problems.extend(answer_problems(documents))
    problems = drop_shadowed_schema_problems(problems)
    if base_root is not None:
        base_documents, _base_loading_problems = load_documents(base_root)
        problems.extend(history_problems(documents, base_documents))
    return sorted(problems, key=lambda problem: (problem.file, problem.code, problem.pointer))
