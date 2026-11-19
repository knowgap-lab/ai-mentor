---
status: начал
agent: grok
date: 2026-11-15 11:30
task: Имя адреса Gotenberg по D329
refs: [D301, deploy/ops_s8_test.py, core/internal/gotenbergtest/gotenberg.go]
touches: [core/internal/pdf/gotenberg.go, deploy/README.md, deploy/secrets.md, deploy/compose.prod.yml, deploy/compose.staging.yml]
---

# Начал: адрес Gotenberg

**Беру в работу.** Ветка `feat/r1-s8-code` уже входит в `origin/feat/release1-demo`. Код читает `PDF_GOTENBERG_URL`. Задание называет имя по D329 — `GOTENBERG_URL`. Замороженные тесты не правлю.

**План.**
- Проверить, что `7c62737` — предок `origin/feat/release1-demo`.
- Найти строку D329 в журнале решений.
- Если имя — `GOTENBERG_URL`, сменить его в клиенте и в `deploy/`.
- Прогнать замороженную проверку имени и не ослаблять её.
- Скачать образ `gotenberg/gotenberg:9.5.0` и один раз получить PDF из HTML.
