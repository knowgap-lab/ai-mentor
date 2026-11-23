# Сгенерированные типы

Сгенерированные типы коммитятся. CI сверяет перегенерацию командой `bash tools/generate.sh` (D330).

| Каталог | Генератор | Источник |
|---|---|---|
| `go/coreapi/`, `go/internalapi/` | oapi-codegen (strict chi server, D310) | `docs/contracts/*-api.yaml` |
| `python/` | datamodel-code-generator (D300) | контракты README, кроме `compute-views` |
| `ts/` | openapi-typescript, json-schema-to-typescript (D300) | те же файлы |

Конфигурация oapi-codegen: `gen/oapi-codegen/`. `compute-views.schema.json` не генерируется (выпуск 3).