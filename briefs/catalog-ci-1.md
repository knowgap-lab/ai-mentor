# Бриф: проверка каталога (CI каталога), пакет 1 «структура»

Для агента-исполнителя. Прочитайте целиком, прежде чем что-либо менять. Бриф самодостаточен; источники правды — спецификации, на которые он ссылается (при расхождении прав бриф только в разделе «Интерфейс»; в остальном права спецификация).

## Цель

Python-пакет `catalog_ci` в `content/` проверяет каталог в git (формат из `docs/catalog/schema.md`) и возвращает список проблем. Это первый шаг D159 и основа CI каталога. Пакет 1 покрывает **структуру**: схему файлов, идентификаторы, ссылки, предпосылки, часы уровней, формат ответа, появления. Не входит: сравнение с историей git (`id_removed`, `id_reused`, `transfer_map_missing`), проверки ключей и опций ответов (AR-R16…R20), размер пула вариантов (VR-R1/R2), независимая проверка ключей. Это пакеты 2 и 3.

## Источники

- `docs/catalog/schema.md` (CS-R1…R30, раздел «Ошибки»), `docs/catalog/schema.examples.yaml`
- `docs/contracts/catalog.schema.json`, `docs/contracts/subject-package.schema.json`, `docs/contracts/fixtures/catalog.cases.yaml`
- `docs/catalog/composer.md` (шаблон урока), `docs/catalog/plan-import.md` (LP-R4)
- демо-каталог: `content/tests/fixtures/demo-catalog/ct-demo/` (синтетический, D142)

## Интерфейс (обязателен)

Расположение: `content/catalog_ci/` (пакет), `content/tests/` (тесты pytest). Python 3.12; зависимости: `jsonschema`, `pyyaml`, `pytest` (см. `tools/requirements.txt`; добавьте `pytest` туда).

```python
from catalog_ci import check_catalog, Problem
problems: list[Problem] = check_catalog(path: pathlib.Path, mode: str = "pr")
```

- `path` — каталог, внутри которого лежат один или несколько каталогов пакетов (`<path>/<пакет>/package.yaml`). Раскладка файлов пакета — как в schema.md («Раскладка файлов»): `package.yaml`, `answer-rules.yaml` (путь из поля `answer_rules` пакета), `topics/<тема>.yaml`, `variants/<тема>/<тип>.yaml`, `blocks/<тема>/<блок>.md` (frontmatter YAML между `---`), `tests/<тест>.yaml`, `lessons/L<nn>.yaml`, `transfer-map.yaml`, `notes/*.yaml`. Файлы вне раскладки игнорируются без ошибки.
- `mode`: `"pr"` (временные ключи `tmp:…` допустимы, CS-R6) или `"main"` (запрещены).
- `Problem` — dataclass (frozen): `code: str`, `file: str` (путь относительно `path`, через `/`), `pointer: str` (JSON Pointer до места в документе; для блока — в frontmatter; пустая строка, если не применимо), `message: str` (по-русски, понятно человеку).
- Результат отсортирован по `(file, code, pointer)` и детерминирован. Пустой список = каталог корректен.

CLI: `python -m catalog_ci check <path> [--mode pr|main] [--json]`, запускается из `content/`. Код выхода: 0 — проблем нет, 1 — есть проблемы, 2 — ошибка использования (нет пути, неверный режим). Без `--json` — по строке `файл: код: сообщение`; с `--json` — JSON-массив объектов `{code, file, pointer, message}` в stdout.

## Коды проблем (пакет 1)

| Код | Когда | Правило |
|---|---|---|
| `id_invalid` | id сущности (не `tmp:`) не подходит под регулярное выражение **своего вида** (тема, элемент, тип, подвариант, вариант, блок, тест, шаблон урока, этап) | CS-R1, R3, R30 |
| `duplicate_id` | один id встречается дважды в загруженных пакетах | CS-R4 |
| `field_not_allowed` | в документе есть поле, которого нет в схеме (например `tier`) | CS-R29 |
| `schema_invalid` | любое другое нарушение схемы (тип, перечисление, обязательное поле, не покрытое другим кодом) | schema.md |
| `citation_missing` | у темы, элемента или типа нет цитат или список пуст | CS-R21 |
| `primary_element_missing` | у типа нет `primary_element` | CS-R9 |
| `prereq_not_element` | в `prerequisites` типа стоит не id элемента (например id темы или типа) | CS-R10 |
| `prereq_cycle` | цикл «тип → элемент-предпосылка → основной элемент другого типа → …» (сообщение называет цикл) | CS-R11 |
| `level_hours_order` | `automatism.hours < base.hours` | CS-R17 |
| `answer_format_unknown` | `answer_format` типа не объявлен в `answer_formats` правил пакета | CS-R16 |
| `dangling_ref` | ссылка на несуществующий id: `primary_element`, `prerequisites`, `subvariant` варианта, `theory_block`, `type` в файле вариантов и в появлениях теста, `topic_id` блока, темы/блоки/типы/подварианты в шаблоне урока и рецепте ДЗ | schema.md §5 |
| `skill_has_appearance` | тип `kind: skill` встречается в появлении теста | CS-R15 |
| `tmp_key_in_main` | `mode="main"` и любой id или ссылка начинается с `tmp:` | CS-R6 |

Правила выбора кода: одно нарушение — один основной код; если исходное значение не проходит схему по формату id, это `id_invalid`, а не `schema_invalid`; лишнее поле — `field_not_allowed`. Недопустимо глотать проблемы: все найденные проблемы возвращаются, а не только первая.

## Ограничения (MUST NOT)

- Не изменять `docs/`, `docs/contracts/`, демо-каталог и существующие файлы вне `content/` и `tools/requirements.txt`.
- Не придумывать коды проблем, поля и id вне брифа и спецификаций.
- Не обращаться к сети, git и базе данных.
- Не добавлять зависимости сверх перечисленных.

## Критерии готовности

**Этап тестов** (отдельная сессия, код не пишется): в `content/tests/` лежат тесты на каждый код из таблицы и на CLI; тесты красные из-за отсутствия `catalog_ci`, но собираются: `cd content && python -m pytest --collect-only -q` без ошибок синтаксиса, не меньше 30 тестов.

**Этап кода** (другая сессия; тесты менять нельзя): `cd content && python -m pytest -q` — все зелёные; демо-каталог не даёт проблем в обоих режимах.

Тесты: фикстура копирует демо-каталог в `tmp_path`, тест мутирует копию минимальным изменением и проверяет, что нужный код присутствует (`assert code in {p.code for p in problems}`), а на неизменённой копии список пуст. На каждый код минимум один тест; для `dangling_ref` — по одному на вид ссылки; для `tmp_key_in_main` — парные тесты режимов `pr` и `main`; для CLI — коды выхода 0, 1, 2 и формат `--json`. В докстринге каждого теста — номер правила спецификации.
