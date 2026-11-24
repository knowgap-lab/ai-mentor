---
status: закончил
agent: cursor
date: 2026-11-24 15:00
task: D321 libvirt terraform VM
refs:
  - D321
  - D340
  - docs/handoff/entries/2026-11-24-1436-cursor-d321-libvirt-start.md
touches:
  - infra/terraform/acceptance-vm/*
  - tools/vm-hypervisor-libvirt-apply.sh
  - docs/handoff/release1-vm-guest-blocker.md
---

# Закончил: Terraform KVM/libvirt (D321)

**Начало:** [2026-11-24-1436-cursor-d321-libvirt-start.md](2026-11-24-1436-cursor-d321-libvirt-start.md)

**Сделано.**
- D340; стек `infra/terraform/acceptance-vm` (libvirt, noble cloud, `ai-mentor-accept-*`).
- `tools/vm-hypervisor-libvirt-apply.sh`; гостевой `second_plan` на том же стеке; тесты second_plan/second_converge.
- Блокер с командой разблокировки для KVM-хоста Alexander.

**Не сделано.** Полный guest (нет libvirt на worker); merge; PR #69.

**Проверки.** `terraform test` (корень) 2/2; second_plan/second_converge тесты; load `--validate-config`.

## Дальше

PR с CI; гостевой прогон на KVM-хосте владельца после секретов.