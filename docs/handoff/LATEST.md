# Текущее состояние

**Обновлено:** 2026-11-19 18:37 (Минск). Выпуск 1 не готов. Полная приёмка не объявлена. Этот сеанс код не менял.

- `main` остаётся `ad16958`. Кандидат `feat/r1-integrate-candidate`: `6f7f54016e89f71fb369efdd2ba6366bdbfd78a9` — экраны [#36](https://github.com/knowgap-lab/ai-mentor/pull/36) и CSRF [#49](https://github.com/knowgap-lab/ai-mentor/pull/49), CI все 3 проверки без падений, событие 2026-11-19. В `main` нет.
- Прежняя вершина кандидата — `8c863dad4290b9f3c632ee8794ea4261f322708f`, squash [PR #51](https://github.com/knowgap-lab/ai-mentor/pull/51). CI этого SHA зелёный: все 3 проверки без падений.
- Локальные `dev-start.sh` и `dev-check.sh` на `09413c482a0814832bf2f66fab665e826f9fac25`: оба код 0, веб 136, pytest 263, `walk.spec.ts` 1 passed. Прямой родитель — `4f0b2cc`, не сам `6f7f540`. Это локальная машина.
- Кнопка срока на `6f7f540`: `PATCH` homework-draft 200, deadline `2026-12-01T15:00:00Z`, на экране 18:00 Минск, `APP_ORIGIN` `http://127.0.0.1:8080`.
- Снимок `bld-20261119-d2db2493-2f91-4650-af03-74dcfb950fa1`, SHA `8c863dad4290b9f3c632ee8794ea4261f322708f`: вершина кандидата совпала. `dev-start.sh` код 0, `dev-check.sh` код 0. Это прогон на снимке сборки, не локальная машина. Код не менялся. Полная приёмка и `main` не закрыты.
- Новая вершина кандидата — `dc1e1604836ea8e7130031a4ee0d2151956d956e`, squash [PR #52](https://github.com/knowgap-lab/ai-mentor/pull/52) поверх `8c863da`. CI все 3 проверки без падений. В `main` нет. Снимок `dev-check` был на `8c863da`, не на этом SHA.
- Стенд не сдвинулся. [#35](https://github.com/knowgap-lab/ai-mentor/pull/35) не менялся: образы на `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715`, живой VM нет. Образы `dc1e160` собраны локально. Push в `ghcr.io/knowgap-lab` отклонён: `installation not allowed to Create organization package`.
- Следующий открытый критерий: стенд. Полная приёмка и `main` не закрыты. [Запись](entries/2026-11-19-1837-grok-release1-evidence-5.md).

Сверяйте [проверяемый прогресс](release1-progress.md), [внешние действия](release1-actions.yaml), [цель выпуска](../../briefs/RELEASE1-GOAL.md) и [правила для агентов](../../AGENTS.md). Оставляйте записи «начал» и «закончил» по [протоколу](README.md).

До подключения реальных учеников открыты D307 и утверждение [текста согласия](../../legal/consent-1.md). Демо использует только синтетические данные.
