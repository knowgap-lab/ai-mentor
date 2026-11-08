---
status: закончил
agent: claude
date: 2026-11-09 10:30
task: Вынести таблицы примеров в YAML (answer-rules, answer-check, attempts, schema)
refs: [docs/handoff/entries/2026-11-09-0940-claude-examples-start.md, D72, D99, D133]
touches: [docs/catalog/answer-rules.examples.yaml, docs/catalog/schema.examples.yaml, docs/platform/answer-check.examples.yaml, docs/platform/attempts.examples.yaml, tools/, docs/contracts/README.md, docs/catalog/answer-rules.md]
---

# Примеры в YAML готовы для первых четырёх модулей

**Сделано.** `answer-rules.examples.yaml` (45 строк с эталонной функцией оценки `tools/examples/check_answer_rules.py`), `answer-check.examples.yaml` (15 сценариев), `attempts.examples.yaml` (23 сценария), `schema.examples.yaml` (29 строк, из них 12 проверяются по регулярным выражениям схемы). Скрипт `tools/examples/check_scenarios.py` сверяет шаги сценариев с `core-api.yaml`: операция существует, запрос и тело ответа по схеме, код ответа описан, правило есть в спецификации. Запуск всего: `tools/check.sh`; отрицательные тесты каждой проверки проведены.

**Замечание.** В таблице answer-rules для `ct.digits` ввод `15` против ключа `124` имел подпись «разность 2»; верно 3 (вердикт «неверно» не менялся). Исправлено в answer-rules.md.

**Ограничения.** Эталонная функция оценки написана мной по тем же правилам, что и примеры, поэтому она независима от будущего кода Go, но не от моего прочтения правил: репетитору стоит просмотреть примеры answer-rules по существу (D99). Список латинских двойников букв (AR-R9, «и т. п.») взят мной: A, B, C, E, H, K, M, O, P, T, X.

**Не сделано.** Примеры для остальных модулей (recheck, auth, privacy, notifications, web, ops, queue, rt-import, lessons, homework, conspect, plan); настройка CI; генерация типов Go/Python/TS.

## Дальше

Настройка CI (запуск `tools/check.sh`, генерация типов), примеры для остальных модулей по мере реализации. Создайте запись «начал».
