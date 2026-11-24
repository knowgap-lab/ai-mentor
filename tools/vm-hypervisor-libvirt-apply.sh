#!/usr/bin/env bash
# Создание одноразовой чистой Ubuntu через Terraform libvirt (D321, D340).
set -euo pipefail
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
VM_STACK="$ROOT/infra/terraform/acceptance-vm"

if [[ -z "${RELEASE_SHA:-}" ]]; then
  RELEASE_SHA="$(git -C "$ROOT" rev-parse HEAD^{commit})"
fi
export RELEASE_SHA

if [[ -z "${TF_VAR_acceptance_ssh_public_key:-}" && -z "${ACCEPTANCE_SSH_PUBLIC_KEY:-}" ]]; then
  for candidate in "${HOME}/.ssh/id_ed25519.pub" "${HOME}/.ssh/id_rsa.pub"; do
    if [[ -f "$candidate" ]]; then
      export TF_VAR_acceptance_ssh_public_key="$(tr -d '\n' <"$candidate")"
      break
    fi
  done
fi
if [[ -n "${ACCEPTANCE_SSH_PUBLIC_KEY:-}" ]]; then
  export TF_VAR_acceptance_ssh_public_key="$ACCEPTANCE_SSH_PUBLIC_KEY"
fi
if [[ -z "${TF_VAR_acceptance_ssh_public_key:-}" ]]; then
  echo "vm-hypervisor-libvirt-apply: задайте TF_VAR_acceptance_ssh_public_key или ACCEPTANCE_SSH_PUBLIC_KEY" >&2
  exit 1
fi

export LIBVIRT_DEFAULT_URI="${LIBVIRT_DEFAULT_URI:-qemu:///system}"
export TF_VAR_libvirt_uri="$LIBVIRT_DEFAULT_URI"

echo "vm-hypervisor-libvirt-apply: stack=$VM_STACK uri=$LIBVIRT_DEFAULT_URI RELEASE_SHA=$RELEASE_SHA"
(
  cd "$VM_STACK"
  terraform init -input=false
  terraform apply -auto-approve -input=false
  terraform output -json acceptance_ipv4 2>/dev/null || true
)