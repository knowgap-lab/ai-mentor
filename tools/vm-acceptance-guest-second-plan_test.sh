#!/usr/bin/env bash
# Без гостя: второй plan — только detailed-exitcode 0; 1, 2 и любой иной ненулевой код — fail.
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
if [[ "${TF_STUB_PLAN_EXIT:-0}" == "crash" ]]; then
  echo "terraform stub crash" >&2
  exit 1
fi
sub="${1:-}"
shift || true
case "$sub" in
  init) exit 0 ;;
  apply) exit 0 ;;
  plan)
    for arg in "$@"; do
      if [[ "$arg" == "-detailed-exitcode" ]]; then
        exit "${TF_STUB_PLAN_EXIT:-0}"
      fi
    done
    exit 0
    ;;
  output) echo 'null'; exit 0 ;;
  *)
    exit 0
    ;;
esac
STUB
chmod +x "$STUB_DIR/terraform"
export PATH="$STUB_DIR:$PATH"

# shellcheck source=/dev/null
source "$ROOT/tools/vm-acceptance-guest.sh"

assert_second_plan_fails() {
  local label="$1"
  if step_second_plan 2>/dev/null; then
    echo "second_plan_test: expected failure on $label" >&2
    exit 1
  fi
}

export TF_STUB_PLAN_EXIT=2
assert_second_plan_fails "exit=2"

export TF_STUB_PLAN_EXIT=0
if ! step_second_plan 2>/dev/null; then
  echo "second_plan_test: expected success on exit=0" >&2
  exit 1
fi

export TF_STUB_PLAN_EXIT=1
assert_second_plan_fails "exit=1"

export TF_STUB_PLAN_EXIT=3
LOG_OUT="$(mktemp)"
trap 'rm -f "$LOG_OUT"' EXIT
set +e
step_second_plan >/dev/null 2>&1
step_status=$?
guest_step second_plan step_second_plan >"$LOG_OUT" 2>&1
guest_status=$?
step_out="$(cat "$LOG_OUT")"
set -e
if [[ "$step_status" -eq 0 ]]; then
  echo "second_plan_test: step_second_plan must not pass on exit=3" >&2
  exit 1
fi
if [[ "$step_status" -ne 3 ]]; then
  echo "second_plan_test: step_second_plan expected 3, got $step_status" >&2
  exit 1
fi
if [[ "$guest_status" -ne 3 ]]; then
  echo "second_plan_test: guest_step expected 3, got $guest_status" >&2
  exit 1
fi
if echo "$step_out" | grep -q 'guest:second_plan: exit=0'; then
  echo "second_plan_test: guest_step must not report exit=0 on exit=3" >&2
  exit 1
fi
if ! echo "$step_out" | grep -q 'guest:second_plan: exit=3'; then
  echo "second_plan_test: guest_step must report exit=3" >&2
  exit 1
fi

export TF_STUB_PLAN_EXIT=127
assert_second_plan_fails "exit=127 (missing command style)"

export TF_STUB_PLAN_EXIT=crash
assert_second_plan_fails "terraform error (exit=1)"

echo "second_plan_test: ok"