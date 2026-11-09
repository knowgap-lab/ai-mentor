# Бриф: A0, разбиение catalog_ci

Для код-исполнителя. Прочитайте целиком. Источник правды по поведению — текущий `content/catalog_ci/checker.py` и зелёные тесты `content/tests/`. Спецификации не расширяют и не сужают этот пакет: поведение списка проблем должно остаться тем же.

## Цель

Разбить `content/catalog_ci/checker.py` (около 1000 строк) на модули без изменения поведения. Публичный интерфейс и CLI не меняются. Тесты не меняются. Все существующие тесты остаются зелёными.

## Источники

- `content/catalog_ci/checker.py`, `__init__.py`, `__main__.py`, `problem.py`
- `content/tests/test_check_catalog.py`, `content/tests/test_cli.py`
- `briefs/catalog-ci-1.md` — контракт интерфейса, который нельзя сломать

## Интерфейс (не менять)

```python
from catalog_ci import check_catalog, Problem
problems: list[Problem] = check_catalog(path: pathlib.Path, mode: str = "pr")
```

CLI из каталога `content/`: `python -m catalog_ci check <path> [--mode pr|main] [--json]`. Коды выхода 0, 1, 2. Тексты проблем, коды, пути, указатели и порядок сортировки `(file, code, pointer)` остаются прежними.

## Куда класть код

Только внутри `content/catalog_ci/`:

| Модуль | Ответственность |
|---|---|
| `load.py` | `Document` и загрузка раскладки: поиск пакетов, YAML, frontmatter блоков, безопасный путь `answer_rules`. Функция `load_documents(root) -> tuple[list[Document], list[Problem]]` |
| `schema_layer.py` | проверка JSON Schema и выбор кода проблемы (`id_invalid`, `field_not_allowed`, `schema_invalid`, `citation_missing`, `primary_element_missing`, `prereq_not_element`). Функция `schema_problems(documents) -> list[Problem]` |
| `cross.py` | `Entity` и межфайловые проверки (ссылки, циклы, часы, формат ответа, появления, временные ключи). Функция `cross_file_problems(documents, mode) -> list[Problem]` |
| `pointers.py` | только если помощник JSON Pointer нужен двум модулям. Иначе оставьте помощник в том модуле, который им пользуется |
| `checker.py` | только `check_catalog`: загрузка, схема, межфайловые проверки, сортировка. Проверки режима и существования каталога остаются здесь |
| `problem.py` | без изменений |
| `__init__.py` | по-прежнему экспортирует только `Problem` и `check_catalog` |
| `__main__.py` | CLI с тем же поведением; импорт `check_catalog` из пакета |

Имена внутренних функций можно сохранить. Новых кодов проблем, полей и зависимостей нет.

## Ограничения

- Не менять `content/tests/`, `docs/`, `docs/contracts/`, фикстуры, `tools/requirements.txt`.
- Не добавлять зависимости.
- Не обращаться к сети, git и базе.
- Не делать `git commit` и `git push`.
- Не менять поведение, чтобы «улучшить» проверку. Если тест покраснел, чините разбиение.

## Критерий готовности

Из корня worktree, интерпретатор `/home/stranger/projects/university/coursework/ai-mentor/.venv/bin/python`:

```bash
cd content && /home/stranger/projects/university/coursework/ai-mentor/.venv/bin/python -m pytest -q
```

Все тесты зелёные (сейчас их 42). `git diff -- content/tests` пуст.
