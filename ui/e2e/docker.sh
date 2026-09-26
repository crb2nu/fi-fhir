#!/usr/bin/env bash
# Browser smoke gate (IDE repair Lane R-D): the local mirror behind `make ui-e2e`.
#
# Runs ui/e2e/ci.sh — the whole CI job — inside the CI job's image on a docker
# context, with PostgreSQL 16 sharing the job container's network namespace:
# the topology of a GitLab Kubernetes-executor pod, where a service is reached
# on localhost and through its alias. Nothing but docker and go is needed on
# the host (no local nginx, no local Chromium), and the run is the job's run.
#
#   UI_E2E_DOCKER_CONTEXT  docker context to run on (default 7900xtx)
#   UI_E2E_NODE_IMAGE      job image (default: the CI image, through Harbor)
#   UI_E2E_PG_IMAGE        service image (default: the CI image, through Harbor)
#   UI_E2E_KEEP=1          leave the containers running for docker exec
#   UI_E2E_NAME            container name (default fi-fhir-ui-e2e-$USER; the
#                          PostgreSQL container is "$UI_E2E_NAME-pg"), so two
#                          worktrees can run at once on one docker host
#
# Results (report, junit, screenshots, traces, API and nginx logs) are copied
# back to ui/e2e-results/.
set -euo pipefail

UI_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_DIR=$(cd "$UI_DIR/.." && pwd)

CONTEXT=${UI_E2E_DOCKER_CONTEXT:-7900xtx}
NODE_IMAGE=${UI_E2E_NODE_IMAGE:-registry.harbor.lan/dockerhub-cache/library/node:22-bookworm}
PG_IMAGE=${UI_E2E_PG_IMAGE:-registry.harbor.lan/dockerhub-cache/library/postgres:16-alpine}
NPM_VERSION=${NPM_VERSION:-10.9.3}
# ci.sh checks the installed version against this; CI keys its browser cache on it.
PLAYWRIGHT_VERSION=${PLAYWRIGHT_VERSION:-$(sed -n 's/.*"@playwright\/test": "\([^"]*\)".*/\1/p' "$UI_DIR/package.json")}
NAME=${UI_E2E_NAME:-fi-fhir-ui-e2e-$(id -un)}
WORKDIR=/builds/fi-fhir

dk() { docker --context "$CONTEXT" "$@"; }

arch=$(dk info --format '{{.Architecture}}')
case "$arch" in
  x86_64 | amd64) goarch=amd64 ;;
  aarch64 | arm64) goarch=arm64 ;;
  *) echo "unsupported docker host architecture: $arch" >&2; exit 1 ;;
esac

echo "[ui-e2e] building linux/$goarch fi-fhir (the CI job uses the test:binary artifact)"
mkdir -p "$REPO_DIR/.cache/ui-e2e"
(cd "$REPO_DIR" && GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -o .cache/ui-e2e/fi-fhir ./cmd/fi-fhir)

# shellcheck disable=SC2329 # invoked by the EXIT trap
cleanup() {
  local status=$?
  if [ "${UI_E2E_KEEP:-}" != 1 ]; then
    dk rm -f "$NAME-pg" "$NAME" >/dev/null 2>&1 || true
  else
    echo "[ui-e2e] kept $NAME and $NAME-pg on context $CONTEXT"
  fi
  exit "$status"
}
trap cleanup EXIT

dk rm -f "$NAME-pg" "$NAME" >/dev/null 2>&1 || true
# `postgres` resolves to the shared network namespace, as the CI alias does.
dk run -d --name "$NAME" --add-host postgres:127.0.0.1 \
  -e CI=true -e PLAYWRIGHT_VERSION="$PLAYWRIGHT_VERSION" \
  -v fi-fhir-ui-e2e-npm:/root/.npm \
  -v fi-fhir-ui-e2e-ms-playwright:/root/.cache/ms-playwright \
  -w "$WORKDIR" "$NODE_IMAGE" sleep infinity >/dev/null
dk run -d --name "$NAME-pg" --network "container:$NAME" \
  -e POSTGRES_DB=fi_fhir_e2e -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
  "$PG_IMAGE" >/dev/null

echo "[ui-e2e] copying the working tree (ui/, the preview registry, the binary) into $NAME"
dk exec "$NAME" mkdir -p "$WORKDIR/bin"
# macOS tar would carry extended attributes (com.apple.provenance), which the
# daemon refuses to set, and AppleDouble ._ files; both flags are portable.
COPYFILE_DISABLE=1 tar --no-xattrs -C "$REPO_DIR" -cf - \
  --exclude 'ui/node_modules' --exclude 'ui/build' --exclude 'ui/.svelte-kit' \
  --exclude 'ui/e2e-results' --exclude 'ui/.npm' \
  ui testdata/golden/integration/adt-http \
  | dk cp - "$NAME:$WORKDIR"
dk cp "$REPO_DIR/.cache/ui-e2e/fi-fhir" "$NAME:$WORKDIR/bin/fi-fhir"

set +e
# The same before_script the CI node image runs (.gitlab-ci.yml .node-image).
dk exec "$NAME" bash -c "
  set -e
  export npm_config_registry=https://registry.npmjs.org NPM_CONFIG_REGISTRY=https://registry.npmjs.org
  npm i -g npm@$NPM_VERSION >/dev/null
  bash ui/e2e/ci.sh $*
"
status=$?
set -e

rm -rf "$UI_DIR/e2e-results"
dk cp "$NAME:$WORKDIR/ui/e2e-results" "$UI_DIR/e2e-results" >/dev/null 2>&1 || true
echo "[ui-e2e] results in ui/e2e-results (open ui/e2e-results/html/index.html)"
exit "$status"
