---
status: начал
agent: claude
date: 2026-11-10 18:20
task: Оркестратор доделки выпуска 1, продолжение в Claude Code
refs: [briefs/ORCHESTRATOR-RELEASE1-FINISH-CLAUDE.md, briefs/ORCHESTRATOR-RELEASE1-FINISH.md, briefs/ORCHESTRATOR-RELEASE1.md, briefs/CURSOR-SETUP.md, 2026-11-10-1815-claude-orchestrator-handover-done]
touches: [briefs/r1-s3.md, briefs/r1-s4.md, briefs/r1-s5.md, briefs/r1-s6.md, briefs/r1-s7.md, briefs/r1-s8.md, briefs/r1-w0.md, docs/handoff/orchestrator-progress-release1-finish.md, docs/handoff/LATEST.md, docs/decisions/log.md, core/cmd/server, core/internal/compose]
---

# Начал: оркестратор доделки выпуска 1 в Claude Code

**Беру в работу.** Продолжаю очередь пакетов `briefs/ORCHESTRATOR-RELEASE1-FINISH.md` по правилам `briefs/ORCHESTRATOR-RELEASE1-FINISH-CLAUDE.md`. Сначала приёмка брошенного S2 (код лежит незакоммиченным в `.worktrees/s2`), одновременно тесты S3–S8 и код W0.

**План.**
- Вернуть чужую правку формата в записи `2026-11-10-1525-…-r1-wave1.md` (`git checkout --`), запустить опись и `tools/check-code.sh`, записать числа в файл прогресса.
- S2: `tools/check-code.sh`, `tools/check.sh`, ручные запросы, два ревьюера (`opus` безопасность, `fable` спецификация и контракт), повторная постановка по находкам, коммит, слияние, подключение маршрутов, опись.
- Написать брифы `briefs/r1-s3.md` … `r1-s8.md`, `r1-w0.md`, создать worktree, запустить одним сообщением шесть тест-исполнителей и код-исполнителя W0.
- Дальше конвейером: заморозка тестов → код → приёмка → ревью → слияние → опись. Файл прогресса и запись после каждого принятого пакета.
- C1 в первую волну не идёт: брифа на него нет, напишу после запуска волны.

Исполнители — субагенты `general-purpose` с моделью и effort из таблицы раздела 2 инструкции. Это прямое указание человека на эту сессию.
