"""acceptance-otel-probe.sh: тег Tempo не должен содержать предкодированный %3D."""

from __future__ import annotations

from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "acceptance-otel-probe.sh"


def test_tempo_search_tags_not_double_encoded() -> None:
    text = SCRIPT.read_text(encoding="utf-8")
    if "service.name%3D" in text:
        raise AssertionError(
            "Tempo tags must not pre-encode = as %3D before curl --data-urlencode"
        )
    if 'data-urlencode "tags=service.name=${SERVICE_NAME}"' not in text:
        raise AssertionError(
            "expected single-encoded tags=service.name=${SERVICE_NAME}"
        )
