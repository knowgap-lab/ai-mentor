# Ansible выпуска 1

Плейбук настраивает **уже созданную** чистую Ubuntu по SSH: базовые пакеты, Docker, доступ по ключу, журналы и стек prod/staging из `deploy/compose*.yml` на одном SHA. Гипервизор здесь не выбирается, VM не создаётся, `terraform apply` в этом пакете нет.

SHA и репозитории образов задаются в `release.json` рядом с плейбуком. На госте клонируется тот же SHA; `IMAGE_TAG` у prod и staging совпадает с ним (OP-R9).

Повторный запуск рассчитан на отсутствие неожиданных изменений: пакеты закреплены версией, юниты — шаблоном, `docker compose up` без `--force-recreate`, загрузчик каталога в обычный запуск не входит.

Это не приёмка VM. Запуск на выделенной Ubuntu в этой сессии не выполнялся.

## Проверка синтаксиса

```bash
cd infra/ansible
ansible-playbook --syntax-check site.yml
```

`inventory/example.yml` — пример для этой проверки. Адрес `192.0.2.10` и домены `prod.example`, `staging.example` не являются живым гостем.

## Запуск на госте

Инвентарь `inventory/guest.yml` в git не входит. Нужны `ansible_host`, `ansible_user` (не root), `cookie_domain_prod` и `cookie_domain_staging`. Домены должны быть разными (OP-R2). Пока домены не выбраны, плейбук не запускать на чужой машине.

```bash
ansible-playbook -i inventory/guest.yml site.yml
# Повтор для проверки идемпотентности:
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

## Роли и что настраивается

| Роль | По спецификации | Содержание |
|------|-----------------|------------|
| `access` | OP-R19 | `ufw`, `ca-certificates`, `curl`, `gnupg`, `git`, `python3`; UFW: deny incoming, allow SSH/80/443; SSH без root и без пароля при наличии ключа |
| `engine` | — | Репозиторий Docker CE с закреплёнными версиями пакетов; `daemon.json` (ограничение логов); служба `docker` включена |
| `stack` | OP-R2, OP-R7, OP-R9, `deploy/compose*.yml` | Клон git на SHA; каталоги `/etc/ai-mentor`; env-файлы; при готовых секретах — `docker compose pull` и `up` для `db`, `gotenberg`, `migrate`, `core`, `web` (prod и staging); юниты backup/restore (таймеры выключены без `backup.env`) |
| `retention` | OP-R18 | `journald` 30 дней; таймер очистки повёрнутых логов Docker |

## Повторный запуск

Допустимые изменения при втором converge перечислены в исходном пакете IAC-VM (индекс apt, новый слой образа, несовпадение контейнера). Каталог (`--profile catalog`) в обычный запуск не входит (OP-R8).

## Пробелы (в спецификации нет — в плейбуке нет)

1. **Terraform / создание VM** — отдельный путь IAC-VM; успешный Ansible на готовой VM не заменяет `terraform plan` и создание одноразовой VM (раздел 8 брифа).
2. **Обратный прокси** — D304 не выбирает Caddy/Nginx; порты 80/443 открыты, слушателя плейбук не ставит.
3. **Наблюдаемость M1** — Collector, Prometheus, Loki, Tempo, Grafana, Alertmanager (D313): пакет `deploy/` для compose приложения есть, стек наблюдаемости в этом плейбуке не разворачивается.
4. **`status-check` по расписанию** — OP-R17 описывает вызов с сервера; systemd-юнит для внешней проверки `/status` здесь не добавлен (в git нет URL страницы состояния).
5. **`disk-check` / `log-retain` по cron** — скрипты в `deploy/`, но расписание сервера в спецификации не детализировано для Ansible; кроме таймера очистки Docker-логов (OP-R18).
6. **Публикация образов GHCR** — плейбук тянет образы по SHA; сборка и публикация — CI, не Ansible.
7. **Версия Ubuntu гостя** — закрепление Docker рассчитано на 22.04/24.04 amd64/arm64; поддерживаемая версия фиксируется после осмотра машины пользователя, не в этом PR.

Пакеты Docker сверены с репозиторием Docker для Ubuntu 22.04 и 24.04: `docker-ce` `5:29.9.3-1`, `containerd.io` `2.4.2-4`, `docker-buildx-plugin` `0.39.0-1`, `docker-compose-plugin` `5.8.0-1`. Суффикс выпуска берётся с гостя.