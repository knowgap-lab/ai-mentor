---
status: начал
agent: grok
date: 2026-11-15 12:45
task: Ошибку HTTP списка учеников оставить на экране «Ученики»
refs: [docs/handoff/entries/2026-11-15-1240-grok-tutor-student-card-fix-done.md, WB-R6, WB-R30]
touches: [web/src/tutor/TutorPage.tsx, web/src/tutor/api-error.ts, web/src/tutor/StudentCardPage.tsx, docs/handoff/LATEST.md]
---

# Начал: ответ списка не сбрасывает вход

**Беру в работу.** `GET /students` с кодом не 2xx после входа возвращал на форму «Войти» без текста. Для 503 это неверно: сеанс уже есть.

**План.**
- 401 — форма входа и текст `login_stale`.
- Другой код — экран «Ученики», текст из таблицы ошибок, кнопка «Повторить».
- Обрыв сети по-прежнему только «Нет связи».
