# Developer Guide (fi-fhir UI)

This UI is built for fi-fhir’s GraphQL server and expects these same-origin endpoints:

- authenticated `POST /graphql`
- `GET /health`

`/graphql/ws` and the legacy profile-YAML helper routes are unavailable in the
authenticated preview phase.

## Local development (recommended)

### 1) Start the API server

From the repo root:

```bash
export FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
export FI_FHIR_GRAPHQL_PRINCIPAL_ID=local-operator
export FI_FHIR_GRAPHQL_ROLES=integration:preview,graphql:operator
export FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=http://localhost:5173
export FI_FHIR_INTEGRATION_REGISTRY_PATH="$PWD/testdata/golden/integration/adt-http/preview-registry.json"
go run ./cmd/fi-fhir serve --port 8081 --no-playground --no-introspection
```

Optional: build a binary first:

```bash
make build
./bin/fi-fhir serve --port 8081
```

### 2) Start the UI dev server (with API proxy)

The UI dev server proxies only `/graphql` and `/health` to `VITE_API_ORIGIN`
(default: `http://localhost:8081`). Legacy `/api` profile-YAML calls fail locally
during authenticated preview hardening.

```bash
cd ui
npm ci
VITE_FI_FHIR_PREVIEW_INTEGRATION_ID=adt-east VITE_API_ORIGIN=http://localhost:8081 npm run dev
```

Paste the same bearer into the credential gate. The token and imported raw
samples stay only in tab memory and are cleared on reload.

To exercise durable session streaming, first enable the PostgreSQL workspace as
described in `docs/operations/INTEGRATION-SESSIONS.md`. The local role list above
already includes `graphql:operator` — it is the compatibility grant, and the
whole IDE surface including the session workspace still sits behind it, so the
narrowed operator control-plane roles are not a substitute here — then start the
UI with:

```bash
VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true \
VITE_API_ORIGIN=http://localhost:8081 \
npm run dev
```

## Contract/codegen workflow

```bash
cd ui
npm run codegen
npm run codegen:check
```

CI enforces that generated artifacts are committed and in sync with:

- GraphQL schema: `internal/api/graphql/schema.graphql`
- OpenAPI spec: `api/openapi.yaml`

## Testing

The UI uses **Vitest** with **@testing-library/svelte** for component testing.

### Run all tests

```bash
cd ui
npm test
```

### Watch mode (for development)

```bash
npm run test:watch
```

### Test coverage

```bash
npm run test:coverage
```

### Test conventions

- Tests live alongside components: `Component.svelte` → `Component.test.ts`
- Use `@testing-library/svelte` for rendering and querying
- Use `@testing-library/jest-dom` for DOM assertions
- Mock stores with `vi.mock()` when needed

### Component test example

```typescript
import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Badge from './Badge.svelte';

describe('Badge', () => {
  it('renders with default variant', () => {
    render(Badge, { props: { variant: 'info' } });
    expect(screen.getByRole('status')).toHaveClass('info');
  });
});
```

### Browser smoke gate (Playwright)

Vitest proves components against mocked stores. `test:ui-e2e`
(`ci/test-ui-e2e.yml`, blocking) proves the deployed shape instead: Chromium
against the **built** UI, served by `nginx/default.conf.template`, in front of
real `fi-fhir serve` processes on PostgreSQL 16, reached from the trusted
network (`127.0.0.1/32` — the address nginx forwards). It runs three stacks,
one Playwright project each:

| Project | API differs by | Proves |
|---|---|---|
| `operator-bundle` | — (full operator bundle, sessions on) | `/api/auth/status` capabilities; operator Messages list with no pre-flight or "forbidden"; an `integrationSessionEvents` SSE stream answers 200 `text/event-stream` and the HL7 run panel listens within 10 s, while Events → Live Stream shows `streaming-unavailable` for `eventStream`; Copilot "not configured"; no phantom Problems badge or Platform indicator |
| `missing-operator-role` | no `integration.operator` | `operator-preflight` names `integration.operator` and no operator query is sent |
| `sessions-off` | `FI_FHIR_INTEGRATION_SESSION_ENABLED` unset | `subscriptions == []`, `streaming: false`, HL7 intake shows `streaming-unavailable` for `integrationSessionEvents` and still previews on the stateless path |

Run it locally with Docker (no local nginx or Chromium needed):

```bash
make ui-e2e                                        # the CI job, on docker context 7900xtx
make ui-e2e UI_E2E_ARGS="--project sessions-off"   # one project
UI_E2E_KEEP=1 make ui-e2e                          # keep the containers for docker exec
```

`make ui-e2e` cross-builds `fi-fhir` for the docker host, then runs
`ui/e2e/ci.sh` — the job's whole script — in the job's image with PostgreSQL
sharing its network namespace. Results land in `ui/e2e-results/`: the HTML
report (`html/index.html`), screenshots and traces of failures
(`npx playwright show-trace ui/e2e-results/artifacts/<test>/trace.zip`), and
every API's and nginx's log. On a Linux host that already has nginx, psql and
a PostgreSQL, `ui/e2e/run.sh` runs the stacks directly (see its header for the
`E2E_*` variables).

The specs live in `ui/e2e/` (outside vitest's `src/**` include) and use the
`data-testid`s the honest surfaces carry: `operator-preflight`
(`data-missing-roles`), `streaming-unavailable` (`data-stream`,
`data-reason`), `copilot-llm-state` (`data-state`), `problems-badge`,
`platform-indicator`. `ui/e2e/check-report.mjs` fails the job if any check or
either negative control did not run.

## Build metadata

The production image bakes build metadata into the UI via build args:

- `VITE_BUILD_SHA`
- `VITE_BUILD_TAG`
- `VITE_BUILD_TIME`
- `VITE_FI_FHIR_PREVIEW_INTEGRATION_ID` (a public registry alias, never a credential)
- `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED` (image default `true`; `npm run dev`
  leaves it unset, which is off). The build flag only permits the Integration
  Session engine: the UI uses it when the API's `/api/auth/status` also reports
  `capabilities.integrationSessions`, and otherwise stays on the stateless
  preview path. Build with `=false` for a UI that never offers it.
