#!/usr/bin/env bash
# Локальная демонстрация выпуска 1 на отдельной синтетической базе.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/tools/dev-toolchain.sh"
cd "$root"

python="$root/.venv/bin/python"
if [[ ! -x "$python" ]]; then
  echo "Нет .venv. Подготовьте инструменты и зависимости по README.md." >&2
  exit 1
fi

docker_host="${DOCKER_HOST:-}"
if [[ -n "${DOCKER_CONTEXT:-}" || -z "$docker_host" ]]; then
  docker_host="$(docker context inspect "$(docker context show)" --format '{{.Endpoints.docker.Host}}')"
fi
if [[ "$docker_host" != unix://* ]]; then
  echo "Для демонстрации нужен локальный Unix-сокет Docker." >&2
  exit 1
fi
docker_cmd=(env -u DOCKER_CONTEXT docker --host "$docker_host")
"${docker_cmd[@]}" info --format '{{.ServerVersion}}' >/dev/null

sha="$(git rev-parse HEAD)"
source_state="рабочая копия без изменений"
if [[ -n "$(git status --porcelain)" ]]; then
  source_state="рабочая копия с изменениями"
fi

preview_dir="$(mktemp -d -t ai-mentor-preview.XXXXXXXX)"
container="${preview_dir##*/}"
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  "${docker_cmd[@]}" rm -f "$container" >/dev/null 2>&1 || true
  rm -rf -- "$preview_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

(
  cd web
  npm ci --ignore-scripts --no-audit --no-fund
  npm run build -- --outDir "$preview_dir/web" --emptyOutDir --logLevel warn
)
(cd core && go build -o "$preview_dir/server" ./cmd/server)

image="$(toolchain_value POSTGRES_IMAGE "$root/mise.toml")"
password="$("$python" -c 'import secrets; print(secrets.token_hex(24))')"
"${docker_cmd[@]}" run -d --rm --name "$container" \
  --tmpfs /var/lib/postgresql/data \
  -p 127.0.0.1::5432 \
  -e "POSTGRES_PASSWORD=$password" \
  -e POSTGRES_DB=ai_mentor_preview \
  "$image" >/dev/null

db_address="$("${docker_cmd[@]}" port "$container" 5432/tcp)"
if [[ ! "$db_address" =~ ^127\.0\.0\.1:[0-9]+$ ]]; then
  echo "Docker не назначил локальный порт PostgreSQL." >&2
  exit 1
fi
dsn="postgres://postgres:$password@$db_address/ai_mentor_preview?sslmode=disable"

deadline=$((SECONDS + 60))
until psql "$dsn" -XAtqc 'SELECT 1' >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "PostgreSQL не запустился." >&2
    "${docker_cmd[@]}" logs --tail 20 "$container" >&2
    exit 1
  fi
  sleep 0.3
done
(cd core && DATABASE_URL="$dsn" go run ./cmd/migrate)

if ! env -i PATH="$PATH" DATABASE_URL="$dsn" "$python" tools/dev_seed.py >"$preview_dir/seed.log" 2>&1; then
  sed "s/$password/[скрыто]/g" "$preview_dir/seed.log" >&2
  exit 1
fi

port="$("$python" -c 'import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
')"
origin="http://127.0.0.1:$port"
env -i ENV=local AUTH_DEV_MODE=true DATABASE_URL="$dsn" APP_ORIGIN="$origin" \
  ADDR="127.0.0.1:$port" WEB_DIST="$preview_dir/web" "$preview_dir/server" &
server_pid=$!

deadline=$((SECONDS + 30))
until kill -0 "$server_pid" 2>/dev/null &&
  [[ "$(curl --noproxy '*' --max-time 1 -fsS "$origin/healthz" 2>/dev/null)" == "ok" ]]; do
  if (( SECONDS >= deadline )) || ! kill -0 "$server_pid" 2>/dev/null; then
    echo "Сервер демонстрации не запустился." >&2
    exit 1
  fi
  sleep 0.2
done

printf '\nИИ-ментор · %s (%s)\nОткрыть: %s/tutor\nОстановка: Ctrl+C\n' "$sha" "$source_state" "$origin"
wait "$server_pid"
