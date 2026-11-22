#!/bin/sh
# Печатает текст тревоги. В служебный чат отправляет его только если
# OPS_CHAT_ID и TELEGRAM_BOT_TOKEN уже заданы в окружении (NT-R21).
# Значений в этом файле нет. Токен в аргументы curl не попадает (как в backup.sh).
set -eu

text=${1:?}
printf '%s\n' "${text}"

if [ -z "${OPS_CHAT_ID:-}" ] || [ -z "${TELEGRAM_BOT_TOKEN:-}" ]; then
  exit 0
fi

body=$(printf '{"chat_id":"%s","text":"%s"}' "${OPS_CHAT_ID}" "${text}")
curl -fsS -o /dev/null \
  --variable '%TELEGRAM_BOT_TOKEN' \
  -H "Content-Type: application/json" \
  -d "${body}" \
  --expand-url 'https://api.telegram.org/bot{{TELEGRAM_BOT_TOKEN}}/sendMessage' || true
