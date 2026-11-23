#!/usr/bin/env bash
# Regenerate committed contract types under gen/ (D97, D300, D310, D333).
# Usage from repo root: bash tools/generate.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DCG="$ROOT/.venv/bin/datamodel-codegen"
if [[ ! -x "$DCG" ]]; then
  echo "datamodel-code-generator не найден: нужен $ROOT/.venv (tools/dev-install.sh или pip install -r tools/requirements.txt)" >&2
  exit 1
fi

mkdir -p gen/go/coreapi gen/go/internalapi gen/python gen/ts

# Go (oapi-codegen v2.8.2, go tool): configs in gen/oapi-codegen/, outputs under gen/go/
(
  cd core
  go tool oapi-codegen -config ../gen/oapi-codegen/core-api.yaml ../docs/contracts/core-api.yaml
  go tool oapi-codegen -config ../gen/oapi-codegen/internal-api.yaml ../docs/contracts/internal-api.yaml
)

# Python (datamodel-code-generator)
DCG_COMMON=(--formatters builtin --disable-timestamp --output-model-type pydantic_v2.BaseModel)
"$DCG" "${DCG_COMMON[@]}" \
  --input "$ROOT/docs/contracts/core-api.yaml" \
  --input-file-type openapi \
  --openapi-scopes schemas \
  --output "$ROOT/gen/python/core_api_models.py"
"$DCG" "${DCG_COMMON[@]}" \
  --input "$ROOT/docs/contracts/internal-api.yaml" \
  --input-file-type openapi \
  --openapi-scopes schemas \
  --output "$ROOT/gen/python/internal_api_models.py"
for schema in catalog events queue subject-package; do
  "$DCG" "${DCG_COMMON[@]}" \
    --input "$ROOT/docs/contracts/${schema}.schema.json" \
    --input-file-type jsonschema \
    --output "$ROOT/gen/python/${schema//-/_}_models.py"
done

# TypeScript (openapi-typescript, json-schema-to-typescript via web devDependencies)
if [[ ! -x "$ROOT/web/node_modules/.bin/openapi-typescript" ]]; then
  echo "openapi-typescript не найден: выполните npm ci в web/" >&2
  exit 1
fi

OT="$ROOT/web/node_modules/.bin/openapi-typescript"
J2TS="$ROOT/web/node_modules/.bin/json2ts"

"$OT" "$ROOT/docs/contracts/core-api.yaml" -o "$ROOT/gen/ts/core-api.d.ts"
"$OT" "$ROOT/docs/contracts/internal-api.yaml" -o "$ROOT/gen/ts/internal-api.d.ts"

(
  cd "$ROOT/docs/contracts"
  for schema in catalog events queue subject-package; do
    "$J2TS" -i "${schema}.schema.json" -o "$ROOT/gen/ts/${schema}.schema.d.ts" --format false --cwd .
  done
)