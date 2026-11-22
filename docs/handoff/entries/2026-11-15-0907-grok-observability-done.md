---
status: закончил
agent: grok
date: 2026-11-15 09:07
task: Правила наблюдаемости OP-R15–OP-R18
refs: [2026-11-15-0903-grok-observability-start.md, docs/platform/ops.md, D250, PV-R27, NT-R21]
touches: [core/internal/opsobs/log.go, core/internal/opsobs/log_test.go, core/cmd/server/main.go, deploy/disk-check.sh, deploy/status-check.sh, deploy/log-retain.sh, deploy/alert-send.sh, deploy/observability_test.py, deploy/README.md, docs/handoff/LATEST.md]
---

# Закончил: журналы и тревоги OP-R15–OP-R18

**Начало работы:** [2026-11-15-0903-grok-observability-start.md](2026-11-15-0903-grok-observability-start.md)

**Сделано.**
- OP-R15: `core/internal/opsobs` пишет JSON. Запись с `display_name`, `name`, `telegram_id`, `birth_date` или `token` не попадает в журнал. `student_id` и `pseudonym` остаются. Сервер в `core/cmd/server/main.go` берёт этот журнал.
- OP-R16, пример «диск 85 %»: `deploy/disk-check.sh` печатает одну тревогу, пока процент больше 80.
- OP-R17: `deploy/status-check.sh` один раз запрашивает переданный адрес; на третьей неудаче подряд печатает «сервис недоступен».
- OP-R18, D250: `deploy/log-retain.sh` удаляет файлы старше 30 дней.
- `deploy/alert-send.sh` отправляет текст в служебный чат только если `OPS_CHAT_ID` и `TELEGRAM_BOT_TOKEN` уже заданы. Значений в git нет.
- Сбой копии и проверки восстановления уже тревожат из `deploy/backup.sh` и `deploy/restore-check.sh`.

**Не сделано.**
- Неудачная загрузка каталога, неудачный пакет перепроверки и задание в `failed` не подключены: доставка смотрит `telegram_id` получателя, а не `OPS_CHAT_ID`, и для этого нет отдельного правила получателя без правки замороженного `notify`.
- «Это не я» не подключено: формат нажатия не задан (открытый вопрос AU-R30).
- Лаг пересчёта и бюджет LLM — выпуски 2 и 4, не этот выпуск.
- Коллектор, Prometheus, Loki, Tempo, Grafana, `/readyz`, `/metrics` и сроки 30/14/7 не добавлялись.

**Решения.** нет

**Открытые вопросы.** нет новых. AU-R30 по-прежнему без формата кнопки.

**Проверки.** `go test ./internal/opsobs/` и `go test -run TestHealthz ./cmd/server/` — проходят. `pytest deploy/observability_test.py` — 3 passed. Сеть Telegram не вызывалась.

## Дальше
Подключить оставшиеся тревоги OP-R16, когда будет задан получатель `OPS_CHAT_ID` в доставке и формат «Это не я».
