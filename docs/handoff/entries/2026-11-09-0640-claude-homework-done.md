---
status: закончил
agent: claude
date: 2026-11-09 06:40
task: Написать спецификацию docs/personalization/homework.md
refs: [docs/handoff/entries/2026-11-09-0600-claude-homework-start.md, D255, D256, D258, D261, D262, D263]
touches: [docs/personalization/homework.md, docs/decisions/log.md, docs/handoff/LATEST.md, docs/README.md]
---

# Спецификация сборки ДЗ готова

**Сделано.** [homework.md](../../personalization/homework.md), статус «на ревью»: HW-R1…HW-R20, примеры, ошибки, открытых вопросов нет.

**Решения пользователя.** D255–D261 (методика), D262 (размер ДЗ по рецепту урока, только объяснённые типы; «вдвое» — подсказка), D263 (сколько задач добавлять: столько, сколько не удалось с первой попытки).

**Что вывел сам (D228).** Что «верно» считается по первой попытке (из D45); резерв срезов в ДЗ не берётся (подтверждено VR-R3). Код `lesson_not_marked` и другие коды ошибок — мои, для ревью.

**Не сделано.** `homework.examples.yaml`; `lessons.md` (даёт отметки и состав урока), `conspect`, `plan`.

## Дальше

`personalization/lessons.md`, затем `conspect.md` и `plan.md`. Методические элементы из L-файлов, которых нет в D255–D263, сначала показать пользователю (D228). Создайте запись «начал».
