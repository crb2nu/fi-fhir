#!/usr/bin/env bash
# smoke-test.sh — Verify core fi-fhir runtime endpoint reachability.
#
# Usage:
#   BASE_URL=http://localhost:8080 bash scripts/smoke-test.sh
#
# Checks:
#   1. GET  /health             → HTTP 200, body contains "status"
#   2. POST /graphql            → HTTP 200, introspection succeeds
#   3. GET  /graphql/ws (probe) → HTTP 101 or 400 (upgrade attempt)
#
# Exit codes:
#   0  All checks passed
#   1  One or more checks failed

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TIMEOUT="${TIMEOUT:-5}"
RETRIES="${RETRIES:-3}"
RETRY_DELAY="${RETRY_DELAY:-2}"

passed=0
failed=0

check() {
  local name="$1"
  shift
  echo -n "  [$name] ... "
  if "$@"; then
    echo "✓"
    ((passed++))
  else
    echo "✗"
    ((failed++))
  fi
}

retry_curl() {
  local attempt=0
  while [ "$attempt" -lt "$RETRIES" ]; do
    if curl "$@" 2>/dev/null; then
      return 0
    fi
    ((attempt++))
    [ "$attempt" -lt "$RETRIES" ] && sleep "$RETRY_DELAY"
  done
  return 1
}

echo ""
echo "fi-fhir Smoke Test"
echo "══════════════════"
echo "  Target: $BASE_URL"
echo ""

# 1. Health endpoint
check "GET /health" bash -c "
  body=\$(curl -sf --max-time $TIMEOUT '$BASE_URL/health') || exit 1
  echo \"\$body\" | grep -q 'status' || { echo \"Unexpected body: \$body\"; exit 1; }
"

# 2. GraphQL introspection
check "POST /graphql (introspection)" bash -c "
  body=\$(curl -sf --max-time $TIMEOUT -X POST '$BASE_URL/graphql' \
    -H 'Content-Type: application/json' \
    -d '{\"query\":\"{__schema{queryType{name}}}\"}') || exit 1
  echo \"\$body\" | grep -q '__schema' || { echo \"Unexpected body: \$body\"; exit 1; }
"

# 3. WebSocket upgrade probe (expect 101 or 400 — both prove the endpoint exists)
check "GET /graphql/ws (upgrade probe)" bash -c "
  status=\$(curl -s -o /dev/null -w '%{http_code}' --max-time $TIMEOUT \
    -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
    -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
    '$BASE_URL/graphql/ws') || true
  case \"\$status\" in
    101|400|426) exit 0 ;;
    *) echo \"Unexpected status: \$status\"; exit 1 ;;
  esac
"

echo ""
echo "Results: $passed passed, $failed failed"
echo ""

if [ "$failed" -gt 0 ]; then
  echo "❌ Smoke test FAILED"
  exit 1
fi

echo "✅ Smoke test passed"
