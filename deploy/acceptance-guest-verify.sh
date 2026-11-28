#!/usr/bin/env bash
# Проверки на гостевой Ubuntu; печатает vm:* и exit 0 только если обязательные проверки прошли.
set -eo pipefail

INSTALL_ROOT="${INSTALL_ROOT:-/opt/ai-mentor}"
STATE_ROOT="${STATE_ROOT:-/etc/ai-mentor}"
STACK="${STACK:-prod}"
SRC="${INSTALL_ROOT}/src"
ENV_FILE="${STATE_ROOT}/${STACK}/env"
SECRET_FILE="${STATE_ROOT}/${STACK}/secret.env"
OVERALL=0

vm_field() {
  echo "vm:${1}=${2}"
}

have_stack_secrets() {
  [[ -f "$ENV_FILE" && -f "$SECRET_FILE" ]]
}

dc_prod() {
  docker compose \
    --project-directory "$SRC" \
    --env-file "$ENV_FILE" \
    --env-file "$SECRET_FILE" \
    -f "${SRC}/deploy/compose.prod.yml" \
    "$@"
}

check_reboot_services() {
  if [[ "${ACCEPTANCE_AFTER_REBOOT:-}" != "1" ]]; then
    vm_field reboot.services fail
    return 1
  fi
  if ! have_stack_secrets; then
    vm_field reboot.services fail
    return 1
  fi
  local running=0
  for svc in db gotenberg; do
    if dc_prod ps --status running -q "$svc" 2>/dev/null | grep -q .; then
      running=$((running + 1))
    fi
  done
  if [[ "$running" -ge 2 ]]; then
    vm_field reboot.services pass
    return 0
  fi
  vm_field reboot.services fail
  return 1
}

check_reboot_readiness() {
  if [[ "${ACCEPTANCE_AFTER_REBOOT:-}" != "1" ]]; then
    vm_field reboot.readiness fail
    return 1
  fi
  if ! have_stack_secrets; then
    vm_field reboot.readiness fail
    return 1
  fi
  local cid
  cid="$(dc_prod ps -q core 2>/dev/null | head -1)"
  if [[ -z "$cid" ]]; then
    vm_field reboot.readiness fail
    return 1
  fi
  if docker exec "$cid" wget -qO- http://127.0.0.1:8080/healthz 2>/dev/null | grep -qx ok; then
    vm_field reboot.readiness pass
    return 0
  fi
  vm_field reboot.readiness fail
  return 1
}

write_reboot_marker() {
  if ! have_stack_secrets; then
    return 1
  fi
  local token
  token="$(date -u +%s)-$$"
  dc_prod exec -T db psql -U ai_mentor -d ai_mentor -v ON_ERROR_STOP=1 <<-SQL
CREATE TABLE IF NOT EXISTS acceptance_reboot_marker (
  token text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
TRUNCATE acceptance_reboot_marker;
INSERT INTO acceptance_reboot_marker (token) VALUES ('$token');
SQL
  printf '%s' "$token" >"${STATE_ROOT}/${STACK}/.acceptance_reboot_marker"
  chmod 600 "${STATE_ROOT}/${STACK}/.acceptance_reboot_marker"
}

check_reboot_data() {
  if [[ "${ACCEPTANCE_AFTER_REBOOT:-}" != "1" ]]; then
    vm_field reboot.data not_persisted
    return 1
  fi
  if ! have_stack_secrets; then
    vm_field reboot.data not_persisted
    return 1
  fi
  local marker="${STATE_ROOT}/${STACK}/.acceptance_reboot_marker"
  if [[ ! -f "$marker" ]]; then
    vm_field reboot.data not_persisted
    return 1
  fi
  local before after
  before="$(cat "$marker")"
  after="$(dc_prod exec -T db psql -U ai_mentor -d ai_mentor -Atqc \
    "SELECT COUNT(*) FROM acceptance_reboot_marker WHERE token='$before'" 2>/dev/null || true)"
  if [[ "$after" == "1" ]]; then
    vm_field reboot.data persisted
    return 0
  fi
  vm_field reboot.data not_persisted
  return 1
}

check_background_job() {
  if sudo systemctl start ai-mentor-log-cleanup.service 2>/dev/null; then
    vm_field background_job pass
    return 0
  fi
  vm_field background_job fail
  return 1
}

gotenberg_pdf_once() {
  local cid="$1"
  local out="$2"
  if ! docker exec "$cid" curl -sf -F 'files=@/tmp/acceptance.html' \
    http://127.0.0.1:3000/forms/chromium/convert/html >"$out"; then
    return 1
  fi
  if [[ ! -s "$out" ]]; then
    return 1
  fi
  if ! head -c 5 "$out" | grep -q '%PDF-'; then
    return 1
  fi
  return 0
}

check_pdf_gotenberg() {
  local cid
  cid="$(dc_prod ps -q gotenberg 2>/dev/null | head -1)"
  if [[ -z "$cid" ]]; then
    vm_field pdf.gotenberg fail
    vm_field pdf.bytes_stable false
    return 1
  fi
  if ! docker exec "$cid" wget -qO- http://127.0.0.1:3000/health 2>/dev/null | grep -q .; then
    vm_field pdf.gotenberg fail
    vm_field pdf.bytes_stable false
    return 1
  fi
  vm_field pdf.gotenberg real
  local work sha1 sha2
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' RETURN
  printf '%s\n' '<html><body><p>acceptance</p></body></html>' >"$work/in.html"
  docker cp "$work/in.html" "$cid:/tmp/acceptance.html"
  if ! docker exec "$cid" sh -c 'command -v curl >/dev/null'; then
    vm_field pdf.bytes_stable false
    return 1
  fi
  if ! gotenberg_pdf_once "$cid" "$work/one.pdf"; then
    vm_field pdf.bytes_stable false
    return 1
  fi
  if ! gotenberg_pdf_once "$cid" "$work/two.pdf"; then
    vm_field pdf.bytes_stable false
    return 1
  fi
  sha1="$(sha256sum "$work/one.pdf" | awk '{print $1}')"
  sha2="$(sha256sum "$work/two.pdf" | awk '{print $1}')"
  if [[ -n "$sha1" && "$sha1" == "$sha2" ]]; then
    vm_field pdf.bytes_stable true
    return 0
  fi
  vm_field pdf.bytes_stable false
  return 1
}

check_observability_stack() {
  local ok=1
  cd "$SRC"
  if ! docker compose -f deploy/compose.observability.yml ps --status running 2>/dev/null | grep -q prometheus; then
    vm_field telemetry.logs fail
    vm_field telemetry.traces fail
    vm_field telemetry.prometheus fail
    vm_field telemetry.grafana fail
    vm_field telemetry.alert_rule fail
    return 1
  fi
  if bash deploy/acceptance-otel-probe.sh; then
    vm_field telemetry.logs pass
    vm_field telemetry.traces pass
  else
    vm_field telemetry.logs fail
    vm_field telemetry.traces fail
    ok=0
  fi
  if curl -sf http://127.0.0.1:9090/-/ready >/dev/null && \
    curl -sf 'http://127.0.0.1:9090/api/v1/query?query=up' | grep -q otel; then
    vm_field telemetry.prometheus pass
  else
    vm_field telemetry.prometheus fail
    ok=0
  fi
  if curl -sf http://127.0.0.1:3030/api/health | grep -q ok; then
    vm_field telemetry.grafana pass
  else
    vm_field telemetry.grafana fail
    ok=0
  fi
  if docker run --rm \
    -v "${SRC}/deploy/observability/prometheus-rules.yml:/rules.yml:ro" \
    prom/prometheus:v2.54.1 \
    promtool check rules /rules.yml >/dev/null 2>&1 && \
    curl -sf 'http://127.0.0.1:9090/api/v1/rules' | grep -q AcceptanceOtelCollectorDown; then
    vm_field telemetry.alert_rule pass
  else
    vm_field telemetry.alert_rule fail
    ok=0
  fi
  if [[ "$ok" -eq 1 ]]; then
    return 0
  fi
  return 1
}

check_telemetry_pii() {
  if journalctl -u 'ai-mentor*' --no-pager -n 200 2>/dev/null | \
    python3 -c 'import re,sys; sys.exit(1 if re.search(r"(?i)(telegram_id|birth_date|first_name)", sys.stdin.read()) else 0)'; then
    vm_field telemetry.pii absent
    return 0
  fi
  vm_field telemetry.pii present
  return 1
}

check_restore_drill() {
  if [[ -x "${SRC}/deploy/acceptance-backup-drill.sh" ]]; then
    if (cd "$SRC" && STACK="$STACK" STATE_ROOT="$STATE_ROOT" bash deploy/acceptance-backup-drill.sh); then
      vm_field restore.separate_environment true
      vm_field restore.records_compared true
      vm_field restore.backup_file_only false
      return 0
    fi
  fi
  vm_field restore.separate_environment false
  vm_field restore.records_compared false
  vm_field restore.backup_file_only true
  return 1
}

emit_telegram_waiting() {
  vm_field telegram.login waiting
  vm_field telegram.device_confirmation waiting
  vm_field telegram.notification waiting
}

run_reboot_checks_only() {
  emit_telegram_waiting
  check_reboot_services || OVERALL=1
  check_reboot_readiness || OVERALL=1
  check_reboot_data || OVERALL=1
  return "$OVERALL"
}

run_acceptance_verify() {
  emit_telegram_waiting
  check_telemetry_pii || OVERALL=1
  check_background_job || OVERALL=1
  check_pdf_gotenberg || OVERALL=1
  check_observability_stack || OVERALL=1
  check_restore_drill || OVERALL=1
  return "$OVERALL"
}

if [[ "${ACCEPTANCE_WRITE_REBOOT_MARKER:-}" == "1" ]]; then
  if write_reboot_marker; then
    echo "acceptance: reboot marker written"
    exit 0
  fi
  echo "acceptance: reboot marker failed" >&2
  exit 1
fi

if [[ "${ACCEPTANCE_REBOOT_CHECKS_ONLY:-}" == "1" ]]; then
  run_reboot_checks_only
  exit $?
fi

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_acceptance_verify
  exit $?
fi