#!/bin/sh
# Ежедневная зашифрованная полная копия и архив WAL (OP-R10).
# Скрипт не запускает сервисы. Значений секретов здесь нет.
# BACKUP_SHIP_BIN получает локальный файл и адрес из BACKUP_STORAGE_URL.
# Перед вызовом экспортируется BACKUP_RETENTION_DAYS=30: объекты старше
# этого срока удаляет уже сама программа отгрузки.
set -eu
umask 077

: "${DATABASE_URL:?}"
: "${BACKUP_ENCRYPTION_KEY:?}"
: "${BACKUP_STORAGE_URL:?}"
: "${BACKUP_WORKDIR:?}"
: "${BACKUP_WAL_DIR:?}"
: "${BACKUP_SHIP_BIN:?}"
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

trap 'alert_and_exit "копия не создана"' ERR

if [ ! -d "${BACKUP_WAL_DIR}" ]; then
  echo "каталог WAL недоступен" >&2
  alert_and_exit "копия не создана"
fi
if [ -z "$(find "${BACKUP_WAL_DIR}" -type f -print -quit)" ]; then
  echo "в каталоге WAL нет файлов" >&2
  alert_and_exit "копия не создана"
fi

mkdir -p "${BACKUP_WORKDIR}"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
plain="${BACKUP_WORKDIR}/full-${stamp}.dump"
cipher="${plain}.enc"
wal_plain="${BACKUP_WORKDIR}/wal-${stamp}.tar"
wal_cipher="${wal_plain}.enc"
dest_root=${BACKUP_STORAGE_URL%/}

pg_dump --dbname="${DATABASE_URL}" --format=custom --file="${plain}" --no-password

tar -C "${BACKUP_WAL_DIR}" -cf "${wal_plain}" .

openssl enc -aes-256-cbc -pbkdf2 -salt \
  -pass env:BACKUP_ENCRYPTION_KEY \
  -in "${plain}" -out "${cipher}"
openssl enc -aes-256-cbc -pbkdf2 -salt \
  -pass env:BACKUP_ENCRYPTION_KEY \
  -in "${wal_plain}" -out "${wal_cipher}"
rm -f "${plain}" "${wal_plain}"

export BACKUP_RETENTION_DAYS=30
"${BACKUP_SHIP_BIN}" "${cipher}" "${dest_root}/full/${stamp}.dump.enc"
"${BACKUP_SHIP_BIN}" "${wal_cipher}" "${dest_root}/wal/${stamp}.tar.enc"
rm -f "${cipher}" "${wal_cipher}"

find "${BACKUP_WORKDIR}" -type f -name 'full-*.dump.enc' -mtime +30 -delete
find "${BACKUP_WORKDIR}" -type f -name 'wal-*.tar.enc' -mtime +30 -delete
