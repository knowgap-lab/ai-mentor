# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-28 (правило PR в AGENTS.md; deploy KVM runner env)
**Ветка:** `cursor/deploy-kvm-runner-env-d5d4` — PR runner env для deploy §8
**Этап:** в main — **каждое изменение через pull request** (AGENTS.md § Git); deploy §8 на acceptance-kvm.

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| AGENTS.md § Git | правило «всё через PR», доска = открытые PR | — (в main) |
| release1-ci deploy | runner env на acceptance-kvm (PR #88) | гостевой pass на железе |

## Что в работе сейчас

PR [#88](https://github.com/knowgap-lab/ai-mentor/pull/88) — `tools/ci/vm-acceptance-runner-env.sh` для job `deploy`.

## Следующий шаг

Merge PR #88; при libvirt на runner — полный `tools/vm-acceptance.sh`.

## Блокеры и вопросы

На `acceptance-kvm` runner env из libvirt (см. `tools/ci/vm-acceptance-runner-env.sh`); иначе секреты GitHub. Schemathesis не зафиксирован в `tools/requirements.txt`.

## Куда смотреть

- [2026-11-28-1920-cursor-agents-pr-rule-done.md](entries/2026-11-28-1920-cursor-agents-pr-rule-done.md)
- [2026-11-28-1710-cursor-deploy-kvm-runner-env-done.md](entries/2026-11-28-1710-cursor-deploy-kvm-runner-env-done.md)