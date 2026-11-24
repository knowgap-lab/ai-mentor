# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-24 (точка паузы release1, main 1285fbd)
**Ветка документации:** `cursor/release1-attestation-0c1c` (PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69))
**Этап:** приёмка release1 — локальные доказательства записаны; выпуск **не** завершён.

## Точка паузы

**Оцениваемый коммит main:** `1285fbdda3477a409b9a89318808de13780219e2` (squash PR #76, D321 libvirt).

**Аттестация на ветке PR #69:** коммит `8261e18d948aa94c03787f80a22380217e673e4e`, сводка ревью: [комментарий #7135574826](https://github.com/knowgap-lab/ai-mentor/pull/69#issuecomment-7135574826). Файл `docs/handoff/release1-attestation.json` в `8261e18` — свежие catalog/lesson/browser на дереве 1285fbd; без копирования с `a3d9344` и более старых SHA.

## Результаты

| Область | Статус |
|---|---|
| catalog, lesson, browser (локально, synthetic) | **pass** |
| CI [code-check 45419835985](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835985) | **success** |
| CI [docs-check 45419835956](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835956) | **success** |
| CI [release1-ci 45419836011](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419836011) | упал **только** job `deploy` (остальные jobs — success) |
| vm, load, reviews | **не** записаны как pass |

**`tools/release1_done.py`:** `exit=1` — catalog, lesson, browser pass; ci fail (deploy); vm, load, reviews fail.

## Блокер deploy (фактическая ошибка)

Отсутствуют секреты репозитория **`QA_VM_SSH_HOST`** и **`VM_HYPERVISOR_ACCESS`**.

**Где поднимается гость:** локальная машина **Alexander**, **KVM/libvirt** (новая acceptance-VM по D321), **не** существующая пользовательская VM.

**Разблокировка (владелец репозитория, GitHub → Secrets):**
- **`VM_HYPERVISOR_ACCESS`** — путь к `tools/vm-hypervisor-libvirt-apply.sh` на хосте KVM.
- **`QA_VM_SSH_HOST`** — `ubuntu@<ip новой acceptance-VM>`.

**Второй независимый ревьюер** того же коммита main **ещё не зафиксирован** — в журнале нет имени; не выдумывать.

## Что в работе сейчас

PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69) — handoff и аттестация **вне** оцениваемого дерева; **merge не делать**.

## Следующая команда (после появления секретов)

```bash
RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh
```

## Куда смотреть

- Запись [2026-11-24-1510-cursor-release1-pause-1285fbd.md](entries/2026-11-24-1510-cursor-release1-pause-1285fbd.md)
- [release1-attestation.json](release1-attestation.json) на ветке `cursor/release1-attestation-0c1c` (коммит `8261e18`)
- `tools/release1_done.py` при проверке того же SHA