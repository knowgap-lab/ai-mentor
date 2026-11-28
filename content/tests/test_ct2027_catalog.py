"""Импорт ct-2027: сборка из sources, CI и проверка ответа целым числом."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
CONTENT = REPO / "content"
SOURCES = CONTENT / "sources" / "ct-2027"
FIXTURE = CONTENT / "tests" / "fixtures" / "ct-2027-catalog"
EXAMPLES = REPO / "tools" / "examples" / "check_answer_rules.py"


def test_ct2027_fixture_passes_catalog_ci_main() -> None:
    from catalog_ci import check_catalog

    problems = check_catalog(FIXTURE, mode="main")
    assert problems == []


def test_ct2027_build_is_idempotent(tmp_path: Path) -> None:
    from ct2027_build import build_catalog
    from catalog_ci import check_catalog

    out = tmp_path / "catalog"
    first = build_catalog(out, source=SOURCES)
    assert first.errors == ()
    assert check_catalog(out, mode="main") == []
    second = build_catalog(out, source=SOURCES)
    assert second.errors == ()


def test_ct2027_variant_integer_answer_scores() -> None:
  """Ответ 18 сходится с ключом M01.T11 из урока 2 математики."""
  spec = EXAMPLES.read_text(encoding="utf-8")
  ns: dict = {}
  exec(spec, ns)
  score = ns["score"]
  result = score("ct.integer", 18, "18")
  assert result["verdict"] == "correct"


def test_dev_seed_default_catalog_is_ct2027() -> None:
    seed = (REPO / "tools" / "dev_seed.py").read_text(encoding="utf-8")
    assert "ct-2027-catalog" in seed