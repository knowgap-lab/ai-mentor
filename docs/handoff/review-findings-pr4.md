# Находки ревью PR #4 (Cursor Bugbot, 2026-11-13)

> Автоматическое ревью кода выпуска 1 при переносе в `main`. Находки **не проверены** человеком или вторым ревьюером. Доделка выпуска 1 сначала проверяет каждую (воспроизводит тестом), затем чинит или отклоняет с причиной. Замороженные тесты (D131) не меняются без решения владельца модуля; где находка спорит с замороженным тестом, она уходит в файл конфликтов замороженных тестов.

| # | Серьёзность | Где | Суть | Связь со спецификацией |
|---|---|---|---|---|
| 1 | high | `core/internal/homework/service.go` `loadLesson` | читает `core.lesson.template` как объект, а уроки пишут туда массив этапов; выдача ДЗ по живому уроку падает на разборе | lessons ↔ homework |
| 2 | high | `core/internal/homework/service.go` выдача, `assemble.go` | снимок выдачи хранит задачи без ключа ответа; проверка ответа читает `item.Key` из снимка и не может оценить | answer-check |
| 3 | high | `core/internal/attempts/ops.go` `PATCH /lessons/{lesson}/homework-draft`, `core/cmd/server/mount.go` | обслуживает attempts: проверяет только роль репетитора, не владельца урока; пишет `items` массивом строк, после чего GET через homework не разбирает | права, D305 (где живут операции) |
| 4 | medium | `core/internal/attempts/service.go` `loadAssignment`, `homework/service.go` | отсутствующий объект даёт 404, чужой — 403: можно перебирать id. answer-check уже отдаёт 403 в обоих случаях | сверить с контрактом и замороженными тестами |
| 5 | medium | `core/internal/attempts/ops.go` `GET /assignments/{id}/attempts` | отдаёт `verdict` и `points` ученику без проверки вида выдачи и разрешения показа | AC-R7 |
| 6 | medium | `core/internal/attempts/ops.go` сдача | ключ идемпотентности хранится как `(user_id, Idempotency-Key)` без id выдачи; повтор ключа на другой выдаче возвращает чужой ответ и не закрывает её | attempts |
| 7 | medium | `core/internal/homework/service.go` `doIssue` | выдача не ставит `core.lesson.issued`; отметки урока можно менять после выдачи ДЗ | LS-R19a |
| 8 | medium | `core/internal/attempts/service.go` | изменяющие запросы attempts не сверяют `Origin` с `APP_ORIGIN`, в отличие от auth, answer-check, lessons, homework | auth, D307 |

Источник: https://github.com/knowgap-lab/ai-mentor/pull/4 (комментарии `cursor[bot]`).
