#!/usr/bin/env bash
# Синхронизирует app_sha во всех install/load артефактах с проверяемым коммитом.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHA="${RELEASE_SHA:-}"
if [[ -z "$SHA" ]]; then
  SHA="$(git -C "$ROOT" rev-parse HEAD^{commit})"
fi
if ! [[ "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  echo "pin_release_sha: invalid SHA: $SHA" >&2
  exit 1
fi
export RELEASE_SHA="$SHA"
python3 - "$ROOT" "$SHA" <<'PY'
import json, sys
from pathlib import Path

root = Path(sys.argv[1])
sha = sys.argv[2]
for rel in ("infra/terraform/release.json", "infra/ansible/release.json"):
    path = root / rel
    data = json.loads(path.read_text(encoding="utf-8"))
    data["app_sha"] = sha
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
profile = root / "tools/load/d326-profile.json"
prof = json.loads(profile.read_text(encoding="utf-8"))
prof["sha"] = sha
profile.write_text(json.dumps(prof, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"pin_release_sha: app_sha={sha}")
PY