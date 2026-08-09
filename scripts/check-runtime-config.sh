#!/usr/bin/env bash
# check-runtime-config.sh — Validate proxy/runtime config assumptions.
#
# Checks:
#   1. .env.example covers all FI_FHIR_* env vars referenced in Go source.
#   2. Docker Compose forwards HTTP ingress OAuth, MLLP listener, and batch
#      source settings.
#   3. Proxy config assumptions are documented.
#
# Usage:
#   bash scripts/check-runtime-config.sh

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_EXAMPLE="${ROOT}/.env.example"
NGINX_CONF="${ROOT}/ui/nginx/default.conf.template"
VITE_CONF="${ROOT}/ui/vite.config.ts"
COMPOSE_FILE="${ROOT}/docker-compose.yaml"
passed=0
warned=0
failed=0

check() {
  local name="$1"
  shift
  echo -n "  [$name] ... "
  if "$@"; then
    echo "✓"
    ((passed++))
  else
    echo "⚠"
    ((warned++))
  fi
}

check_required() {
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

echo ""
echo "fi-fhir Runtime Config Check"
echo "════════════════════════════"
echo ""

# --------------------------------------------------------------------------
# 1. Enumerate FI_FHIR_* vars used in Go source (exclude test files)
# --------------------------------------------------------------------------
echo "─── Env Var Coverage ───"

# Keep filenames in the grep output (-o without -h) so the test-file filter has
# something to match on, then strip the path off the surviving lines. An earlier
# version used `grep -roh ... | grep -v '_test.go'`, but -h suppresses filenames,
# so the filter saw bare tokens, never matched, and reported every test-only var
# as missing from .env.example.
#
# --include/--exclude would read better but are GNU/BSD extensions that busybox
# grep ignores, silently yielding an empty var list on the alpine CI image --
# which would make the compose check_required gates below pass vacuously.
go_vars=$(grep -ro 'FI_FHIR_[A-Z_]*' "${ROOT}/cmd/" "${ROOT}/internal/" "${ROOT}/pkg/" 2>/dev/null \
  | grep '\.go:' | grep -v '_test\.go:' | sed 's/.*://' | sort -u || true)

if [ -f "$ENV_EXAMPLE" ]; then
  example_contents=$(cat "$ENV_EXAMPLE")
  missing=""
  for var in $go_vars; do
    if ! echo "$example_contents" | grep -q "$var"; then
      missing="$missing $var"
    fi
  done

  check ".env.example covers Go vars" bash -c "
    if [ -n '$missing' ]; then
      echo 'Missing:$missing'
      exit 1
    fi
  "
else
  echo "  [.env.example] ... ⚠ file not found"
  ((warned++))
fi

# --------------------------------------------------------------------------
# 2. Docker Compose: production ingress environment forwarding
# --------------------------------------------------------------------------
echo ""
echo "─── Compose Config ───"

oauth_ingress_vars=$(printf '%s\n' "$go_vars" | grep '^FI_FHIR_HTTP_INGRESS_OAUTH_' || true)
mllp_vars=$(printf '%s\n' "$go_vars" | grep '^FI_FHIR_MLLP_' || true)
batch_vars=$(printf '%s\n' "$go_vars" | grep '^FI_FHIR_BATCH_' || true)
missing_compose=""
missing_mllp=""
missing_batch=""
if [ -f "$COMPOSE_FILE" ]; then
  for var in $oauth_ingress_vars; do
    if ! grep -q "$var" "$COMPOSE_FILE"; then
      missing_compose="$missing_compose $var"
    fi
  done
  check_required "Compose forwards ingress OAuth vars" bash -c "
    if [ -n '$missing_compose' ]; then
      echo 'Missing:$missing_compose'
      exit 1
    fi
  "
  for var in $mllp_vars; do
    if ! grep -q "$var" "$COMPOSE_FILE"; then
      missing_mllp="$missing_mllp $var"
    fi
  done
  check_required "Compose forwards MLLP listener vars" bash -c "
    if [ -n '$missing_mllp' ]; then
      echo 'Missing:$missing_mllp'
      exit 1
    fi
  "
  for var in $batch_vars; do
    if ! grep -q "$var" "$COMPOSE_FILE"; then
      missing_batch="$missing_batch $var"
    fi
  done
  check_required "Compose forwards batch source vars" bash -c "
    if [ -n '$missing_batch' ]; then
      echo 'Missing:$missing_batch'
      exit 1
    fi
  "
  check_required "Compose local IDE has preview and compatibility roles" bash -c '
    file="'"$COMPOSE_FILE"'"
    roles=$(grep -E "^[[:space:]]*FI_FHIR_GRAPHQL_ROLES:" "$file" | head -1 | cut -d: -f2- | tr -d "\\\"[:space:]" )
    case ",$roles," in
      *,integration:preview,*) ;;
      *) echo "local IDE is missing integration:preview"; exit 1 ;;
    esac
    case ",$roles," in
      *,graphql:operator,*) ;;
      *) echo "local IDE is missing graphql:operator"; exit 1 ;;
    esac
  '
else
  echo "  [docker-compose.yaml] ... ⚠ file not found"
  ((warned++))
fi

check_required ".env.example local IDE has preview and compatibility roles" bash -c '
  file="'"$ENV_EXAMPLE"'"
  roles=$(grep -E "^[[:space:]]*FI_FHIR_GRAPHQL_ROLES=" "$file" | head -1 | cut -d= -f2- | tr -d "\\\"[:space:]" )
  case ",$roles," in
    *,integration:preview,*) ;;
    *) echo ".env.example is missing integration:preview"; exit 1 ;;
  esac
  case ",$roles," in
    *,graphql:operator,*) ;;
    *) echo ".env.example is missing graphql:operator"; exit 1 ;;
  esac
'

# --------------------------------------------------------------------------
# 3. Proxy config: nginx template references
# --------------------------------------------------------------------------
echo ""
echo "─── Proxy Config ───"

if [ -f "$NGINX_CONF" ]; then
  check "nginx proxies /graphql" grep -q '/graphql' "$NGINX_CONF"
  check "nginx proxies /health"  grep -q '/health'  "$NGINX_CONF"
  check "nginx disables WebSocket" grep -q 'location = /graphql/ws' "$NGINX_CONF"
  check "nginx avoids request buffering" grep -q 'proxy_request_buffering off' "$NGINX_CONF"
  check "nginx does not forward upgrades" bash -c "! grep -q 'proxy_set_header Upgrade' '$NGINX_CONF'"
  check "nginx proxies trusted-network auth status" grep -Fq 'location = /api/auth/status {' "$NGINX_CONF"
  check "nginx does not proxy general legacy /api" bash -c "! grep -q 'location /api' '$NGINX_CONF'"
  check "nginx rejects legacy /api root" grep -Fq 'location = /api {' "$NGINX_CONF"
  check "nginx rejects legacy /api subtree" grep -Fq 'location ^~ /api/ {' "$NGINX_CONF"
else
  echo "  [nginx conf] ... ⚠ $NGINX_CONF not found"
  ((warned++))
fi

if [ -f "$VITE_CONF" ]; then
  check "Vite does not proxy legacy /api" bash -c "! grep -q \"'/api'\" '$VITE_CONF'"
  check "Vite does not enable WebSocket proxying" bash -c "! grep -q 'ws: true' '$VITE_CONF'"
fi

# --------------------------------------------------------------------------
# 3. Observability truth (Slice 4.3)
# --------------------------------------------------------------------------
#
# Before Slice 4.3 a complete observability façade shipped around endpoints that
# did not exist: pod annotations, a named metrics containerPort, two Services, a
# Prometheus scrape job, a Grafana dashboard, and 32 alert rules, all pointing at
# nothing. These checks are what stop it regrowing.
echo ""
echo "─── Observability Truth ───"

K8S_DEPLOYMENT="${ROOT}/deploy/kubernetes/base/deployment.yaml"
HELM_DEPLOYMENT="${ROOT}/deploy/helm/fi-fhir/templates/deployment.yaml"

check_required "Kubernetes probes address the served endpoints" bash -c '
  file="'"$K8S_DEPLOYMENT"'"
  [ -f "$file" ] || { echo "deployment manifest not found"; exit 1; }
  if grep -q "command: \[\"/fi-fhir\", \"version\"\]" "$file"; then
    echo "probes still exec /fi-fhir version, which proves only that a subprocess can print a version string"
    exit 1
  fi
  grep -q "path: /health" "$file" || { echo "no liveness probe on /health"; exit 1; }
  grep -q "path: /ready" "$file" || { echo "no readiness probe on /ready"; exit 1; }
'

check_required "Helm readiness probe is distinct from liveness" bash -c '
  file="'"$HELM_DEPLOYMENT"'"
  [ -f "$file" ] || { echo "helm deployment template not found"; exit 1; }
  grep -q "path: /ready" "$file" || {
    echo "readiness probe still hits /health, so readiness can never fail"
    exit 1
  }
'

check_required ".env.example documents the observability surface" bash -c '
  file="'"$ENV_EXAMPLE"'"
  for var in FI_FHIR_METRICS_ENABLED FI_FHIR_METRICS_PORT FI_FHIR_METRICS_ENDPOINT FI_FHIR_OBSERVABILITY_MODE; do
    grep -q "$var" "$file" || { echo "missing $var"; exit 1; }
  done
'

check_required "no deployment artifact selects the legacy observability mode" bash -c '
  root="'"$ROOT"'"
  if grep -rn "FI_FHIR_OBSERVABILITY_MODE" "$root/deploy" "$root/docker-compose.yaml" 2>/dev/null | grep -q "legacy"; then
    echo "a deployment artifact sets FI_FHIR_OBSERVABILITY_MODE=legacy, which disables /ready, metrics, and the multi-replica fixes"
    exit 1
  fi
'

check_required "no deployment artifact advertises unimplemented tracing" bash -c '
  root="'"$ROOT"'"
  # FI_FHIR_TRACING_* is parsed and validated by pkg/config and consumed by
  # nothing: there is no OpenTelemetry exporter in the serve path. A deployment
  # artifact that sets it tells an operator the deployment exports traces, and
  # the operator finds out otherwise during an incident. Slice 4.4a stripped
  # every such setting; this keeps them from coming back before the exporter
  # does (4.4d). Comment lines are fine — the point is the label, not the string.
  offenders=""
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    text="${line#*:}"   # strip path
    text="${text#*:}"   # strip line number
    text="${text#"${text%%[![:space:]]*}"}"
    case "$text" in
      "#"* | "--"* | "//"*) continue ;;
    esac
    offenders="${offenders}${line}
"
  done <<EOF
$(grep -rn "FI_FHIR_TRACING_" "$root/deploy" "$root/configs" "$root/docker-compose.yaml" 2>/dev/null || true)
EOF
  if [ -n "$offenders" ]; then
    echo "a deployment artifact sets an unimplemented tracing variable:"
    echo "$offenders"
    echo "FI_FHIR_TRACING_* is consumed by nothing (see docs/operations/README.md)."
    echo "Remove it, or land the exporter (slice 4.4d) in the same change."
    exit 1
  fi
'

check_required "no deployment artifact advertises unimplemented tracing in YAML form" bash -c '
  root="'"$ROOT"'"
  # The env-variable assertion above greps for the literal FI_FHIR_TRACING_ and
  # is structurally blind to the form that actually shipped:
  # deploy/kubernetes/base/configmap.yaml set `tracing_enabled: true` and
  # `tracing_sampler: 0.1` inside a config.yaml block mounted at /app/config —
  # the exact snake_case keys pkg/config binds (pkg/config/config.go, the
  # Observability struct). Same false claim, different syntax, invisible to the
  # gate that existed to catch it. Slice 4.4d closes the hole in both directions.
  #
  # Comment lines are fine: the point is the setting, not the string. A key set
  # to an explicit false is also fine — turning a thing off is not advertising it.
  offenders=""
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    text="${line#*:}"   # strip path
    text="${text#*:}"   # strip line number
    text="${text#"${text%%[![:space:]]*}"}"
    case "$text" in
      "#"* | "--"* | "//"*) continue ;;
    esac
    case "$text" in
      *"tracing_enabled"*"false"* | *"tracingEnabled"*"false"*) continue ;;
    esac
    offenders="${offenders}${line}
"
  done <<EOF
$(grep -rnE "(tracing_enabled|tracing_sampler|tracing_endpoint|tracingEnabled|tracingSampler|tracingEndpoint)[[:space:]]*:" "$root/deploy" "$root/configs" "$root/docker-compose.yaml" 2>/dev/null || true)
EOF
  if [ -n "$offenders" ]; then
    echo "a deployment artifact configures tracing in YAML, which no exporter consumes:"
    echo "$offenders"
    echo "pkg/config binds these snake_case keys, so setting them is the same claim"
    echo "FI_FHIR_TRACING_* makes. Remove them, or land the exporter (slice 4.4d)"
    echo "in the same change."
    exit 1
  fi
'

check_required "batch worker identity is not a shared literal" bash -c '
  file="'"$ENV_EXAMPLE"'"
  value=$(grep -E "^[[:space:]]*FI_FHIR_BATCH_WORKER_ID=" "$file" | head -1 | cut -d= -f2- | tr -d "\"'"'"' ")
  if [ -n "$value" ]; then
    echo "publishes FI_FHIR_BATCH_WORKER_ID=$value; two replicas sharing a lease owner process the same object concurrently"
    exit 1
  fi
'

# --------------------------------------------------------------------------
# 4. Key runtime vars documented
# --------------------------------------------------------------------------
echo ""
echo "─── Key Variables ───"
echo ""
echo "  The following env vars control proxy/runtime behavior:"
echo ""
echo "  FI_FHIR_ADDR            Listen address for serve command (default :8080)"
echo "  FI_FHIR_UI_API_ORIGIN   Backend origin for UI reverse proxy"
echo "  VITE_FI_FHIR_PREVIEW_INTEGRATION_ID Public preview registry alias"
echo "  FI_FHIR_DEPLOYMENT_TENANT_ID Preview deployment tenant"
echo "  FI_FHIR_GRAPHQL_BEARER_TOKEN or _FILE  Preview credential source"
echo "  FI_FHIR_GRAPHQL_ALLOWED_ORIGINS Exact browser origins"
echo "  FI_FHIR_INTEGRATION_REGISTRY_PATH Immutable preview registry"
echo "  FI_FHIR_DATABASE_URL    PostgreSQL connection string"
echo "  FI_FHIR_DATABASE_HOST   PostgreSQL host (alternative to URL)"
echo "  FI_FHIR_DATABASE_NAME   PostgreSQL database name"
echo "  FI_FHIR_DATABASE_USER   PostgreSQL username"
echo "  FI_FHIR_DATABASE_SSL_MODE  SSL mode (default: disable in-cluster)"
echo "  FI_FHIR_TERMINOLOGY_DB_URL Terminology database connection"
echo ""

# --------------------------------------------------------------------------
# Summary
# --------------------------------------------------------------------------
echo ""
echo "Results: $passed passed, $warned warnings, $failed failures"
echo ""

if [ "$failed" -gt 0 ]; then
  echo "❌ Required runtime config checks failed"
  exit 1
fi

if [ "$warned" -gt 0 ]; then
  echo "⚠ Some config checks had warnings (non-blocking)"
fi

echo "✅ Runtime config check complete"
exit 0
