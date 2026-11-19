# Проверяемый прогресс выпуска 1

Срез 19 ноября 2026 года. Перед началом работы обновите сведения по GitHub, веткам и результатам проверок. [Цель выпуска](../../briefs/RELEASE1-GOAL.md) · [Порядок продолжения](../../briefs/CURSOR-PROJECT-RESUME.md).

## Ветки и проверки

- `main` — `ad16958`. В [#6](https://github.com/knowgap-lab/ai-mentor/pull/6) из `feat/release1-demo` в `main` на `61f6fd5` падали S6 JSON 404, AT-R17, RC-R20 и PL-R5. [Лог](https://github.com/knowgap-lab/ai-mentor/actions/runs/43613206881).
- В [#27](https://github.com/knowgap-lab/ai-mentor/pull/27) на `d3a5e8e` объединены серверные исправления #14–#16, PDF-ветки и экраны ученика и репетитора. Обе обязательные проверки на базе `feat/release1-demo` прошли. В CI прошли Go, 107 веб-тестов и 252 Python-теста при двух пропусках. [Code check](https://github.com/knowgap-lab/ai-mentor/actions/runs/44322308661) · [Docs check](https://github.com/knowgap-lab/ai-mentor/actions/runs/44322308573).
- Браузерный smoke на `d3a5e8e` прошёл по отчёту облачного запуска. Он проверил тестовый вход, ответ на заранее подготовленное ДЗ и просмотр попытки репетитором. Полный путь создания урока на этом кандидате сначала получил 422 без плана. Повторный прогон записан в #34 и на main не переносился.
- [#28](https://github.com/knowgap-lab/ai-mentor/pull/28) на `99c206058b4075b09f70564bdbe93c28c9720c87` добавляет `.cursor/environment.json` и общие скрипты установки, запуска и проверок. Оба обязательных CI прошли. По отчёту автора, скрипты проверены на свежем worktree #27 в подготовленной машине. A1 не принят: нет свежего Build ID, lock Python, SHA256 Go и Node, pin Chrome и дайджеста Postgres. [CI #28](https://github.com/knowgap-lab/ai-mentor/actions/runs/44531901549).
- PDF на кандидате `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715`: Gotenberg 9.5.0, `GET /lessons/synth-l01/conspect.pdf` вернул 200 и 17218 байт. Код для этого результата не менялся. Это не приёмка этапа.
- [#34](https://github.com/knowgap-lab/ai-mentor/pull/34) на `8d335c8889e6ad758558974b38ad32e8ce112893`: на чистой Postgres урок 201, ДЗ 201, верный ответ 5 → 2, повтор без дубля, неверный 1 → 0, журнал две попытки, итог 2/4, чужой репетитор 403. На кандидате первый отказ был 422 без плана. На принятом main SHA прогон не повторялся.
- [#39](https://github.com/knowgap-lab/ai-mentor/pull/39) на `e1fb4576120c4490dfa8c22f830816de2dd581a3`: `catalog_ci` и загрузка `ct-math:L01` в Postgres 16.15. Приёмка каталога не объявлена.
- [#36](https://github.com/knowgap-lab/ai-mentor/pull/36) на `3c1ab95ef355cfa1f250ea54702b25aec42f1714`: четыре маршрута, локальные тесты зелёные, CI зелёный. Браузер на `a8d2becc84dfd24e02d16a5985aa9f405f7e47af`: четыре маршрута, 360 px, CSP чистый. Кнопка срока вернула 403 csrf из-за Origin `127.0.0.1:8080`. Приёмка экранов не объявлена.
- [#37](https://github.com/knowgap-lab/ai-mentor/pull/37) на `50328c6ffe6b70adcee73cf8811df3bad9ea4709`: 15 тестов приёмщика прошли. `python3 tools/release1_done.py` на голове [#17](https://github.com/knowgap-lab/ai-mentor/pull/17) даёт код 1. Это не приёмка выпуска.
- [#35](https://github.com/knowgap-lab/ai-mentor/pull/35) на `0e63192cc658695a95941ea0b110f11b7341547d`: `terraform validate` и `ansible-playbook --syntax-check` прошли. Живая VM не ставилась. GHCR ответил 401.
- [#33](https://github.com/knowgap-lab/ai-mentor/pull/33) на `b24f23cde1295d62132e6e7e8a60098fdebfd980`: code-check красный только на `TestPL_R5_PrereqOrder` и `TestPL_R5_S4_WarningsInEnum`. [#40](https://github.com/knowgap-lab/ai-mentor/pull/40) на `71bb5175615b03b955923840a50623dfc1d79483` локально чинит эти тесты. Отдельный зачёт CI и двух ревью #40 этой записью не добавляется.
- Сданная вершина `feat/r1-integrate-candidate` — `f9bc1d068f68ca6b1afe9f4e9eb9e1b387187a32`. В ней предупреждение #33, тест `prereq_order` #40 и тестовый отправитель [#42](https://github.com/knowgap-lab/ai-mentor/pull/42). В `main` её нет. На момент записи ветка указывает на `d32091b6c82dd9d59371434a59ba3c3385387829`; этот поздний коммит не разбирался.
- Права на `8d335c8889e6ad758558974b38ad32e8ce112893`: без сессии 401, ученик и чужой 403, повтор ответа оставляет одну попытку. На принятом main SHA урок не повторялся.
- [#46](https://github.com/knowgap-lab/ai-mentor/pull/46) на `e4a9e62186632fe05ffe87d649abf0b68d46b5a8`: без согласия 423, после give приглашение 201, перепроверка 0 → 2, старый вердикт сохранён, чужой 403. CI не засчитан.
- [#44](https://github.com/knowgap-lab/ai-mentor/pull/44) на `1ceb14fc22125dc7a2da7b050531c38d286c6c1b`: два прогона с кодом 0, третий без новых строк, approve комментарием. CI ещё не зелёный.
- [#45](https://github.com/knowgap-lab/ai-mentor/pull/45) на `7a53d9136f1942eae5daf4207d92d085eb7040fb`: CI зелёный. Черновик сборки `bld-20261119-d2db2493` SUCCEEDED и не активирован. `dev-start` и `dev-check` на снимке не шли. A1 не принят.
- [#48](https://github.com/knowgap-lab/ai-mentor/pull/48) на `1eddf44baa4577efaf6330605ab1d49edb54f07b`: список «разобрать». Чужой отчёт на кандидате уже 200 владельцу и 403 чужому.
- Наблюдаемость [#20](https://github.com/knowgap-lab/ai-mentor/pull/20), приёмщик [#17](https://github.com/knowgap-lab/ai-mentor/pull/17) и workflow очереди [#24](https://github.com/knowgap-lab/ai-mentor/pull/24) открыты отдельно. У #24 зелёный CI. Очередь в правилах `main` пока не включена.
- [#12](https://github.com/knowgap-lab/ai-mentor/pull/12) содержит более ранний реестр прогресса и изменения CI. При его разборе сверяйте документацию с этой редакцией и рассматривайте самостоятельные изменения CI отдельно.

## Что осталось

Выпуск не готов. Записанные PR не заменяют приёмку этапа.

- Следующий открытый критерий: `dev-check` на снимке сборки `bld-20261119-d2db2493` и CSRF Origin для кнопки срока на `a8d2becc84dfd24e02d16a5985aa9f405f7e47af`.
- A1 не принят. Сборка #45 не активирована, `dev-start` и `dev-check` на снимке не шли. Прежние пробелы #28 этой записью не закрыты.
- CI #46 и CI #44 не засчитаны. Урок не повторён на принятом main SHA. Живая VM по #35 не ставилась, GHCR ответил 401.
- Приёмщик #37 на голове #17 даёт код 1. Ноль без полного attestation одного main SHA по-прежнему не означает готовность. [Критерии](../../briefs/CURSOR-PROJECT-RELEASE1.md#7-приёмка-на-доказательствах).
- D320 для технических PR #15, #16 и #18 и фактическое слияние в `main` остаются открытыми.

## Как сохранять доказательства

Интеграционный исполнитель записывает по каждому пакету владельца, ветку, base/head SHA, пути, заморозку тестов, ревью, команды и результаты, PR или merge SHA, зависимости и следующий шаг. При изменении кода обновляйте затронутые доказательства. Сводку обновляйте в [LATEST.md](LATEST.md), внешние действия — в [release1-actions.yaml](release1-actions.yaml). В Git храните инструкции и безопасные ссылки на результаты. Секреты и данные учеников в журнал не попадают.

## Очередь задач для передачи между аккаунтами

Используйте ID из этого реестра в заданиях исполнителям, ветках и PR. ID карточки Cursor привязан к конкретной доске и служит ссылкой для сверки. Статус из реестра и опубликованные доказательства позволяют восстановить карточку на новой доске.

Снимок доски `mentor` за 19 ноября дал ID карточек. Статусы строк с новыми SHA обновлены по сданным PR: For Review значит опубликованный SHA, не приёмку. Done не ставился.

| ID в Git | Карточка Cursor | Статус на снимке | База и материалы | Следующий шаг |
|---|---|---|---|---|
| `release1` | MEN-1 | In Progress | [Полная цель](../../briefs/RELEASE1-GOAL.md) | Принимать этапы по критериям и продолжать до полной приёмки |
| `a1-cloud-build` | MEN-2 | In Progress | [#45](https://github.com/knowgap-lab/ai-mentor/pull/45), `7a53d9136f1942eae5daf4207d92d085eb7040fb`, черновик `bld-20261119-d2db2493` | CI зелёный, сборка SUCCEEDED и не активирована. `dev-start` и `dev-check` не шли. A1 не принят |
| `server-review-1` | MEN-3 | In Progress | [#27](https://github.com/knowgap-lab/ai-mentor/pull/27), `d3a5e8e` | Найти первый независимый вердикт с SHA и замечаниями по техническим PR |
| `server-review-2` | MEN-4 | In Progress | [#27](https://github.com/knowgap-lab/ai-mentor/pull/27), `d3a5e8e` | Найти второй независимый вердикт с SHA и замечаниями по техническим PR |
| `e3-first-lesson` | MEN-5 | In Progress | [#34](https://github.com/knowgap-lab/ai-mentor/pull/34), `8d335c8889e6ad758558974b38ad32e8ce112893` | Права: без сессии 401, ученик и чужой 403, повтор ответа — одна попытка. На main не повторён |
| `codex-local-env` | MEN-6 | For Review | [#30](https://github.com/knowgap-lab/ai-mentor/pull/30), ветка `cursor/codex-desktop-env-a90e`, SHA `96ad4360a0a270c6618f95af889331da6a148f83`, база #28 | Дождаться CI #30. Не критерий сервера. TOML вызывает `tools/dev-install.sh`, `tools/dev-start.sh`, `tools/dev-check.sh` |
| `server-integration` | MEN-7 | Queued | [#6](https://github.com/knowgap-lab/ai-mentor/pull/6) · [#27](https://github.com/knowgap-lab/ai-mentor/pull/27) | Дождаться независимых ревью, проверить итоговый diff к свежему main и подтвердить слияние |
| `w7-w8-screens` | MEN-8 | In Progress | [#36](https://github.com/knowgap-lab/ai-mentor/pull/36), CI `3c1ab95ef355cfa1f250ea54702b25aec42f1714`, браузер `a8d2becc84dfd24e02d16a5985aa9f405f7e47af` | CI зелёный. Четыре маршрута, 360 px, CSP чистый. Кнопка срока 403 csrf из-за Origin `127.0.0.1:8080` |
| `c1-composer` | MEN-9 | For Review | [#39](https://github.com/knowgap-lab/ai-mentor/pull/39), `e1fb4576120c4490dfa8c22f830816de2dd581a3` | `catalog_ci` и загрузка `ct-math:L01` в Postgres 16.15 сданы. Приёмка каталога не объявлена |
| `iac-vm-draft` | MEN-10 | In Progress | [#35](https://github.com/knowgap-lab/ai-mentor/pull/35), `0e63192cc658695a95941ea0b110f11b7341547d` | `terraform validate` и syntax-check Ansible сданы. Живая VM не ставилась, GHCR 401 |
| `env-risk-review` | MEN-11 | In Progress | [#28](https://github.com/knowgap-lab/ai-mentor/pull/28) | Только чтение. A1 не принимать до этого разбора |
| `qa-gate` | MEN-12 | For Review | [#37](https://github.com/knowgap-lab/ai-mentor/pull/37), `50328c6ffe6b70adcee73cf8811df3bad9ea4709`, база #17 | 15 тестов прошли. На голове #17 приёмщик даёт код 1. Это не приёмка выпуска |
| `pl-r5-tests` | — | For Review | кандидат `f9bc1d068f68ca6b1afe9f4e9eb9e1b387187a32`, [#33](https://github.com/knowgap-lab/ai-mentor/pull/33), [#40](https://github.com/knowgap-lab/ai-mentor/pull/40), [#42](https://github.com/knowgap-lab/ai-mentor/pull/42) | Предупреждение, тест `prereq_order` и тестовый отправитель есть в кандидате. В main нет. Приёмка не объявлена |
| `consent-recheck` | — | For Review | [#46](https://github.com/knowgap-lab/ai-mentor/pull/46), `e4a9e62186632fe05ffe87d649abf0b68d46b5a8` | Без согласия 423, после give приглашение 201, перепроверка 0 → 2, старый вердикт сохранён, чужой 403. CI не засчитан |
| `lesson-seed` | — | For Review | [#44](https://github.com/knowgap-lab/ai-mentor/pull/44), `1ceb14fc22125dc7a2da7b050531c38d286c6c1b` | Два прогона с кодом 0, третий без новых строк, approve комментарием. CI ещё не зелёный |
| `triage-list` | — | For Review | [#48](https://github.com/knowgap-lab/ai-mentor/pull/48), `1eddf44baa4577efaf6330605ab1d49edb54f07b` | Чужой отчёт на кандидате: 200 владельцу и 403 чужому. Приёмка списка не объявлена |

Интегратор обновляет реестр при назначении, переходе статуса, новых доказательствах и перед передачей. Добавляйте к записи владельца задачи, публикуемые ветку/head SHA, PR, команду и фактический результат последней проверки, зависимости и следующий точный шаг. Статусы: Queued, In Progress, For Review, Done и Canceled. Если работа ждёт внешнего действия, сохраните её фактический статус и ссылку на пункт [очереди действий](release1-actions.yaml). For Review означает, что есть опубликованный SHA и материалы для ревью. Done означает, что критерии подтверждены и нужные изменения действительно влиты.

Не оставляйте передачу на момент исчерпания лимита. Перед переключением аккаунта опубликуйте проверенные шаги в ветках задач и обновите реестр в Git. Если изменения ещё не попали в main, сохраните ссылку на PR или имя опубликованной ветки реестра. Файлы, остающиеся только на облачной машине, не являются точкой восстановления.
