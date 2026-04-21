#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${REPO_ROOT}/.env"
ENV_EXAMPLE="${REPO_ROOT}/.env.example"
HEALTH_URL="http://127.0.0.1:8080/healthz"
HEALTH_TIMEOUT_SECONDS=90
SKIP_HEALTH_CHECK=0
DRY_RUN=0
NO_BUILD=0

usage() {
  cat <<'EOF'
Bootstrap the local TeamPulse Bridge stack with Docker Compose.

Usage:
  bash ./scripts/install.sh [options]

Options:
  --skip-health-check    Start the stack without waiting for /healthz
  --timeout <seconds>    Override the health-check timeout (default: 90)
  --no-build             Skip docker compose --build
  --dry-run              Print the planned actions without executing them
  --help                 Show this help text
EOF
}

log_step() {
  printf '\n== %s ==\n' "$1"
}

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

render_command() {
  local rendered=""
  local arg
  for arg in "$@"; do
    if [[ -n "${rendered}" ]]; then
      rendered+=" "
    fi
    rendered+="$(printf '%q' "${arg}")"
  done
  printf '%s' "${rendered}"
}

run() {
  if (( DRY_RUN )); then
    printf '[dry-run] %s\n' "$(render_command "$@")"
    return 0
  fi
  "$@"
}

run_in_repo() {
  if (( DRY_RUN )); then
    printf '[dry-run] (cd %s && %s)\n' "$(printf '%q' "${REPO_ROOT}")" "$(render_command "$@")"
    return 0
  fi
  (
    cd "${REPO_ROOT}"
    "$@"
  )
}

have_command() {
  command -v "$1" >/dev/null 2>&1
}

check_prerequisites() {
  log_step "Checking prerequisites"

  if (( DRY_RUN )); then
    printf '[dry-run] would verify Docker CLI, Docker Compose plugin, and Docker daemon availability\n'
    return 0
  fi

  have_command docker || fail "docker was not found. Install Docker Desktop or Docker Engine first."
  docker compose version >/dev/null 2>&1 || fail "docker compose is not available. Install Docker Compose v2."
  docker info >/dev/null 2>&1 || fail "Docker daemon is not reachable. Start Docker and retry."
}

prepare_env() {
  log_step "Preparing local environment"

  if [[ -f "${ENV_FILE}" ]]; then
    printf '.env already exists; keeping your current local settings.\n'
    return 0
  fi

  [[ -f "${ENV_EXAMPLE}" ]] || fail ".env.example was not found at the repository root."

  if (( DRY_RUN )); then
    printf '[dry-run] would create %s from %s\n' "${ENV_FILE}" "${ENV_EXAMPLE}"
    return 0
  fi

  run cp "${ENV_EXAMPLE}" "${ENV_FILE}"
  printf 'Created .env from .env.example\n'
}

start_stack() {
  log_step "Starting local stack"

  local compose_args=(docker compose up -d)
  if (( NO_BUILD == 0 )); then
    compose_args+=(--build)
  fi

  run_in_repo "${compose_args[@]}"
}

health_check_ready() {
  if have_command curl; then
    curl -fsS "${HEALTH_URL}" >/dev/null 2>&1
    return $?
  fi
  if have_command wget; then
    wget -q -O - "${HEALTH_URL}" >/dev/null 2>&1
    return $?
  fi
  fail "curl or wget is required for health checks. Re-run with --skip-health-check if you only want to start the containers."
}

wait_for_health() {
  if (( SKIP_HEALTH_CHECK )); then
    log_step "Skipping health check"
    return 0
  fi

  log_step "Waiting for ingestion gateway readiness"

  if (( DRY_RUN )); then
    printf '[dry-run] would poll %s for up to %s seconds\n' "${HEALTH_URL}" "${HEALTH_TIMEOUT_SECONDS}"
    return 0
  fi

  local deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
  while (( SECONDS < deadline )); do
    if health_check_ready; then
      printf 'ingestion-gateway is healthy at %s\n' "${HEALTH_URL}"
      return 0
    fi
    sleep 3
  done

  printf '\nThe stack did not become healthy within %s seconds.\n' "${HEALTH_TIMEOUT_SECONDS}" >&2
  printf 'Leaving containers running so you can inspect them.\n' >&2
  printf '\nCurrent compose status:\n' >&2
  run_in_repo docker compose ps >&2 || true
  printf '\nRecent ingestion-gateway logs:\n' >&2
  run_in_repo docker compose logs --no-color --tail=120 ingestion-gateway >&2 || true
  exit 1
}

print_success() {
  printf '\nTeamPulse Bridge is ready.\n'
  printf '  Operator UI: %s\n' "http://127.0.0.1:8080/"
  printf '  Health:      %s\n' "${HEALTH_URL}"
  printf '  Metrics:     %s\n' "http://127.0.0.1:8080/metrics"
  printf '  Prometheus:  %s\n' "http://127.0.0.1:9090"
  printf '  Grafana:     %s\n' "http://127.0.0.1:3000"
  printf '\nStop the stack with:\n'
  printf '  docker compose down -v\n'
}

while (($# > 0)); do
  case "$1" in
    --skip-health-check)
      SKIP_HEALTH_CHECK=1
      ;;
    --timeout)
      shift
      [[ $# -gt 0 ]] || fail "--timeout requires a value in seconds."
      [[ "$1" =~ ^[0-9]+$ ]] || fail "--timeout must be an integer number of seconds."
      HEALTH_TIMEOUT_SECONDS="$1"
      ;;
    --no-build)
      NO_BUILD=1
      ;;
    --dry-run)
      DRY_RUN=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      fail "Unknown option: $1. Use --help for usage."
      ;;
  esac
  shift
done

check_prerequisites
prepare_env
start_stack
wait_for_health
print_success
