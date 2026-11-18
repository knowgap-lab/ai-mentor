---
status: закончил
agent: grok
date: 2026-11-15 11:36
task: Имя адреса Gotenberg по D329
refs: [D301, 2026-11-15-1130-grok-gotenberg-url-start]
touches: [core/internal/pdf/gotenberg.go, deploy/README.md, deploy/secrets.md, deploy/compose.prod.yml, deploy/compose.staging.yml]
---

# Закончил: адрес Gotenberg

**Начало работы:** [2026-11-15-1130-grok-gotenberg-url-start](2026-11-15-1130-grok-gotenberg-url-start.md)

**Сделано.**
- `7c627376f988689cf2057ea5f310224d9429e36f` — предок `origin/feat/release1-demo` `61f6fd5e3615b6b1aba0ef68a1e2db7b75ce32f3`.
- Клиент и `deploy/` читают одно имя: `GOTENBERG_URL`.
- Образ `gotenberg/gotenberg:9.5.0` скачан. Один HTML превращён в PDF: 10778 байт, одна страница.

**Не сделано.**
- Строки D329 в `docs/decisions/log.md` нет. Журнал на этой ветке кончается на D316. Имя взято из задания, где D329 требует `GOTENBERG_URL`.
- Замороженные `deploy/ops_s8_test.py` и `core/internal/gotenbergtest/gotenberg.go` не видят имя, которое начинается с `GOTENBERG`. Проверка не ослаблена. Это дефект теста.

**Решения.** нет

**Открытые вопросы.** Строку D329 в журнал должен добавить человек или отдельная сессия: следующий номер в файле — D317, а задание называет D329.

**Проверки.** `pytest deploy`: 15 passed, `test_s8_gotenberg_address_named` failed. `go test -run '^$'` для pdf, conspect и gotenbergtest собрался. Конвертация — живой контейнер, не пропуск.

## Дальше
Дефект замороженной проверки имени чинить отдельным PR. Этот коммит имя не подгоняет под регулярное выражение.
