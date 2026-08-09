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
export FI_FHIR_GRAPHQL_ROLES=integration:preview
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
described in `docs/operations/INTEGRATION-SESSIONS.md`. Include
`graphql:operator` in the API role list — it is the compatibility grant, and the
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

## Build metadata

The production image bakes build metadata into the UI via build args:

- `VITE_BUILD_SHA`
- `VITE_BUILD_TAG`
- `VITE_BUILD_TIME`
- `VITE_FI_FHIR_PREVIEW_INTEGRATION_ID` (a public registry alias, never a credential)
- `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED` (defaults to `false`)
