#!/usr/bin/env bash
# Browser smoke gate (IDE repair Lane R-D): provision, build, run.
#
# The whole of `test:ui-e2e` (ci/test-ui-e2e.yml) and of `make ui-e2e`, which
# runs this same script in the same image. Expects a Debian node image running
# as root (apt-get), the fi-fhir binary at bin/fi-fhir, and PostgreSQL 16
# reachable as $E2E_PG_HOST (default `postgres`, the CI service alias).
set -euo pipefail

UI_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
: "${PLAYWRIGHT_VERSION:?PLAYWRIGHT_VERSION must name the pinned @playwright/test version}"

section() { printf '\n[ui-e2e] ==== %s\n' "$*"; }

section "OS packages: nginx (serves the build via the production template), envsubst, psql"
# Debian's nginx postinst tries to start the service; nothing may bind :80.
printf '#!/bin/sh\nexit 101\n' >/usr/sbin/policy-rc.d
chmod +x /usr/sbin/policy-rc.d
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  ca-certificates curl gettext-base nginx postgresql-client >/dev/null
rm -rf /var/lib/apt/lists/*
nginx -v

section "npm ci"
cd "$UI_DIR"
npm ci --no-audit --no-fund

section "Playwright ${PLAYWRIGHT_VERSION} + Chromium"
installed=$(npx --no-install playwright --version | awk '{print $2}')
if [ "$installed" != "$PLAYWRIGHT_VERSION" ]; then
  echo "[ui-e2e] ui/package.json pins @playwright/test $installed but PLAYWRIGHT_VERSION is $PLAYWRIGHT_VERSION;"
  echo "[ui-e2e] update ci/test-ui-e2e.yml (it keys the browser cache) and Makefile together."
  exit 1
fi
npx --no-install playwright install --with-deps chromium

section "Build the UI with the session engine on"
# The engine still runs only when the API reports capabilities.integrationSessions
# (resolveIntegrationSessionEngine); the sessions-off stack proves that.
VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true \
VITE_FI_FHIR_PREVIEW_INTEGRATION_ID=adt-east \
  npm run build

section "Type-check the specs"
npx --no-install tsc -p e2e/tsconfig.json

section "Run the three stacks and Playwright"
# Arguments narrow the Playwright run for local iteration (e.g. --project
# sessions-off); CI passes none, which also turns on the existence guard.
bash e2e/run.sh "$@"
