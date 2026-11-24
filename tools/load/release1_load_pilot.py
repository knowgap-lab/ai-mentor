#!/usr/bin/env python3
"""Нагрузочный пилот D326 для выпуска 1. Не записывает pass в аттестацию."""

from __future__ import annotations

import argparse
import http.cookiejar
import json
import os
import random
import subprocess
import threading
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

PROFILE_PATH = Path(__file__).with_name("d326-profile.json")
ROOT = PROFILE_PATH.resolve().parents[1]
FORBIDDEN_VIRT = frozenset({"cloud_container", "cursor_cloud", "", None})


def evaluated_sha() -> str:
    pinned = os.environ.get("RELEASE_SHA", "").strip()
    if pinned:
        return pinned
    return subprocess.check_output(
        ["git", "-C", str(ROOT), "rev-parse", "HEAD^{commit}"],
        text=True,
    ).strip()


def load_profile() -> dict:
    return json.loads(PROFILE_PATH.read_text(encoding="utf-8"))


def validate_profile(profile: dict) -> list[str]:
    problems: list[str] = []
    if profile.get("students") != 20 or profile.get("tutors") != 2:
        problems.append("профиль не 20 учеников и 2 репетитора")
    p = profile.get("profile") or {}
    if (
        p.get("vcpu") != 4
        or p.get("ram_gib") != 8
        or p.get("observability") is not True
    ):
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
    profile_sha = profile.get("sha")
    if profile_sha is not None and profile_sha != evaluated_sha():
        problems.append("sha в профиле не совпадает с коммитом под тестом")
    return problems


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


class PilotSession:
    def __init__(self, origin: str, app_origin: str) -> None:
        self.origin = origin.rstrip("/")
        self.app_origin = app_origin.rstrip("/")
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar)
        )
        self.csrf = ""
        self.user_id = ""
        self.lock = threading.Lock()

    def _request(
        self,
        method: str,
        path: str,
        body: bytes | None = None,
        *,
        record: bool,
        bucket: str,
        samples: dict[str, list[float]],
    ) -> tuple[int, float]:
        url = f"{self.origin}{path}"
        req = urllib.request.Request(url, data=body, method=method)
        if body is not None:
            req.add_header("Content-Type", "application/json")
        if self.csrf:
            req.add_header("X-CSRF-Token", self.csrf)
        req.add_header("Origin", self.app_origin)
        start = time.perf_counter()
        try:
            with self.opener.open(req, timeout=60) as resp:
                status = resp.status
                if path == "/me" and resp.length is not None:
                    payload = resp.read()
                    try:
                        data = json.loads(payload.decode("utf-8"))
                        uid = data.get("user_id") or data.get("userId") or ""
                        if uid:
                            with self.lock:
                                self.user_id = uid
                    except json.JSONDecodeError:
                        pass
        except urllib.error.HTTPError as exc:
            status = exc.code
        elapsed = (time.perf_counter() - start) * 1000.0
        if record:
            with self.lock:
                samples[bucket].append(elapsed)
        return status, elapsed

    def login(self, telegram_id: str, role: str) -> None:
        body = json.dumps(
            {"telegram_id": telegram_id, "role": role}, ensure_ascii=False
        ).encode()
        url = f"{self.origin}/auth/dev"
        req = urllib.request.Request(url, data=body, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Origin", self.app_origin)
        with self.opener.open(req, timeout=60) as resp:
            token = resp.headers.get("X-CSRF-Token", "")
            if not token:
                raise RuntimeError(
                    "login: нет X-CSRF-Token (нужен ENV=local и AUTH_DEV_MODE на стенде)"
                )
            self.csrf = token
            _ = resp.read()
        self._request("GET", "/me", record=False, bucket="read", samples={"read": []})

    def student_action(
        self, rng: random.Random, samples: dict[str, list[float]], record: bool
    ) -> None:
        roll = rng.random()
        mix = load_profile().get("mix", {})
        read_cut = mix.get("read", 0.6)
        write_cut = read_cut + mix.get("answer_or_submit", 0.3)
        if roll < read_cut:
            path = rng.choice(["/", "/me", "/me/sessions"])
            self._request("GET", path, record=record, bucket="read", samples=samples)
        elif roll < write_cut:
            body = json.dumps({}, ensure_ascii=False).encode()
            path = "/auth/logout"
            self._request(
                "POST", path, body, record=record, bucket="write", samples=samples
            )
            self.login("load-student-relogin", "student")
        else:
            body = json.dumps({}, ensure_ascii=False).encode()
            self._request(
                "POST",
                "/assignments/synthetic-load/submit",
                body,
                record=record,
                bucket="write",
                samples=samples,
            )

    def tutor_action(
        self, rng: random.Random, samples: dict[str, list[float]], record: bool
    ) -> None:
        uid = self.user_id or "demo-student"
        self._request(
            "GET",
            f"/students/{uid}",
            record=record,
            bucket="tutor_review",
            samples=samples,
        )
        if rng.random() < 0.5:
            self._request("GET", "/me", record=record, bucket="read", samples=samples)


def run_worker(
    label: str,
    role: str,
    origin: str,
    app_origin: str,
    profile: dict,
    end_at: float,
    record_from: float,
    samples: dict[str, list[float]],
) -> None:
    seed = f"{profile.get('seed', 'release1-pilot-d326')}-{label}"
    rng = random.Random(seed)
    think = profile.get("think_seconds") or {"min": 3, "max": 10}
    sess = PilotSession(origin, app_origin)
    sess.login(f"load-{label}", role)
    while time.monotonic() < end_at:
        record = time.monotonic() >= record_from
        if role == "tutor":
            sess.tutor_action(rng, samples, record)
        else:
            sess.student_action(rng, samples, record)
        time.sleep(rng.uniform(think["min"], think["max"]))


def run_measure(origin: str, profile: dict) -> dict:
    app_origin = os.environ.get("LOAD_PILOT_APP_ORIGIN", origin).rstrip("/")
    warmup = profile["warmup_seconds"]
    measure = profile["measure_seconds"]
    limits = profile["limits_ms"]
    samples: dict[str, list[float]] = {"read": [], "write": [], "tutor_review": []}
    sample_lock = threading.Lock()

    shared_samples: dict[str, list[float]] = samples

    started = time.monotonic()
    record_from = started + warmup
    end_at = started + warmup + measure

    workers: list[tuple[str, str]] = []
    for i in range(profile["students"]):
        workers.append((f"student-{i:02d}", "student"))
    for i in range(profile["tutors"]):
        workers.append((f"tutor-{i:02d}", "tutor"))

    with ThreadPoolExecutor(max_workers=len(workers)) as pool:
        futures = [
            pool.submit(
                run_worker,
                label,
                role,
                origin,
                app_origin,
                profile,
                end_at,
                record_from,
                shared_samples,
            )
            for label, role in workers
        ]
        for fut in as_completed(futures):
            exc = fut.exception()
            if exc is not None:
                return {
                    "mode": "measure",
                    "result": "error",
                    "attestation_pass": False,
                    "error": str(exc),
                }

    with sample_lock:
        read_p95 = int(percentile(samples["read"], 95))
        write_p95 = int(
            max(
                percentile(samples["write"], 95),
                percentile(samples.get("write", []), 95),
            )
        )
        tutor_p95 = int(percentile(samples["tutor_review"], 95))
        write_budget = max(limits.get("answer", 1000), limits.get("submit", 1000))

    within = (
        read_p95 <= limits["read"]
        and write_p95 <= write_budget
        and tutor_p95 <= limits.get("tutor_review", 500)
    )

    return {
        "mode": "measure",
        "result": "within_budget" if within else "over_budget",
        "attestation_pass": False,
        "duration_seconds": {
            "warmup": warmup,
            "measure": measure,
            "elapsed": int(time.monotonic() - started),
        },
        "samples": {k: len(v) for k, v in samples.items()},
        "p95_ms": {
            "read": read_p95,
            "write": write_p95,
            "tutor_review": tutor_p95,
        },
        "limits_ms": limits,
        "virtualization": os.environ.get("LOAD_PILOT_VIRTUALIZATION", ""),
        "origin": origin,
        "note": "результат не записывается в release1-attestation.json автоматически",
    }


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
        print(
            json.dumps({"result": "blocked", "problems": problems}, ensure_ascii=False)
        )
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
            print(
                json.dumps(
                    measure_blocked("set RELEASE_LOAD_PILOT_CONFIRM=1 on dedicated VM")
                )
            )
            return 2
        virt = os.environ.get("LOAD_PILOT_VIRTUALIZATION", "")
        if virt in FORBIDDEN_VIRT:
            print(
                json.dumps(
                    measure_blocked(
                        "LOAD_PILOT_VIRTUALIZATION must name real VM (e.g. kvm)"
                    )
                )
            )
            return 2
        if not args.origin:
            print(json.dumps(measure_blocked("LOAD_PILOT_ORIGIN or --origin required")))
            return 2
        result = run_measure(args.origin.rstrip("/"), profile)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        if result.get("result") == "error":
            return 1
        return 0

    parser.print_help()
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
