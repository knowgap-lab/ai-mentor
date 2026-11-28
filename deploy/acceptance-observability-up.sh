#!/usr/bin/env bash
# Поднять стек наблюдаемости на гостевой приёмке. Только loopback.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
exec docker compose -f deploy/compose.observability.yml up -d --pull always