#!/usr/bin/env bash
# Установка инструментов из свежего клона на Ubuntu.
# Версии — только mise.toml. Бинарники ставятся в /usr/local и ~/.config/mise,
# не в каталог репозитория. Системный python3 не заменяется.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source "$root/tools/dev-common.sh"
source "$root/tools/dev-toolchain.sh"

export DEBIAN_FRONTEND=noninteractive
APT_OPTS=(-y -qq -o Dpkg::Options::=--force-confold)
pins="$root/mise.toml"
export PATH="/usr/local/bin:${PATH}"

apt_install() {
  sudo apt-get update -qq
  sudo apt-get install "${APT_OPTS[@]}" "$@"
}

go_ver="$(toolchain_value go "$pins")"
node_ver="$(toolchain_value node "$pins")"
py_ver="$(toolchain_value python "$pins")"
uv_ver="$(toolchain_value uv "$pins")"
mise_ver="$(toolchain_value mise "$pins")"
py_apt="$(toolchain_value PYTHON_APT "$pins")"
postgres_image="$(toolchain_value POSTGRES_IMAGE "$pins")"
chrome_ver="$(toolchain_value CHROME_VERSION "$pins")"
chrome_sha="$(toolchain_value CHROME_SHA256 "$pins")"

go_mod="$(awk '/^go / { print $2; exit }' core/go.mod)"
if [[ "$go_ver" != "$go_mod" ]]; then
  echo "go ${go_ver} в mise.toml не совпадает с core/go.mod (${go_mod})" >&2
  exit 1
fi
py_file="$(tr -d '[:space:]' < .python-version)"
if [[ "$py_ver" != "$py_file" && "$py_ver" != "$py_file".* ]]; then
  echo "python ${py_ver} в mise.toml не продолжает .python-version (${py_file})" >&2
  exit 1
fi
if [[ "$py_apt" != "$py_ver"-* ]]; then
  echo "PYTHON_APT ${py_apt} не закрепляет python ${py_ver}" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1 || ! command -v psql >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1 || ! command -v cmp >/dev/null 2>&1; then
  apt_install ca-certificates curl xz-utils python3 postgresql-client diffutils
fi

if ! command -v docker >/dev/null 2>&1 || ! command -v fuse-overlayfs >/dev/null 2>&1; then
  missing=()
  if ! command -v docker >/dev/null 2>&1; then
    missing+=(docker.io)
  fi
  if ! command -v fuse-overlayfs >/dev/null 2>&1; then
    missing+=(fuse-overlayfs)
  fi
  apt_install "${missing[@]}"
fi

if ! python3.14 -c "import sys; raise SystemExit(0 if '.'.join(map(str, sys.version_info[:3])) == '${py_ver}' else 1)" >/dev/null 2>&1; then
  apt_install software-properties-common
  if ! grep -Rqs 'deadsnakes' /etc/apt/sources.list /etc/apt/sources.list.d 2>/dev/null; then
    sudo add-apt-repository -y ppa:deadsnakes/ppa
  fi
  apt_install "python3.14=${py_apt}" "python3.14-venv=${py_apt}" "python3.14-dev=${py_apt}"
fi

install_verified_tar() {
  local url="$1"
  local sum="$2"
  local archive
  archive="$(mktemp)"
  curl -fsSL -o "$archive" "$url"
  sha256_file "$archive" "$sum"
  printf '%s\n' "$archive"
}

if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${go_ver} "; then
  go_sum="$(published_go_sha256 "$go_ver")"
  go_tar="$(install_verified_tar "https://go.dev/dl/go${go_ver}.linux-amd64.tar.gz" "$go_sum")"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$go_tar"
  rm -f "$go_tar"
fi
sudo ln -sfn /usr/local/go/bin/go /usr/local/bin/go
sudo ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt

if ! /usr/local/bin/node -v 2>/dev/null | grep -qx "v${node_ver}"; then
  node_name="node-v${node_ver}-linux-x64.tar.xz"
  node_sums="$(mktemp)"
  curl -fsSL -o "$node_sums" "https://nodejs.org/dist/v${node_ver}/SHASUMS256.txt"
  node_sum="$(published_name_sha256 "$node_sums" "$node_name")"
  rm -f "$node_sums"
  node_tar="$(install_verified_tar "https://nodejs.org/dist/v${node_ver}/${node_name}" "$node_sum")"
  sudo tar -C /usr/local --strip-components=1 -xJf "$node_tar"
  rm -f "$node_tar"
fi

if ! /usr/local/bin/uv --version 2>/dev/null | grep -q "uv ${uv_ver}"; then
  uv_name="uv-x86_64-unknown-linux-gnu.tar.gz"
  uv_sums="$(mktemp)"
  curl -fsSL -o "$uv_sums" "https://github.com/astral-sh/uv/releases/download/${uv_ver}/${uv_name}.sha256"
  uv_sum="$(published_name_sha256 "$uv_sums" "$uv_name")"
  rm -f "$uv_sums"
  uv_tar="$(install_verified_tar "https://github.com/astral-sh/uv/releases/download/${uv_ver}/${uv_name}" "$uv_sum")"
  uv_dir="$(mktemp -d)"
  tar -C "$uv_dir" -xzf "$uv_tar"
  sudo install -m 0755 "$uv_dir"/*/uv /usr/local/bin/uv
  rm -rf "$uv_tar" "$uv_dir"
fi

if ! /usr/local/bin/mise --version 2>/dev/null | grep -q "${mise_ver}"; then
  mise_name="mise-v${mise_ver}-linux-x64.tar.gz"
  mise_sums="$(mktemp)"
  curl -fsSL -o "$mise_sums" "https://github.com/jdx/mise/releases/download/v${mise_ver}/SHASUMS256.txt"
  mise_sum="$(published_name_sha256 "$mise_sums" "$mise_name")"
  rm -f "$mise_sums"
  mise_tar="$(install_verified_tar "https://github.com/jdx/mise/releases/download/v${mise_ver}/${mise_name}" "$mise_sum")"
  mise_dir="$(mktemp -d)"
  tar -C "$mise_dir" -xzf "$mise_tar"
  sudo install -m 0755 "$mise_dir"/mise/bin/mise /usr/local/bin/mise
  rm -rf "$mise_tar" "$mise_dir"
fi

install -d "${HOME}/.config/mise"
cp "$pins" "${HOME}/.config/mise/config.toml"
if ! cmp -s "$pins" "${HOME}/.config/mise/config.toml"; then
  echo "глобальный config.toml mise не совпал с mise.toml" >&2
  exit 1
fi

chrome_now="$(dpkg-query -W -f '${Version}' google-chrome-stable 2>/dev/null || true)"
if [[ "$chrome_now" != "$chrome_ver" ]]; then
  apt_install gnupg
  key_tmp="$(mktemp)"
  curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --batch --dearmor >"$key_tmp"
  sudo install -m 0644 "$key_tmp" /usr/share/keyrings/google-linux.gpg
  rm -f "$key_tmp"
  echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-linux.gpg] https://dl.google.com/linux/chrome/deb/ stable main" | sudo tee /etc/apt/sources.list.d/google-chrome.list >/dev/null
  sudo apt-get update -qq
  deb_dir="$(mktemp -d)"
  (cd "$deb_dir" && apt-get download "google-chrome-stable=${chrome_ver}")
  deb="$(echo "$deb_dir"/google-chrome-stable_*.deb)"
  sha256_file "$deb" "$chrome_sha"
  sudo apt-get install "${APT_OPTS[@]}" "$deb"
  rm -rf "$deb_dir"
fi

if command -v docker >/dev/null 2>&1 && ! dev_in_docker_group; then
  sudo usermod -aG docker "$(id -un)"
fi

dev_ensure_docker

if [[ ! -x .venv/bin/python ]] || ! .venv/bin/python -c "import sys; raise SystemExit(0 if '.'.join(map(str, sys.version_info[:3])) == '${py_ver}' else 1)"; then
  rm -rf .venv
  python3.14 -m venv .venv
fi
uv pip sync --python .venv/bin/python tools/requirements.lock

(cd web && npm ci --ignore-scripts)
if [[ -f tools/e2e/package-lock.json ]]; then
  (cd tools/e2e && npm ci --ignore-scripts)
fi

(cd core && go mod download)
(cd core && go tool golangci-lint version)
(cd core && go tool govulncheck -version)

docker pull "$postgres_image"
echo "Установка готова. Системный python3: $(/usr/bin/python3 --version 2>/dev/null || echo не найден). Проект: $(.venv/bin/python --version)."
