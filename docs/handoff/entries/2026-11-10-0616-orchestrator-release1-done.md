---
status: закончил
agent: grok
date: 2026-11-10 06:16
task: итоговый отчёт выпуска 1
refs:
  - briefs/ORCHESTRATOR-RELEASE1.md
  - docs/handoff/orchestrator-progress-release1.md
touches:
  - docs/handoff/entries/2026-11-10-0616-orchestrator-release1-done.md
  - docs/handoff/LATEST.md
  - docs/handoff/orchestrator-progress-release1.md
---

# Выпуск 1: очередь пакетов закрыта

**Начало работы:** `docs/handoff/entries/2026-11-09-1740-claude-orchestrator-release1.md`.

**Сделано.**

Интеграционная ветка `feat/release1` от `feat/python-block`, не от свежего `main`. На GitHub ничего не отправлялось.

| Пакет | Состояние | Ветка | Чем проверено | База |
|---|---|---|---|---|
| F0 каркас | принят | `feat/r1-f0` | ревью APPROVE | нет |
| F1 миграции | принят | `feat/r1-f1`, `feat/r1-f1b` | живой тест ролей, без SKIP | Postgres 16.15 |
| F2 прогонщик | принят | `feat/r1-f2` | ревью APPROVE | нет |
| G1 правила ответа | принят | `feat/r1-g1`, `feat/r1-g1b` | `go test` | нет |
| G2 журнал | принят, ревью нет | `feat/r1-g2` | `go test` | Postgres 16.15 |
| G3 проверка ответов | принят | `feat/r1-g3`, `feat/r1-g3b` | ревью, затем правка | Postgres 16.15 |
| G4 локальный вход | принят | `feat/r1-g4` | ревью APPROVE | Postgres 16.15 |
| P1 загрузчик | принят, ревью нет | `feat/r1-p1` | pytest | Postgres 16.15 |
| L0 локальный запуск | принят | `feat/r1-l0` | pytest, 2 теста | Postgres 16.15 |
| W1 страница варианта | принят | `feat/r1-w1` | vitest | нет |
| W2 экран репетитора | принят | `feat/r1-w2` | vitest | нет |
| E1 сквозной прогон | принят | `feat/r1-e1`, `feat/r1-e1b` | Go-тест и Playwright, 1 passed | Postgres 16.15 |
| G5 Telegram | принят, ревью просило правки | `feat/r1-g5` | `go test` 21 с, заморозка `7c333bf` | Postgres 16.15 |
| G6 согласие | принят, ревью просило правки | `feat/r1-g6` | `go test` | Postgres 16.15 |
| G7 исходящие | принят, ревью просило правки | `feat/r1-g7` | `go test` | Postgres 16.15 |
| G8 перепроверка | принят, ревью просило правки | `feat/r1-g8` | `go test` 15 с, заморозка `3aee3f9` | Postgres 16.15 |
| G9 уроки | принят, ревью нет | `feat/r1-g9` | `go test` 51 с, заморозка `27da0aa` | Postgres 16.15 |
| G10 ДЗ | принят, ревью нет | `feat/r1-g10` | `go test` 74 с, заморозка `a6cb031` | Postgres 16.15 |
| G11 план | принят, ревью нет | `feat/r1-g11` | `go test` 34 с, заморозка `bc972f7` | Postgres 16.15 |
| G12 конспект | принят без PDF | `feat/r1-g12` | `go test` 43 с, заморозка `8fb70ab` | Postgres 16.15 |
| W3 кабинет ученика | принят, ревью нет | `feat/r1-w3` | vitest, заморозка `ea730e0` | нет |
| O1 эксплуатация | принят | `feat/r1-o1` | pytest, 13 passed | не запускалось |

Полный прогон на `feat/release1` после W3: `tools/check.sh` — 251 passed; `tools/check-code.sh` — все пакеты `go test` зелёные, vitest 15 passed, `tsc` и ESLint/Prettier зелёные. `golangci-lint` и `ruff` на машине нет, скрипт печатает пропуск и идёт дальше. `npm audit` после установки: 4 low и 7 moderate, высоких нет.

**До какой фазы дошли.** Фазы 0–3 по очереди пакетов закрыты. Сквозной путь фазы 1 уже был зелёным: демо-каталог, ответ ученика, журнал, экран репетитора, один сценарий Playwright на Chrome. В этом заходе браузер заново не открывался; Go-тест `core/internal/e2e` в полном прогоне зелёный (5.9 с). Дальше по коду есть уроки, черновик ДЗ, план 0, конспект и кабинет ученика. PDF конспекта сервер не отдаёт.

**Не получилось.**

- PDF: вопрос `docs/handoff/entries/2026-11-10-0445-grok-orchestrator-question-g12.md`. `GET` PDF отвечает `503 pdf_unavailable`.
- Перекрёстное ревью фазы 2 вернуло `REQUEST_CHANGES`. Повторная правка не вливалась: часть находок ломает замороженные тесты (`Exchange` не вызывает `Check`, согласия без `Origin`). Список — `docs/handoff/entries/2026-11-10-0410-grok-orchestrator-phase2-review.md`.
- Ревью нет у G2, P1, G9, G10, G11, G12, W3. Перекрёстного ревью фаз 1 и 3 нет.
- C1 Python-блока по-прежнему заблокирован: `docs/handoff/entries/2026-11-09-1640-orchestrator-question-c1.md`.
- `npm install --ignore-scripts` в `web/` нужен на чистом дереве, иначе vitest не находит `dompurify` и `react-markdown`.

**Вопросы человеку.** Способ PDF. Юридическая проверка текста согласия по-прежнему не сделана, статус `draft`. Напарнику посмотреть значения по умолчанию: D300 (библиотеки), а также уже записанные D242, D244, D249–D254, D264, D267, D269, D272, D274.

**Проверки.** `PATH="$PWD/.venv/bin:$PATH" tools/check.sh` и `bash tools/check-code.sh` на `975b50a` и соседнем коммите документации. Playwright в этом заходе не повторялся.

## Дальше

Человеку: выбрать способ PDF и решить, закрывать ли находки ревью фазы 2 отдельными пакетами. Ветку не отправлять, пока не попросят.
