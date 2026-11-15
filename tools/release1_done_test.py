"""Программные тесты tools/release1_done.py. Замороженные тесты не трогает."""

from __future__ import annotations

import io
import json
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path

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
    if "TestS7_ValidateRejectsMissingSchemaVersion" in text:
        return 0, "--- PASS: TestS7_ValidateRejectsMissingSchemaVersion\n"
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


def run(root: Path, runner=passing_runner):
    return done.evaluate(root, OPS, SCREENS, packages=packages(root), runner=runner)


class Release1DoneTest(unittest.TestCase):
    def test_exit_code_map(self) -> None:
        passed = done.Check("operations", "pass")
        failed = done.Check("screens", "fail")
        waiting = done.Check("install", "waiting")
        self.assertEqual(done.exit_code([passed]), 0)
        self.assertEqual(done.exit_code([passed, failed, waiting]), 1)
        self.assertEqual(done.exit_code([passed, waiting, failed]), 2)

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
