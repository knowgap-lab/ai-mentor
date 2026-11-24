---
status: начал
agent: cursor
date: 2026-11-24 14:36
task: D321 libvirt terraform VM
refs:
  - D321
  - D320
  - D339
touches:
  - infra/terraform/*
  - tools/vm-acceptance*.sh
  - tools/vm-hypervisor*.sh
  - docs/decisions/log.md
  - docs/handoff/release1-vm-guest-blocker.md
---

# Начал: Terraform KVM/libvirt для чистой Ubuntu (D321)

**Беру в работу.** Гостевая приёмка §8: reproducible VM через libvirt, second_plan exit 0, D339 runtime SHA, без ослабления D320.

**План.** libvirt-модуль; скрипт apply; гостевые шаги create_vm=true; тесты second_plan/second_converge; handoff; PR с main.