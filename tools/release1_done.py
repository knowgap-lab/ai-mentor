#!/usr/bin/env python3
"""Проверка одного SHA выпуска 1.

Код 0 — на этом коммите сразу все обязательные операции и экраны,
проверенный вход через Telegram, PDF, журнал, установка по ops.md
(Compose, backup.sh, restore-check.sh) и наблюдаемость OP-R15…OP-R18.
Код 1 — проверка не прошла. Код 2 — дочерний процесс завершился с кодом 2
или в доказательстве написано, что ждём человека. Оба кода значат, что
выпуск не закончен. Скрипт не объявляет выпуск готовым.

Список операций и экранов берётся из контрактов и web.md рядом с этим
скриптом (кроме putRtImport, как в tools/release1_inventory.py). Код
читается только из дерева указанного коммита.
"""

from __future__ import annotations

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
PDF_TESTS = ("TestWB_R28_S8_SheetPdf", "TestWB_R29_S8_TutorPdfOmitsKey")
WAITING_RE = re.compile(
    r"waiting_on_user|ждёт человека|ждет человека|^status:\s*waiting\b",
    re.MULTILINE,
)
PINNED_IMAGE = re.compile(r":(\d+\.\d+\.\d+)$")
SECRET_ASSIGN = re.compile(r"(?i)([A-Za-z0-9_]*(?:password|token))\s*[:=]\s*(\S+)")
DEV_MODE = re.compile(r"(?i)\bAUTH_DEV_MODE\b\s*[:=]\s*([^\s#]+)")
MIGRATION = re.compile(r"(?i)migrat|миграц")
PII_IN_LOG = re.compile(
    r"(?i)(telegram_id|birth_date|date_of_birth|first_name|display_name|дату рождения)"
)
COMPOSE_NAMES = ("compose.prod.yml", "compose.staging.yml")
COMPOSE_SERVICES = ("web", "core", "content", "db")
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
            if value.startswith(("${", "$(", "$", "{{", "?", "env:", "var.", "local.")):
                continue
            if value.lower() in {"false", "true", "null"}:
                continue
            return True
    stripped = SHA256_DIGEST.sub("", text)
    return HEX_RUN.search(stripped) is not None


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


def command_problem(
    kind: str, code: int, out: str, marker: str, required: tuple[str, ...] = ()
) -> str | None:
    if code == 2 or WAITING_RE.search(out):
        return "waiting"
    if code != 0:
        tail = out.strip().splitlines()
        extra = tail[-1] if tail else ""
        return f"команда завершилась с кодом {code} {extra}".strip()
    if kind == "go":
        missing = [name for name in required if f"--- PASS: {name}" not in out]
        if "--- SKIP:" in out or missing or (not required and "--- PASS:" not in out):
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
            if missing:
                extra = (
                    (extra + "; " if extra else "") + "нет PASS: " + ", ".join(missing)
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
    required: tuple[str, ...] = (),
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
        problem = command_problem(kind, command[0], out, marker, required)
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
    return settle("pdf", problems, "", command, "", "go", PDF_TESTS)


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


JOURNAL_VALIDATE_TESTS = (
    "TestS7_ValidateAcceptsEachType",
    "TestS7_ValidateRejectsMissingSchemaVersion",
    "TestS7_ValidateRejectsUnknownEventType",
    "TestS7_ValidateRejectsForeignPayload",
    "TestS7_ValidateRejectsExtraField",
)


def check_journal(tree: Path, runner) -> Check:
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
    package = tree / "core/internal/events"
    bodies = ""
    if package.is_dir():
        bodies = "\n".join(
            path.read_text(encoding="utf-8")
            for path in package.glob("*.go")
            if not path.name.endswith("_test.go")
        )
    if "func Validate(" not in bodies or "func Documents(" not in bodies:
        problems.append("в core/internal/events нет Documents и Validate")
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
                "./internal/events/",
                "-run",
                "^TestS7_Validate",
            ],
            tree / "core",
        )
    return settle("journal", problems, "", command, "", "go", JOURNAL_VALIDATE_TESTS)


def compose_problems(name: str, text: str) -> list[str]:
    problems = []
    if text == "":
        return [f"нет deploy/{name}"]
    try:
        document = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        return [f"{name}: не YAML ({exc})"]
    services = document.get("services") if isinstance(document, dict) else None
    if not isinstance(services, dict):
        return [f"{name}: нет services"]
    missing = [service for service in COMPOSE_SERVICES if service not in services]
    if missing:
        problems.append(f"{name}: нет сервисов {', '.join(missing)}")
    if re.search(r"(?i)\blatest\b", text):
        problems.append(f"{name}: тег latest")
    if literal_secret(text):
        problems.append(f"{name}: литерал секрета")
    values = [item.strip("'\"").casefold() for item in DEV_MODE.findall(text)]
    if not values or any(item not in {"false", "0"} for item in values):
        problems.append(f"{name}: AUTH_DEV_MODE не выключен")
    mention = MIGRATION.search(text)
    if mention is None:
        problems.append(f"{name}: миграция не упомянута")
    else:
        for service in ("core", "web"):
            header = re.search(
                rf"(?m)^[ \t]+{re.escape(service)}:[ \t]*(?:#.*)?$", text
            )
            if header is None:
                problems.append(f"{name}: нет сервиса {service}")
            elif mention.start() > header.start():
                problems.append(f"{name}: миграция после сервиса {service}")
    return problems


def install_problems(tree: Path) -> tuple[list[str], str]:
    problems = []
    blobs = []
    for name in COMPOSE_NAMES:
        text = read_text(tree / "deploy" / name)
        blobs.append(text)
        problems.extend(compose_problems(name, text))
    backup = read_text(tree / "deploy" / "backup.sh")
    blobs.append(backup)
    if backup == "":
        problems.append("нет deploy/backup.sh")
    else:
        if "docker compose up" in backup:
            problems.append("backup.sh запускает docker compose up")
        if "openssl enc" not in backup:
            problems.append("backup.sh не шифрует копию")
        if literal_secret(backup):
            problems.append("backup.sh: литерал секрета")
    restore = read_text(tree / "deploy" / "restore-check.sh")
    blobs.append(restore)
    for needle in (
        "openssl enc -d",
        "postgres:",
        "CONSENT_REPLAY_BIN",
        "JOURNAL_RECALC_BIN",
    ):
        if needle not in restore:
            problems.append(f"в restore-check.sh нет шага {needle}")
    if restore and literal_secret(restore):
        problems.append("restore-check.sh: литерал секрета")
    return problems, "\n".join(blobs)


def check_install(tree: Path, _runner) -> Check:
    problems, blob = install_problems(tree)
    return settle("install", problems, blob, None, "", "pytest")


def alert_text(tree: Path, sources) -> str:
    chunks = []
    for _path, text in sources:
        if "OPS_CHAT_ID" in text or "ops_alert" in text:
            chunks.append(text)
    deploy = tree / "deploy"
    if deploy.is_dir():
        for path in sorted(deploy.rglob("*")):
            if path.is_file() and path.suffix in {".sh", ".yml", ".yaml"}:
                chunks.append(path.read_text(encoding="utf-8"))
    return "\n".join(chunks)


def observability_problems(tree: Path, sources) -> tuple[list[str], str]:
    """OP-R15…OP-R18. Стек метрик и чужие сроки хранения spec не называет."""
    problems = []
    go_text = "\n".join(text for _, text in sources)
    log_lines = [
        line for line in go_text.splitlines() if re.search(r"\b(?:log|slog)\.", line)
    ]
    if "slog." not in go_text:
        problems.append("OP-R15: журналы не структурированы")
    if any(PII_IN_LOG.search(line) for line in log_lines):
        problems.append("OP-R15: в журнале есть личные данные")
    alerts = alert_text(tree, sources)
    rules = (
        ("OP-R16: нет тревоги в служебный чат", r"OPS_CHAT_ID|ops_alert"),
        ("OP-R16: нет тревоги о диске", r"80\s*%|диск\s*>\s*80"),
        (
            "OP-R16: нет тревоги о копии или восстановлении",
            r"копия не создана|проверка восстановления не прошла",
        ),
        (
            "OP-R16: нет тревоги о загрузке каталога",
            r"загрузк\w* каталога|catalog load",
        ),
        ("OP-R16: нет тревоги о перепроверке", r"перепроверк"),
        ("OP-R16: нет тревоги о задании failed", r"задани\w*.*failed|job.*failed"),
        ("OP-R16: нет тревоги «Это не я»", r"Это не я"),
    )
    for message, pattern in rules:
        if re.search(pattern, alerts, re.IGNORECASE) is None:
            problems.append(message)
    deploy = tree / "deploy"
    deploy_text = alerts
    minute = re.search(r"минут|\* \* \* \* \*|per minute", deploy_text, re.IGNORECASE)
    three = re.search(
        r"3\s*(?:подряд|неудач)|three failures", deploy_text, re.IGNORECASE
    )
    if "/status" not in deploy_text or minute is None or three is None:
        problems.append("OP-R17: нет внешней проверки страницы состояния")
    kept = False
    if deploy.is_dir():
        for path in deploy.rglob("*"):
            if not path.is_file() or path.name in {"backup.sh", "restore-check.sh"}:
                continue
            if path.suffix not in {".sh", ".yml", ".yaml"}:
                continue
            text = path.read_text(encoding="utf-8")
            if (
                re.search(r"(?i)журнал|service log", text)
                and "30" in text
                and re.search(r"(?i)удал|delete", text)
            ):
                kept = True
    if not kept:
        problems.append("OP-R18: журналы сервисов не удаляются через 30 дней")
    return problems, alerts


def check_observability(tree: Path, sources, _runner) -> Check:
    problems, blob = observability_problems(tree, sources)
    return settle("observability", problems, blob, None, "", "pytest")


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
        check_journal(tree, runner),
        check_install(tree, runner),
        check_observability(tree, sources, runner),
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
