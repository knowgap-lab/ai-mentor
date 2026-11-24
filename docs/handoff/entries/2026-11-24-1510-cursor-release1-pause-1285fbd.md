---
status: закончил
agent: cursor
date: 2026-11-24 15:10
task: release1 pause 1285fbd
refs:
  - docs/handoff/release1-attestation.json@8261e18d948aa94c03787f80a22380217e673e4e
  - entries/2026-11-24-1135-cursor-release1-pause-a3d9344.md
  - https://github.com/knowgap-lab/ai-mentor/pull/69#issuecomment-7135574826
touches:
  - docs/handoff/LATEST.md
---

# Точка паузы release1: main 1285fbd

**Начало работы:** [2026-11-24-1135-cursor-release1-pause-a3d9344.md](2026-11-24-1135-cursor-release1-pause-a3d9344.md)

**Сделано.**
- Обновлён [LATEST.md](../LATEST.md): оцениваемый **main** `1285fbdda3477a409b9a89318808de13780219e2` (squash PR #76, D321 libvirt); аттестация на PR #69 в коммите `8261e18d948aa94c03787f80a22380217e673e4e`; ревью [PR #69#issuecomment-7135574826](https://github.com/knowgap-lab/ai-mentor/pull/69#issuecomment-7135574826).
- Сводка `tools/release1_done.py`: **catalog, lesson, browser — pass**; **ci — fail** (только `deploy`); **vm, load, reviews — fail**; `exit=1`, выпуск не завершён.
- CI main push: **code-check** [45419835985](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835985) — success; **docs-check** [45419835956](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419835956) — success; **release1-ci** [45419836011](https://github.com/knowgap-lab/ai-mentor/actions/runs/45419836011) — success у jobs build, chrome, generation, tools/check-code.sh, storybook, size-limit, e2e, contract-live, tools/check.sh, docker; **deploy — failure**.

**Не сделано.**
- Чистая Ubuntu на гостевой VM, нагрузочный пилот и **второе независимое ревью** того же SHA main — не записаны; имя второго ревьюера **неизвестно** (не выдумывать).
- Merge PR #69 и изменения **main** — не выполнялись.
- `release1-attestation.json` в этом коммите **не** менялся (уже в `8261e18`).

**Решения.** нет

**Открытые вопросы.**
- Владельцу репозитория: разблокировать deploy/VM (см. блокер ниже).
- Кому поручено: второе независимое ревью коммита `1285fbdda3477a409b9a89318808de13780219e2`.

**Проверки.**
- Доказательства catalog/lesson/browser и снимок CI — в `8261e18` (`release1-attestation.json`); этот коммит только handoff.

## Блокер deploy / VM

**Фактическая ошибка:** в CI job `deploy` (и локально `vm-acceptance`) отсутствуют секреты репозитория **`QA_VM_SSH_HOST`** и **`VM_HYPERVISOR_ACCESS`**.

**Где исполняется гость:** локальная машина **Alexander** с **KVM/libvirt** (новый acceptance-домен `ai-mentor-accept-*` по D321/D340), **не** существующая пользовательская VM.

**Одна разблокировка (владелец репозитория, GitHub → Secrets):**
- **`VM_HYPERVISOR_ACCESS`** — путь к скрипту `tools/vm-hypervisor-libvirt-apply.sh` на хосте KVM.
- **`QA_VM_SSH_HOST`** — `ubuntu@<ip новой acceptance-VM>` после подъёма гостя.

## Дальше

После появления секретов:

```bash
RELEASE_SHA=1285fbdda3477a409b9a89318808de13780219e2 bash tools/vm-acceptance.sh
```

Затем — нагрузка, второе ревью, при необходимости обновление аттестации на ветке PR #69 (merge только по явному запросу).