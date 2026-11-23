---
status: начал
agent: grok
date: 2026-11-15 12:35
task: Заморозить тесты пакета W6 «Репетитор: урок и подготовка»
refs: [docs/handoff/entries/2026-11-15-1230-grok-tutor-student-card-done.md, briefs/ORCHESTRATOR-RELEASE1-FINISH.md, docs/platform/web.md, docs/personalization/lessons.md, WB-R2, WB-R4, WB-R6, WB-R22, WB-R30, LS-R3, LS-R4, LS-R6, LS-R13, LS-R14, LS-R16, LS-R20, LS-R24, PL-R4a, PL-R8]
touches: [web/src/tutor/tutor-lesson.test.tsx, web/src/tutor/preparation.test.tsx, docs/handoff/LATEST.md]
---

# Начал: тесты урока и подготовки

**Беру в работу.** Пакет W6 «Репетитор: урок и подготовка» на `feat/r1-tutor-student-card` (`791e9cf`). Два экрана: «Репетитор/Урок» и «Репетитор/Подготовка». Это сессия тестов. Код экранов, `web/screens.json` и замороженные тесты W5 не меняю.

**План.**
- Ветка `feat/r1-tutor-lesson-tests`.
- Тесты состава «Урок»: этапы, блоки, практика, снимок, две отметки, перенос, отмена, пропуск.
- Тесты «Подготовка»: ключ и решение скрыты до «показать» (WB-R22, web-15).
- Уже существующие vitest остаются зелёными. Новые файлы могут быть красными: страниц ещё нет.
