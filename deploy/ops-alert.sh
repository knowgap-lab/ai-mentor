#!/bin/sh
# Тревога в служебный чат (OP-R16, NT-R21). Значений секретов нет.
# Использование: ops-alert.sh ПРИЧИНА
#   disk     диск > 80 %
#   catalog  неудачная загрузка каталога
#   recheck  неудачный пакет перепроверки
#   job      задание в failed
#   not-me   Это не я
set -eu

reason=${1:?}
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

case ${reason} in
  disk)
    text="диск > 80 %"
    ;;
  catalog)
    text="неудачная загрузка каталога"
    ;;
  recheck)
    text="неудачный пакет перепроверки"
    ;;
  job)
    text="задание в failed"
    ;;
  not-me)
    text="Это не я"
    ;;
  *)
    echo "неизвестная тревога" >&2
    exit 1
    ;;
esac

"${root}/alert-send.sh" "${text}"
