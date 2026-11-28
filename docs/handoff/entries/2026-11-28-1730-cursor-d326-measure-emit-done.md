---
status: закончил
agent: cursor
date: 2026-11-28 17:30
task: D326 --measure отдаёт поля для check_load без attestation_pass
refs:
  - docs/handoff/entries/2026-11-28-1720-cursor-d326-measure-emit-start.md
  - D326
touches:
  - tools/load/release1_load_pilot.py
  - tools/load/release1_load_pilot_test.py
---

# D326: измерения пилота для последующей аттестации

**Начало работы:** [2026-11-28-1720-cursor-d326-measure-emit-start.md](2026-11-28-1720-cursor-d326-measure-emit-start.md)

**Сделано.**
- `tools/load/release1_load_pilot.py`: buckets по read/answer/submit/tutor_review, `build_measure_payload`, `attestation_gaps`, `actions.*` для `load_action_problems`.
- `tools/load/release1_load_pilot_test.py`: unit-тесты без живого `--measure`.
- Ветка `cursor/d326-measure-attestation-fields-cec8` от `origin/main`.

**Не сделано.**
- Полный 20-минутный пилот на VM (по заданию не запускался).

**Решения.** нет

**Открытые вопросы.** нет

**Проверки.** `python3 -m unittest tools/load/release1_load_pilot_test.py`; `python3 -m unittest tools/release1_done_test.py`.

## Дальше
После merge — прогон `RELEASE_LOAD_PILOT_CONFIRM=1` на KVM и ручная сверка JSON с `check_load`.