---
status: начал
agent: grok
date: 2026-11-15 09:03
task: Правила наблюдаемости OP-R15–OP-R18
refs: [docs/platform/ops.md, docs/platform/ops.examples.yaml, docs/platform/notifications.md, docs/platform/privacy.md, D249, D250, D129, NT-R21, PV-R27]
touches: [core/internal/opsobs, core/cmd/server/main.go, deploy/disk-check.sh, deploy/status-check.sh, deploy/log-retain.sh, deploy/alert-send.sh, deploy/observability_test.py, deploy/README.md, docs/handoff/LATEST.md]
---

# Начал: журналы и тревоги OP-R15–OP-R18

**Беру в работу.** В `docs/platform/ops.md` правила OP-R15–OP-R18: структурированные журналы без ПДн, тревога в служебный чат, внешняя проверка страницы состояния, хранение журналов 30 дней (D250). Коллектор, Prometheus, Loki, Tempo, Grafana, `/readyz`, `/metrics` и сроки 30/14/7 не добавляю.

**План.**
- Журнал JSON в `core`, запись с запрещённым полем не пишется (OP-R15, PV-R27).
- `deploy/disk-check.sh`: диск выше 80 % — одна тревога (OP-R16, строка примера).
- `deploy/status-check.sh`: один запрос страницы состояния, тревога на третьей неудаче подряд (OP-R17).
- `deploy/log-retain.sh`: файлы старше 30 дней удаляются (OP-R18, D250).
- Токен бота и значение `OPS_CHAT_ID` не записывать. Отправка — только если эти имена уже есть в окружении, тем же приёмом, что в `backup.sh`.
- Триггеры без заданного формата или без места в уже принятых модулях не подключать и назвать в записи «закончил».
