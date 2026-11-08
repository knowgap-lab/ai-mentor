---
status: начал
agent: claude
date: 2026-11-09 11:10
task: Настроить CI для проверок документации и генерацию типов из контрактов
refs: [D97, D127, D131, D133]
touches: [.github/workflows/, tools/, docs/contracts/README.md]
---

# Начал: CI и генерация типов

Workflow GitHub Actions запускает tools/check.sh; скрипт генерации типов проверяю для Python и TypeScript.
