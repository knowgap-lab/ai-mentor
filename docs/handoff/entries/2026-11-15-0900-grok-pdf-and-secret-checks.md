---
status: закончил
agent: grok
date: 2026-11-15 09:00
task: Закрыть две дыры проверки PDF и секретов
refs: [2026-11-15-0856-grok-journal-interface]
touches: [tools/release1_done.py, tools/release1_done_test.py]
---

# Закончил: оба PDF-теста и ссылки Terraform

**Начало работы:** замечания ревью к `b60892b`. Отдельной записи «начал» нет.

**Сделано.**
- PDF проходит, только если оба `TestWB_R28_S8_SheetPdf` и `TestWB_R29_S8_TutorPdfOmitsKey` напечатали `--- PASS:`. Пропуск любого по-прежнему провал.
- `literal_secret` не считает секретом `var.` и `local.`. Литерал токена по-прежнему провал.

**Не сделано.**
- Падения `code-check` в `cmd/server`, `attempts`, `internalapi` и `plan` были на дереве демо до этой проверки. Замороженные тесты не менялись.

**Решения.** нет

**Открытые вопросы.** нет

**Проверки.** `python3 tools/release1_done_test.py`.

## Дальше
PR #17 не сливать. Экранный провал и пропуск PDF без Docker остаются.
