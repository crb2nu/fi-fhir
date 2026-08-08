#!/usr/bin/env bash
# smoke-test.sh — Verify core fi-fhir runtime endpoint reachability.
#
# Usage:
#   GRAPHQL_BEARER_TOKEN=... BASE_URL=http://localhost:8080 bash scripts/smoke-test.sh
#
# Checks:
#   1. GET  /health             → HTTP 200, body names the components it checked
#   2. GET  /ready              → HTTP 200 or 503, and the code agrees with the
#                                 component states in the body
#   3. POST /graphql            → HTTP 200, health query projects real components
#   4. GET  /graphql/ws (probe) → rejected; preview has no WS transport
#   5. GET  <metrics>/metrics   → HTTP 200, Prometheus exposition
#
# Check 1 asserted only that the body contained the substring "status" before
# Slice 4.3, which the hardcoded literal `{"status":"healthy","service":"graphql"}`
# satisfied forever. It now requires the component projection, so a regression to
# a literal fails here.
#
# Exit codes:
#   0  All checks passed
#   1  One or more checks failed

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TIMEOUT="${TIMEOUT:-5}"
RETRIES="${RETRIES:-3}"
RETRY_DELAY="${RETRY_DELAY:-2}"
CURL_BIN="${CURL_BIN:-curl}"
GRAPHQL_BEARER_TOKEN="${GRAPHQL_BEARER_TOKEN:-}"
GRAPHQL_ALLOWED_ORIGIN="${GRAPHQL_ALLOWED_ORIGIN:-http://localhost:5173}"
METRICS_URL="${METRICS_URL:-http://localhost:9090}"

if [ -z "$GRAPHQL_BEARER_TOKEN" ]; then
  echo "GRAPHQL_BEARER_TOKEN is required for authenticated smoke checks" >&2
  exit 1
fi

passed=0
failed=0

check() {
  local name="$1"
  shift
  echo -n "  [$name] ... "
  if "$@"; then
    echo "✓"
    passed=$((passed + 1))
  else
    echo "✗"
    failed=$((failed + 1))
  fi
}

retry_curl() {
  local attempt=1
  while [ "$attempt" -le "$RETRIES" ]; do
    if "$CURL_BIN" "$@" 2>/dev/null; then
      return 0
    fi
    attempt=$((attempt + 1))
    [ "$attempt" -le "$RETRIES" ] && sleep "$RETRY_DELAY"
  done
  return 1
}

check_health() {
  local body
  body=$(retry_curl -sf --max-time "$TIMEOUT" "$BASE_URL/health") || return 1
  printf '%s' "$body" | grep -q '"status"' || return 1
  # A literal cannot satisfy this: the probe must name what it checked.
  printf '%s' "$body" | grep -q '"components"'
}

check_ready() {
  local status body
  status=$("$CURL_BIN" -s -o /dev/null -w '%{http_code}' --max-time "$TIMEOUT" "$BASE_URL/ready" || true)
  if [ "$status" != "200" ] && [ "$status" != "503" ]; then
    echo "Unexpected /ready status: $status"
    return 1
  fi
  body=$("$CURL_BIN" -s --max-time "$TIMEOUT" "$BASE_URL/ready" || true)
  printf '%s' "$body" | grep -q '"components"' || return 1
  # The status code must be explained by the body: 503 requires an unhealthy
  # component, 200 forbids one. Anything else means readiness aggregates a lie.
  if [ "$status" = "503" ]; then
    printf '%s' "$body" | grep -q '"unhealthy"' || {
      echo "/ready returned 503 with no unhealthy component"
      return 1
    }
  else
    if printf '%s' "$body" | grep -q '"unhealthy"'; then
      echo "/ready returned 200 with an unhealthy component"
      return 1
    fi
  fi
}

check_metrics() {
  local body
  body=$(retry_curl -sf --max-time "$TIMEOUT" "$METRICS_URL/metrics") || return 1
  printf '%s' "$body" | grep -q 'fi_fhir_build_info' || return 1
  # The pre-4.3 deployment façade advertised workflow_* names nothing emitted.
  if printf '%s' "$body" | grep -q 'workflow_events_processed_total'; then
    echo "legacy workflow_* metric names reappeared"
    return 1
  fi
}

check_graphql() {
  local body
  body=$(retry_curl -sf --max-time "$TIMEOUT" -X POST "$BASE_URL/graphql" \
    -H "Authorization: Bearer $GRAPHQL_BEARER_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{"query":"{health{status}}"}') || return 1
  printf '%s' "$body" | grep -q 'health' || return 1
  # The resolver must project components rather than answering from a literal.
  printf '%s' "$body" | grep -q 'components'
}

check_websocket_disabled() {
  local status
  status=$("$CURL_BIN" -s -o /dev/null -w '%{http_code}' --max-time "$TIMEOUT" \
    --http1.1 \
    -H 'Connection: Upgrade' \
    -H 'Upgrade: websocket' \
    -H 'Sec-WebSocket-Protocol: graphql-transport-ws' \
    -H 'Sec-WebSocket-Version: 13' \
    -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
    -H "Origin: $GRAPHQL_ALLOWED_ORIGIN" \
    "$BASE_URL/graphql/ws" || true)
  if [ "$status" != "404" ]; then
    echo "Unexpected status: $status"
    return 1
  fi
}

echo ""
echo "fi-fhir Smoke Test"
echo "══════════════════"
echo "  Target:  $BASE_URL"
echo "  Metrics: $METRICS_URL"
echo ""

# 1. Liveness endpoint
check "GET /health" check_health

# 2. Readiness endpoint
check "GET /ready" check_ready

# 3. GraphQL health query
check "POST /graphql (authenticated health query)" check_graphql

# 4. WebSocket transport containment
check "GET /graphql/ws (transport disabled)" check_websocket_disabled

# 5. Prometheus exposition
check "GET $METRICS_URL/metrics" check_metrics

echo ""
echo "Results: $passed passed, $failed failed"
echo ""

if [ "$failed" -gt 0 ]; then
  echo "❌ Smoke test FAILED"
  exit 1
fi

echo "✅ Smoke test passed"
