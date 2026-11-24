#!/usr/bin/env bash
# Коммит под тестом: RELEASE_SHA или git rev-parse HEAD^{commit}.
# Если в release.json или d326-profile.json есть app_sha/sha, они обязаны совпасть — иначе код 1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHA="${RELEASE_SHA:-}"
if [[ -z "$SHA" ]]; then
  SHA="$(git -C "$ROOT" rev-parse HEAD^{commit})"
fi
if ! [[ "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  echo "verify_release_sha: invalid SHA: $SHA" >&2
  exit 1
fi
export RELEASE_SHA="$SHA"
export TF_VAR_app_sha="$SHA"

python3 - "$ROOT" "$SHA" <<'PY'
import json, sys
from pathlib import Path

root = Path(sys.argv[1])
head = sys.argv[2]
problems: list[str] = []

def check_app_sha(path: Path, key: str) -> None:
    if not path.is_file():
        return
    data = json.loads(path.read_text(encoding="utf-8"))
    if key not in data:
        return
    pinned = data.get(key)
    if pinned != head:
        problems.append(f"{path}: {key}={pinned} != HEAD {head}")

check_app_sha(root / "infra/terraform/release.json", "app_sha")
check_app_sha(root / "infra/ansible/release.json", "app_sha")
prof = root / "tools/load/d326-profile.json"
if prof.is_file():
    data = json.loads(prof.read_text(encoding="utf-8"))
    if "sha" in data and data.get("sha") != head:
        problems.append(f"{prof}: sha={data.get('sha')} != HEAD {head}")

if problems:
    for line in problems:
        print(line, file=sys.stderr)
    sys.exit(1)
print(f"verify_release_sha: HEAD={head}")
PY