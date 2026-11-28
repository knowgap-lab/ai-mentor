#!/usr/bin/env bash
# Приёмка Ubuntu VM (briefs/CURSOR-PROJECT-RELEASE1.md §8).
# Артефакт: RELEASE_SHA (по умолчанию HEAD^{commit} проверяемого дерева).
# Без QA_VM_SSH_HOST и VM_HYPERVISOR_ACCESS выполняются только проверки синтаксиса/конфига;
# шаги create, second_plan, converge, reboot, Gotenberg и no-PII помечаются blocked и дают код 1.
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "$ROOT/tools/verify_release_sha.sh"
# shellcheck source=/dev/null
source "$ROOT/tools/vm-acceptance-fields.sh"

section() {
  echo ""
  echo "=== $1 ==="
}

syntax_ok=0
guest_ok=0
blocked_steps=()

record_syntax() {
  local name="$1"
  shift
  section "syntax: $name"
  if "$@"; then
    echo "syntax:$name: exit=0"
  else
    local code=$?
    echo "syntax:$name: exit=$code"
    syntax_ok=1
    return "$code"
  fi
}

record_guest_blocked() {
  local step="$1"
  local reason="$2"
  section "guest: $step"
  echo "guest:$step: blocked ($reason)"
  blocked_steps+=("$step")
  guest_ok=1
}

verify_release_pin() {
  section "config: release.json pins"
  echo "config:evaluated_sha: app_sha=$RELEASE_SHA (HEAD / GITHUB_SHA)"
}

record_syntax verify_release_pin || true

record_syntax "ansible-playbook" bash -c "cd '$ROOT/infra/ansible' && ansible-playbook --syntax-check site.yml" || true

record_syntax "terraform-fmt" bash -c "cd '$ROOT/infra/terraform' && terraform fmt -check -recursive" || true

record_syntax "terraform-init-validate" bash -c "
  set -e
  cd '$ROOT/infra/terraform'
  terraform init -backend=false -input=false >/dev/null
  terraform validate
" || true

record_syntax "terraform-test" bash -c "cd '$ROOT/infra/terraform' && terraform test" || true

record_syntax "terraform-acceptance-vm-fmt" bash -c "cd '$ROOT/infra/terraform/acceptance-vm' && terraform fmt -check -recursive" || true

compose_config() {
  local file="$1"
  if docker compose version >/dev/null 2>&1; then
    docker compose -f "$file" config >/dev/null
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose -f "$file" config >/dev/null
  else
    python3 -c 'import sys, yaml; yaml.safe_load(open(sys.argv[1]))' "$file"
  fi
}

record_syntax "compose-prod" compose_config "$ROOT/deploy/compose.prod.yml" || true
record_syntax "compose-staging" compose_config "$ROOT/deploy/compose.staging.yml" || true
record_syntax "compose-observability" compose_config "$ROOT/deploy/compose.observability.yml" || true

section "syntax summary"
echo "syntax:overall: exit=$syntax_ok"

have_guest=0
if [[ -n "${QA_VM_SSH_HOST:-}" && -n "${VM_HYPERVISOR_ACCESS:-}" ]]; then
  have_guest=1
fi

if [[ "$have_guest" -eq 0 ]]; then
  if [[ -z "${QA_VM_SSH_HOST:-}" ]]; then
    echo "missing: QA_VM_SSH_HOST (SSH к отдельной чистой Ubuntu, не существующей пользовательской VM)"
  fi
  if [[ -z "${VM_HYPERVISOR_ACCESS:-}" ]]; then
    echo "missing: VM_HYPERVISOR_ACCESS (ограниченный доступ к гипервизору для одноразовой тестовой VM)"
  fi
  for step in create second_plan converge second_converge reboot observability verify gotenberg no_pii; do
    record_guest_blocked "$step" "no guest secrets or hypervisor access"
  done
  section_vm_evidence
  emit_vm_static_waiting "$RELEASE_SHA"
  blocked_vm="blocked (no guest secrets or hypervisor access)"
  vm_field terraform.create "$blocked_vm"
  vm_field terraform.second_plan "$blocked_vm"
  vm_field ansible.converge "$blocked_vm"
  vm_field ansible.second_converge "$blocked_vm"
  vm_field reboot.services "$blocked_vm"
  vm_field reboot.readiness "$blocked_vm"
  vm_field reboot.data not_persisted
  vm_field background_job "$blocked_vm"
  vm_field pdf.gotenberg "$blocked_vm"
  vm_field pdf.bytes_stable false
  vm_field telemetry.logs "$blocked_vm"
  vm_field telemetry.traces "$blocked_vm"
  vm_field telemetry.prometheus "$blocked_vm"
  vm_field telemetry.grafana "$blocked_vm"
  vm_field telemetry.alert_rule "$blocked_vm"
  vm_field telemetry.pii "$blocked_vm"
  vm_field restore.separate_environment false
  vm_field restore.records_compared false
  vm_field restore.backup_file_only true
  echo "guest:run_command: RELEASE_SHA=${RELEASE_SHA} QA_VM_SSH_HOST=<user@host> VM_HYPERVISOR_ACCESS=<path> bash tools/vm-acceptance.sh"
else
  section "guest: remote acceptance"
  if bash "$ROOT/tools/vm-acceptance-guest.sh"; then
    echo "guest:remote: exit=0"
  else
    guest_ok=1
    echo "guest:remote: exit=1"
  fi
fi

section "guest summary"
echo "guest:overall: exit=$guest_ok blocked=${blocked_steps[*]:-none}"

exit_code=0
if [[ "$syntax_ok" -ne 0 || "$guest_ok" -ne 0 ]]; then
  exit_code=1
fi
echo "vm-acceptance: release_sha=$RELEASE_SHA exit=$exit_code"
exit "$exit_code"