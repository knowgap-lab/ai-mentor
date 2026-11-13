"""Приёмка артефакта каталога: LD-R1, LD-R3…LD-R8, LD-R10, LD-R12.

Импорт ``catalog_artifact`` выполняется внутри теста. Эти тесты фиксируют
наблюдаемый интерфейс брифа:

- исключения ``BuildRefused``, ``ArtifactInvalid``, ``SignatureInvalid``;
  у двух последних атрибут ``code`` равен ``artifact_invalid`` или
  ``signature_invalid``;
- ``VersionLog()`` — пустой журнал: ``head is None``, ``commit is None``,
  пустые ``revisions`` и ``calls``;
- ревизия: ``id``, ``catalog_version_from``, ``content_hash``, ``data``
  (объект JSON либо канонические байты);
- ``apply_version`` возвращает ``ApplyResult``: ``code is None`` при успехе,
  иначе ``artifact_invalid`` или ``load_failed``; входной журнал не меняется;
- вызовы — словари в ``log.calls``. Порядок вызовов бриф не задаёт.

ВОПРОС: точные крайние переводы строки поля ``body`` (текст после закрывающего
frontmatter) не заданы. Тест проверяет вхождение текста Markdown.
ВОПРОС: копируется ли поле ``type`` файла вариантов в сущность варианта — не
сказано. Тест это поле не требует и не запрещает.
ВОПРОС: если хеши сущностей совпали, а ``catalog_version`` артефакта равен
голове плюс 1, бриф прямо описывает только повтор с тем же номером. Этот
случай не тестируется.
"""

from __future__ import annotations

import hashlib
import io
import json
import shutil
import tarfile
from pathlib import Path

import pytest

from conftest import mutate_yaml


TOPIC_ID = "ct-demo:D1"
TYPE_ID = "ct-demo:D1.T01"
SUBVARIANT_ID = "ct-demo:D1.T01.S1"
ELEMENT_ID = "ct-demo:D1.E1"
BLOCK_ID = "ct-demo:D1.B01"
OTHER_BLOCK_ID = "ct-demo:D1.B02"
LESSON_ID = "ct-demo:L01"
VARIANT_ID = "ct-demo:D1.T01.S1.V003"
VARIANT_V001 = "ct-demo:D1.T01.S1.V001"
VARIANT_V005 = "ct-demo:D1.T01.S1.V005"
NEW_BLOCK_ID = "ct-demo:D1.B03"
BLOCK_SENTENCE = "Результат операции $a + b$ находится по определению."
ELEMENT_TITLE = "Сложение: тема целиком"
TYPE_TITLE = "Сложение: число"
SUBVARIANT_GIVEN = "два числа"


def _demo_ids() -> frozenset[str]:
    identifiers = {
        "ct-demo:D1",
        "ct-demo:D2",
        "ct-demo:D1.E1",
        "ct-demo:D2.E1",
        "ct-demo:D1.T01",
        "ct-demo:D1.T02",
        "ct-demo:D2.T01",
        "ct-demo:D2.T02",
        "ct-demo:D1.T01.S1",
        "ct-demo:D1.T02.S1",
        "ct-demo:D2.T01.S1",
        "ct-demo:D2.T02.S1",
        "ct-demo:D1.B01",
        "ct-demo:D1.B02",
        "ct-demo:D2.B01",
        "ct-demo:D2.B02",
        "ct-demo:L01",
        "ct-demo:L01.G1",
        "ct-demo:L01.G2",
        "ct-demo:TEST.2025-CT-V1",
    }
    for topic in ("D1", "D2"):
        for task_type in ("T01", "T02"):
            for number in range(1, 6):
                identifiers.add(f"ct-demo:{topic}.{task_type}.S1.V{number:03d}")
    if len(identifiers) != 40:
        raise RuntimeError(f"ожидалось 40 id демо-каталога, получено {len(identifiers)}")
    return frozenset(identifiers)


DEMO_IDS = _demo_ids()


def artifact_api():
    """Импорт пакета только в момент вызова теста."""
    import catalog_artifact

    return catalog_artifact


def canonical_bytes(payload: object) -> bytes:
    """Канонические байты D275."""
    return json.dumps(
        payload,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")


def payload_dict(value: object) -> dict:
    """Приводит payload ревизии к объекту JSON."""
    if isinstance(value, (bytes, bytearray)):
        value = bytes(value).decode("utf-8")
    if isinstance(value, str):
        loaded = json.loads(value)
        if not isinstance(loaded, dict):
            raise AssertionError("payload сущности должен быть объектом")
        return loaded
    if isinstance(value, dict):
        return value
    raise AssertionError("payload ревизии должен быть объектом или каноническими байтами")


def payload_bytes(value: object) -> bytes:
    """Байты payload: уже готовые канонические либо объект JSON."""
    if isinstance(value, (bytes, bytearray)):
        return bytes(value)
    if isinstance(value, str):
        return value.encode("utf-8")
    return canonical_bytes(value)


def read_archive(path: Path) -> dict[str, bytes]:
    """Читает файлы архива zstd, пропуская узлы-каталоги."""
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


def load_archive(path: Path) -> tuple[dict, dict[str, bytes]]:
    """Проверяет канонический манифест, хеши и состав архива."""
    files = read_archive(path)
    manifest_raw = files["manifest.json"]
    manifest = json.loads(manifest_raw)
    assert manifest_raw == canonical_bytes(manifest)
    assert set(manifest) == {
        "active",
        "catalog_version",
        "commit",
        "corrected_keys",
        "retired",
    }
    entries = list(manifest["active"]) + list(manifest["retired"])
    expected_names = {"manifest.json", *(f"entities/{item['id']}.json" for item in entries)}
    assert set(files) == expected_names
    assert all("figures" not in name for name in files)
    active_ids = [item["id"] for item in manifest["active"]]
    retired_ids = [item["id"] for item in manifest["retired"]]
    assert active_ids == sorted(active_ids)
    assert retired_ids == sorted(retired_ids)
    assert set(active_ids).isdisjoint(retired_ids)
    for item in entries:
        assert set(item) == {"content_hash", "id"}
        raw = files[f"entities/{item['id']}.json"]
        assert raw == canonical_bytes(json.loads(raw))
        assert item["content_hash"] == hashlib.sha256(raw).hexdigest()
        payload = json.loads(raw)
        retired = payload.get("status") == "retired"
        assert (item["id"] in retired_ids) is retired
    return manifest, files


def entity_payload(files: dict[str, bytes], identifier: str) -> dict:
    return json.loads(files[f"entities/{identifier}.json"])


def entry_hash(manifest: dict, identifier: str) -> str:
    for item in manifest["active"] + manifest["retired"]:
        if item["id"] == identifier:
            return item["content_hash"]
    raise AssertionError(identifier)


def manifest_changes(before: dict, after: dict) -> set[str]:
    """Id, у которых хеш появился, исчез или изменился."""
    left = {item["id"]: item["content_hash"] for item in before["active"] + before["retired"]}
    right = {item["id"]: item["content_hash"] for item in after["active"] + after["retired"]}
    return {
        identifier
        for identifier in set(left) | set(right)
        if left.get(identifier) != right.get(identifier)
    }


def dumped_calls(calls) -> list[str]:
    """Сравнивает вызовы без учёта их порядка."""
    return sorted(
        json.dumps(dict(call), ensure_ascii=False, sort_keys=True) for call in calls
    )


def log_view(log) -> tuple:
    revisions = tuple(
        sorted(
            (
                revision.id,
                revision.catalog_version_from,
                revision.content_hash,
                payload_bytes(revision.data),
            )
            for revision in log.revisions
        )
    )
    return (log.head, log.commit, revisions, tuple(dumped_calls(log.calls)))


def build(root: Path, out: Path, **kwargs):
    api = artifact_api()
    return api.build_artifact(
        root,
        ref=kwargs.get("ref", "main"),
        commit=kwargs.get("commit", "commit-1"),
        catalog_version=kwargs.get("catalog_version", 1),
        previous=kwargs.get("previous"),
        out=out,
    )


def apply(log, artifact, *, fail_at: int | None = None):
    return artifact_api().apply_version(log, artifact, fail_at=fail_at)


def empty_log():
    return artifact_api().VersionLog()


def replace_once(root: Path, relative: str, old: str, new: str) -> None:
    path = root / relative
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise AssertionError(f"{old!r} не найден в {relative}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")


def repack(source: Path, dest: Path, mutate) -> None:
    files = dict(read_archive(source))
    mutate(files)
    with tarfile.open(dest, "w:zst") as archive:
        for name in sorted(files):
            data = files[name]
            info = tarfile.TarInfo(name)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))


def test_loader_01_pr_ref_is_build_refused(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R1, loader-01: ref="pr" — BuildRefused, файл артефакта не создаётся."""
    out = tmp_path / "catalog-pr.tar.zst"
    with pytest.raises(artifact_api().BuildRefused):
        build(catalog_root, out, ref="pr", commit="commit-pr", catalog_version=1)
    assert not out.exists()


def test_ld_r1_invalid_catalog_is_artifact_invalid(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R1: непустой check_catalog(mode="main") — artifact_invalid, файла нет."""
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc.__setitem__("id", "ct-demo:D1.T1"),
    )
    out = tmp_path / "invalid.tar.zst"
    with pytest.raises(artifact_api().ArtifactInvalid) as caught:
        build(catalog_root, out, commit="commit-invalid", catalog_version=1)
    assert caught.value.code == "artifact_invalid"
    assert not out.exists()


def test_ld_r3_two_builds_share_canonical_hash(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R3: два сбора одного каталога дают одни канонические байты и один хеш."""
    first = build(
        catalog_root,
        tmp_path / "first.tar.zst",
        commit="commit-1",
        catalog_version=1,
    )
    second = build(
        catalog_root,
        tmp_path / "second.tar.zst",
        commit="commit-1",
        catalog_version=1,
    )
    manifest, files = load_archive(tmp_path / "first.tar.zst")
    again, again_files = load_archive(tmp_path / "second.tar.zst")
    assert files == again_files
    assert manifest["catalog_version"] == 1
    assert manifest["commit"] == "commit-1"
    assert manifest["corrected_keys"] == []
    assert manifest["retired"] == []
    active_ids = {item["id"] for item in manifest["active"]}
    assert active_ids == DEMO_IDS
    assert "ct-demo" not in active_ids
    assert "off_by_one" not in active_ids
    topic_raw = files[f"entities/{TOPIC_ID}.json"]
    type_raw = files[f"entities/{TYPE_ID}.json"]
    variant_raw = files[f"entities/{VARIANT_V001}.json"]
    assert "Сложение".encode() in topic_raw
    assert b'"difficulty":1,' in type_raw
    assert b'"hours":0.5' in type_raw
    assert b'"hours":1.0' in type_raw
    assert b'"key":5,' in variant_raw
    assert b'"key":5.0"' not in variant_raw
    variant = entity_payload(files, VARIANT_V001)
    assert variant["key"] == 5
    assert isinstance(variant["key"], int)
    assert first is not None and second is not None


def test_ld_r3_same_payload_has_one_hash(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R3: тот же payload после смены порядка вложенных типов даёт тот же хеш."""
    build(catalog_root, tmp_path / "original.tar.zst", commit="commit-1", catalog_version=1)
    reordered = tmp_path / "reordered-catalog"
    shutil.copytree(catalog_root, reordered)
    mutate_yaml(
        reordered,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc.__setitem__("types", list(reversed(doc["types"]))),
    )
    build(reordered, tmp_path / "reordered.tar.zst", commit="commit-1", catalog_version=1)
    _, original_files = load_archive(tmp_path / "original.tar.zst")
    _, reordered_files = load_archive(tmp_path / "reordered.tar.zst")
    assert original_files == reordered_files


def test_ld_r5_parent_payload_lists_sorted_child_ids(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R5: вложенные сущности родителя заменяются отсортированным списком id."""
    build(catalog_root, tmp_path / "normalized.tar.zst", commit="commit-1", catalog_version=1)
    _, files = load_archive(tmp_path / "normalized.tar.zst")
    topic = entity_payload(files, TOPIC_ID)
    task_type = entity_payload(files, TYPE_ID)
    lesson = entity_payload(files, LESSON_ID)
    block = entity_payload(files, BLOCK_ID)
    element = entity_payload(files, ELEMENT_ID)
    subvariant = entity_payload(files, SUBVARIANT_ID)
    assert topic["types"] == ["ct-demo:D1.T01", "ct-demo:D1.T02"]
    assert topic["elements"] == [ELEMENT_ID]
    assert task_type["subvariants"] == [SUBVARIANT_ID]
    assert lesson["stages"] == ["ct-demo:L01.G1", "ct-demo:L01.G2"]
    assert BLOCK_SENTENCE in block["body"]
    assert ELEMENT_TITLE not in files[f"entities/{TOPIC_ID}.json"].decode()
    assert ELEMENT_TITLE in files[f"entities/{ELEMENT_ID}.json"].decode()
    assert TYPE_TITLE not in files[f"entities/{TOPIC_ID}.json"].decode()
    assert SUBVARIANT_GIVEN not in files[f"entities/{TYPE_ID}.json"].decode()
    assert SUBVARIANT_GIVEN in files[f"entities/{SUBVARIANT_ID}.json"].decode()
    assert element["id"] == ELEMENT_ID
    assert subvariant["id"] == SUBVARIANT_ID


def test_loader_03_retired_type_keeps_previous_revision(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R4, loader-03: старая ревизия типа остаётся, в новой версии статус retired."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    before, _ = load_archive(first_out)
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: (
            doc["types"][0].__setitem__("status", "retired"),
            doc["types"][0].__setitem__("retired_reason", "синтетический вывод"),
        ),
    )
    second_out = tmp_path / "v2.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    after, files = load_archive(second_out)
    assert manifest_changes(before, after) == {TYPE_ID}
    assert entry_hash(after, TYPE_ID) != entry_hash(before, TYPE_ID)
    retired_ids = [item["id"] for item in after["retired"]]
    active_ids = {item["id"] for item in after["active"]}
    assert retired_ids == [TYPE_ID]
    assert TYPE_ID not in active_ids
    assert SUBVARIANT_ID in active_ids
    assert entity_payload(files, TYPE_ID)["status"] == "retired"

    log = apply(empty_log(), first)
    assert log.code is None
    held = log.log
    before_apply = log_view(held)
    result = apply(held, second)
    assert result.code is None
    assert log_view(held) == before_apply
    assert result.log.head == 2
    assert result.log.commit == "commit-2"
    type_revisions = sorted(
        (revision for revision in result.log.revisions if revision.id == TYPE_ID),
        key=lambda revision: revision.catalog_version_from,
    )
    assert [revision.catalog_version_from for revision in type_revisions] == [1, 2]
    assert type_revisions[0].content_hash == entry_hash(before, TYPE_ID)
    assert type_revisions[1].content_hash == entry_hash(after, TYPE_ID)
    assert [revision.catalog_version_from for revision in result.log.revisions if revision.id == TOPIC_ID] == [1]
    current = payload_dict(artifact_api().read_version(result.log, 2)[TYPE_ID])
    previous = payload_dict(artifact_api().read_version(result.log, 1)[TYPE_ID])
    assert current["status"] == "retired"
    assert current["retired_reason"] == "синтетический вывод"
    assert previous.get("status") != "retired"
    assert dumped_calls(result.log.calls) == dumped_calls(
        [
            {"kind": "version", "catalog_version": 1, "commit": "commit-1"},
            {"kind": "version", "catalog_version": 2, "commit": "commit-2"},
        ]
    )


def test_ld_r4_deleted_id_is_artifact_invalid(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R4: id из previous, которого нет в новом каталоге, — artifact_invalid, файла нет."""
    previous_out = tmp_path / "previous.tar.zst"
    previous = build(catalog_root, previous_out, commit="commit-1", catalog_version=1)
    previous_manifest, _ = load_archive(previous_out)
    assert VARIANT_V005 in {item["id"] for item in previous_manifest["active"]}
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc.__setitem__("variants", doc["variants"][:-1]),
    )
    out = tmp_path / "deleted.tar.zst"
    with pytest.raises(artifact_api().ArtifactInvalid) as caught:
        build(
            catalog_root,
            out,
            commit="commit-2",
            catalog_version=2,
            previous=previous,
        )
    assert caught.value.code == "artifact_invalid"
    assert not out.exists()


def test_loader_04_one_typo_bumps_head_to_8(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R5, loader-04: голова 7 и одна опечатка дают голову 8 и одну новую ревизию."""
    first_out = tmp_path / "v7.tar.zst"
    first = build(catalog_root, first_out, commit="commit-7", catalog_version=7)
    before, _ = load_archive(first_out)
    loaded = apply(empty_log(), first)
    assert loaded.code is None
    assert loaded.log.head == 7
    assert loaded.log.commit == "commit-7"
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
    after, _ = load_archive(second_out)
    assert after["corrected_keys"] == []
    assert manifest_changes(before, after) == {TYPE_ID}
    before_apply = log_view(loaded.log)
    result = apply(loaded.log, second)
    assert result.code is None
    assert log_view(loaded.log) == before_apply
    assert result.log.head == 8
    assert result.log.commit == "commit-8"
    added = [revision for revision in result.log.revisions if revision.catalog_version_from == 8]
    assert len(added) == 1
    assert added[0].id == TYPE_ID
    assert added[0].content_hash == entry_hash(after, TYPE_ID)
    assert [
        revision.catalog_version_from
        for revision in result.log.revisions
        if revision.id == TOPIC_ID
    ] == [7]
    assert dumped_calls(result.log.calls) == dumped_calls(
        [
            {"kind": "version", "catalog_version": 7, "commit": "commit-7"},
            {"kind": "version", "catalog_version": 8, "commit": "commit-8"},
        ]
    )


def test_ld_r5_empty_log_rejects_version_zero(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R5: пустой журнал не принимает catalog_version меньше 1."""
    log = empty_log()
    assert log.head is None
    assert log.commit is None
    assert list(log.revisions) == []
    assert list(log.calls) == []
    out = tmp_path / "zero.tar.zst"
    artifact = build(catalog_root, out, commit="commit-0", catalog_version=0)
    assert out.is_file()
    before = log_view(log)
    result = apply(log, artifact)
    assert result.code == "artifact_invalid"
    assert log_view(result.log) == before
    assert log_view(log) == before


def test_ld_r5_changed_catalog_must_follow_head(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R5: при изменении хеша номер равен голове плюс 1, иначе журнал прежний."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    loaded = apply(empty_log(), first)
    assert loaded.code is None
    replace_once(
        catalog_root,
        "ct-demo/blocks/D1/B01.md",
        "по определению",
        "по определению сейчас",
    )
    same_number = build(
        catalog_root,
        tmp_path / "same.tar.zst",
        commit="commit-same",
        catalog_version=1,
        previous=first,
    )
    skipped = build(
        catalog_root,
        tmp_path / "skip.tar.zst",
        commit="commit-3",
        catalog_version=3,
        previous=first,
    )
    before = log_view(loaded.log)
    same_result = apply(loaded.log, same_number)
    assert same_result.code == "artifact_invalid"
    assert log_view(same_result.log) == before
    assert log_view(loaded.log) == before
    skipped_result = apply(loaded.log, skipped)
    assert skipped_result.code == "artifact_invalid"
    assert log_view(skipped_result.log) == before
    assert log_view(loaded.log) == before


def test_ld_r6_new_id_adds_one_revision(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R6: новая ревизия появляется для нового id и не появляется для прежнего хеша."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    before, _ = load_archive(first_out)
    (catalog_root / "ct-demo/blocks/D1/B03.md").write_text(
        "---\n"
        "id: ct-demo:D1.B03\n"
        "topic_id: ct-demo:D1\n"
        "kind: theory\n"
        "title: Дополнение\n"
        "---\n"
        "\n"
        "Синтетическое дополнение.\n",
        encoding="utf-8",
    )
    second_out = tmp_path / "v2.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    after, files = load_archive(second_out)
    assert manifest_changes(before, after) == {NEW_BLOCK_ID}
    loaded = apply(empty_log(), first)
    result = apply(loaded.log, second)
    assert result.code is None
    added = [revision for revision in result.log.revisions if revision.catalog_version_from == 2]
    assert [revision.id for revision in added] == [NEW_BLOCK_ID]
    assert all(
        revision.catalog_version_from == 1
        for revision in result.log.revisions
        if revision.id in DEMO_IDS
    )
    version_one = artifact_api().read_version(result.log, 1)
    version_two = artifact_api().read_version(result.log, 2)
    assert NEW_BLOCK_ID not in version_one
    assert NEW_BLOCK_ID in version_two
    assert payload_bytes(version_two[NEW_BLOCK_ID]) == files[f"entities/{NEW_BLOCK_ID}.json"]


def test_loader_06_version_6_block_survives_version_8(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R7, loader-06: чтение версии 6 после версии 8 возвращает прежние байты блока."""
    version_6 = build(
        catalog_root,
        tmp_path / "v6.tar.zst",
        commit="commit-6",
        catalog_version=6,
    )
    manifest_6, files_6 = load_archive(tmp_path / "v6.tar.zst")
    block_6 = files_6[f"entities/{BLOCK_ID}.json"]
    log = apply(empty_log(), version_6).log
    mutate_yaml(
        catalog_root,
        "ct-demo/topics/D1.yaml",
        lambda doc: doc["types"][0].__setitem__("title", "Сложение: чисо"),
    )
    version_7 = build(
        catalog_root,
        tmp_path / "v7.tar.zst",
        commit="commit-7",
        catalog_version=7,
        previous=version_6,
    )
    manifest_7, _ = load_archive(tmp_path / "v7.tar.zst")
    assert manifest_changes(manifest_6, manifest_7) == {TYPE_ID}
    log = apply(log, version_7).log
    replace_once(
        catalog_root,
        "ct-demo/blocks/D1/B01.md",
        "по определению",
        "по определению (версия 8)",
    )
    version_8 = build(
        catalog_root,
        tmp_path / "v8.tar.zst",
        commit="commit-8",
        catalog_version=8,
        previous=version_7,
    )
    manifest_8, files_8 = load_archive(tmp_path / "v8.tar.zst")
    assert manifest_changes(manifest_7, manifest_8) == {BLOCK_ID}
    result = apply(log, version_8)
    assert result.code is None
    assert result.log.head == 8
    block_8 = files_8[f"entities/{BLOCK_ID}.json"]
    assert block_6 != block_8
    read = artifact_api().read_version
    assert set(read(result.log, 6)) == DEMO_IDS
    assert set(read(result.log, 8)) == DEMO_IDS
    assert payload_bytes(read(result.log, 6)[BLOCK_ID]) == block_6
    assert payload_bytes(read(result.log, 7)[BLOCK_ID]) == block_6
    assert payload_bytes(read(result.log, 8)[BLOCK_ID]) == block_8


def test_loader_05_second_apply_keeps_log_and_calls(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R8, loader-05: повтор того же артефакта версии 8 не меняет журнал и вызовы."""
    artifact = build(
        catalog_root,
        tmp_path / "v8.tar.zst",
        commit="commit-8",
        catalog_version=8,
    )
    manifest, _ = load_archive(tmp_path / "v8.tar.zst")
    assert {item["id"] for item in manifest["active"]} == DEMO_IDS
    first = apply(empty_log(), artifact)
    assert first.code is None
    assert first.log.head == 8
    assert first.log.commit == "commit-8"
    assert len(list(first.log.revisions)) == len(DEMO_IDS)
    assert {revision.catalog_version_from for revision in first.log.revisions} == {8}
    assert dumped_calls(first.log.calls) == dumped_calls(
        [{"kind": "version", "catalog_version": 8, "commit": "commit-8"}]
    )
    before = log_view(first.log)
    second = apply(first.log, artifact)
    assert second.code is None
    assert log_view(second.log) == before
    assert log_view(first.log) == before
    assert second.log.head == 8
    assert list(second.log.calls) == list(first.log.calls)


def test_loader_07_fail_at_rolls_back_two_new_revisions(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R10, loader-07: fail_at=1 при двух новых ревизиях возвращает прежний журнал."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    before, _ = load_archive(first_out)
    loaded = apply(empty_log(), first)
    assert loaded.code is None
    replace_once(catalog_root, "ct-demo/blocks/D1/B01.md", "по определению", "по правилу")
    replace_once(catalog_root, "ct-demo/blocks/D1/B02.md", "Пример: $2 + 3$.", "Разбор: $2 + 3$.")
    second_out = tmp_path / "v2.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    after, _ = load_archive(second_out)
    assert manifest_changes(before, after) == {BLOCK_ID, OTHER_BLOCK_ID}
    snapshot = log_view(loaded.log)
    result = apply(loaded.log, second, fail_at=1)
    assert result.code == "load_failed"
    assert log_view(result.log) == snapshot
    assert log_view(loaded.log) == snapshot
    assert dumped_calls(result.log.calls) == dumped_calls(
        [{"kind": "version", "catalog_version": 1, "commit": "commit-1"}]
    )


def test_loader_08_key_change_requests_recheck(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R12, loader-08: ключ 12 меняется на 14, перепроверка несёт correction_id версии."""
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc["variants"][2].__setitem__("key", 12),
    )
    first_out = tmp_path / "v7.tar.zst"
    first = build(catalog_root, first_out, commit="commit-7", catalog_version=7)
    before, files_before = load_archive(first_out)
    assert before["corrected_keys"] == []
    previous_key = entity_payload(files_before, VARIANT_ID)["key"]
    assert previous_key == 12
    assert isinstance(previous_key, int)
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc["variants"][2].__setitem__("key", 14),
    )
    second_out = tmp_path / "v8.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-8",
        catalog_version=8,
        previous=first,
    )
    after, files_after = load_archive(second_out)
    assert after["corrected_keys"] == [VARIANT_ID]
    assert manifest_changes(before, after) == {VARIANT_ID}
    changed_key = entity_payload(files_after, VARIANT_ID)["key"]
    assert changed_key == 14
    assert isinstance(changed_key, int)
    loaded = apply(empty_log(), first)
    result = apply(loaded.log, second)
    assert result.code is None
    assert result.log.head == 8
    assert dumped_calls(result.log.calls) == dumped_calls(
        [
            {"kind": "version", "catalog_version": 7, "commit": "commit-7"},
            {"kind": "version", "catalog_version": 8, "commit": "commit-8"},
            {
                "kind": "recheck",
                "correction_id": 8,
                "variants": [VARIANT_ID],
            },
        ]
    )


def test_ld_r12_corrected_keys_are_alphabetical(catalog_root: Path, tmp_path: Path) -> None:
    """LD-R12: id вариантов с новым ключом идут в corrected_keys по алфавиту."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)

    def change_keys(doc: dict) -> None:
        doc["variants"][0]["key"] = 6
        doc["variants"][2]["key"] = 10

    mutate_yaml(catalog_root, "ct-demo/variants/D1/T01.yaml", change_keys)
    second_out = tmp_path / "v2.tar.zst"
    second = build(
        catalog_root,
        second_out,
        commit="commit-2",
        catalog_version=2,
        previous=first,
    )
    manifest, _ = load_archive(second_out)
    assert manifest["corrected_keys"] == [VARIANT_V001, VARIANT_ID]
    result = apply(apply(empty_log(), first).log, second)
    assert result.code is None
    rechecks = [call for call in result.log.calls if call["kind"] == "recheck"]
    assert len(rechecks) == 1
    assert rechecks[0]["correction_id"] == 2
    assert list(rechecks[0]["variants"]) == [VARIANT_V001, VARIANT_ID]


def test_ld_r12_statement_change_is_not_corrected_key(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R12: другое поле при том же key не попадает в corrected_keys."""
    first_out = tmp_path / "v1.tar.zst"
    first = build(catalog_root, first_out, commit="commit-1", catalog_version=1)
    before, _ = load_archive(first_out)
    mutate_yaml(
        catalog_root,
        "ct-demo/variants/D1/T01.yaml",
        lambda doc: doc["variants"][1].__setitem__(
            "statement", "Найдите значение выражения $3 + 4$. Опечатка."
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
    after, _ = load_archive(second_out)
    assert after["corrected_keys"] == []
    assert manifest_changes(before, after) == {"ct-demo:D1.T01.S1.V002"}


def test_loader_02_rejected_signature_keeps_log(catalog_root: Path, tmp_path: Path) -> None:
    """loader-02: signer вернул False — signature_invalid, журнал ревизий не меняется."""
    out = tmp_path / "signed.tar.zst"
    artifact = build(catalog_root, out, commit="commit-1", catalog_version=1)
    loaded = apply(empty_log(), artifact)
    assert loaded.code is None
    snapshot = log_view(loaded.log)
    seen: list[Path] = []

    def reject(path: Path) -> bool:
        seen.append(Path(path))
        return False

    with pytest.raises(artifact_api().SignatureInvalid) as caught:
        artifact_api().verify_artifact(out, signer=reject)
    assert caught.value.code == "signature_invalid"
    assert [path.resolve() for path in seen] == [out.resolve()]
    assert log_view(loaded.log) == snapshot


def test_loader_02_signer_none_accepts_archive(catalog_root: Path, tmp_path: Path) -> None:
    """Точка расширения подписи: signer=None и signer True не проверяют minisign."""
    out = tmp_path / "open.tar.zst"
    built = build(catalog_root, out, commit="commit-1", catalog_version=1)
    calls: list[Path] = []

    def untouched(path: Path) -> bool:
        calls.append(Path(path))
        return True

    skipped = artifact_api().verify_artifact(out, signer=None)
    accepted = artifact_api().verify_artifact(out, signer=untouched)
    assert [path.resolve() for path in calls] == [out.resolve()]
    from_build = apply(empty_log(), built)
    from_skip = apply(empty_log(), skipped)
    from_accept = apply(empty_log(), accepted)
    assert from_build.code is None
    assert log_view(from_skip.log) == log_view(from_build.log)
    assert log_view(from_accept.log) == log_view(from_build.log)


def test_ld_r3_verify_rejects_tampered_entity_byte(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R3: verify_artifact принимает свой архив и отвергает подменённый байт сущности."""
    out = tmp_path / "intact.tar.zst"
    build(catalog_root, out, commit="commit-1", catalog_version=1)
    assert artifact_api().verify_artifact(out) is not None
    tampered = tmp_path / "tampered.tar.zst"

    def flip(files: dict[str, bytes]) -> None:
        name = f"entities/{BLOCK_ID}.json"
        blob = bytearray(files[name])
        blob[0] ^= 0x01
        files[name] = bytes(blob)

    repack(out, tampered, flip)
    with pytest.raises(artifact_api().ArtifactInvalid) as caught:
        artifact_api().verify_artifact(tampered)
    assert caught.value.code == "artifact_invalid"


def test_ld_r3_verify_rejects_incomplete_archive(
    catalog_root: Path, tmp_path: Path
) -> None:
    """LD-R3: лишний файл, пропавший файл и неканонический манифест — artifact_invalid."""
    out = tmp_path / "intact.tar.zst"
    build(catalog_root, out, commit="commit-1", catalog_version=1)
    extra = tmp_path / "extra.tar.zst"
    missing = tmp_path / "missing.tar.zst"
    loose = tmp_path / "loose.tar.zst"

    def add_extra(files: dict[str, bytes]) -> None:
        files[f"entities/{NEW_BLOCK_ID}.json"] = canonical_bytes({"id": NEW_BLOCK_ID})

    def drop_block(files: dict[str, bytes]) -> None:
        del files[f"entities/{OTHER_BLOCK_ID}.json"]

    def expand_manifest(files: dict[str, bytes]) -> None:
        files["manifest.json"] = json.dumps(
            json.loads(files["manifest.json"]),
            ensure_ascii=False,
            indent=2,
        ).encode("utf-8")

    repack(out, extra, add_extra)
    repack(out, missing, drop_block)
    repack(out, loose, expand_manifest)
    api = artifact_api()
    for path in (extra, missing, loose):
        with pytest.raises(api.ArtifactInvalid) as caught:
            api.verify_artifact(path)
        assert caught.value.code == "artifact_invalid"
