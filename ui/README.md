# fi-fhir UI (SvelteKit)

This directory contains the (future) mapping frontend for fi-fhir.

![Mapping Studio Loop](../docs/mermaid/ui-mapping-flow.svg)

## Goals

- Strictly typed frontend (TypeScript + generated API clients).
- Strict contracts between frontend and backend:
  - GraphQL schema: `../internal/api/graphql/schema.graphql`
  - OpenAPI spec: `../api/openapi.yaml`
- CI enforces codegen drift (generated artifacts must be committed and up to date).

## Design docs

- `ui/docs/ARCHITECTURE.md`
- `ui/docs/FEATURES.md`
- `ui/docs/ITERATION-LOOP.md`
- `ui/docs/DEVELOPER-GUIDE.md`
- `ui/docs/USER-GUIDE.md`

## Commands

```bash
cd ui
npm install
npm run dev

# Type checks
npm run check
npm run typecheck

# Contract/codegen
npm run codegen
npm run codegen:check
```

### Local dev with a running API server

The UI expects same-origin `/graphql`, `/api`, and `/health`. When running `npm run dev`, the dev server proxies those paths to `VITE_API_ORIGIN` (default: `http://localhost:8081`):

```bash
VITE_API_ORIGIN=http://localhost:8081 npm run dev
```
