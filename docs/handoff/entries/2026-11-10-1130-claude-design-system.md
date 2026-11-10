---
status: закончил
agent: claude
date: 2026-11-10 11:30
task: Дизайн-система веба до доделки выпуска 1
refs: [D303, 2026-11-10-1000-claude-pdf-and-stage-kinds]
touches: [docs/platform/design.md, web/src/design/, web/src/ui/, web/scripts/theme-css.mjs, tools/check_design.py, tools/check-code.sh, .cursor/rules/frontend-design.mdc, briefs/ORCHESTRATOR-RELEASE1-FINISH.md]
---

# Закончил: дизайн-система

По поручению пользователя («реши сам») выбран вид «тетрадь и ручка» (D303), спецификация `docs/platform/design.md` (DS-R1…R11). Тема Mantine `web/src/design/theme.ts`, статический `theme.css` генерируется (`npm run theme:css`, сверка `theme:check` в `npm run lint`): CSP запрещает Mantine вставлять переменные во время работы. Шрифт Golos Text из `@fontsource-variable/golos-text`. Общие компоненты `web/src/ui/` с тестами. Образец на маршруте `/design` только в `vite dev`. `tools/check_design.py` (в `tools/check-code.sh`) запрещает цвета вне `web/src/design/`. Правило Cursor `frontend-design.mdc` теперь требует только дизайн-систему.

Проверено: в браузере на 360 px образец без горизонтальной прокрутки, все нажимаемые элементы не ниже 44 px, шрифт загружен, ошибок консоли нет; `vitest` 19 passed, `tsc`, ESLint, Prettier, `vite build` (образец в сборку не попадает), `tools/check.sh` 251 passed.

Не сделано: готовые экраны (Главная, Урок, Выдача, Задача, «Ученики») свёрстаны простым HTML и тему не получают; перевод — пакет W0 инструкции доделки.
