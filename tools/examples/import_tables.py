#!/usr/bin/env python3
"""Однократный перенос таблиц «4. Таблицы примеров» спецификаций в <модуль>.examples.yaml (D133).

Строка таблицы (Правило | Ситуация | Результат) → {id, rule, given, expect}. Смысл не меняется.
После переноса YAML правится вручную; таблица в Markdown остаётся краткой справкой.
Запуск: python3 tools/examples/import_tables.py docs/platform/recheck.md ...
"""
import pathlib, re, sys
import yaml

def split_row(line):
    cells = re.split(r"(?<!\\)\|", line.strip().strip("|"))
    return [c.strip().replace("\\|", "|") for c in cells]

def convert(md_path):
    text = md_path.read_text(encoding="utf8")
    i = text.index("## 4. Таблицы примеров")
    j = text.index("## 5.", i)
    rows = [l for l in text[i:j].splitlines() if l.startswith("|")][2:]
    stem = md_path.stem
    out = []
    for n, line in enumerate(rows, 1):
        cells = split_row(line)
        assert len(cells) == 3, f"{md_path}: строка {n}: {len(cells)} ячеек: {line[:80]}"
        out.append({"id": f"{stem}-{n:02d}", "rule": cells[0], "given": cells[1], "expect": cells[2]})
    header = (f"# Примеры к {md_path.parent.name}/{md_path.name} (D133). Перенесено из раздела «Таблицы примеров» без изменения смысла.\n"
              "# Поля: id, rule (номера правил спецификации), given (ситуация или вход), expect (ожидаемый результат).\n"
              "# Проверяет tools/examples/check_text_examples.py: число строк, наличие правил в спецификации, уникальность id.\n")
    body = yaml.safe_dump(out, allow_unicode=True, sort_keys=False, width=1000)
    md_path.with_suffix(".examples.yaml").write_text(header + body, encoding="utf8")
    return len(out)

if __name__ == "__main__":
    for p in sys.argv[1:]:
        print(p, convert(pathlib.Path(p)))
