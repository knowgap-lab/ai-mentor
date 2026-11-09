"""Загрузка артефакта каталога в PostgreSQL.

Сеть к core не ходит: «новая версия» и «перепроверка» — строки
``catalog.core_call``. Роль загрузчика только читает и добавляет строки.
``content_hash`` копируется из архива и не считается заново по jsonb.
"""

from __future__ import annotations

import json
import os
import secrets
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import catalog_artifact


class LoadRefused(Exception):
    """Отказ до или вместо записи. ``code`` читают тесты."""

    def __init__(self, code: str) -> None:
        self.code = code
        super().__init__(code)


@dataclass(frozen=True, slots=True)
class LoadResult:
    """Итог одной загрузки в указанную среду."""

    catalog_version: int
    new: bool
    recheck_ids: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class _Revision:
    entity_id: str
    catalog_version_from: int
    content_hash: str
    data: Any


@dataclass(frozen=True, slots=True)
class _LoadRow:
    catalog_version: int
    environment: str
    commit: str


@dataclass(frozen=True, slots=True)
class _Write:
    """Что добавить одной транзакцией. Пустые поля не пишутся."""

    entities: tuple[catalog_artifact.Entity, ...] = ()
    record_load: bool = False
    notify: bool = False


_ENVIRONMENTS = ("staging", "prod")
_MISSING = object()

_REVISION_SQL = """
SELECT COALESCE(json_agg(json_build_object(
    'entity_id', entity_id,
    'catalog_version_from', catalog_version_from,
    'content_hash', content_hash,
    'data', data
)), '[]'::json)
FROM catalog.revision
"""

_LOAD_SQL = """
SELECT COALESCE(json_agg(json_build_object(
    'catalog_version', catalog_version,
    'environment', environment,
    'commit', commit
)), '[]'::json)
FROM catalog.load
"""


def load_artifact(dsn: str, artifact_path: Path, *, environment: str) -> LoadResult:
    """Загрузить проверенный артефакт в ``staging`` или ``prod``."""

    if environment not in _ENVIRONMENTS:
        raise LoadRefused("artifact_invalid")

    artifact = catalog_artifact.verify_artifact(Path(artifact_path))
    revisions = _read_revisions(dsn)
    loads = _read_loads(dsn)
    result, write = _plan(artifact, environment, revisions, loads)
    if write is not None:
        _apply(dsn, artifact, environment, result.recheck_ids, write)
    return result


def _plan(
    artifact: catalog_artifact.Artifact,
    environment: str,
    revisions: list[_Revision],
    loads: list[_LoadRow],
) -> tuple[LoadResult, _Write | None]:
    present = {entity.id for entity in artifact.entities}
    if any(revision.entity_id not in present for revision in revisions):
        raise LoadRefused("artifact_invalid")

    staged = {row.catalog_version for row in loads if row.environment == "staging"}
    if environment == "prod" and artifact.catalog_version not in staged:
        raise LoadRefused("not_in_staging")

    latest = _latest(revisions)
    changed = tuple(
        entity
        for entity in artifact.entities
        if entity.id not in latest or latest[entity.id].content_hash != entity.content_hash
    )
    head = _head(loads)
    already = _loaded(loads, artifact.catalog_version, environment)

    if not changed:
        # Повтор тех же хешей. Голова и голова плюс 1 не добавляют ревизий.
        if head is None or artifact.catalog_version not in (head, head + 1):
            raise LoadRefused("artifact_invalid")
        if already:
            return _result(artifact.catalog_version, new=False), None
        if artifact.catalog_version == head + 1:
            # Номер не записывается и не растёт: в ответе остаётся голова.
            return _result(head, new=False), None
        return (
            _result(artifact.catalog_version, new=True),
            _Write(record_load=True),
        )

    if head is None:
        if type(artifact.catalog_version) is not int or artifact.catalog_version < 1:
            raise LoadRefused("artifact_invalid")
    elif artifact.catalog_version != head + 1:
        raise LoadRefused("artifact_invalid")

    recheck = _recheck_ids(latest, artifact)
    return (
        _result(artifact.catalog_version, new=not already, recheck_ids=recheck),
        _Write(entities=changed, record_load=True, notify=True),
    )


def _result(
    catalog_version: int,
    *,
    new: bool,
    recheck_ids: list[str] | tuple[str, ...] = (),
) -> LoadResult:
    return LoadResult(
        catalog_version=catalog_version,
        new=new,
        recheck_ids=tuple(recheck_ids),
    )


def _latest(revisions: list[_Revision]) -> dict[str, _Revision]:
    latest: dict[str, _Revision] = {}
    for revision in revisions:
        current = latest.get(revision.entity_id)
        if current is None or revision.catalog_version_from >= current.catalog_version_from:
            latest[revision.entity_id] = revision
    return latest


def _head(loads: list[_LoadRow]) -> int | None:
    if not loads:
        return None
    return max(row.catalog_version for row in loads)


def _loaded(loads: list[_LoadRow], catalog_version: int, environment: str) -> bool:
    return any(
        row.catalog_version == catalog_version and row.environment == environment
        for row in loads
    )


def _recheck_ids(
    latest: dict[str, _Revision],
    artifact: catalog_artifact.Artifact,
) -> list[str]:
    found = [
        entity.id
        for entity in artifact.entities
        if entity.id in latest and _key(latest[entity.id].data) != _key(json.loads(entity.data))
    ]
    found.sort()
    return found


def _key(payload: Any) -> Any:
    if not isinstance(payload, dict):
        return _MISSING
    return payload.get("key", _MISSING)


def _apply(
    dsn: str,
    artifact: catalog_artifact.Artifact,
    environment: str,
    recheck_ids: tuple[str, ...],
    write: _Write,
) -> None:
    statements: list[str] = []
    version = artifact.catalog_version
    if type(version) is not int:
        raise LoadRefused("artifact_invalid")
    for entity in sorted(write.entities, key=lambda item: item.id):
        # Хеш и байты — из архива. ::jsonb только разбирает JSON для колонки.
        statements.append(
            "INSERT INTO catalog.revision "
            "(entity_id, catalog_version_from, content_hash, data) VALUES ("
            f"{_dollar(entity.id)}, {version}, {_dollar(entity.content_hash)}, "
            f"{_dollar(entity.data.decode('utf-8'))}::jsonb)"
        )
    if write.record_load:
        statements.append(
            "INSERT INTO catalog.load (catalog_version, commit, loaded_at, environment) "
            f"VALUES ({version}, {_dollar(artifact.commit)}, clock_timestamp(), "
            f"{_sql_environment(environment)})"
        )
    if write.notify:
        version_payload = json.dumps(
            {"catalog_version": version, "commit": artifact.commit},
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        )
        statements.append(
            "INSERT INTO catalog.core_call (kind, catalog_version, payload) VALUES ("
            f"'version', {version}, {_dollar(version_payload)}::jsonb)"
        )
        if recheck_ids:
            recheck_payload = json.dumps(
                {"correction_id": version, "variants": list(recheck_ids)},
                ensure_ascii=False,
                sort_keys=True,
                separators=(",", ":"),
            )
            statements.append(
                "INSERT INTO catalog.core_call (kind, catalog_version, payload) VALUES ("
                f"'recheck', {version}, {_dollar(recheck_payload)}::jsonb)"
            )
    if not statements:
        return
    script = "BEGIN;\n" + ";\n".join(statements) + ";\nCOMMIT;\n"
    try:
        _psql(dsn, script)
    except RuntimeError as error:
        raise LoadRefused("load_failed") from error


def _read_revisions(dsn: str) -> list[_Revision]:
    rows = _fetch_json(dsn, _REVISION_SQL)
    revisions: list[_Revision] = []
    for row in rows:
        revisions.append(
            _Revision(
                entity_id=row["entity_id"],
                catalog_version_from=row["catalog_version_from"],
                content_hash=row["content_hash"],
                data=row["data"],
            )
        )
    return revisions


def _read_loads(dsn: str) -> list[_LoadRow]:
    rows = _fetch_json(dsn, _LOAD_SQL)
    return [
        _LoadRow(
            catalog_version=row["catalog_version"],
            environment=row["environment"],
            commit=row["commit"],
        )
        for row in rows
    ]


def _fetch_json(dsn: str, sql: str) -> list[dict[str, Any]]:
    raw = _psql(dsn, sql, tuples_only=True)
    if raw == "":
        return []
    parsed = json.loads(raw)
    if not isinstance(parsed, list):
        raise RuntimeError("psql вернул не список")
    return parsed


def _psql(dsn: str, sql: str, *, tuples_only: bool = False) -> str:
    args = ["psql", dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q"]
    if tuples_only:
        args.extend(["-t", "-A"])
    env = os.environ.copy()
    env["PGCLIENTENCODING"] = "UTF8"
    env["PSQL_PAGER"] = "off"
    completed = subprocess.run(
        args,
        input=sql,
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env,
        check=False,
    )
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise RuntimeError(detail or f"psql завершился с кодом {completed.returncode}")
    return completed.stdout.strip()


def _dollar(value: str) -> str:
    tag = "j" + secrets.token_hex(8)
    while f"${tag}$" in value:
        tag = "j" + secrets.token_hex(8)
    return f"${tag}${value}${tag}$"


def _sql_environment(environment: str) -> str:
    if environment == "staging":
        return "'staging'"
    if environment == "prod":
        return "'prod'"
    raise LoadRefused("artifact_invalid")


__all__ = ["LoadRefused", "LoadResult", "load_artifact"]
