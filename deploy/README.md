# Развёртывание

Здесь будут compose-файлы.

Образ Postgres — официальный `postgres:16`. Точный патч: `16.15` (тег `postgres:16.15` библиотеки `library/postgres` на Docker Hub, сверено 2026-11-09). Локально этого образа нет, есть только `postgres:17`, поэтому в этой сессии образ не скачивался.

`compose.prod.yml` и `compose.staging.yml` описывают `web`, `core`, `content`, `db` и разовый шаг миграции до `core` и `web`. У staging своя сеть. Тег образов приложения берётся из `IMAGE_TAG`, база — `postgres:16.15`. `backup.sh` и `restore-check.sh` шифруют копию ключом `BACKUP_ENCRYPTION_KEY` и отдают её в `BACKUP_STORAGE_URL`; значений секретов в git нет, имена — в `secrets.md`. В обоих compose `AUTH_DEV_MODE` выключен. Ночная проверка восстановления запускается по расписанию сервера, не из compose.
