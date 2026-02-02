# Developer Guide (fi-fhir UI)

This UI is built for fi-fhir’s GraphQL server and expects these same-origin endpoints:

- `POST /graphql` (and `WS /graphql/ws`)
- `GET /health`
- `GET/PUT /api/profiles/*` (profile YAML helpers)

## Local development (recommended)

### 1) Start the API server

From the repo root:

```bash
go run ./cmd/fi-fhir serve --port 8081
```

Optional: build a binary first:

```bash
make build
./bin/fi-fhir serve --port 8081
```

### 2) Start the UI dev server (with API proxy)

The UI dev server proxies `/graphql`, `/api`, and `/health` to `VITE_API_ORIGIN` (default: `http://localhost:8081`).

```bash
cd ui
npm ci
VITE_API_ORIGIN=http://localhost:8081 npm run dev
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

