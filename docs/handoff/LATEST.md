# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-24 (точка паузы release1, main a3d9344)
**Ветка документации:** `cursor/release1-attestation-0c1c` (PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69))
**Этап:** приёмка release1 — локальные доказательства записаны; выпуск **не** завершён.

## Точка паузы

**Оцениваемый коммит main:** `a3d93446e4067763d89cc41ea938ceec500d1cd9` (squash PR #75).

**Аттестация на ветке PR #69:** коммит `6388206f3cf881675221ad9504db3c6a67127692`, сводка ревью: [комментарий #7126238344](https://github.com/knowgap-lab/ai-mentor/pull/69#issuecomment-7126238344). Файл `docs/handoff/release1-attestation.json` на этом коммите — без копирования доказательств с `c6d9603` / `a35f8b0`.

## Результаты

| Область | Статус |
|---|---|
| catalog, lesson, browser (локально, synthetic) | **pass** |
| CI [code-check 45354846359](https://github.com/knowgap-lab/ai-mentor/actions/runs/45354846359) | **success** |
| CI [docs-check 45354846398](https://github.com/knowgap-lab/ai-mentor/actions/runs/45354846398) | **success** |
| CI [release1-ci 45354846472](https://github.com/knowgap-lab/ai-mentor/actions/runs/45354846472) | упал **только** job `deploy` |
| vm, load, reviews | **не** записаны как pass |

## Блокер deploy (фактическая ошибка)

В CI job `deploy` отсутствуют секреты репозитория **`QA_VM_SSH_HOST`** и **`VM_HYPERVISOR_ACCESS`**. Скрипт `vm-acceptance` вывел `release_sha=a3d93446e4067763d89cc41ea938ceec500d1cd9`, **exit=1**.

**Владелец секретов:** владелец репозитория (настройки GitHub → Secrets).

**Второй независимый ревьюер** того же коммита main **ещё не зафиксирован** — в журнале нет имени; не выдумывать.

## Что в работе сейчас

PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69) — только handoff и аттестация **вне** оцениваемого дерева; **merge не делать**.

## Следующая команда (после появления секретов)

```bash
RELEASE_SHA=a3d93446e4067763d89cc41ea938ceec500d1cd9 bash tools/vm-acceptance.sh
```

## Куда смотреть

- Запись [2026-11-24-1135-cursor-release1-pause-a3d9344.md](entries/2026-11-24-1135-cursor-release1-pause-a3d9344.md)
- [release1-attestation.json](release1-attestation.json) на ветке `cursor/release1-attestation-0c1c`
- `tools/release1_done.py` при проверке того же SHA