# Release 1 VM acceptance и D326 load — закончил

**Статус:** закончил  
**Ветка:** `cursor/release1-vm-load-4c59`

## Сделано

- `tools/vm-acceptance.sh` для SHA `7720ffc36c90c4475ea3f273bfd6b90f066ebb9e`: синтаксис Ansible/Terraform/Compose отдельно от шагов гостя; без секретов шаги create, second_plan, converge, reboot, Gotenberg, no_pii — blocked, код 1.
- Terraform перенесён из `cursor/iac-vm-one-sha-5bd3` с `app_sha` 7720ffc (источник, не merge #35).
- Пилот D326: `tools/load/d326-profile.json`, `tools/load/release1_load_pilot.py`, README; полный замер не в CI, pass в аттестацию не пишется.
- D337 в `docs/decisions/log.md`.

## Следующий шаг (deploy / VM)

На дереве до этого коммита job `deploy` останавливался раньше приложения: нет `tools/vm-acceptance.sh`, у GitHub runner нет `VM_HYPERVISOR_ACCESS` и `QA_VM_SSH_HOST`. **Действие владельца:** выделить отдельную чистую Ubuntu (не трогать существующую VM) и завести эти два секрета. Гипервизор в этом пакете не проверялся.

Аттестация по-прежнему в PR #69; второе независимое ревью и прогон гостя — отдельные сессии.

## Открыто

Полный §8 на госте, нагрузочный `--measure` на VM, `check_reviews` (два ревью), зелёный `deploy` в CI.