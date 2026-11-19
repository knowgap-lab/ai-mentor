---
status: начал
agent: grok
date: 2026-11-15 11:55
task: Заморозить тесты пакета W5 «Репетитор: ученик»
refs: [briefs/ORCHESTRATOR-RELEASE1-FINISH.md, docs/platform/web.md, docs/handoff/orchestrator-progress-release1-finish.md, WB-R21, WB-R25, WB-R4, WB-R6, WB-R9, WB-R30, PL-R8]
touches: [web/src/tutor/student-card.test.tsx, web/src/tutor/students-indicators.test.tsx, docs/handoff/LATEST.md]
---

# Начал: тесты карточки ученика

**Беру в работу.** На `origin/feat/release1-demo` (`61f6fd5`) в `web/screens.json` нет экранов репетитора, кроме «Ученики». Следующий незамороженный пакет экранов репетитора в брифе доделки — W5 «Репетитор: ученик». W4 («Итог», «Профиль») уже есть на `feat/r1-w4-code`, его не повторяю.

**План.**
- Тесты состава «Ученик» и показателей «Ученики» по `web.md` и `core-api.yaml`.
- Экраны, маршруты в приложении и `web/screens.json` не писать.
- Замороженные `web/src/tutor/TutorPage.test.tsx` не менять.
- Ветка `feat/r1-tutor-screen-tests` от `61f6fd5`.
