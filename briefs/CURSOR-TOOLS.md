# Инструменты для работы с выпуском 1

- Git и GitHub CLI используются для проверки актуальных веток, PR, SHA и логов CI. Права подтверждайте на нужном репозитории. Подключение интеграции и действия владельца описывайте в [реестре внешних действий](../docs/handoff/release1-actions.yaml).
- Установку Go, Node/npm, Python, Docker, Postgres и Chrome оформляйте скриптами из репозитория. Предложенная реализация рабочей среды — [PR #28](https://github.com/knowgap-lab/ai-mentor/pull/28). После слияния используйте инструкции README из свежего клона. Для Cursor храните адаптер `.cursor/environment.json` рядом с общими скриптами.
- Playwright и локальные проверки запускаются командами из Git. Chrome DevTools и Context7 помогают в диагностике и чтении документации. При недоступном Context7 используйте официальную документацию версии из lockfile.
- В новом аккаунте подключите GitHub, выберите окружение репозитория в Workspace нового Project и откройте [порядок продолжения](CURSOR-PROJECT-RESUME.md). Для запуска проверьте источник конфигурации, активный Cursor Build и SHA checkout. Настройки интерфейса, права и результаты фиксируйте в журнале. Храните секреты в предусмотренном хранилище по [описанию](../deploy/secrets.md).

[Cursor Cloud Environment](https://cursor.com/docs/cloud-agent/setup) · [Builds](https://cursor.com/docs/cloud-agent/builds)
