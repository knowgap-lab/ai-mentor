#!/usr/bin/env python3
"""Проверяет legal/consent-*.md (PV-R9, D194): обязательные разделы; опубликованный текст без пометок [ЗАПОЛНИТЬ].
Запуск: python3 tools/examples/check_consent.py"""
import pathlib, re, sys

root = pathlib.Path(__file__).resolve().parents[2]
REQUIRED = ["Какие данные хранятся", "Зачем", "Кто видит данные", "Срок хранения", "Внешние сервисы", "Как отозвать согласие"]
bad = 0
files = sorted((root / "legal").glob("consent-*.md"))
if not files:
    bad += 1; print("FAIL нет файла legal/consent-<номер>.md")
for f in files:
    text = f.read_text(encoding="utf8")
    fm = re.match(r"---\n(.*?)\n---\n", text, re.S)
    meta = dict(l.split(": ", 1) for l in fm.group(1).splitlines()) if fm else {}
    heads = set(re.findall(r"^## (.+)$", text, re.M))
    for r in REQUIRED:
        if r not in heads:
            bad += 1; print(f"FAIL {f.name}: нет раздела «{r}»")
    if "7 дней" not in text:
        bad += 1; print(f"FAIL {f.name}: нет указания «7 дней» (PV-R9)")
    if meta.get("status") == "published" and "[ЗАПОЛНИТЬ" in text:
        bad += 1; print(f"FAIL {f.name}: опубликованный текст содержит [ЗАПОЛНИТЬ]")
    if meta.get("version") != re.sub(r"\D", "", f.stem):
        bad += 1; print(f"FAIL {f.name}: version в шапке не совпадает с номером файла")
    if not (meta.get("status") in ("draft", "published")):
        bad += 1; print(f"FAIL {f.name}: status должен быть draft или published")
print(f"{'FAILED' if bad else 'OK'}: {bad} ошибок, файлов {len(files)}")
sys.exit(1 if bad else 0)
