#!/usr/bin/env bash
# Создание одноразовой чистой Ubuntu на стороне гипервизора (D321, §8).
# VM_HYPERVISOR_ACCESS — путь к исполняемому сценарию оператора (не содержимое в git).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -z "${VM_HYPERVISOR_ACCESS:-}" ]]; then
  echo "missing: VM_HYPERVISOR_ACCESS (владелец: добавить секрет GitHub Actions или путь к скрипту создания VM)"
  exit 1
fi

if [[ ! -f "$VM_HYPERVISOR_ACCESS" ]]; then
  echo "vm-hypervisor-create: VM_HYPERVISOR_ACCESS не файл: $VM_HYPERVISOR_ACCESS"
  exit 1
fi
if [[ ! -x "$VM_HYPERVISOR_ACCESS" ]]; then
  echo "vm-hypervisor-create: сценарий гипервизора не исполняемый: $VM_HYPERVISOR_ACCESS"
  exit 1
fi

echo "vm-hypervisor-create: invoking $VM_HYPERVISOR_ACCESS (RELEASE_SHA=${RELEASE_SHA:-unset})"
exec env RELEASE_SHA="${RELEASE_SHA:-}" ROOT="$ROOT" bash "$VM_HYPERVISOR_ACCESS"