---
status: начал
agent: grok
date: 2026-11-15 12:35
task: Поправить вход и отмену отзыва на экранах W5
refs: [docs/handoff/entries/2026-11-15-1230-grok-tutor-student-card-done.md, PV-R18, WB-R30]
touches: [web/src/tutor/TutorPage.tsx, web/src/tutor/StudentCardPage.tsx, docs/handoff/LATEST.md]
---

# Начал: правки по ревью карточки

**Беру в работу.** Две находки ревью на `feat/r1-tutor-student-card`: обрыв сети на входе открывает «Ученики», успешная отмена отзыва оставляет кнопку.

**План.**
- Ошибку сети на `POST /auth/dev` оставить на форме входа.
- После 204 на отмену отзыва спрятать кнопку: согласие снова действующее (PV-R18).
- Замороженные тесты не менять.
