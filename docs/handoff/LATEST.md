# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-23 (release1-ci size-limit baseline)
**Ветка:** `cursor/release1-ci-jobs-ac9d`
**Этап:** PR #64 — docker/chrome wired; size-limit baseline; contract-live открыт.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| release1-ci | generation, build, storybook, e2e, docker, chrome, **size-limit** (D335) | contract-live (E3), deploy (VM) |
| Бюджет bundle | `web/size-limit.budget.json`, 252639 gzip bytes JS | — |

## Что в работе сейчас

PR [#64](https://github.com/knowgap-lab/ai-mentor/pull/64) после коммита с D335.

## Следующий шаг

Именованная команда contract-live (сервер + schemathesis или сценарий E3). VM acceptance для deploy.

## Блокеры и вопросы

Секреты VM и `tools/vm-acceptance.sh` отсутствуют. Schemathesis не зафиксирован в `tools/requirements.txt`.

## Куда смотреть

- D335 в [docs/decisions/log.md](../decisions/log.md)
- [release1-ci-open-checks.yaml](release1-ci-open-checks.yaml)