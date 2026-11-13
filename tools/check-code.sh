#!/usr/bin/env bash
# Проверки каркаса выпуска 1. Запуск из корня: bash tools/check-code.sh
set -euo pipefail

cd "$(dirname "$0")/.."

# Общий .venv лежит в каталоге интеграции; worktree своего не создаёт.
venv_root="$PWD"
while [[ ! -x "$venv_root/.venv/bin/ruff" ]]; do
  parent="$(dirname "$venv_root")"
  if [[ "$parent" == "$venv_root" ]]; then
    echo "ruff не найден в .venv: проверка обязательна" >&2
    exit 1
  fi
  venv_root="$parent"
done
export PATH="$venv_root/.venv/bin:$PATH"

mapfile -d '' go_files < <(find core -name '*.go' -print0)
if ((${#go_files[@]} > 0)); then
  unformatted="$(gofmt -l "${go_files[@]}")"
  if [[ -n "${unformatted}" ]]; then
    printf '%s\n' "${unformatted}"
    exit 1
  fi
fi

(cd core && go test ./...)

# -race только у пакетов, чьи тесты не запускают внешний процесс
# (exec.Command, docker, Postgres). Остальные остаются в go test ./... выше.
race_packages=(
  ./cmd/migrate
  ./cmd/server
  ./internal/answerrules
)
(cd core && go test -race "${race_packages[@]}")

(cd core && go vet ./...)

if ! (cd core && go tool -n golangci-lint >/dev/null 2>&1); then
  echo "golangci-lint не установлен: проверка обязательна" >&2
  exit 1
fi
(cd core && go tool golangci-lint run ./...)

if ! (cd core && go tool -n govulncheck >/dev/null 2>&1); then
  echo "govulncheck не установлен: проверка обязательна" >&2
  exit 1
fi
(cd core && go tool govulncheck ./...)

python3 tools/check_design.py
(cd web && npm test)
(cd web && npm run typecheck)
if node -e 'const s=require("./web/package.json").scripts||{}; process.exit(s.lint?0:1)'; then
  (cd web && npm run lint)
fi
# Ненулевой код только при high или critical. moderate и low проверку не валят.
(cd web && npm audit --omit=dev --audit-level=high)

ruff check
ruff format --check
