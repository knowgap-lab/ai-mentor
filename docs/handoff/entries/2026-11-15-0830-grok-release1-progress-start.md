---
status: начал
agent: grok
date: 2026-11-15 08:30
task: Сверить головы PR #6–#10 с git и CI и записать прогресс выпуска 1
refs: [docs/handoff/LATEST.md, briefs/RELEASE1-GOAL.md, D320]
touches: [docs/handoff/release1-progress.md, docs/handoff/LATEST.md, .github/workflows/code-check.yml, .github/workflows/docs-check.yml]
---

# Начал: прогресс выпуска 1

**Беру в работу.** Сверить локальный git, головы PR #6–#10 на GitHub и выводы CI, записать `docs/handoff/release1-progress.md`. Если ни один workflow не слушает `merge_group`, добавить триггер. Настройку очереди слияния в репозитории не включать.

**План.**
- Прочитать головы PR #6–#10 и последние check runs.
- Проверить, что головы #7 и #8 являются предками #6.
- Найти `briefs/RELEASE1-GOAL.md`.
- Проверить ruleset `protect-main` и GraphQL `mergeQueue`.
- Записать прогресс и, при отсутствии триггера, добавить `merge_group` в обязательные workflow.
