---
status: начал
agent: grok
date: 2026-11-15 12:20
task: Экран «Ученик» и показатели «Ученики» по замороженным тестам W5
refs: [docs/handoff/entries/2026-11-15-1210-grok-tutor-screen-tests-done.md, web/src/tutor/student-card.test.tsx, web/src/tutor/students-indicators.test.tsx, docs/platform/web.md, WB-R2, WB-R4, WB-R6, WB-R9, WB-R25, WB-R30, PL-R8, PV-R12, PV-R22]
touches: [web/src/tutor/StudentCardPage.tsx, web/src/tutor/minsk-time.ts, web/src/tutor/TutorPage.tsx, web/src/App.tsx, web/screens.json, docs/handoff/LATEST.md]
---

# Начал: карточка ученика

**Беру в работу.** Код-сессия по замороженным тестам W5 на `feat/r1-tutor-screen-tests` (`1e813b8`). Экран «Ученик» и показатели списка «Ученики». Тесты и экраны W4 не меняю.

**План.**
- Ветка `feat/r1-tutor-student-card`.
- `StudentCardPage` на `/tutor/students/:studentId` и ключ «Репетитор/Ученик» в `web/screens.json`.
- Показатели и «Нет связи» на существующем списке «Ученики», клик по имени как в замороженном тесте.
- Прогнать vitest в `web/`.
