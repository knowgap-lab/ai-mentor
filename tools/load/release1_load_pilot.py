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
# AU-R20: не более 10 запросов /auth/* и /invite/* в минуту с одного IP.
AUTH_IP_LIMIT_PER_MINUTE = 10
AUTH_IP_LIMIT_MAX = AUTH_IP_LIMIT_PER_MINUTE - 1
AUTH_LOGIN_STAGGER_S = 60.0 / AUTH_IP_LIMIT_MAX


class AuthIPLimiter:
    """Скользящее окно для одного исходящего IP пилота (туннель / прокси)."""

    def __init__(self, max_per_minute: int) -> None:
        self._max = max_per_minute
        self._times: list[float] = []
        self._lock = threading.Lock()

    def wait(self) -> None:
        while True:
            with self._lock:
                now = time.monotonic()
                cutoff = now - 60.0
                self._times = [t for t in self._times if t > cutoff]
                if len(self._times) < self._max:
                    self._times.append(now)
                    return
                sleep_s = max(0.05, self._times[0] + 60.0 - now)
            time.sleep(sleep_s)


AUTH_IP_LIMITER = AuthIPLimiter(AUTH_IP_LIMIT_MAX)
HTTP_TOO_MANY_REQUESTS = 429


class PilotRateLimited(RuntimeError):
    """Сервер вернул rate_limited; замер нельзя считать успешным."""


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


ACTION_BUCKETS = ("read", "answer", "submit", "tutor_review")


def empty_action_buckets() -> dict[str, dict[str, list]]:
    return {name: {"latencies": [], "statuses": []} for name in ACTION_BUCKETS}


def build_action_metrics(
    latencies: list[float],
    statuses: list[int],
    *,
    measure_seconds: int,
    active_users: int,
) -> dict:
    samples = len(latencies)
    successes = sum(1 for s in statuses if 200 <= s < 300)
    errors = samples - successes
    p50 = int(percentile(latencies, 50))
    p95 = int(percentile(latencies, 95))
    max_ms = int(max(latencies)) if latencies else 0
    min_ms = int(min(latencies)) if latencies else 0
    throughput = round(samples / measure_seconds, 4) if measure_seconds > 0 else 0.0
    queue_ms = max(0, p50 - min_ms)
    return {
        "samples": samples,
        "successes": successes,
        "attempts": samples,
        "errors": errors,
        "p50_ms": p50,
        "p95_ms": p95,
        "max_ms": max_ms,
        "throughput": throughput,
        "active_users": active_users,
        "queue_ms": queue_ms,
    }


AUTH_FAILURE_STATUSES = frozenset({401, 403})


def all_recorded_statuses(buckets: dict[str, dict[str, list]]) -> list[int]:
    statuses: list[int] = []
    for name in ACTION_BUCKETS:
        statuses.extend(buckets[name]["statuses"])
    return statuses


def derive_auth_failures_only(buckets: dict[str, dict[str, list]]) -> bool | None:
    statuses = all_recorded_statuses(buckets)
    if not statuses:
        return None
    if all(status in AUTH_FAILURE_STATUSES for status in statuses):
        return True
    return None


def within_latency_limits(actions: dict, limits: dict) -> bool:
    checks = (
        ("read", limits.get("read", 500)),
        ("answer", limits.get("answer", 1000)),
        ("submit", limits.get("submit", 1000)),
        ("tutor_review", limits.get("tutor_review", 500)),
    )
    for name, budget in checks:
        item = actions.get(name)
        if not item:
            return False
        if item.get("successes", 0) < 1:
            return False
        if item.get("p95_ms", budget + 1) > budget:
            return False
    return True


def build_measure_payload(
    profile: dict,
    buckets: dict[str, dict[str, list]],
    *,
    started: float,
    unexpected_5xx: int,
    origin: str,
    students_started: int,
    tutors_started: int,
) -> dict:
    warmup = int(profile["warmup_seconds"])
    measure = int(profile["measure_seconds"])
    limits = profile.get("limits_ms") or {}
    prof = profile.get("profile") or {}
    mix = profile.get("mix") or {}
    think = profile.get("think_seconds") or {}

    active_by_action = {
        "read": students_started + tutors_started,
        "answer": students_started,
        "submit": students_started,
        "tutor_review": tutors_started,
    }
    actions: dict[str, dict] = {}
    attestation_gaps: list[str] = []
    for name in ACTION_BUCKETS:
        latencies = buckets[name]["latencies"]
        statuses = buckets[name]["statuses"]
        if not latencies:
            attestation_gaps.append(f"actions.{name} not performed")
            continue
        actions[name] = build_action_metrics(
            latencies,
            statuses,
            measure_seconds=measure,
            active_users=active_by_action[name],
        )

    read_p95 = actions.get("read", {}).get(
        "p95_ms", int(percentile(buckets["read"]["latencies"], 95))
    )
    answer_p95 = actions.get("answer", {}).get(
        "p95_ms", int(percentile(buckets["answer"]["latencies"], 95))
    )
    submit_p95 = actions.get("submit", {}).get(
        "p95_ms", int(percentile(buckets["submit"]["latencies"], 95))
    )
    tutor_p95 = actions.get("tutor_review", {}).get(
        "p95_ms", int(percentile(buckets["tutor_review"]["latencies"], 95))
    )
    write_p95 = max(answer_p95, submit_p95)

    attestation_gaps.append("browser_repeats not executed")
    if not os.environ.get("RELEASE_SHA", "").strip():
        attestation_gaps.append("sha (RELEASE_SHA unset)")
    attestation_gaps.append("ubuntu")
    attestation_gaps.append("browser not exercised by HTTP pilot")
    attestation_gaps.append("viewport not exercised by HTTP pilot")
    attestation_gaps.append("pdf_worker_separate not verified by HTTP pilot")
    attestation_gaps.append("reviewers")
    attestation_gaps.append("lost_confirmed_answers not measured")

    payload: dict = {
        "mode": "measure",
        "attestation_pass": False,
        "students": students_started,
        "tutors": tutors_started,
        "warmup_seconds": warmup,
        "measure_seconds": measure,
        "profile": {
            "vcpu": prof.get("vcpu"),
            "ram_gib": prof.get("ram_gib"),
            "observability": prof.get("observability"),
        },
        "virtualization": os.environ.get("LOAD_PILOT_VIRTUALIZATION", ""),
        "mix": {
            "read": mix.get("read"),
            "answer_or_submit": mix.get("answer_or_submit"),
            "tutor": mix.get("tutor"),
        },
        "think_seconds": {"min": think.get("min"), "max": think.get("max")},
        "seed": profile.get("seed"),
        "unexpected_5xx": unexpected_5xx,
        "actions": actions,
        "attestation_gaps": attestation_gaps,
        "duration_seconds": {
            "warmup": warmup,
            "measure": measure,
            "elapsed": int(time.monotonic() - started),
        },
        "samples": {name: len(buckets[name]["latencies"]) for name in ACTION_BUCKETS},
        "p95_ms": {
            "read": read_p95,
            "write": write_p95,
            "answer": answer_p95,
            "submit": submit_p95,
            "tutor_review": tutor_p95,
        },
        "limits_ms": limits,
        "origin": origin,
        "note": "результат не записывается в release1-attestation.json автоматически",
    }

    pinned_sha = os.environ.get("RELEASE_SHA", "").strip()
    if pinned_sha:
        payload["sha"] = pinned_sha

    auth_only = derive_auth_failures_only(buckets)
    if auth_only is not None:
        payload["auth_failures_only"] = auth_only

    payload["result"] = (
        "within_budget" if within_latency_limits(actions, limits) else "over_budget"
    )
    return payload


class PilotSession:
    def __init__(
        self,
        origin: str,
        app_origin: str,
        *,
        buckets: dict[str, dict[str, list]] | None = None,
        five_xx: dict[str, int] | None = None,
        five_xx_lock: threading.Lock | None = None,
    ) -> None:
        self.origin = origin.rstrip("/")
        self.app_origin = app_origin.rstrip("/")
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar)
        )
        self.csrf = ""
        self.user_id = ""
        self.lock = threading.Lock()
        self.buckets = buckets
        self.five_xx = five_xx
        self.five_xx_lock = five_xx_lock

    def _request(
        self,
        method: str,
        path: str,
        body: bytes | None = None,
        *,
        record: bool,
        bucket: str,
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
        if status == HTTP_TOO_MANY_REQUESTS:
            raise PilotRateLimited(f"HTTP {status} on {method} {path}")
        elapsed = (time.perf_counter() - start) * 1000.0
        if record and self.buckets is not None:
            with self.lock:
                self.buckets[bucket]["latencies"].append(elapsed)
                self.buckets[bucket]["statuses"].append(status)
            if (
                self.five_xx is not None
                and self.five_xx_lock is not None
                and 500 <= status < 600
            ):
                with self.five_xx_lock:
                    self.five_xx["count"] += 1
        return status, elapsed

    def login(self, telegram_id: str, role: str) -> None:
        path = "/auth/dev"
        body = json.dumps(
            {"telegram_id": telegram_id, "role": role}, ensure_ascii=False
        ).encode()
        url = f"{self.origin}{path}"
        req = urllib.request.Request(url, data=body, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Origin", self.app_origin)
        AUTH_IP_LIMITER.wait()
        try:
            with self.opener.open(req, timeout=60) as resp:
                token = resp.headers.get("X-CSRF-Token", "")
                if not token:
                    raise RuntimeError(
                        "login: нет X-CSRF-Token (нужен ENV=local и AUTH_DEV_MODE на стенде)"
                    )
                self.csrf = token
                _ = resp.read()
        except urllib.error.HTTPError as exc:
            if exc.code == HTTP_TOO_MANY_REQUESTS:
                raise PilotRateLimited(f"HTTP {exc.code} on POST {path}") from exc
            raise RuntimeError(f"HTTP {exc.code} on POST {path}") from exc
        self._request("GET", "/me", record=False, bucket="read")

    def student_action(self, rng: random.Random, mix: dict, record: bool) -> None:
        roll = rng.random()
        read_cut = mix.get("read", 0.6)
        write_cut = read_cut + mix.get("answer_or_submit", 0.3)
        if roll < read_cut:
            path = rng.choice(["/", "/me", "/me/sessions"])
            self._request("GET", path, record=record, bucket="read")
        elif roll < write_cut:
            # Записывающий микс без /auth/* (AU-R20: 10 auth+invite/мин с IP).
            body = json.dumps({}, ensure_ascii=False).encode()
            self._request(
                "POST",
                "/me/sessions/revoke-all",
                body,
                record=record,
                bucket="answer",
            )
        else:
            body = json.dumps({}, ensure_ascii=False).encode()
            self._request(
                "POST",
                "/assignments/synthetic-load/submit",
                body,
                record=record,
                bucket="submit",
            )

    def tutor_action(self, rng: random.Random, record: bool) -> None:
        uid = self.user_id or "demo-student"
        self._request(
            "GET",
            f"/students/{uid}",
            record=record,
            bucket="tutor_review",
        )
        if rng.random() < 0.5:
            self._request("GET", "/me", record=record, bucket="read")


def run_worker(
    label: str,
    role: str,
    origin: str,
    app_origin: str,
    profile: dict,
    end_at: float,
    record_from: float,
    buckets: dict[str, dict[str, list]],
    five_xx: dict[str, int],
    five_xx_lock: threading.Lock,
    login_stagger_s: float,
) -> None:
    seed = f"{profile.get('seed', 'release1-pilot-d326')}-{label}"
    rng = random.Random(seed)
    think = profile.get("think_seconds") or {"min": 3, "max": 10}
    mix = profile.get("mix") or {}
    if login_stagger_s > 0:
        time.sleep(login_stagger_s)
    sess = PilotSession(
        origin,
        app_origin,
        buckets=buckets,
        five_xx=five_xx,
        five_xx_lock=five_xx_lock,
    )
    sess.login(f"load-{label}", role)
    while time.monotonic() < end_at:
        record = time.monotonic() >= record_from
        if role == "tutor":
            sess.tutor_action(rng, record)
        else:
            sess.student_action(rng, mix, record)
        time.sleep(rng.uniform(think["min"], think["max"]))


def run_measure(origin: str, profile: dict) -> dict:
    app_origin = os.environ.get("LOAD_PILOT_APP_ORIGIN", origin).rstrip("/")
    warmup = profile["warmup_seconds"]
    buckets = empty_action_buckets()
    five_xx: dict[str, int] = {"count": 0}
    five_xx_lock = threading.Lock()

    started = time.monotonic()
    record_from = started + warmup
    end_at = started + warmup + profile["measure_seconds"]

    students_started = int(profile["students"])
    tutors_started = int(profile["tutors"])
    workers: list[tuple[str, str]] = []
    for i in range(students_started):
        workers.append((f"student-{i:02d}", "student"))
    for i in range(tutors_started):
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
                buckets,
                five_xx,
                five_xx_lock,
                idx * AUTH_LOGIN_STAGGER_S,
            )
            for idx, (label, role) in enumerate(workers)
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

    return build_measure_payload(
        profile,
        buckets,
        started=started,
        unexpected_5xx=five_xx["count"],
        origin=origin,
        students_started=students_started,
        tutors_started=tutors_started,
    )


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
