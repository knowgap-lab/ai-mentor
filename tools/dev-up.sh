#!/usr/bin/env bash
# Локальная база: Postgres 16.15, миграции Up, синтетический сид.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
container="ai-mentor-dev"
image="postgres:16.15"
volume="ai-mentor-dev-data"
dsn="${DATABASE_URL:-postgres://postgres@127.0.0.1:5432/ai_mentor?sslmode=disable}"
python="$root/.venv/bin/python"

if [[ ! -x "$python" ]]; then
  echo "нет интерпретатора $python" >&2
  exit 1
fi

running="$(docker inspect -f '{{.State.Running}}' "$container" 2>/dev/null || true)"
if [[ "$running" != "true" ]]; then
  if docker container inspect "$container" >/dev/null 2>&1; then
    docker start "$container" >/dev/null
  else
    docker run -d \
      --name "$container" \
      -e POSTGRES_HOST_AUTH_METHOD=trust \
      -e POSTGRES_USER=postgres \
      -e POSTGRES_DB=ai_mentor \
      -p 127.0.0.1:5432:5432 \
      -v "${volume}:/var/lib/postgresql/data" \
      "$image" >/dev/null
  fi
fi

deadline=$((SECONDS + 60))
until psql "$dsn" -v ON_ERROR_STOP=1 -X -q -t -A -c "SELECT 1" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "база не готова: $dsn" >&2
    exit 1
  fi
  sleep 0.3
done

psql "$dsn" -v ON_ERROR_STOP=1 -X -q -c \
  "CREATE TABLE IF NOT EXISTS public.dev_schema_migration (filename text PRIMARY KEY);"

shopt -s nullglob
files=("$root"/migrations/*.sql)
shopt -u nullglob
if (( ${#files[@]} == 0 )); then
  echo "нет файлов в $root/migrations" >&2
  exit 1
fi

for file in "${files[@]}"; do
  name="$(basename "$file")"
  applied="$(psql "$dsn" -v ON_ERROR_STOP=1 -X -q -t -A \
    -c "SELECT 1 FROM public.dev_schema_migration WHERE filename = '${name}'")"
  if [[ "$applied" == "1" ]]; then
    continue
  fi
  tmp="$(mktemp)"
  awk '
    $0 == "-- +goose Up" { capture = 1; next }
    $0 == "-- +goose Down" { capture = 0; next }
    capture { print }
  ' "$file" >"$tmp"
  if [[ ! -s "$tmp" ]]; then
    echo "$name: пустая секция Up" >&2
    rm -f "$tmp"
    exit 1
  fi
  psql "$dsn" -v ON_ERROR_STOP=1 -X -q -f "$tmp"
  rm -f "$tmp"
  psql "$dsn" -v ON_ERROR_STOP=1 -X -q \
    -c "INSERT INTO public.dev_schema_migration (filename) VALUES ('${name}')"
done

cd "$root"
DATABASE_URL="$dsn" "$python" "$root/tools/dev_seed.py"
