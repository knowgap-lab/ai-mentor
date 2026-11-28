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


def test_dev_seed_defaults_to_demo_catalog() -> None:
    seed = (REPO / "tools" / "dev_seed.py").read_text(encoding="utf-8")
    assert '"demo-catalog"' in seed or "demo-catalog" in seed
    assert "AI_MENTOR_CT2027_CATALOG" in seed


def test_sanitize_nashego_uchenika_whole_phrase() -> None:
    import importlib.util

    script = SOURCES / "sanitize_public.py"
    spec = importlib.util.spec_from_file_location("sanitize_public", script)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    sample = "На ЦТ редкость, для нашего ученика не нужно."
    cleaned = module.sanitize_text(sample)
    assert "нашего" not in cleaned.casefold()
    assert "ученика" not in cleaned.casefold()
    assert "уровеньа" not in cleaned.casefold()
    assert "на базовом уровне" in cleaned


def test_ct2027_sources_have_no_private_pupil_data() -> None:
    import importlib.util

    script = SOURCES / "sanitize_public.py"
    spec = importlib.util.spec_from_file_location("sanitize_public", script)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    skip = {"README.md", "sanitize_public.py"}
    for path in SOURCES.rglob("*.md"):
        if path.name in skip:
            continue
        text = path.read_text(encoding="utf-8")
        for rule in module.FORBIDDEN:
            assert not rule.search(text), f"{path}: {rule.pattern}"
