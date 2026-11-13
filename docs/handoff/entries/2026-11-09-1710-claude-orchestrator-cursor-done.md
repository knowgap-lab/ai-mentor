---
status: закончил
agent: claude
date: 2026-11-09 17:10
task: Переписать briefs/ORCHESTRATOR.md под Cursor, удалить устаревший промпт MVP
refs: [D72, D131, D155, D159, 2026-11-09-1700-claude-orchestrator-cursor-start]
touches: [briefs/ORCHESTRATOR.md, docs/archive/mvp/prompts/S0.md]
---

# Закончил: инструкция оркестратору для Cursor

Сделано: `briefs/ORCHESTRATOR.md` переписан без привязки к Claude Code, Fusion, Codex (роли вместо моделей; порядок «тесты, заморозка, код» вынесен в раздел 6; раздел 1 прямо говорит, что покрыт только Python-блок). Удалён `docs/archive/mvp/prompts/S0.md` (остаётся в истории git).

Не сделано: платформа на Go и веб-интерфейс в очередь не входят (ждут решений человека и напарника, см. `LATEST.md`). Коммит не делался.
