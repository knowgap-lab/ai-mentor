#!/usr/bin/env bash
# Без гостя: second_plan передаёт ACCEPTANCE_SSH_PUBLIC_KEY в TF_VAR_acceptance_ssh_public_key.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_SHA="$(git -C "$ROOT" rev-parse HEAD^{commit})"
export RELEASE_SHA
export QA_VM_SSH_HOST="acceptance-test@127.0.0.1"
export VM_HYPERVISOR_ACCESS="/bin/true"

STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT

cat >"$STUB_DIR/terraform" <<'STUB'
#!/usr/bin/env bash
sub="${1:-}"
shift || true
case "$sub" in
  init) exit 0 ;;
  plan)
    for arg in "$@"; do
      if [[ "$arg" == "-detailed-exitcode" ]]; then
        if [[ -z "${TF_VAR_acceptance_ssh_public_key:-}" ]]; then
          echo "acceptance_ssh_public_key required" >&2
          exit 1
        fi
        exit "${TF_STUB_PLAN_EXIT:-0}"
      fi
    done
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
STUB
chmod +x "$STUB_DIR/terraform"
export PATH="$STUB_DIR:$PATH"
export TF_STUB_PLAN_EXIT=0

# shellcheck source=/dev/null
source "$ROOT/tools/vm-acceptance-guest.sh"

unset TF_VAR_acceptance_ssh_public_key
unset ACCEPTANCE_SSH_PUBLIC_KEY

set +e
step_second_plan >/dev/null 2>&1
missing_status=$?
set -e
if [[ "$missing_status" -ne 1 ]]; then
  echo "second_plan_ssh_key_test: missing key expected exit=1, got $missing_status" >&2
  exit 1
fi

export ACCEPTANCE_SSH_PUBLIC_KEY="ssh-ed25519 AAAA acceptance-key-regression"
unset TF_VAR_acceptance_ssh_public_key

set +e
step_second_plan >/dev/null 2>&1
with_key_status=$?
set -e
if [[ "$with_key_status" -ne 0 ]]; then
  echo "second_plan_ssh_key_test: with ACCEPTANCE_SSH_PUBLIC_KEY expected exit=0, got $with_key_status" >&2
  exit 1
fi

export TF_STUB_PLAN_EXIT=2
set +e
step_second_plan >/dev/null 2>&1
drift_status=$?
set -e
if [[ "$drift_status" -ne 2 ]]; then
  echo "second_plan_ssh_key_test: with key and drift expected exit=2, got $drift_status" >&2
  exit 1
fi

echo "second_plan_ssh_key_test: ok"