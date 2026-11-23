#!/usr/bin/env python3
"""Нагрузочный пилот D326 для выпуска 1. Не записывает pass в аттестацию."""
from __future__ import annotations

import argparse
import json
import os
import random
import statistics
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

PROFILE_PATH = Path(__file__).with_name("d326-profile.json")
FORBIDDEN_VIRT = frozenset({"cloud_container", "cursor_cloud", "", None})


def load_profile() -> dict:
    return json.loads(PROFILE_PATH.read_text(encoding="utf-8"))


def validate_profile(profile: dict) -> list[str]:
    problems: list[str] = []
    if profile.get("students") != 20 or profile.get("tutors") != 2:
        problems.append("профиль не 20 учеников и 2 репетитора")
    p = profile.get("profile") or {}
    if p.get("vcpu") != 4 or p.get("ram_gib") != 8 or p.get("observability") is not True:
        problems.append("профиль не 4 vCPU / 8 GiB с наблюдаемостью")
    if (profile.get("warmup_seconds") or 0) < 300:
        problems.append("warmup < 300s")
    if (profile.get("measure_seconds") or 0) < 1200:
        problems.append("measure < 1200s")
    limits = profile.get("limits_ms") or {}
    if limits.get("read") != 500:
        problems.append("read limit != 500ms")
    if limits.get("answer") != 1000 or limits.get("submit") != 1000:
        problems.append("write limits != 1000ms")
    return problems


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    k = (len(ordered) - 1) * (pct / 100.0)
    f = int(k)
    c = min(f + 1, len(ordered) - 1)
    if f == c:
        return ordered[f]
    return ordered[f] + (ordered[c] - ordered[f]) * (k - f)


def http_ms(origin: str, path: str, cookie: str | None = None) -> tuple[int, float]:
    url = f"{origin.rstrip('/')}{path}"
    req = urllib.request.Request(url)
    if cookie:
        req.add_header("Cookie", cookie)
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            status = resp.status
    except urllib.error.HTTPError as exc:
        status = exc.code
    elapsed = (time.perf_counter() - start) * 1000.0
    return status, elapsed


def smoke(origin: str, profile: dict) -> dict:
    seed = profile.get("seed", "release1-pilot-d326")
    rng = random.Random(seed)
    reads: list[float] = []
    for _ in range(5):
        _, ms = http_ms(origin, "/")
        reads.append(ms)
        time.sleep(rng.uniform(0.05, 0.1))
    return {
        "mode": "smoke",
        "result": "unfinished",
        "note": "smoke не заменяет 20-минутный пилот и не записывается как pass",
        "samples": {"read": len(reads)},
        "read_p95_ms": int(percentile(reads, 95)),
    }


def measure_blocked(reason: str) -> dict:
    return {
        "mode": "measure",
        "result": "blocked",
        "reason": reason,
        "attestation_pass": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Release 1 load pilot (D326)")
    parser.add_argument("--validate-config", action="store_true")
    parser.add_argument("--smoke", action="store_true")
    parser.add_argument("--measure", action="store_true")
    parser.add_argument("--origin", default=os.environ.get("LOAD_PILOT_ORIGIN", ""))
    args = parser.parse_args()

    profile = load_profile()
    problems = validate_profile(profile)
    if problems and not args.validate_config:
        print(json.dumps({"result": "blocked", "problems": problems}, ensure_ascii=False))
        return 1

    if args.validate_config:
        payload = {
            "result": "ok" if not problems else "fail",
            "problems": problems,
            "profile": PROFILE_PATH.as_posix(),
            "sha": profile.get("sha"),
        }
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return 0 if not problems else 1

    if args.smoke:
        if not args.origin:
            print(json.dumps(measure_blocked("LOAD_PILOT_ORIGIN or --origin required")))
            return 1
        print(json.dumps(smoke(args.origin, profile), ensure_ascii=False, indent=2))
        return 0

    if args.measure:
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print(json.dumps(measure_blocked("full measure forbidden in CI")))
            return 2
        if os.environ.get("RELEASE_LOAD_PILOT_CONFIRM") != "1":
            print(json.dumps(measure_blocked("set RELEASE_LOAD_PILOT_CONFIRM=1 on dedicated VM")))
            return 2
        virt = os.environ.get("LOAD_PILOT_VIRTUALIZATION", "")
        if virt in FORBIDDEN_VIRT:
            print(json.dumps(measure_blocked("LOAD_PILOT_VIRTUALIZATION must name real VM (e.g. kvm)")))
            return 2
        if not args.origin:
            print(json.dumps(measure_blocked("LOAD_PILOT_ORIGIN or --origin required")))
            return 2
        print(
            json.dumps(
                {
                    **measure_blocked(
                        "orchestrator not implemented: run warm-up/measure loop on VM per tools/load/README.md"
                    ),
                    "planned_seconds": {
                        "warmup": profile["warmup_seconds"],
                        "measure": profile["measure_seconds"],
                    },
                    "virtualization": virt,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 2

    parser.print_help()
    return 1


if __name__ == "__main__":
    raise SystemExit(main())