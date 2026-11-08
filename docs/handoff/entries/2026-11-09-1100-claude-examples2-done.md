---
status: закончил
agent: claude
date: 2026-11-09 11:00
task: Вынести таблицы примеров остальных модулей в YAML
refs: [docs/handoff/entries/2026-11-09-1040-claude-examples2-start.md, D133]
touches: [docs/catalog/, docs/platform/, docs/personalization/, tools/, docs/handoff/LATEST.md]
---

# Примеры всех модулей выпуска 1 в YAML

**Сделано.** Для 16 модулей (variants, loader, plan-import, composer, recheck, auth, privacy, notifications, web, ops, queue, rt-import, lessons, homework, conspect, plan) таблицы «Ключевые строки» перенесены скриптом `tools/examples/import_tables.py` в `<модуль>.examples.yaml` (266 строк, смысл не менялся). Проверка `tools/examples/check_text_examples.py`: число строк равно таблице, правила существуют в спецификации, id уникальны, ячейки не пусты. В `check_scenarios.py` добавлен контроль покрытия: каждое правило из таблицы answer-check и attempts должно встречаться в сценарии; это нашло два пробела (AT-R4, AT-R19), сценарии добавлены.

**Ограничение.** Эти 16 файлов — текстовые тестовые спецификации (`given`/`expect` словами), не исполняемые тесты. Исполняемыми (с запросами по core-api.yaml) пока остаются answer-rules, answer-check, attempts, schema. Остальные доводятся при реализации модуля.

**Не сделано.** Настройка CI (запуск `tools/check.sh`, генерация типов); исполняемые сценарии для остальных модулей.

## Дальше

Настройка CI. Затем по порядку D159 приступать к коду: контракт каталога, загрузчик, страница варианта, проверка в Go, журнал. Создайте запись «начал».
