# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-15 08:45 (grok)
**Ветка:** сверка снята с `origin/feat/release1-demo` `61f6fd5e3615b6b1aba0ef68a1e2db7b75ce32f3`. Запись прогресса — отдельная ветка от этой головы.
**Этап:** выпуск 1 собран в PR #6. PR красный, в `main` его не сливать.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| Интеграция | PR #6 `feat/release1-demo` содержит головы #7 и #8. `docs-check` на #6 зелёный | `code-check` на #6 красный: четыре программных теста в `go test ./...` |
| W4 | PR #9 замораживает экраны, PR #10 пишет код | Обе головы вне #6 и стоят на `2855e02`, на 3 коммита позади демо. Их `code-check` тоже красный |
| Очередь слияния | триггер `merge_group` добавлен в `code-check` и `docs-check` | правило **Require merge queue** в ruleset `protect-main` выключено |

Подробная таблица SHA и прогонов — [release1-progress.md](release1-progress.md). Запись сессии — [2026-11-15-0845-grok-release1-progress-done.md](entries/2026-11-15-0845-grok-release1-progress-done.md).

## Что в работе сейчас

Классификация четырёх красных программных тестов на `61f6fd5` по D320. Установка на диск и `install-machine` в выпуск не входят. `briefs/RELEASE1-GOAL.md` на полученных ветках нет.

## Следующий шаг

На `61f6fd5` воспроизвести и классифицировать `TestS6_MountOperations/S6_recheck_state`, `TestAT_R17_S5_DiscussListOneItem`, `TestRC_R20_S6_ReportForeignTutorAndStudent` и `TestPL_R5_S4_WarningsInEnum`. Совпадает с контрактом — чинить реализацию. Дефект замороженного программного теста — отдельный PR, два независимых ревью и зелёный CI. Красный PR #6 не сливать. После этого переложить #9 и #10 на `61f6fd5`.

## Блокеры и вопросы

- Владельцу: включить **Settings → Rules → Rulesets → protect-main → Require merge queue** ([ruleset 24420268](https://github.com/knowgap-lab/ai-mentor/rules/24420268)). Агент настройку не включает.
- В `docs/decisions/log.md` на `61f6fd5` строки D320 нет. Журнал заканчивается на D316.
- Запись [2026-11-13-1630](entries/2026-11-13-1630-claude-cursor-projects-prep-start.md) закрыта как [прервано](entries/2026-11-15-0835-grok-cursor-projects-prep-interrupted.md).

## Куда смотреть

- Прогресс выпуска 1: [docs/handoff/release1-progress.md](release1-progress.md)
- Решения: [docs/decisions/log.md](../decisions/log.md)
- Карта документации: [docs/README.md](../README.md)
- Правила для агентов: [AGENTS.md](../../AGENTS.md)
