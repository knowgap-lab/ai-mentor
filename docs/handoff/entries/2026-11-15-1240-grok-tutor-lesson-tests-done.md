---
status: закончил
agent: grok
date: 2026-11-15 12:40
task: Заморозить тесты пакета W6 «Репетитор: урок и подготовка»
refs: [docs/handoff/entries/2026-11-15-1235-grok-tutor-lesson-tests-start.md, briefs/ORCHESTRATOR-RELEASE1-FINISH.md, docs/platform/web.md, docs/personalization/lessons.md, WB-R2, WB-R4, WB-R6, WB-R22, WB-R30, LS-R3, LS-R4, LS-R6, LS-R13, LS-R14, LS-R16, LS-R20, LS-R24, PL-R4a, PL-R8]
touches: [web/src/tutor/tutor-lesson.test.tsx, web/src/tutor/preparation.test.tsx, docs/handoff/LATEST.md]
---

# Тесты урока и подготовки заморожены

**Начало работы:** [2026-11-15-1235-grok-tutor-lesson-tests-start](2026-11-15-1235-grok-tutor-lesson-tests-start.md)

**Сделано.**
- Пакет W6 «Репетитор: урок и подготовка»: [web/src/tutor/tutor-lesson.test.tsx](../../../web/src/tutor/tutor-lesson.test.tsx) (экран «Урок», маршрут теста `/tutor/lessons/:lessonId`) и [web/src/tutor/preparation.test.tsx](../../../web/src/tutor/preparation.test.tsx) (экран «Подготовка», маршрут теста `/tutor/lessons/:lessonId/preparation`).
- Экраны, `web/screens.json` и замороженные тесты W5 не менялись.

**Не сделано.**
- Кода экранов нет: `TutorLessonPage` и `PreparationPage` ещё не существуют. Так и задумано: новые тесты красные до код-сессии.
- `web/screens.json` по-прежнему без ключей «Репетитор/Урок» и «Репетитор/Подготовка». Их пишет код-сессия.

**Решения.** нет

**Открытые вопросы.** В шапках тестовых файлов: русские слова для `planned`, `frozen` и `held`; подпись поля переноса; тексты кодов `already_marked`, `lesson_not_held`, `slot_taken`, `mark_not_in_lesson`; слова подтверждения действий; версия каталога не входит в `LessonDetail`; у `Preparation` нет вида «урок / ДЗ / срез»; одна кнопка «показать» или по кнопке на задачу.

**Проверки.** Из `web/`: уже существующие vitest — 8 файлов, 43 теста зелёные. Новые файлы не собираются: нет `./TutorLessonPage` и `./PreparationPage`. eslint и prettier по новым файлам без ошибок.

## Дальше

Код-сессия по этим тестам: экраны «Урок» и «Подготовка». Ключи реестра — «Репетитор/Урок» и «Репетитор/Подготовка». Тесты W6 не менять. Следующий пакет экранов репетитора после этого — W7 «Репетитор: ДЗ».
