---
status: закончил
agent: cursor
date: 2026-11-28 17:25
task: dev smoke на origin/main 4573fd5 (Release 1 demo)
refs:
  - 4573fd57be8aafa0360d2904f5def559eddd982e
  - tools/dev-install.sh
  - tools/dev-start.sh
  - tools/dev-check.sh
  - docs/handoff/am43-dev-smoke-evidence.md
touches:
  - docs/handoff/entries/2026-11-28-1725-cursor-dev-smoke-4573fd-done.md
---

# Закончил: dev smoke @ 4573fd5

**Начало работы:** облачный прогон по запросу Release 1 demo (без Fast mode, без KVM/ubuntu26.04/mmf-auto-schedule). `git rev-parse HEAD` → `4573fd57be8aafa0360d2904f5def559eddd982e` (`fix(load): pace pilot auth…`, PR #80).

## SHA

`4573fd57be8aafa0360d2904f5def559eddd982e`

## Итог (pass только при всех трёх exit 0)

| Проверка | Результат | Деталь |
|----------|-----------|--------|
| Docker | **pass** | `dev-install.sh` поставил `docker.io`; `dev_ensure_docker` в `dev-start`/`dev-check` без ослабления скриптов; контейнер `ai-mentor-dev` (образ postgres по `mise.toml`). При установке: `docker: unrecognized service` (systemd unit), демон поднят штатным путём скриптов. |
| Postgres | **pass** | `dev-start.sh`: `postgres://postgres@127.0.0.1:5432/ai_mentor?sslmode=disable`, seed demo; `psql` → `SELECT 1` ok. |
| Smoke (`dev-check.sh`) | **pass** | exit **0**: `check-code.sh`, `check.sh`, Playwright **20/20**. |

## Команды (порядок)

| Команда | Exit |
|---------|------|
| `bash tools/dev-install.sh` | **0** |
| `bash tools/dev-start.sh` | **0** |
| `bash tools/dev-check.sh` | **0** |

Первой ошибки нет.

**Сделано.**
- Полный dev smoke на чистом дереве `4573fd5` (Ubuntu cloud VM, Europe/Minsk 2026-11-28).

**Не сделано.**
- Обновление `LATEST.md`, `release1-progress.md`, `release1-attestation.json` (по заданию не трогали).
- KVM-гость, ubuntu26.04, mmf-auto-schedule.

**Решения.** нет

**Открытые вопросы.** нет

**Проверки.** Только перечисленные три скрипта; скрипты не менялись.

## Дальше

Смержить PR с записью; при необходимости скопировать таблицу в attestation вручную на отдельной ветке.