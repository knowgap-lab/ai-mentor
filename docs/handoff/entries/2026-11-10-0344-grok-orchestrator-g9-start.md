---
status: начал
agent: grok
date: 2026-11-10 03:44
task: расписание и уроки (G9)
refs:
  - briefs/r1-g9.md
  - docs/personalization/lessons.md
  - docs/handoff/entries/2026-11-10-0343-grok-orchestrator-g5-g8.md
touches:
  - briefs/r1-g9.md
  - core/internal/lessons/
  - migrations/08001_lessons.sql
---

# Беру G9

**Беру в работу.** Расписание, снимок урока и две отметки по `lessons.md`. Рецепт ДЗ и PDF не входят в пакет.

**План.**

- Бриф `briefs/r1-g9.md` уже написан.
- Тест-исполнитель пишет тесты на `lessons-01`…`lessons-21` в worktree `feat/r1-g9`.
- Заморозка, затем другой исполнитель пишет код.
- Приёмка: diff заморозки пуст, `go test ./internal/lessons/` зелёный, слияние `--no-ff` в `feat/release1`.
