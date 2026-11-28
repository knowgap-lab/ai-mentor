#!/usr/bin/env bash
# Self-hosted KVM runners: fill VM_HYPERVISOR_ACCESS and QA_VM_SSH_HOST when libvirt can
# create the one-off acceptance guest (D340). GitHub secrets stay optional overrides.
# Does not print SSH keys or other secret material.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIBVIRT_HELPER="$ROOT/tools/vm-hypervisor-libvirt-apply.sh"

qa_ssh_host_part() {
  local spec="$1"
  if [[ "$spec" == *@* ]]; then
    echo "${spec#*@}"
  else
    echo "$spec"
  fi
}

qa_ssh_forbidden_host() {
  local host="$1"
  local forbidden
  IFS=',' read -ra forbidden <<<"${QA_VM_SSH_FORBIDDEN_HOSTS:-192.168.122.62}"
  for item in "${forbidden[@]}"; do
    item="${item#"${item%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    if [[ -n "$item" && "$host" == "$item" ]]; then
      return 0
    fi
  done
  return 1
}

if [[ -n "${QA_VM_SSH_HOST:-}" ]]; then
  if qa_ssh_forbidden_host "$(qa_ssh_host_part "$QA_VM_SSH_HOST")"; then
    echo "vm-acceptance-runner-env: QA_VM_SSH_HOST targets forbidden existing user VM; ignoring (use libvirt-created guest)" >&2
    unset QA_VM_SSH_HOST
  fi
fi

if [[ -n "${VM_HYPERVISOR_ACCESS:-}" ]]; then
  if [[ ! -f "${VM_HYPERVISOR_ACCESS}" || ! -x "${VM_HYPERVISOR_ACCESS}" ]]; then
    echo "vm-acceptance-runner-env: VM_HYPERVISOR_ACCESS is not an executable file; ignoring" >&2
    unset VM_HYPERVISOR_ACCESS
  fi
fi

libvirt_usable=0
if [[ -x "$LIBVIRT_HELPER" ]]; then
  export LIBVIRT_DEFAULT_URI="${LIBVIRT_DEFAULT_URI:-qemu:///system}"
  if command -v virsh >/dev/null 2>&1; then
    if virsh -c "$LIBVIRT_DEFAULT_URI" uri >/dev/null 2>&1; then
      libvirt_usable=1
    fi
  fi
fi

if [[ -z "${VM_HYPERVISOR_ACCESS:-}" && "$libvirt_usable" -eq 1 ]]; then
  export VM_HYPERVISOR_ACCESS="$LIBVIRT_HELPER"
  echo "vm-acceptance-runner-env: using in-repo libvirt helper for VM_HYPERVISOR_ACCESS" >&2
fi

if [[ -z "${QA_VM_SSH_HOST:-}" && "$libvirt_usable" -eq 1 ]]; then
  export QA_VM_SSH_USER="${QA_VM_SSH_USER:-ubuntu}"
  export QA_VM_SSH_HOST="${QA_VM_SSH_USER}@acceptance-pending"
  echo "vm-acceptance-runner-env: QA_VM_SSH_HOST placeholder until terraform acceptance_ipv4 (guest create)" >&2
fi

if [[ -n "${GITHUB_ENV:-}" ]]; then
  if [[ -n "${VM_HYPERVISOR_ACCESS:-}" ]]; then
    {
      echo "VM_HYPERVISOR_ACCESS<<EOF_VM_HYPERVISOR_ACCESS"
      echo "$VM_HYPERVISOR_ACCESS"
      echo "EOF_VM_HYPERVISOR_ACCESS"
    } >>"$GITHUB_ENV"
  fi
  if [[ -n "${QA_VM_SSH_HOST:-}" ]]; then
    echo "QA_VM_SSH_HOST=$QA_VM_SSH_HOST" >>"$GITHUB_ENV"
  fi
fi

export VM_HYPERVISOR_ACCESS="${VM_HYPERVISOR_ACCESS:-}"
export QA_VM_SSH_HOST="${QA_VM_SSH_HOST:-}"