#!/usr/bin/env bash
# Без гостя: ansible-playbook с кодом 3 → step_second_converge и guest_step не «pass».
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_SHA="$(git -C "$ROOT" rev-parse HEAD^{commit})"
export RELEASE_SHA
export QA_VM_SSH_HOST="acceptance-test@127.0.0.1"
export VM_HYPERVISOR_ACCESS="/bin/true"

STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT
cat >"$STUB_DIR/ansible-playbook" <<'STUB'
#!/usr/bin/env bash
echo "PLAY RECAP"
echo "acceptance_guest ok changed=0"
exit 3
STUB
chmod +x "$STUB_DIR/ansible-playbook"
export PATH="$STUB_DIR:$PATH"

# shellcheck source=/dev/null
source "$ROOT/tools/vm-acceptance-guest.sh"

LOG_OUT="$(mktemp)"
trap 'rm -f "$LOG_OUT"' EXIT

set +e
step_second_converge
step_status=$?
guest_step second_converge step_second_converge >"$LOG_OUT" 2>&1
guest_status=$?
step_out="$(cat "$LOG_OUT")"
set -e

if [[ "$step_status" -ne 3 ]]; then
  echo "second_converge_test: step_second_converge expected 3, got $step_status" >&2
  exit 1
fi
if [[ "$guest_status" -ne 3 ]]; then
  echo "second_converge_test: guest_step expected 3, got $guest_status" >&2
  exit 1
fi
if echo "$step_out" | grep -q 'guest:second_converge: exit=0'; then
  echo "second_converge_test: guest_step must not report exit=0" >&2
  exit 1
fi
if ! echo "$step_out" | grep -q 'guest:second_converge: exit=3'; then
  echo "second_converge_test: guest_step must report exit=3" >&2
  exit 1
fi
echo "second_converge_test: ok"