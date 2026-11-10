"""Артефакт версии каталога: сборка, проверка целостности, журнал ревизий.

Подпись minisign не ставится и не проверяется. Журнал ревизий живёт в памяти
этого процесса: PostgreSQL и сеть не используются.
"""

from __future__ import annotations

import copy
import hashlib
import io
import json
import tarfile
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from catalog_ci import check_catalog
from catalog_ci.load import load_documents


class BuildRefused(Exception):
    """Сборка разрешена только для ref ``main`` (LD-R1)."""


class ArtifactInvalid(Exception):
    """Архив или каталог нельзя принять. Код всегда ``artifact_invalid``."""

    code = "artifact_invalid"

    def __init__(self) -> None:
        super().__init__(self.code)


class SignatureInvalid(Exception):
    """Проверка подписи вернула отказ. Код всегда ``signature_invalid``."""

    code = "signature_invalid"

    def __init__(self) -> None:
        super().__init__(self.code)


@dataclass(frozen=True, slots=True)
class Revision:
    """Одна добавленная ревизия сущности (LD-R6)."""

    id: str
    catalog_version_from: int
    content_hash: str
    data: bytes


@dataclass(frozen=True, slots=True)
class VersionLog:
    """Неизменяемый журнал ревизий и записанных вызовов."""

    head: int | None = None
    commit: str | None = None
    revisions: tuple[Revision, ...] = ()
    calls: tuple[Mapping[str, Any], ...] = ()


@dataclass(frozen=True, slots=True)
class ApplyResult:
    """Итог загрузки версии. ``code is None`` — успех."""

    code: str | None
    log: VersionLog


@dataclass(frozen=True, slots=True)
class Entity:
    """Сущность артефакта: канонические байты и их хеш."""

    id: str
    content_hash: str
    data: bytes


@dataclass(frozen=True, slots=True)
class Artifact:
    """Нормализованный артефакт одной версии каталога."""

    catalog_version: int
    commit: str
    corrected_keys: tuple[str, ...]
    entities: tuple[Entity, ...]


_MANIFEST_KEYS = {"active", "catalog_version", "commit", "corrected_keys", "retired"}
_ENTRY_KEYS = {"content_hash", "id"}
_MISSING = object()


def build_artifact(
    catalog: Path,
    *,
    ref: str,
    commit: str,
    catalog_version: int,
    previous: Artifact | None = None,
    out: Path,
) -> Artifact:
    """Собрать tar.zst с манифестом и нормализованными сущностями."""

    if ref != "main":
        raise BuildRefused("артефакт собирается только из коммита в main")

    try:
        problems = check_catalog(Path(catalog), mode="main")
    except ValueError as error:
        raise ArtifactInvalid() from error
    if problems:
        raise ArtifactInvalid()

    payloads = _payloads(Path(catalog))
    if previous is not None:
        known = {entity.id for entity in previous.entities}
        if known - set(payloads):
            raise ArtifactInvalid()

    entities = tuple(
        _entity(identifier, payloads[identifier]) for identifier in sorted(payloads)
    )
    artifact = Artifact(
        catalog_version=catalog_version,
        commit=commit,
        corrected_keys=_corrected_keys(previous, entities),
        entities=entities,
    )
    _write_archive(Path(out), artifact)
    return artifact


def verify_artifact(path: Path, *, signer: Any = None) -> Artifact:
    """Проверить целостность архива и, если передана, функцию подписи."""

    archive = Path(path)
    artifact = _read_archive(archive)
    if signer is not None and not signer(archive):
        raise SignatureInvalid()
    return artifact


def apply_version(
    log: VersionLog,
    artifact: Artifact,
    *,
    fail_at: int | None = None,
) -> ApplyResult:
    """Добавить ревизии изменившихся сущностей, не меняя входной журнал."""

    # LD-R4: id из журнала нельзя стереть, просто убрав его из артефакта.
    present = {entity.id for entity in artifact.entities}
    if any(revision.id not in present for revision in log.revisions):
        return ApplyResult(code="artifact_invalid", log=log)

    changed = _changed_ids(log, artifact)
    if not changed:
        # Те же хеши. Голова и голова плюс 1 — тихий успех без вызовов.
        # Любой другой номер — artifact_invalid, журнал не меняется.
        same_head = log.head is not None and artifact.catalog_version in (
            log.head,
            log.head + 1,
        )
        if not same_head:
            return ApplyResult(code="artifact_invalid", log=log)
        return ApplyResult(code=None, log=log)

    if log.head is None:
        if not _version_at_least_one(artifact.catalog_version):
            return ApplyResult(code="artifact_invalid", log=log)
    elif artifact.catalog_version != log.head + 1:
        return ApplyResult(code="artifact_invalid", log=log)

    if fail_at is not None and fail_at >= 1 and len(changed) >= fail_at:
        return ApplyResult(code="load_failed", log=log)

    by_id = {entity.id: entity for entity in artifact.entities}
    added = tuple(
        Revision(
            id=identifier,
            catalog_version_from=artifact.catalog_version,
            content_hash=by_id[identifier].content_hash,
            data=by_id[identifier].data,
        )
        for identifier in changed
    )
    calls = tuple(dict(call) for call in log.calls)
    calls += (
        {
            "kind": "version",
            "catalog_version": artifact.catalog_version,
            "commit": artifact.commit,
        },
    )
    # Ключ сверяется с последней ревизией журнала, а не с corrected_keys.
    recheck_variants = _recheck_variants(log, artifact)
    if recheck_variants:
        calls += (
            {
                "kind": "recheck",
                "correction_id": artifact.catalog_version,
                "variants": recheck_variants,
            },
        )
    return ApplyResult(
        code=None,
        log=VersionLog(
            head=artifact.catalog_version,
            commit=artifact.commit,
            revisions=log.revisions + added,
            calls=calls,
        ),
    )


def read_version(log: VersionLog, number: int) -> dict[str, bytes]:
    """Последняя ревизия каждого id с ``catalog_version_from <= number``.

    Выведенные из использования сущности остаются в ответе.
    """

    chosen: dict[str, Revision] = {}
    for revision in log.revisions:
        if revision.catalog_version_from > number:
            continue
        current = chosen.get(revision.id)
        if (
            current is None
            or revision.catalog_version_from >= current.catalog_version_from
        ):
            chosen[revision.id] = revision
    return {identifier: revision.data for identifier, revision in chosen.items()}


def canonical_bytes(payload: Any) -> bytes:
    """Канонические байты D275: UTF-8, без хвостового перевода строки."""

    return json.dumps(
        payload,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")


def _version_at_least_one(catalog_version: object) -> bool:
    return type(catalog_version) is int and catalog_version >= 1


def _entity(identifier: str, payload: dict[str, Any]) -> Entity:
    data = canonical_bytes(payload)
    return Entity(
        id=identifier,
        content_hash=hashlib.sha256(data).hexdigest(),
        data=data,
    )


def _latest_revisions(log: VersionLog) -> dict[str, Revision]:
    latest: dict[str, Revision] = {}
    for revision in log.revisions:
        current = latest.get(revision.id)
        if (
            current is None
            or revision.catalog_version_from >= current.catalog_version_from
        ):
            latest[revision.id] = revision
    return latest


def _changed_ids(log: VersionLog, artifact: Artifact) -> list[str]:
    latest = _latest_revisions(log)
    changed = [
        entity.id
        for entity in artifact.entities
        if entity.id not in latest
        or latest[entity.id].content_hash != entity.content_hash
    ]
    changed.sort()
    return changed


def _recheck_variants(log: VersionLog, artifact: Artifact) -> list[str]:
    """Id, чей key отличается от последней ревизии того же id."""

    latest = _latest_revisions(log)
    found = [
        entity.id
        for entity in artifact.entities
        if entity.id in latest and _key(latest[entity.id].data) != _key(entity.data)
    ]
    found.sort()
    return found


def _corrected_keys(
    previous: Artifact | None, entities: tuple[Entity, ...]
) -> tuple[str, ...]:
    if previous is None:
        return ()
    previous_by_id = {entity.id: entity for entity in previous.entities}
    found: list[str] = []
    for entity in entities:
        old = previous_by_id.get(entity.id)
        if old is None:
            continue
        old_key = _key(old.data)
        new_key = _key(entity.data)
        if old_key != new_key:
            found.append(entity.id)
    found.sort()
    return tuple(found)


def _key(data: bytes) -> Any:
    payload = json.loads(data)
    if not isinstance(payload, dict):
        return _MISSING
    return payload.get("key", _MISSING)


def _payloads(root: Path) -> dict[str, dict[str, Any]]:
    documents, _problems = load_documents(root)
    raw: dict[str, dict[str, Any]] = {}
    for document in documents:
        data = document.data
        if not isinstance(data, dict):
            continue
        if document.definition == "topic":
            _put(raw, data)
            for element in _dicts(data.get("elements")):
                _put(raw, element)
            for task_type in _dicts(data.get("types")):
                _put(raw, task_type)
                for subvariant in _dicts(task_type.get("subvariants")):
                    _put(raw, subvariant)
        elif document.definition == "variants_file":
            # Поле type принадлежит файлу, а не варианту.
            for variant in _dicts(data.get("variants")):
                _put(raw, variant)
        elif document.definition == "block_frontmatter":
            payload = copy.deepcopy(data)
            payload["body"] = _block_body(
                (root / document.file).read_text(encoding="utf-8")
            )
            _put(raw, payload)
        elif document.definition == "test":
            _put(raw, data)
        elif document.definition == "lesson_template":
            _put(raw, data)
            for stage in _dicts(data.get("stages")):
                _put(raw, stage)
    entity_ids = set(raw)
    return {
        identifier: _normalize(payload, entity_ids)
        for identifier, payload in raw.items()
    }


def _put(raw: dict[str, dict[str, Any]], payload: dict[str, Any]) -> None:
    identifier = payload.get("id")
    if isinstance(identifier, str):
        raw[identifier] = copy.deepcopy(payload)


def _dicts(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, dict)]


def _normalize(payload: dict[str, Any], entity_ids: set[str]) -> dict[str, Any]:
    normalized = _normalize_value(payload, entity_ids)
    if not isinstance(normalized, dict):
        raise ArtifactInvalid()
    return normalized


def _normalize_value(value: Any, entity_ids: set[str]) -> Any:
    if isinstance(value, list):
        if value and all(_is_nested_entity(item, entity_ids) for item in value):
            return sorted(item["id"] for item in value)
        return [_normalize_value(item, entity_ids) for item in value]
    if isinstance(value, dict):
        return {key: _normalize_value(item, entity_ids) for key, item in value.items()}
    return value


def _is_nested_entity(value: Any, entity_ids: set[str]) -> bool:
    return isinstance(value, dict) and value.get("id") in entity_ids


def _block_body(text: str) -> str:
    """Текст после закрывающего ``---``.

    ``split("---\\n")`` снимает маркер вместе с одним переводом строки,
    которым этот маркер заканчивается. Второй перевод строки, если он есть,
    остаётся в теле.
    """

    if not text.startswith("---\n"):
        raise ArtifactInvalid()
    parts = text.split("---\n", 2)
    if len(parts) < 3:
        raise ArtifactInvalid()
    return parts[2]


def _write_archive(path: Path, artifact: Artifact) -> None:
    files = {"manifest.json": canonical_bytes(_manifest(artifact))}
    for entity in artifact.entities:
        files[f"entities/{entity.id}.json"] = entity.data
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
            info.uname = ""
            info.gname = ""
            archive.addfile(info, io.BytesIO(blob))
    path.write_bytes(buffer.getvalue())


def _manifest(artifact: Artifact) -> dict[str, Any]:
    active: list[dict[str, str]] = []
    retired: list[dict[str, str]] = []
    for entity in artifact.entities:
        entry = {"content_hash": entity.content_hash, "id": entity.id}
        payload = json.loads(entity.data)
        if isinstance(payload, dict) and payload.get("status") == "retired":
            retired.append(entry)
        else:
            active.append(entry)
    active.sort(key=lambda item: item["id"])
    retired.sort(key=lambda item: item["id"])
    return {
        "active": active,
        "catalog_version": artifact.catalog_version,
        "commit": artifact.commit,
        "corrected_keys": list(artifact.corrected_keys),
        "retired": retired,
    }


def _read_archive(path: Path) -> Artifact:
    try:
        files = _tar_files(path)
        manifest_raw = files["manifest.json"]
        manifest = json.loads(manifest_raw)
    except (
        OSError,
        tarfile.TarError,
        KeyError,
        json.JSONDecodeError,
        UnicodeError,
    ) as error:
        raise ArtifactInvalid() from error
    if not isinstance(manifest, dict) or canonical_bytes(manifest) != manifest_raw:
        raise ArtifactInvalid()
    if set(manifest) != _MANIFEST_KEYS:
        raise ArtifactInvalid()
    catalog_version = manifest["catalog_version"]
    commit = manifest["commit"]
    corrected = manifest["corrected_keys"]
    if type(catalog_version) is not int or not isinstance(commit, str):
        raise ArtifactInvalid()
    if not isinstance(corrected, list) or not all(
        isinstance(item, str) for item in corrected
    ):
        raise ArtifactInvalid()
    entries = _entries(manifest["active"]) + _entries(manifest["retired"])
    identifiers = [entry["id"] for entry in entries]
    if len(identifiers) != len(set(identifiers)):
        raise ArtifactInvalid()
    expected = {
        "manifest.json",
        *(f"entities/{identifier}.json" for identifier in identifiers),
    }
    if set(files) != expected:
        raise ArtifactInvalid()
    entities: list[Entity] = []
    for entry in entries:
        raw = files[f"entities/{entry['id']}.json"]
        payload = _object(raw)
        # D275: файл сущности принимается только как точные канонические байты.
        if payload is None or canonical_bytes(payload) != raw:
            raise ArtifactInvalid()
        if hashlib.sha256(raw).hexdigest() != entry["content_hash"]:
            raise ArtifactInvalid()
        entities.append(
            Entity(id=entry["id"], content_hash=entry["content_hash"], data=raw)
        )
    entities.sort(key=lambda item: item.id)
    return Artifact(
        catalog_version=catalog_version,
        commit=commit,
        corrected_keys=tuple(corrected),
        entities=tuple(entities),
    )


def _entries(value: Any) -> list[dict[str, str]]:
    if not isinstance(value, list):
        raise ArtifactInvalid()
    entries: list[dict[str, str]] = []
    for item in value:
        if not isinstance(item, dict) or set(item) != _ENTRY_KEYS:
            raise ArtifactInvalid()
        identifier = item["id"]
        content_hash = item["content_hash"]
        if not isinstance(identifier, str) or not isinstance(content_hash, str):
            raise ArtifactInvalid()
        entries.append({"id": identifier, "content_hash": content_hash})
    return entries


def _object(raw: bytes) -> dict[str, Any] | None:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        return None
    if not isinstance(payload, dict):
        return None
    return payload


def _tar_files(path: Path) -> dict[str, bytes]:
    files: dict[str, bytes] = {}
    with tarfile.open(path, "r:zst") as archive:
        for member in archive.getmembers():
            if not member.isfile():
                continue
            if member.name in files:
                raise ArtifactInvalid()
            extracted = archive.extractfile(member)
            if extracted is None:
                raise ArtifactInvalid()
            files[member.name] = extracted.read()
    return files


__all__ = [
    "ApplyResult",
    "Artifact",
    "ArtifactInvalid",
    "BuildRefused",
    "Entity",
    "Revision",
    "SignatureInvalid",
    "VersionLog",
    "apply_version",
    "build_artifact",
    "canonical_bytes",
    "read_version",
    "verify_artifact",
]
