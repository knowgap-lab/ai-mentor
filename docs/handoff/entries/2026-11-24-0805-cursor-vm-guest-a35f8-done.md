# закончил

**Статус:** закончил  
**Ветка:** `cursor/release1-vm-guest-a35f8-4c59`  
**SHA кандидата:** `a35f8b0489cd355501e40d187d3a0231a1bd3077` (база `origin/main`).

**Сделано:** `tools/pin_release_sha.sh` (HEAD/`RELEASE_SHA` → terraform, ansible, D326); deploy job — Terraform 1.9.8 + `RELEASE_SHA=${{ github.sha }}`; гостевой create = apply + `vm-hypervisor-create.sh`; second_plan exit 0 only; second_converge pipefail; reboot по boot_id/uptime; блокер в `docs/handoff/release1-vm-guest-blocker.md`; D338.

**Не сделано:** гостевой прогон (нет секретов / self-hosted). PR не мержить до зелёных docs-check и code-check.

**Дальше:** владелец — секреты VM; после мержа в main — новые доказательства аттестации на новый SHA.