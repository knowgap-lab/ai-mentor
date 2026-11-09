#!/usr/bin/env bash
# Проверки каркаса выпуска 1. Запуск из корня: bash tools/check-code.sh
set -euo pipefail

cd "$(dirname "$0")/.."
export PATH="$PWD/.venv/bin:$PATH"

mapfile -d '' go_files < <(find core -name '*.go' -print0)
if ((${#go_files[@]} > 0)); then
  unformatted="$(gofmt -l "${go_files[@]}")"
  if [[ -n "${unformatted}" ]]; then
    printf '%s\n' "${unformatted}"
    exit 1
  fi
fi

(cd core && go test ./...)
(cd core && go vet ./...)

if command -v golangci-lint >/dev/null 2>&1; then
  (cd core && golangci-lint run ./...)
else
  echo "golangci-lint не установлен, проверка пропущена"
fi

(cd web && npm test)
(cd web && npm run typecheck)
if node -e 'const s=require("./web/package.json").scripts||{}; process.exit(s.lint?0:1)'; then
  (cd web && npm run lint)
fi

if [[ -x .venv/bin/ruff ]]; then
  .venv/bin/ruff check
else
  echo "ruff не найден в .venv, проверка пропущена"
fi
