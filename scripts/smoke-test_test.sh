#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SMOKE_SCRIPT="$ROOT_DIR/scripts/smoke-test.sh"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "smoke-test_test: $*" >&2
  exit 1
}

write_fake_curl() {
  local path="$1"
  # The single-quoted arguments are literal lines for the generated fake client.
  # shellcheck disable=SC2016
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'printf "%s\n" "$*" >> "$FAKE_CURL_LOG"' \
    'case "$*" in' \
    '  *"/health"*)' \
    '    if [ "${FAKE_CURL_MODE:-success}" = "retry-health" ]; then' \
    '      count=$(grep -c "/health" "$FAKE_CURL_LOG" || true)' \
    '      [ "$count" -gt 1 ] || exit 22' \
    '    fi' \
    '    printf "%s" "{\"status\":\"healthy\"}"' \
    '    ;;' \
    '  *"/graphql/ws"*) printf "%s" "101" ;;' \
    '  *"/graphql"*)' \
    '    [ "${FAKE_CURL_MODE:-success}" != "graphql-failure" ] || exit 22' \
    '    printf "%s" "{\"data\":{\"__schema\":{}}}"' \
    '    ;;' \
    '  *) exit 22 ;;' \
    'esac' > "$path"
  chmod +x "$path"
}

FAKE_CURL="$TMP_DIR/fake-curl"
write_fake_curl "$FAKE_CURL"

success_log="$TMP_DIR/success.log"
success_output=$(FAKE_CURL_LOG="$success_log" CURL_BIN="$FAKE_CURL" \
  RETRIES=1 RETRY_DELAY=0 bash "$SMOKE_SCRIPT")
grep -q 'Results: 3 passed, 0 failed' <<<"$success_output" ||
  fail "positive path did not report all three checks"

failure_log="$TMP_DIR/failure.log"
set +e
failure_output=$(FAKE_CURL_LOG="$failure_log" FAKE_CURL_MODE=graphql-failure \
  CURL_BIN="$FAKE_CURL" RETRIES=1 RETRY_DELAY=0 bash "$SMOKE_SCRIPT" 2>&1)
failure_status=$?
set -e
[ "$failure_status" -eq 1 ] || fail "negative path exited $failure_status, want 1"
grep -q 'Results: 2 passed, 1 failed' <<<"$failure_output" ||
  fail "negative path did not aggregate the failed check"
grep -q '/graphql/ws' "$failure_log" ||
  fail "WebSocket check did not run after the GraphQL failure"

retry_log="$TMP_DIR/retry.log"
retry_output=$(FAKE_CURL_LOG="$retry_log" FAKE_CURL_MODE=retry-health \
  CURL_BIN="$FAKE_CURL" RETRIES=2 RETRY_DELAY=0 bash "$SMOKE_SCRIPT")
grep -q 'Results: 3 passed, 0 failed' <<<"$retry_output" ||
  fail "retry path did not recover"
[ "$(grep -c '/health' "$retry_log")" -eq 2 ] ||
  fail "health check did not retry exactly once"

echo "smoke-test_test: all assertions passed"
