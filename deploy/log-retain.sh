#!/bin/sh
# Журналы сервисов старше 30 дней удаляются (OP-R18, D250).
# Каталог передаётся аргументом: путь на сервере в git не входит.
# Использование: log-retain.sh DIRECTORY
set -eu

dir=${1:?}
find "${dir}" -type f -mtime +30 -delete
