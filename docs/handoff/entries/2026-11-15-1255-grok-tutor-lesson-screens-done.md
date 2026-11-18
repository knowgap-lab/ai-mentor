---
status: закончил
agent: grok
date: 2026-11-15 12:55
task: Экраны «Урок» и «Подготовка» по замороженным тестам W6
refs: [docs/handoff/entries/2026-11-15-1245-grok-tutor-lesson-screens-start.md, web/src/tutor/tutor-lesson.test.tsx, web/src/tutor/preparation.test.tsx, docs/platform/web.md, WB-R2, WB-R4, WB-R6, WB-R22, WB-R30, LS-R3, LS-R4, LS-R6, LS-R13, LS-R14, LS-R16, LS-R20, PL-R4a, PL-R8]
touches: [web/src/tutor/TutorLessonPage.tsx, web/src/tutor/PreparationPage.tsx, web/src/tutor/api-error.ts, web/src/tutor/minsk-time.ts, web/src/App.tsx, web/screens.json, docs/handoff/LATEST.md]
---

# Урок и подготовка на экране

**Начало работы:** [2026-11-15-1245-grok-tutor-lesson-screens-start](2026-11-15-1245-grok-tutor-lesson-screens-start.md)

**Сделано.**
- Экран «Урок»: [web/src/tutor/TutorLessonPage.tsx](../../../web/src/tutor/TutorLessonPage.tsx), маршрут `/tutor/lessons/:lessonId` в [web/src/App.tsx](../../../web/src/App.tsx), ключ «Репетитор/Урок» в [web/screens.json](../../../web/screens.json). Состав, снимок или «файл урока», время Минска, перенос, отмена, пропуск, две отметки или один выбор покрытия (WB-R2, WB-R4, WB-R6, WB-R30, LS-R3, LS-R4, LS-R6, LS-R13, LS-R14, LS-R16, LS-R20, PL-R4a, PL-R8).
- Экран «Подготовка»: [web/src/tutor/PreparationPage.tsx](../../../web/src/tutor/PreparationPage.tsx), маршрут `/tutor/lessons/:lessonId/preparation`, ключ «Репетитор/Подготовка». Ключ и решение появляются только после «показать» (WB-R22).
- Общие тексты ошибок: [web/src/tutor/api-error.ts](../../../web/src/tutor/api-error.ts). Перевод местного времени Минска в UTC: [web/src/tutor/minsk-time.ts](../../../web/src/tutor/minsk-time.ts).
- Тесты W6 и карточка ученика не менялись.

**Не сделано.**
- W7 «Репетитор: ДЗ» не начат.

**Решения.** нет

**Открытые вопросы.** Те же, что в шапках тестовых файлов: русские слова для `planned`, `frozen` и `held`; подпись поля переноса; тексты кодов `already_marked`, `lesson_not_held`, `slot_taken`, `mark_not_in_lesson`; слова подтверждения действий; версия каталога не входит в `LessonDetail`; у `Preparation` нет вида «урок / ДЗ / срез»; одна кнопка «показать» или по кнопке на задачу. На экране подготовки одна кнопка «показать» на весь ответ: тест это допускает.

**Проверки.** Из `web/`: vitest — 10 файлов, 74 теста зелёные, включая W5 и W6. `tsc --noEmit`, eslint и prettier по новым файлам без ошибок. `tools/check_design.py` — DS-R1 ок.

## Дальше

Следующий пакет экранов репетитора — W7 «Репетитор: ДЗ». Тесты W6 не менять.
