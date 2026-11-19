---
status: закончил
agent: grok
date: 2026-11-19 18:09
task: записать CI кандидата, локальные скрипты и кнопку срока
refs:
  - docs/handoff/entries/2026-11-19-1742-grok-release1-evidence-2.md
  - https://github.com/knowgap-lab/ai-mentor/pull/36
  - https://github.com/knowgap-lab/ai-mentor/pull/49
  - https://github.com/knowgap-lab/ai-mentor/pull/51
touches:
  - docs/handoff/LATEST.md
  - docs/handoff/release1-progress.md
  - docs/handoff/release1-actions.yaml
---

# CI кандидата и локальные скрипты

**Начало работы:** отдельной записи «начал» нет. Предыдущий срез не правится: [вторая запись](2026-11-19-1742-grok-release1-evidence-2.md). Новый прогон не запускался.

**Сделано.**
- Кандидат `feat/r1-integrate-candidate`, SHA `6f7f54016e89f71fb369efdd2ba6366bdbfd78a9`: CI, все 3 проверки без падений, событие 2026-11-19. Это экраны [#36](https://github.com/knowgap-lab/ai-mentor/pull/36) и CSRF [#49](https://github.com/knowgap-lab/ai-mentor/pull/49). В `main` этого SHA нет.
- Вершина после этого — `8c863dad4290b9f3c632ee8794ea4261f322708f`, squash [PR #51](https://github.com/knowgap-lab/ai-mentor/pull/51), скрипты среды. Прямой родитель squash — `6f7f540`. CI этого SHA зелёный: все 3 проверки без падений. Это событие ветки, не новый прогон.
- Локальные `tools/dev-start.sh` и `tools/dev-check.sh` на `09413c482a0814832bf2f66fab665e826f9fac25`: оба код 0. Веб 136, pytest 263, `walk.spec.ts` 1 passed. Прямой родитель `09413c4` — `4f0b2ccea58a4510184aba45b75683dc434403f9`, а его родитель — `6f7f540`. `09413c4` не входит в squash `8c863da`. Это не снимок Cloud Build. A1 не закрыт.
- Кнопка срока на `6f7f540`: `PATCH` homework-draft 200, deadline `2026-12-01T15:00:00Z`, на экране 18:00 Минск, `APP_ORIGIN` `http://127.0.0.1:8080`.

**Не сделано.**
- Выпуск не готов. A1 не закрыт. Снимок сборки не прогнан. Локальный `dev-check` на `09413c4` остаётся локальным.
- Стенд [#35](https://github.com/knowgap-lab/ai-mentor/pull/35) не сливать: образы на `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715`.

**Решения.** нет

**Открытые вопросы.** нет

**Проверки.** Новый прогон не запускался. Родители SHA сверены по git. CI `8c863da` записан по событию ветки: все 3 проверки без падений.

## Дальше

Следующий открытый критерий: снимок сборки. A1 не закрыт. Стенд #35 не сливать.
