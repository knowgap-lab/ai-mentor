# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-23 (release1-ci, PR #64)
**Ветка:** `cursor/release1-ci-jobs-ac9d`
**Этап:** подключение job release1-ci по D334.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| release1-ci workflow | generation, build, storybook, e2e; docker, chrome, contract-live (D334) | size-limit (нет `web/size-limit.budget.json`), deploy (VM) |
| Документация | D334, реестр [release1-ci-open-checks.yaml](release1-ci-open-checks.yaml) | синхронизация с release1-progress на docs/r1-repo-handoff |

## Что в работе сейчас

PR [#64](https://github.com/knowgap-lab/ai-mentor/pull/64) — дождаться CI на новом коммите.

## Следующий шаг

Зафиксировать измеренный bundle в `web/size-limit.budget.json`; подготовить VM acceptance для job `deploy`.

## Блокеры и вопросы

Нет зафиксированного числового лимита bundle (D315). Секреты VM и `tools/vm-acceptance.sh` отсутствуют.

## Куда смотреть

- D334 в [docs/decisions/log.md](../decisions/log.md)
- Открытые job: [release1-ci-open-checks.yaml](release1-ci-open-checks.yaml)
- Порядок: briefs/CURSOR-PROJECT-RELEASE1.md §12 (ветка `docs/r1-repo-handoff`)