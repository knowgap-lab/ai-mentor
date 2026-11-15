---
status: начал
agent: grok
date: 2026-11-15 09:26
task: Тревога при load_failed
refs: [docs/catalog/loader.md, LD-R10, deploy/ops-alert.sh, content/loader/__init__.py]
touches: [content/loader/__init__.py, content/tests/test_load_failed_alert.py, docs/handoff/LATEST.md]
---

# Начал: тревога при срыве транзакции каталога

**Беру в работу.** LD-R10: если транзакция содержимого не удалась, `load_artifact` поднимает `LoadRefused("load_failed")` и служебный чат должен получить тревогу. Сейчас `deploy/ops-alert.sh catalog` никто не вызывает. Подключу вызов только на этом пути. Токен не добавляю. Перепроверку, задание очереди и «Это не я» не подключаю.

**План.**
- В `content/loader` после срыва `_psql` вызвать `deploy/ops-alert.sh catalog`, затем поднять `load_failed`.
- Тест: этот путь зовёт скрипт с аргументом `catalog`; другой отказ не зовёт.
