---
status: закончил
agent: integration-agent
date: 2026-11-22 19:30
task: зафиксировать правила запуска Cursor и экономии лимита
refs: [docs/handoff/entries/2026-11-22-1928-integration-cursor-usage-policy-start.md, D320, D324, D327, D332]
touches: [AGENTS.md, .cursor/rules/repository-source.mdc, briefs/CURSOR-PROJECT-RELEASE1.md, briefs/CURSOR-PROJECT-RESUME.md, briefs/RELEASE1-GOAL.md, docs/decisions/log.md]
---

Начало работы — [запись](2026-11-22-1928-integration-cursor-usage-policy-start.md).

# Правила запуска Cursor для выпуска 1

- В брифе определены выбор моделей, условия повышения effort, первая группа исполнителей и правила повторного использования результатов. Общий Red Team и пакеты запускаются по актуальному объёму работ.
- Решение D332, AGENTS.md, правило Cursor, цель и инструкция возобновления ведут к одному брифу.
- tools/check.sh завершился с кодом 0, прошёл 251 тест. Ссылки на разделы и фронтматтер правила Cursor проверены.

## Дальше

При следующем согласованном запуске сверить фактические модели на карточках Cursor, начать с группы из раздела 12 и оценить расход на первый принятый результат. Сессия ещё не запускалась.
