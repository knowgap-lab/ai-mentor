#!/usr/bin/env python3
"""Проверка одного SHA выпуска 1.

Код 0 — на этом коммите сразу все обязательные операции и экраны,
проверенный вход через Telegram, PDF, журнал, воспроизводимая установка
Terraform и Ansible, наблюдаемость и проверенное восстановление.
Код 1 — проверка не прошла. Код 2 — дочерний процесс завершился с кодом 2
или в доказательстве написано, что ждём человека. Оба кода значат, что
выпуск не закончен. Скрипт не объявляет выпуск готовым.

Список операций и экранов берётся из контрактов и web.md рядом с этим
скриптом (кроме putRtImport, как в tools/release1_inventory.py). Код
читается только из дерева указанного коммита.

Доказательства, без которых код 0 невозможен:
- deploy/terraform/*.tf с required_version, resource и .terraform.lock.hcl (h1);
- deploy/ansible/install.yml и requirements.yml с версией x.y.z;
- deploy/install_repro_test.py печатает install_repro=passed;
- deploy/observability/: otel-collector.yaml, prometheus.yml, loki.yaml,
  tempo.yaml, alerts.yml, grafana/*.json, сроки 30d, 14d и 7d;
  в сервере /healthz, /readyz, /metrics и go.opentelemetry.io/otel;
  deploy/observability_test.py печатает observability=passed;
- deploy/restore-check.sh со шагами расшифровки, Postgres, CONSENT_REPLAY_BIN
  и JOURNAL_RECALC_BIN; deploy/restore_drill_test.py вызывает этот скрипт
  и печатает restore_drill=passed.
"""

from __future__ import annotations

import ast
import io
import json
import re
import subprocess
import sys
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path

import release1_inventory as inv
import yaml

MIN_OPERATIONS = 58
MIN_SCREENS = 14
PDF_OPS = ("getConspectPdf", "getSheetPdf", "getTestFormPdf")
WAITING_RE = re.compile(
    r"waiting_on_user|ждёт человека|ждет человека|^status:\s*waiting\b",
    re.MULTILINE,
)
PINNED_IMAGE = re.compile(r":(\d+\.\d+\.\d+)$")
VERSION_PIN = re.compile(r"version:\s*[\"']?(\d+\.\d+\.\d+)[\"']?")
SECRET_ASSIGN = re.compile(r"(?i)([A-Za-z0-9_]*(?:password|token))\s*[:=]\s*(\S+)")
HEX_RUN = re.compile(r"\b[0-9a-fA-F]{32,}\b")
SHA256_DIGEST = re.compile(r"sha256:[0-9a-fA-F]{64}")


@dataclass(frozen=True)
class Check:
    name: str
    status: str
    detail: str = ""


def exit_code(checks: list[Check]) -> int:
    for item in checks:
        if item.status == "pass":
            continue
        if item.status == "waiting":
            return 2
        return 1
    return 0


def format_report(sha: str, checks: list[Check], code: int) -> str:
    first = next((item.name for item in checks if item.status != "pass"), "-")
    result = "all_checks_passed" if code == 0 else "unfinished"
    lines = [
        f"sha: {sha}",
        f"result: {result}",
        f"exit: {code}",
        f"first_failing_check: {first}",
    ]
    for item in checks:
        lines.append(f"{item.name}: {item.status}")
        for line in item.detail.splitlines():
            lines.append(f"  {line}")
    return "\n".join(lines) + "\n"


def resolve_sha(repo: Path, ref: str) -> str:
    proc = subprocess.run(
        ["git", "rev-parse", "--verify", "--end-of-options", f"{ref}^{{commit}}"],
        cwd=repo,
        capture_output=True,
        text=True,
        check=False,
    )
    sha = proc.stdout.strip()
    if proc.returncode != 0 or len(sha) != 40 or "\n" in sha:
        message = proc.stderr.strip() or f"не один коммит: {ref}"
        raise LookupError(message)
    return sha


def extract_commit(repo: Path, sha: str, dest: Path) -> None:
    proc = subprocess.run(
        ["git", "archive", sha],
        cwd=repo,
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0:
        raise LookupError(proc.stderr.decode("utf-8", "replace").strip())
    with tarfile.open(fileobj=io.BytesIO(proc.stdout), mode="r:") as archive:
        archive.extractall(dest, filter="data")


def load_mandatory(repo: Path) -> tuple[list[tuple[str, str, str, str]], list[str]]:
    previous = inv.root
    inv.root = repo.resolve()
    try:
        ops = [op for op in inv.contract_ops() if op[0] not in inv.EXCLUDED]
        screens = inv.spec_screens()
    finally:
        inv.root = previous
    return ops, screens


def default_runner(args: list[str], cwd: Path) -> tuple[int, str]:
    try:
        proc = subprocess.run(
            args,
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=200,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        out = f"{exc.stdout or ''}{exc.stderr or ''}\ntimeout"
        return 1, out
    return proc.returncode, f"{proc.stdout or ''}{proc.stderr or ''}"


def load_packages(tree: Path) -> tuple[set[Path] | None, str]:
    previous = inv.root
    inv.root = tree.resolve()
    try:
        found = {path.resolve() for path in inv.server_packages()}
    except subprocess.CalledProcessError as exc:
        err = (exc.stderr or exc.stdout or str(exc)).strip()
        return None, err
    finally:
        inv.root = previous
    return found, ""


def package_sources(tree: Path, packages: set[Path]) -> list[tuple[Path, str]]:
    found = []
    root = tree / "core"
    if not root.is_dir():
        return found
    for go in root.rglob("*.go"):
        if go.name.endswith("_test.go") or go.parent.resolve() not in packages:
            continue
        found.append((go, go.read_text(encoding="utf-8")))
    return found


def declared_routes(sources: list[tuple[Path, str]]) -> dict[tuple[str, str], Path]:
    routes: dict[tuple[str, str], Path] = {}
    for go, text in sources:
        for method, path in inv.ROUTE.findall(text):
            routes.setdefault((method, path), go)
        for method, path in inv.MANUAL.findall(text):
            routes.setdefault((method.upper(), path), go)
    return routes


def literal_secret(text: str) -> bool:
    for raw in text.splitlines():
        line = raw.split("#", 1)[0]
        for match in SECRET_ASSIGN.finditer(line):
            value = match.group(2).strip("'\"")
            if value.startswith(("${", "{{", "?", "env:")):
                continue
            if value.lower() in {"false", "true", "null"}:
                continue
            return True
    stripped = SHA256_DIGEST.sub("", text)
    return HEX_RUN.search(stripped) is not None


def nontrivial_assert(path: Path) -> bool:
    try:
        parsed = ast.parse(path.read_text(encoding="utf-8"))
    except SyntaxError:
        return False
    for node in ast.walk(parsed):
        if not isinstance(node, ast.Assert):
            continue
        test = node.test
        if isinstance(test, ast.Constant) and test.value is True:
            continue
        return True
    return False


def calls_restore_script(path: Path) -> bool:
    try:
        parsed = ast.parse(path.read_text(encoding="utf-8"))
    except SyntaxError:
        return False
    for node in ast.walk(parsed):
        if (
            isinstance(node, ast.Constant)
            and isinstance(node.value, str)
            and "restore-check.sh" in node.value
        ):
            return True
    return False


def read_text(path: Path) -> str:
    if not path.is_file():
        return ""
    return path.read_text(encoding="utf-8")


def pinned_gotenberg(tree: Path) -> list[str]:
    problems = []
    images = []
    for name in ("compose.prod.yml", "compose.staging.yml"):
        path = tree / "deploy" / name
        text = read_text(path)
        if text == "":
            problems.append(f"нет deploy/{name}")
            continue
        try:
            document = yaml.safe_load(text)
        except yaml.YAMLError as exc:
            problems.append(f"{name}: не YAML ({exc})")
            continue
        services = document.get("services") if isinstance(document, dict) else None
        service = services.get("gotenberg") if isinstance(services, dict) else None
        if not isinstance(service, dict):
            problems.append(f"{name}: нет gotenberg")
            continue
        image = service.get("image")
        if (
            not isinstance(image, str)
            or "latest" in image.lower()
            or not PINNED_IMAGE.search(image)
        ):
            problems.append(f"{name}: образ Gotenberg не закреплён ({image!r})")
        else:
            images.append(image)
        ports = service.get("ports")
        if (isinstance(ports, list) and ports) or service.get("network_mode") == "host":
            problems.append(f"{name}: gotenberg опубликован")
    if len(images) == 2 and images[0] != images[1]:
        problems.append("образы Gotenberg prod и staging различаются")
    return problems


def command_problem(kind: str, code: int, out: str, marker: str) -> str | None:
    if code == 2 or WAITING_RE.search(out):
        return "waiting"
    if code != 0:
        tail = out.strip().splitlines()
        extra = tail[-1] if tail else ""
        return f"команда завершилась с кодом {code} {extra}".strip()
    if kind == "go":
        if "--- SKIP:" in out or "--- PASS:" not in out:
            extra = next(
                (
                    line.strip()
                    for line in out.splitlines()
                    if "SKIP:" in line
                    or line.startswith("--- FAIL")
                    or "unavailable" in line
                ),
                "",
            )
            reason = "программный тест не прошёл или пропущен"
            return f"{reason}: {extra}" if extra else reason
        return None
    if marker not in out or "skipped" in out.lower():
        return "программный тест не напечатал успешный прогон"
    return None


def run_command(runner, args: list[str], cwd: Path) -> tuple[int, str]:
    try:
        return runner(args, cwd)
    except Exception as exc:  # noqa: BLE001 — любой сбой команды есть провал проверки
        return 1, str(exc)


def settle(
    name: str,
    problems: list[str],
    blob: str,
    command: tuple[int, str] | None,
    marker: str,
    kind: str,
) -> Check:
    out = "" if command is None else command[1]
    if (
        WAITING_RE.search(blob)
        or WAITING_RE.search(out)
        or (command is not None and command[0] == 2)
    ):
        detail = "\n".join([*problems, "ожидание: код 2 или waiting_on_user"])
        return Check(name, "waiting", detail.strip())
    if command is not None:
        problem = command_problem(kind, command[0], out, marker)
        if problem == "waiting":
            return Check(name, "waiting", "ожидание: код 2 или waiting_on_user")
        if problem:
            problems = [*problems, problem]
    if problems:
        return Check(name, "fail", "\n".join(problems))
    return Check(name, "pass", "")


def check_operations(ops, routes, package_error: str) -> Check:
    if not ops:
        return Check("operations", "fail", "нет обязательных операций")
    if package_error:
        return Check("operations", "fail", package_error)
    missing = [
        f"нет: {op} {method} {path} ({contract})"
        for op, method, path, contract in ops
        if (method, path) not in routes
    ]
    if missing:
        return Check("operations", "fail", "\n".join(missing))
    return Check("operations", "pass", str(len(ops)))


def check_screens(tree: Path, screens: list[str]) -> Check:
    if len(screens) < 1:
        return Check("screens", "fail", "нет обязательных экранов")
    registry_file = tree / "web" / "screens.json"
    try:
        registry = (
            json.loads(registry_file.read_text(encoding="utf-8"))
            if registry_file.is_file()
            else {}
        )
    except json.JSONDecodeError as exc:
        return Check("screens", "fail", f"web/screens.json: {exc}")
    if not isinstance(registry, dict):
        return Check("screens", "fail", "web/screens.json должен быть объектом")
    paths: set[str] = set()
    web = tree / "web" / "src"
    if web.is_dir():
        for tsx in web.rglob("*.tsx"):
            if ".test." in tsx.name:
                continue
            paths.update(re.findall(r'path="([^"]+)"', tsx.read_text(encoding="utf-8")))
    missing = []
    for screen in screens:
        route = registry.get(screen)
        if route is None:
            missing.append(f"нет: {screen} — нет в web/screens.json")
        elif route not in paths:
            missing.append(f"нет: {screen} — маршрута {route} нет в web/src")
    if missing:
        return Check("screens", "fail", "\n".join(missing))
    return Check("screens", "pass", str(len(screens)))


def check_telegram(tree: Path, routes, sources, runner) -> Check:
    problems = []
    if ("POST", "/auth/telegram") not in routes:
        problems.append("нет маршрута POST /auth/telegram в зависимостях сервера")
    if not any("telegram.Check(" in text for _, text in sources):
        problems.append("вход не вызывает telegram.Check")
    command = None
    if not problems:
        command = run_command(
            runner,
            [
                "go",
                "test",
                "-count=1",
                "-timeout",
                "180s",
                "-v",
                "./internal/telegram/",
                "-run",
                "^TestAU_R12_RejectsWrongHash$",
            ],
            tree / "core",
        )
    return settle("telegram", problems, "", command, "", "go")


def check_pdf(tree: Path, ops, routes, runner) -> Check:
    wanted = {op: (method, path) for op, method, path, _contract in ops}
    problems = []
    for op in PDF_OPS:
        route = wanted.get(op)
        if route is None:
            problems.append(f"нет операции {op} в обязательном списке")
        elif route not in routes:
            problems.append(f"нет маршрута {op} {route[0]} {route[1]}")
    problems.extend(pinned_gotenberg(tree))
    command = None
    if not problems:
        command = run_command(
            runner,
            [
                "go",
                "test",
                "-count=1",
                "-timeout",
                "180s",
                "-v",
                "./internal/pdf/",
                "-run",
                "^(TestWB_R28_S8_SheetPdf|TestWB_R29_S8_TutorPdfOmitsKey)$",
            ],
            tree / "core",
        )
    return settle("pdf", problems, "", command, "", "go")


# D306 и briefs/r1-s7.md: Documents и Validate — интерфейс чтения для тестов.
# Модули сами пишут kind и payload. Сервер не обязан импортировать пакет.
JOURNAL_TESTS = (
    "core/internal/events/events_s7_test.go",
    "core/internal/attempts/events_s7_test.go",
    "core/internal/answercheck/events_s7_test.go",
    "core/internal/lessons/events_s7_test.go",
    "core/internal/plan/events_s7_test.go",
    "core/internal/homework/events_s7_test.go",
    "core/internal/recheck/events_s7_test.go",
    "core/internal/privacy/events_s7_test.go",
    "core/internal/auth/events_s7_test.go",
    "core/internal/e2e/events_s7_test.go",
)


def check_journal(tree: Path) -> Check:
    problems = []
    schema = read_text(tree / "docs/contracts/events.schema.json")
    if "schema_version" not in schema:
        problems.append("нет docs/contracts/events.schema.json")
    for rel in JOURNAL_TESTS:
        text = read_text(tree / rel)
        if text == "":
            problems.append(f"нет {rel}")
            continue
        if "events.Validate" not in text:
            problems.append(f"{rel} не вызывает Validate")
        if rel != JOURNAL_TESTS[0] and "events.Documents" not in text:
            problems.append(f"{rel} не вызывает Documents")
    return settle("journal", problems, "", None, "", "go")


def install_problems(tree: Path) -> tuple[list[str], str]:
    problems = []
    terraform = tree / "deploy" / "terraform"
    tf_files = sorted(terraform.glob("*.tf")) if terraform.is_dir() else []
    tf_text = "\n".join(path.read_text(encoding="utf-8") for path in tf_files)
    if not tf_files:
        problems.append("нет deploy/terraform/*.tf")
    elif "required_version" not in tf_text or not re.search(r"\d+\.\d+\.\d+", tf_text):
        problems.append("Terraform required_version не закреплён")
    if 'resource "' not in tf_text:
        problems.append("в Terraform нет resource")
    lock = terraform / ".terraform.lock.hcl"
    lock_text = read_text(lock)
    if "h1:" not in lock_text:
        problems.append("нет воспроизводимого .terraform.lock.hcl")
    playbook = read_text(tree / "deploy" / "ansible" / "install.yml")
    if "hosts:" not in playbook or (
        "tasks:" not in playbook and "roles:" not in playbook
    ):
        problems.append("нет deploy/ansible/install.yml")
    requirements = read_text(tree / "deploy" / "ansible" / "requirements.yml")
    if not VERSION_PIN.search(requirements):
        problems.append("версии Ansible не закреплены как x.y.z")
    blob = f"{tf_text}\n{lock_text}\n{playbook}\n{requirements}"
    if re.search(r"(?i)\blatest\b", blob):
        problems.append("установка содержит тег latest")
    if (
        literal_secret(tf_text)
        or literal_secret(playbook)
        or literal_secret(requirements)
    ):
        problems.append("в установке литерал секрета")
    test_path = tree / "deploy" / "install_repro_test.py"
    if not test_path.is_file() or not nontrivial_assert(test_path):
        problems.append("нет программного теста воспроизводимой установки")
    return problems, blob + "\n" + read_text(test_path)


def check_install(tree: Path, runner) -> Check:
    problems, blob = install_problems(tree)
    command = None
    if not problems:
        command = run_command(
            runner,
            [
                sys.executable,
                "-m",
                "pytest",
                "-q",
                "-s",
                "deploy/install_repro_test.py",
            ],
            tree,
        )
    return settle("install", problems, blob, command, "install_repro=passed", "pytest")


def observability_problems(tree: Path, sources) -> tuple[list[str], str]:
    problems = []
    base = tree / "deploy" / "observability"
    required = {
        "otel-collector.yaml": "otlp",
        "prometheus.yml": "scrape_configs",
        "loki.yaml": "loki",
        "tempo.yaml": "tempo",
        "alerts.yml": "groups:",
    }
    blob_parts = []
    for name, needle in required.items():
        text = read_text(base / name)
        blob_parts.append(text)
        if needle not in text:
            problems.append(f"нет {needle} в deploy/observability/{name}")
    dashboards = (
        sorted((base / "grafana").glob("*.json")) if (base / "grafana").is_dir() else []
    )
    if not any('"title"' in read_text(path) for path in dashboards):
        problems.append("нет дашборда Grafana")
    evidence = "\n".join(blob_parts + [read_text(path) for path in dashboards])
    for token in ("30d", "14d", "7d"):
        if token not in evidence:
            problems.append(f"нет срока хранения {token}")
    source = "\n".join(text for _, text in sources)
    for route in ("/healthz", "/readyz", "/metrics"):
        if not re.search(rf'(?:["\s]){re.escape(route)}(?:["\s?]|$)', source):
            problems.append(f"нет маршрута {route}")
    if "go.opentelemetry.io/otel" not in source:
        problems.append("сервер не импортирует OpenTelemetry")
    test_path = tree / "deploy" / "observability_test.py"
    if not test_path.is_file() or not nontrivial_assert(test_path):
        problems.append("нет программного теста наблюдаемости")
    return problems, evidence + "\n" + read_text(test_path)


def check_observability(tree: Path, sources, runner) -> Check:
    problems, blob = observability_problems(tree, sources)
    command = None
    if not problems:
        command = run_command(
            runner,
            [
                sys.executable,
                "-m",
                "pytest",
                "-q",
                "-s",
                "deploy/observability_test.py",
            ],
            tree,
        )
    return settle(
        "observability", problems, blob, command, "observability=passed", "pytest"
    )


def check_restore(tree: Path, runner) -> Check:
    script = read_text(tree / "deploy" / "restore-check.sh")
    problems = []
    for needle in (
        "openssl enc -d",
        "postgres:",
        "CONSENT_REPLAY_BIN",
        "JOURNAL_RECALC_BIN",
    ):
        if needle not in script:
            problems.append(f"в restore-check.sh нет шага {needle}")
    if script and literal_secret(script):
        problems.append("в restore-check.sh литерал секрета")
    test_path = tree / "deploy" / "restore_drill_test.py"
    if (
        not test_path.is_file()
        or not nontrivial_assert(test_path)
        or not calls_restore_script(test_path)
    ):
        problems.append(
            "нет программного теста восстановления, вызывающего restore-check.sh"
        )
    blob = script + "\n" + read_text(test_path)
    command = None
    if not problems:
        command = run_command(
            runner,
            [
                sys.executable,
                "-m",
                "pytest",
                "-q",
                "-s",
                "deploy/restore_drill_test.py",
            ],
            tree,
        )
    return settle("restore", problems, blob, command, "restore_drill=passed", "pytest")


def evaluate(
    tree: Path,
    ops,
    screens,
    *,
    packages: set[Path] | None = None,
    runner=default_runner,
) -> list[Check]:
    package_error = ""
    if packages is None:
        packages, package_error = load_packages(tree)
    resolved = {path.resolve() for path in packages} if packages else set()
    sources = package_sources(tree, resolved)
    routes = declared_routes(sources)
    return [
        check_operations(ops, routes, package_error),
        check_screens(tree, screens),
        check_telegram(tree, routes, sources, runner),
        check_pdf(tree, ops, routes, runner),
        check_journal(tree),
        check_install(tree, runner),
        check_observability(tree, sources, runner),
        check_restore(tree, runner),
    ]


def floor_checks(ops, screens) -> list[Check] | None:
    short = []
    if len(ops) < MIN_OPERATIONS:
        short.append(
            Check(
                "operations",
                "fail",
                f"обязательных операций {len(ops)}, нужно минимум {MIN_OPERATIONS}",
            )
        )
    if len(screens) < MIN_SCREENS:
        short.append(
            Check(
                "screens",
                "fail",
                f"обязательных экранов {len(screens)}, нужно минимум {MIN_SCREENS}",
            )
        )
    if not short:
        return None
    names = {item.name for item in short}
    if "operations" not in names:
        short.insert(0, Check("operations", "pass", str(len(ops))))
    if "screens" not in names:
        short.append(Check("screens", "pass", str(len(screens))))
    return short


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) > 1:
        sys.stdout.write(
            "sha: -\nresult: unfinished\nexit: 1\nfirst_failing_check: sha\n"
            "sha: fail\n  нужен один коммит\n"
        )
        return 1
    ref = args[0] if args else "HEAD"
    repo = Path(__file__).resolve().parent.parent
    try:
        sha = resolve_sha(repo, ref)
        ops, screens = load_mandatory(repo)
    except LookupError as exc:
        sys.stdout.write(
            "sha: -\nresult: unfinished\nexit: 1\nfirst_failing_check: sha\n"
            f"sha: fail\n  {exc}\n"
        )
        return 1
    short = floor_checks(ops, screens)
    if short is not None:
        code = exit_code(short)
        sys.stdout.write(format_report("-", short, code))
        return code
    with tempfile.TemporaryDirectory(prefix="release1-done-") as tmp:
        tree = Path(tmp)
        try:
            extract_commit(repo, sha, tree)
        except LookupError as exc:
            sys.stdout.write(
                f"sha: {sha}\nresult: unfinished\nexit: 1\nfirst_failing_check: sha\n"
                f"sha: fail\n  {exc}\n"
            )
            return 1
        checks = evaluate(tree, ops, screens)
    code = exit_code(checks)
    sys.stdout.write(format_report(sha, checks, code))
    return code


if __name__ == "__main__":
    sys.exit(main())
