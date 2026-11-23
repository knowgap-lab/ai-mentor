#!/usr/bin/env bash
# Шаги §8 на выделенной гостевой Ubuntu (QA_VM_SSH_HOST). Результат — в stdout, без записи pass в аттестацию.
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_SHA="${RELEASE_SHA:-7720ffc36c90c4475ea3f273bfd6b90f066ebb9e}"
GUEST_OVERALL=0

guest_step() {
  local name="$1"
  shift
  echo ""
  echo "=== guest: $name ==="
  if "$@"; then
    echo "guest:$name: exit=0"
  else
    local code=$?
    echo "guest:$name: exit=$code"
    GUEST_OVERALL=1
    return "$code"
  fi
}

ssh_guest() {
  ssh -o BatchMode=yes -o ConnectTimeout=30 -o StrictHostKeyChecking=accept-new "$QA_VM_SSH_HOST" "$@"
}

step_create() {
  ssh_guest "uname -a" >/dev/null
  ssh_guest "test -f /etc/os-release && grep -q '^ID=ubuntu' /etc/os-release"
}

step_second_plan() {
  (cd "$ROOT/infra/terraform" && terraform init -backend=false -input=false >/dev/null && terraform plan -detailed-exitcode -input=false)
  local code=$?
  [[ "$code" -eq 0 || "$code" -eq 2 ]]
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
  (cd "$ROOT/infra/ansible" && ansible-playbook -i "$inv" site.yml)
}

step_second_converge() {
  local inv
  inv="$(ansible_guest_inventory)"
  local log
  log="$(mktemp)"
  (cd "$ROOT/infra/ansible" && ansible-playbook -i "$inv" site.yml) | tee "$log"
  if awk '/PLAY RECAP/,0' "$log" | grep -E 'changed=[1-9][0-9]*' >/dev/null; then
    echo "second converge: unexpected changes in recap"
    rm -f "$log"
    return 1
  fi
  rm -f "$log"
}

step_reboot() {
  ssh_guest "sudo -n reboot" || ssh_guest "sudo reboot" || true
  sleep 15
  for _ in $(seq 1 30); do
    if ssh_guest "systemctl is-system-running --wait" 2>/dev/null; then
      return 0
    fi
    sleep 10
  done
  return 1
}

step_gotenberg() {
  ssh_guest "curl -sf -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/health" | grep -q 200 || \
    ssh_guest "docker ps --format '{{.Names}}' 2>/dev/null | grep -qi gotenberg"
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

if [[ -z "${QA_VM_SSH_HOST:-}" ]]; then
  echo "guest: fatal: QA_VM_SSH_HOST unset"
  exit 1
fi

guest_step create step_create || true
guest_step second_plan step_second_plan || true
guest_step converge step_converge || true
guest_step second_converge step_second_converge || true
guest_step reboot step_reboot || true
guest_step gotenberg step_gotenberg || true
guest_step no_pii step_no_pii || true

echo "guest:overall: exit=$GUEST_OVERALL (not an attestation pass)"
exit "$GUEST_OVERALL"