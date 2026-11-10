---
status: закончил
agent: grok
date: 2026-11-10 15:10
task: Пакет Q0, линтеры и чистая установка
refs: [briefs/r1-q0.md, 2026-11-10-0912-grok-orchestrator-release1-finish-start, D131]
touches: [core/.golangci.yml, ruff.toml, tools/check-code.sh, tools/requirements.txt, core/go.mod, core/go.sum]
---

# Закончил: Q0

**Начало работы:** [2026-11-10-0912-grok-orchestrator-release1-finish-start.md](2026-11-10-0912-grok-orchestrator-release1-finish-start.md)

**Сделано.**
- Ветка `feat/r1-q0`, коммит `873c5c1`, влита в `feat/release1-finish` (`649db82`).
- `golangci-lint` v2.15.0 и `govulncheck` v1.9.0 — инструменты в `core/go.mod`. `ruff` 0.17.4 в `tools/requirements.txt`.
- `tools/check-code.sh` не пропускает отсутствие линтеров, гоняет `go test -race` для `cmd/migrate`, `cmd/server`, `internal/answerrules`, `govulncheck` и `npm audit --omit=dev --audit-level=high`.
- `*_test.go` и каталог `content/tests` линтер не правит (D131). Замечания в нетестовом коде сняты без смены поведения. Ревью: APPROVE.

**Не сделано.**
- У `katex` внутри `remark-math` остаются 4 low. High и critical нет. Ломать `rehype-katex` ради них не стали.

**Решения.** нет

**Открытые вопросы.** нет

**Проверки.** `bash tools/check-code.sh` код 0. `bash tools/check.sh` — 251 passed. `git diff` по `*_test.go` и `content/tests` пуст.

## Дальше

Пакет S1: подключить готовые обработчики к `core/cmd/server`. Ревью долгов R1 параллельно.
