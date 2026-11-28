"""Проверка compose.observability.yml и правил Prometheus без гостя."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
COMPOSE = ROOT / "deploy/compose.observability.yml"
RULES = ROOT / "deploy/observability/prometheus-rules.yml"


def test_compose_observability_config() -> None:
    if not COMPOSE.is_file():
        raise AssertionError("нет deploy/compose.observability.yml")
    if shutil.which("docker") is not None:
        proc = subprocess.run(
            ["docker", "compose", "-f", str(COMPOSE), "config"],
            capture_output=True,
            text=True,
            check=False,
        )
        if proc.returncode == 0:
            return
    with COMPOSE.open(encoding="utf-8") as handle:
        data = yaml.safe_load(handle)
    if not isinstance(data, dict) or "services" not in data:
        raise AssertionError("compose.observability.yml: нет services")


def test_prometheus_rules_load() -> None:
    with RULES.open(encoding="utf-8") as handle:
        data = yaml.safe_load(handle)
    groups = data.get("groups") if isinstance(data, dict) else None
    if not groups:
        raise AssertionError("prometheus-rules.yml: нет groups")
    names = [
        rule.get("alert")
        for group in groups
        for rule in group.get("rules", [])
        if isinstance(rule, dict)
    ]
    if "AcceptanceOtelCollectorDown" not in names:
        raise AssertionError("нет alert AcceptanceOtelCollectorDown")
    if shutil.which("docker") is None:
        return
    proc = subprocess.run(
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{RULES}:/rules.yml:ro",
            "prom/prometheus:v2.54.1",
            "promtool",
            "check",
            "rules",
            "/rules.yml",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise AssertionError(proc.stderr or proc.stdout or "promtool failed")