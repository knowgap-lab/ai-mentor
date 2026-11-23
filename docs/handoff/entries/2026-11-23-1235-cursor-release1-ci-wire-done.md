---
status: закончил
agent: cursor
date: 2026-11-23 12:35
task: PR #64 — docker, chrome, contract-live, size-limit, реестр открытых job
---

## Сделано

- Job `docker`, `chrome`, `contract-live` и команда `size-limit` по D334.
- Реестр открытых: `docs/handoff/release1-ci-open-checks.yaml` (size-limit без бюджета, deploy без VM).

## Не сделано

- Числовой бюджет bundle (`web/size-limit.budget.json`).
- `deploy` / `tools/vm-acceptance.sh`.

## Дальше

- Измерить bundle на зелёном SHA, зафиксировать бюджет, закрыть `size-limit`.
- VM acceptance после секретов и скрипта.

## Ссылка

- D334, PR #64 branch `cursor/release1-ci-jobs-ac9d`