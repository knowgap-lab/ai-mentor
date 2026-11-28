# Регистрация репетитора через Telegram (локально)

Токен бота и секрет вебхука **не хранятся в git**. Имена переменных: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET` (см. `deploy/secrets.md`).

## 1. Бот и секреты

1. В Telegram откройте [@BotFather](https://t.me/BotFather), создайте бота, скопируйте токен.
2. Придумайте длинную случайную строку для вебхука (например `openssl rand -hex 32`).
3. Задайте переменные в окружении (файл вне репозитория, права 600):

   ```bash
   export TELEGRAM_BOT_TOKEN='…'
   export TELEGRAM_WEBHOOK_SECRET='…'
   ```

   Значения не коммитьте и не вставляйте в чаты.

## 2. База и миграции

```bash
export DATABASE_URL='postgres://postgres@127.0.0.1:55432/ai_mentor?sslmode=disable'
cd core && go run ./cmd/migrate
```

## 3. Учётная запись и приглашение

```bash
export APP_ORIGIN='http://127.0.0.1:8080'
cd core && go run ./cmd/tutorctl create --tutor tutor-alexander
```

В stdout — одноразовый `invite_token` (72 часа, в базе только SHA-256). Сохраните его в менеджере паролей, не в git.

## 4. Сервер

```bash
export ENV=local
export AUTH_DEV_MODE=true
export DATABASE_URL='…'
export TELEGRAM_BOT_TOKEN='…'
export TELEGRAM_WEBHOOK_SECRET='…'
export APP_ORIGIN='http://127.0.0.1:8080'
cd core && go run ./cmd/server
```

Вход через Telegram идёт только через `POST /auth/telegram`: сервер проверяет подпись виджета (AU-R12, AU-R13). Обхода проверки нет.

## 5. Webhook (бот)

На публичном HTTPS-адресе, доступном Telegram:

```bash
curl -sS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=https://ВАШ_ДОМЕН/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

Локально без туннеля webhook необязателен для **первой привязки** репетитора: достаточно виджета Telegram Login на сайте с `invite_token` в теле `POST /auth/telegram`.

## 6. Первый вход репетитора

1. Откройте страницу с виджетом входа Telegram (домен должен совпадать с настройками бота у BotFather).
2. Войдите через виджет; в запрос добавьте поле `invite_token` из шага 3 (вместе с `id`, `auth_date`, `hash` от Telegram).
3. Сохраняется только числовой `telegram_id` (AU-R15, D239). Первый вход подтверждает устройство автоматически (AU-R31, D238).

Повторный вход — без `invite_token`, по уже привязанному `telegram_id`.

## Правила

- Приглашение: 32 байта, в базе SHA-256, 72 часа, один раз (AU-R9, AU-R10).
- Один Telegram — один пользователь (AU-R16).
- `POST /telegram/webhook` сверяет заголовок `X-Telegram-Bot-Api-Secret-Token` с `TELEGRAM_WEBHOOK_SECRET` (NT-R19).