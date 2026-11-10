#!/bin/sh
# Ночная проверка восстановления в одноразовом контейнере (OP-R11).
# После восстановления заново применяется журнал отзыва согласия (OP-R12),
# затем пересчитываются производные данные из журнала.
# Стек окружения этот скрипт не поднимает. Значений секретов здесь нет.
set -eu
umask 077

: "${BACKUP_ENCRYPTION_KEY:?}"
: "${BACKUP_STORAGE_URL:?}"
: "${BACKUP_WORKDIR:?}"
: "${BACKUP_FETCH_BIN:?}"
: "${POSTGRES_USER:?}"
: "${POSTGRES_PASSWORD:?}"
: "${POSTGRES_DB:?}"
: "${RESTORE_PORT:?}"
: "${CONSENT_REPLAY_BIN:?}"
: "${JOURNAL_RECALC_BIN:?}"
: "${TELEGRAM_BOT_TOKEN:?}"
: "${OPS_CHAT_ID:?}"

alert() {
  body=$(printf '{"chat_id":"%s","text":"%s"}' "${OPS_CHAT_ID}" "$1")
  curl -fsS -o /dev/null \
    -H "Content-Type: application/json" \
    -d "${body}" \
    "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" || true
}

alert_and_exit() {
  trap - ERR
  alert "$1" || true
  exit 1
}

cleanup() {
  docker rm -f ai-mentor-restore-check >/dev/null 2>&1 || true
  docker network rm ai-mentor-restore-check >/dev/null 2>&1 || true
  if [ -n "${workdir:-}" ]; then
    rm -rf "${workdir}"
  fi
  unset PGPASSWORD RESTORE_DATABASE_URL || true
}

trap 'alert_and_exit "проверка восстановления не прошла"' ERR

mkdir -p "${BACKUP_WORKDIR}"
workdir=$(mktemp -d "${BACKUP_WORKDIR}/restore.XXXXXX")
trap cleanup EXIT

cipher="${workdir}/full.dump.enc"
plain="${workdir}/full.dump"
dest_root=${BACKUP_STORAGE_URL%/}
"${BACKUP_FETCH_BIN}" "${dest_root}/full/" "${cipher}"

openssl enc -d -aes-256-cbc -pbkdf2 \
  -pass env:BACKUP_ENCRYPTION_KEY \
  -in "${cipher}" -out "${plain}"
rm -f "${cipher}"

docker rm -f ai-mentor-restore-check >/dev/null 2>&1 || true
docker network rm ai-mentor-restore-check >/dev/null 2>&1 || true
docker network create ai-mentor-restore-check >/dev/null
docker run -d \
  --name ai-mentor-restore-check \
  --network ai-mentor-restore-check \
  -p "127.0.0.1:${RESTORE_PORT}:5432" \
  -e POSTGRES_USER \
  -e POSTGRES_PASSWORD \
  -e POSTGRES_DB \
  -v "${workdir}/pgdata:/var/lib/postgresql/data" \
  postgres:16.15

ready=0
i=0
while [ "${i}" -lt 30 ]; do
  if docker exec ai-mentor-restore-check \
    pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1
  then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 1
done
if [ "${ready}" -ne 1 ]; then
  echo "одноразовая база не поднялась" >&2
  alert_and_exit "проверка восстановления не прошла"
fi

export PGPASSWORD="${POSTGRES_PASSWORD}"
export AUTH_DEV_MODE=false
export RESTORE_DATABASE_URL="postgresql://${POSTGRES_USER}@127.0.0.1:${RESTORE_PORT}/${POSTGRES_DB}"

pg_restore --dbname="${RESTORE_DATABASE_URL}" --no-owner --no-privileges \
  --exit-on-error "${plain}"
rm -f "${plain}"

"${CONSENT_REPLAY_BIN}"
"${JOURNAL_RECALC_BIN}"
