# Бриф: B1, артефакт каталога

Для агента-исполнителя. При расхождении прав бриф только в разделе «Интерфейс». Смысл правил — в `docs/catalog/loader.md`. Алгоритм хеша — D275.

## Цель

Собрать один нормализованный артефакт версии, посчитать `content_hash` сущностей, список вариантов с исправленным ключом и проверить целостность архива. В памяти пакета — журнал ревизий LD-R4…LD-R8 без PostgreSQL и без вызовов `core` по сети. Подпись minisign не реализуется.

Не входит: LD-R9, LD-R11, LD-R13…LD-R16, LD-R22, окружения staging и prod, примеры `loader-09` … `loader-13`. Их не реализовать и не помечать как пройденные.

## Источники

- `docs/catalog/loader.md` (LD-R1…LD-R8, LD-R10, LD-R12, раздел «Артефакт версии», раздел «Ошибки»)
- `docs/catalog/loader.examples.yaml`: в этом пакете `loader-01` … `loader-08`
- D275 в `docs/decisions/log.md`
- `content/catalog_ci/` — `check_catalog` и загрузка документов

## Интерфейс

Пакет `content/catalog_artifact/`. Тесты `content/tests/test_artifact.py`. Новых зависимостей нет: `hashlib`, `json`, `tarfile`, `compression.zstd` (Python 3.14).

Канонический JSON сущности, D275: `json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))`, кодировка UTF-8, без перевода строки в конце. `content_hash` — SHA-256 этих байтов, 64 шестнадцатеричных символа в нижнем регистре. В архиве лежат те же байты. Числа не переводятся в форму RFC 8785: целое остаётся целым, дробное пишется так, как его отдал `json.dumps`.

Нормализация сущности перед хешем. Вложенные объекты, у которых свой id и которые сами являются сущностями, в payload родителя заменяются отсортированным списком их id. Тогда опечатка в типе меняет одну ревизию, а не ревизию всей темы (LD-R5). Сам вложенный объект хранится отдельной сущностью. У блока в payload есть frontmatter и поле `body` с текстом Markdown.

Сущности: тема, элемент, тип, подвариант, вариант, блок, тест, шаблон урока, этап. Файл вариантов сущностью не является: сущность — каждый вариант.

```python
def build_artifact(
    catalog: pathlib.Path,
    *,
    ref: str,
    commit: str,
    catalog_version: int,
    previous: Artifact | None = None,
    out: pathlib.Path,
) -> Artifact:
    ...
```

- `ref` не `"main"` — исключение `BuildRefused`. Файл `out` не создаётся (`loader-01`, LD-R1). Отдельного кода ошибки в спецификации для этого нет, новый код не вводить.
- `check_catalog(catalog, mode="main")` непустой — исключение `ArtifactInvalid` с кодом `artifact_invalid`. Файла нет.
- `previous` задан и в нём есть id, которого нет в новом каталоге — `ArtifactInvalid` / `artifact_invalid`. Удаление ревизии запрещено (LD-R4); вывод из использования — `status: retired` в новом каталоге.
- Иначе пишется `out`: tar, сжатый zstd. Имя файла роли не играет. Внутри `manifest.json` и `entities/<id>.json`. Каталога `figures/` нет, пока пакет сам не рисует SVG.

`manifest.json` — тот же канонический JSON:

```json
{
  "active": [{"content_hash": "...", "id": "..."}],
  "catalog_version": 8,
  "commit": "...",
  "corrected_keys": [],
  "retired": [{"content_hash": "...", "id": "..."}]
}
```

Списки `active` и `retired` отсортированы по `id`. `retired` — сущности со `status: retired`. Остальные — `active`. `corrected_keys` — id вариантов, у которых в `previous` тот же id и другое поле `key`, по алфавиту. Нет `previous` — список пуст.

```python
def verify_artifact(path: pathlib.Path, *, signer=None) -> Artifact:
    ...
```

Проверка целостности: архив открывается; у каждого id из манифеста есть файл; хеш байтов файла равен `content_hash`; лишних файлов сущностей нет; манифест сам канонический. Иначе `ArtifactInvalid` / `artifact_invalid`.

`signer` — необязательная функция `(path) -> bool`. `None` значит, что подпись не проверяется (точка расширения, LD-R2 не реализован). Вернула `False` — `SignatureInvalid` / `signature_invalid`, журнал ревизий не меняется (`loader-02`). Minisign не вызывать и не ставить.

```python
def apply_version(log: VersionLog, artifact: Artifact, *, fail_at: int | None = None) -> ApplyResult:
    ...
```

`VersionLog` и `ApplyResult` неизменяемы. Входной `log` функция не мутирует.

Ревизия: `id`, `catalog_version_from`, `content_hash`, `data` (payload). Новая ревизия добавляется только если хеш id изменился или id новый (LD-R6). Старые ревизии остаются (LD-R4).

- Журнал пуст: принимается `catalog_version >= 1`. Иначе `artifact_invalid`, журнал тот же.
- Есть изменения хешей: `catalog_version` обязан быть равен голове журнала плюс 1. Иначе `artifact_invalid`. Голова становится этим номером. Рядом хранится `commit` (LD-R5).
- Изменений нет: голова и ревизии те же, даже если в артефакте тот же номер. Вызовов нет (LD-R8, `loader-05`).
- `read_version(log, n)` возвращает по каждому id payload последней ревизии с `catalog_version_from <= n`, включая retired. Чтение версии 6 после загрузки версии 8 отдаёт прежние байты блока (`loader-06`, LD-R7).
- `fail_at=k` — после решения добавить k-ю новую ревизию функция откатывается: возвращается прежний журнал, код `load_failed`, список вызовов пуст (`loader-07`, LD-R10). `fail_at is None` — обычный путь.

Вызовы, которые результат только записывает, без сети:

- всегда при успешной загрузке с новым номером: `{"kind": "version", "catalog_version": N, "commit": "..."}`;
- если `corrected_keys` не пуст: `{"kind": "recheck", "correction_id": N, "variants": ["..."]}` (`loader-08`, LD-R12). `correction_id` равен `catalog_version` артефакта.

Повтор той же успешной загрузки вызовов не добавляет.

## Примеры, которые надо закрыть тестами

| id | Ожидание |
|---|---|
| loader-01 | `ref="pr"` → `BuildRefused`, файла нет |
| loader-02 | `signer` вернул `False` → `signature_invalid`, журнал не изменился |
| loader-03 | тип стал retired → старая ревизия на месте, в новой версии статус retired |
| loader-04 | голова 7, одна опечатка, версия 8 → голова 8 и ровно одна новая ревизия |
| loader-05 | тот же артефакт второй раз → журнал и вызовы те же |
| loader-06 | чтение версии 6 после версии 8 возвращает старый блок |
| loader-07 | `fail_at=1` при двух новых ревизиях → `load_failed`, журнал прежний, вызовов нет |
| loader-08 | ключ варианта сменился с 12 на 14 → `recheck` с `correction_id` версии и этим id |

Плюс отдельные тесты: одинаковые payload дают один хеш (LD-R3); правка пробела в сериализации не является отдельным правилом — хеш считается от канонических байтов, два вызова `build_artifact` на одном каталоге дают один и тот же хеш; `verify_artifact` принимает свой архив и отвергает подменённый байт сущности.

Демо-каталог `content/tests/fixtures/demo-catalog` можно копировать и точечно менять. Данные только синтетические.

## Ограничения

- Не менять `content/catalog_ci/`, кроме чтения его публичных функций и загрузчика документов.
- Не менять `docs/` и контракты.
- Не ставить зависимости. Не вызывать сеть, git, minisign.
- Тест-исполнитель не пишет пакет. Импорт внутри теста. `pytest --collect-only` проходит.
- Код-исполнитель не меняет тесты.

## Критерий готовности

Интерпретатор: `/home/stranger/projects/university/coursework/ai-mentor/.venv/bin/python`.

`cd content && python -m pytest -q` целиком зелёный.
