# Handoff — закончил

**Статус:** закончил  
**Когда:** 2026-11-28  
**Тема:** deploy self-hosted KVM — runner env для §8

**Сделано:** `tools/ci/vm-acceptance-runner-env.sh` + тест; deploy job source перед приёмкой; запрет SSH на `192.168.122.62` (или `QA_VM_SSH_FORBIDDEN_HOSTS`).

**Не сделано:** прогон гостя на железе Alexander (нужен libvirt на runner user `stranger`).

**Дальше:** PR; на runner: группа `libvirt`, `~/.ssh/*.pub` или `ACCEPTANCE_SSH_PUBLIC_KEY` для cloud-init.