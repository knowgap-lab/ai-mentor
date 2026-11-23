---
status: закончил
agent: grok
date: 2026-11-15 12:10
task: Заморозить тесты пакета W5 «Репетитор: ученик»
refs: [docs/handoff/entries/2026-11-15-1155-grok-tutor-screen-tests-start.md, briefs/ORCHESTRATOR-RELEASE1-FINISH.md, docs/platform/web.md, WB-R4, WB-R6, WB-R9, WB-R25, WB-R30, PL-R8, PV-R12, PV-R22]
touches: [web/src/tutor/student-card.test.tsx, web/src/tutor/students-indicators.test.tsx, docs/handoff/LATEST.md]
---

# Тесты карточки ученика заморожены

**Начало работы:** [2026-11-15-1155-grok-tutor-screen-tests-start](2026-11-15-1155-grok-tutor-screen-tests-start.md)

**Сделано.**
- Пакет W5 «Репетитор: ученик»: [web/src/tutor/student-card.test.tsx](../../../web/src/tutor/student-card.test.tsx) (экран «Ученик», маршрут теста `/tutor/students/:studentId`) и [web/src/tutor/students-indicators.test.tsx](../../../web/src/tutor/students-indicators.test.tsx) (показатели «Ученики»).
- Экраны, `web/screens.json` и замороженный `TutorPage.test.tsx` не менялись. W4 не повторялся.

**Не сделано.**
- Кода экрана нет: `StudentCardPage` ещё не существует, показатели на «Ученики» не показаны. Так и задумано: тесты красные до код-сессии.
- `web/screens.json` по-прежнему без ключа «Репетитор/Ученик». Его пишет код-сессия.

**Решения.** нет

**Открытые вопросы.** В шапках тестовых файлов: нет GET расписания; русские слова для `planned`, `frozen`, `held`; подпись вида выдачи; слово подтверждения архивации; `consent_kind_invalid` нет в таблице `web.md`.

**Проверки.** Из `web/`: уже существующие vitest — 21 тест зелёные. Новый файл карточки не собирается: нет `./StudentCardPage`. Показатели падают на фразе «следующий урок». Сеть списка падает: нет «Нет связи», текущий `TutorPage` не ловит обрыв `fetch`. Пустой список уже зелёный.

## Дальше

Код-сессия по этим тестам: экран «Ученик» и показатели «Ученики». Ключ реестра — «Репетитор/Ученик». Следующий пакет экранов репетитора после этого — W6 «Урок» и «Подготовка». Тесты W5 не менять.
