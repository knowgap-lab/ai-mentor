# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-24 (AM-43 Chrome apt pin, main `c6d9603`)
**Ветка:** `cursor/chrome-apt-pin-am43-4c59` — PR chrome pin
**Этап:** main после #73; AM-43 smoke задокументирован.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| release1-ci | generation, build, storybook, e2e, docker, chrome, size-limit (D335), contract-live (D336) | deploy (VM) |
| Бюджет bundle | `web/size-limit.budget.json`, 252639 gzip bytes JS | — |

## Что в работе сейчас

PR [#64](https://github.com/knowgap-lab/ai-mentor/pull/64) после коммита с D335.

## Следующий шаг

Зелёный CI на PR #73; при наличии секретов — гостевая приёмка §8.

## Блокеры и вопросы

`QA_VM_SSH_HOST`, `VM_HYPERVISOR_ACCESS` для гостя. Schemathesis не зафиксирован в `tools/requirements.txt`.

## Куда смотреть

- D335 в [docs/decisions/log.md](../decisions/log.md)
- [release1-ci-open-checks.yaml](release1-ci-open-checks.yaml)