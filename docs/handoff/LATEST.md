# Текущее состояние

**Обновлено:** 2026-11-19 17:42 (Минск). Выпуск 1 не готов. Этот срез только дописывает уже сданное. Новый прогон не запускался, приёмка не объявлена.

- `main` остаётся `ad16958`. Сданная вершина кандидата `feat/r1-integrate-candidate` — `f9bc1d068f68ca6b1afe9f4e9eb9e1b387187a32`: в ней [#33](https://github.com/knowgap-lab/ai-mentor/pull/33), [#40](https://github.com/knowgap-lab/ai-mentor/pull/40) и [#42](https://github.com/knowgap-lab/ai-mentor/pull/42). В `main` её нет.
- Права на `8d335c8889e6ad758558974b38ad32e8ce112893`: без сессии 401, ученик и чужой 403, повтор ответа — одна попытка.
- Согласие и перепроверка: [PR #46](https://github.com/knowgap-lab/ai-mentor/pull/46), `e4a9e62186632fe05ffe87d649abf0b68d46b5a8`. CI не засчитан. Сид: [PR #44](https://github.com/knowgap-lab/ai-mentor/pull/44), `1ceb14fc22125dc7a2da7b050531c38d286c6c1b`, approve комментарием, CI ещё не зелёный.
- Экраны [PR #36](https://github.com/knowgap-lab/ai-mentor/pull/36): CI на `3c1ab95ef355cfa1f250ea54702b25aec42f1714` зелёный. Браузер на `a8d2becc84dfd24e02d16a5985aa9f405f7e47af` получил 403 csrf на кнопке срока из-за Origin `127.0.0.1:8080`.
- Среда [PR #45](https://github.com/knowgap-lab/ai-mentor/pull/45), `7a53d9136f1942eae5daf4207d92d085eb7040fb`: CI зелёный, черновик `bld-20261119-d2db2493` SUCCEEDED и не активирован. `dev-start` и `dev-check` на снимке не шли. A1 не принят.
- Список «разобрать»: [PR #48](https://github.com/knowgap-lab/ai-mentor/pull/48), `1eddf44baa4577efaf6330605ab1d49edb54f07b`. Чужой отчёт на кандидате: 200 владельцу, 403 чужому.
- Следующий открытый критерий: `dev-check` на снимке сборки и CSRF Origin. [Запись](entries/2026-11-19-1742-grok-release1-evidence-2.md).

Сверяйте [проверяемый прогресс](release1-progress.md), [внешние действия](release1-actions.yaml), [цель выпуска](../../briefs/RELEASE1-GOAL.md) и [правила для агентов](../../AGENTS.md). Оставляйте записи «начал» и «закончил» по [протоколу](README.md).

До подключения реальных учеников открыты D307 и утверждение [текста согласия](../../legal/consent-1.md). Демо использует только синтетические данные.
