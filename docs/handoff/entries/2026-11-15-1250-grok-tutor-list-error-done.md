---
status: закончил
agent: grok
date: 2026-11-15 12:50
task: Ошибку HTTP списка учеников оставить на экране «Ученики»
refs: [docs/handoff/entries/2026-11-15-1245-grok-tutor-list-error-start.md, WB-R6, WB-R30]
touches: [web/src/tutor/TutorPage.tsx, web/src/tutor/api-error.ts, web/src/tutor/StudentCardPage.tsx, docs/handoff/LATEST.md]
---

# Ответ списка остаётся на «Ученики»

**Начало работы:** [2026-11-15-1245-grok-tutor-list-error-start](2026-11-15-1245-grok-tutor-list-error-start.md)

**Сделано.**
- После входа ответ `GET /students` не 2xx, кроме 401, оставляет экран «Ученики»: текст из таблицы ошибок и «Повторить». Сеанс не сбрасывается.
- 401 возвращает на «Войти» с текстом `login_stale`.
- Обрыв сети списка по-прежнему ровно «Нет связи», без «попробуйте ещё раз».
- Общая функция текста ошибки — [web/src/tutor/api-error.ts](../../../web/src/tutor/api-error.ts).

**Не сделано.** Тесты Go в `code-check` по-прежнему не этот пакет.

**Решения.** нет

**Открытые вопросы.** нет новых

**Проверки.** Из `web/`: vitest по `tutor/` — 25 тестов зелёные, eslint по изменённым файлам чистый.

## Дальше

W6, «Урок» и «Подготовка».
