# Phase 3 Slice 3.2 Handoff — Streaming Diagnostics and Server Lineage

## Outcome

Slice 3.2 implements an opt-in live diagnostics path for durable Integration
Sessions. Mapping Studio subscribes before starting a preview, renders
server-owned stage progression, reconciles the immutable terminal run, surfaces
deduplicated diagnostics in Problems, and navigates canonical lineage into the
HL7 inspector.

## Security boundary

- Streaming uses authenticated GraphQL SSE on bounded `POST /graphql`.
- Only `integrationSessionEvents` and `sessionRunEvents` are permitted on SSE.
- `/graphql/ws` remains unmounted and returns 404.
- The backend and UI feature gates default to disabled.
- Session operations still require the temporary `graphql:operator` role.
- Stream session snapshots omit samples. Lineage projections omit persisted raw
  value previews.

## Verification

- `go test ./...`
- `go vet ./...`
- targeted `go test -race` for Integration Sessions, GraphQL, and the CLI
- golangci-lint: zero issues after clearing a stale removed-worktree cache
- gosec high-severity/high-confidence gate: passed
- UI Svelte check and TypeScript check: passed
- UI tests: 607 passed, 3 skipped; 68 files passed, 1 skipped
- UI ESLint, stylelint, and production build: passed
- npm audit: no high or critical findings; three existing low transitive findings
- GraphQL and UI generation completed deterministically
- `make docs-validate`: passed without warnings

## Remaining work

- Production GitOps activation remains intentionally pending.
- Durable cross-replica stream fanout/replay remains Phase 4 work.
- Phase 3 Slice 3.3 is workflow draft simulation against durable session data.

## Landing evidence

- MR `!115` pipeline `19464` passed 34/34, including required session job
  `187950` and benchmark job `187953`.
- MR `!115` merged as `36f2bb8c`.
- Main pipeline `19482` passed 37/37 and independently repeated the session
  proof in job `188135`.
