---
status: закончил
agent: grok
date: 2026-11-19 17:42
task: дописать уже сданные доказательства выпуска 1
refs:
  - docs/handoff/entries/2026-11-19-1725-grok-release1-evidence.md
  - https://github.com/knowgap-lab/ai-mentor/pull/46
  - https://github.com/knowgap-lab/ai-mentor/pull/44
  - https://github.com/knowgap-lab/ai-mentor/pull/36
  - https://github.com/knowgap-lab/ai-mentor/pull/45
  - https://github.com/knowgap-lab/ai-mentor/pull/48
touches:
  - docs/handoff/LATEST.md
  - docs/handoff/release1-progress.md
  - docs/handoff/release1-actions.yaml
---

# Второй срез сданных доказательств

**Начало работы:** отдельной записи «начал» нет. Предыдущий срез не правится: [доказательства](2026-11-19-1725-grok-release1-evidence.md). Новый прогон не запускался.

**Сделано.**
- Кандидат `feat/r1-integrate-candidate`, сданная вершина `f9bc1d068f68ca6b1afe9f4e9eb9e1b387187a32`. В ней предупреждение [#33](https://github.com/knowgap-lab/ai-mentor/pull/33), тест `prereq_order` [#40](https://github.com/knowgap-lab/ai-mentor/pull/40) и тестовый отправитель [#42](https://github.com/knowgap-lab/ai-mentor/pull/42). В `main` этой вершины нет.
- Права на прежнем SHA урока `8d335c8889e6ad758558974b38ad32e8ce112893`: без сессии 401, ученик и чужой 403, повтор ответа оставляет одну попытку.
- Согласие и перепроверка: [PR #46](https://github.com/knowgap-lab/ai-mentor/pull/46), `e4a9e62186632fe05ffe87d649abf0b68d46b5a8`. Без согласия 423, после give приглашение 201, перепроверка 0 → 2, старый вердикт сохранён, чужой 403. CI не засчитан.
- Сид: [PR #44](https://github.com/knowgap-lab/ai-mentor/pull/44), `1ceb14fc22125dc7a2da7b050531c38d286c6c1b`. Два прогона с кодом 0, третий без новых строк, approve комментарием. CI ещё не зелёный.
- Экраны [PR #36](https://github.com/knowgap-lab/ai-mentor/pull/36): на `3c1ab95ef355cfa1f250ea54702b25aec42f1714` CI зелёный. Браузер на `a8d2becc84dfd24e02d16a5985aa9f405f7e47af`: четыре маршрута, 360 px, CSP чистый. Кнопка срока вернула 403 csrf из-за Origin `127.0.0.1:8080`.
- Среда: [PR #45](https://github.com/knowgap-lab/ai-mentor/pull/45), `7a53d9136f1942eae5daf4207d92d085eb7040fb`, CI зелёный. Черновик сборки `bld-20261119-d2db2493` имеет исход SUCCEEDED и не активирован. `dev-start` и `dev-check` на снимке не шли. A1 не принят.
- Список «разобрать»: [PR #48](https://github.com/knowgap-lab/ai-mentor/pull/48), `1eddf44baa4577efaf6330605ab1d49edb54f07b`. Чужой отчёт на кандидате уже 200 владельцу и 403 чужому.

**Не сделано.**
- Выпуск не готов. Ни один пункт не объявлен принятым.
- `dev-check` на снимке сборки не выполнен. CSRF Origin для кнопки срока не закрыт.
- Урок по-прежнему не повторён на принятом main SHA.

**Решения.** нет

**Открытые вопросы.** нет. Следующий критерий уже назван.

**Проверки.** Новый прогон не запускался. Короткие SHA сверены с опубликованными головами PR и с объектом `f9bc1d068f68ca6b1afe9f4e9eb9e1b387187a32`. На момент записи ветка кандидата указывает на `d32091b6c82dd9d59371434a59ba3c3385387829`, на один коммит позже сданного среза. Этот коммит не разбирался.

## Дальше

Следующий открытый критерий: `dev-check` на снимке сборки и CSRF Origin. Выпуск не готов.
