---
status: закончил
agent: grok
date: 2026-11-19 17:25
task: записать уже сданные доказательства выпуска 1
refs:
  - briefs/RELEASE1-GOAL.md
  - docs/handoff/entries/2026-11-19-1705-grok-agent-limit-checkpoint.md
  - https://github.com/knowgap-lab/ai-mentor/pull/34
  - https://github.com/knowgap-lab/ai-mentor/pull/39
  - https://github.com/knowgap-lab/ai-mentor/pull/36
  - https://github.com/knowgap-lab/ai-mentor/pull/37
  - https://github.com/knowgap-lab/ai-mentor/pull/28
  - https://github.com/knowgap-lab/ai-mentor/pull/35
  - https://github.com/knowgap-lab/ai-mentor/pull/33
  - https://github.com/knowgap-lab/ai-mentor/pull/40
touches:
  - docs/handoff/LATEST.md
  - docs/handoff/release1-progress.md
  - docs/handoff/release1-actions.yaml
---

# Срез сданных доказательств

**Начало работы:** отдельной записи «начал» нет. Это запись уже сданного, без нового прогона. Предыдущий срез: [лимит агентов](2026-11-19-1705-grok-agent-limit-checkpoint.md).

**Сделано.**
- Урок: [PR #34](https://github.com/knowgap-lab/ai-mentor/pull/34), `8d335c8889e6ad758558974b38ad32e8ce112893`. На чистой Postgres: урок 201, ДЗ 201, верный ответ 5 → 2 балла, повтор без дубля, неверный 1 → 0, журнал две попытки, итог 2/4, чужой репетитор 403. На кандидате `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715` первый отказ был 422 без плана. На main прогон не повторялся.
- PDF: тот же кандидат `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715`, Gotenberg 9.5.0, `GET /lessons/synth-l01/conspect.pdf` 200, 17218 байт. Код не менялся.
- C1: [PR #39](https://github.com/knowgap-lab/ai-mentor/pull/39), `e1fb4576120c4490dfa8c22f830816de2dd581a3`. `catalog_ci` и загрузка `ct-math:L01` в Postgres 16.15.
- Экраны: [PR #36](https://github.com/knowgap-lab/ai-mentor/pull/36), `3c1ab95ef355cfa1f250ea54702b25aec42f1714`. Четыре маршрута, локальные тесты зелёные. CI не засчитан.
- Приёмщик: [PR #37](https://github.com/knowgap-lab/ai-mentor/pull/37), `50328c6ffe6b70adcee73cf8811df3bad9ea4709`. 15 тестов прошли. `python3 tools/release1_done.py` на голове #17 даёт код 1.
- Среда: [PR #28](https://github.com/knowgap-lab/ai-mentor/pull/28), `99c206058b4075b09f70564bdbe93c28c9720c87`. A1 не принят. Нет свежего Build ID, lock Python, SHA256 Go и Node, pin Chrome и дайджеста Postgres.
- Установка: [PR #35](https://github.com/knowgap-lab/ai-mentor/pull/35), `0e63192cc658695a95941ea0b110f11b7341547d`. Прошли `terraform validate` и `ansible-playbook --syntax-check`. Живая VM не ставилась. GHCR ответил 401.
- [PR #33](https://github.com/knowgap-lab/ai-mentor/pull/33), `b24f23cde1295d62132e6e7e8a60098fdebfd980`: code-check красный только на `TestPL_R5_PrereqOrder` и `TestPL_R5_S4_WarningsInEnum`. [PR #40](https://github.com/knowgap-lab/ai-mentor/pull/40), `71bb5175615b03b955923840a50623dfc1d79483`, локально чинит эти тесты. CI и два ревью ещё не засчитаны.

**Не сделано.**
- Выпуск не готов. Ни один пункт выше не объявлен принятым этапом.
- Урок не повторён на принятом main SHA.
- Свежий Cloud Build среды не сохранён.
- Живая VM, CI экранов, CI и два ревью PR #40 не засчитаны.

**Решения.** нет

**Открытые вопросы.** нет. Следующий критерий уже назван.

**Проверки.** Новый прогон урока, PDF, каталога, экранов, приёмщика и VM не запускался. SHA сверены с головами открытых PR. Код 1 приёмщика перенесён из уже сданного прогона на голове #17.

## Дальше

Следующий открытый критерий: свежий Cloud Build среды и повтор урока на принятом main SHA. Выпуск не готов.
