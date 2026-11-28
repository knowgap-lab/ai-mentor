# Снимок материалов ct-2027 (без PII)

Санитизированная копия шаблонов уроков и карточек тем из репозитория `aleksandr-mazhul/tutoring` (ветка `main`). В снимок входят только `math/`, `physics/`, `templates/` и `plan/`; каталоги учеников, handoff, strategy и research **не** копируются.

Секции «Заметки для генерации материалов» в исходных карточках обрезаны. Метаданные снимка — в `MANIFEST.json`.

## Сборка каталога

```bash
cd content
python3 -m ct2027_build ../content/tests/fixtures/ct-2027-catalog
```

По умолчанию собираются уроки математики и физики `L02`. Локальный сид по умолчанию грузит `demo-catalog`; для ct-2027 задайте `AI_MENTOR_CT2027_CATALOG=1` или `AI_MENTOR_DEV_CATALOG` с путём к fixture.