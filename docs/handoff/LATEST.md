# Текущее состояние

> Единственный перезаписываемый файл журнала. Обновляйте его в конце каждой сессии. Подробности — в [записях](entries/); протокол — в [README](README.md).

**Обновлено:** 2026-11-24 (AM-43 smoke a35f8b0 — fail)
**Ветка:** `cursor/review-card-decisions` (PR pending)
**Этап:** сверка For Review с main `a35f8b0`

## Где мы

| Область | Готово | Не готово |
|---|---|---|
| Main demo | release1-ci кроме deploy; code-check/docs-check зелёные | `deploy`, attestation на main, checker exit 0 |
| For Review | Решения записаны в [review-card-decisions.md](review-card-decisions.md) | Закрытие карточек — за человеком |
| Observability / journal / PDF на main | Через PR #6 (AM-37, AM-20, AM-19 landed) | Устаревшие открытые PR #20, #8, #7 |

## Что в работе сейчас

Документация решений по карточкам; аттестация по-прежнему на [PR #69](https://github.com/knowgap-lab/ai-mentor/pull/69), не на main.

## Следующий шаг

AM-43: повторить dev smoke на VM с `sg docker` и доступным Chrome deb; VM acceptance и второй review (AM-64, AM-1).

## Блокеры и вопросы

Секреты VM, чистый Ubuntu guest, `release1_done.py` exit 0 на одном SHA в main.

## Куда смотреть

- [review-card-decisions.md](review-card-decisions.md)
- Запись [2026-11-24-1055-cursor-review-card-decisions-done.md](entries/2026-11-24-1055-cursor-review-card-decisions-done.md)
- AM-1 rollup в том же файле