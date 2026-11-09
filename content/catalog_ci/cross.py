"""Межфайловые проверки каталога."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Iterable, Iterator
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .load import Document
from .problem import Problem
from .schema_layer import _looks_like_catalog_id, _looks_like_element_id


CatalogData = dict[str, Any]


@dataclass(frozen=True)
class Entity:
    """Сущность с id, извлечённая из допустимого файла."""

    kind: str
    identifier: str
    document: Document
    pointer: str
    data: CatalogData


def cross_file_problems(documents: Iterable[Document], mode: str) -> list[Problem]:
    docs = list(documents)
    entities = _collect_entities(docs)
    indexes: dict[str, dict[str, list[Entity]]] = defaultdict(lambda: defaultdict(list))
    all_ids: dict[str, list[Entity]] = defaultdict(list)
    for entity in entities:
        indexes[entity.kind][entity.identifier].append(entity)
        all_ids[entity.identifier].append(entity)

    problems: list[Problem] = []
    seen_temporary_locations: set[tuple[str, str]] = set()

    def add(problem: Problem) -> None:
        if problem.code == "tmp_key_in_main":
            location = (problem.file, problem.pointer)
            if location in seen_temporary_locations:
                return
            seen_temporary_locations.add(location)
        problems.append(problem)

    for identifier, occurrences in sorted(all_ids.items()):
        for duplicate in sorted(occurrences, key=_entity_sort_key)[1:]:
            add(
                Problem(
                    "duplicate_id",
                    duplicate.document.file,
                    duplicate.pointer,
                    f"Идентификатор {identifier!r} уже встречается в каталоге.",
                )
            )

    for entity in entities:
        _check_temporary(entity.document, entity.pointer, entity.identifier, mode, add)

    topics = indexes["topic"]
    elements = indexes["element"]
    types = indexes["type"]
    subvariants = indexes["subvariant"]
    blocks = indexes["block"]

    for document in docs:
        data = document.data
        if not isinstance(data, dict):
            continue
        if document.definition == "topic":
            _check_topic(document, data, elements, types, mode, add)
        elif document.definition == "variants_file":
            _check_variants_file(document, data, types, subvariants, blocks, mode, add)
        elif document.definition == "block_frontmatter":
            _check_reference(
                document,
                "/topic_id",
                data.get("topic_id"),
                topics,
                "Ссылка topic_id указывает на несуществующую тему.",
                mode,
                add,
            )
        elif document.definition == "test":
            _check_test(document, data, types, mode, add)
        elif document.definition == "lesson_template":
            _check_lesson(document, data, topics, types, subvariants, blocks, mode, add)
        elif document.definition == "transfer_map":
            _check_transfer_map(document, data, all_ids, types, mode, add)

    _check_prerequisite_cycles(entities, elements, mode, add)
    _check_skill_appearances(docs, types, mode, add)
    problems.extend(_answer_format_problems(docs))
    return problems


def _collect_entities(documents: Iterable[Document]) -> list[Entity]:
    entities: list[Entity] = []
    for document in documents:
        data = document.data
        if not isinstance(data, dict):
            continue
        if document.definition == "topic":
            _append_entity(entities, "topic", document, "/id", data)
            for index, element in _objects(data.get("elements")):
                _append_entity(entities, "element", document, f"/elements/{index}/id", element)
            for type_index, task_type in _objects(data.get("types")):
                _append_entity(entities, "type", document, f"/types/{type_index}/id", task_type)
                for subvariant_index, subvariant in _objects(task_type.get("subvariants")):
                    _append_entity(
                        entities,
                        "subvariant",
                        document,
                        f"/types/{type_index}/subvariants/{subvariant_index}/id",
                        subvariant,
                    )
        elif document.definition == "variants_file":
            for index, variant in _objects(data.get("variants")):
                _append_entity(entities, "variant", document, f"/variants/{index}/id", variant)
        elif document.definition == "block_frontmatter":
            _append_entity(entities, "block", document, "/id", data)
        elif document.definition == "test":
            _append_entity(entities, "test", document, "/id", data)
        elif document.definition == "lesson_template":
            _append_entity(entities, "lesson", document, "/id", data)
            for index, stage in _objects(data.get("stages")):
                _append_entity(entities, "stage", document, f"/stages/{index}/id", stage)
    return entities


def _append_entity(
    entities: list[Entity], kind: str, document: Document, pointer: str, data: CatalogData
) -> None:
    identifier = data.get("id")
    if isinstance(identifier, str):
        entities.append(Entity(kind, identifier, document, pointer, data))


def _objects(value: Any) -> Iterator[tuple[int, CatalogData]]:
    if not isinstance(value, list):
        return iter(())
    return ((index, item) for index, item in enumerate(value) if isinstance(item, dict))


def _entity_sort_key(entity: Entity) -> tuple[str, str]:
    return entity.document.file, entity.pointer


def _check_topic(
    document: Document,
    data: CatalogData,
    elements: dict[str, list[Entity]],
    types: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    for index, reference in _values(data.get("prerequisite_elements")):
        _check_reference(
            document,
            f"/prerequisite_elements/{index}",
            reference,
            elements,
            "Предпосылка темы указывает на несуществующий элемент.",
            mode,
            add,
        )

    for type_index, task_type in _objects(data.get("types")):
        _check_reference(
            document,
            f"/types/{type_index}/primary_element",
            task_type.get("primary_element"),
            elements,
            "Основной элемент типа не существует.",
            mode,
            add,
        )

        for prerequisite_index, prerequisite in _values(task_type.get("prerequisites")):
            pointer = f"/types/{type_index}/prerequisites/{prerequisite_index}"
            _check_temporary(document, pointer, prerequisite, mode, add)
            if not isinstance(prerequisite, str):
                continue
            if prerequisite in elements:
                continue
            if _looks_like_element_id(prerequisite):
                add(
                    Problem(
                        "dangling_ref",
                        document.file,
                        pointer,
                        "Предпосылка типа указывает на несуществующий элемент.",
                    )
                )
            elif prerequisite in types or _looks_like_catalog_id(prerequisite):
                add(
                    Problem(
                        "prereq_not_element",
                        document.file,
                        pointer,
                        "Предпосылка типа должна ссылаться только на элемент.",
                    )
                )

        levels = task_type.get("levels")
        if isinstance(levels, dict):
            base = _hours(levels.get("base"))
            automatism = _hours(levels.get("automatism"))
            if base is not None and automatism is not None and automatism < base:
                add(
                    Problem(
                        "level_hours_order",
                        document.file,
                        f"/types/{type_index}/levels/automatism/hours",
                        "Часы automatism не могут быть меньше часов base.",
                    )
                )


def _hours(value: Any) -> float | int | None:
    if not isinstance(value, dict):
        return None
    hours = value.get("hours")
    if isinstance(hours, (int, float)) and not isinstance(hours, bool):
        return hours
    return None


def _check_variants_file(
    document: Document,
    data: CatalogData,
    types: dict[str, list[Entity]],
    subvariants: dict[str, list[Entity]],
    blocks: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    _check_reference(
        document,
        "/type",
        data.get("type"),
        types,
        "Файл вариантов указывает на несуществующий тип.",
        mode,
        add,
    )
    for index, variant in _objects(data.get("variants")):
        _check_reference(
            document,
            f"/variants/{index}/subvariant",
            variant.get("subvariant"),
            subvariants,
            "Вариант указывает на несуществующий подвариант.",
            mode,
            add,
        )
        _check_reference(
            document,
            f"/variants/{index}/theory_block",
            variant.get("theory_block"),
            blocks,
            "Вариант указывает на несуществующий блок теории.",
            mode,
            add,
        )


def _check_test(
    document: Document,
    data: CatalogData,
    types: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    for index, appearance in _objects(data.get("appearances")):
        _check_reference(
            document,
            f"/appearances/{index}/type",
            appearance.get("type"),
            types,
            "Появление теста указывает на несуществующий тип.",
            mode,
            add,
        )


def _check_lesson(
    document: Document,
    data: CatalogData,
    topics: dict[str, list[Entity]],
    types: dict[str, list[Entity]],
    subvariants: dict[str, list[Entity]],
    blocks: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    for field in ("topics", "review"):
        for index, reference in _values(data.get(field)):
            _check_reference(
                document,
                f"/{field}/{index}",
                reference,
                topics,
                "Шаблон урока указывает на несуществующую тему.",
                mode,
                add,
            )

    for stage_index, stage in _objects(data.get("stages")):
        for block_index, reference in _values(stage.get("blocks")):
            _check_reference(
                document,
                f"/stages/{stage_index}/blocks/{block_index}",
                reference,
                blocks,
                "Этап урока указывает на несуществующий блок.",
                mode,
                add,
            )
        for practice_index, practice in _objects(stage.get("practice")):
            base = f"/stages/{stage_index}/practice/{practice_index}"
            _check_reference(
                document,
                f"{base}/type",
                practice.get("type"),
                types,
                "Практика урока указывает на несуществующий тип.",
                mode,
                add,
            )
            _check_reference(
                document,
                f"{base}/subvariant",
                practice.get("subvariant"),
                subvariants,
                "Практика урока указывает на несуществующий подвариант.",
                mode,
                add,
            )
            _check_reference(
                document,
                f"{base}/theory_block",
                practice.get("theory_block"),
                blocks,
                "Практика урока указывает на несуществующий блок теории.",
                mode,
                add,
            )

    homework = data.get("homework")
    if not isinstance(homework, dict):
        return
    for item_index, item in _objects(homework.get("items")):
        for series_index, series in _objects(item.get("series")):
            _check_reference(
                document,
                f"/homework/items/{item_index}/series/{series_index}/type",
                series.get("type"),
                types,
                "Рецепт домашней работы указывает на несуществующий тип.",
                mode,
                add,
            )


def _check_transfer_map(
    document: Document,
    data: CatalogData,
    all_ids: dict[str, list[Entity]],
    types: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    for entry_index, entry in _objects(data.get("entries")):
        for field in ("from", "to"):
            for value_index, reference in _values(entry.get(field)):
                _check_reference(
                    document,
                    f"/entries/{entry_index}/{field}/{value_index}",
                    reference,
                    all_ids,
                    "Карта переноса указывает на несуществующий идентификатор.",
                    mode,
                    add,
                )
        _check_reference(
            document,
            f"/entries/{entry_index}/subvariants_to",
            entry.get("subvariants_to"),
            types,
            "Карта переноса указывает на несуществующий тип.",
            mode,
            add,
        )


def _check_reference(
    document: Document,
    pointer: str,
    value: Any,
    index: dict[str, list[Entity]],
    message: str,
    mode: str,
    add: Any,
) -> None:
    _check_temporary(document, pointer, value, mode, add)
    if isinstance(value, str) and value not in index:
        add(Problem("dangling_ref", document.file, pointer, message))


def _check_temporary(
    document: Document, pointer: str, value: Any, mode: str, add: Any
) -> None:
    if mode == "main" and isinstance(value, str) and value.startswith("tmp:"):
        add(
            Problem(
                "tmp_key_in_main",
                document.file,
                pointer,
                "Временный ключ tmp: допускается только в режиме pr.",
            )
        )


def _values(value: Any) -> Iterator[tuple[int, Any]]:
    if not isinstance(value, list):
        return iter(())
    return enumerate(value)


def _check_prerequisite_cycles(
    entities: Iterable[Entity],
    elements: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    del mode
    type_entities = sorted(
        (entity for entity in entities if entity.kind == "type"),
        key=_entity_sort_key,
    )
    primary_to_types: dict[str, list[Entity]] = defaultdict(list)
    for entity in type_entities:
        primary = entity.data.get("primary_element")
        if isinstance(primary, str) and primary in elements:
            primary_to_types[primary].append(entity)

    graph: dict[str, list[str]] = defaultdict(list)
    edge_sources: dict[tuple[str, str], list[tuple[Document, str]]] = defaultdict(list)
    for entity in type_entities:
        prerequisites = entity.data.get("prerequisites")
        for prerequisite_index, prerequisite in _values(prerequisites):
            if not isinstance(prerequisite, str) or prerequisite not in elements:
                continue
            for successor in primary_to_types.get(prerequisite, []):
                graph[entity.identifier].append(successor.identifier)
                edge_sources[(entity.identifier, successor.identifier)].append(
                    (
                        entity.document,
                        f"{entity.pointer.rsplit('/id', 1)[0]}/prerequisites/{prerequisite_index}",
                    )
                )

    for node in graph:
        graph[node] = sorted(set(graph[node]))

    reported: set[frozenset[str]] = set()
    for cycle in _find_cycles(graph):
        members = frozenset(cycle[:-1])
        if members in reported:
            continue
        reported.add(members)
        source, target = cycle[-2], cycle[-1]
        document, pointer = sorted(
            edge_sources[(source, target)],
            key=lambda item: (item[0].file, item[1]),
        )[0]
        add(
            Problem(
                "prereq_cycle",
                document.file,
                pointer,
                f"Цикл предпосылок: {' → '.join(cycle)}.",
            )
        )


def _find_cycles(graph: dict[str, list[str]]) -> Iterator[list[str]]:
    """Yield one deterministic simple cycle for each cyclic component."""

    visited: set[str] = set()
    active: list[str] = []
    active_set: set[str] = set()

    def visit(node: str) -> Iterator[list[str]]:
        visited.add(node)
        active.append(node)
        active_set.add(node)
        for successor in graph.get(node, []):
            if successor in active_set:
                start = active.index(successor)
                yield active[start:] + [successor]
            elif successor not in visited:
                yield from visit(successor)
        active.pop()
        active_set.remove(node)

    for node in sorted(graph):
        if node not in visited:
            yield from visit(node)


def _check_skill_appearances(
    documents: Iterable[Document],
    types: dict[str, list[Entity]],
    mode: str,
    add: Any,
) -> None:
    del mode
    skill_ids = {
        identifier
        for identifier, occurrences in types.items()
        if any(occurrence.data.get("kind") == "skill" for occurrence in occurrences)
    }
    for document in documents:
        if document.definition != "test" or not isinstance(document.data, dict):
            continue
        for index, appearance in _objects(document.data.get("appearances")):
            type_id = appearance.get("type")
            if isinstance(type_id, str) and type_id in skill_ids:
                add(
                    Problem(
                        "skill_has_appearance",
                        document.file,
                        f"/appearances/{index}/type",
                        "Тип skill не может встречаться в появлении теста.",
                    )
                )


def _answer_format_problems(documents: Iterable[Document]) -> list[Problem]:
    """Return CS-R16 problems after package answer rules have been collected."""

    formats_by_package: dict[Path, set[str]] = {}
    for document in documents:
        if document.definition != "subject_package" or not isinstance(document.data, dict):
            continue
        formats_by_package[document.package_dir] = {
            item["id"]
            for _, item in _objects(document.data.get("answer_formats"))
            if isinstance(item.get("id"), str)
        }

    problems: list[Problem] = []
    for document in documents:
        if document.definition != "topic" or not isinstance(document.data, dict):
            continue
        known = formats_by_package.get(document.package_dir)
        if known is None:
            continue
        for type_index, task_type in _objects(document.data.get("types")):
            answer_format = task_type.get("answer_format")
            if isinstance(answer_format, str) and answer_format not in known:
                problems.append(
                    Problem(
                        "answer_format_unknown",
                        document.file,
                        f"/types/{type_index}/answer_format",
                        "Формат ответа типа не объявлен в правилах пакета.",
                    )
                )
    return problems
