#!/usr/bin/env bash
# Копия prod Postgres → отдельный контейнер → сравнение синтетической строки (OP-R11).
# Значений секретов в git нет. Не шлёт в Telegram: ACCEPTANCE_BACKUP_DRILL=1 отключает alert.
set -euo pipefail

STACK="${STACK:-prod}"
STATE_ROOT="${STATE_ROOT:-/etc/ai-mentor}"
INSTALL_ROOT="${INSTALL_ROOT:-/opt/ai-mentor}"
SRC="${INSTALL_ROOT}/src"
MARKER_VALUE="acceptance-drill-$(date -u +%Y%m%dT%H%M%SZ)"
RESTORE_NAME=ai-mentor-acceptance-restore
RESTORE_PORT="${ACCEPTANCE_RESTORE_PORT:-55432}"
NET_NAME=ai-mentor-acceptance-restore-net
WORKDIR="${STATE_ROOT}/${STACK}/work"
DUMP="${WORKDIR}/acceptance-drill.dump"

compose() {
  docker compose \
    --project-directory "$SRC" \
    --env-file "${STATE_ROOT}/${STACK}/env" \
    --env-file "${STATE_ROOT}/${STACK}/secret.env" \
    -f "${SRC}/deploy/compose.prod.yml" \
    "$@"
}

cleanup() {
  docker rm -f "$RESTORE_NAME" >/dev/null 2>&1 || true
  docker network rm "$NET_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$WORKDIR"
umask 077

compose exec -T db psql -U ai_mentor -d ai_mentor -v ON_ERROR_STOP=1 <<-SQL
CREATE TABLE IF NOT EXISTS acceptance_backup_drill (
  marker text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO acceptance_backup_drill (marker) VALUES ('${MARKER_VALUE}')
ON CONFLICT (marker) DO NOTHING;
SQL

compose exec -T db pg_dump -U ai_mentor -Fc ai_mentor >"$DUMP"
if [[ ! -s "$DUMP" ]]; then
  echo "acceptance-backup-drill: dump empty" >&2
  exit 1
fi
echo "acceptance-backup-drill: backup_file=${DUMP}"

docker network create "$NET_NAME" >/dev/null
docker rm -f "$RESTORE_NAME" >/dev/null 2>&1 || true
docker run -d --name "$RESTORE_NAME" \
  --network "$NET_NAME" \
  -e POSTGRES_USER=ai_mentor \
  -e POSTGRES_PASSWORD=acceptance-drill-local \
  -e POSTGRES_DB=ai_mentor_restore \
  postgres:16.15 >/dev/null

ready=0
for _ in $(seq 1 30); do
  if docker exec "$RESTORE_NAME" pg_isready -U ai_mentor >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" -ne 1 ]]; then
  echo "acceptance-backup-drill: restore db not ready" >&2
  exit 1
fi

docker exec -i "$RESTORE_NAME" pg_restore -U ai_mentor -d ai_mentor_restore --no-owner --no-privileges <"$DUMP"
count="$(
  docker exec "$RESTORE_NAME" psql -U ai_mentor -d ai_mentor_restore -Atqc \
    "SELECT COUNT(*) FROM acceptance_backup_drill WHERE marker='${MARKER_VALUE}'"
)"
source_count="$(
  compose exec -T db psql -U ai_mentor -d ai_mentor -Atqc \
    "SELECT COUNT(*) FROM acceptance_backup_drill WHERE marker='${MARKER_VALUE}'"
)"
if [[ "$count" != "1" || "$source_count" != "1" ]]; then
  echo "acceptance-backup-drill: records differ (source=$source_count restore=$count)" >&2
  exit 1
fi
echo "acceptance-backup-drill: records_compared marker=${MARKER_VALUE}"