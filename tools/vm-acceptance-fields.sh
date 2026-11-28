#!/usr/bin/env bash
# Имена полей совпадают с evidence.vm в release1_done.check_vm (точки → двоеточия).
# Пример: vm:terraform.create=pass  vm:pdf.bytes_stable=true  vm:telegram.login=waiting
# Не пишет pass за шаг, который не выполнялся или упал.
set -u

VM_FIELDS_FILE="${VM_FIELDS_FILE:-}"

vm_field() {
  local key="$1"
  local value="$2"
  echo "vm:${key}=${value}"
  if [[ -n "$VM_FIELDS_FILE" ]]; then
    printf '%s=%s\n' "$key" "$value" >>"$VM_FIELDS_FILE"
  fi
}

vm_field_blocked() {
  local key="$1"
  local reason="${2:-blocked}"
  vm_field "$key" "$reason"
}

emit_vm_static_waiting() {
  local sha="$1"
  vm_field sha "$sha"
  vm_field environment clean_ubuntu_vm
  vm_field telegram.login waiting
  vm_field telegram.device_confirmation waiting
  vm_field telegram.notification waiting
}

section_vm_evidence() {
  echo ""
  echo "=== vm: evidence fields (check_vm paths) ==="
}