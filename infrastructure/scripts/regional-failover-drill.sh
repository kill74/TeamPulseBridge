#!/usr/bin/env bash
set -euo pipefail

PRIMARY_HEALTH_URL="${PRIMARY_HEALTH_URL:-}"
SECONDARY_HEALTH_URL="${SECONDARY_HEALTH_URL:-}"
FAILOVER_HEALTH_URL="${FAILOVER_HEALTH_URL:-$SECONDARY_HEALTH_URL}"
FAIL_ACTION="${FAIL_ACTION:-}"
RECOVERY_ACTION="${RECOVERY_ACTION:-}"
RTO_SLO_SECONDS="${RTO_SLO_SECONDS:-300}"
RECOVERY_SLO_SECONDS="${RECOVERY_SLO_SECONDS:-600}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-5}"
FAILOVER_TIMEOUT_SECONDS="${FAILOVER_TIMEOUT_SECONDS:-900}"
RECOVERY_TIMEOUT_SECONDS="${RECOVERY_TIMEOUT_SECONDS:-900}"

require_env() {
  local name="$1"
  local value="$2"
  if [[ -z "$value" ]]; then
    echo "Missing required environment variable: $name" >&2
    exit 1
  fi
}

now_epoch() {
  date +%s
}

healthcheck() {
  local url="$1"
  curl -fsS --max-time 5 "$url" >/dev/null
}

wait_for_healthy() {
  local url="$1"
  local timeout="$2"
  local start_ts
  start_ts="$(now_epoch)"

  while true; do
    if healthcheck "$url"; then
      local end_ts
      end_ts="$(now_epoch)"
      echo $((end_ts - start_ts))
      return 0
    fi

    local now_ts
    now_ts="$(now_epoch)"
    if (( now_ts - start_ts >= timeout )); then
      return 1
    fi
    sleep "$POLL_INTERVAL_SECONDS"
  done
}

run_action() {
  local label="$1"
  local action="$2"
  if [[ -z "$action" ]]; then
    echo "Skipping $label action; no command provided."
    return 0
  fi
  echo "Running $label action:"
  echo "  $action"
  bash -lc "$action"
}

require_env "PRIMARY_HEALTH_URL" "$PRIMARY_HEALTH_URL"
require_env "SECONDARY_HEALTH_URL" "$SECONDARY_HEALTH_URL"

echo "== Regional Failover Drill =="
echo "Primary health URL:   $PRIMARY_HEALTH_URL"
echo "Secondary health URL: $SECONDARY_HEALTH_URL"
echo "Failover health URL:  $FAILOVER_HEALTH_URL"
echo "RTO SLO (seconds):    $RTO_SLO_SECONDS"
echo "Recovery SLO (sec):   $RECOVERY_SLO_SECONDS"
echo ""

echo "Pre-flight checks:"
healthcheck "$PRIMARY_HEALTH_URL"
echo "  [OK] primary endpoint is healthy"
healthcheck "$SECONDARY_HEALTH_URL"
echo "  [OK] secondary endpoint is healthy"

run_action "failover" "$FAIL_ACTION"

echo ""
echo "Waiting for failover target to become healthy..."
failover_rto="$(wait_for_healthy "$FAILOVER_HEALTH_URL" "$FAILOVER_TIMEOUT_SECONDS")" || {
  echo "Failover target did not become healthy within ${FAILOVER_TIMEOUT_SECONDS}s" >&2
  exit 1
}
echo "  [OK] failover target recovered in ${failover_rto}s"

if (( failover_rto > RTO_SLO_SECONDS )); then
  echo "Failover RTO breached the SLO (${failover_rto}s > ${RTO_SLO_SECONDS}s)" >&2
  exit 1
fi

run_action "recovery" "$RECOVERY_ACTION"

if [[ -n "$RECOVERY_ACTION" ]]; then
  echo ""
  echo "Waiting for primary endpoint to recover..."
  recovery_time="$(wait_for_healthy "$PRIMARY_HEALTH_URL" "$RECOVERY_TIMEOUT_SECONDS")" || {
    echo "Primary endpoint did not recover within ${RECOVERY_TIMEOUT_SECONDS}s" >&2
    exit 1
  }
  echo "  [OK] primary endpoint recovered in ${recovery_time}s"

  if (( recovery_time > RECOVERY_SLO_SECONDS )); then
    echo "Primary recovery breached the SLO (${recovery_time}s > ${RECOVERY_SLO_SECONDS}s)" >&2
    exit 1
  fi
fi

echo ""
echo "Failover drill completed successfully."
