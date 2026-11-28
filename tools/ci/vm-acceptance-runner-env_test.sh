#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_SH="$ROOT/tools/ci/vm-acceptance-runner-env.sh"

stub_bin="$(mktemp -d)"
trap 'rm -rf "$stub_bin"' EXIT

write_stub_virsh() {
  local mode="$1"
  cat >"$stub_bin/virsh" <<'STUB'
#!/usr/bin/env bash
if [[ "${VIRSH_STUB_MODE:-fail}" == ok ]]; then
  if [[ "$*" == *" uri" ]]; then
    echo "qemu:///system"
    exit 0
  fi
fi
exit 1
STUB
  chmod +x "$stub_bin/virsh"
  export VIRSH_STUB_MODE="$mode"
}

source_runner_env() {
  local hypervisor="${1:-}"
  local qa_host="${2:-}"
  (
    export PATH="$stub_bin:$PATH"
    unset GITHUB_ENV
    if [[ -n "$hypervisor" ]]; then
      export VM_HYPERVISOR_ACCESS="$hypervisor"
    else
      unset VM_HYPERVISOR_ACCESS
    fi
    if [[ -n "$qa_host" ]]; then
      export QA_VM_SSH_HOST="$qa_host"
    else
      unset QA_VM_SSH_HOST
    fi
    # shellcheck disable=SC1090
    source "$ENV_SH"
    printf '%s\n' "${VM_HYPERVISOR_ACCESS:-}" "${QA_VM_SSH_HOST:-}"
  )
}

FORBIDDEN_TEST_IP=10.254.0.99

# Denylist: forbidden QA target is dropped; without libvirt no placeholder replaces it.
write_stub_virsh fail
export QA_VM_SSH_FORBIDDEN_HOSTS="$FORBIDDEN_TEST_IP"
out="$(source_runner_env "" "ubuntu@${FORBIDDEN_TEST_IP}")"
if [[ -n "$(sed -n '2p' <<<"$out")" ]]; then
  echo "denylist: expected QA_VM_SSH_HOST cleared after forbidden host" >&2
  exit 1
fi

# virsh failure: executable VM_HYPERVISOR_ACCESS must not imply guest path via fake QA host.
out="$(source_runner_env "/bin/true" "")"
vm_out="$(sed -n '1p' <<<"$out")"
qa_out="$(sed -n '2p' <<<"$out")"
if [[ "$vm_out" != /bin/true ]]; then
  echo "virsh-fail: expected VM_HYPERVISOR_ACCESS preserved" >&2
  exit 1
fi
if [[ -n "$qa_out" ]]; then
  echo "virsh-fail: must not set QA_VM_SSH_HOST placeholder when libvirt unusable" >&2
  exit 1
fi

# libvirt ok: repo helper and placeholder.
write_stub_virsh ok
unset QA_VM_SSH_FORBIDDEN_HOSTS
out="$(source_runner_env "" "")"
if [[ "$(sed -n '1p' <<<"$out")" != "$ROOT/tools/vm-hypervisor-libvirt-apply.sh" ]]; then
  echo "libvirt-ok: expected in-repo VM_HYPERVISOR_ACCESS" >&2
  exit 1
fi
if [[ "$(sed -n '2p' <<<"$out")" != ubuntu@acceptance-pending ]]; then
  echo "libvirt-ok: expected acceptance-pending placeholder" >&2
  exit 1
fi

# Denylist + libvirt ok: forbidden cleared, placeholder still set for create.
export QA_VM_SSH_FORBIDDEN_HOSTS="$FORBIDDEN_TEST_IP"
out="$(source_runner_env "" "ubuntu@${FORBIDDEN_TEST_IP}")"
if [[ "$(sed -n '2p' <<<"$out")" != ubuntu@acceptance-pending ]]; then
  echo "denylist+libvirt: expected placeholder after forbidden host dropped" >&2
  exit 1
fi

echo "vm-acceptance-runner-env_test: exit=0"