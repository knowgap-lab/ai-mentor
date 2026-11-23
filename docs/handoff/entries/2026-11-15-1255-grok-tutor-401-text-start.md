---
status: начал
agent: grok
date: 2026-11-15 12:55
task: На 401 списка учеников показать «Войдите ещё раз»
refs: [docs/handoff/entries/2026-11-15-1250-grok-tutor-list-error-done.md, WB-R6, core/internal/compose/handler.go]
touches: [web/src/tutor/TutorPage.tsx, docs/handoff/LATEST.md]
---

# Начал: текст при 401 списка

**Беру в работу.** `GET /students` без сеанса отвечает `401 unauthorized`. Этого кода нет в таблице `web.md`, поэтому форма «Войти» оставалась без текста.

**План.** Показать «Войдите ещё раз», код `unauthorized` на экран не выводить.
