Generated artifacts live in this directory.

- `graphql.ts` is generated from `../internal/api/graphql/schema.graphql` + `src/lib/graphql/**/*.graphql`.
- `openapi.ts` is generated from `../api/openapi.yaml`.

CI runs `npm run codegen:check` to ensure these are up to date.

