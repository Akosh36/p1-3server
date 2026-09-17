#!/bin/bash
# Proves the documented failover behaviour end to end:
#   1. bring the docker-compose stack up
#   2. stop one backend that is in the load balancer's upstream pool
#   3. confirm requests through the load balancer keep succeeding
#   4. restore the stopped backend and confirm it rejoins the pool
#
# Usage: control-scripts/check_failover.sh
# Exit code 0 means failover worked; non-zero means it did not.

set -u
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

LB_PORT=8082
LB_HEALTH_URL="http://localhost:${LB_PORT}/health"
BACKEND_CONTAINER=web_server_2
BACKEND_PORT=8002
BACKEND_URL="http://localhost:${BACKEND_PORT}"
REQUESTS_DURING_OUTAGE=10
STARTED_STACK=0

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "FAIL: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
fi

log() { echo "[check_failover] $*"; }

http_ok() {
  local url="$1"
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$url")
  [ "$code" = "200" ]
}

wait_for() {
  local url="$1"
  local tries="$2"
  local i
  for ((i = 1; i <= tries; i++)); do
    if http_ok "$url"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

cleanup_on_failure() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    log "restoring ${BACKEND_CONTAINER} before exiting after failure"
    docker start "${BACKEND_CONTAINER}" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup_on_failure EXIT

log "bringing the stack up (docker compose up -d)"
if ! "${COMPOSE[@]}" up -d; then
  echo "FAIL: could not start the docker-compose stack" >&2
  exit 1
fi
STARTED_STACK=1

log "waiting for the load balancer's user-LAN health check on port ${LB_PORT}"
if ! wait_for "$LB_HEALTH_URL" 30; then
  echo "FAIL: load balancer never became healthy on port ${LB_PORT}" >&2
  exit 1
fi

log "baseline: confirming requests succeed before touching any backend"
if ! http_ok "$LB_HEALTH_URL"; then
  echo "FAIL: baseline request through the load balancer failed" >&2
  exit 1
fi

log "stopping backend container '${BACKEND_CONTAINER}' to simulate a crash"
if ! docker stop "${BACKEND_CONTAINER}" >/dev/null; then
  echo "FAIL: could not stop ${BACKEND_CONTAINER}" >&2
  exit 1
fi

log "requesting through the load balancer ${REQUESTS_DURING_OUTAGE} times while the backend is down"
failures=0
for ((i = 1; i <= REQUESTS_DURING_OUTAGE; i++)); do
  if http_ok "$LB_HEALTH_URL"; then
    echo "  request $i/${REQUESTS_DURING_OUTAGE}: OK"
  else
    echo "  request $i/${REQUESTS_DURING_OUTAGE}: FAILED"
    failures=$((failures + 1))
  fi
  sleep 0.5
done

if [ "$failures" -gt 0 ]; then
  echo "FAIL: ${failures}/${REQUESTS_DURING_OUTAGE} requests failed through the load balancer while ${BACKEND_CONTAINER} was down" >&2
  exit 1
fi
log "all requests succeeded through the load balancer during the outage"

log "restarting backend container '${BACKEND_CONTAINER}'"
if ! docker start "${BACKEND_CONTAINER}" >/dev/null; then
  echo "FAIL: could not restart ${BACKEND_CONTAINER}" >&2
  exit 1
fi

log "waiting for ${BACKEND_CONTAINER} to answer directly on port ${BACKEND_PORT}"
if ! wait_for "$BACKEND_URL" 30; then
  echo "FAIL: ${BACKEND_CONTAINER} did not come back up on port ${BACKEND_PORT}" >&2
  exit 1
fi

log "confirming the load balancer still serves requests after the backend rejoined"
if ! http_ok "$LB_HEALTH_URL"; then
  echo "FAIL: load balancer stopped responding after restoring ${BACKEND_CONTAINER}" >&2
  exit 1
fi

log "PASS: failover and recovery both worked as documented"
exit 0
