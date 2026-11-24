# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-24 (vm acceptance SHA pin + deploy terraform)
**Ветка:** `cursor/release1-vm-guest-a35f8-4c59`
**Этап:** evaluated main `a35f8b0`; приёмка VM — pin SHA, deploy с Terraform; гость ждёт секреты.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| release1-ci | generation, build, storybook, e2e, docker, chrome, size-limit (D335), contract-live (D336) | deploy (VM) |
| Бюджет bundle | `web/size-limit.budget.json`, 252639 gzip bytes JS | — |

## Что в работе сейчас

PR [#64](https://github.com/knowgap-lab/ai-mentor/pull/64) после коммита с D335.

## Следующий шаг

Расширить E3 (schemathesis). VM acceptance для deploy.

## Блокеры и вопросы

Секреты VM и `tools/vm-acceptance.sh` отсутствуют. Schemathesis не зафиксирован в `tools/requirements.txt`.

## Куда смотреть

- D335 в [docs/decisions/log.md](../decisions/log.md)
- [release1-ci-open-checks.yaml](release1-ci-open-checks.yaml)