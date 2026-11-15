---
status: закончил
agent: grok
date: 2026-11-15 09:18
task: Добрать тревоги OP-R16 и проверку OP-R17
refs: [2026-11-15-0916-grok-observability-alerts-start.md, docs/platform/ops.md, OP-R15, OP-R16, OP-R17, OP-R18]
touches: [deploy/ops-alert.sh, deploy/status-check.sh, deploy/disk-check.sh, deploy/observability_test.py, deploy/README.md, docs/handoff/LATEST.md]
---

# Закончил: тревоги служебного чата и страница состояния

**Начало работы:** [2026-11-15-0916-grok-observability-alerts-start.md](2026-11-15-0916-grok-observability-alerts-start.md)

**Сделано.**
- OP-R15 и OP-R18 без изменений: JSON-журнал в `core/internal/opsobs`, удаление файлов старше 30 дней в `deploy/log-retain.sh`.
- OP-R16: `deploy/ops-alert.sh` шлёт в служебный чат тексты про диск, неудачную загрузку каталога, неудачный пакет перепроверки, задание в `failed` и «Это не я». Токен в файл не записан.
- OP-R17: `deploy/status-check.sh` запрашивает `/status` и тревожит после 3 подряд неудач. Запуск по расписанию — раз в минуту.

**Не сделано.**
- Очередь заданий и переход пакета перепроверки в `failed` в коде выпуска 1 не ставят эту тревогу сами: скрипт вызывается с причиной `job` или `recheck`.
- Стек коллектора не добавлялся.

**Решения.** нет

**Открытые вопросы.** Формат кнопки «Это не я» по-прежнему не задан (AU-R30). Текст тревоги есть.

**Проверки.** `pytest deploy/observability_test.py` — 4 passed. Проверка наблюдаемости OP-R15…OP-R18 на рабочем дереве — без замечаний. Telegram не вызывался.

## Дальше
Вызывать `ops-alert.sh recheck` и `ops-alert.sh job` из мест, где пакет и задание реально переходят в `failed`, когда эти переходы появятся в коде.
