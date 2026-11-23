#!/usr/bin/env bash
# D315: сравнить gzip-сумму production JS с web/size-limit.budget.json (база измерена в PR, D328).
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
web="$root/web"
budget_file="$web/size-limit.budget.json"
dist="$web/dist"

if [[ ! -f "$budget_file" ]]; then
  echo "Нет $budget_file"
  exit 1
fi

cd "$web"
npm run build

mapfile -d '' js_files < <(find "$dist/assets" -name '*.js' -print0 2>/dev/null || true)
if (( ${#js_files[@]} == 0 )); then
  echo "Нет JS в $dist/assets"
  exit 1
fi

total=0
for f in "${js_files[@]}"; do
  size=$(gzip -c "$f" | wc -c | tr -d ' ')
  rel="${f#"$root"/}"
  echo "$rel gzip=${size}"
  total=$((total + size))
done

python3 - "$budget_file" "$total" <<'PY'
import json, sys
path, measured = sys.argv[1], int(sys.argv[2])
with open(path) as f:
    budget = json.load(f)
limit = int(budget["maxGzipJsBytes"])
unit = budget.get("unit", "gzip_bytes")
print(f"total {unit}: {measured} (budget {limit}, sha baseline {budget.get('measuredAtSha', '?')})")
if measured > limit:
    print(f"over budget by {measured - limit} {unit}", file=sys.stderr)
    raise SystemExit(1)
PY