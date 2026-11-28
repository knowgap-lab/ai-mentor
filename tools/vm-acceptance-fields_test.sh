#!/usr/bin/env bash
# Сухая проверка vm-acceptance-fields.sh и bash -n гостевых скриптов (без SSH).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "$ROOT/tools/vm-acceptance-fields.sh"

out="$(vm_field terraform.create pass)"
if [[ "$out" != "vm:terraform.create=pass" ]]; then
  echo "vm-acceptance-fields_test: unexpected line: $out" >&2
  exit 1
fi

for script in \
  "$ROOT/tools/vm-acceptance-guest.sh" \
  "$ROOT/tools/vm-acceptance.sh" \
  "$ROOT/deploy/acceptance-guest-verify.sh" \
  "$ROOT/deploy/acceptance-observability-up.sh" \
  "$ROOT/deploy/acceptance-backup-drill.sh"; do
  bash -n "$script"
done

echo "vm-acceptance-fields_test: ok"