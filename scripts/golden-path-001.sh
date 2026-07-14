#!/usr/bin/env bash
# shellcheck disable=SC2016 # jq and GraphQL programs intentionally keep $ variables literal.
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

fixture="$root/testdata/golden/integration/adt-http"
evidence="$root/.tmp/golden-path-001"
registry="$evidence/registry.json"
compose_file="$fixture/docker-compose.yaml"
port="${GOLDEN_PATH_PORT:-18081}"
base_url="http://127.0.0.1:${port}"
graphql_token="golden-graphql-preview-token-0001"
ingress_token="golden-http-ingress-token-000001"
idempotency_key="golden-path-001-idempotency"
correlation_id="golden-path-001-correlation"
compose_project="fi-fhir-golden-path-001-${USER:-ci}"
server_pid=""
mode="compose"
tests=0

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'golden-path-001: required tool not found: %s\n' "$1" >&2
    exit 1
  }
}

for tool in curl jq go; do
  require_tool "$tool"
done

compose() {
  COMPOSE_PROJECT_NAME="$compose_project" \
    GOLDEN_PATH_REGISTRY_PATH="$registry" \
    GOLDEN_PATH_PORT="$port" \
    docker compose -f "$compose_file" "$@"
}

cleanup() {
  local status=$?
  if [[ -n "$server_pid" ]]; then
    kill -TERM "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  if [[ "$mode" == "compose" ]] && command -v docker >/dev/null 2>&1; then
    compose logs --no-color >"$evidence/compose.log" 2>&1 || true
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  if [[ -n "${GOLDEN_PATH_PSQL_CONTAINER:-}" ]]; then
    docker --context "${GOLDEN_PATH_DOCKER_CONTEXT:-default}" rm --force "$GOLDEN_PATH_PSQL_CONTAINER" >/dev/null 2>&1 || true
  fi
  if (( status != 0 )); then
    write_junit "failure" "golden-path-001 failed at line ${BASH_LINENO[0]:-unknown}"
  fi
  exit "$status"
}
trap cleanup EXIT

write_junit() {
  local outcome="$1"
  local message="${2:-}"
  if [[ "$outcome" == "failure" ]]; then
    jq -n --arg message "$message" --argjson tests "$tests" \
      '{testsuite:{name:"golden-path-001",tests:$tests,failures:1,message:$message}}' \
      >"$evidence/junit-summary.json"
    printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' \
      '<testsuite name="golden-path-001" tests="1" failures="1">' \
      "  <testcase name=\"authenticated-durable-ide-parity\"><failure message=\"golden-path-001 failed\"/></testcase>" \
      '</testsuite>' >"$evidence/junit.xml"
  else
    jq -n --argjson tests "$tests" \
      '{testsuite:{name:"golden-path-001",tests:$tests,failures:0}}' \
      >"$evidence/junit-summary.json"
    printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' \
      '<testsuite name="golden-path-001" tests="1" failures="0">' \
      '  <testcase name="authenticated-durable-ide-parity"/>' \
      '</testsuite>' >"$evidence/junit.xml"
  fi
}

assert_jq() {
  local file="$1"
  local expression="$2"
  local label="$3"
  tests=$((tests + 1))
  if ! jq -e "$expression" "$file" >/dev/null; then
    printf 'golden-path-001 assertion failed: %s\n' "$label" >&2
    jq . "$file" >&2 || true
    return 1
  fi
}

assert_jq_pair() {
  local left="$1"
  local right="$2"
  local expression="$3"
  local label="$4"
  tests=$((tests + 1))
  if ! jq -e -s "$expression" "$left" "$right" >/dev/null; then
    printf 'golden-path-001 assertion failed: %s\n' "$label" >&2
    return 1
  fi
}

assert_equal_files() {
  local left="$1"
  local right="$2"
  local label="$3"
  tests=$((tests + 1))
  if ! cmp -s "$left" "$right"; then
    printf 'golden-path-001 assertion failed: %s\n' "$label" >&2
    diff -u "$left" "$right" >&2 || true
    return 1
  fi
}

rm -rf "$evidence"
mkdir -p "$evidence/http"
go run ./scripts/golden-path-001-fixture -fixture "$fixture" -output "$registry"
chmod 0444 "$registry"

wait_ready() {
  for _ in $(seq 1 90); do
    if curl --fail --silent --show-error "$base_url/health" >"$evidence/http/health.json" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  printf 'golden-path-001: server did not become ready\n' >&2
  return 1
}

start_external_server() {
  FI_FHIR_DEPLOYMENT_TENANT_ID="tenant-a" \
  FI_FHIR_GRAPHQL_PRINCIPAL_ID="golden-ide-user" \
  FI_FHIR_GRAPHQL_ROLES="integration:preview" \
  FI_FHIR_GRAPHQL_ALLOWED_ORIGINS="https://ide.golden.test" \
  FI_FHIR_GRAPHQL_BEARER_TOKEN="$graphql_token" \
  FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE="" \
  FI_FHIR_INTEGRATION_REGISTRY_PATH="$registry" \
  FI_FHIR_HTTP_INGRESS_AUTH_MODE="bearer" \
  FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID="golden-ingress-service" \
  FI_FHIR_HTTP_INGRESS_INTEGRATION_ID="adt-tolerant" \
  FI_FHIR_HTTP_INGRESS_SECRET="$ingress_token" \
  FI_FHIR_HTTP_INGRESS_SECRET_FILE="" \
  FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES="1048576" \
  "$evidence/fi-fhir" serve --port "$port" --no-playground --no-introspection \
    >>"$evidence/server.log" 2>&1 &
  server_pid=$!
  wait_ready
}

stop_external_server() {
  kill -TERM "$server_pid"
  wait "$server_pid"
  server_pid=""
}

psql_query() {
  local query="$1"
  if [[ "$mode" == "compose" ]]; then
    compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -U golden -d fi_fhir_golden -A -t -c "$query"
  elif [[ -n "${GOLDEN_PATH_PSQL_CONTAINER:-}" ]]; then
    docker --context "${GOLDEN_PATH_DOCKER_CONTEXT:-default}" exec "$GOLDEN_PATH_PSQL_CONTAINER" \
      psql -X -v ON_ERROR_STOP=1 -U "${FI_FHIR_DATABASE_USERNAME}" -d "${FI_FHIR_DATABASE_NAME}" -A -t -c "$query"
  else
    psql "$POSTGRES_TEST_URL" -X -v ON_ERROR_STOP=1 -A -t -c "$query"
  fi
}

if [[ -n "${POSTGRES_TEST_URL:-}" ]]; then
  mode="external"
  if [[ -z "${GOLDEN_PATH_PSQL_CONTAINER:-}" ]]; then
    require_tool psql
  fi
  if [[ "${GOLDEN_PATH_ALLOW_DATABASE_RESET:-}" != "1" ]]; then
    printf 'golden-path-001: external database reset requires GOLDEN_PATH_ALLOW_DATABASE_RESET=1\n' >&2
    exit 1
  fi
  : "${FI_FHIR_DATABASE_HOST:?FI_FHIR_DATABASE_HOST is required}"
  : "${FI_FHIR_DATABASE_NAME:?FI_FHIR_DATABASE_NAME is required}"
  : "${FI_FHIR_DATABASE_USERNAME:?FI_FHIR_DATABASE_USERNAME is required}"
  database_ready=false
  for _ in $(seq 1 60); do
    if psql_query 'SELECT 1;' >/dev/null 2>&1; then
      database_ready=true
      break
    fi
    sleep 1
  done
  if [[ "$database_ready" != "true" ]]; then
    printf 'golden-path-001: PostgreSQL did not become ready\n' >&2
    exit 1
  fi
  psql_query 'DROP TABLE IF EXISTS integration_delivery_outbox, integration_delivery_attempts, integration_message_lineage, integration_canonical_events, integration_receipts, integration_submission_schema_migrations CASCADE;'
  go build -o "$evidence/fi-fhir" ./cmd/fi-fhir
  start_external_server
else
  require_tool docker
  docker compose version >/dev/null
  compose up --build --detach
  wait_ready
fi

http_submit() {
  local output="$1"
  local payload="$2"
  local key="$3"
  local token="$4"
  shift 4
  curl --silent --show-error \
    --output "$output" --write-out '%{http_code}' \
    --request POST "$base_url/v1/hl7v2" \
    --header 'Content-Type: application/hl7-v2+er7' \
    --header 'X-Fi-Fhir-Integration-ID: adt-tolerant' \
    --header "Authorization: Bearer ${token}" \
    --header "Idempotency-Key: ${key}" \
    --header "X-Correlation-ID: ${correlation_id}" \
    --data-binary "@${payload}" "$@"
}

assert_http_code() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  tests=$((tests + 1))
  if [[ "$actual" != "$expected" ]]; then
    printf 'golden-path-001 assertion failed: %s (want %s, got %s)\n' "$label" "$expected" "$actual" >&2
    return 1
  fi
}

# Fail-closed protocol boundary.
code="$(curl --silent --output "$evidence/http/reject-method.json" --write-out '%{http_code}' "$base_url/v1/hl7v2")"
assert_http_code 405 "$code" 'GET rejected'
code="$(curl --silent --output "$evidence/http/reject-media.json" --write-out '%{http_code}' --request POST "$base_url/v1/hl7v2" --header 'Content-Type: text/plain' --data-binary "@$fixture/input.hl7")"
assert_http_code 415 "$code" 'wrong media rejected'
code="$(http_submit "$evidence/http/reject-origin.json" "$fixture/input.hl7" protocol-origin "$ingress_token" --header 'Origin: https://browser.invalid')"
assert_http_code 403 "$code" 'browser Origin rejected'
code="$(http_submit "$evidence/http/reject-auth.json" "$fixture/input.hl7" protocol-auth wrong-golden-http-ingress-token)"
assert_http_code 401 "$code" 'wrong bearer rejected'
code="$(curl --silent --output "$evidence/http/reject-integration.json" --write-out '%{http_code}' --request POST "$base_url/v1/hl7v2" --header 'Content-Type: application/hl7-v2+er7' --header 'X-Fi-Fhir-Integration-ID: missing' --header "Authorization: Bearer ${ingress_token}" --data-binary "@$fixture/input.hl7")"
assert_http_code 404 "$code" 'unbound integration rejected'
dd if=/dev/zero of="$evidence/oversized.bin" bs=1048577 count=1 2>/dev/null
code="$(http_submit "$evidence/http/reject-oversized.json" "$evidence/oversized.bin" protocol-size "$ingress_token")"
assert_http_code 413 "$code" 'oversized body rejected'

# First durable admission and live duplicate collapse.
code="$(http_submit "$evidence/http/accepted-1.json" "$fixture/input.hl7" "$idempotency_key" "$ingress_token")"
assert_http_code 202 "$code" 'first submission accepted'
code="$(http_submit "$evidence/http/accepted-2.json" "$fixture/input.hl7" "$idempotency_key" "$ingress_token")"
assert_http_code 202 "$code" 'duplicate submission accepted'
assert_equal_files "$evidence/http/accepted-1.json" "$evidence/http/accepted-2.json" 'duplicate response is byte-identical'

assert_jq "$evidence/http/accepted-1.json" '
  .receipt.status == "accepted" and
  (.events | length == 1)
' 'accepted response has one event'
assert_jq_pair "$evidence/http/accepted-1.json" "$fixture/expected.json" '
  .[0] as $got | .[1] as $expected |
  ($got.events[0].type == $expected.event_type) and
  ([$got.warnings[].code] == $expected.warning_codes) and
  ($got.routes[0].route == $expected.route) and
  ($got.deliveries[0].action == $expected.action) and
  ($got.deliveries[0].status == $expected.production_delivery_status)
' 'accepted response exposes warning, route, and queued delivery'

# IDE/Integration Session preview uses the same server-owned artifacts and kernel.
preview_query='mutation Preview($input: PreviewIntegrationMessageInput!) { previewIntegrationMessage(input: $input) { mode tenantId integrationRevision { artifactId revisionId digest } artifactRevisions { source { artifactId revisionId digest } profile { artifactId revisionId digest } workflow { artifactId revisionId digest } } events { tenantId id type sourceMessageId correlationId classification payload } diagnostics { tenantId severity stage code message path source classification } routes { tenantId eventId route matched skipped skipReason transformCount plannedActions diagnosticCodes } deliveries { tenantId eventId route action status diagnosticCodes destination { artifactId revisionId digest class } } correlations { tenantId correlationId traceId sourceMessageId eventIds workflowRunId } } }'
preview_request() {
  local integration="$1"
  local output="$2"
  jq -n --arg query "$preview_query" --rawfile data "$fixture/input.hl7" --arg integration "$integration" --arg correlation "$correlation_id" \
    '{query:$query,variables:{input:{integrationId:$integration,data:$data,correlationId:$correlation,reason:"Golden Path 001 parity proof"}}}' \
    | curl --silent --show-error --output "$output" --request POST "$base_url/graphql" \
        --header 'Content-Type: application/json' \
        --header 'Origin: https://ide.golden.test' \
        --header "Authorization: Bearer ${graphql_token}" \
        --data-binary @-
}
preview_request adt-tolerant "$evidence/http/preview-tolerant.json"
preview_request adt-strict "$evidence/http/preview-strict.json"
assert_jq "$evidence/http/preview-tolerant.json" '.errors == null and .data.previewIntegrationMessage.mode == "preview" and .data.previewIntegrationMessage.deliveries[0].status == "suppressed"' 'tolerant IDE preview succeeds without production delivery'
assert_jq "$evidence/http/preview-strict.json" '.errors | length > 0' 'strict profile changes parse outcome'

# Changed bytes under the committed key must conflict.
sed 's/Patient\^Golden/Patient^Changed/' "$fixture/input.hl7" >"$evidence/changed-input.hl7"
code="$(http_submit "$evidence/http/reject-idempotency-conflict.json" "$evidence/changed-input.hl7" "$idempotency_key" "$ingress_token")"
assert_http_code 409 "$code" 'changed-body idempotency reuse rejected'

# Real process restart, followed by durable replay and evidence query.
if [[ "$mode" == "compose" ]]; then
  compose restart fi-fhir
  wait_ready
else
  stop_external_server
  start_external_server
fi
code="$(http_submit "$evidence/http/accepted-after-restart.json" "$fixture/input.hl7" "$idempotency_key" "$ingress_token")"
assert_http_code 202 "$code" 'duplicate accepted after process restart'
assert_equal_files "$evidence/http/accepted-1.json" "$evidence/http/accepted-after-restart.json" 'post-restart response is byte-identical'

psql_query "
SELECT jsonb_pretty(jsonb_build_object(
  'counts', jsonb_build_object(
    'receipts', (SELECT count(*) FROM integration_receipts),
    'events', (SELECT count(*) FROM integration_canonical_events),
    'lineage', (SELECT count(*) FROM integration_message_lineage),
    'attempts', (SELECT count(*) FROM integration_delivery_attempts),
    'outbox', (SELECT count(*) FROM integration_delivery_outbox)
  ),
  'receipt', (SELECT to_jsonb(r) - 'request_fingerprint' - 'principal_json' FROM integration_receipts r LIMIT 1),
  'result', (SELECT result_json FROM integration_receipts LIMIT 1),
  'event', (SELECT to_jsonb(e) FROM integration_canonical_events e LIMIT 1),
  'lineage', (SELECT to_jsonb(l) FROM integration_message_lineage l LIMIT 1),
  'attempt', (SELECT to_jsonb(a) FROM integration_delivery_attempts a LIMIT 1),
  'outbox', (SELECT to_jsonb(o) FROM integration_delivery_outbox o LIMIT 1)
));" >"$evidence/sql-export.json"

assert_jq_pair "$evidence/sql-export.json" "$fixture/expected.json" '
  .[0] as $got | .[1] as $expected |
  ($got.counts == $expected.counts) and
  ($got.attempt.status == $expected.production_delivery_status) and
  ($got.attempt.route_name == $expected.route) and
  ($got.attempt.action_id == $expected.action) and
  ($got.outbox.status == "pending")
' 'one durable receipt/event/lineage/attempt/outbox survives restart'

# Compare normalized business payload, diagnostics, routes, and exact provenance.
jq -S '.result | {
  integration_revision,
  artifact_revisions,
  events: [.events[] | {type, payload: (.payload | del(.received_at))}],
  diagnostics: [.diagnostics[] | {severity,stage,code,path}],
  routes: [.routes[] | {route,matched,skipped:(.skipped // false),planned_actions}]
}' "$evidence/sql-export.json" >"$evidence/production-normalized.json"
jq -S '.data.previewIntegrationMessage | {
  integration_revision: (.integrationRevision | {artifact_id:.artifactId,revision_id:.revisionId,digest}),
  artifact_revisions: {
    source:(.artifactRevisions.source | {artifact_id:.artifactId,revision_id:.revisionId,digest}),
    profile:(.artifactRevisions.profile | {artifact_id:.artifactId,revision_id:.revisionId,digest}),
    workflow:(.artifactRevisions.workflow | {artifact_id:.artifactId,revision_id:.revisionId,digest})
  },
  events: [.events[] | {type,payload:(.payload | del(.received_at))}],
  diagnostics: [.diagnostics[] | {severity,stage,code,path}],
  routes: [.routes[] | {route,matched,skipped,planned_actions:.plannedActions}]
}' "$evidence/http/preview-tolerant.json" >"$evidence/preview-normalized.json"
assert_equal_files "$evidence/production-normalized.json" "$evidence/preview-normalized.json" 'production and IDE semantics are equivalent'

# Preview must remain side-effect free even while sharing the durable processor.
psql_query "SELECT json_build_object('receipts', (SELECT count(*) FROM integration_receipts), 'events', (SELECT count(*) FROM integration_canonical_events), 'attempts', (SELECT count(*) FROM integration_delivery_attempts), 'outbox', (SELECT count(*) FROM integration_delivery_outbox));" >"$evidence/counts-after-preview.json"
assert_jq "$evidence/counts-after-preview.json" '.receipts == 1 and .events == 1 and .attempts == 1 and .outbox == 1' 'preview creates no durable side effect'

# Public responses and every persisted JSON value must not contain raw/secret sentinels.
psql_query "
SELECT concat_ws(E'\\n',
  (SELECT string_agg(result_json::text, E'\\n') FROM integration_receipts),
  (SELECT string_agg(payload_json::text, E'\\n') FROM integration_canonical_events),
  (SELECT string_agg(diagnostics_json::text || routes_json::text || artifact_revisions_json::text, E'\\n') FROM integration_message_lineage),
  (SELECT string_agg(destination_revision_json::text, E'\\n') FROM integration_delivery_attempts),
  (SELECT string_agg(payload_json::text, E'\\n') FROM integration_delivery_outbox)
);" >"$evidence/persisted-json.txt"
tests=$((tests + 1))
if rg -F 'RAW-GOLDEN-PHI-SENTINEL' "$evidence/http" "$evidence/persisted-json.txt" >/dev/null || \
   rg -F "$graphql_token" "$evidence/http" "$evidence/persisted-json.txt" >/dev/null || \
   rg -F "$ingress_token" "$evidence/http" "$evidence/persisted-json.txt" >/dev/null; then
  printf 'golden-path-001 assertion failed: raw clinical data or credential leaked\n' >&2
  exit 1
fi

jq -n \
  --arg status passed \
  --arg mode "$mode" \
  --argjson assertions "$tests" \
  --slurpfile production "$evidence/production-normalized.json" \
  --slurpfile preview "$evidence/preview-normalized.json" \
  --slurpfile durable "$evidence/sql-export.json" \
  '{golden_path:"001",status:$status,environment:$mode,assertions:$assertions,production:$production[0],preview:$preview[0],durable:$durable[0].counts}' \
  >"$evidence/assertions.json"
write_junit success
printf 'Golden Path 001 passed: %d assertions; evidence: %s\n' "$tests" "$evidence"
