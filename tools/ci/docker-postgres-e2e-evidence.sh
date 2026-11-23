#!/usr/bin/env bash
# Docker/Postgres smoke: те же команды, что tools/e2e/stack.ts и job e2e (docker pull postgres:16.15).
set -euo pipefail

name="release1-ci-pg-smoke-$$"
vol="${name}-data"

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker volume rm -f "$vol" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker pull postgres:16.15

docker run -d --rm --pull never --name "$name" \
  -v "${vol}:/var/lib/postgresql/data" \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  -e POSTGRES_DB=ai_mentor_test \
  -p 127.0.0.1::5432 \
  postgres:16.15

deadline=$((SECONDS + 60))
last=""
while (( SECONDS < deadline )); do
  if docker exec "$name" pg_isready -U postgres -d ai_mentor_test >/dev/null 2>&1; then
    echo "postgres:16.15 ready (tools/e2e/stack.ts, release1-ci job e2e)"
    exit 0
  fi
  sleep 0.3
done

echo "postgres not ready in 60s"
exit 1