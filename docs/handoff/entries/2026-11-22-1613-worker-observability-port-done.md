---
status: закончил
agent: integration-agent
date: 2026-11-22 16:13
task: перенести проверки наблюдаемости на кандидат выпуска 1
refs: [docs/handoff/entries/2026-11-22-1611-worker-observability-port-start.md, D328, D330]
touches: [content/loader, content/tests, core/cmd/server, core/internal/opsobs, deploy]
---

Начало работы — [запись](2026-11-22-1611-worker-observability-port-start.md).

# Наблюдаемость на кандидате выпуска 1

- Перенесены код и существующие тесты из PR #20, SHA 41aa9b5, поверх dc1e160. Сохранены фоновая перепроверка и имя GOTENBERG_URL.
- Go test и Go vet прошли для cmd/server, internal/opsobs и internal/recheck. Pytest прошёл для deploy/observability_test.py и tests/test_load_failed_alert.py: 4 и 2 теста. Ruff и ShellCheck прошли.
- Локальная проверка observability из приёмщика #55 прошла. Действительную доставку тревог и расписания на VM не проверяли.

## Дальше

Ревью переноса и прогон на стенде: подключить вызовы для перепроверки, задания failed и «Это не я», установить расписание проверки /status и очистки журналов, проверить доставку тревог с синтетическими данными.
