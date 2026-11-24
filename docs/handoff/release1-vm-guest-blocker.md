# Блокер гостевой приёмки §8 (синтетические данные)

SHA под тестом — checkout (`git rev-parse HEAD` / `GITHUB_SHA`), не захардкоженный pin в json (D339).

## Готовая команда (CI deploy)

```bash
bash tools/vm-acceptance.sh
# или явно:
RELEASE_SHA="$(git rev-parse HEAD^{commit})" bash tools/vm-acceptance.sh
```

В GitHub Actions job `deploy` то же с `RELEASE_SHA=${{ github.sha }}` после `hashicorp/setup-terraform@v3` (1.9.8).

## Фактическая ошибка без секретов (fail-closed, не pass)

```
missing: QA_VM_SSH_HOST (SSH к отдельной чистой Ubuntu, не существующей пользовательской VM)
missing: VM_HYPERVISOR_ACCESS (ограниченный доступ к гипервизору для одноразовой тестовой VM)
guest:create: blocked (no guest secrets or hypervisor access)
...
vm-acceptance: release_sha=<HEAD> exit=1
```

## Разблокировка KVM (D321, D340) — владелец репозитория

**Где:** рабочая станция Alexander с **локальным KVM/libvirt** (не произвольная пользовательская Ubuntu в облаке и не уже существующая личная VM).

**Одно действие:** на этой машине один раз выдать доступ к libvirt и зафиксировать путь к скрипту в секрете `VM_HYPERVISOR_ACCESS`:

```bash
sudo apt-get install -y qemu-kvm libvirt-daemon-system virt-install terraform
sudo usermod -aG libvirt "$USER"
# новая login-сессия, затем:
export VM_HYPERVISOR_ACCESS="/path/to/checkout/ai-mentor/tools/vm-hypervisor-libvirt-apply.sh"
export QA_VM_SSH_HOST="ubuntu@<dhcp-ip-из-terraform-output-acceptance_ipv4>"
export ACCEPTANCE_SSH_PUBLIC_KEY="$(cat ~/.ssh/id_ed25519.pub)"
```

Секреты GitHub (`QA_VM_SSH_HOST`, `VM_HYPERVISOR_ACCESS`) — владелец репозитория.

**Полная приёмка на KVM-хосте:**

```bash
cd /path/to/checkout/ai-mentor
RELEASE_SHA="$(git rev-parse HEAD^{commit})" \
  VM_HYPERVISOR_ACCESS="$VM_HYPERVISOR_ACCESS" \
  QA_VM_SSH_HOST="$QA_VM_SSH_HOST" \
  ACCEPTANCE_SSH_PUBLIC_KEY="$(cat ~/.ssh/id_ed25519.pub)" \
  bash tools/vm-acceptance.sh
```

Cloud Agent worker **не** имеет libvirt (`virsh` отсутствует). Гостевой pass в аттестацию **не** записывался.