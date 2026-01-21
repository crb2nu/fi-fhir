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

## Build metadata

The production image bakes build metadata into the UI via build args:

- `VITE_BUILD_SHA`
- `VITE_BUILD_TAG`
- `VITE_BUILD_TIME`

