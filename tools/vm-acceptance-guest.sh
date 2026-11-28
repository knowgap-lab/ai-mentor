#!/usr/bin/env bash
# Шаги §8 на выделенной гостевой Ubuntu (QA_VM_SSH_HOST). Результат — в stdout, без записи pass в аттестацию.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# RELEASE_SHA задаётся tools/verify_release_sha.sh (родитель) или окружением deploy.
RELEASE_SHA="${RELEASE_SHA:?RELEASE_SHA unset; run via tools/vm-acceptance.sh}"
GUEST_OVERALL=0

sync_acceptance_ssh_public_key_tf_var() {
  if [[ -n "${ACCEPTANCE_SSH_PUBLIC_KEY:-}" ]]; then
    export TF_VAR_acceptance_ssh_public_key="$ACCEPTANCE_SSH_PUBLIC_KEY"
  fi
}

guest_step() {
  local name="$1"
  shift
  local code
  echo ""
  echo "=== guest: $name ==="
  "$@"
  code=$?
  if [[ "$code" -eq 0 ]]; then
    echo "guest:$name: exit=0"
  else
    echo "guest:$name: exit=$code"
    GUEST_OVERALL=1
    return "$code"
  fi
}

ssh_guest() {
  ssh -o BatchMode=yes -o ConnectTimeout=30 -o StrictHostKeyChecking=accept-new "$QA_VM_SSH_HOST" "$@"
}

sync_acceptance_ssh_host_from_tf() {
  local ip ssh_user host
  ip="$(terraform_vm_dir terraform output -raw acceptance_ipv4 2>/dev/null || true)"
  if [[ -z "$ip" ]]; then
    return 0
  fi
  if [[ "$QA_VM_SSH_HOST" == *@* ]]; then
    ssh_user="${QA_VM_SSH_HOST%%@*}"
    host="${QA_VM_SSH_HOST#*@}"
    if [[ "$host" != "$ip" ]]; then
      export QA_VM_SSH_HOST="${ssh_user}@${ip}"
    fi
  else
    export QA_VM_SSH_HOST="$ip"
  fi
}

wait_ssh_guest() {
  local attempt
  for attempt in $(seq 1 60); do
    sync_acceptance_ssh_host_from_tf
    if ssh_guest "true" 2>/dev/null; then
      return 0
    fi
    sleep 5
  done
  echo "create: guest SSH did not come up"
  return 1
}

terraform_contract_dir() {
  (
    cd "$ROOT/infra/terraform"
    terraform init -backend=false -input=false >/dev/null
    "$@" -var="app_sha=${RELEASE_SHA}" -var="create_vm=false"
  )
}

terraform_vm_dir() {
  sync_acceptance_ssh_public_key_tf_var
  (
    cd "$ROOT/infra/terraform/acceptance-vm"
    terraform init -input=false >/dev/null
    "$@"
  )
}

step_create() {
  terraform_contract_dir terraform apply -auto-approve -input=false
  bash "$ROOT/tools/vm-hypervisor-create.sh"
  terraform_vm_dir terraform apply -auto-approve -input=false
  sync_acceptance_ssh_host_from_tf
  wait_ssh_guest
  ssh_guest "uname -a" >/dev/null
  ssh_guest "test -f /etc/os-release && grep -q '^ID=ubuntu' /etc/os-release"
}

step_second_plan() {
  terraform_vm_dir terraform apply -auto-approve -input=false
  local code=0
  terraform_vm_dir terraform plan -detailed-exitcode -input=false || code=$?
  if [[ "$code" -eq 0 ]]; then
    return 0
  fi
  case "$code" in
    1) echo "second_plan: terraform plan failed (exit=1)" ;;
    2) echo "second_plan: unexpected changes pending (detailed-exitcode=2)" ;;
    *) echo "second_plan: terraform plan exit=$code" ;;
  esac
  return "$code"
}

run_ansible_site() {
  local inv="$1"
  local -a extra=(
    -e "app_sha=${RELEASE_SHA}"
    -e "acceptance_local_images=${ACCEPTANCE_LOCAL_IMAGES:-false}"
  )
  if [[ "${ACCEPTANCE_LOCAL_IMAGES:-false}" == true ]]; then
    : "${ACCEPTANCE_CORE_IMAGE_REPOSITORY:=ai-mentor-core-local}"
    : "${ACCEPTANCE_WEB_IMAGE_REPOSITORY:=ai-mentor-web-local}"
    : "${ACCEPTANCE_CONTENT_IMAGE_REPOSITORY:=ai-mentor-content-local}"
    extra+=(
      -e "core_image_repository=${ACCEPTANCE_CORE_IMAGE_REPOSITORY}"
      -e "web_image_repository=${ACCEPTANCE_WEB_IMAGE_REPOSITORY}"
      -e "content_image_repository=${ACCEPTANCE_CONTENT_IMAGE_REPOSITORY}"
    )
  fi
  (
    cd "$ROOT/infra/ansible" && ansible-playbook -i "$inv" site.yml "${extra[@]}"
  )
}

ansible_guest_inventory() {
  local inv="$ROOT/infra/ansible/inventory/acceptance-guest.yml"
  mkdir -p "$(dirname "$inv")"
  local ssh_user="${QA_VM_SSH_USER:-ubuntu}"
  local ssh_host="$QA_VM_SSH_HOST"
  if [[ "$QA_VM_SSH_HOST" == *@* ]]; then
    ssh_user="${QA_VM_SSH_HOST%%@*}"
    ssh_host="${QA_VM_SSH_HOST#*@}"
  fi
  cat >"$inv" <<EOF
all:
  hosts:
    acceptance_guest:
      ansible_host: ${ssh_host}
      ansible_user: ${ssh_user}
      cookie_domain_prod: acceptance-prod.invalid
      cookie_domain_staging: acceptance-staging.invalid
EOF
  printf '%s' "$inv"
}

step_converge() {
  local inv
  inv="$(ansible_guest_inventory)"
  run_ansible_site "$inv"
}

step_second_converge() {
  local inv
  inv="$(ansible_guest_inventory)"
  local log
  log="$(mktemp)"
  # Кавычки в trap — путь зафиксирован до RETURN (иначе set -u и local log уже снят).
  trap "rm -f '${log}'" RETURN
  set -o pipefail
  local playbook_status
  run_ansible_site "$inv" 2>&1 | tee "$log"
  playbook_status=$?
  if [[ "$playbook_status" -ne 0 ]]; then
    return "$playbook_status"
  fi
  if awk '/PLAY RECAP/,0' "$log" | grep -E 'changed=[1-9][0-9]*' >/dev/null; then
    echo "second converge: unexpected changes in recap"
    return 1
  fi
}

step_reboot() {
  local boot_before uptime_before boot_after
  boot_before="$(ssh_guest 'cat /proc/sys/kernel/random/boot_id')"
  uptime_before="$(ssh_guest 'cut -d. -f1 /proc/uptime')"
  if [[ -z "$boot_before" || -z "$uptime_before" ]]; then
    echo "reboot: cannot read boot_id or uptime before reboot"
    return 1
  fi
  ssh_guest "sudo -n reboot" || ssh_guest "sudo reboot"
  sleep 20
  for _ in $(seq 1 40); do
    if ssh_guest "true" 2>/dev/null; then
      break
    fi
    sleep 10
  done
  if ! ssh_guest "true" 2>/dev/null; then
    echo "reboot: guest did not return after reboot"
    return 1
  fi
  boot_after="$(ssh_guest 'cat /proc/sys/kernel/random/boot_id')"
  uptime_after="$(ssh_guest 'cut -d. -f1 /proc/uptime')"
  if [[ -z "$boot_after" || -z "$uptime_after" ]]; then
    echo "reboot: cannot read boot_id or uptime after reconnect"
    return 1
  fi
  if [[ "$boot_before" == "$boot_after" ]]; then
    echo "reboot: boot_id unchanged ($boot_before)"
    return 1
  fi
  if [[ "$uptime_after" -ge "$uptime_before" ]]; then
    echo "reboot: uptime did not drop (before=$uptime_before after=$uptime_after)"
    return 1
  fi
}

step_gotenberg() {
  ssh_guest "curl -sf -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/health" | grep -q 200 || \
    ssh_guest "sudo docker ps --format '{{.Names}}' 2>/dev/null | grep -qi gotenberg"
}

step_no_pii() {
  ssh_guest "journalctl -u 'ai-mentor*' --no-pager -n 200 2>/dev/null || true" | \
    python3 -c '
import re, sys
text = sys.stdin.read()
bad = re.search(r"(?i)(telegram_id|birth_date|first_name)", text)
sys.exit(1 if bad else 0)
'
}

run_guest_acceptance() {
  if [[ -z "${QA_VM_SSH_HOST:-}" ]]; then
    echo "guest: fatal: QA_VM_SSH_HOST unset"
    exit 1
  fi
  if [[ -z "${VM_HYPERVISOR_ACCESS:-}" ]]; then
    echo "guest: fatal: VM_HYPERVISOR_ACCESS unset"
    exit 1
  fi

  guest_step create step_create
  guest_step second_plan step_second_plan
  guest_step converge step_converge
  guest_step second_converge step_second_converge
  guest_step reboot step_reboot
  guest_step gotenberg step_gotenberg
  guest_step no_pii step_no_pii

  echo "guest:overall: exit=$GUEST_OVERALL (not an attestation pass)"
  exit "$GUEST_OVERALL"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_guest_acceptance
fi