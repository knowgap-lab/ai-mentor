# Блокер гостевой приёмки §8 (синтетические данные)

Evaluated SHA на main: `a35f8b0489cd355501e40d187d3a0231a1bd3077`.

## Готовая команда (CI deploy)

```bash
RELEASE_SHA=a35f8b0489cd355501e40d187d3a0231a1bd3077 bash tools/vm-acceptance.sh
```

В GitHub Actions job `deploy` то же с `RELEASE_SHA=${{ github.sha }}` после `hashicorp/setup-terraform@v3` (1.9.8).

## Фактическая ошибка без секретов (fail-closed, не pass)

```
missing: QA_VM_SSH_HOST (SSH к отдельной чистой Ubuntu, не существующей пользовательской VM)
missing: VM_HYPERVISOR_ACCESS (ограниченный доступ к гипервизору для одноразовой тестовой VM)
guest:create: blocked (no guest secrets or hypervisor access)
...
vm-acceptance: release_sha=a35f8b0489cd355501e40d187d3a0231a1bd3077 exit=1
```

## Что добавить владельцу

| Секрет / хост | Кто | Зачем |
|---------------|-----|--------|
| `VM_HYPERVISOR_ACCESS` | владелец репозитория (GitHub Actions secrets) | Путь к **исполняемому** сценарию, который создаёт одноразовую чистую Ubuntu (не существующую VM пользователя). |
| `QA_VM_SSH_HOST` | владелец репозитория | `user@host` SSH к этой Ubuntu после create. |

Self-hosted worker с теми же секретами в этом окружении **не** подключён.

Гостевой pass в аттестацию **не** записывался.