---
status: закончил
agent: grok
date: 2026-11-15 08:45
task: Записать сверку PR #6–#10 и добавить триггер merge_group
refs: [docs/handoff/entries/2026-11-15-0830-grok-release1-progress-start.md, D320]
touches: [docs/handoff/release1-progress.md, docs/handoff/LATEST.md, .github/workflows/code-check.yml, .github/workflows/docs-check.yml]
---

# Закончил: прогресс выпуска 1

**Начало работы:** [2026-11-15-0830-grok-release1-progress-start.md](2026-11-15-0830-grok-release1-progress-start.md)

**Сделано.**
- Сверка записана в [release1-progress.md](../release1-progress.md). Головы #7 `7c627376` и #8 `b342ad6f` — предки #6 `61f6fd5e`. Головы #9 и #10 в #6 не входят.
- `briefs/RELEASE1-GOAL.md` на полученных ветках нет.
- Очередь слияния выключена: GraphQL `mergeQueue(branch: "main")` равен `null`, в ruleset `protect-main` нет правила `merge_queue`.
- Триггер `merge_group` добавлен в `.github/workflows/code-check.yml` и `.github/workflows/docs-check.yml`.

**Не сделано.**
- Настройка **Require merge queue** в ruleset `protect-main` остаётся владельцу.
- Четыре красных программных теста на `61f6fd5` не классифицированы.
- PR #6 не сливался.

**Решения.** нет

**Открытые вопросы.** Владельцу: включить **Require merge queue** в [ruleset protect-main](https://github.com/knowgap-lab/ai-mentor/rules/24420268). Строки D320 в журнале решений на `61f6fd5` нет.

**Проверки.** `git fetch` веток PR, `git merge-base --is-ancestor` для #7 и #8, GitHub check runs и логи `code-check`. Локальные программные тесты этой сессией не запускались.

## Дальше
На `61f6fd5` воспроизвести и классифицировать четыре красных программных теста по D320. PR #6 не сливать, пока `code-check` красный. Затем переложить #9 и #10 на `61f6fd5`.
