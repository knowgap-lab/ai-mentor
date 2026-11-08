---
status: закончил
agent: claude
date: 2026-11-09 07:20
task: Написать спецификацию docs/personalization/lessons.md
refs: [docs/handoff/entries/2026-11-09-0650-claude-lessons-start.md, D8, D9, D82, D96, D105, D106, D152, D264]
touches: [docs/personalization/lessons.md, docs/personalization/homework.md, docs/decisions/log.md, docs/handoff/LATEST.md, docs/README.md]
---

# Спецификация уроков и расписания готова

**Сделано.** [lessons.md](../../personalization/lessons.md), статус «на ревью»: LS-R1…LS-R24 (+LS-R19a), примеры, ошибки. В `homework.md` HW-R11 дополнено: вариант, использованный на уроке, считается виденным (D193).

**Решения.** Всё из принятых (D8, D9, D82, D96, D105, D106, D120, D148, D152, D153, D193). По умолчанию: D264 (отметки можно исправить, пока ДЗ не выдано). События `lesson_*` и коды ошибок названы мной, для ревью напарника.

**Замечание для composer.** В шаблоне урока есть вид этапа `exit_test`, а мини-теста в платформе нет (D257): этап остаётся отрезком времени, результатов нет (LS-R23). Если нужно, поправить композер.

**Не сделано.** `lessons.examples.yaml`; `conspect.md`, `plan.md`.

## Дальше

`personalization/conspect.md`, `plan.md` (статичный план), затем контракты в `docs/contracts/` и YAML-примеры первых модулей. Создайте запись «начал».
