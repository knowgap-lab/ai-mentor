#!/usr/bin/env python3
"""Проверка одного SHA выпуска 1.

Код 0 — на этом коммите пройдены диагностические проверки и есть отдельные
доказательства этапов 1–5: CI, живой урок через API, каталог, браузер, VM,
нагрузочный пилот и независимые ревью. Файл
docs/handoff/release1-attestation.json лежит вне проверяемого коммита и
ссылается на тот же SHA.

Код 1 — проверка не прошла. Код 2 — непройдены только внешние ожидания,
а остальные проверки уже пройдены. Оба кода значат, что выпуск не закончен.
--quick, частичный зелёный прогон, открытый PR и отчёт «принято» без
доказательств не дают код 0. Ошибка самого приёмщика даёт код 1.

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
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
ATTESTATION_REL = Path("docs/handoff/release1-attestation.json")
ACCEPTANCE_WORD = re.compile(
    r"принято|accepted|all_checks_passed|полностью выполнен",
    re.IGNORECASE,
)
CI_JOBS = (
    "tools/check.sh",
    "tools/check-code.sh",
    "generation",
    "build",
    "storybook",
    "size-limit",
    "e2e",
    "contract-live",
    "docker",
    "chrome",
    "deploy",
)
LESSON_STEPS = (
    "clean_postgres",
    "tutor_creates_lesson",
    "tutor_assigns_homework",
    "student_correct",
    "student_incorrect",
    "tutor_sees_score_and_attempts",
    "foreign_tutor_denied",
    "repeat_keeps_one_attempt",
)
LESSON_SUCCESS = (
    "tutor_creates_lesson",
    "tutor_assigns_homework",
    "student_correct",
    "student_incorrect",
    "tutor_sees_score_and_attempts",
    "repeat_keeps_one_attempt",
)
BROWSER_BEHAVIORS = (
    "roles",
    "keys_hidden",
    "mobile_360",
    "accessibility",
    "csp",
    "reduced_motion",
    "dialogs",
    "toasts",
    "charts",
    "no_console_errors",
    "no_horizontal_scroll",
    "action_size",
    "homework",
    "result",
    "invite",
    "sessions",
)
LOAD_LIMITS = {"read": 500, "answer": 1000, "submit": 1000, "tutor_review": 500}
BLOCKER_CATEGORIES = frozenset(
    {
        "telegram",
        "hypervisor",
        "visual_approval",
        "merge_queue",
        "github_review",
        "cloud_build",
        "disk",
    }
)
PROOF_SECTIONS = ("ci", "lesson", "catalog", "browser", "vm", "load", "reviews")


@dataclass(frozen=True)
class Check:
    name: str
    status: str
    detail: str = ""


def exit_code(checks: list[Check]) -> int:
    waiting = False
    for item in checks:
        if item.status == "pass":
            continue
        if item.status == "waiting":
            waiting = True
            continue
        return 1
    return 2 if waiting else 0


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


def attestation_secret(value, key: str = "") -> bool:
    if isinstance(value, dict):
        return any(attestation_secret(item, name) for name, item in value.items())
    if isinstance(value, list):
        return any(attestation_secret(item, key) for item in value)
    if isinstance(value, str) and re.search(r"(?i)password|token|secret", key):
        return value != "" and not value.startswith(("$", "${", "{{"))
    return False


def section(evidence, name: str):
    if not isinstance(evidence, dict):
        return None
    value = evidence.get(name)
    return value if isinstance(value, dict) else None


def same_sha(value, sha: str) -> bool:
    return (
        isinstance(value, dict)
        and value.get("sha") == sha
        and SHA_RE.fullmatch(sha or "")
    )


def dependency_missing(
    evidence, owner: str, need: tuple[str, ...], sha: str
) -> list[str]:
    body = section(evidence, owner)
    if body is None or body.get("depends_on") != list(need):
        return [f"нет зависимости {', '.join(need)}"]
    missing = []
    for name in need:
        if not same_sha(section(evidence, name), sha):
            missing.append(f"нет зависимости {name}")
    return missing


def status_only(evidence) -> bool:
    if not isinstance(evidence, dict):
        return (
            isinstance(evidence, str) and ACCEPTANCE_WORD.search(evidence) is not None
        )
    blob = json.dumps(evidence, ensure_ascii=False)
    if ACCEPTANCE_WORD.search(blob) is None:
        return False
    for name in PROOF_SECTIONS:
        value = evidence.get(name)
        if not isinstance(value, (dict, list)) or value in ({}, []):
            return True
    return False


def findings_problems(evidence: dict, sha: str) -> list[str]:
    if "open_findings" in evidence and "findings" not in evidence:
        return ["ноль открытых находок не закрывает находку"]
    problems = []
    if evidence.get("open_findings", []) not in ([], None):
        problems.append("есть открытые обязательные находки")
    findings = evidence.get("findings", [])
    if not isinstance(findings, list):
        return ["находки должны быть списком"]
    for item in findings:
        closed = (
            isinstance(item, dict)
            and item.get("state") == "closed"
            and item.get("sha") == sha
            and isinstance(item.get("closure"), str)
            and item.get("closure") != ""
        )
        if not closed:
            problems.append("находка без доказательства закрытия")
    return problems


def check_attestation(evidence, sha: str, evidence_error: str) -> Check:
    if evidence_error:
        return Check("attestation", "fail", evidence_error)
    problems = []
    if not isinstance(evidence, dict):
        problems.append("нет отдельного attestation с доказательствами этого SHA")
    else:
        if not same_sha(evidence, sha) or evidence.get("artifact_sha") != sha:
            problems.append("проверяемый артефакт не совпадает с заявленным SHA")
        if attestation_secret(evidence):
            problems.append("в доказательстве литерал секрета")
        if status_only(evidence):
            problems.append("отчёт «принято» без доказательств")
        problems.extend(findings_problems(evidence, sha))
    if problems:
        return Check("attestation", "fail", "\n".join(problems))
    return Check("attestation", "pass", "")


def check_integration(evidence, sha: str) -> Check:
    problems = []
    if not isinstance(evidence, dict):
        return Check("integration", "fail", "нет доказательства слияния в main")
    pr = evidence.get("pull_request")
    state = pr.get("state") if isinstance(pr, dict) else None
    if state == "open":
        problems.append("открытый PR не является слиянием")
    elif state == "queued":
        problems.append("постановка в очередь не является слиянием")
    elif state != "merged":
        problems.append("нет слияния в main")
    if evidence.get("merged") is not True or evidence.get("integrated_ref") != "main":
        problems.append("SHA не влит в main")
    if not isinstance(pr, dict) or pr.get("merge_sha") != sha:
        problems.append("слияние относится к другому SHA")
    if problems:
        return Check("integration", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("integration", "pass", "")


def check_ci(evidence, sha: str) -> Check:
    ci = section(evidence, "ci")
    if ci is None:
        return Check("ci", "fail", "нет полного CI этого SHA")
    problems = []
    if ci.get("sha") != sha:
        problems.append("CI относится к другому SHA")
    if ci.get("partial") is True:
        problems.append("частичный зелёный прогон")
    found = {}
    checks = ci.get("checks")
    if not isinstance(checks, list):
        problems.append("нет списка CI")
        checks = []
    for item in checks:
        name = item.get("name") if isinstance(item, dict) else None
        if not isinstance(name, str):
            problems.append("запись CI без имени")
            continue
        if name in found:
            problems.append(f"дублированная проверка {name}")
        found[name] = item
    for name in CI_JOBS:
        item = found.get(name)
        if item is None:
            problems.append(f"нет {name}")
            problems.append("частичный зелёный прогон")
            continue
        if item.get("sha") != sha:
            problems.append(f"{name} не привязан к этому SHA")
        conclusion = item.get("conclusion")
        if conclusion == "skipped":
            problems.append(f"пропущен {name}")
        elif conclusion != "success":
            problems.append(f"{name}: {conclusion}")
        if name == "generation" and item.get("diff_empty") is not True:
            problems.append("генерация с непустым diff")
    if problems:
        return Check("ci", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("ci", "pass", "")


def http_problem(name: str, step: dict) -> list[str]:
    problems = []
    status = step.get("http_status")
    if status == 501:
        problems.append(f"{name}: заглушка 501")
    if step.get("content_type") == "text/html":
        problems.append(f"{name}: SPA вместо API")
    if step.get("result") in {"skip", "skipped"}:
        problems.append(f"пропущенный сценарий {name}")
    return problems


def check_lesson(evidence, sha: str) -> Check:
    lesson = section(evidence, "lesson")
    if lesson is None:
        return Check("lesson", "fail", "нет живого урока через API")
    problems = dependency_missing(evidence, "lesson", ("catalog",), sha)
    if lesson.get("sha") != sha:
        problems.append("урок снят с другого SHA")
    if lesson.get("transport") != "live_api":
        problems.append("урок идёт не через живой API")
    steps = lesson.get("steps")
    if not isinstance(steps, dict):
        return Check("lesson", "fail", "\n".join([*problems, "нет шагов урока"]))
    statuses = []
    for name in LESSON_STEPS:
        step = steps.get(name)
        if not isinstance(step, dict):
            problems.append(f"нет шага {name}")
            continue
        if not isinstance(step.get("command"), str) or step.get("command") == "":
            problems.append(f"{name}: нет команды")
        if step.get("result") != "pass":
            problems.append(f"{name}: нет успешного результата")
        problems.extend(http_problem(name, step))
        if "http_status" in step:
            statuses.append(step.get("http_status"))
        if name in LESSON_SUCCESS and step.get("http_status") not in range(200, 300):
            problems.append(f"{name}: нет успешного кода API")
        if name == "foreign_tutor_denied" and step.get("http_status") != 403:
            problems.append("чужому репетитору не отказано")
        if name == "repeat_keeps_one_attempt" and step.get("attempt_count") != 1:
            problems.append("повтор не сохранил одну попытку")
    if statuses and set(statuses) <= {401, 403}:
        problems.append("один набор 401/403 не подтверждает операции")
    if problems:
        return Check("lesson", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("lesson", "pass", "")


def check_catalog(evidence, sha: str) -> Check:
    catalog = section(evidence, "catalog")
    if catalog is None:
        return Check("catalog", "fail", "нет каталога")
    problems = []
    if catalog.get("sha") != sha:
        problems.append("каталог снят с другого SHA")
    files = catalog.get("lesson_files")
    cards = catalog.get("topic_cards")
    if not isinstance(files, list) or not any(
        isinstance(item, str) and item for item in files
    ):
        problems.append("нет синтетического файла урока")
    if not isinstance(cards, list) or not any(
        isinstance(item, str) and item for item in cards
    ):
        problems.append("нет карточки темы")
    if catalog.get("catalog_ci") != "pass":
        problems.append("catalog_ci не пройден")
    if catalog.get("postgres_load") != "pass":
        problems.append("каталог не загружен в Postgres")
    if not catalog.get("command") or not catalog.get("result"):
        problems.append("нет команды или результата каталога")
    if problems:
        return Check("catalog", "fail", "\n".join(problems))
    return Check("catalog", "pass", "")


def check_browser(evidence, sha: str, screens: list[str]) -> Check:
    browser = section(evidence, "browser")
    if browser is None:
        return Check("browser", "fail", "нет браузерной проверки")
    problems = dependency_missing(evidence, "browser", ("lesson", "catalog"), sha)
    if browser.get("sha") != sha:
        problems.append("браузер снят с другого SHA")
    if browser.get("backend") != "real":
        problems.append("браузер смотрит не на настоящий сервер")
    if browser.get("database") != "synthetic":
        problems.append("браузер без синтетической базы")
    if browser.get("screenshots_only") is True:
        problems.append("скриншот не заменяет проверку поведения")
    shown = browser.get("screens")
    if not isinstance(shown, dict):
        problems.append("нет результата по экранам")
        shown = {}
    for screen in screens:
        item = shown.get(screen)
        if isinstance(item, str):
            problems.append(f"{screen}: имя в комментарии не является покрытием")
            continue
        if not isinstance(item, dict):
            problems.append(f"нет поведения экрана {screen}")
            continue
        evidence_kind = item.get("evidence")
        if evidence_kind == "comment":
            problems.append(f"{screen}: имя в комментарии не является покрытием")
        elif evidence_kind == "route":
            problems.append(f"{screen}: регистрация маршрута не является покрытием")
        elif evidence_kind == "screenshot":
            problems.append(f"{screen}: скриншот не заменяет проверку поведения")
        elif evidence_kind != "scenario":
            problems.append(f"{screen}: нет поведенческого сценария")
        if item.get("behavior") in {"skip", "skipped"}:
            problems.append(f"пропущенный сценарий {screen}")
        elif item.get("behavior") != "pass":
            problems.append(f"{screen}: поведение не пройдено")
    behaviors = browser.get("behaviors")
    if not isinstance(behaviors, dict):
        problems.append("нет проверок состояний браузера")
        behaviors = {}
    for name in BROWSER_BEHAVIORS:
        if behaviors.get(name) == "skip":
            problems.append(f"пропущенный сценарий {name}")
        elif behaviors.get(name) != "pass":
            problems.append(f"нет проверки {name}")
    if problems:
        return Check("browser", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("browser", "pass", "")


def check_vm(evidence, sha: str) -> Check:
    vm = section(evidence, "vm")
    if vm is None:
        return Check("vm", "fail", "нет проверки чистой VM")
    problems = dependency_missing(evidence, "vm", ("ci", "browser"), sha)
    waiting = False
    if vm.get("sha") != sha:
        problems.append("VM снята с другого SHA")
    if vm.get("environment") == "cloud_container":
        problems.append("облачный контейнер не является VM")
    elif vm.get("environment") != "clean_ubuntu_vm":
        problems.append("нет чистой Ubuntu VM")
    terraform = vm.get("terraform") if isinstance(vm.get("terraform"), dict) else {}
    ansible = vm.get("ansible") if isinstance(vm.get("ansible"), dict) else {}
    if (
        terraform.get("create") != "pass"
        or terraform.get("second_plan") != "no_changes"
    ):
        if ansible.get("converge") == "pass":
            problems.append("установка на готовую VM не заменяет Terraform")
        else:
            problems.append("Terraform не воспроизвёл чистую VM")
    if (
        ansible.get("converge") != "pass"
        or ansible.get("second_converge") != "no_changes"
    ):
        problems.append("повторный Ansible меняет систему")
    reboot = vm.get("reboot") if isinstance(vm.get("reboot"), dict) else {}
    if (
        reboot.get("services") != "pass"
        or reboot.get("readiness") != "pass"
        or reboot.get("data") != "persisted"
    ):
        problems.append("после перезагрузки данные или готовность не подтверждены")
    if vm.get("background_job") != "pass":
        problems.append("фоновая задача не исполнена")
    pdf = vm.get("pdf") if isinstance(vm.get("pdf"), dict) else {}
    if pdf.get("gotenberg") != "real" or pdf.get("bytes_stable") is not True:
        problems.append("PDF не проверен настоящим Gotenberg")
    telemetry = vm.get("telemetry") if isinstance(vm.get("telemetry"), dict) else {}
    for name in ("logs", "traces", "prometheus", "grafana", "alert_rule"):
        if telemetry.get(name) != "pass":
            problems.append(f"нет телеметрии {name}")
    if telemetry.get("pii") != "absent":
        problems.append("в телеметрии есть личные данные")
    restore = vm.get("restore") if isinstance(vm.get("restore"), dict) else {}
    if (
        restore.get("backup_file_only") is True
        or restore.get("records_compared") is not True
    ):
        problems.append("файл копии не доказывает восстановление")
    if restore.get("separate_environment") is not True:
        problems.append("восстановление не в отдельной среде")
    recorded = vm.get("recorded") if isinstance(vm.get("recorded"), dict) else {}
    for name in ("commands", "versions", "resources", "timings", "results"):
        if recorded.get(name) is not True:
            problems.append(f"не записано: {name}")
    telegram = vm.get("telegram") if isinstance(vm.get("telegram"), dict) else {}
    for name in ("login", "device_confirmation", "notification"):
        value = telegram.get(name)
        if value == "waiting":
            waiting = True
        elif value != "verified":
            problems.append(f"Telegram {name} не проверен")
    if problems:
        return Check("vm", "fail", "\n".join(problems))
    if waiting:
        return Check("vm", "waiting", "настоящий вход Telegram ещё не проверен")
    return Check("vm", "pass", "")


def load_action_problems(actions) -> list[str]:
    problems = []
    if not isinstance(actions, dict):
        return ["нет измерений по типам действий"]
    for name, limit in LOAD_LIMITS.items():
        item = actions.get(name)
        if not isinstance(item, dict):
            problems.append(f"нет измерений {name}")
            continue
        samples = item.get("samples")
        if not isinstance(samples, int) or samples < 100:
            problems.append(f"{name}: меньше 100 измерений")
        successes = item.get("successes")
        if not isinstance(successes, int) or successes < 1:
            problems.append(f"{name}: нет успешных действий")
        p95 = item.get("p95_ms")
        if not isinstance(p95, int) or p95 > limit:
            problems.append(f"{name}: p95 выше {limit} мс")
        for field in (
            "attempts",
            "p50_ms",
            "max_ms",
            "errors",
            "throughput",
            "active_users",
            "queue_ms",
        ):
            if field not in item:
                problems.append(f"{name}: нет {field}")
    return problems


def check_load(evidence, sha: str) -> Check:
    load = section(evidence, "load")
    if load is None:
        return Check("load", "fail", "нет нагрузочного пилота")
    problems = dependency_missing(evidence, "load", ("vm",), sha)
    if load.get("sha") != sha:
        problems.append("пилот снят с другого SHA")
    if load.get("students") != 20 or load.get("tutors") != 2:
        problems.append("пилот не на 20 учениках и 2 репетиторах")
    profile = load.get("profile") if isinstance(load.get("profile"), dict) else {}
    if (
        profile.get("vcpu") != 4
        or profile.get("ram_gib") != 8
        or profile.get("observability") is not True
    ):
        problems.append("профиль не 4 vCPU / 8 GiB со наблюдаемостью")
    if load.get("virtualization") in {None, "", "cloud_container", "cursor_cloud"}:
        problems.append("нагрузка снята не с VM")
    for field in ("ubuntu", "browser", "viewport", "seed"):
        if not load.get(field):
            problems.append(f"не зафиксировано: {field}")
    warmup = load.get("warmup_seconds")
    measure = load.get("measure_seconds")
    if (
        not isinstance(warmup, int)
        or warmup < 300
        or not isinstance(measure, int)
        or measure < 1200
    ):
        problems.append("прогон короче 5 минут разогрева и 20 минут измерений")
    if (
        not isinstance(load.get("browser_repeats"), int)
        or load.get("browser_repeats") < 5
    ):
        problems.append("меньше 5 повторов браузерного сценария")
    mix = load.get("mix") if isinstance(load.get("mix"), dict) else {}
    if (
        mix.get("read") != 0.6
        or mix.get("answer_or_submit") != 0.3
        or mix.get("tutor") != 0.1
    ):
        problems.append("смесь нагрузки не 60/30/10")
    think = (
        load.get("think_seconds") if isinstance(load.get("think_seconds"), dict) else {}
    )
    low = think.get("min")
    high = think.get("max")
    if (
        not isinstance(low, int)
        or not isinstance(high, int)
        or low < 3
        or high > 10
        or low > high
    ):
        problems.append("пауза между действиями не 3–10 секунд")
    if load.get("auth_failures_only") is True:
        problems.append("зелёный прогон из одних отказов авторизации")
    if load.get("unexpected_5xx") != 0:
        problems.append("неожиданные 5xx")
    if load.get("lost_confirmed_answers") != 0:
        problems.append("потеряны подтверждённые ответы")
    if load.get("pdf_worker_separate") is not True:
        problems.append("PDF смешан с профилем API")
    reviewers = load.get("reviewers")
    if (
        not isinstance(reviewers, list)
        or len(set(reviewers)) < 2
        or any(not isinstance(item, str) or item == "" for item in reviewers)
    ):
        problems.append("профиль пилота не подтвердили два независимых проверяющих")
    problems.extend(load_action_problems(load.get("actions")))
    if problems:
        return Check("load", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("load", "pass", "")


def check_reviews(evidence, sha: str) -> Check:
    if not isinstance(evidence, dict):
        return Check("reviews", "fail", "нет независимых ревью")
    reviews = evidence.get("reviews")
    if not isinstance(reviews, list) or len(reviews) < 2:
        return Check("reviews", "fail", "нужны два независимых ревью этого SHA")
    problems = []
    reviewers = []
    for item in reviews:
        if not isinstance(item, dict):
            problems.append("ревью без проверяющего")
            continue
        reviewer = item.get("reviewer")
        author = item.get("author")
        if not isinstance(reviewer, str) or reviewer == "" or reviewer == author:
            problems.append("ревьюер не независим от автора")
        reviewers.append(reviewer)
        if item.get("sha") != sha or item.get("stale") is True:
            problems.append("старое ревью")
        if item.get("state") == "approve-with-conflicts":
            problems.append("approve-with-conflicts не означает готовность")
        elif item.get("state") != "approved":
            problems.append("ревью не approved")
        requirements = item.get("requirements")
        if not isinstance(requirements, list) or not requirements:
            problems.append("в ревью нет списка требований")
        if item.get("comments_closed") is not True:
            problems.append("замечания ревью не закрыты")
    if len(set(reviewers)) < 2:
        problems.append("два ревью от одного проверяющего")
    if problems:
        return Check("reviews", "fail", "\n".join(dict.fromkeys(problems)))
    return Check("reviews", "pass", "")


def check_blockers(evidence) -> Check:
    if not isinstance(evidence, dict):
        return Check("blockers", "fail", "нет реестра блокеров")
    raw = evidence.get("blockers", [])
    if raw in (None, []):
        return Check("blockers", "pass", "")
    if not isinstance(raw, list):
        return Check("blockers", "fail", "неверный или дублированный блокер")
    seen = set()
    problems = []
    for item in raw:
        if not isinstance(item, dict):
            problems.append("неверный или дублированный блокер")
            continue
        ident = item.get("id")
        if (
            not isinstance(ident, str)
            or ident == ""
            or ident in seen
            or item.get("category") not in BLOCKER_CATEGORIES
            or item.get("status") != "waiting"
            or not item.get("dependency")
        ):
            problems.append("неверный или дублированный блокер")
        else:
            seen.add(ident)
    if problems:
        return Check("blockers", "fail", "\n".join(dict.fromkeys(problems)))
    return Check(
        "blockers",
        "waiting",
        "активный блокер при зелёных проверках не завершает выпуск",
    )


def proof_checks(
    evidence, sha: str, screens: list[str], evidence_error: str
) -> list[Check]:
    return [
        check_attestation(evidence, sha, evidence_error),
        check_integration(evidence, sha),
        check_ci(evidence, sha),
        check_catalog(evidence, sha),
        check_lesson(evidence, sha),
        check_browser(evidence, sha, screens),
        check_vm(evidence, sha),
        check_load(evidence, sha),
        check_reviews(evidence, sha),
        check_blockers(evidence),
    ]


def load_attestation(repo: Path, tree: Path) -> tuple[dict | None, str]:
    path = repo / ATTESTATION_REL
    if not path.is_file():
        return None, f"нет отдельного {ATTESTATION_REL.as_posix()}"
    text = path.read_text(encoding="utf-8")
    inside = tree / ATTESTATION_REL
    if inside.is_file() and inside.read_text(encoding="utf-8") == text:
        return None, "отчёт лежит внутри проверяемого SHA и не отделён от артефакта"
    try:
        data = json.loads(text)
    except json.JSONDecodeError as exc:
        return None, f"attestation не JSON: {exc}"
    if not isinstance(data, dict):
        return None, "attestation должен быть объектом"
    return data, ""


def collect_checks(thunk) -> list[Check]:
    try:
        return thunk()
    except Exception as exc:  # noqa: BLE001 — сбой приёмщика не является успехом
        return [Check("checker", "fail", f"ошибка приёмщика: {exc}")]


def evaluate(
    tree: Path,
    ops,
    screens,
    *,
    packages: set[Path] | None = None,
    runner=default_runner,
    evidence=None,
    sha: str = "",
    evidence_error: str = "",
    quick: bool = False,
) -> list[Check]:
    package_error = ""
    if packages is None:
        packages, package_error = load_packages(tree)
    resolved = {path.resolve() for path in packages} if packages else set()
    sources = package_sources(tree, resolved)
    routes = declared_routes(sources)
    checks = [
        check_operations(ops, routes, package_error),
        check_screens(tree, screens),
        check_telegram(tree, routes, sources, runner),
        check_pdf(tree, ops, routes, runner),
        check_journal(tree, runner),
        check_install(tree, runner),
        check_observability(tree, sources, runner),
        *proof_checks(evidence, sha, screens, evidence_error),
    ]
    if quick:
        checks.insert(0, Check("quick", "fail", "--quick не подтверждает завершение"))
    return checks


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
    if "--quick" in args:
        sys.stdout.write(
            "sha: -\nresult: unfinished\nexit: 1\nfirst_failing_check: quick\n"
            "quick: fail\n  --quick не подтверждает завершение\n"
        )
        return 1
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
        evidence, evidence_error = load_attestation(repo, tree)
        checks = collect_checks(
            lambda: evaluate(
                tree,
                ops,
                screens,
                evidence=evidence,
                sha=sha,
                evidence_error=evidence_error,
            )
        )
    code = exit_code(checks)
    sys.stdout.write(format_report(sha, checks, code))
    return code


if __name__ == "__main__":
    sys.exit(main())
