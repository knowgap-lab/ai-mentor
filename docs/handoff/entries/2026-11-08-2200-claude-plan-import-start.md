---
status: начал
agent: claude
date: 2026-11-08 22:00
task: Написать спецификацию docs/catalog/plan-import.md
refs: [docs/handoff/entries/2026-11-08-2145-claude-docs-foundation.md, D100, D105, D106, D190, D192, D193, D202, D204]
touches: [docs/catalog/plan-import.md, docs/decisions/log.md, docs/handoff/LATEST.md]
---

# Спецификация импорта плана

**Беру в работу.** Разовый импорт статичного плана (`PLAN.md` и файлы уроков из ct-2027) в каталог и в `core.*` как версию плана 0.

**План.**
- изучить форму данных в ct-2027 (PLAN.md, frontmatter L-файлов, graph.json);
- написать спецификацию по шаблону;
- открытые вопросы решить с пользователем, решения записать в журнал;
- закончить запись, обновить LATEST.
