---
status: начал
agent: cursor
date: 2026-11-23 12:00
task: PR #64 — docker, chrome, contract-live, size-limit, реестр открытых job
refs: ["PR #64", D328, D334]
touches: [.github/workflows/release1-ci.yml, tools/ci/, docs/handoff/release1-ci-open-checks.yaml, docs/decisions/log.md, docs/handoff/LATEST.md]
---

## Цель

Подключить проверки release1-ci по D328 и §12 briefs/CURSOR-PROJECT-RELEASE1.md (ветка docs/r1-repo-handoff). Generation не трогать.

## Файлы

- `.github/workflows/release1-ci.yml`
- `tools/ci/*.sh`
- `docs/handoff/release1-ci-open-checks.yaml`
- `docs/decisions/log.md` (D334)
- `docs/handoff/LATEST.md`
