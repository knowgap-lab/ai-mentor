---
status: закончил
agent: cursor
date: 2026-11-28 17:00
task: импорт ct-2027 в каталог, сид и PR
refs:
  - docs/handoff/entries/2026-11-28-1400-cursor-ct2027-import-start.md
touches:
  - content/sources/ct-2027/
  - content/ct2027_build/
  - content/composer/compose.py
  - content/tests/fixtures/ct-2027-catalog/
  - content/tests/test_ct2027_catalog.py
  - tools/dev_seed.py
  - tools/dev_seed_scenario.sql
---

## Сделано

- Снимок `content/sources/ct-2027/` (148 файлов, MANIFEST, README), сборщик `ct2027_build`, fixture с `ct-math:L02` и `ct-phys:L02`, `catalog_ci mode=main` без замечаний.
- Расширен composer: ct-2027 задачи, роли D302, импортные ключи/choice fallback, legacy опора теории.
- `dev_seed` по умолчанию грузит ct-2027; сценарий с НОД(252,198)=18.
- Тесты `test_ct2027_catalog.py`.

## Дальше

Расширить compose для полноценных choice из разобранных примеров; больше уроков в build.