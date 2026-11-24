# 2026-11-24 — cursor/release1-vm-guest-a35f8-4c59 — закончил

**Статус:** закончил

**Сделано:** runtime SHA (D339): `tools/verify_release_sha.sh` вместо `pin_release_sha.sh`; `app_sha` — Terraform variable; json без обязательного pin; load pilot сверяет `sha` с HEAD; deploy job без изменений (Terraform + `RELEASE_SHA=${{ github.sha }}`).

**Не сделано:** гостевой pass (нет `QA_VM_SSH_HOST` / `VM_HYPERVISOR_ACCESS`).

**Дальше:** зелёный CI на PR #73; секреты гипервизора.

**Вопросы:** —