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

go_vars=$(grep -roh 'FI_FHIR_[A-Z_]*' "${ROOT}/cmd/" "${ROOT}/internal/" "${ROOT}/pkg/" 2>/dev/null \
  | grep -v '_test.go' | sort -u || true)

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
else
  echo "  [docker-compose.yaml] ... ⚠ file not found"
  ((warned++))
fi

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
# 3. Key runtime vars documented
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
