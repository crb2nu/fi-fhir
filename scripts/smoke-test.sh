#!/usr/bin/env bash
# smoke-test.sh — Verify core fi-fhir runtime endpoint reachability.
#
# Usage:
#   GRAPHQL_BEARER_TOKEN=... BASE_URL=http://localhost:8080 bash scripts/smoke-test.sh
#
# Checks:
#   1. GET  /health             → HTTP 200, body contains "status"
#   2. POST /graphql            → HTTP 200, authenticated health query succeeds
#   3. GET  /graphql/ws (probe) → rejected; preview has no WS transport
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
  printf '%s' "$body" | grep -q 'status'
}

check_graphql() {
  local body
  body=$(retry_curl -sf --max-time "$TIMEOUT" -X POST "$BASE_URL/graphql" \
    -H "Authorization: Bearer $GRAPHQL_BEARER_TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{"query":"{health{status}}"}') || return 1
  printf '%s' "$body" | grep -q 'health'
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
echo "  Target: $BASE_URL"
echo ""

# 1. Health endpoint
check "GET /health" check_health

# 2. GraphQL introspection
check "POST /graphql (authenticated health query)" check_graphql

# 3. WebSocket transport containment
check "GET /graphql/ws (transport disabled)" check_websocket_disabled

echo ""
echo "Results: $passed passed, $failed failed"
echo ""

if [ "$failed" -gt 0 ]; then
  echo "❌ Smoke test FAILED"
  exit 1
fi

echo "✅ Smoke test passed"
