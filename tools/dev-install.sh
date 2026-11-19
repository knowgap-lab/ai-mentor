#!/usr/bin/env bash
# Установка инструментов из свежего клона на Ubuntu с Docker.
# Системный python3 не заменяется. Python 3.14 живёт в .venv.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source "$root/tools/dev-common.sh"

export DEBIAN_FRONTEND=noninteractive
APT_OPTS=(-y -qq -o Dpkg::Options::=--force-confold)

apt_install() {
  sudo apt-get update -qq
  sudo apt-get install "${APT_OPTS[@]}" "$@"
}

if ! command -v curl >/dev/null 2>&1 || ! command -v psql >/dev/null 2>&1; then
  apt_install ca-certificates curl xz-utils postgresql-client
fi

if ! command -v docker >/dev/null 2>&1; then
  apt_install docker.io
fi

py_want="$(tr -d '[:space:]' < .python-version)"
if ! command -v "python${py_want}" >/dev/null 2>&1; then
  apt_install software-properties-common
  if ! grep -Rqs 'deadsnakes' /etc/apt/sources.list /etc/apt/sources.list.d 2>/dev/null; then
    sudo add-apt-repository -y ppa:deadsnakes/ppa
  fi
  apt_install "python${py_want}" "python${py_want}-venv" "python${py_want}-dev"
fi

go_want="$(awk '/^go / { print $2; exit }' core/go.mod)"
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${go_want} "; then
  tmp="$(mktemp)"
  curl -fsSL -o "$tmp" "https://go.dev/dl/go${go_want}.linux-amd64.tar.gz"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$tmp"
  rm -f "$tmp"
fi
sudo ln -sfn /usr/local/go/bin/go /usr/local/bin/go
sudo ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt

if ! command -v node >/dev/null 2>&1 || [[ "$(node -p 'process.versions.node.split(".")[0]')" != 22 ]]; then
  node_version="22.23.4"
  tmp="$(mktemp)"
  curl -fsSL -o "$tmp" "https://nodejs.org/dist/v${node_version}/node-v${node_version}-linux-x64.tar.xz"
  sudo tar -C /usr/local --strip-components=1 -xJf "$tmp"
  rm -f "$tmp"
fi

if ! command -v google-chrome >/dev/null 2>&1 && ! command -v google-chrome-stable >/dev/null 2>&1; then
  apt_install gnupg
  curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | sudo gpg --dearmor -o /usr/share/keyrings/google-linux.gpg
  echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-linux.gpg] https://dl.google.com/linux/chrome/deb/ stable main" | sudo tee /etc/apt/sources.list.d/google-chrome.list >/dev/null
  apt_install google-chrome-stable
fi

if command -v docker >/dev/null 2>&1 && ! dev_in_docker_group; then
  sudo usermod -aG docker "$(id -un)"
fi

dev_ensure_docker

py="python${py_want}"
if [[ ! -x .venv/bin/python ]] || ! .venv/bin/python -c "import sys; raise SystemExit(0 if sys.version.startswith('${py_want}.') or '.'.join(map(str, sys.version_info[:2])) == '${py_want}' else 1)"; then
  rm -rf .venv
  "$py" -m venv .venv
fi
.venv/bin/pip install -r tools/requirements.txt

(cd web && npm ci --ignore-scripts)
if [[ -f tools/e2e/package-lock.json ]]; then
  (cd tools/e2e && npm ci --ignore-scripts)
fi

(cd core && go mod download)
(cd core && go tool golangci-lint version)
(cd core && go tool govulncheck -version)

docker pull postgres:16.15
echo "Установка готова. Системный python3: $(/usr/bin/python3 --version 2>/dev/null || echo не найден). Проект: $(.venv/bin/python --version)."
