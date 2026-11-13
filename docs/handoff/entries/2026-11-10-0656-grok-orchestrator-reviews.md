---
status: закончил
agent: grok
date: 2026-11-10 06:56
task: ревью фаз 1 и 3 и одна правка
refs:
  - docs/handoff/entries/2026-11-10-0616-orchestrator-release1-done.md
  - docs/handoff/entries/2026-11-10-0410-grok-orchestrator-phase2-review.md
touches:
  - core/internal/privacy/handler.go
  - core/internal/lessons/service.go
  - core/internal/homework/service.go
  - core/internal/plan/service.go
  - core/internal/conspect/service.go
  - deploy/backup.sh
  - migrations/06002_telegram_unique.sql
---

# Ревью фаз дописано, одна правка влита

**Начало работы:** отчёт `docs/handoff/entries/2026-11-10-0616-orchestrator-release1-done.md`.

**Сделано.**

Четыре ревьюера фаз 1 и 3, все `VERDICT: REQUEST_CHANGES`. Одна повторная постановка, ветка `feat/r1-p2fix`, код `76c525a`, слияние `d01ffaf`. Тесты не менялись.

Влито и проверено заново:

- отзыв согласия ставит `revoked_at` у сеансов ученика; `go test` privacy, auth, telegram, attempts зелёные;
- непустой `telegram_id` уникален в `user_pii` и `student_pii`;
- Origin уроков, ДЗ и плана берётся из `Config.AppOrigin`, пустое значение — `http://localhost`; lessons, homework, plan, conspect зелёные;
- тег `script` в конспекте отбрасывается без учёта регистра;
- `backup.sh` не кладёт DSN и токен бота в аргументы и удаляет открытый дамп, если шифрование не вышло. `pytest deploy/ops_test.py` — 13 passed.

**Не сделано.**

Вторая постановка по остальным находкам. Повторно их не чинил: замороженные тесты задают другой контракт, а часть — это целые экраны и правила, которых тесты не вызывают.

Остаётся в отчёте человеку, без новой правки:

- фаза 2: `Exchange` не вызывает `Check`; `POST /consent` не смотрит Origin; приглашение не единственное и не требует согласия; обезличивания через 7 дней нет;
- фаза 1: список попыток может показать вердикт до сдачи; чужой черновик ДЗ в пакете attempts не проверяет владельца; журнал событий — не конверт `events.schema.json`;
- фаза 3: нет дополнительного занятия, пропуска урока и экрана «Подготовка»; выдача ДЗ не ставит признак, который запрещает править отметки; в кабинете нет итога, профиля, `ct.match` и `ct.digits`.

**Решения.** нет

**Открытые вопросы.** Способ PDF — прежний файл. C1 Python-блока — прежний файл.

**Проверки.** Пакетные `go test` и pytest файлов эксплуатации. Полный `tools/check-code.sh` после этой правки не гонялся.

## Дальше

Очередь обоих оркестраторов закрыта: каждый пакет принят или заблокирован. Новые пакеты не начинать, пока человек не выберет PDF и не решит, какие находки ревью чинить отдельно.
