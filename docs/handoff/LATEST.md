# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-15 12:40 (grok, правки W5)
**Ветка:** `feat/r1-tutor-student-card` (от `feat/r1-tutor-screen-tests`, `1e813b8`)
**Этап:** экран «Ученик» и показатели «Ученики» собраны. Обрыв входа остаётся на форме «Войти», отмена отзыва прячет кнопку.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| Экраны | шесть ключей в `web/screens.json`, включая «Репетитор/Ученик» | шесть экранов репетитора из `web.md` |
| W4 | «Итог» и «Профиль» на `feat/r1-w4-code` | в эту ветку не переносились |
| W5 | карточка, показатели, ключ реестра, тесты зелёные | слова `planned` / `frozen` / `held` и подпись вида выдачи |

## Что в работе сейчас

Ничего. Код W5 и правки ревью в этой ветке. Записи — [2026-11-15-1230-grok-tutor-student-card-done](entries/2026-11-15-1230-grok-tutor-student-card-done.md), [2026-11-15-1240-grok-tutor-student-card-fix-done](entries/2026-11-15-1240-grok-tutor-student-card-fix-done.md).

## Следующий шаг

W6: «Урок» и «Подготовка». Тесты W5 не менять.

## Блокеры и вопросы

Открытые формулировки — в шапках `web/src/tutor/student-card.test.tsx` и `web/src/tutor/students-indicators.test.tsx`. Новых решений нет.

## Куда смотреть

- Состав экранов: [docs/platform/web.md](../platform/web.md)
- Пакет W5: [briefs/ORCHESTRATOR-RELEASE1-FINISH.md](../../briefs/ORCHESTRATOR-RELEASE1-FINISH.md)
- Правила для агентов: [AGENTS.md](../../AGENTS.md)
