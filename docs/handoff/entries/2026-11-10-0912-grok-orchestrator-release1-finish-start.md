---
status: начал
agent: grok
date: 2026-11-10 09:12
task: Доделка выпуска 1 по briefs/ORCHESTRATOR-RELEASE1-FINISH.md
refs: [briefs/ORCHESTRATOR-RELEASE1-FINISH.md, briefs/ORCHESTRATOR-RELEASE1.md, briefs/CURSOR-SETUP.md, D301, D302, D303, 2026-11-10-0616-orchestrator-release1-done, 2026-11-10-0656-grok-orchestrator-reviews]
touches: [docs/handoff/LATEST.md, docs/handoff/orchestrator-progress-release1-finish.md, briefs/r1-q0.md, feat/release1-finish]
---

# Начал: доделка выпуска 1

**Беру в работу.** Закрыть разрыв между зелёными тестами модулей и работающим приложением: опись контракта, линтеры, подключение модулей к серверу, экраны по дизайн-системе, сквозные E2 и E3.

**План.**
- Ветка `feat/release1-finish` от `feat/release1`. Номера решений с D304. Состояние в `docs/handoff/orchestrator-progress-release1-finish.md`.
- Зафиксировать исходную опись и `tools/check-code.sh`.
- Фаза Q: пакет Q0 (линтеры, чистая установка), затем фазы S, W, L, E и долги ревью R1–R2. C1 — отдельный worktree.
- Пакет принят только по разделу 5 инструкции доделки, не по одним зелёным тестам.
- GitHub не использовать.

**Исходные числа (2026-11-10 09:08, ветка `feat/release1`, `23eee65`).**
- `tools/release1_inventory.py`: операции 15 из 58, экраны 5 из 14, код выхода 1.
- `tools/check-code.sh`: код выхода 1. `go test ./...` в `core/` прошёл, затем «golangci-lint не установлен». До `ruff`, веба, `govulncheck` и `npm audit` скрипт не дошёл. `ruff` в `.venv` нет.
