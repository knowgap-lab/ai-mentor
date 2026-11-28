# Handoff — начал

**Статус:** начал  
**Когда:** 2026-11-28  
**Тема:** deploy self-hosted KVM — runner env для §8

**Берусь за:** job `deploy` на `acceptance-kvm`: `tools/ci/vm-acceptance-runner-env.sh` подставляет `VM_HYPERVISOR_ACCESS` и placeholder `QA_VM_SSH_HOST` при локальном libvirt; workflow вызывает source перед `vm-acceptance.sh`.

**Файлы:** `.github/workflows/release1-ci.yml`, `tools/ci/vm-acceptance-runner-env*.sh`, `tools/check-code.sh`.