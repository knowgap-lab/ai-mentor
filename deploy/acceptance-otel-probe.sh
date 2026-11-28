#!/usr/bin/env bash
# OTLP probe via telemetrygen в сеть observability; без успешной записи в Tempo/Loki — код 1.
set -euo pipefail

SERVICE_NAME="${ACCEPTANCE_OTEL_SERVICE:-acceptance-probe}"
NETWORK="${ACCEPTANCE_OBSERVABILITY_NETWORK:-ai-mentor-observability}"
COLLECTOR_OTLP="${ACCEPTANCE_OTEL_COLLECTOR:-otel-collector:4317}"
GEN_IMAGE="${ACCEPTANCE_TELEMETRYGEN_IMAGE:-ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:0.111.0}"

if ! docker network inspect "$NETWORK" >/dev/null 2>&1; then
  echo "acceptance-otel-probe: network $NETWORK missing" >&2
  exit 1
fi

docker run --rm --network "$NETWORK" "$GEN_IMAGE" \
  traces --otlp-endpoint "$COLLECTOR_OTLP" --traces 1 --service "$SERVICE_NAME"
docker run --rm --network "$NETWORK" "$GEN_IMAGE" \
  logs --otlp-endpoint "$COLLECTOR_OTLP" --logs 1 --service "$SERVICE_NAME"

for attempt in $(seq 1 15); do
  trace_ok=0
  log_ok=0
  if curl -sf -G "http://127.0.0.1:3200/api/search" \
    --data-urlencode "tags=service.name=${SERVICE_NAME}" | grep -q traceID; then
    trace_ok=1
  fi
  if curl -sf -G "http://127.0.0.1:3100/loki/api/v1/query_range" \
    --data-urlencode 'query={}' \
    --data-urlencode "limit=50" | grep -q "$SERVICE_NAME"; then
    log_ok=1
  fi
  if [[ "$trace_ok" -eq 1 && "$log_ok" -eq 1 ]]; then
    echo "acceptance-otel-probe: trace and log present"
    exit 0
  fi
  sleep 2
done
echo "acceptance-otel-probe: no trace in Tempo or log in Loki" >&2
exit 1