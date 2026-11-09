"""Проверка JSON Schema и выбор кода проблемы."""

from __future__ import annotations

import copy
import json
from collections.abc import Iterable
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, ValidationError

from .load import Document, _PACKAGE_DEFINITIONS
from .problem import Problem


_ID_FIELD_NAMES = {
    "id",
    "primary_element",
    "prerequisites",
    "prerequisite_elements",
    "subvariant",
    "theory_block",
    "topic_id",
    "type",
    "topics",
    "review",
    "blocks",
    "from",
    "to",
    "subvariants_to",
}


def schema_problems(documents: Iterable[Document]) -> list[Problem]:
    catalog_schema, subject_schema = _read_schemas()
    validators: dict[str, Draft202012Validator] = {
        definition: _definition_validator(catalog_schema, definition)
        for definition in set(_PACKAGE_DEFINITIONS.values())
    }
    validators["subject_package"] = Draft202012Validator(subject_schema)

    problems: list[Problem] = []
    for document in documents:
        validator = validators[document.definition]
        errors = sorted(
            validator.iter_errors(document.data),
            key=lambda error: (
                tuple(str(part) for part in error.absolute_path),
                tuple(str(part) for part in error.absolute_schema_path),
                error.message,
            ),
        )
        for error in errors:
            problems.extend(_problems_from_schema_error(document, error))
    return problems


def _repository_root() -> Path:
    """Return the repository root from ``content/catalog_ci``."""

    return Path(__file__).resolve().parents[2]


def _read_schemas() -> tuple[dict[str, Any], dict[str, Any]]:
    contracts = _repository_root() / "docs" / "contracts"
    try:
        catalog = json.loads((contracts / "catalog.schema.json").read_text(encoding="utf-8"))
        subject = json.loads(
            (contracts / "subject-package.schema.json").read_text(encoding="utf-8")
        )
    except (OSError, json.JSONDecodeError) as error:  # pragma: no cover - repository invariant
        raise RuntimeError(f"Не удалось загрузить JSON Schema: {error}") from error
    return catalog, subject


def _definition_validator(schema: dict[str, Any], definition: str) -> Draft202012Validator:
    """Build a validator whose root points at one catalog ``$defs`` entry."""

    root = copy.deepcopy(schema)
    root["$ref"] = f"#/$defs/{definition}"
    return Draft202012Validator(root)


def _problems_from_schema_error(document: Document, error: ValidationError) -> list[Problem]:
    pointer = _pointer(error.absolute_path)

    if error.validator == "additionalProperties":
        return _additional_property_problems(document, error, pointer)

    if _is_citation_error(document, error):
        return [
            Problem(
                "citation_missing",
                document.file,
                pointer,
                "У темы, элемента или типа должна быть хотя бы одна цитата.",
            )
        ]

    if _is_missing_primary_element(document, error):
        return [
            Problem(
                "primary_element_missing",
                document.file,
                pointer,
                "У типа отсутствует основной элемент primary_element.",
            )
        ]

    if _is_id_error(error) and not _is_non_element_prerequisite(document, pointer):
        return [
            Problem(
                "id_invalid",
                document.file,
                pointer,
                "Идентификатор не соответствует формату своего вида.",
            )
        ]

    return [
        Problem(
            "schema_invalid",
            document.file,
            pointer,
            f"Нарушена схема: {error.message}.",
        )
    ]


def _additional_property_problems(
    document: Document, error: ValidationError, parent_pointer: str
) -> list[Problem]:
    instance = error.instance
    schema = error.schema
    if not isinstance(instance, dict) or not isinstance(schema, dict):
        return [
            Problem(
                "field_not_allowed",
                document.file,
                parent_pointer,
                "Поле не допускается схемой.",
            )
        ]

    allowed = set(schema.get("properties", {}))
    unexpected = sorted(str(key) for key in instance if key not in allowed)
    if not unexpected:
        return [
            Problem(
                "field_not_allowed",
                document.file,
                parent_pointer,
                "Поле не допускается схемой.",
            )
        ]
    return [
        Problem(
            "field_not_allowed",
            document.file,
            _join_pointer(parent_pointer, field),
            f"Поле {field!r} не допускается схемой.",
        )
        for field in unexpected
    ]


def _is_citation_error(document: Document, error: ValidationError) -> bool:
    if document.definition != "topic":
        return False
    path = tuple(error.absolute_path)
    if error.validator == "minItems" and path and path[-1] == "citations":
        return _citation_owner_path(path[:-1])
    if error.validator == "required" and "'citations' is a required property" in error.message:
        return _citation_owner_path(path)
    return False


def _citation_owner_path(path: tuple[Any, ...]) -> bool:
    return path == () or (
        len(path) == 2
        and path[0] in {"elements", "types"}
        and isinstance(path[1], int)
    )


def _is_missing_primary_element(document: Document, error: ValidationError) -> bool:
    return (
        document.definition == "topic"
        and error.validator == "required"
        and "'primary_element' is a required property" in error.message
        and len(error.absolute_path) == 2
        and tuple(error.absolute_path)[0] == "types"
        and isinstance(tuple(error.absolute_path)[1], int)
    )


def _is_id_error(error: ValidationError) -> bool:
    path = tuple(error.absolute_path)
    if not path:
        return False
    field = path[-1]
    if isinstance(field, int):
        field = path[-2] if len(path) > 1 else ""
    return isinstance(field, str) and field in _ID_FIELD_NAMES and error.validator != "required"


def _is_non_element_prerequisite(document: Document, pointer: str) -> bool:
    """Avoid an id-format duplicate when the dedicated prerequisite code applies."""

    parts = _pointer_parts(pointer)
    if len(parts) != 4 or parts[0] != "types" or parts[2] != "prerequisites":
        return False
    value = _at_pointer(document.data, pointer)
    return isinstance(value, str) and _looks_like_catalog_id(value) and not _looks_like_element_id(value)


def _pointer(parts: Iterable[Any]) -> str:
    return "".join("/" + _escape_pointer_part(part) for part in parts)


def _join_pointer(pointer: str, part: Any) -> str:
    return f"{pointer}/{_escape_pointer_part(part)}" if pointer else f"/{_escape_pointer_part(part)}"


def _escape_pointer_part(part: Any) -> str:
    return str(part).replace("~", "~0").replace("/", "~1")


def _pointer_parts(pointer: str) -> tuple[str, ...]:
    if not pointer:
        return ()
    return tuple(part.replace("~1", "/").replace("~0", "~") for part in pointer[1:].split("/"))


def _at_pointer(data: Any, pointer: str) -> Any:
    current = data
    for part in _pointer_parts(pointer):
        if isinstance(current, dict):
            current = current.get(part)
        elif isinstance(current, list):
            try:
                current = current[int(part)]
            except (ValueError, IndexError):
                return None
        else:
            return None
    return current


def _looks_like_catalog_id(value: str) -> bool:
    return ":" in value and not value.startswith("tmp:")


def _looks_like_element_id(value: str) -> bool:
    return ".E" in value and _looks_like_catalog_id(value)
