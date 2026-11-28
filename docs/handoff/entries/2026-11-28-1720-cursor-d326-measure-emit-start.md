---
status: начал
agent: cursor
date: 2026-11-28 17:20
task: D326 --measure отдаёт поля для check_load без attestation_pass
refs:
  - tools/release1_done.py
  - tools/load/release1_load_pilot.py
  - D326
touches:
  - tools/load/release1_load_pilot.py
  - tools/load/release1_load_pilot_test.py
---

# D326: измерения пилота для последующей аттестации

**Беру в работу.** Расширить JSON `--measure`, чтобы в нём были фактические workers, длительности, profile/mix и сводка `actions.*` с p50/p95/max и метриками по собранным latency. `attestation_pass` остаётся false; непроверенные поля — в `attestation_gaps`.

**План.**
- Собрать статусы HTTP по bucket read/answer/submit/tutor_review.
- Вынести `build_measure_payload` для тестов без 20-минутного прогона.
- Добавить unit-тест на фейковых latency.
- Ветка от `origin/main`, PR в main, без merge.