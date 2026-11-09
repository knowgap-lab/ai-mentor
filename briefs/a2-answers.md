# Бриф: A2, ключи и пул вариантов

Для агента-исполнителя. При расхождении прав бриф только в разделе «Интерфейс». Смысл — в `docs/catalog/answer-rules.md` и `docs/catalog/variants.md`.

## Цель

Добавить к `catalog_ci` проверки ключей и опций (AR-R16…AR-R20) и пула вариантов (VR-R1, VR-R2, VR-R4, VR-R10). Независимую проверку ключа `key_mismatch` не делать.

## Источники

- `docs/catalog/answer-rules.md` (AR-R16…AR-R20, раздел «Ошибки»)
- `docs/catalog/answer-rules.examples.yaml`, строки CI: ключ `А1Б1В4` валиден; `2.5` без фразы масштаба → `key_not_integer`; две одинаковые опции → `options_not_distinct`
- `docs/catalog/variants.md` (VR-R1, VR-R2, VR-R4, VR-R10, коды `pool_too_small`, `reinforcement_impossible`, `minutes_out_of_range`)
- `docs/contracts/subject-package.schema.json`, поле `scale_phrases`
- `docs/contracts/catalog.schema.json`, определения `variant` и `option`
- пакет `content/catalog_ci/` после A1

## Интерфейс

Новый модуль `content/catalog_ci/answers.py`. `check_catalog` вызывает его и добавляет проблемы в общий отсортированный список. Старые тесты не менять: демо-каталог остаётся без этих кодов.

Коды только из спецификации: `key_not_integer`, `key_digits_order`, `key_match_incomplete`, `options_not_distinct`, `distractor_equals_key`, `correct_count_mismatch`, `trap_missing`, `pool_too_small`, `reinforcement_impossible`, `minutes_out_of_range`.

Формат ответа берётся у типа варианта. Нормализация текста опции — убрать пробелы в начале, в конце и внутри (AR-R6).

| Код | Условие |
|---|---|
| `key_not_integer` | формат `ct.integer`, ключ не целое (не int и не строка `^-?[0-9]+$`) и в `statement` нет ни одной фразы из `scale_phrases` пакета как подстроки. Фраза есть — кода нет. Для этого указателя нет второго кода `schema_invalid` |
| `key_digits_order` | формат `ct.digits`, ключ не строка цифр, записанная строго по возрастанию без повторов |
| `key_match_incomplete` | формат `ct.match`, в ключе нет ровно одной пары на каждую из букв А, Б, В. Образец валидного ключа: `А1Б1В4`. Латиница в ключе не заменяет кириллицу |
| `options_not_distinct` | формат `ct.choice1` или `ct.choiceN`: опций не 5, номера `n` не есть множество {1,2,3,4,5}, или нормализованные тексты совпали |
| `distractor_equals_key` | нормализованный текст опции, чей `n` не входит в ключ, совпал с текстом опции, чей `n` входит в ключ |
| `correct_count_mismatch` | `ct.choice1` и в ключе не ровно один номер; `ct.choiceN` и в ключе меньше двух номеров. Целое в ключе choice считается одним номером |
| `trap_missing` | у опции, чей `n` не входит в ключ, нет `trap_id`, либо этого id нет среди `traps` варианта, либо у ловушки пустое `description`. У верной опции ловушка не требуется |
| `minutes_out_of_range` | `minutes` варианта не целое от 1 до 60 включительно. Для этого указателя нет второго кода `schema_invalid` |
| `pool_too_small` | тип упомянут в шаблоне урока (практика или ряд рецепта) и активных вариантов меньше 5. `retired` не считается. Тип вне уроков не проверяется. 5 активных и несколько retired — норма |
| `reinforcement_impossible` | подвариант упомянут в `practice` урока и активных вариантов этого подварианта меньше 2 |

Один указатель одного поля — один главный код из этой таблицы, без второго `schema_invalid` на том же указателе.

## Тесты

Только `content/tests/test_answers.py`. На каждый код минимум один тест. Имя содержит код (`test_ar_r16_key_not_integer`). Демо-каталог копируется, правка минимальная. Неизменённая копия этих кодов не даёт. Не читать реализацию заранее: модуль можно импортировать, тесты красные, пока проверки нет. `pytest --collect-only` проходит.

## Ограничения

- Не менять `content/tests/test_check_catalog.py`, `test_cli.py`, `test_history.py`.
- Не делать `key_mismatch`, подпись, сеть, git commit.
- Не менять docs и контракты.

## Критерий готовности

Интерпретатор `/home/stranger/projects/university/coursework/ai-mentor/.venv/bin/python`. Из `content`: `python -m pytest -q` целиком зелёный.
