#!/usr/bin/env bash
# E3 live API на rootHandler и Postgres (core/cmd/server/contract_live_test.go).
set -euo pipefail
cd "$(dirname "$0")/../.."
(cd core && go test -count=1 -timeout 10m ./cmd/server/ -run '^TestContractLiveE3$')