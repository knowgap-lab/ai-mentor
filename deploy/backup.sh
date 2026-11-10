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
  # Токен бота curl берёт из окружения: в argv только имя переменной.
  curl -fsS -o /dev/null \
    --variable '%TELEGRAM_BOT_TOKEN' \
    -H "Content-Type: application/json" \
    -d "${body}" \
    --expand-url 'https://api.telegram.org/bot{{TELEGRAM_BOT_TOKEN}}/sendMessage' || true
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

# pg_dump читает PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE, PGSSLMODE.
# URI в PGDATABASE он не разбирает, поэтому параметры забираются из DATABASE_URL здесь.
urldecode() {
  printf '%s' "$1" | awk '
    BEGIN { hex = "0123456789abcdef" }
    {
      s = $0
      out = ""
      i = 1
      n = length(s)
      while (i <= n) {
        c = substr(s, i, 1)
        if (c == "%" && i + 2 <= n) {
          h = tolower(substr(s, i + 1, 2))
          h1 = index(hex, substr(h, 1, 1))
          h2 = index(hex, substr(h, 2, 1))
          if (h1 > 0 && h2 > 0) {
            out = out sprintf("%c", (h1 - 1) * 16 + (h2 - 1))
            i += 3
            continue
          }
        }
        out = out c
        i++
      }
      printf "%s", out
    }
  '
}

pg_url=${DATABASE_URL}
case ${pg_url} in
  postgres://*|postgresql://*) ;;
  *)
    echo "DATABASE_URL: нужен URI postgres://" >&2
    alert_and_exit "копия не создана"
    ;;
esac
pg_url=${pg_url#postgres://}
pg_url=${pg_url#postgresql://}
pg_query=
case ${pg_url} in
  *\?*)
    pg_query=${pg_url#*\?}
    pg_url=${pg_url%%\?*}
    ;;
esac
case ${pg_url} in
  */*) ;;
  *)
    echo "DATABASE_URL: нет имени базы" >&2
    alert_and_exit "копия не создана"
    ;;
esac
pg_database=$(urldecode "${pg_url#*/}")
pg_authority=${pg_url%%/*}
pg_userinfo=
pg_hostport=${pg_authority}
case ${pg_authority} in
  *@*)
    pg_userinfo=${pg_authority%@*}
    pg_hostport=${pg_authority##*@}
    ;;
esac
pg_user=$(urldecode "${pg_userinfo%%:*}")
pg_password=
case ${pg_userinfo} in
  *:*) pg_password=$(urldecode "${pg_userinfo#*:}") ;;
esac
pg_host=${pg_hostport}
pg_port=
case ${pg_hostport} in
  \[*\])
    pg_host=${pg_hostport#\[}
    pg_host=${pg_host%\]}
    ;;
  \[*:*)
    pg_host=${pg_hostport#\[}
    pg_host=${pg_host%%\]*}
    pg_port=${pg_hostport##*:}
    ;;
  *:*)
    pg_host=${pg_hostport%%:*}
    pg_port=${pg_hostport##*:}
    ;;
esac
pg_host=$(urldecode "${pg_host}")
pg_sslmode=
pg_rest=${pg_query}
while [ -n "${pg_rest}" ]; do
  pg_pair=${pg_rest%%&*}
  case ${pg_rest} in
    *\&*) pg_rest=${pg_rest#*&} ;;
    *) pg_rest= ;;
  esac
  case ${pg_pair} in
    sslmode=*) pg_sslmode=${pg_pair#sslmode=} ;;
  esac
done
if [ -z "${pg_database}" ]; then
  echo "DATABASE_URL: нет имени базы" >&2
  alert_and_exit "копия не создана"
fi
if [ -n "${pg_host}" ]; then export PGHOST="${pg_host}"; fi
if [ -n "${pg_port}" ]; then export PGPORT="${pg_port}"; fi
if [ -n "${pg_user}" ]; then export PGUSER="${pg_user}"; fi
if [ -n "${pg_password}" ]; then export PGPASSWORD="${pg_password}"; fi
if [ -n "${pg_sslmode}" ]; then export PGSSLMODE="${pg_sslmode}"; fi
export PGDATABASE="${pg_database}"
pg_dump --format=custom --file="${plain}" --no-password

tar -C "${BACKUP_WAL_DIR}" -cf "${wal_plain}" .

drop_plain() {
  rm -f "${BACKUP_WORKDIR}"/full-*.dump "${BACKUP_WORKDIR}"/wal-*.tar
}
if ! openssl enc -aes-256-cbc -pbkdf2 -salt \
  -pass env:BACKUP_ENCRYPTION_KEY \
  -in "${plain}" -out "${cipher}"; then
  drop_plain
  alert_and_exit "копия не создана"
fi
if ! openssl enc -aes-256-cbc -pbkdf2 -salt \
  -pass env:BACKUP_ENCRYPTION_KEY \
  -in "${wal_plain}" -out "${wal_cipher}"; then
  drop_plain
  alert_and_exit "копия не создана"
fi
rm -f "${plain}" "${wal_plain}"

export BACKUP_RETENTION_DAYS=30
"${BACKUP_SHIP_BIN}" "${cipher}" "${dest_root}/full/${stamp}.dump.enc"
"${BACKUP_SHIP_BIN}" "${wal_cipher}" "${dest_root}/wal/${stamp}.tar.enc"
rm -f "${cipher}" "${wal_cipher}"

find "${BACKUP_WORKDIR}" -type f -name 'full-*.dump.enc' -mtime +30 -delete
find "${BACKUP_WORKDIR}" -type f -name 'wal-*.tar.enc' -mtime +30 -delete
