"""Validate the published Release 1 queue, external actions and new journal records."""

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
HANDOFF = ROOT / "docs/handoff"
STATES = {"Queued", "In Progress", "For Review", "Done", "Canceled"}
ENTRY_STATES = {"начал", "закончил", "прервано", "заблокировано"}
# Historical records are append-only and predate YAML validation.
VALIDATE_ENTRIES_FROM = "2026-11-22-"


def yaml_list(path, fields, states, errors):
    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
    except yaml.YAMLError as exc:
        errors.append(f"{path.name}: {exc}")
        return
    if not isinstance(data, list) or not data:
        errors.append(f"{path.name}: ожидался непустой список действий")
        return
    ids = set()
    for item in data:
        if not isinstance(item, dict) or any(
            not isinstance(item.get(key), str) or not item[key].strip()
            for key in fields
        ):
            errors.append(f"{path.name}: обязательные поля {', '.join(fields)}")
            continue
        if item["id"] in ids or item["status"] not in states:
            errors.append(f"{path.name}: повтор ID или неверный статус {item['id']}")
        ids.add(item["id"])


def check_queue(path, errors):
    text = path.read_text(encoding="utf-8")
    header = next(
        (line for line in text.splitlines() if line.startswith("| ID в Git |")), ""
    )
    if not header:
        errors.append(f"{path.name}: таблица очереди отсутствует")
        return
    table = text.split(header, 1)[1].lstrip("\n").split("\n\n", 1)[0].splitlines()
    rows = table[1:]
    if not rows:
        errors.append(f"{path.name}: таблица очереди пуста")
        return
    ids = set()
    for row in rows:
        parts = [
            value.strip().strip("`").strip() for value in row.strip("|").split("|")
        ]
        if len(parts) != 7:
            errors.append(f"{path.name}: задача должна содержать семь полей")
            continue
        task, _, state, owner, branch, proof, step = parts
        if not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", task) or task in ids:
            errors.append(f"{path.name}: неверный или повторный ID {task}")
        ids.add(task)
        if state not in STATES or not proof or not step:
            errors.append(
                f"{path.name}: {task} без допустимого статуса, материалов или следующего шага"
            )
        if state in {"In Progress", "For Review"} and (
            owner in {"", "—", "-"} or branch in {"", "—", "-"}
        ):
            errors.append(f"{path.name}: {task} без ответственного или ветки")
        if state in {"In Progress", "For Review", "Done"} and not re.search(
            r"\b[0-9a-f]{7,40}\b", proof
        ):
            errors.append(f"{path.name}: {task} без SHA в материалах")


def check_entry(path, errors):
    text = path.read_text(encoding="utf-8")
    front, separator, body = text.removeprefix("---\n").partition("\n---\n")
    try:
        fields = (
            yaml.safe_load(front) if text.startswith("---\n") and separator else None
        )
    except yaml.YAMLError as exc:
        errors.append(f"{path.name}: {exc}")
        return
    if (
        not isinstance(fields, dict)
        or fields.get("status") not in ENTRY_STATES
        or any(not fields.get(key) for key in ("agent", "date", "task"))
        or any(not isinstance(fields.get(key), list) for key in ("refs", "touches"))
    ):
        errors.append(f"{path.name}: неверные поля записи журнала")
        return
    if fields["status"] != "начал" and not re.search(
        r"^## Дальше\s*$", body, re.MULTILINE
    ):
        errors.append(f"{path.name}: отсутствует раздел «Дальше»")
    for target in re.findall(r"\]\(([^)#\s]+)(?:#[^)]*)?\)", body):
        if "://" not in target and not (path.parent / target).exists():
            errors.append(f"{path.name}: битая ссылка {target}")


def main():
    errors = []
    yaml_list(
        HANDOFF / "release1-actions.yaml",
        ("id", "status", "owner", "action", "verification"),
        {"pending", "done"},
        errors,
    )
    check_queue(HANDOFF / "release1-progress.md", errors)
    for path in sorted((HANDOFF / "entries").glob("*.md")):
        if path.name >= VALIDATE_ENTRIES_FROM:
            check_entry(path, errors)
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("OK: очередь выпуска и журнал передачи")
    return 0


if __name__ == "__main__":
    sys.exit(main())
