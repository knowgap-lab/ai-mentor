#!/usr/bin/env bash
# Проверки CI и Playwright smoke. Python берётся из .venv.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source "$root/tools/dev-common.sh"
dev_ensure_docker

if [[ ! -x .venv/bin/python ]]; then
  echo "нет .venv: сначала bash tools/dev-install.sh" >&2
  exit 1
fi
export PATH="$PWD/.venv/bin:$PATH"

echo "== tools/check-code.sh =="
bash tools/check-code.sh
echo "== tools/check.sh =="
bash tools/check.sh
if [[ -f tools/e2e/package.json ]]; then
  echo "== Playwright smoke =="
  (cd tools/e2e && npm test)
fi
