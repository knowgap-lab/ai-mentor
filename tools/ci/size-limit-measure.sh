#!/usr/bin/env bash
# D315: size-limit в CI. Бюджет только из зафиксированного файла (не выдумывать число).
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
web="$root/web"
budget_file="$web/size-limit.budget.json"
dist="$web/dist"

if [[ ! -f "$budget_file" ]]; then
  echo "Нет зафиксированного бюджета: $budget_file (D315 требует size-limit, числовой лимит ещё не записан в репозитории)."
  exit 1
fi

cd "$web"
npm run build

total=0
while IFS= read -r -d '' f; do
  size=$(gzip -c "$f" | wc -c | tr -d ' ')
  total=$((total + size))
done < <(find "$dist/assets" -name '*.js' -print0 2>/dev/null)

python3 - "$budget_file" "$total" <<'PY'
import json, sys
path, measured = sys.argv[1], int(sys.argv[2])
with open(path) as f:
    budget = json.load(f)
limit = int(budget["maxGzipJsBytes"])
print(f"gzip JS total: {measured} bytes (budget {limit})")
if measured > limit:
    print(f"over budget by {measured - limit} bytes", file=sys.stderr)
    raise SystemExit(1)
PY