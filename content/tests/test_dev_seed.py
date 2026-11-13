"""Сид локальной базы: два синтетических человека и одна версия демо-каталога.

Проверяется процесс ``tools/dev_seed.py``, не ``tools/dev-up.sh``.
Интерпретатор процесса:
``.venv/bin/python`` в корне репозитория.
Рабочий каталог — корень репозитория. DSN передаётся только через
``DATABASE_URL``. Модуль сида при сборе тестов не импортируется.

Тест сам поднимает отдельный контейнер ``postgres:16.15`` (не ``ai-mentor-dev``),
применяет секции Up из ``migrations/*.sql`` по порядку имён и удаляет том.
Нет Docker или этого образа — ``pytest.skip``. Образ не скачивается.

ВОПРОС: ``core.app_user.status`` обязателен, бриф значение не называет.
Тест конкретную строку статуса не фиксирует, только что повтор её не меняет.
ВОПРОС: ``owner_tutor_id`` репетитора бриф не задаёт. Тест требует NULL:
владелец назван только у ученика.
ВОПРОС: точный текст двух команд входа не задан. Тест требует в stdout
не меньше двух подстрок ``POST /auth/dev``, оба ``telegram_id`` и ту же
строку DSN, что записана в ``DATABASE_URL``.
ВОПРОС: номер ``catalog_version`` и ``commit`` бриф не задаёт. Тест требует
одну строку ``catalog.load`` со средой ``staging``, те же ``content_hash``,
что у сборки ``content/tests/fixtures/demo-catalog``, и один вызов ``version``.
Номер версии не фиксируется.
ВОПРОС: пишет ли сид ``core.student_pii``, бриф не говорит. Тест эти строки
не требует и не запрещает. Любое значение ``telegram_id`` в любой таблице
входит в проверку «только два демо-id», повтор не добавляет и не меняет строк.
ВОПРОС: «не создаёт вторых строк» не говорит, можно ли обновить уже
записанную строку. Тест считает правку существующей строки, включая
``loaded_at``, нарушением идемпотентности.
"""

from __future__ import annotations

import json
import os
import secrets
import shutil
import subprocess
import tempfile
import time
from collections.abc import Iterator
from pathlib import Path

import pytest


REPO = Path(__file__).resolve().parents[2]
DEMO_CATALOG = Path(__file__).parent / "fixtures" / "demo-catalog"
SEED = REPO / "tools" / "dev_seed.py"
INTERPRETER = REPO / ".venv" / "bin" / "python"
MIGRATIONS = REPO / "migrations"
POSTGRES_IMAGE = "postgres:16.15"

TUTOR_ID = "tutor-demo"
STUDENT_ID = "student-demo"
TUTOR_TELEGRAM = "tg-demo-tutor"
STUDENT_TELEGRAM = "tg-demo-student"
ALLOWED_TELEGRAM = frozenset({TUTOR_TELEGRAM, STUDENT_TELEGRAM})

_PG_KEYS = (
    "PGHOST",
    "PGHOSTADDR",
    "PGPORT",
    "PGDATABASE",
    "PGUSER",
    "PGPASSWORD",
    "PGPASSFILE",
    "PGSERVICE",
    "PGSERVICEFILE",
    "PGOPTIONS",
)


def quote_ident(name: str) -> str:
    if not name.replace("_", "").isalnum():
        raise AssertionError(f"небезопасное имя {name!r}")
    return '"' + name.replace('"', '""') + '"'


def up_sql(text: str, source: Path) -> str:
    """Оставляет секцию Up. Down поднимает исключение и в psql выполнился бы."""
    lines = text.splitlines()
    start = None
    for index, line in enumerate(lines):
        if line.strip() == "-- +goose Up":
            start = index + 1
            break
    if start is None:
        raise AssertionError(f"{source} не содержит -- +goose Up")
    end = len(lines)
    for index in range(start, len(lines)):
        if lines[index].strip() == "-- +goose Down":
            end = index
            break
    body = "\n".join(lines[start:end]).strip()
    if not body:
        raise AssertionError(f"{source}: пустая секция Up")
    return body + "\n"


def psql(dsn: str, sql: str) -> str:
    env = os.environ.copy()
    env["PGCLIENTENCODING"] = "UTF8"
    env["PSQL_PAGER"] = "off"
    completed = subprocess.run(
        ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-t", "-A", "-c", sql],
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env,
    )
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise AssertionError(detail or f"psql завершился с кодом {completed.returncode}")
    return completed.stdout.strip()


def fetch_json(dsn: str, sql: str) -> object:
    return json.loads(psql(dsn, sql))


def apply_migrations(dsn: str) -> None:
    files = sorted(MIGRATIONS.glob("*.sql"))
    if not files:
        raise AssertionError(f"нет файлов в {MIGRATIONS}")
    with tempfile.TemporaryDirectory() as directory:
        folder = Path(directory)
        for index, path in enumerate(files):
            target = folder / f"{index:02d}-{path.name}"
            target.write_text(up_sql(path.read_text(encoding="utf-8"), path), encoding="utf-8")
            env = os.environ.copy()
            env["PGCLIENTENCODING"] = "UTF8"
            env["PSQL_PAGER"] = "off"
            completed = subprocess.run(
                ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-f", str(target)],
                check=False,
                capture_output=True,
                text=True,
                encoding="utf-8",
                env=env,
            )
            if completed.returncode != 0:
                detail = (completed.stderr or completed.stdout or "").strip()
                raise AssertionError(f"{path.name}: {detail}")


def run_seed(dsn: str) -> subprocess.CompletedProcess[str]:
    if not SEED.is_file():
        pytest.fail(f"нет файла {SEED}")
    if not INTERPRETER.is_file():
        pytest.fail(f"нет интерпретатора {INTERPRETER}")
    env = os.environ.copy()
    for key in _PG_KEYS:
        env.pop(key, None)
    env["DATABASE_URL"] = dsn
    env["PGCLIENTENCODING"] = "UTF8"
    env["PYTHONUNBUFFERED"] = "1"
    return subprocess.run(
        [str(INTERPRETER), str(SEED)],
        cwd=REPO,
        env=env,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=120,
        check=False,
    )


def assert_seed_ok(result: subprocess.CompletedProcess[str]) -> None:
    assert result.returncode == 0, (
        f"dev_seed.py завершился с кодом {result.returncode}\n"
        f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
    )


def assert_login_text(stdout: str, dsn: str) -> None:
    assert dsn in stdout
    assert stdout.count("POST /auth/dev") >= 2
    assert TUTOR_TELEGRAM in stdout
    assert STUDENT_TELEGRAM in stdout
    assert "password" not in stdout.lower()


def users(dsn: str) -> list[dict]:
    rows = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'id', u.id,
            'role', u.role,
            'owner_tutor_id', u.owner_tutor_id,
            'telegram_id', p.telegram_id
        ) ORDER BY u.id), '[]'::json)
        FROM core.app_user AS u
        JOIN core.user_pii AS p ON p.user_id = u.id
        """,
    )
    assert isinstance(rows, list)
    return rows


def assert_demo_users(dsn: str) -> None:
    rows = users(dsn)
    assert [row["id"] for row in rows] == [STUDENT_ID, TUTOR_ID]
    by_id = {row["id"]: row for row in rows}
    tutor = by_id[TUTOR_ID]
    student = by_id[STUDENT_ID]
    assert tutor["role"] == "tutor"
    assert tutor["telegram_id"] == TUTOR_TELEGRAM
    assert tutor["owner_tutor_id"] is None
    assert student["role"] == "student"
    assert student["telegram_id"] == STUDENT_TELEGRAM
    assert student["owner_tutor_id"] == TUTOR_ID
    assert int(psql(dsn, "SELECT count(*) FROM core.app_user")) == 2
    assert int(psql(dsn, "SELECT count(*) FROM core.user_pii")) == 2


def telegram_locations(dsn: str) -> list[dict]:
    tables = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'schema', table_schema,
            'table', table_name
        ) ORDER BY table_schema, table_name), '[]'::json)
        FROM information_schema.columns
        WHERE column_name = 'telegram_id'
          AND table_schema NOT IN ('pg_catalog', 'information_schema')
        """,
    )
    assert isinstance(tables, list)
    located = []
    for item in tables:
        schema = item["schema"]
        table = item["table"]
        values = fetch_json(
            dsn,
            "SELECT COALESCE(json_agg(telegram_id ORDER BY telegram_id), '[]'::json) "
            f"FROM {quote_ident(schema)}.{quote_ident(table)}",
        )
        assert isinstance(values, list)
        located.append({"schema": schema, "table": table, "values": values})
    return located


def assert_only_demo_telegram_ids(dsn: str) -> None:
    seen: set[str] = set()
    for item in telegram_locations(dsn):
        values = item["values"]
        assert len(values) == len(set(values)), item
        for value in values:
            assert value in ALLOWED_TELEGRAM, item
            seen.add(value)
    assert seen == set(ALLOWED_TELEGRAM)


def assert_one_staging_catalog(dsn: str, tmp_path: Path) -> None:
    import catalog_artifact

    out = tmp_path / "demo-probe.tar.zst"
    artifact = catalog_artifact.build_artifact(
        DEMO_CATALOG,
        ref="main",
        commit="probe-commit",
        catalog_version=1,
        out=out,
    )
    expected = {entity.id: entity.content_hash for entity in artifact.entities}
    revisions = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'entity_id', entity_id,
            'catalog_version_from', catalog_version_from,
            'content_hash', content_hash
        ) ORDER BY entity_id, catalog_version_from), '[]'::json)
        FROM catalog.revision
        """,
    )
    assert isinstance(revisions, list)
    assert revisions, "сид не записал catalog.revision"
    assert len(revisions) == len(expected)
    assert {row["entity_id"]: row["content_hash"] for row in revisions} == expected
    versions = {row["catalog_version_from"] for row in revisions}
    assert len(versions) == 1
    version = next(iter(versions))
    loads = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'catalog_version', catalog_version,
            'environment', environment,
            'commit', commit
        ) ORDER BY catalog_version, environment), '[]'::json)
        FROM catalog.load
        """,
    )
    assert isinstance(loads, list)
    assert len(loads) == 1
    assert loads[0]["environment"] == "staging"
    assert loads[0]["catalog_version"] == version
    calls = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'kind', kind,
            'catalog_version', catalog_version
        ) ORDER BY id), '[]'::json)
        FROM catalog.core_call
        """,
    )
    assert isinstance(calls, list)
    assert calls == [{"kind": "version", "catalog_version": version}]


def table_snapshot(dsn: str) -> dict[str, object]:
    names = fetch_json(
        dsn,
        """
        SELECT COALESCE(json_agg(json_build_object(
            'schema', n.nspname,
            'table', c.relname
        ) ORDER BY n.nspname, c.relname), '[]'::json)
        FROM pg_class AS c
        JOIN pg_namespace AS n ON n.oid = c.relnamespace
        WHERE n.nspname IN ('core', 'catalog')
          AND c.relkind = 'r'
        """,
    )
    assert isinstance(names, list)
    snapshot: dict[str, object] = {}
    for item in names:
        schema = item["schema"]
        table = item["table"]
        qualified = f"{schema}.{table}"
        snapshot[qualified] = fetch_json(
            dsn,
            "SELECT COALESCE(json_agg(q.row ORDER BY q.ord), '[]'::json) "
            "FROM ("
            "SELECT to_json(t) AS row, t.ctid::text AS ord "
            f"FROM {quote_ident(schema)}.{quote_ident(table)} AS t"
            ") AS q",
        )
    return snapshot


def assert_unchanged(before: dict[str, object], after: dict[str, object]) -> None:
    assert list(before) == list(after)
    changed = [name for name in before if before[name] != after[name]]
    assert changed == []


def _docker(args: list[str], timeout: float) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=timeout,
    )


def _trim(text: str) -> str:
    text = text.strip()
    if len(text) > 400:
        return text[:400]
    return text


def _published_port(container_id: str) -> str:
    deadline = time.monotonic() + 15
    last_error = "port is empty"
    while time.monotonic() < deadline:
        result = _docker(["port", container_id, "5432/tcp"], timeout=2)
        if result.returncode == 0:
            for line in result.stdout.splitlines():
                line = line.strip()
                if line.startswith("127.0.0.1:"):
                    return line.removeprefix("127.0.0.1:")
            last_error = result.stdout.strip() or last_error
        else:
            last_error = _trim(result.stderr)
        time.sleep(0.2)
    raise AssertionError(f"docker port: {last_error}")


def _container_running(container_id: str) -> bool:
    result = _docker(["inspect", "-f", "{{.State.Running}}", container_id], timeout=5)
    return result.returncode == 0 and result.stdout.strip() == "true"


def _container_logs(container_id: str) -> str:
    result = _docker(["logs", "--tail", "40", container_id], timeout=5)
    return _trim(result.stdout + result.stderr)


def _wait_ready(container_id: str, admin_dsn: str) -> None:
    deadline = time.monotonic() + 60
    last_error = "not started"
    while time.monotonic() < deadline:
        try:
            psql(admin_dsn, "SELECT 1")
        except AssertionError as error:
            last_error = _trim(str(error))
        else:
            return
        if not _container_running(container_id):
            raise AssertionError(
                f"postgres container exited: {last_error}\n{_container_logs(container_id)}"
            )
        time.sleep(0.3)
    raise AssertionError(f"postgres not ready: {last_error}\n{_container_logs(container_id)}")


def _remove_database(container_id: str | None, volume: str) -> None:
    if container_id:
        _docker(["rm", "-f", container_id], timeout=30)
    deadline = time.monotonic() + 15
    last_error = "volume was not removed"
    while time.monotonic() < deadline:
        removed = _docker(["volume", "rm", volume], timeout=15)
        if removed.returncode == 0 or "No such volume" in removed.stderr:
            return
        last_error = _trim(removed.stderr)
        time.sleep(0.3)
    raise RuntimeError(f"postgres cleanup failed: {last_error}")


@pytest.fixture
def dsn() -> Iterator[str]:
    """Отдельный Postgres 16.15. Том удаляется. Нет Docker — пропуск."""
    if shutil.which("docker") is None:
        pytest.skip("docker unavailable: executable not found")
    info = _docker(["info", "--format", "{{.ServerVersion}}"], timeout=20)
    if info.returncode != 0:
        pytest.skip(f"docker unavailable: {_trim(info.stderr)}")
    image = _docker(["image", "inspect", POSTGRES_IMAGE], timeout=20)
    if image.returncode != 0:
        pytest.skip(f"image {POSTGRES_IMAGE} unavailable")

    name = "ai-mentor-test-" + secrets.token_hex(4)
    volume = name + "-data"
    container_id: str | None = None
    try:
        started = _docker(
            [
                "run",
                "-d",
                "--rm",
                "--pull",
                "never",
                "--name",
                name,
                "-v",
                f"{volume}:/var/lib/postgresql/data",
                "-e",
                "POSTGRES_HOST_AUTH_METHOD=trust",
                "-e",
                "POSTGRES_DB=ai_mentor_test",
                "-p",
                "127.0.0.1::5432",
                POSTGRES_IMAGE,
            ],
            timeout=30,
        )
        container_id = started.stdout.strip() or None
        if started.returncode != 0:
            raise AssertionError(f"docker run: {_trim(started.stderr)}")
        if container_id is None:
            raise AssertionError("docker run returned an empty container id")
        port = _published_port(container_id)
        admin_dsn = f"postgres://postgres@127.0.0.1:{port}/ai_mentor_test?sslmode=disable"
        _wait_ready(container_id, admin_dsn)
        apply_migrations(admin_dsn)
        version = psql(admin_dsn, "SHOW server_version")
        if not version.startswith("16.15"):
            raise AssertionError(f"server version {version}")
        yield admin_dsn
    finally:
        _remove_database(container_id, volume)


def test_dev_seed_demo_people_and_telegram_ids(dsn: str, tmp_path: Path) -> None:
    """Один запуск: двое людей, только их telegram_id, одна версия staging."""
    result = run_seed(dsn)
    assert_seed_ok(result)
    assert_login_text(result.stdout, dsn)
    assert_demo_users(dsn)
    assert_only_demo_telegram_ids(dsn)
    assert_one_staging_catalog(dsn, tmp_path)


def test_dev_seed_rerun_does_not_add_rows(dsn: str, tmp_path: Path) -> None:
    """Повтор не добавляет людей, версию каталога и чужие telegram_id."""
    first = run_seed(dsn)
    assert_seed_ok(first)
    assert_login_text(first.stdout, dsn)
    assert_demo_users(dsn)
    assert_only_demo_telegram_ids(dsn)
    assert_one_staging_catalog(dsn, tmp_path)
    before = table_snapshot(dsn)
    second = run_seed(dsn)
    assert_seed_ok(second)
    assert_login_text(second.stdout, dsn)
    assert_unchanged(before, table_snapshot(dsn))
    assert_demo_users(dsn)
    assert_only_demo_telegram_ids(dsn)
