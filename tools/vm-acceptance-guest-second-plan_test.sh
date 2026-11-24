#!/usr/bin/env bash
# Без гостя: второй plan обязан дать detailed-exitcode 0; код 2 — fail.
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

export TF_STUB_PLAN_EXIT=2
if step_second_plan 2>/dev/null; then
  echo "second_plan_test: expected failure on exit=2" >&2
  exit 1
fi

export TF_STUB_PLAN_EXIT=0
if ! step_second_plan 2>/dev/null; then
  echo "second_plan_test: expected success on exit=0" >&2
  exit 1
fi

export TF_STUB_PLAN_EXIT=crash
set +e
step_second_plan >/dev/null 2>&1
code=$?
set -e
if [[ "$code" -eq 0 ]]; then
  echo "second_plan_test: expected failure on terraform error" >&2
  exit 1
fi

echo "second_plan_test: ok"