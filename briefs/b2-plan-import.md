# Бриф: B2, импорт плана

Для агента-исполнителя. Прочитайте целиком. При расхождении прав этот бриф только в разделе «Интерфейс»; смысл правил — в `docs/catalog/plan-import.md`. Не читайте реализацию, если вы тест-исполнитель: её ещё нет, и писать её нельзя.

## Цель

Пакет `content/plan_import/` разбирает синтетические карточки тем и файлы уроков формата `ct-2027` в набор плана 0 по контракту `PlanImportRequest` (`docs/contracts/internal-api.yaml`) и в скелет каталога (темы и элементы E1). Правила LP-R1…LP-R20. В базу и в сеть пакет не ходит. Только синтетические данные.

## Источники

- `docs/catalog/plan-import.md` (LP-R1…R20, разделы «Ошибки» и «Таблицы примеров»)
- `docs/catalog/plan-import.examples.yaml` (id `plan-import-01` … `plan-import-14`)
- `docs/contracts/internal-api.yaml`, схемы `PlanImportLesson` и `PlanImportRequest`
- `docs/contracts/catalog.schema.json`, определение `topic` и `element`
- `docs/catalog/schema.md` (CS-R21, CS-R22, CS-R26, CS-R27, CS-R29)

Реальный каталог учеников и `/home/stranger/projects/tutoring/` не открывать.

## Интерфейс (обязателен)

Расположение: `content/plan_import/` (пакет), тесты `content/tests/test_plan_import.py`, синтетические фикстуры `content/tests/fixtures/plan-import/`. Зависимости только из `tools/requirements.txt` (`pyyaml`; `pytest` для тестов). Новых зависимостей нет. Хеш — SHA-256, шестнадцатеричная строка в нижнем регистре, от сырых байтов файла. Это фиксация интерфейса, не правило методики.

```python
import datetime as dt
from pathlib import Path
from plan_import import import_plan, ImportResult

result: ImportResult = import_plan(
    source: Path,
    student_id: str,
    subject: str,
    shift_weeks: int = 0,
    already_imported: bool = False,
    dry_run: bool = False,
    today: dt.date | None = None,
    previous_snapshot: dict | None = None,
    write_dir: Path | None = None,
)
```

`source` — корень синтетического дерева:

```text
<source>/<subject>/topics/*.md
<source>/<subject>/plan/L*.md
<source>/graph/graph.json    # необязательно, только сверка
<source>/PLAN.md             # необязательно, только сверка
```

`subject` — `math` или `physics`. Другое значение — ошибка использования (`ValueError`), не код из спецификации.

`today is None` означает `datetime.date.today()`. Тесты всегда передают дату явно.

### Карточка темы

Frontmatter YAML между `---`: `id`, `title`, `section`, `tier`, `level`, `prereqs`, `ct_positions`, `hours_lesson`, `hours_home`.

Обязательны `id`, `title`, `section`, `level`, `hours_lesson`, `hours_home`. Нет `prereqs` — пустой список. Нет `ct_positions` — пустой список. `tier` читается и выбрасывается.

Пакет: `math` → `ct-math`, `physics` → `ct-phys`. Таблица исключений кодов пуста (LP-R2): код карточки входит в id как есть (`M14` → `ct-math:M14`).

Скелет одной темы — объект, который проходит определение `topic` в `catalog.schema.json`:

- `id`: `<пакет>:<код>`
- `title`, `section` — из карточки
- `foundation_level` — строка `level` как в карточке, без перевода
- `topic_hours` — сумма `hours_lesson + hours_home` (числа)
- `ct_positions` — список строк из карточки; поле не является появлениями
- `prerequisite_elements` — для каждого кода в `prereqs` id `<пакет>:<код>.E1`. Типов импорт не создаёт, поэтому предпосылки остаются на теме (LP-R3)
- `citations` — ровно одна цитата `{doc: topic-card, ref: <путь карточки>}`; путь относителен к `source`, через `/`
- `elements` — ровно один элемент `{id: <тема>.E1, title: "тема целиком", citations: <та же цитата>}`
- поля `tier` нет
- ключа `types` нет

Неизвестный код в `prereqs`, `topics` или `review` — ошибка `topic_unknown`.

### Урок

Frontmatter: `subject`, `lesson`, `week`, `phase`, `kind`, `topics`, `review`. Название — текст первой строки файла, которая начинается с `# `, без решётки и пробелов по краям. Нет такой строки или нет обязательного поля — `frontmatter_invalid`.

`week` — дата `YYYY-MM-DD`. `planned_week` — эта дата плюс `shift_weeks * 7` дней, снова строка `YYYY-MM-DD`. Номер урока не меняется.

`kind` только `diagnostic`, `new`, `consolidation`, `review`, `mock`, `analysis`. Иначе `kind_unknown`. Все виды становятся обычными уроками: ключей ответов импорт не добавляет (LP-R8). Текст урока ниже заголовка не разбирается (LP-R20): в результат попадают путь и хеш.

Урок в `PlanImportRequest.lessons` содержит ровно поля контракта: `subject` (как в frontmatter), `lesson`, `planned_week`, `phase`, `kind`, `topics`, `review`, `title`, `source_path`, `source_hash`. `topics` и `review` — id тем каталога (`ct-math:M10`), не короткие коды карточек. `source_path` — путь от `source` через `/`. Других полей нет: ни даты занятия, ни времени, ни длительности, ни отметок, ни снимка урока (LP-R12).

Нумерация уроков предмета — 1, 2, 3, … без пропусков. Иначе `lesson_gap`. `week` каждого урока — понедельник. Иначе `week_not_monday`. По номерам уроков даты `week` строго возрастают. Иначе `week_not_increasing`.

`review` урока N может ссылаться только на темы, которые есть в `topics` какого-либо урока с меньшим номером. Иначе `review_before_intro`.

### Состояние held (LP-R17)

Поле состояния в `PlanImportLesson` контракт не задаёт, поэтому в запрос его не добавлять. Урок, у которого `planned_week` раньше понедельника недели `today`, попадает в отчёт:

```python
{"lesson": 4, "state": "held", "snapshot": None, "marks": None}
```

Снимок плана 0 (хеши файлов) при этом есть: «без снимка» относится к снимку урока, не к `snapshot.source_hashes`.

### Повтор и сухой прогон (LP-R13, LP-R14, LP-R15)

- `already_imported=True` и `dry_run=False`: единственная блокирующая ошибка `already_imported`. `request` и `skeleton` равны `None`. Файлы не пишутся.
- `dry_run=True`: блокировки `already_imported` нет. `request` строится. База не трогается (её и нет). Если передан `previous_snapshot`, в `report["differences"]` список путей, у которых хеш отличается от `previous_snapshot["source_hashes"]` или которых там нет. Если снимок не передан, `differences` — пустой список.
- `snapshot.source_hashes` — объект «относительный путь → sha256» по каждой карточке и каждому файлу урока. Повторный вызов на тех же байтах даёт те же хеши. Функции, которая меняет уже возвращённый снимок, нет.

### Атомарность (LP-R16)

Любая блокирующая ошибка: `request is None`, `skeleton is None`, `write_dir` не изменяется (ни одного частично записанного файла). Предупреждения импорт не блокируют.

Код `import_failed` — только если `write_dir` задан и запись на диск не удалась после того, как разбор уже успешен. Тогда каталог `write_dir` остаётся как до вызова.

### Сверка (LP-R18)

Формат ниже — только для синтетических фикстур. Реальные `ct-2027` не читать.

- `graph/graph.json`: `{"topics": ["M14", "M13"]}`. Сравнивается множество кодов карточек. Расхождение — предупреждение `generated_mismatch`, в тексте есть код пропавшей или лишней темы. Импорт не блокируется.
- `PLAN.md`: строки таблицы Markdown `| <номер> | <YYYY-MM-DD> |`. Сравниваются число уроков, порядок номеров и недели (до сдвига). Расхождение — то же предупреждение, не блокировка.
- Нет файла — сверки нет и предупреждения нет.
- Файл есть, но не разбирается этим форматом — предупреждение `generated_mismatch`, не блокировка.

### Результат

```python
@dataclass(frozen=True)
class ImportResult:
    request: dict | None          # PlanImportRequest либо None
    skeleton: dict[str, dict] | None  # "ct-math/topics/M14.yaml" -> объект темы
    errors: tuple[dict, ...]      # блокирующие
    warnings: tuple[dict, ...]
    report: dict
```

Ошибка и предупреждение: `{"code", "file", "line", "message"}`. `file` — путь от `source` через `/` (для сверки — `graph/graph.json` или `PLAN.md`). `line` — номер строки с 1; для ошибки набора уроков, у которой нет одной строки, `line` равен 1 и `file` — путь файла урока, из-за которого правило нарушено (для `lesson_gap` — файл с пропущенным номером нет, тогда `file` пустая строка и в `message` есть пропущенный номер). `message` по-русски.

`report` всегда словарь с ключами `created_topics`, `created_lessons`, `held_lessons`, `differences`. При блокировке списки созданного пустые, кроме `dry_run` (там отчёт строится, `request` не `None`, если нет других блокировок).

`write_dir` по умолчанию `None` (ничего не пишет). Если задан и блокировок нет, пакет создаёт все файлы скелета относительно `write_dir` и не создаёт `package.yaml`.

Порядок `errors` и `warnings`: `(file, code, line)`. Порядок уроков в запросе — по номеру. Порядок тем в скелете не важен; ключи путей стабильны.

### CLI

Из каталога `content/`:

```text
python -m plan_import <source> --student <id> --subject math|physics
    [--shift-weeks N] [--dry-run] [--already-imported]
    [--today YYYY-MM-DD] [--json]
```

Код выхода: 0 — блокирующих ошибок нет (предупреждения допустимы), 1 — есть блокирующие ошибки, 2 — ошибка использования. С `--json` в stdout один JSON объекта `ImportResult` (dataclass через поля; `request`/`skeleton` = null). Без `--json` — по строке `файл: код: сообщение` для ошибок, затем предупреждения с префиксом `warning:`.

## Коды

Только эти: `frontmatter_invalid`, `kind_unknown`, `topic_unknown`, `review_before_intro`, `lesson_gap`, `week_not_monday`, `week_not_increasing`, `already_imported`, `import_failed`, `generated_mismatch` (предупреждение). Других кодов не вводить.

## Привязка примеров

Каждый id из `plan-import.examples.yaml` — отдельный тест. Окружение примера достраивается минимальными соседними карточками и уроками, чтобы проверяемое правило было единственным нарушенным или единственным ожидаемым эффектом. Для `plan-import-05` уроки 1–6 вводят темы из `topics` и `review` урока 7, недели — понедельники по возрастанию, утверждение — поля урока 7 и `planned_week`. Для `plan-import-13` сегодня `2026-11-09`, урок 4 с исходной неделей `2026-11-02`, уроки 1–3 существуют и не ломают нумерацию.

Остальные правила без отдельной строки примера тоже имеют тест: LP-R2, LP-R5, LP-R8, LP-R12, LP-R14, LP-R15, LP-R16, LP-R19, LP-R20.

## Ограничения

- Не изменять `docs/`, `docs/contracts/`, `legal/`, `content/catalog_ci/`, демо-каталог.
- Не придумывать поля контракта и коды вне этого брифа.
- Не читать приватные данные учеников. В фикстурах ученик только `student-demo`.
- Не обращаться к сети, git и базе.
- Тест-исполнитель не пишет реализацию и не делает `git commit`.
- Код-исполнитель не меняет тесты и `docs/contracts/`.

## Уточнения после вопросов тест-исполнителя

Это часть интерфейса. Методика предмета не меняется.

- `dry_run=True` не пишет в `write_dir`. Сухой прогон только строит запрос и отчёт (LP-R13).
- У `already_imported` и `import_failed` нет строки источника: `file` — пустая строка, `line` — 1.
- Два файла с одним номером урока — `frontmatter_invalid`. Это не `lesson_gap`.
- Пустой `topics` не проходит контракт (`minItems: 1`) — `frontmatter_invalid`.
- Номер урока меньше 1 — `frontmatter_invalid`. Пропуск номера 1 при нумерации с 2 остаётся `lesson_gap`, как в тестах.
- `subject` во frontmatter урока не совпадает с аргументом `subject` — `frontmatter_invalid`.
- Нет каталога `<source>/<subject>/` — `ValueError`, как при неверном предмете.
- Нет обязательного поля или строки `# ` — `line` равен 1. Поле есть, но тип неверный — `line` равен строке этого поля.
- `week` не разбирается как дата — только `frontmatter_invalid`, без `week_not_monday`.
- Проверок HTTP, CSRF, Origin и ключа идемпотентности у пакета нет: писатель базы — `core`. Граница пакета — чужой ученик и чужой предмет.

## Критерии готовности

**Тесты.** `cd content && python -m pytest --collect-only -q` без ошибки синтаксиса. На каждый LP-R1…R20 есть тест, в имени есть номер (`test_lp_r4_…`). В докстринге — номер правила и, где есть, id примера. Красные допустимы только из-за отсутствия `plan_import`.

**Код.** `cd content && python -m pytest -q` целиком зелёный. Интерпретатор: `/home/stranger/projects/university/coursework/ai-mentor/.venv/bin/python`.
