---
status: начал
agent: grok
date: 2026-11-09 14:09
task: Подготовка Cursor к оркестратору выпуска 1
refs:
  - briefs/CURSOR-SETUP.md
  - AGENTS.md
touches:
  - .cursor/rules/code-guidelines.mdc
  - .cursor/rules/frontend-design.mdc
---

# Подготовка Cursor

## Для статуса «начал»

**Беру в работу.** Настройка Cursor по `briefs/CURSOR-SETUP.md`, раздел 1, без запуска оркестратора и без коммита.

**План.**
- Записать два MCP-сервера и проверить, что инструменты видны.
- Показать diff автозапуска и глобальных правил и не записывать их до «ок».
- Поставить ESLint и Prettier, добавить два правила в репозиторий, проверить context-mode и пробный worktree.
