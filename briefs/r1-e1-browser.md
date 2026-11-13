# Бриф: E1, браузерная половина

Go-прогон `TestWalkSkeleton` уже зелёный. Здесь только браузер.

## Что сделать

1. `core/cmd/server` при `ENV=local`, `AUTH_DEV_MODE=true` и `DATABASE_URL` поднимает `compose.Handler` вместе с `/healthz`. Без этих переменных остаётся прежний `/healthz`, чтобы старый тест сервера не сломался.
2. Ответы сервера несут заголовок  
   `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`.  
   `script-src` без `unsafe-inline` и без `unsafe-eval`.
3. Сервер отдаёт собранный `web/dist` с того же порта, что и API, чтобы страница и API были одним источником.
4. `tools/e2e/walk.spec.ts` на Playwright, `channel: 'chrome'`, браузер не скачивать. Размер экрана 390×844.  
   Сценарий: ученик открывает `/task/asg-demo/item-a`, отвечает, ключ до ответа не виден; репетитор на `/tutor` видит попытку.  
   `@axe-core/playwright` без серьёзных нарушений. Ошибки консоли роняют тест. Нарушение CSP по скриптам роняет тест.
5. Перед сценарием тест сам поднимает Postgres 16.15 (том с именем, удалить после), применяет миграции, вставляет тех же `tutor-demo` и `student-demo` и выдачу `asg-demo`, что и `TestWalkSkeleton`, затем собирает `web` и запускает сервер.

## Запреты

Не скачивать браузеры Chromium от Playwright. Не менять замороженные тесты `walk_test.go`, `TaskPage.test.tsx`, `TutorPage.test.tsx`. `git commit` нельзя.

## Приёмка

Из корня worktree, Chrome уже установлен как `google-chrome-stable`:

```bash
cd core && go test -count=1 -timeout 60s ./cmd/server/
cd tools/e2e && npm install --ignore-scripts && npx playwright test --reporter=line
```
