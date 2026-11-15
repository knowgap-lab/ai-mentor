# Прогресс выпуска 1

> Сверка git, голов веток на GitHub и проверок CI. Снято 2026-11-15 08:45 UTC. Локальный `feat/release1-demo`, `origin/feat/release1-demo` и голова PR #6 — один коммит `61f6fd5e3615b6b1aba0ef68a1e2db7b75ce32f3`. Концы `origin/feat/r1-s8-code`, `origin/feat/r1-s7-code`, `origin/feat/r1-w4`, `origin/feat/r1-w4-code` совпадают с головами PR #7–#10. `origin/main` совпадает с базой PR #6: `ad16958ae1d86f1d8c1330680500164052969106`.

## Цель

Файла `briefs/RELEASE1-GOAL.md` нет на `origin/main`, нет на `origin/feat/release1-demo` и нет среди объектов полученных веток. Цитировать цель из него нельзя.

Установка на диск и `install-machine` в этот выпуск не входят.

Красный PR #6 не сливать.

D320: дефект замороженного технического программного теста чинится отдельным PR, с двумя независимыми ревью и зелёным CI. Сначала воспроизвести и классифицировать. Если программный тест совпадает с контрактом, чинить реализацию. Программные тесты и код пишут разные сессии. На `61f6fd5` в `docs/decisions/log.md` последняя строка — D316, строки D320 нет.

## Ветки и CI

Обязательные проверки ruleset `protect-main`: `code-check` и `docs-check`. Ниже — последний прогон на голове PR.

| PR | Ветка | Состояние | Голова | База | Голова внутри #6 | CI |
|---|---|---|---|---|---|---|
| [#6](https://github.com/knowgap-lab/ai-mentor/pull/6) | `feat/release1-demo` | open, `mergeable_state` blocked | `61f6fd5e3615b6b1aba0ef68a1e2db7b75ce32f3` | `main` `ad16958ae1d86f1d8c1330680500164052969106` | это и есть #6 | `code-check` failure ([run 43613206881](https://github.com/knowgap-lab/ai-mentor/actions/runs/43613206881)), `docs-check` success ([run 43613206845](https://github.com/knowgap-lab/ai-mentor/actions/runs/43613206845)), Bugbot neutral |
| [#7](https://github.com/knowgap-lab/ai-mentor/pull/7) | `feat/r1-s8-code` | open, clean | `7c627376f988689cf2057ea5f310224d9429e36f` | `feat/r1-s8` `7a589ee98b496b40c20eae98f1593754eedcfb4c` | да, `git merge-base --is-ancestor` | `code-check` success ([run 43607787563](https://github.com/knowgap-lab/ai-mentor/actions/runs/43607787563)), `docs-check` success ([run 43607787546](https://github.com/knowgap-lab/ai-mentor/actions/runs/43607787546)), Bugbot neutral |
| [#8](https://github.com/knowgap-lab/ai-mentor/pull/8) | `feat/r1-s7-code` | open, clean | `b342ad6fd10e854c4ac9a3d9d56bb92ba3e84511` | `feat/r1-s7` `8398be42c8f579520239aabef517a0e1e70472bb` | да, `git merge-base --is-ancestor` | `code-check` success ([run 43611972035](https://github.com/knowgap-lab/ai-mentor/actions/runs/43611972035)), `docs-check` success ([run 43611972062](https://github.com/knowgap-lab/ai-mentor/actions/runs/43611972062)), Bugbot success |
| [#9](https://github.com/knowgap-lab/ai-mentor/pull/9) | `feat/r1-w4` | open, unstable | `901533b4e589dd33f15eb5e9a7f33caa4aee1d3a` | `feat/release1-demo` `2855e02bcff6649eb3d0fd27bc533630bcd8f5f5` | нет | `code-check` failure ([run 43612158153](https://github.com/knowgap-lab/ai-mentor/actions/runs/43612158153)), `docs-check` success ([run 43612158039](https://github.com/knowgap-lab/ai-mentor/actions/runs/43612158039)), Bugbot success |
| [#10](https://github.com/knowgap-lab/ai-mentor/pull/10) | `feat/r1-w4-code` | open, unstable | `759426f814fa0ddfe418e11e586e9786319a2ae8` | `feat/r1-w4` `901533b4e589dd33f15eb5e9a7f33caa4aee1d3a` | нет | `code-check` failure ([run 43613857285](https://github.com/knowgap-lab/ai-mentor/actions/runs/43613857285)), `docs-check` success ([run 43613857273](https://github.com/knowgap-lab/ai-mentor/actions/runs/43613857273)), Bugbot success |

`clean` у #7 и #8 считается относительно их баз `feat/r1-s8` и `feat/r1-s7`. Ruleset `protect-main` целится в ветку по умолчанию.

## Почему красный #6

`tools/check-code.sh` падает на `go test ./...` в `61f6fd5`. Дальше скрипт не идёт: `-race`, `go vet`, `npm test` на этом прогоне не запускались. Четыре падения:

- `TestS6_MountOperations/S6_recheck_state` — маршрут не подключён, код 404 (`core/cmd/server/mount_s6_test.go:40`)
- `TestAT_R17_S5_DiscussListOneItem` — список `[{ItemID:i1 AssignmentID:a1 VariantID:}]` (`core/internal/attempts/rules_s5_test.go:294`)
- `TestRC_R20_S6_ReportForeignTutorAndStudent` — код 403 (`core/internal/internalapi/handler_s6_test.go:517`)
- `TestPL_R5_S4_WarningsInEnum` — warning `prereq_order` вне enum контракта (`core/internal/plan/rules_s4_test.go:93`)

На `61f6fd5` пакет `core/internal/recheck` зелёный, включая то, что чинили коммиты `f033235` и `b342ad6`.

#9 и #10 стоят на `2855e02`. Этот коммит — предок `61f6fd5`, и текущая голова демо впереди на 3 коммита. Их `code-check` повторяет те же четыре падения и добавляет `TestS7_Recheck` (`events_s7_test.go:29`, конверт журнала не проходит схему). `npm test` экранов «Итог» и «Профиль» на этих прогонах не запускался.

## Очередь слияния

Очередь выключена. GraphQL `repository.mergeQueue(branch: "main")` вернул `null`. В активном ruleset `protect-main` (id `24420268`, цель `~DEFAULT_BRANCH`) есть правила `deletion`, `non_fast_forward`, `pull_request` и `required_status_checks` (`code-check`, `docs-check`). Правила `merge_queue` нет.

Владелец включает его сам: **Settings → Rules → Rulesets → protect-main → Require merge queue** (тип правила API `merge_queue`) на [ruleset 24420268](https://github.com/knowgap-lab/ai-mentor/rules/24420268). Этот PR настройку репозитория не меняет.

На `origin/feat/release1-demo`, `origin/main`, `origin/feat/r1-w4` и `origin/feat/r1-w4-code` ни один workflow не слушает `merge_group`. В этом PR триггер `merge_group` добавлен в `.github/workflows/code-check.yml` и `.github/workflows/docs-check.yml`, обе обязательные проверки.

## Дальше

Следующий исполнимый шаг: на `61f6fd5` воспроизвести и классифицировать четыре красных программных теста по D320. Совпадает с контрактом — чинить реализацию. Дефект в замороженном программном тесте — отдельный PR, два независимых ревью и зелёный CI. PR #6 остаётся открытым, пока `code-check` красный. После классификации переложить #9 и #10 на `61f6fd5`, чтобы `TestS7_Recheck` взялся из уже зелёного предка и `tools/check-code.sh` дошёл до `npm test`.
