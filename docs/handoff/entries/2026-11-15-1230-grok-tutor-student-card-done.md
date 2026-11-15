---
status: закончил
agent: grok
date: 2026-11-15 12:30
task: Экран «Ученик» и показатели «Ученики» по замороженным тестам W5
refs: [docs/handoff/entries/2026-11-15-1220-grok-tutor-student-card-start.md, web/src/tutor/student-card.test.tsx, web/src/tutor/students-indicators.test.tsx, docs/platform/web.md, WB-R2, WB-R4, WB-R6, WB-R9, WB-R25, WB-R30, PL-R4a, PL-R8, PV-R12, PV-R22]
touches: [web/src/tutor/StudentCardPage.tsx, web/src/tutor/minsk-time.ts, web/src/tutor/TutorPage.tsx, web/src/App.tsx, web/screens.json, docs/handoff/LATEST.md]
---

# Карточка ученика собрана

**Начало работы:** [2026-11-15-1220-grok-tutor-student-card-start](2026-11-15-1220-grok-tutor-student-card-start.md)

**Сделано.**
- Экран «Ученик»: [web/src/tutor/StudentCardPage.tsx](../../../web/src/tutor/StudentCardPage.tsx), маршрут `/tutor/students/:studentId` в [web/src/App.tsx](../../../web/src/App.tsx), ключ «Репетитор/Ученик» в [web/screens.json](../../../web/screens.json).
- Состав по тестам W5: предупреждения текстом, «Расписание» и уроки плана, «Выдачи» со сроком в Europe/Minsk, сырой ввод, «разобрать на уроке», «Согласие», архив и возврат, приглашение, ссылка согласия (`parent` до 18 лет), «отменить отзыв» только при `revoked`. Ключ, решение и «верная опция» на экран не попадают (WB-R4).
- Список «Ученики»: показатели и плашка, обрыв сети — «Нет связи» и «Повторить». Клик по имени по-прежнему показывает сырой ввод на этом экране.
- Время — [web/src/tutor/minsk-time.ts](../../../web/src/tutor/minsk-time.ts) (WB-R2).

**Не сделано.**
- Слова для `planned`, `frozen` и `held` не выбраны: в web.md их нет, тест их не фиксирует. На экране есть только «пропущен», «перенесено» и «добавлено».
- Подпись вида выдачи не выбрана: показан срок, слово `homework` не пишется.
- Отдельного слова подтверждения архивации нет: кнопка «Архив» сразу отправляет запрос, как в тесте.
- W6 не начат. Экраны W4 не менялись. Замороженные тесты не менялись.

**Решения.** нет

**Открытые вопросы.** Те же, что в шапках тестов W5: слова состояний урока, подпись вида выдачи, слово подтверждения архивации, текст `consent_kind_invalid`. Чтения расписания в контракте по-прежнему нет.

**Проверки.** Из `web/`: vitest 43 теста зелёные, `tsc --noEmit` и eslint по изменённым файлам без ошибок, `tools/check_design.py` — DS-R1 ок. Браузер не открывался: экран покрыт jsdom-тестами.

## Дальше

Следующий пакет экранов репетитора — W6, «Урок» и «Подготовка». Тесты W5 не менять.
