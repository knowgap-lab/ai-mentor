# Текущее состояние

**Обновлено:** 2026-11-19 17:25 (Минск). Выпуск 1 не готов. Этот срез только записывает уже сданное. Новый прогон не запускался, приёмка не объявлена.

- `main` на прошлом срезе — `ad16958`. [PR #6](https://github.com/knowgap-lab/ai-mentor/pull/6) открыт. [Кандидат #27](https://github.com/knowgap-lab/ai-mentor/pull/27) — `d3a5e8ed06fd2e9aeb5fa4e11a37cb8a1de95715`.
- Урок: [PR #34](https://github.com/knowgap-lab/ai-mentor/pull/34), `8d335c8889e6ad758558974b38ad32e8ce112893`, чистая Postgres, не main. На кандидате первый отказ был 422 без плана.
- PDF на кандидате: Gotenberg 9.5.0, `GET /lessons/synth-l01/conspect.pdf` 200, 17218 байт. Код не менялся.
- C1: [PR #39](https://github.com/knowgap-lab/ai-mentor/pull/39), `e1fb4576120c4490dfa8c22f830816de2dd581a3`. Экраны: [PR #36](https://github.com/knowgap-lab/ai-mentor/pull/36), `3c1ab95ef355cfa1f250ea54702b25aec42f1714`, CI не засчитан.
- Приёмщик: [PR #37](https://github.com/knowgap-lab/ai-mentor/pull/37), `50328c6ffe6b70adcee73cf8811df3bad9ea4709`, 15 тестов, на голове #17 код 1.
- Среда [PR #28](https://github.com/knowgap-lab/ai-mentor/pull/28), `99c206058b4075b09f70564bdbe93c28c9720c87`: A1 не принят. Установка [PR #35](https://github.com/knowgap-lab/ai-mentor/pull/35), `0e63192cc658695a95941ea0b110f11b7341547d`: синтаксис есть, живой VM нет, GHCR 401.
- [PR #33](https://github.com/knowgap-lab/ai-mentor/pull/33) `b24f23cde1295d62132e6e7e8a60098fdebfd980` красный на двух тестах плана. [PR #40](https://github.com/knowgap-lab/ai-mentor/pull/40) `71bb5175615b03b955923840a50623dfc1d79483` чинит их локально. CI и два ревью не засчитаны.
- [PR #30](https://github.com/knowgap-lab/ai-mentor/pull/30) на `96ad436` остаётся локальным TOML Codex и не является критерием сервера.
- Следующий открытый критерий: свежий Cloud Build среды и повтор урока на принятом main SHA. [Запись](entries/2026-11-19-1725-grok-release1-evidence.md).

Сверяйте [проверяемый прогресс](release1-progress.md), [внешние действия](release1-actions.yaml), [цель выпуска](../../briefs/RELEASE1-GOAL.md) и [правила для агентов](../../AGENTS.md). Оставляйте записи «начал» и «закончил» по [протоколу](README.md).

До подключения реальных учеников открыты D307 и утверждение [текста согласия](../../legal/consent-1.md). Демо использует только синтетические данные.
