"""Загрузка артефакта каталога в Postgres: LD-R4…LD-R8, LD-R12, LD-R15.

Импорт ``loader`` выполняется внутри теста. Эти тесты фиксируют интерфейс брифа:

- ``load_artifact(dsn, artifact_path, *, environment) -> LoadResult``;
- ``environment`` — ``staging`` или ``prod``;
- ``LoadResult.catalog_version``, ``LoadResult.new``, ``LoadResult.recheck_ids``;
- отказ — любое исключение с атрибутом ``code``;
- битый архив и пропавший id — ``artifact_invalid``; prod без staging — ``not_in_staging``;
- вызов «новая версия» и «перепроверка» — строки ``catalog.core_call``, не сеть;
- DSN — роль ``loader``: только ``SELECT`` и ``INSERT`` в ``catalog.*``.

Чтение версии N эти тесты делают сами: последняя строка ``catalog.revision``
каждого ``entity_id`` с ``catalog_version_from <= N``.

ВОПРОС: первичный ключ ``catalog.load`` в брифе — один ``catalog_version``, но для
одной базы нужна колонка ``environment``. Тест принимает ключ
``(catalog_version, environment)``, иначе строка prod не добавляется без правки
строки staging, а LD-R4 правку запрещает.
ВОПРОС: ``LoadResult.new`` при успешной загрузке в prod версии, которая уже есть
в staging той же базы, бриф не определяет. Тест это поле там не проверяет.
ВОПРОС: D275 хранит байты сущности как есть, а колонка ``data`` имеет тип jsonb.
jsonb не обязан сохранить ``1.0`` и порядок ключей. Тест сверяет ``content_hash``
с хешем архива и JSON после приведения целых чисел, а не текст jsonb.
ВОПРОС: код отказа для артефакта без id, уже лежащего в базе, бриф отдельно не
называет. Тест требует ``artifact_invalid``, как принятый журнал в памяти.
ВОПРОС: если хеши те же, а номер артефакта на единицу больше головы, бриф
описывает повтор того же артефакта. Этот случай не тестируется.
ВОПРОС: пример loader-08 называет ``ct-math:M14.T07.S1.V003``. В публичном
репозитории тест берёт синтетический ``ct-demo:D1.T01.S1.V003``.
"""

from __future__ import annotations

import io
import json
import secrets
import shutil
import subprocess
import tarfile
import time
from collections.abc import Iterator
from pathlib import Path

import pytest

import catalog_artifact
from conftest import mutate_yaml


SCHEMA = Path(__file__).parent / "fixtures" / "loader" / "schema.sql"
POSTGRES_IMAGE = "postgres:16.15"
TYPE_ID = "ct-demo:D1.T01"
VARIANT_ID = "ct-demo:D1.T01.S1.V003"
DROPPED_ID = "ct-demo:D1.T01.S1.V005"
BLOCK_ID = "ct-demo:D1.B01"

_REVISION_SQL = """
SELECT COALESCE(json_agg(json_build_object(
    'entity_id', entity_id,
    'catalog_version_from', catalog_version_from,
    'content_hash', content_hash,
    'data', data
) ORDER BY entity_id, catalog_version_from), '[]'::json)
FROM catalog.revision
"""

_LOAD_SQL = """
SELECT COALESCE(json_agg(json_build_object(
    'catalog_version', catalog_version,
    'commit', commit,
    'loaded_at', loaded_at,
    'environment', environment
) ORDER BY catalog_version, environment), '[]'::json)
FROM catalog.load
"""

_CALL_SQL = """
SELECT COALESCE(json_agg(json_build_object(
    'id', id,
    'kind', kind,
    'catalog_version', catalog_version,
    'payload', payload
) ORDER BY id), '[]'::json)
FROM catalog.core_call
"""


def load_artifact(dsn: str, artifact_path: Path, *, environment: str):
    """Импорт пакета только в момент вызова теста."""
    import loader

    return loader.load_artifact(dsn, artifact_path, environment=environment)


def normalize(value: object) -> object:
    """Сглаживает число jsonb: ``1.0`` и ``1`` равны, строки и логика — нет."""
    if isinstance(value, bool):
        return value
    if isinstance(value, float) and value.is_integer():
        return int(value)
    if isinstance(value, dict):
        return {key: normalize(item) for key, item in value.items()}
    if isinstance(value, list):
        return [normalize(item) for item in value]
    return value


def psql(dsn: str, sql: str) -> str:
    completed = subprocess.run(
        ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-t", "-A", "-c", sql],
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout.strip()


def fetch_json(dsn: str, sql: str) -> object:
    return json.loads(psql(dsn, sql))


def snapshot(dsn: str) -> dict[str, object]:
    return {
        "revision": fetch_json(dsn, _REVISION_SQL),
        "load": fetch_json(dsn, _LOAD_SQL),
        "core_call": fetch_json(dsn, _CALL_SQL),
    }


def revisions(dsn: str) -> list[dict]:
    rows = fetch_json(dsn, _REVISION_SQL)
    assert isinstance(rows, list)
    return rows


def loads(dsn: str) -> list[dict]:
    rows = fetch_json(dsn, _LOAD_SQL)
    assert isinstance(rows, list)
    return rows


def calls(dsn: str) -> list[dict]:
    rows = fetch_json(dsn, _CALL_SQL)
    assert isinstance(rows, list)
    return rows


def read_version(dsn: str, number: int) -> dict[str, dict]:
    """LD-R7 по строкам таблицы, без отдельной функции загрузчика."""
    chosen: dict[str, dict] = {}
    for row in revisions(dsn):
        if row["catalog_version_from"] > number:
            continue
        current = chosen.get(row["entity_id"])
        if current is None or row["catalog_version_from"] >= current["catalog_version_from"]:
            chosen[row["entity_id"]] = row
    return chosen


def build(root: Path, out: Path, **kwargs):
    return catalog_artifact.build_artifact(
        root,
        ref="main",
        commit=kwargs.get("commit", "commit-1"),
        catalog_version=kwargs.get("catalog_version", 1),
        previous=kwargs.get("previous"),
        out=out,
    )


def replace_once(root: Path, relative: str, old: str, new: str) -> None:
    path = root / relative
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise AssertionError(f"{old!r} не найден в {relative}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")


def read_archive(path: Path) -> dict[str, bytes]:
    with tarfile.open(path, "r:zst") as archive:
        files: dict[str, bytes] = {}
        for member in archive.getmembers():
            if not member.isfile():
                continue
            extracted = archive.extractfile(member)
            if extracted is None:
                raise AssertionError(member.name)
            files[member.name] = extracted.read()
        return files


def write_archive(path: Path, files: dict[str, bytes]) -> None:
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:zst") as archive:
        for name in sorted(files):
            blob = files[name]
            info = tarfile.TarInfo(name)
            info.size = len(blob)
            info.mode = 0o644
            info.mtime = 0
            info.uid = 0
            info.gid = 0
            archive.addfile(info, io.BytesIO(blob))
    path.write_bytes(buffer.getvalue())


def archive_without_entity(
    source: Path,
    dest: Path,
    entity_id: str,
    *,
    catalog_version: int,
    commit: str,
) -> None:
    """Архив той же целостности, но без одного id. Схему каталога он не проходит."""
    files = read_archive(source)
    manifest = json.loads(files["manifest.json"])
    for key in ("active", "retired"):
        manifest[key] = [item for item in manifest[key] if item["id"] != entity_id]
    manifest["catalog_version"] = catalog_version
    manifest["commit"] = commit
    manifest["corrected_keys"] = [
        item for item in manifest["corrected_keys"] if item != entity_id
    ]
    del files[f"entities/{entity_id}.json"]
    files["manifest.json"] = catalog_artifact.canonical_bytes(manifest)
    write_archive(dest, files)


def assert_result(result, *, catalog_version: int, new: bool | None, recheck_ids: list[str]) -> None:
    assert result.catalog_version == catalog_version
    if new is not None:
        assert result.new is new
    assert list(result.recheck_ids) == recheck_ids


def assert_version_call(row: dict, catalog_version: int, commit: str) -> None:
    assert row["kind"] == "version"
    assert row["catalog_version"] == catalog_version
    payload = row["payload"]
    assert isinstance(payload, dict)
    assert set(payload) <= {"catalog_version", "commit"}
    assert payload["catalog_version"] == catalog_version
    if "commit" in payload:
        assert payload["commit"] == commit


def assert_stored_entities(dsn: str, artifact_path: Path, catalog_version: int) -> None:
    artifact = catalog_artifact.verify_artifact(artifact_path)
    rows = [
        row for row in revisions(dsn) if row["catalog_version_from"] == catalog_version
    ]
    assert len(rows) == len(artifact.entities)
    by_id = {row["entity_id"]: row for row in rows}
    assert set(by_id) == {entity.id for entity in artifact.entities}
    for entity in artifact.entities:
        row = by_id[entity.id]
        assert row["content_hash"] == entity.content_hash
        assert normalize(row["data"]) == normalize(json.loads(entity.data))


def expect_refusal(dsn: str, path: Path, environment: str, code: str) -> None:
    before = snapshot(dsn)
    with pytest.raises(Exception) as caught:
        load_artifact(dsn, path, environment=environment)
    error = caught.value
    if isinstance(error, ImportError):
        raise error
    assert getattr(error, "code", None) == code
    assert snapshot(dsn) == before


def _docker(args: list[str], timeout: float) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=False,
        capture_output=True,
        text=True,
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
        except subprocess.CalledProcessError as error:
            last_error = _trim(error.stderr or error.stdout or str(error))
        else:
            return
        if not _container_running(container_id):
            raise AssertionError(f"postgres container exited: {last_error}\n{_container_logs(container_id)}")
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
    """Postgres 16.15, том удаляется после теста. Нет Docker — пропуск."""
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
        admin_dsn = f"postgresql://postgres@127.0.0.1:{port}/ai_mentor_test?sslmode=disable&connect_timeout=2"
        _wait_ready(container_id, admin_dsn)
        subprocess.run(
            ["psql", admin_dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-f", str(SCHEMA)],
            check=True,
            capture_output=True,
            text=True,
        )
        loader_dsn = f"postgresql://loader@127.0.0.1:{port}/ai_mentor_test?sslmode=disable&connect_timeout=2"
        version = psql(loader_dsn, "SHOW server_version")
        if not version.startswith("16.15"):
            raise AssertionError(f"server version {version}")
        yield loader_dsn
    finally:
        _remove_database(container_id, volume)


def test_ld_r6_first_load_inserts_revisions(dsn: str, catalog_root: Path, tmp_path: Path) -> None:
    """LD-R6: первая загрузка пишет ревизию каждого id и один вызов версии."""
    out = tmp_path / "v1.tar.zst"
    artifact = build(catalog_root, out, commit="commit-1", catalog_version=1)
    assert artifact.catalog_version == 1
    result = load_artifact(dsn, out, environment="staging")
    assert_result(result, catalog_version=1, new=True, recheck_ids=[])
    assert_stored_entities(dsn, out, 1)
    assert len(revisions(dsn)) == len(artifact.entities)
    stored_loads = loads(dsn)
    assert [row["catalog_version"] for row in stored_loads] == [1]
    assert stored_loads[0]["commit"] == "commit-1"
    assert stored_loads[0]["environment"] == "staging"
    assert stored_loads[0]["loaded_at"]
    stored_calls = calls(dsn)
    assert len(stored_calls) == 1
    assert_version_call(stored_calls[0], 1, "commit-1")


def test_ld_r8_reload_is_noop(dsn: str, catalog_root: Path, tmp_path: Path) -> None:
    """LD-R8, loader-05: повтор того же артефакта версии 8 ничего не меняет и не вызывает core."""
    out = tmp_path / "v8.tar.zst"
    build(catalog_root, out, commit="commit-8", catalog_version=8)
    first = load_artifact(dsn, out, environment="staging")
    assert_result(first, catalog_version=8, new=True, recheck_ids=[])
    before = snapshot(dsn)
    second = load_artifact(dsn, out, environment="staging")
    assert_result(second, catalog_version=8, new=False, recheck_ids=[])
    assert snapshot(dsn) == before


def test_ld_r5_typo_adds_one_revision(dsn: str, catalog_root: Path, tmp_path: Path) -> None:
    """LD-R5, loader-04: версия 7 и одна опечатка дают версию 8 и одну ревизию."""
    first_out = tmp_path / "v7.tar.zst"
    first = build(catalog_root, first_out, commit="commit-7", catalog_version=7)
    load_artifact(dsn, first_out, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("title", "Сложение: чисо"),
    )
    second_out = tmp_path / "v8.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-8",
        catalog_version=8,
        previous=first,
    )
    before = catalog_artifact.verify_artifact(first_out)
    after = catalog_artifact.verify_artifact(second_out)
    changed = [
        entity.id
        for entity in after.entities
        if entity.content_hash != next(item.content_hash for item in before.entities if item.id == entity.id)
    ]
    assert changed == [TYPE_ID]
    result = load_artifact(dsn, second_out, environment="staging")
    assert_result(result, catalog_version=8, new=True, recheck_ids=[])
    added = [row for row in revisions(dsn) if row["catalog_version_from"] == 8]
    assert [row["entity_id"] for row in added] == [TYPE_ID]
    assert added[0]["content_hash"] == next(entity.content_hash for entity in after.entities if entity.id == TYPE_ID)
    assert [row["catalog_version"] for row in loads(dsn)] == [7, 8]
    assert [row["commit"] for row in loads(dsn)] == ["commit-7", "commit-8"]
    assert {row["environment"] for row in loads(dsn)} == {"staging"}
    version_calls = [row for row in calls(dsn) if row["kind"] == "version"]
    assert [row["catalog_version"] for row in version_calls] == [7, 8]
    assert [row for row in calls(dsn) if row["kind"] == "recheck"] == []


def test_ld_r6_unchanged_hash_adds_no_revision(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R6: у id с прежним хешем новая ревизия не пишется."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    load_artifact(dsn, first_out, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("title", "Сложение: чисо"),
    )
    second_out = tmp_path / "v2.tar.zst"
    build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    load_artifact(dsn, second_out, environment="staging")
    rows = revisions(dsn)
    assert [row["catalog_version_from"] for row in rows if row["entity_id"] == TYPE_ID] == [1, 2]
    assert all(
        row["catalog_version_from"] == 1
        for row in rows
        if row["entity_id"] != TYPE_ID
    )


def test_ld_r4_missing_id_keeps_old_revision(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R4: архив без уже загруженного id отвергается, старая ревизия остаётся."""
    first_out = tmp_path / "v1.tar.zst"
    build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    load_artifact(dsn, first_out, environment="staging")
    assert any(row["entity_id"] == DROPPED_ID for row in revisions(dsn))
    dropped = tmp_path / "v2-dropped.tar.zst"
    archive_without_entity(first_out, dropped, DROPPED_ID, catalog_version=2, commit="commit-2")
    verified = catalog_artifact.verify_artifact(dropped)
    assert DROPPED_ID not in {entity.id for entity in verified.entities}
    expect_refusal(dsn, dropped, "staging", "artifact_invalid")
    kept = [row for row in revisions(dsn) if row["entity_id"] == DROPPED_ID]
    assert [row["catalog_version_from"] for row in kept] == [1]
    assert [row["catalog_version"] for row in loads(dsn)] == [1]


def test_ld_r4_retired_type_keeps_old_revision(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R4, loader-03: старая ревизия типа остаётся, новая помечена retired."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    load_artifact(dsn, first_out, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: (
            doc["types"][0].__setitem__("status", "retired"),
            doc["types"][0].__setitem__("retired_reason", "синтетический вывод"),
        ),
    )
    second_out = tmp_path / "v2.tar.zst"
    build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    result = load_artifact(dsn, second_out, environment="staging")
    assert_result(result, catalog_version=2, new=True, recheck_ids=[])
    type_rows = [row for row in revisions(dsn) if row["entity_id"] == TYPE_ID]
    assert [row["catalog_version_from"] for row in type_rows] == [1, 2]
    assert type_rows[0]["data"].get("status") != "retired"
    assert type_rows[1]["data"]["status"] == "retired"
    assert type_rows[1]["data"]["retired_reason"] == "синтетический вывод"
    assert all(
        row["catalog_version_from"] == 1
        for row in revisions(dsn)
        if row["entity_id"] != TYPE_ID
    )


def test_ld_r7_read_version_returns_revision_at_most_n(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R7, loader-06: версия 6 после версии 8 показывает прежний блок."""
    version_6 = tmp_path / "v6.tar.zst"
    built_6 = build(catalog_root, version_6, commit="commit-6", catalog_version=6)
    block_6 = json.loads(next(entity.data for entity in built_6.entities if entity.id == BLOCK_ID))
    load_artifact(dsn, version_6, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("title", "Сложение: чисо"),
    )
    version_7 = tmp_path / "v7.tar.zst"
    built_7 = build(
        catalog_root,
        version_7,
        commit="commit-7",
        catalog_version=7,
        previous=built_6,
    )
    load_artifact(dsn, version_7, environment="staging")
    replace_once(
        catalog_root,
        "ct-demo/blocks/D1/B01.md",
        "по определению",
        "по определению (версия 8)",
    )
    version_8 = tmp_path / "v8.tar.zst"
    built_8 = build(
        catalog_root,
        version_8,
        commit="commit-8",
        catalog_version=8,
        previous=built_7,
    )
    block_8 = json.loads(next(entity.data for entity in built_8.entities if entity.id == BLOCK_ID))
    assert block_6 != block_8
    load_artifact(dsn, version_8, environment="staging")
    ids_6 = {entity.id for entity in built_6.entities}
    assert set(read_version(dsn, 6)) == ids_6
    assert set(read_version(dsn, 8)) == ids_6
    assert normalize(read_version(dsn, 6)[BLOCK_ID]["data"]) == normalize(block_6)
    assert normalize(read_version(dsn, 7)[BLOCK_ID]["data"]) == normalize(block_6)
    assert normalize(read_version(dsn, 8)[BLOCK_ID]["data"]) == normalize(block_8)
    assert read_version(dsn, 6)[TYPE_ID]["catalog_version_from"] == 6
    assert read_version(dsn, 8)[TYPE_ID]["catalog_version_from"] == 7


def test_ld_r12_corrected_key_records_recheck(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R12, loader-08: ключ 12 меняется на 14, перепроверка несёт correction_id версии."""
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc["variants"][2].__setitem__("key", 12),
    )
    first_out = tmp_path / "v7.tar.zst"
    first = build(catalog_root, first_out, commit="commit-7", catalog_version=7)
    load_artifact(dsn, first_out, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc["variants"][2].__setitem__("key", 14),
    )
    second_out = tmp_path / "v8.tar.zst"
    build(
        catalog_root,
        second_out,
        commit="commit-8",
        catalog_version=8,
        previous=first,
    )
    verified = catalog_artifact.verify_artifact(second_out)
    assert list(verified.corrected_keys) == [VARIANT_ID]
    result = load_artifact(dsn, second_out, environment="staging")
    assert_result(result, catalog_version=8, new=True, recheck_ids=[VARIANT_ID])
    added = [row for row in revisions(dsn) if row["catalog_version_from"] == 8]
    assert [row["entity_id"] for row in added] == [VARIANT_ID]
    assert added[0]["data"]["key"] == 14
    previous = read_version(dsn, 7)[VARIANT_ID]["data"]["key"]
    assert previous == 12
    rechecks = [row for row in calls(dsn) if row["kind"] == "recheck"]
    assert len(rechecks) == 1
    assert rechecks[0]["catalog_version"] == 8
    payload = rechecks[0]["payload"]
    assert set(payload) == {"correction_id", "variants"}
    assert payload["correction_id"] == 8
    assert payload["variants"] == [VARIANT_ID]
    version_calls = [row for row in calls(dsn) if row["kind"] == "version"]
    assert [row["catalog_version"] for row in version_calls] == [7, 8]


def test_ld_r15_prod_without_staging_is_not_in_staging(
    dsn: str, catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R15, loader-10: prod без этой версии в staging — not_in_staging, база прежняя."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    expect_refusal(dsn, first_out, "prod", "not_in_staging")
    assert revisions(dsn) == []
    load_artifact(dsn, first_out, environment="staging")
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("title", "Сложение: чисо"),
    )
    second_out = tmp_path / "v2.tar.zst"
    build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    expect_refusal(dsn, second_out, "prod", "not_in_staging")
    assert [row["catalog_version"] for row in loads(dsn)] == [1]
    assert all(row["environment"] == "staging" for row in loads(dsn))
    assert all(row["catalog_version_from"] == 1 for row in revisions(dsn))


def test_ld_r15_prod_appends_load_row(dsn: str, catalog_root: Path, tmp_path: Path) -> None:
    """LD-R15: версия, уже есть в staging, принимается в prod новой строкой."""
    out = tmp_path / "v1.tar.zst"
    build(catalog_root, out, commit="commit-1", catalog_version=1)
    load_artifact(dsn, out, environment="staging")
    before = snapshot(dsn)
    result = load_artifact(dsn, out, environment="prod")
    assert result.catalog_version == 1
    assert list(result.recheck_ids) == []
    after = snapshot(dsn)
    assert after["revision"] == before["revision"]
    def call_key(row: dict) -> tuple:
        payload = json.dumps(row["payload"], ensure_ascii=False, sort_keys=True)
        return (row["id"], row["kind"], row["catalog_version"], payload)

    before_calls = {call_key(row) for row in before["core_call"]}
    after_calls = {call_key(row) for row in after["core_call"]}
    assert before_calls <= after_calls
    staging = [row for row in after["load"] if row["environment"] == "staging"]
    prod = [row for row in after["load"] if row["environment"] == "prod"]
    assert staging == before["load"]
    assert len(prod) == 1
    assert prod[0]["catalog_version"] == 1
    assert prod[0]["commit"] == "commit-1"
    assert prod[0]["loaded_at"]


def test_artifact_invalid_broken_archive_leaves_database(
    dsn: str, tmp_path: Path
) -> None:
    """Битый архив отвергается до записи: code artifact_invalid, таблицы пустые."""
    broken = tmp_path / "broken.tar.zst"
    broken.write_bytes(b"not-an-archive")
    expect_refusal(dsn, broken, "staging", "artifact_invalid")
    assert snapshot(dsn) == {"revision": [], "load": [], "core_call": []}
