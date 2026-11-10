# Секреты

Значений в этом файле нет. На сервере они лежат в файле вне git, права 600 (OP-R4). В compose и скриптах записаны только имена `${…}`.

| Имя | Правило |
|---|---|
| `DATABASE_URL` | строка соединения приложения и `pg_dump` |
| `POSTGRES_PASSWORD` | пароль роли базы в контейнере `db` |
| `TELEGRAM_BOT_TOKEN` | токен бота, читается при старте (AU-R35) |
| `TELEGRAM_WEBHOOK_SECRET` | секрет заголовка webhook (NT-R19) |
| `OPS_CHAT_ID` | служебный чат (NT-R21) |
| `STAGING_CHAT_ALLOWLIST` | куда staging вообще имеет право писать (NT-R22) |
| `CONTENT_API_TOKEN` | служебный токен загрузчика каталога (AU-R7) |
| `BACKUP_ENCRYPTION_KEY` | шифрование копий; копия ключа хранится и вне сервера (OP-R5) |
| `BACKUP_STORAGE_URL` | хранилище другого провайдера, без адреса в git (OP-R10) |
| `CATALOG_PUBLIC_KEY` | открытый ключ проверки артефакта на сервере (OP-R5) |

Ключ подписи каталога в этот файл и на сервер не кладётся: он остаётся в секретах CI (OP-R5). У staging свой бот и свои значения тех же имён, в отдельном файле (OP-R2, AU-R35).

`AUTH_DEV_MODE` — не секрет. В `compose.prod.yml` и `compose.staging.yml` он выключен. Включать его можно только в окружении `local` (AU-R36, OP-R3).

Остальные имена, без значений: `POSTGRES_USER`, `POSTGRES_DB`, `DB_PORT`, `IMAGE_TAG`, `CORE_IMAGE_REPOSITORY`, `WEB_IMAGE_REPOSITORY`, `CONTENT_IMAGE_REPOSITORY`, `APP_ENV`, `COOKIE_DOMAIN`, `SECRETS_DIR`, `BACKUP_WAL_DIR`, `BACKUP_WORKDIR`, `BACKUP_SHIP_BIN`, `BACKUP_FETCH_BIN`, `BACKUP_RETENTION_DAYS`, `RESTORE_PORT`, `CONSENT_REPLAY_BIN`, `JOURNAL_RECALC_BIN`.

`BACKUP_WAL_DIR` — каталог архива WAL на сервере. Текст `archive_command` в git не входит. `BACKUP_SHIP_BIN` и `BACKUP_FETCH_BIN` отгружают и забирают уже зашифрованные файлы по `BACKUP_STORAGE_URL`. Срок хранения копий — 30 дней (OP-R10): скрипт копий выставляет `BACKUP_RETENTION_DAYS` и не хранит адрес провайдера. `CONSENT_REPLAY_BIN` заново применяет журнал отзыва согласия (OP-R12). `JOURNAL_RECALC_BIN` пересчитывает производные данные из журнала (OP-R11).

## Замена

Для каждого секрета из таблицы:

1. Новое значение записать в файл на сервере (права 600) и в менеджер паролей. В git его нет.
2. Перезапустить только тот процесс, который читает это имя.
3. Убедиться, что старое значение больше не принимается.
4. Для токена бота: в момент замены недействительны входы, действующие сеансы не трогать (AU-R35).
5. Для ключа шифрования копий: одна копия остаётся вне сервера (OP-R5).
