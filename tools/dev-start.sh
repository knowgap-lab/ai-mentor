#!/usr/bin/env bash
# Локальная база: штатный Docker и tools/dev-up.sh.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source "$root/tools/dev-common.sh"
dev_ensure_docker
bash tools/dev-up.sh
