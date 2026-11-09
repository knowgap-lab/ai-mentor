"""Сборка проверки каталога: загрузка, схема, межфайловые проверки."""

from __future__ import annotations

from pathlib import Path

from .cross import cross_file_problems
from .load import load_documents
from .problem import Problem
from .schema_layer import schema_problems


def check_catalog(path: Path, mode: str = "pr") -> list[Problem]:
    """Return every structural problem in catalog packages below *path*.

    ``path`` is the directory that contains package directories, not a package
    directory itself.  The function deliberately does not inspect unrecognised
    files: they are outside the catalog layout.
    """

    if mode not in {"pr", "main"}:
        raise ValueError("mode должен быть 'pr' или 'main'")

    root = Path(path)
    if not root.is_dir():
        raise ValueError("путь к каталогу должен быть существующим каталогом")

    documents, loading_problems = load_documents(root)
    problems = list(loading_problems)
    problems.extend(schema_problems(documents))
    problems.extend(cross_file_problems(documents, mode))
    return sorted(problems, key=lambda problem: (problem.file, problem.code, problem.pointer))
