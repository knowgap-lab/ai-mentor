"""Сравнение id каталога с базой: CS-R4, CS-R23, CS-R25."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Iterable
from typing import Any

from .cross import Entity, _collect_entities
from .load import Document
from .problem import Problem


def history_problems(
    result_documents: Iterable[Document], base_documents: Iterable[Document]
) -> list[Problem]:
    """Сравнивает id базы и результата.

    Без вызова этой функции коды ``id_removed``, ``id_reused`` и
    ``transfer_map_missing`` не появляются. Файлы каталога не переписываются.
    """

    result_docs = list(result_documents)
    base_docs = list(base_documents)
    result_entities = _collect_entities(result_docs)
    base_entities = _collect_entities(base_docs)
    result_by_id = _by_id(result_entities)
    base_by_id = _by_id(base_entities)

    problems: list[Problem] = []
    for identifier, base_group in base_by_id.items():
        result_group = result_by_id.get(identifier, [])
        if not result_group:
            # Id в результате уже нет, указывать там некуда: file и pointer
            # берутся из базы, даже если сам файл в результате остался.
            for entity in base_group:
                problems.append(
                    Problem(
                        "id_removed",
                        entity.document.file,
                        entity.pointer,
                        f"Идентификатор {identifier!r} удалён из каталога.",
                    )
                )
            continue
        if not any(_is_retired(entity) for entity in base_group):
            continue
        for entity in result_group:
            if _is_retired(entity):
                continue
            problems.append(
                Problem(
                    "id_reused",
                    entity.document.file,
                    entity.pointer,
                    f"Идентификатор {identifier!r} использован повторно после status: retired.",
                )
            )

    used_types = _used_type_ids(base_docs)
    used_elements = _elements_with_use(base_entities, used_types)
    mapped = _mapped_ids(result_docs)
    for entity in result_entities:
        if entity.kind == "type":
            used = entity.identifier in used_types
        elif entity.kind == "element":
            used = entity.identifier in used_elements
        else:
            continue
        if not used or not _is_retired(entity) or entity.identifier in mapped:
            continue
        problems.append(
            Problem(
                "transfer_map_missing",
                entity.document.file,
                entity.pointer,
                (
                    f"Идентификатор {entity.identifier!r} со status: retired "
                    "отсутствует в карте переноса."
                ),
            )
        )
    return problems


def _by_id(entities: Iterable[Entity]) -> dict[str, list[Entity]]:
    grouped: dict[str, list[Entity]] = defaultdict(list)
    for entity in entities:
        grouped[entity.identifier].append(entity)
    return grouped


def _is_retired(entity: Entity) -> bool:
    """Нет поля status — сущность считается active."""

    return entity.data.get("status") == "retired"


def _used_type_ids(documents: Iterable[Document]) -> set[str]:
    """Типы, у которых в базе были появления или файл вариантов.

    Появление — id типа среди значений файла теста. Варианты — файл вариантов
    с полем ``type``, равным этому id.
    """

    used: set[str] = set()
    for document in documents:
        data = document.data
        if not isinstance(data, dict):
            continue
        if document.definition == "variants_file":
            type_id = data.get("type")
            if isinstance(type_id, str):
                used.add(type_id)
        elif document.definition == "test":
            used.update(_string_values(data))
    return used


def _elements_with_use(entities: Iterable[Entity], used_types: set[str]) -> set[str]:
    """Элемент «был занят», если в базе он primary_element занятого типа."""

    elements: set[str] = set()
    for entity in entities:
        if entity.kind != "type" or entity.identifier not in used_types:
            continue
        primary = entity.data.get("primary_element")
        if isinstance(primary, str):
            elements.add(primary)
    return elements


def _mapped_ids(documents: Iterable[Document]) -> set[str]:
    """Id из from, to и subvariants_to карты переноса результата."""

    found: set[str] = set()
    for document in documents:
        if document.definition != "transfer_map" or not isinstance(document.data, dict):
            continue
        entries = document.data.get("entries")
        if not isinstance(entries, list):
            continue
        for entry in entries:
            if not isinstance(entry, dict):
                continue
            for field in ("from", "to"):
                values = entry.get(field)
                if not isinstance(values, list):
                    continue
                for value in values:
                    if isinstance(value, str):
                        found.add(value)
            subvariants_to = entry.get("subvariants_to")
            if isinstance(subvariants_to, str):
                found.add(subvariants_to)
    return found


def _string_values(value: Any) -> set[str]:
    found: set[str] = set()

    def walk(node: Any) -> None:
        if isinstance(node, str):
            found.add(node)
        elif isinstance(node, dict):
            for item in node.values():
                walk(item)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(value)
    return found
