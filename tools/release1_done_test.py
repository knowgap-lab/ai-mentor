"""Программные тесты tools/release1_done.py. Замороженные тесты не трогает."""

from __future__ import annotations

import io
import json
import subprocess
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

import release1_done as done

OPS = [
    ("loginTelegram", "POST", "/auth/telegram", "core-api.yaml"),
    ("getConspectPdf", "GET", "/lessons/{lesson}/conspect.pdf", "core-api.yaml"),
    ("getSheetPdf", "GET", "/assignments/{id}/sheet.pdf", "core-api.yaml"),
    ("getTestFormPdf", "GET", "/tests/{test}/form.pdf", "core-api.yaml"),
    ("getStatus", "GET", "/status", "core-api.yaml"),
]
SCREENS = ["Ученик/Главная", "Репетитор/Ученики"]


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def passing_runner(args, _cwd):
    text = " ".join(args)
    if "TestAU_R12_RejectsWrongHash" in text:
        return 0, "--- PASS: TestAU_R12_RejectsWrongHash\n"
    if "TestWB_R28_S8_SheetPdf" in text:
        return 0, (
            "--- PASS: TestWB_R28_S8_SheetPdf\n"
            "--- PASS: TestWB_R29_S8_TutorPdfOmitsKey\n"
        )
    if "TestS7_Validate" in text:
        return 0, "".join(
            f"--- PASS: {name}\n"
            for name in (
                "TestS7_ValidateAcceptsEachType",
                "TestS7_ValidateRejectsMissingSchemaVersion",
                "TestS7_ValidateRejectsUnknownEventType",
                "TestS7_ValidateRejectsForeignPayload",
                "TestS7_ValidateRejectsExtraField",
            )
        )
    if text.endswith("deploy/install_repro_test.py"):
        return 0, "install_repro=passed\n"
    if text.endswith("deploy/observability_test.py"):
        return 0, "observability=passed\n"
    if text.endswith("deploy/restore_drill_test.py"):
        return 0, "restore_drill=passed\n"
    return 1, text


def build(root: Path) -> None:
    write(
        root / "core/cmd/server/mount.go",
        """package main
import "log/slog"

func mount() {
	telegram.Check(fields, token, now)
	_ = slog.NewJSONHandler(os.Stdout, nil)
	mux.HandleFunc("POST /auth/telegram", h)
	mux.HandleFunc("GET /lessons/{lesson}/conspect.pdf", h)
	mux.HandleFunc("GET /assignments/{id}/sheet.pdf", h)
	mux.HandleFunc("GET /tests/{test}/form.pdf", h)
	mux.HandleFunc("GET /status", h)
	mux.HandleFunc("GET /healthz", h)
	mux.HandleFunc("GET /readyz", h)
	mux.HandleFunc("GET /metrics", h)
}
""",
    )
    write(
        root / "docs/contracts/events.schema.json",
        '{"schema_version": 1}\n',
    )
    write(
        root / "core/internal/events/validate.go",
        "package events\nfunc Validate(doc []byte) error { return nil }\n"
        "func Documents() [][]byte { return nil }\n",
    )
    write(
        root / "core/internal/events/events_s7_test.go",
        "package events_test\nfunc TestS7(t *testing.T) {\n"
        "\tif err := events.Validate(doc); err != nil {}\n}\n",
    )
    for folder in (
        "attempts",
        "answercheck",
        "lessons",
        "plan",
        "homework",
        "recheck",
        "privacy",
        "auth",
        "e2e",
    ):
        write(
            root / "core/internal" / folder / "events_s7_test.go",
            "package demo_test\nfunc TestS7(t *testing.T) {\n"
            "\tdocs, err := events.Documents(ctx, conn)\n"
            "\tif err := events.Validate(doc); err != nil {}\n}\n",
        )
    write(
        root / "web/screens.json",
        json.dumps({"Ученик/Главная": "/", "Репетитор/Ученики": "/tutor"}),
    )
    write(root / "web/src/App.tsx", '<Route path="/" /><Route path="/tutor" />\n')
    compose = """services:
  migrate:
    image: example:1.2.3
  db:
    image: postgres:16.15
  content:
    image: example:1.2.3
  core:
    image: example:1.2.3
    environment:
      AUTH_DEV_MODE: false
  web:
    image: example:1.2.3
    environment:
      AUTH_DEV_MODE: false
  gotenberg:
    image: gotenberg/gotenberg:9.5.0
"""
    write(root / "deploy/compose.prod.yml", compose)
    write(root / "deploy/compose.staging.yml", compose)
    write(
        root / "deploy/backup.sh",
        "#!/bin/sh\nopenssl enc -aes-256-cbc -pbkdf2 -pass env:KEY\n"
        "echo копия не создана\n",
    )
    write(
        root / "deploy/restore-check.sh",
        "#!/bin/sh\n"
        ': "${CONSENT_REPLAY_BIN:?}"\n'
        ': "${JOURNAL_RECALC_BIN:?}"\n'
        'openssl enc -d -aes-256-cbc -in "$1" -out "$2"\n'
        "postgres:16.15\n",
    )
    write(
        root / "deploy/service-watch.sh",
        "OPS_CHAT_ID\n"
        "диск > 80 %\n"
        "загрузка каталога\n"
        "перепроверка\n"
        "задание failed\n"
        "Это не я\n"
        "внешняя проверка /status раз в минуту, тревога после 3 подряд неудач\n"
        "журналы сервисов 30 дней, затем удалить\n",
    )


def packages(root: Path) -> set[Path]:
    return {
        path.parent.resolve()
        for path in (root / "core").rglob("*.go")
        if not path.name.endswith("_test.go")
    }


def action(p95: int) -> dict:
    return {
        "attempts": 120,
        "successes": 120,
        "p50_ms": 80,
        "p95_ms": p95,
        "max_ms": p95,
        "errors": 0,
        "throughput": 1.2,
        "active_users": 20,
        "queue_ms": 15,
        "samples": 120,
    }


def lesson_step(status: int, command: str, **extra) -> dict:
    step = {"result": "pass", "command": command, "http_status": status}
    step.update(extra)
    return step


def complete_evidence(sha: str, screens: list[str]) -> dict:
    return {
        "sha": sha,
        "artifact_sha": sha,
        "integrated_ref": "main",
        "merged": True,
        "pull_request": {"state": "merged", "merge_sha": sha},
        "ci": {
            "sha": sha,
            "partial": False,
            "checks": [
                {
                    "name": name,
                    "conclusion": "success",
                    "sha": sha,
                    **({"diff_empty": True} if name == "generation" else {}),
                }
                for name in done.CI_JOBS
            ],
        },
        "catalog": {
            "sha": sha,
            "lesson_files": ["synthetic/lesson.md"],
            "topic_cards": ["synthetic/topic.json"],
            "catalog_ci": "pass",
            "postgres_load": "pass",
            "command": "python3 tools/catalog_ci.py synthetic/lesson.md",
            "result": "loaded 1 lesson and 1 card",
        },
        "lesson": {
            "sha": sha,
            "transport": "live_api",
            "depends_on": ["catalog"],
            "steps": {
                "clean_postgres": {
                    "result": "pass",
                    "command": "createdb ai_mentor_synthetic",
                },
                "tutor_creates_lesson": lesson_step(201, "POST /lessons"),
                "tutor_assigns_homework": lesson_step(201, "POST /assignments"),
                "student_correct": lesson_step(200, "POST /attempts correct"),
                "student_incorrect": lesson_step(200, "POST /attempts incorrect"),
                "tutor_sees_score_and_attempts": lesson_step(200, "GET /attempts"),
                "foreign_tutor_denied": lesson_step(403, "GET /attempts foreign"),
                "repeat_keeps_one_attempt": lesson_step(
                    200, "POST /attempts repeat", attempt_count=1
                ),
            },
        },
        "browser": {
            "sha": sha,
            "depends_on": ["lesson", "catalog"],
            "backend": "real",
            "database": "synthetic",
            "screenshots_only": False,
            "screens": {
                screen: {"behavior": "pass", "evidence": "scenario"}
                for screen in screens
            },
            "behaviors": {name: "pass" for name in done.BROWSER_BEHAVIORS},
        },
        "vm": {
            "sha": sha,
            "depends_on": ["ci", "browser"],
            "environment": "clean_ubuntu_vm",
            "terraform": {"create": "pass", "second_plan": "no_changes"},
            "ansible": {"converge": "pass", "second_converge": "no_changes"},
            "reboot": {"services": "pass", "readiness": "pass", "data": "persisted"},
            "background_job": "pass",
            "pdf": {"gotenberg": "real", "bytes_stable": True},
            "telemetry": {
                "logs": "pass",
                "traces": "pass",
                "prometheus": "pass",
                "grafana": "pass",
                "alert_rule": "pass",
                "pii": "absent",
            },
            "restore": {
                "separate_environment": True,
                "records_compared": True,
                "backup_file_only": False,
            },
            "telegram": {
                "login": "verified",
                "device_confirmation": "verified",
                "notification": "verified",
            },
            "recorded": {
                "commands": True,
                "versions": True,
                "resources": True,
                "timings": True,
                "results": True,
            },
        },
        "load": {
            "sha": sha,
            "depends_on": ["vm"],
            "students": 20,
            "tutors": 2,
            "profile": {"vcpu": 4, "ram_gib": 8, "observability": True},
            "ubuntu": "24.04",
            "virtualization": "kvm",
            "browser": "chrome",
            "viewport": "1280x720",
            "seed": "release1-pilot",
            "warmup_seconds": 300,
            "measure_seconds": 1200,
            "browser_repeats": 5,
            "mix": {"read": 0.6, "answer_or_submit": 0.3, "tutor": 0.1},
            "think_seconds": {"min": 3, "max": 10},
            "auth_failures_only": False,
            "unexpected_5xx": 0,
            "lost_confirmed_answers": 0,
            "pdf_worker_separate": True,
            "reviewers": ["reviewer-a", "reviewer-b"],
            "actions": {
                "read": action(400),
                "answer": action(800),
                "submit": action(900),
                "tutor_review": action(450),
            },
        },
        "reviews": [
            {
                "reviewer": "reviewer-a",
                "author": "author-1",
                "sha": sha,
                "state": "approved",
                "requirements": ["CI", "E2", "E3", "VM"],
                "comments_closed": True,
            },
            {
                "reviewer": "reviewer-b",
                "author": "author-1",
                "sha": sha,
                "state": "approved",
                "requirements": ["CI", "E2", "E3", "VM"],
                "comments_closed": True,
            },
        ],
        "findings": [],
        "open_findings": [],
        "blockers": [],
    }


def run(
    root: Path, runner=passing_runner, evidence=None, sha: str | None = None, **kwargs
):
    sha = sha or ("a" * 40)
    if evidence is None:
        evidence = complete_evidence(sha, SCREENS)
    return done.evaluate(
        root,
        OPS,
        SCREENS,
        packages=packages(root),
        runner=runner,
        evidence=evidence,
        sha=sha,
        **kwargs,
    )


class Release1DoneTest(unittest.TestCase):
    def test_exit_code_map(self) -> None:
        passed = done.Check("operations", "pass")
        failed = done.Check("screens", "fail")
        waiting = done.Check("install", "waiting")
        self.assertEqual(done.exit_code([passed]), 0)
        self.assertEqual(done.exit_code([passed, failed, waiting]), 1)
        self.assertEqual(done.exit_code([passed, waiting, failed]), 1)
        self.assertEqual(done.exit_code([passed, waiting]), 2)

    def test_complete_tree_exits_0(self) -> None:
        with self._tree() as root:
            checks = run(root)
            code = done.exit_code(checks)
            report = done.format_report("a" * 40, checks, code)
        self.assertEqual(code, 0, report)
        self.assertIn("result: all_checks_passed", report)
        self.assertNotIn("ready", report.lower())
        self.assertNotIn("готов", report)
        self.assertEqual(report.splitlines()[3], "first_failing_check: -")

    def test_missing_screen_is_first_failure(self) -> None:
        with self._tree() as root:
            path = root / "web/screens.json"
            path.write_text('{"Ученик/Главная": "/"}\n', encoding="utf-8")
            checks = run(root)
        code = done.exit_code(checks)
        self.assertEqual(code, 1)
        self.assertEqual(
            next(item.name for item in checks if item.status != "pass"), "screens"
        )

    def test_server_need_not_import_events(self) -> None:
        with self._tree() as root:
            mount = root / "core/cmd/server/mount.go"
            mount.write_text(
                mount.read_text(encoding="utf-8").replace(
                    'import "github.com/knowgap-lab/ai-mentor/core/internal/events"\n',
                    "",
                ),
                encoding="utf-8",
            )
            checks = run(root)
        journal = next(item for item in checks if item.name == "journal")
        self.assertEqual(journal.status, "pass")
        screens = next(item for item in checks if item.name == "screens")
        self.assertEqual(screens.status, "pass")

    def test_each_later_gap_stays_unfinished(self) -> None:
        cases = {
            "telegram": lambda root: (root / "core/cmd/server/mount.go").write_text(
                (root / "core/cmd/server/mount.go")
                .read_text(encoding="utf-8")
                .replace("telegram.Check(fields, token, now)", "telegram.Skip(fields)"),
                encoding="utf-8",
            ),
            "pdf": lambda root: (root / "deploy/compose.prod.yml").write_text(
                "services:\n  gotenberg:\n    image: gotenberg/gotenberg:latest\n",
                encoding="utf-8",
            ),
            "journal": lambda root: (
                root / "docs/contracts/events.schema.json"
            ).unlink(),
            "install": lambda root: (root / "deploy/backup.sh").unlink(),
            "observability": lambda root: (
                root / "core/cmd/server/mount.go"
            ).write_text(
                (root / "core/cmd/server/mount.go")
                .read_text(encoding="utf-8")
                .replace("\t_ = slog.NewJSONHandler(os.Stdout, nil)\n", ""),
                encoding="utf-8",
            ),
        }
        for name, change in cases.items():
            with self.subTest(name=name):
                with self._tree() as root:
                    change(root)
                    checks = run(root)
                code = done.exit_code(checks)
                first = next(item.name for item in checks if item.status != "pass")
                self.assertNotEqual(code, 0)
                self.assertEqual(first, name)

    def test_exit_2_and_skip_are_unfinished(self) -> None:
        def exit_two(args, cwd):
            if "TestAU_R12_RejectsWrongHash" in " ".join(args):
                return 2, "--- PASS: TestAU_R12_RejectsWrongHash\n"
            return passing_runner(args, cwd)

        def skipped(args, cwd):
            if "TestWB_R28_S8_SheetPdf" in " ".join(args):
                return 0, "--- SKIP: TestWB_R28_S8_SheetPdf\n"
            return passing_runner(args, cwd)

        with self._tree() as root:
            waiting = run(root, exit_two)
            skipped_checks = run(root, skipped)
        self.assertEqual(done.exit_code(waiting), 2)
        self.assertEqual(done.exit_code(skipped_checks), 1)
        self.assertEqual(
            next(item.status for item in skipped_checks if item.name == "pdf"),
            "fail",
        )

    def test_pdf_requires_both_passes(self) -> None:
        def only_sheet(args, cwd):
            if "TestWB_R28_S8_SheetPdf" in " ".join(args):
                return 0, "--- PASS: TestWB_R28_S8_SheetPdf\n"
            return passing_runner(args, cwd)

        with self._tree() as root:
            checks = run(root, only_sheet)
        pdf = next(item for item in checks if item.name == "pdf")
        self.assertEqual(pdf.status, "fail")

    def test_waiting_marker_is_unfinished(self) -> None:
        with self._tree() as root:
            path = root / "deploy/backup.sh"
            path.write_text(
                path.read_text(encoding="utf-8") + "status: waiting_on_user\n",
                encoding="utf-8",
            )
            checks = run(root)
        install = next(item for item in checks if item.name == "install")
        self.assertEqual(install.status, "waiting")
        self.assertEqual(done.exit_code(checks), 2)

    def test_two_refs_and_unknown_ref(self) -> None:
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = done.main(["HEAD", "HEAD"])
        self.assertEqual(code, 1)
        self.assertIn("unfinished", buf.getvalue())
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = done.main(["refs/no-such-release1-5c18"])
        self.assertEqual(code, 1)
        self.assertIn("unfinished", buf.getvalue())
        self.assertNotIn("all_checks_passed", buf.getvalue())

    def test_inventory_is_loaded_from_exported_commit(self) -> None:
        sha = "a" * 40
        buf = io.StringIO()
        with (
            patch.object(done, "resolve_sha", return_value=sha),
            patch.object(done, "extract_commit") as extract,
            patch.object(done, "load_mandatory", return_value=([], [])) as load,
            redirect_stdout(buf),
        ):
            code = done.main(["candidate"])
        repo, exported_sha, snapshot = extract.call_args.args
        self.assertEqual(exported_sha, sha)
        self.assertNotEqual(repo, snapshot)
        load.assert_called_once_with(snapshot)
        self.assertEqual(code, 1)
        self.assertIn(f"sha: {sha}", buf.getvalue())
        self.assertIn("first_failing_check: operations", buf.getvalue())

    def test_main_history_uses_remote_tracking_ref(self) -> None:
        sha, main_sha = "a" * 40, "b" * 40
        repo = Path("/repo")
        with (
            patch.object(done, "resolve_sha", return_value=main_sha) as resolve,
            patch.object(
                done.subprocess,
                "run",
                return_value=subprocess.CompletedProcess([], 0),
            ) as git,
        ):
            self.assertEqual(done.main_history_error(repo, sha), "")
        resolve.assert_called_once_with(repo, "refs/remotes/origin/main")
        self.assertEqual(
            git.call_args.args[0], ["git", "merge-base", "--is-ancestor", sha, main_sha]
        )

    def test_integration_report_cannot_replace_main_history(self) -> None:
        sha = "a" * 40
        with (
            patch.object(done, "resolve_sha", return_value="b" * 40),
            patch.object(
                done.subprocess,
                "run",
                return_value=subprocess.CompletedProcess([], 1),
            ),
            self._tree() as root,
        ):
            history_error = done.main_history_error(root, sha)
            checks = run(root, sha=sha, history_error=history_error)
        self.assertEqual(done.exit_code(checks), 1)
        self.assertEqual(self._status(checks, "integration"), "fail")
        self.assertIn(
            "SHA не найден в истории origin/main", self._detail(checks, "integration")
        )

    def test_quick_partial_open_pr_and_bare_acceptance_are_unfinished(self) -> None:
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = done.main(["--quick"])
        self.assertEqual(code, 1)
        self.assertIn("quick: fail", buf.getvalue())
        self.assertNotIn("all_checks_passed", buf.getvalue())
        sha = "a" * 40
        with self._tree() as root:
            quick = run(root, quick=True)
            partial = complete_evidence(sha, SCREENS)
            partial["ci"]["checks"] = partial["ci"]["checks"][:2]
            partial_checks = run(root, evidence=partial)
            opened = complete_evidence(sha, SCREENS)
            opened["pull_request"]["state"] = "open"
            opened["merged"] = False
            open_checks = run(root, evidence=opened)
            accepted = {"sha": sha, "artifact_sha": sha, "result": "принято"}
            accepted_checks = run(root, evidence=accepted)
        self.assertEqual(done.exit_code(quick), 1)
        self.assertEqual(quick[0].name, "quick")
        self.assertEqual(self._status(partial_checks, "ci"), "fail")
        self.assertIn("частичный", self._detail(partial_checks, "ci"))
        self.assertEqual(self._status(open_checks, "integration"), "fail")
        self.assertIn("открытый PR", self._detail(open_checks, "integration"))
        self.assertEqual(self._status(accepted_checks, "attestation"), "fail")
        self.assertIn("принято", self._detail(accepted_checks, "attestation"))
        for checks in (quick, partial_checks, open_checks, accepted_checks):
            self.assertNotEqual(done.exit_code(checks), 0)

    def test_stale_review_skip_comment_stub_and_sha_mismatch(self) -> None:
        sha = "a" * 40
        other = "b" * 40
        with self._tree() as root:
            stale = complete_evidence(sha, SCREENS)
            stale["reviews"][0]["sha"] = other
            stale_checks = run(root, evidence=stale)
            skipped = complete_evidence(sha, SCREENS)
            skipped["browser"]["screens"][SCREENS[0]]["behavior"] = "skip"
            skipped_checks = run(root, evidence=skipped)
            comment = complete_evidence(sha, SCREENS)
            comment["browser"]["screens"][SCREENS[0]] = "экран упомянут в комментарии"
            comment_checks = run(root, evidence=comment)
            stub = complete_evidence(sha, SCREENS)
            stub["lesson"]["steps"]["tutor_creates_lesson"]["http_status"] = 501
            stub_checks = run(root, evidence=stub)
            mismatch = run(root, evidence=complete_evidence(other, SCREENS), sha=sha)
        self.assertIn("старое ревью", self._detail(stale_checks, "reviews"))
        self.assertIn("пропущенный", self._detail(skipped_checks, "browser"))
        self.assertIn("комментарии", self._detail(comment_checks, "browser"))
        self.assertIn("501", self._detail(stub_checks, "lesson"))
        self.assertIn("SHA", self._detail(mismatch, "attestation"))
        for checks in (
            stale_checks,
            skipped_checks,
            comment_checks,
            stub_checks,
            mismatch,
        ):
            self.assertEqual(done.exit_code(checks), 1)

    def test_blocker_dependency_and_checker_error(self) -> None:
        sha = "a" * 40
        with self._tree() as root:
            active = complete_evidence(sha, SCREENS)
            active["blockers"] = [
                {
                    "id": "telegram-bot",
                    "category": "telegram",
                    "status": "waiting",
                    "dependency": "vm.telegram",
                }
            ]
            active_checks = run(root, evidence=active)
            duplicated = complete_evidence(sha, SCREENS)
            duplicated["blockers"] = [
                {
                    "id": "telegram-bot",
                    "category": "telegram",
                    "status": "waiting",
                    "dependency": "vm.telegram",
                },
                {
                    "id": "telegram-bot",
                    "category": "not-a-category",
                    "status": "waiting",
                    "dependency": "vm.telegram",
                },
            ]
            duplicated_checks = run(root, evidence=duplicated)
            missing = complete_evidence(sha, SCREENS)
            del missing["catalog"]
            missing_checks = run(root, evidence=missing)
        self.assertEqual(done.exit_code(active_checks), 2)
        self.assertEqual(self._status(active_checks, "operations"), "pass")
        self.assertEqual(self._status(active_checks, "blockers"), "waiting")
        self.assertEqual(done.exit_code(duplicated_checks), 1)
        self.assertIn("дублированный", self._detail(duplicated_checks, "blockers"))
        self.assertEqual(done.exit_code(missing_checks), 1)
        self.assertIn("зависимости", self._detail(missing_checks, "lesson"))
        checks = done.collect_checks(self._explode)
        self.assertEqual(checks[0].name, "checker")
        self.assertEqual(done.exit_code(checks), 1)

    def test_substitute_runs_are_not_the_release(self) -> None:
        sha = "a" * 40
        with self._tree() as root:
            mocked = complete_evidence(sha, SCREENS)
            mocked["browser"]["backend"] = "msw"
            cloud = complete_evidence(sha, SCREENS)
            cloud["vm"]["environment"] = "cloud_container"
            backup = complete_evidence(sha, SCREENS)
            backup["vm"]["restore"]["backup_file_only"] = True
            backup["vm"]["restore"]["records_compared"] = False
            denied = complete_evidence(sha, SCREENS)
            for step in denied["lesson"]["steps"].values():
                step["http_status"] = 401
            checks = {
                "browser": run(root, evidence=mocked),
                "vm": run(root, evidence=cloud),
                "restore": run(root, evidence=backup),
                "lesson": run(root, evidence=denied),
            }
        self.assertIn("настоящий сервер", self._detail(checks["browser"], "browser"))
        self.assertIn("контейнер", self._detail(checks["vm"], "vm"))
        self.assertIn("файл копии", self._detail(checks["restore"], "vm"))
        self.assertIn("401/403", self._detail(checks["lesson"], "lesson"))
        for item in checks.values():
            self.assertEqual(done.exit_code(item), 1)

    def test_attestation_must_stay_outside_the_commit(self) -> None:
        with self._tree() as root:
            write(
                root / "docs/handoff/release1-attestation.json",
                '{"result": "принято"}\n',
            )
            data, error = done.load_attestation(root, root)
        self.assertIsNone(data)
        self.assertIn("не отделён", error)
        with self._tree() as root:
            data, error = done.load_attestation(root, root)
        self.assertIsNone(data)
        self.assertIn("нет отдельного", error)

    def _explode(self) -> list[done.Check]:
        raise RuntimeError("boom")

    def _status(self, checks, name: str) -> str:
        return next(item.status for item in checks if item.name == name)

    def _detail(self, checks, name: str) -> str:
        return next(item.detail for item in checks if item.name == name)

    def test_real_contract_floor_and_restore_script(self) -> None:
        repo = Path(__file__).resolve().parents[1]
        ops, screens = done.load_mandatory(repo)
        self.assertGreaterEqual(len(ops), done.MIN_OPERATIONS)
        self.assertGreaterEqual(len(screens), done.MIN_SCREENS)
        self.assertIsNone(done.floor_checks(ops, screens))
        self.assertEqual(set(done.inv.EXCLUDED), {"putRtImport"})
        script = (repo / "deploy/restore-check.sh").read_text(encoding="utf-8")
        self.assertFalse(done.literal_secret(script))
        self.assertTrue(done.literal_secret('token: "synthetic-secret-value"\n'))
        self.assertFalse(done.literal_secret("password = var.db_password\n"))
        self.assertFalse(done.literal_secret("token = local.bot_token\n"))
        self.assertFalse(
            done.literal_secret('pg_password=$(urldecode "${pg_userinfo#*:}")\n')
        )

    def _tree(self):
        return tempfile_tree()


class tempfile_tree:
    def __enter__(self) -> Path:
        import tempfile

        self._tmp = tempfile.TemporaryDirectory()
        root = Path(self._tmp.name)
        build(root)
        return root

    def __exit__(self, *_args) -> None:
        self._tmp.cleanup()


if __name__ == "__main__":
    unittest.main()
