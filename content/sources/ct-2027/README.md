# Снимок материалов ct-2027 (без PII)

Санитизированная копия шаблонов уроков и карточек тем из репозитория `aleksandr-mazhul/tutoring` (ветка `main`). В снимок входят только `math/`, `physics/`, `templates/` и `plan/`; каталоги учеников, handoff, strategy и research **не** копируются.

Секции «Заметки для генерации материалов» в исходных карточках обрезаны. Метаданные снимка — в `MANIFEST.json`.

## Сборка каталога

```bash
cd content
python3 -m ct2027_build ../content/tests/fixtures/ct-2027-catalog
```

По умолчанию собираются уроки математики и физики `L02`. Локальный сид `tools/dev_seed.py` грузит fixture `content/tests/fixtures/ct-2027-catalog`. Чтобы вернуть синтетический `demo-catalog`, задайте `AI_MENTOR_DEV_CATALOG`.