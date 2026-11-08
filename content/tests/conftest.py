"""Общие фикстуры и минимальные мутации синтетического каталога."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
import shutil

import pytest
import yaml


DEMO_CATALOG = Path(__file__).parent / "fixtures" / "demo-catalog"


@pytest.fixture
def catalog_root(tmp_path: Path) -> Path:
    """Копирует синтетический каталог и возвращает его корень с пакетами."""
    target = tmp_path / "catalog"
    shutil.copytree(DEMO_CATALOG, target)
    return target


def mutate_yaml(
    catalog_root: Path,
    relative_path: str,
    mutate: Callable[[dict], None],
) -> None:
    """Применяет минимальную мутацию к YAML-файлу внутри копии каталога."""
    path = catalog_root / relative_path
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    mutate(document)
    path.write_text(
        yaml.safe_dump(document, allow_unicode=True, sort_keys=False),
        encoding="utf-8",
    )


def mutate_frontmatter(
    catalog_root: Path,
    relative_path: str,
    mutate: Callable[[dict], None],
) -> None:
    """Применяет минимальную мутацию к YAML-frontmatter Markdown-блока."""
    path = catalog_root / relative_path
    frontmatter, body = path.read_text(encoding="utf-8").split("---\n", 2)[1:]
    document = yaml.safe_load(frontmatter)
    mutate(document)
    path.write_text(
        "---\n"
        + yaml.safe_dump(document, allow_unicode=True, sort_keys=False)
        + "---\n"
        + body,
        encoding="utf-8",
    )
