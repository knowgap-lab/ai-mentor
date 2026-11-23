---
status: начал
agent: grok
date: 2026-11-15 11:48
task: Проверка имени GOTENBERG_URL
refs: [2026-11-15-1136-grok-gotenberg-url-done, deploy/ops_s8_test.py, core/internal/gotenbergtest/gotenberg.go]
touches: [deploy/ops_s8_test.py, core/internal/gotenbergtest/gotenberg.go]
---

# Начал: проверка имени адреса

**Беру в работу.** Замороженная проверка не видит имя, которое начинается с `GOTENBERG`. Клиент и `deploy/` уже читают `GOTENBERG_URL`. Меняю только шаблон в проверке и в `gotenbergtest.EnvName`.

**План.**
- Ветка `feat/r1-gotenberg-name-test` от `cursor/gotenberg-url-252c`.
- Поправить шаблон так, чтобы точное имя `GOTENBERG_URL` находилось.
- Пустое и отсутствующее имя по-прежнему ошибка.
- Не трогать клиент PDF, документы `deploy/` и код продукта.
- Прогнать `pytest` и `EnvName`.
