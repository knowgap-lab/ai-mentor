#!/bin/sh
# Внешняя проверка страницы состояния: GET /status раз в минуту (OP-R17, D249).
# Домен в файл не входит. Тревога после 3 подряд неудач, один раз за серию.
# Использование: status-check.sh URL STATE_FILE
set -eu

url=${1:?}
state=${2:?}
url=${url%/}
case ${url} in
  */status) ;;
  *) url="${url}/status" ;;
esac

fails=0
if [ -f "${state}" ]; then
  fails=$(tr -d '[:space:]' < "${state}")
fi
case ${fails} in
  *[!0-9]*)
    echo "счётчик проверок повреждён" >&2
    exit 1
    ;;
esac
if [ -z "${fails}" ]; then
  fails=0
fi

ok=0
if curl -fsS -o /dev/null --max-time 10 "${url}"; then
  ok=1
fi
if [ "${ok}" -eq 1 ]; then
  printf '%s\n' 0 > "${state}"
  exit 0
fi

fails=$((fails + 1))
printf '%s\n' "${fails}" > "${state}"
if [ "${fails}" -eq 3 ]; then
  root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
  "${root}/alert-send.sh" "сервис недоступен"
fi
