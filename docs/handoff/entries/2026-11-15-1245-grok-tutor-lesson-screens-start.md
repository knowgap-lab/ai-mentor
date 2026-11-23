---
status: начал
agent: grok
date: 2026-11-15 12:45
task: Экраны «Урок» и «Подготовка» по замороженным тестам W6
refs: [docs/handoff/entries/2026-11-15-1240-grok-tutor-lesson-tests-done.md, web/src/tutor/tutor-lesson.test.tsx, web/src/tutor/preparation.test.tsx, docs/platform/web.md, WB-R2, WB-R4, WB-R6, WB-R22, WB-R30, LS-R3, LS-R4, LS-R6, LS-R13, LS-R14, LS-R16, LS-R20, PL-R4a, PL-R8]
touches: [web/src/tutor/TutorLessonPage.tsx, web/src/tutor/PreparationPage.tsx, web/src/tutor/api-error.ts, web/src/tutor/minsk-time.ts, web/src/App.tsx, web/screens.json, docs/handoff/LATEST.md]
---

# Начал: урок и подготовка

**Беру в работу.** Код-сессия по замороженным тестам W6 на `feat/r1-tutor-lesson-tests` (`95b6540`). Экраны «Урок» и «Подготовка». Тесты W6 и карточка ученика не меняю.

**План.**
- Ветка `feat/r1-tutor-lesson-screens`.
- `TutorLessonPage` на `/tutor/lessons/:lessonId`, ключ «Репетитор/Урок».
- `PreparationPage` на `/tutor/lessons/:lessonId/preparation`, ключ «Репетитор/Подготовка».
- Прогнать vitest в `web/`.
