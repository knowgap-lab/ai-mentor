---
status: закончил
agent: grok
date: 2026-11-15 11:52
task: Проверка имени GOTENBERG_URL
refs: [2026-11-15-1148-grok-gotenberg-name-test-start, 2026-11-15-1136-grok-gotenberg-url-done]
touches: [deploy/ops_s8_test.py, core/internal/gotenbergtest/gotenberg.go]
---

# Закончил: проверка имени адреса

**Начало работы:** [2026-11-15-1148-grok-gotenberg-name-test-start](2026-11-15-1148-grok-gotenberg-name-test-start.md)

**Сделано.**
- В `deploy/ops_s8_test.py` и `gotenbergtest.EnvName` шаблон принимает имя, которое начинается с `GOTENBERG`. Точное имя `GOTENBERG_URL` находится.
- Пустой текст и файлы без имени по-прежнему не проходят проверку. Проверка не удалена.
- Клиент PDF, документы `deploy/` и код продукта не менялись.

**Не сделано.** нет

**Решения.** нет. D329 уже стоит в журнале на `cursor/gotenberg-url-252c` (`8ea1991`): переменная адреса — `GOTENBERG_URL`.

**Открытые вопросы.** нет

**Проверки.** `pytest deploy`: 16 passed, включая `test_s8_gotenberg_address_named`. Те же проверки на пустых файлах и на отсутствующем `secrets.md` падают. `EnvName()` вернул `GOTENBERG_URL`; пустой текст и текст без имени не дали совпадений.

## Дальше
Ревью этой проверки. В этот PR её не вливать.
