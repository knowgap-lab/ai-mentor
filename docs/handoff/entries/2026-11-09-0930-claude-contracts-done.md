---
status: закончил
agent: claude
date: 2026-11-09 09:30
task: Написать контракты в docs/contracts/
refs: [docs/handoff/entries/2026-11-09-0810-claude-contracts-start.md, D97, D133, D269]
touches: [docs/contracts/, docs/platform/attempts.md, docs/platform/queue.md, docs/decisions/log.md, docs/handoff/LATEST.md, docs/README.md]
---

# Контракты готовы

**Сделано.** В `docs/contracts/`: `catalog.schema.json`, `subject-package.schema.json`, `events.schema.json` (26 событий), `queue.schema.json`, `core-api.yaml` (48 путей), `internal-api.yaml` (6 путей). Проверка: `python3 docs/contracts/check.py` (примеры в `fixtures/*.cases.yaml`, структура OpenAPI, правило WB-R4 «ключи ответов только в трёх схемах»). Все проверки проходят; проверка правила WB-R4 проверена отрицательным тестом.

**Что вывел сам (D269).** Пути и поля, кроме названных в спецификациях, предложены мной и ждут ревью напарника; события `lesson_*`, `catalog_version_loaded`, `student_unarchived` и др. названы мной. Тип идентификаторов в событиях — непрозрачная строка. В AT-R5 добавлен источник `unsolved`; в queue.md исправлено имя файла `compute-views.schema.json`.

**Не сделано.** `compute-views.schema.json` (выпуск 3); предложения агента в internal-api (выпуск 4); генерация типов для Go/Python/TS (настройка CI); `*.examples.yaml` для спецификаций (сейчас таблицы внутри Markdown).

## Дальше

YAML-примеры для первых модулей по порядку D159: schema, answer-rules, answer-check, attempts; затем настройка CI. Создайте запись «начал».
