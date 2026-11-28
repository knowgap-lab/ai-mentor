---
status: закончил
agent: cursor
date: 2026-11-28 10:10
task: vm-acceptance fail mkisofs 1285fbd
refs:
  - entries/2026-11-24-1510-cursor-release1-pause-1285fbd.md
  - docs/handoff/release1-attestation.json@8261e18d948aa94c03787f80a22380217e673e4e
  - tools/vm-acceptance.sh
touches:
  - docs/handoff/LATEST.md
---

# Локальный vm-acceptance 1285fbd: не pass (mkisofs)

**Начало работы:** [2026-11-24-1510-cursor-release1-pause-1285fbd.md](2026-11-24-1510-cursor-release1-pause-1285fbd.md)

**Сделано.**
- Зафиксирован в handoff прогон **2026-11-28** на локальном KVM-хосте Alexander: пользователь в группе `libvirt`, есть `virsh`, `terraform`, `ansible`, `virt-install`.
- Команда: `RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh` — **не pass** (неуспешная приёмка VM).
- Terraform в libvirt создал только тома `ai-mentor-accept-guest-base` и `ai-mentor-accept-guest-disk`. **Домен не создан.**
- Ошибка при создании cloud-init ISO: **`mkisofs` не в PATH**.
- Существующий домен `ubuntu26.04` оставлен **выключенным** и **нетронутым**.
- **Нет** результата second plan, ansible, reboot, нагрузки.
- Обновлён [LATEST.md](../LATEST.md); **`release1-attestation.json` не менялся** — vm/load **не** записаны как pass.

**Не сделано.**
- Гостевая acceptance-VM, deploy CI, нагрузка, **второе независимое ревью** того же SHA main (имя ревьюера неизвестно — не выдумывать).
- Merge PR #69, изменения **main** — не выполнялись.

**Решения.** нет

**Открытые вопросы.**
- На KVM-хосте: установить `cdrtools` и повторить `vm-acceptance.sh`.
- Второй ревьюер `1285fbdda3477a409b9a89318808de13780219e2` — кто, не зафиксировано.

**Проверки.**
- Только журналирование факта прогона по сообщению владельца; аттестация по-прежнему в `8261e18`.

## Разблокировка (одна команда на том же ПК)

```bash
sudo pacman -S cdrtools
```

Пакет ставит `mkisofs` и разблокирует создание cloud-init ISO.

## Дальше

После установки `cdrtools`:

```bash
RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh
```

При успехе гостя — обновить аттестацию на ветке PR #69 (не merge без явного запроса).