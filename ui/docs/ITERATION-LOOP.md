# Iteration Loop (Repo Standards)

Use this loop for each UX increment to keep the UI modular, typed, and CI-safe.

## 1) Design (small + explicit)

- Define the smallest user-visible outcome (1 screen or 1 panel).
- Identify the source of truth:
  - GraphQL schema: `internal/api/graphql/schema.graphql`
  - OpenAPI spec: `api/openapi.yaml`
- Decide what is **client-only** vs requires a backend contract change.

## 2) Implement (vertical slice)

- Follow UI layering (`ui/docs/ARCHITECTURE.md`):
  - `src/lib/domain/*` for UI-owned types/helpers
  - `src/lib/features/<feature>/*` for feature state + components
  - `src/lib/ui/*` for shared primitives
- Keep routes thin: `src/routes/*` should primarily compose feature components.

## 3) Enforce contracts + strict typing

Run locally (and keep passing in CI):

```bash
cd ui
npm run codegen:check
npm run check
npm run typecheck
```

Backend contract drift checks (CI):
- `lint:gqlgen` ensures gqlgen output matches schema/resolvers.
- `lint:ui` ensures generated TS clients are in sync.

## 4) Ship

- Commit small and scoped.
- Prefer feature-flagging incomplete UX behind an “experimental” route rather than half-shipping a core path.

