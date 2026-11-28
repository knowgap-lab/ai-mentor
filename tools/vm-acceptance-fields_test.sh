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

TMP="$(mktemp)"
export VM_FIELDS_FILE="$TMP"
vm_field terraform.create pass
vm_field ansible.converge pass
# Снимок без повторного append (как print_vm_evidence_summary).
mapfile -t seeded <"$TMP"
lines=0
for line in "${seeded[@]}"; do
  echo "vm:${line%%=*}=${line#*=}"
  lines=$((lines + 1))
done
if [[ "$lines" -ne 2 ]] || ! grep -q 'terraform.create=pass' "$TMP"; then
  echo "vm-acceptance-fields_test: snapshot replay failed" >&2
  exit 1
fi
rm -f "$TMP"

for script in \
  "$ROOT/tools/vm-acceptance-guest.sh" \
  "$ROOT/tools/vm-acceptance.sh" \
  "$ROOT/deploy/acceptance-guest-verify.sh" \
  "$ROOT/deploy/acceptance-observability-up.sh" \
  "$ROOT/deploy/acceptance-backup-drill.sh" \
  "$ROOT/deploy/acceptance-otel-probe.sh"; do
  bash -n "$script"
done

echo "vm-acceptance-fields_test: ok"