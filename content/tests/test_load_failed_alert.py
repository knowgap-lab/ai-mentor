"""LD-R10: срыв транзакции содержимого зовёт тревогу служебного чата."""

from __future__ import annotations

import subprocess

import catalog_artifact
import loader


def _artifact() -> catalog_artifact.Artifact:
    entity = catalog_artifact.Entity(
        id="ct-demo:block",
        content_hash="abc",
        data=b'{"id":"ct-demo:block"}',
    )
    return catalog_artifact.Artifact(
        catalog_version=2,
        commit="commit-2",
        corrected_keys=(),
        entities=(entity,),
    )


def test_ld_r10_load_failed_calls_catalog_alert(monkeypatch) -> None:
    def fail_psql(*_args, **_kwargs):
        raise RuntimeError("transaction")

    seen: list[list[str]] = []

    def record(args, **_kwargs):
        seen.append([str(item) for item in args])
        return subprocess.CompletedProcess(args, 0)

    monkeypatch.setattr(loader, "_psql", fail_psql)
    monkeypatch.setattr(loader.subprocess, "run", record)
    artifact = _artifact()
    try:
        loader._apply(
            "postgres://loader@localhost/demo",
            artifact,
            "staging",
            (),
            loader._Write(entities=artifact.entities, record_load=True),
        )
    except loader.LoadRefused as error:
        assert error.code == "load_failed"
    else:
        raise AssertionError("load_failed не возник")
    assert len(seen) == 1
    assert seen[0][0].endswith("deploy/ops-alert.sh")
    assert seen[0][1] == "catalog"


def test_ld_r10_other_refusal_does_not_alert(monkeypatch) -> None:
    seen: list[list[str]] = []

    def record(args, **_kwargs):
        seen.append([str(item) for item in args])
        return subprocess.CompletedProcess(args, 0)

    monkeypatch.setattr(loader.subprocess, "run", record)
    artifact = _artifact()
    bad = catalog_artifact.Artifact(
        catalog_version=True,  # type: ignore[arg-type]
        commit=artifact.commit,
        corrected_keys=artifact.corrected_keys,
        entities=artifact.entities,
    )
    try:
        loader._apply(
            "postgres://loader@localhost/demo",
            bad,
            "staging",
            (),
            loader._Write(entities=artifact.entities, record_load=True),
        )
    except loader.LoadRefused as error:
        assert error.code == "artifact_invalid"
    else:
        raise AssertionError("artifact_invalid не возник")
    assert seen == []
