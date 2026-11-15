#!/bin/sh
# Одна внешняя проверка страницы состояния (OP-R17, D249).
# Расписание сервера запускает скрипт раз в минуту. Домен в файл не входит.
# Использование: status-check.sh URL STATE_FILE
# Тревога «сервис недоступен» — на третьей неудаче подряд, один раз за серию.
set -eu

url=${1:?}
state=${2:?}

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
  root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
  "${root}/alert-send.sh" "сервис недоступен"
fi
