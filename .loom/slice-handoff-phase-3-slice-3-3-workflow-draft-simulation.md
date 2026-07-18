# Phase 3 Slice 3.3 Handoff — Workflow Draft Simulation

## Outcome

Slice 3.3 adds durable, exact-revision workflow simulation over explicit
immutable Integration Session runs. Workflow Builder saves the current YAML as
an append-only workflow revision, asks the server to plan session-owned events,
and renders revision provenance, event/route/transform/action traces, and the
deterministic delta from the latest simulation over the same run set.

## Safety boundary

- The server reuses the production pure `workflow.Planner`; it never dispatches
  transform or action handlers.
- The browser sends session, workflow revision, run, and optional baseline IDs,
  not event JSON.
- Persisted and GraphQL traces contain no raw sample, event payload, transformed
  value, action configuration, or secret.
- Simulations reference one exact workflow revision ID and SHA-256 digest and
  survive PostgreSQL store/service reconstruction.
- Existing feature gates and temporary `graphql:operator` authorization remain
  in force; publication and deployment are not implied.

## Verification

- `go test ./...`: passed.
- `go vet ./...`: passed.
- targeted workflow/session/GraphQL race tests: passed.
- PostgreSQL 16 `make integration-session` restart/leakage/side-effect kill test:
  passed with an external PostgreSQL URL.
- UI Svelte check: 0 errors; 9 pre-existing unused-selector warnings.
- UI tests: 636 passed, 3 skipped; 73 files passed, 1 skipped.
- UI ESLint, stylelint, component contract test, and production build: passed.
- Backend and UI GraphQL generation: deterministic.

## Remaining work

- Phase 3 Slice 3.4: signed/versioned publication, approval, and deployment of
  exact tested revisions.
- Production GitOps activation remains intentionally pending.
- Durable cross-replica session stream fanout/replay remains Phase 4 work.

## Landing evidence

- Pending MR and post-merge main pipeline evidence.
