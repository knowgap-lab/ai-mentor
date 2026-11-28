# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-28 (локальный vm-acceptance, не pass — mkisofs)
**Ветка документации:** `cursor/release1-attestation-0c1c` (PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69))
**Этап:** приёмка release1 — catalog/lesson/browser записаны; VM на KVM **не** пройдена; выпуск **не** завершён.

## Точка паузы

**Оцениваемый коммит main:** `1285fbdda3477a409b9a89318808de13780219e2` (squash PR #76, D321 libvirt).

**Аттестация на ветке PR #69:** коммит `8261e18d948aa94c03787f80a22380217e673e4e`, сводка ревью: [комментарий #7135574826](https://github.com/knowgap-lab/ai-mentor/pull/69#issuecomment-7135574826). `docs/handoff/release1-attestation.json` в `8261e18` **не** обновлялся после прогона VM.

## Результаты

| Область | Статус |
|---|---|
| catalog, lesson, browser (локально, synthetic) | **pass** (в аттестации `8261e18`) |
| CI [code-check 45419835985](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835985) | **success** |
| CI [docs-check 45419835956](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835956) | **success** |
| CI [release1-ci 45419836011](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419836011) | упал **только** job `deploy` |
| **vm** (локальный KVM, 2026-11-28) | **не pass** — см. блокер mkisofs |
| load, reviews | **не** записаны как pass |

**`tools/release1_done.py`:** без изменений аттестации — vm/load/reviews по-прежнему **fail**; vm **не** помечать pass.

## Прогон vm-acceptance 2026-11-28 (локальный KVM Alexander)

Хост: KVM/libvirt, пользователь в группе `libvirt`, `virsh` / `terraform` / `ansible` / `virt-install` на месте.

```bash
RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh
```

**Итог: не pass.** Terraform создал тома libvirt `ai-mentor-accept-guest-base` и `ai-mentor-accept-guest-disk` only; **домен не создан**. Ошибка: **`mkisofs` не в PATH** при создании cloud-init ISO. Домен `ubuntu26.04` — **выключен**, **не трогали**. Нет second plan, ansible, reboot, нагрузки.

## Блокер mkisofs (разблокировка на том же ПК)

```bash
sudo pacman -S cdrtools
```

Ставит `mkisofs` и разблокирует cloud-init ISO.

## CI deploy (по-прежнему)

Секреты **`QA_VM_SSH_HOST`** и **`VM_HYPERVISOR_ACCESS`** в GitHub для job `deploy` — отдельно от локального прогона; после гостя — выставить `ubuntu@<ip новой acceptance-VM>`.

**Второй независимый ревьюер** того же SHA main **ещё не зафиксирован** — имени в журнале нет; не выдумывать.

## Что в работе сейчас

PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69) — только handoff; **merge не делать**.

## Следующая команда (после cdrtools)

```bash
RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh
```

## Куда смотреть

- Запись [2026-11-28-1010-cursor-vm-acceptance-mkisofs-fail.md](entries/2026-11-28-1010-cursor-vm-acceptance-mkisofs-fail.md)
- [2026-11-24-1510-cursor-release1-pause-1285fbd.md](entries/2026-11-24-1510-cursor-release1-pause-1285fbd.md)
- [release1-attestation.json](release1-attestation.json) (коммит `8261e18`, без vm pass)