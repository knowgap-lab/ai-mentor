#!/bin/sh
# Диск > 80 % — одна тревога, пока порог превышен (OP-R16).
# Повтор той же тревоги не печатается. На 80 % и ниже защёлка снимается.
# Использование: disk-check.sh PERCENT STATE_FILE
# PERCENT — целое число процентов. Расписание сервера передаёт его из df.
set -eu

percent=${1:?}
state=${2:?}
while [ "${percent#0}" != "${percent}" ]; do
  percent=${percent#0}
done
if [ -z "${percent}" ]; then
  percent=0
fi
case ${percent} in
  *[!0-9]*)
    echo "процент не число" >&2
    exit 1
    ;;
esac

open=0
if [ -f "${state}" ]; then
  open=$(tr -d '[:space:]' < "${state}")
fi
case ${open} in
  0 | 1) ;;
  *)
    echo "состояние диска повреждено" >&2
    exit 1
    ;;
esac

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if [ "${percent}" -le 80 ]; then
  printf '%s\n' 0 > "${state}"
  exit 0
fi
if [ "${open}" -eq 1 ]; then
  exit 0
fi
printf '%s\n' 1 > "${state}"
"${root}/alert-send.sh" "диск > 80 %"
