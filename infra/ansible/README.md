# Ansible выпуска 1

Плейбук ставит выпуск 1 на уже созданную чистую Ubuntu по SSH. SHA, репозиторий исходников и репозитории образов читаются из `../terraform/release.json`. Другого SHA в плейбуке нет.

На гостя клонируется этот SHA. `IMAGE_TAG` у prod и staging равен ему. Тег `latest` не используется (OP-R9). Файлы `deploy/compose*.yml` плейбук не правит.

Повторный запуск рассчитан на то, чтобы не менять уже совпавшее: пакеты закреплены версией, каталоги и юниты шаблоном, `docker compose up` без `--force-recreate`, загрузчик каталога в обычный запуск не входит. Таймеры копий остаются выключенными, пока нет `backup.env`.

Это не приёмка VM. Запуск на выделенной Ubuntu здесь не выполнялся.

## Проверка синтаксиса

```bash
cd infra/ansible
ansible-playbook --syntax-check site.yml
```

`inventory/example.yml` — пример для этой проверки. Адрес `192.0.2.10` и домены `prod.example`, `staging.example` не являются живым гостем.

## Запуск на госте

Инвентарь `inventory/guest.yml` в git не входит. В нём нужны `ansible_host`, `ansible_user` (не root), `cookie_domain_prod` и `cookie_domain_staging`. Домены должны быть разными (OP-R2). Пока домены не выбраны, плейбук не запускать на чужой машине.

```bash
ansible-playbook -i inventory/guest.yml site.yml
```

Перед выключением пароля плейбук проверяет, что у пользователя Ansible уже есть `authorized_keys`. Открываются только SSH, 80 и 443 (OP-R19).

## Секреты

Значений в git нет. Пока на госте нет файлов ниже, плейбук останавливается до `docker compose` и ничего в этих файлах не записывает:

- `/etc/ai-mentor/prod/secret.env` и `/etc/ai-mentor/staging/secret.env`, права 600
- имена prod: `DATABASE_URL`, `POSTGRES_PASSWORD`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `OPS_CHAT_ID`, `CONTENT_API_TOKEN`, `CATALOG_PUBLIC_KEY`
- у staging дополнительно `STAGING_CHAT_ALLOWLIST`
- файлы Compose в `secrets/`: `database_url`, `postgres_password`, `telegram_bot_token`, `telegram_webhook_secret`, `content_api_token`, `catalog_public_key`

Их ставит отдельная задача секретов (D325), не этот плейбук.

Несекретный `/etc/ai-mentor/<окружение>/env` плейбук пишет сам: там SHA, репозитории образов, `POSTGRES_USER`, `POSTGRES_DB`, порт базы и `COOKIE_DOMAIN`.

## Что повторный запуск может показать изменением

- Вход в GHCR, если на госте уже лежит `/etc/ai-mentor/ghcr_token`, помечен как без изменения. `docker login` может перезаписать `/root/.docker/config.json`. Токен в вывод не попадает. Без файла вход пропускается.
- `docker compose pull` меняет состояние только если скачан новый слой. Повторная загрузка уже лежащего образа изменением не считается.
- `docker compose up` меняет состояние, только если контейнер создан, запущен, пересоздан или сирота удалён.
- Индекс apt обновляется, когда источник Docker новый или закреплённого пакета ещё нет.

Каталог (`--profile catalog`) в этот запуск не входит, чтобы повтор не загружал данные снова (OP-R8). Команда ниже не является частью converge и здесь не проверялась:

```bash
docker compose --project-directory /opt/ai-mentor/src \
  --env-file /etc/ai-mentor/staging/env \
  --env-file /etc/ai-mentor/staging/secret.env \
  -f /opt/ai-mentor/src/deploy/compose.staging.yml \
  --profile catalog run --rm content
```

## Очередь до живой Ubuntu

1. Гипервизор не выбран. `terraform apply -var=create_vm=true` не выполнять: план отвергается. Последствие: VM из Terraform не появляется. Доступ SSH к гостю не даёт права создавать VM.
2. Нужна отдельная чистая Ubuntu и SSH-ключ пользователя с sudo. Без этого плейбук не к чему применять. Существующую машину и данные не пересоздавать.
3. Секреты по списку выше. Без них стек не поднимается.
4. Образы `ghcr.io/knowgap-lab/ai-mentor-core`, `ai-mentor-web` и `ai-mentor-content` с тегом SHA из `release.json` этим пакетом не публикуются. Запрос манифеста core на этот SHA вернул 401. Пока образов нет, `docker compose pull` завершится ошибкой. Workflow сборки не менялся.
5. Обратный прокси не выбран (D304, напарник). Порты 80 и 443 открываются, слушателя плейбук не добавляет. Порт внутри образа `web` в репозитории не задан. `core` слушает `:8080`, если `ADDR` пуст.
6. `backup.env` плейбук не создаёт. Таймеры копий и ночной проверки выключены. Файл не писать, пока нет архива WAL и программ `BACKUP_SHIP_BIN`, `BACKUP_FETCH_BIN`, `CONSENT_REPLAY_BIN`, `JOURNAL_RECALC_BIN`: пустой каталог WAL даёт тревогу.
7. Наблюдаемость D313 и внешняя проверка `/status` (OP-R17) в этот пакет не входят.

Пакеты Docker сверены 2026-11-19 с репозиторием Docker для Ubuntu 22.04 и 24.04: `docker-ce` `5:29.9.3-1`, `containerd.io` `2.4.2-4`, `docker-buildx-plugin` `0.39.0-1`, `docker-compose-plugin` `5.8.0-1`. Суффикс выпуска берётся с гостя. Другой выпуск Ubuntu останавливает установку, а не ставит незакреплённую версию. Это не выбор поддерживаемой Ubuntu: его фиксируют после осмотра компьютера пользователя.
