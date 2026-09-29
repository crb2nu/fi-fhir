#!/usr/bin/env bash
# Operator-bundle fixture (.loom/42 E-0): durable data before the checks run.
#
# The operator-bundle stack must start with at least one `deployed` definition
# in the lifecycle catalog and at least two `accepted` receipts with queued
# delivery attempts, produced by the real admission path. This script writes
# them into the bundle stack's database with the same fi-fhir binary:
#
#   1. `fi-fhir lifecycle seed --validate skip --through deployed` puts the
#      batch definition e2e-batch-adt/v1 through create_draft -> validate ->
#      approve -> publish -> deploy (VALIDATION_SKIPPED, 24 h max age).
#   2. Admission process A: a short-lived `fi-fhir serve` with the durable HTTP
#      ingress (/v1/hl7v2, bearer) and the delivery worker pointed at a Kafka
#      broker that is not there (127.0.0.1:9). Message E2E-FIXTURE-001 is
#      admitted; the worker claims it three times and dead-letters it
#      (KAFKA_PUBLISH_FAILED): six audit rows and one open dead letter.
#   3. Admission process B: the ingress alone. E2E-FIXTURE-002 and -003 are
#      admitted and their attempts stay `queued` (no worker, no broker).
#
# The ingress runs in these side processes, never on the bundle API (:18081),
# so that stack still mounts no adapter (checks 8 and 9). Both side processes
# write their own runtime-observation heartbeats; they stop, so the Engine tab
# and Home show them going stale — which is what a replica that left looks like.
#
# Every message is synthetic (SYNTHETIC^PATIENT, placeholder identifiers); the
# bearer values are test strings, not credentials. The ingress refuses any
# request with an Origin header, so curl posts without one.
#
# Inputs (from run.sh): E2E_API_BIN, E2E_REGISTRY_PATH, E2E_RESULTS_DIR,
# E2E_PG_HOST/PORT/USER/PASSWORD, FIXTURE_DATABASE. Writes
# $E2E_RESULTS_DIR/fixture.json (the seed summary and the admission receipts).
set -euo pipefail

E2E_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
FIXTURES=$E2E_DIR/fixtures
: "${E2E_API_BIN:?}" "${E2E_REGISTRY_PATH:?}" "${E2E_RESULTS_DIR:?}" "${FIXTURE_DATABASE:?}"
: "${E2E_PG_HOST:?}" "${E2E_PG_PORT:?}" "${E2E_PG_USER:?}" "${E2E_PG_PASSWORD:?}"
FIXTURE_PORT_A=${FIXTURE_PORT_A:-18095}
FIXTURE_PORT_B=${FIXTURE_PORT_B:-18096}
INGRESS_BEARER=e2e-fixture-ingress-not-a-secret-01
LOG_DIR=$E2E_RESULTS_DIR/logs

flog() { printf '[ui-e2e fixture] %s\n' "$*"; }
fdie() { printf '[ui-e2e fixture] FAILED: %s\n' "$*" >&2; exit 1; }

db_url="postgres://$E2E_PG_USER@$E2E_PG_HOST:$E2E_PG_PORT/$FIXTURE_DATABASE?sslmode=disable"
export PGPASSWORD=$E2E_PG_PASSWORD
sql() { psql "$db_url" -qtAX -v ON_ERROR_STOP=1 -c "$1"; }

# The environment every fixture process shares: the bundle stack's tenant,
# registry and database, and nothing inherited from the caller.
fixture_env() {
  while IFS= read -r variable; do unset "$variable"; done < <(compgen -e | grep '^FI_FHIR_' || true)
  export FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a
  export FI_FHIR_INTEGRATION_REGISTRY_PATH=$E2E_REGISTRY_PATH
  export FI_FHIR_DATABASE_DRIVER=postgres
  export FI_FHIR_DATABASE_HOST=$E2E_PG_HOST
  export FI_FHIR_DATABASE_PORT=$E2E_PG_PORT
  export FI_FHIR_DATABASE_NAME=$FIXTURE_DATABASE
  export FI_FHIR_DATABASE_USERNAME=$E2E_PG_USER
  export FI_FHIR_DATABASE_PASSWORD=$E2E_PG_PASSWORD
  export FI_FHIR_DATABASE_SSL_MODE=disable
  export FI_FHIR_METRICS_ENABLED=false
}

# --- 1. lifecycle seed ---------------------------------------------------------
seed_args=(
  lifecycle seed
  --source "$FIXTURES/batch-source-s3.json"
  --definition-id e2e-batch-adt
  --integration adt-east
  --registry "$E2E_REGISTRY_PATH"
  --destination "$FIXTURES/destination-fhir-primary.json"
  --principal e2e-fixture-seed
  --reason "e2e fixture: a deployed batch definition, no connection check"
  --tenant tenant-a
  --validate skip
  --through deployed
  --validation-max-age 86400
)
( fixture_env; "$E2E_API_BIN" "${seed_args[@]}" --dry-run ) >"$LOG_DIR/fixture-seed-dry-run.json" 2>&1 \
  || { cat "$LOG_DIR/fixture-seed-dry-run.json" >&2; fdie "lifecycle seed --dry-run"; }
( fixture_env; "$E2E_API_BIN" "${seed_args[@]}" ) >"$LOG_DIR/fixture-seed.json" 2>&1 \
  || { cat "$LOG_DIR/fixture-seed.json" >&2; fdie "lifecycle seed"; }
flog "seeded e2e-batch-adt/v1 through deployed"

# --- 2./3. admission processes -------------------------------------------------
ADMISSION_PID=""
stop_admission() {
  if [ -n "$ADMISSION_PID" ]; then
    kill "$ADMISSION_PID" 2>/dev/null || true
    wait "$ADMISSION_PID" 2>/dev/null || true
    ADMISSION_PID=""
  fi
}
trap stop_admission EXIT

# start_admission NAME PORT [worker]
start_admission() {
  local name=$1 port=$2 worker=${3:-}
  (
    fixture_env
    export FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=http://127.0.0.1:1
    export FI_FHIR_GRAPHQL_PRINCIPAL_ID=e2e-fixture
    export FI_FHIR_GRAPHQL_ROLES=integration:preview
    export FI_FHIR_GRAPHQL_BEARER_TOKEN=e2e-fixture-graphql-not-a-secret-00
    export FI_FHIR_HTTP_INGRESS_AUTH_MODE=bearer
    export FI_FHIR_HTTP_INGRESS_INTEGRATION_ID=adt-east
    export FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID=e2e-fixture-sender
    export FI_FHIR_HTTP_INGRESS_SECRET=$INGRESS_BEARER
    if [ "$worker" = worker ]; then
      export FI_FHIR_DELIVERY_WORKER_ENABLED=true
      export FI_FHIR_QUEUE_DRIVER=kafka
      # Nothing listens here: every publish fails, and the third failure
      # dead-letters the attempt through the worker's own MarkFailed.
      export FI_FHIR_QUEUE_BROKERS=127.0.0.1:9
      export FI_FHIR_DELIVERY_MAX_ATTEMPTS=3
      export FI_FHIR_DELIVERY_PUBLISH_TIMEOUT=2s
      export FI_FHIR_DELIVERY_POLL_INTERVAL=200ms
      export FI_FHIR_DELIVERY_RETRY_BASE_DELAY=200ms
      export FI_FHIR_DELIVERY_RETRY_MAX_DELAY=400ms
    fi
    exec "$E2E_API_BIN" serve --host 127.0.0.1 --port "$port" --no-playground --no-introspection
  ) >"$LOG_DIR/api-fixture-$name.log" 2>&1 &
  ADMISSION_PID=$!
  local deadline=$((SECONDS + 120))
  until curl -sf -o /dev/null "http://127.0.0.1:$port/health"; do
    if [ "$SECONDS" -ge "$deadline" ] || ! kill -0 "$ADMISSION_PID" 2>/dev/null; then
      tail -n 40 "$LOG_DIR/api-fixture-$name.log" >&2
      fdie "admission process $name did not start"
    fi
    sleep 0.5
  done
}

# admit PORT MSH10 CORRELATION -> prints the ingress response body
admit() {
  local port=$1 control=$2 correlation=$3 body status
  body=$(printf 'MSH|^~\\&|FI_FHIR_E2E|TEST_FACILITY|FI_FHIR|TEST_FACILITY|20260101090000||ADT^A01|%s|T|2.5.1\rEVN|A01|20260101090000\rPID|1||SYNTHETIC-0001^^^E2E^MR||SYNTHETIC^PATIENT||20000101|U\rPV1|1|I|TEST_WARD^TEST_ROOM^TEST_BED' "$control")
  local out=$E2E_RESULTS_DIR/logs/fixture-admit-$control.json
  status=$(printf '%s' "$body" | curl -s -o "$out" -w '%{http_code}' -X POST "http://127.0.0.1:$port/v1/hl7v2" \
    -H "Authorization: Bearer $INGRESS_BEARER" \
    -H 'Content-Type: application/hl7-v2+er7' \
    -H 'X-Fi-Fhir-Integration-ID: adt-east' \
    -H "Idempotency-Key: $control" \
    -H "X-Correlation-ID: $correlation" \
    --data-binary @-)
  [ "$status" = 202 ] || { cat "$out" >&2; fdie "admission of $control answered $status"; }
  flog "admitted $control"
}

start_admission a "$FIXTURE_PORT_A" worker
admit "$FIXTURE_PORT_A" E2E-FIXTURE-001 e2e-fixture-dead-letter
deadline=$((SECONDS + 90))
until [ "$(sql "SELECT count(*) FROM integration_delivery_dlq WHERE active")" = 1 ]; do
  [ "$SECONDS" -lt "$deadline" ] || { tail -n 40 "$LOG_DIR/api-fixture-a.log" >&2; fdie "E2E-FIXTURE-001 was not dead-lettered within 90s"; }
  sleep 1
done
stop_admission
flog "E2E-FIXTURE-001 dead-lettered by the worker (unreachable broker)"

start_admission b "$FIXTURE_PORT_B"
admit "$FIXTURE_PORT_B" E2E-FIXTURE-002 e2e-fixture-queued-002
admit "$FIXTURE_PORT_B" E2E-FIXTURE-003 e2e-fixture-queued-003
stop_admission

# --- what the checks may rely on -------------------------------------------------
accepted=$(sql "SELECT count(*) FROM integration_receipts WHERE status = 'accepted'")
queued=$(sql "SELECT count(*) FROM integration_delivery_attempts WHERE status = 'queued'")
dead=$(sql "SELECT count(*) FROM integration_delivery_dlq WHERE active")
deployed=$(sql "SELECT count(*) FROM integration_lifecycle_snapshots WHERE state = 'deployed'")
[ "$accepted" -ge 3 ] && [ "$queued" -ge 2 ] && [ "$dead" -ge 1 ] && [ "$deployed" -ge 1 ] \
  || fdie "fixture incomplete: accepted=$accepted queued=$queued dead_letters=$dead deployed=$deployed"
printf '{"accepted":%s,"queued":%s,"deadLetters":%s,"deployed":%s,"definition":"e2e-batch-adt","revision":"v1"}\n' \
  "$accepted" "$queued" "$dead" "$deployed" >"$E2E_RESULTS_DIR/fixture.json"
flog "ready: $accepted accepted receipts, $queued queued attempts, $dead open dead letter, $deployed deployed definition"
