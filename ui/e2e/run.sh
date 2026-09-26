#!/usr/bin/env bash
# Browser smoke gate (IDE repair Lane R-D): the runner.
#
# Starts three real `fi-fhir serve` processes against PostgreSQL, serves the
# BUILT UI (ui/build) through the production nginx template
# (ui/nginx/default.conf.template) once per API, and runs the Playwright
# projects in ui/playwright.config.ts against them:
#
#   stack                  UI     API     what differs
#   operator-bundle        :3000  :18081  the full operator bundle, sessions on
#   missing-operator-role  :3001  :18082  the bundle minus integration.operator
#   sessions-off           :3002  :18083  FI_FHIR_INTEGRATION_SESSION_ENABLED unset
#
# The fourth project, `visual`, reuses the operator-bundle stack and writes its
# review PNGs to $E2E_RESULTS_DIR/visual/.
#
# Each stack gets its own database, so the operator Messages list is empty by
# construction and no stack sees another's migrations.
#
# Trusted network: the browser and the Playwright request fixture both reach
# nginx on 127.0.0.1. The template sets no X-Real-IP and forwards
# X-Forwarded-For as $proxy_add_x_forwarded_for, i.e. "127.0.0.1" for a client
# that sent none; requestsecurity.trustedClientAddress reads X-Real-IP, then the
# FIRST X-Forwarded-For hop, then RemoteAddr — so the API sees 127.0.0.1 and
# FI_FHIR_GRAPHQL_TRUSTED_CIDRS=127.0.0.1/32 covers exactly this caller. The
# nginx access log in the artifacts records the forwarded value per request.
#
# Needs: bash, curl, psql + pg_isready, envsubst, nginx, node, the fi-fhir
# binary, a built ui/build, and PostgreSQL reachable as $E2E_PG_HOST. ci.sh
# provisions all of that on the CI image; `make ui-e2e` runs ci.sh in the same
# image. Everything the run leaves behind is under $E2E_RESULTS_DIR.
set -euo pipefail

UI_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_DIR=$(cd "$UI_DIR/.." && pwd)

E2E_API_BIN=${E2E_API_BIN:-$REPO_DIR/bin/fi-fhir}
E2E_PG_HOST=${E2E_PG_HOST:-postgres}
E2E_PG_PORT=${E2E_PG_PORT:-5432}
E2E_PG_USER=${E2E_PG_USER:-testuser}
E2E_PG_PASSWORD=${E2E_PG_PASSWORD:-testpass}
E2E_PG_ADMIN_DB=${E2E_PG_ADMIN_DB:-fi_fhir_e2e}
E2E_RESULTS_DIR=${E2E_RESULTS_DIR:-$UI_DIR/e2e-results}
E2E_REGISTRY_PATH=${E2E_REGISTRY_PATH:-$REPO_DIR/testdata/golden/integration/adt-http/preview-registry.json}
NGINX_BIN=${NGINX_BIN:-nginx}

# The documented operator bundle (docs/planning/GRAPHQL-API.md, "What an
# operator's token must carry"). The negative control drops exactly one role.
BUNDLE_ROLES=integration:preview,graphql:operator,clinical:read,integration.operator,integration.delivery.operator,integration.deployment.operator
NO_OPERATOR_ROLES=integration:preview,graphql:operator,clinical:read,integration.delivery.operator,integration.deployment.operator

#        name                   ui    api    database              roles               sessions
STACKS=(
  "operator-bundle       3000  18081  fi_fhir_e2e_bundle    $BUNDLE_ROLES       on"
  "missing-operator-role 3001  18082  fi_fhir_e2e_no_op     $NO_OPERATOR_ROLES  on"
  "sessions-off          3002  18083  fi_fhir_e2e_no_sess   $BUNDLE_ROLES       off"
)

log() { printf '[ui-e2e] %s\n' "$*"; }
die() { printf '[ui-e2e] FAILED: %s\n' "$*" >&2; exit 1; }

for tool in curl psql pg_isready envsubst node "$NGINX_BIN"; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is not on PATH (ci.sh installs it on Debian)"
done
[ -x "$E2E_API_BIN" ] || die "fi-fhir binary not found at $E2E_API_BIN (CI: test:binary artifact; local: make ui-e2e builds it)"
[ -f "$UI_DIR/build/index.html" ] || die "ui/build is missing; build the UI first (ci.sh does)"
[ -f "$E2E_REGISTRY_PATH" ] || die "integration registry not found at $E2E_REGISTRY_PATH"

rm -rf "$E2E_RESULTS_DIR"
mkdir -p "$E2E_RESULTS_DIR/logs"
RUNTIME_DIR=$(mktemp -d "${TMPDIR:-/tmp}/fi-fhir-ui-e2e.XXXXXX")

PIDS=()
# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
  local status=$?
  for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
    kill "$pid" 2>/dev/null || true
  done
  for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
    wait "$pid" 2>/dev/null || true
  done
  cp "$RUNTIME_DIR"/nginx/logs/*.log "$E2E_RESULTS_DIR/logs/" 2>/dev/null || true
  rm -rf "$RUNTIME_DIR"
  exit "$status"
}
trap cleanup EXIT

wait_for_url() {
  local url=$1 budget=$2 what=$3 log_file=${4:-}
  local deadline=$((SECONDS + budget))
  until curl -sf -o /dev/null "$url"; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      [ -n "$log_file" ] && tail -n 60 "$log_file" >&2
      die "$what did not answer $url within ${budget}s"
    fi
    sleep 0.5
  done
}

# --- PostgreSQL: one fresh database per stack -------------------------------
export PGPASSWORD=$E2E_PG_PASSWORD
admin_url="postgres://$E2E_PG_USER@$E2E_PG_HOST:$E2E_PG_PORT/$E2E_PG_ADMIN_DB?sslmode=disable"
deadline=$((SECONDS + 90))
until pg_isready -q -h "$E2E_PG_HOST" -p "$E2E_PG_PORT" -U "$E2E_PG_USER"; do
  [ "$SECONDS" -lt "$deadline" ] || die "PostgreSQL at $E2E_PG_HOST:$E2E_PG_PORT never became ready"
  sleep 1
done
for stack in "${STACKS[@]}"; do
  read -r _ _ _ database _ _ <<<"$stack"
  psql "$admin_url" -q -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS $database WITH (FORCE)" \
    -c "CREATE DATABASE $database"
done
log "databases ready on $E2E_PG_HOST:$E2E_PG_PORT"

# --- API: one fi-fhir serve per stack ---------------------------------------
start_api() {
  local name=$1 api_port=$2 database=$3 roles=$4 sessions=$5 ui_port=$6
  (
    # Nothing from the caller's environment may leak into what the stack is
    # meant to prove.
    while IFS= read -r variable; do unset "$variable"; done < <(compgen -e | grep '^FI_FHIR_' || true)
    export FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a
    export FI_FHIR_INTEGRATION_REGISTRY_PATH=$E2E_REGISTRY_PATH
    export FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=http://127.0.0.1:$ui_port
    export FI_FHIR_GRAPHQL_PRINCIPAL_ID=e2e-ide-operator
    export FI_FHIR_GRAPHQL_ROLES=$roles
    # Static mode requires a bearer; the browser never presents it (trusted
    # network), and this value is a test string, not a credential.
    export FI_FHIR_GRAPHQL_BEARER_TOKEN=e2e-browser-smoke-not-a-secret-000
    export FI_FHIR_GRAPHQL_TRUSTED_CIDRS=127.0.0.1/32
    # The operator plane exists on every stack (it only needs the database),
    # so each stack differs from the bundle stack in exactly one variable.
    export FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true
    if [ "$sessions" = on ]; then
      export FI_FHIR_INTEGRATION_SESSION_ENABLED=true
    fi
    export FI_FHIR_DATABASE_DRIVER=postgres
    export FI_FHIR_DATABASE_HOST=$E2E_PG_HOST
    export FI_FHIR_DATABASE_PORT=$E2E_PG_PORT
    export FI_FHIR_DATABASE_NAME=$database
    export FI_FHIR_DATABASE_USERNAME=$E2E_PG_USER
    export FI_FHIR_DATABASE_PASSWORD=$E2E_PG_PASSWORD
    export FI_FHIR_DATABASE_SSL_MODE=disable
    # Three processes share one host: a shared default metrics port would
    # make the second one fail to bind. Metrics are not what this proves.
    export FI_FHIR_METRICS_ENABLED=false
    exec "$E2E_API_BIN" serve --host 127.0.0.1 --port "$api_port" --no-playground
  ) >"$E2E_RESULTS_DIR/logs/api-$name.log" 2>&1 &
  PIDS+=("$!")
}

for stack in "${STACKS[@]}"; do
  read -r name ui_port api_port database roles sessions <<<"$stack"
  start_api "$name" "$api_port" "$database" "$roles" "$sessions" "$ui_port"
done
for stack in "${STACKS[@]}"; do
  read -r name _ api_port _ _ _ <<<"$stack"
  # Startup runs the database migrations before the listener opens.
  wait_for_url "http://127.0.0.1:$api_port/health" 180 "fi-fhir serve ($name)" "$E2E_RESULTS_DIR/logs/api-$name.log"
  log "api $name ready on :$api_port"
done

# --- nginx: the production template, once per stack --------------------------
# Rendered exactly as the nginx image's entrypoint renders it: envsubst limited
# to the one variable the template declares, so $host, $scheme and friends
# survive. Two lines are rewritten, each asserted to exist exactly once so a
# template change fails here rather than serving something else: the listen
# port (three instances on one host) and the document root (ui/build instead of
# the image's /usr/share/nginx/html).
TEMPLATE=$UI_DIR/nginx/default.conf.template
[ "$(grep -c '^    listen 3000;$' "$TEMPLATE")" = 1 ] || die "template no longer has exactly one 'listen 3000;'"
[ "$(grep -c '^    root /usr/share/nginx/html;$' "$TEMPLATE")" = 1 ] || die "template no longer has exactly one 'root /usr/share/nginx/html;'"

NGINX_DIR=$RUNTIME_DIR/nginx
mkdir -p "$NGINX_DIR/conf.d" "$NGINX_DIR/logs" "$NGINX_DIR/tmp"
for stack in "${STACKS[@]}"; do
  read -r name ui_port api_port _ _ _ <<<"$stack"
  # shellcheck disable=SC2016 # the literal is envsubst's SHELL-FORMAT argument
  FI_FHIR_UI_API_ORIGIN=http://127.0.0.1:$api_port envsubst '${FI_FHIR_UI_API_ORIGIN}' <"$TEMPLATE" \
    | sed -e "s|^    listen 3000;\$|    listen $ui_port;|" \
          -e "s|^    root /usr/share/nginx/html;\$|    root $UI_DIR/build;|" \
    >"$NGINX_DIR/conf.d/$name.conf"
  cp "$NGINX_DIR/conf.d/$name.conf" "$E2E_RESULTS_DIR/logs/nginx-$name.conf"
done

mime_types=""
nginx_conf_path=$("$NGINX_BIN" -V 2>&1 | sed -n 's/.*--conf-path=\([^ ]*\).*/\1/p')
for candidate in /etc/nginx/mime.types "$(dirname "${nginx_conf_path:-/etc/nginx/nginx.conf}")/mime.types"; do
  if [ -f "$candidate" ]; then mime_types=$candidate; break; fi
done
[ -n "$mime_types" ] || die "nginx mime.types not found (module scripts need application/javascript)"

user_line=""
if [ "$(id -u)" = 0 ]; then
  # CI runs as root; workers must read ui/build under the project directory.
  user_line="user root;"
fi
cat >"$NGINX_DIR/nginx.conf" <<EOF
$user_line
worker_processes 1;
pid $NGINX_DIR/nginx.pid;
error_log $NGINX_DIR/logs/nginx-error.log warn;
events { worker_connections 256; }
http {
    include $mime_types;
    default_type application/octet-stream;
    log_format e2e '\$server_port \$remote_addr "\$request" \$status forwarded_for="\$proxy_add_x_forwarded_for" accept="\$http_accept" ct="\$sent_http_content_type"';
    access_log $NGINX_DIR/logs/nginx-access.log e2e;
    client_body_temp_path $NGINX_DIR/tmp/client_body;
    proxy_temp_path $NGINX_DIR/tmp/proxy;
    fastcgi_temp_path $NGINX_DIR/tmp/fastcgi;
    uwsgi_temp_path $NGINX_DIR/tmp/uwsgi;
    scgi_temp_path $NGINX_DIR/tmp/scgi;
    include $NGINX_DIR/conf.d/*.conf;
}
EOF
"$NGINX_BIN" -p "$NGINX_DIR" -e "$NGINX_DIR/logs/nginx-error.log" -c "$NGINX_DIR/nginx.conf" -t
"$NGINX_BIN" -p "$NGINX_DIR" -e "$NGINX_DIR/logs/nginx-error.log" -c "$NGINX_DIR/nginx.conf" -g 'daemon off;' &
PIDS+=("$!")
for stack in "${STACKS[@]}"; do
  read -r name ui_port _ _ _ _ <<<"$stack"
  wait_for_url "http://127.0.0.1:$ui_port/ui/health" 30 "nginx ($name)" "$NGINX_DIR/logs/nginx-error.log"
  wait_for_url "http://127.0.0.1:$ui_port/health" 30 "nginx -> api ($name)" "$NGINX_DIR/logs/nginx-error.log"
  log "ui $name served on http://127.0.0.1:$ui_port"
done

# --- Playwright ---------------------------------------------------------------
cd "$UI_DIR"
set +e
E2E_RESULTS_DIR=$E2E_RESULTS_DIR \
E2E_BUNDLE_URL=http://127.0.0.1:3000 \
E2E_NO_OPERATOR_URL=http://127.0.0.1:3001 \
E2E_SESSIONS_OFF_URL=http://127.0.0.1:3002 \
  npx --no-install playwright test "$@"
playwright_status=$?
set -e

# Existence guard, as the Go proof jobs do with `go test -list`: every check,
# both negative controls and every visual capture must have run and passed. A
# renamed spec or a project that matched no file would otherwise make this job
# greener.
if [ "$#" -eq 0 ]; then
  node "$UI_DIR/e2e/check-report.mjs" "$E2E_RESULTS_DIR/report.json" || playwright_status=1
fi

if grep -l -E '^panic:|fatal error:' "$E2E_RESULTS_DIR"/logs/api-*.log >/dev/null 2>&1; then
  log "an API process panicked during the run:"
  grep -H -E '^panic:|fatal error:' "$E2E_RESULTS_DIR"/logs/api-*.log >&2 || true
  playwright_status=1
fi

exit "$playwright_status"
