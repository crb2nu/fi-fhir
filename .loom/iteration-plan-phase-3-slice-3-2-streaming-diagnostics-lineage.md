# RALPH Iteration Plan — Phase 3 Slice 3.2 Streaming Diagnostics and Server Lineage

## Review

- Roadmap milestone: Phase 3 durable IDE lifecycle, Slice 3.2.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  Golden Path 001 preview parity; `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  Slice 3.2.
- Prior decisions to preserve: PostgreSQL remains the durable session truth;
  browser data remains memory-only; raw samples are redacted before durable
  storage by default; production GitOps activation is separate; GraphQL
  mutations remain bounded authenticated POST operations.

## Align

- Slice name: streaming diagnostics and server lineage.
- Scope in:
  - opt-in, authenticated GraphQL subscription streaming over the existing
    bounded POST endpoint;
  - server-owned run/stage/diagnostic snapshots emitted after subscribing and
    reconciled with the terminal durable run;
  - deduplicated session diagnostics in the IDE Problems panel;
  - server lineage links that navigate to the matching HL7 inspector field;
  - explicit connecting, running, success, and stream-error states.
- Scope out:
  - workflow draft simulation, publication, promotion, deployment, and GitOps
    activation;
  - durable cross-replica subscription fanout and replay;
  - legacy/debug/event subscriptions outside Integration Sessions;
  - new PHI retention behavior or fine-grained Phase 4 RBAC.
- Acceptance criteria:
  1. A client can establish an authenticated, exact-origin, session-only stream
     before a preview mutation and observe ordered server stage progression.
  2. The terminal streamed snapshot matches the durable run returned by the
     mutation, including exact profile revision provenance.
  3. Streamed diagnostics are deduplicated by run and diagnostic identity and
     appear in Problems without duplicating legacy debug/session events.
  4. A diagnostic or lineage link selects the server-reported HL7 field in the
     inspector using canonical paths, including repeated OBX fields.
  5. Disabled streaming and unauthorized/non-session subscription operations
     continue to fail closed; raw retained samples are not included in stream
     envelopes.
- Dependencies/blockers: Slice 3.1 session store/runner; existing GraphQL
  authentication and operation authorization; browser `fetch` streaming.
- Risk notes:
  - Riskiest assumption: authenticated GraphQL SSE can deliver sufficiently
    low-latency stage snapshots while preserving the bounded POST boundary,
    avoiding the unbounded pre-authentication WebSocket frame risk deferred by
    Slice 1.
  - Subscription fanout is process-local in this slice. Every event contains a
    durable snapshot so missed progress can reconcile, but multi-replica fanout
    remains Phase 4.

## Land

- Planned file areas: `internal/integration/session`, `internal/api/graphql`,
  `cmd/fi-fhir`, `ui/src/lib/graphql`, `ui/src/lib/features/integration-session`,
  HL7/IDE Problems components, generated GraphQL types, tests, and operations
  documentation.
- Implementation steps:
  1. Canonicalize lineage paths and expose complete lineage on durable run
     snapshots.
  2. Add a feature-gated session-only SSE transport and transport-level
     authorization tests.
  3. Add the typed browser stream/session client, live run projection, Problems
     integration, and lineage navigation.
  4. Reconcile docs and executable checks without enabling production GitOps.

## UI Implementation Brief

- Surface: Mapping Studio HL7 preview Results, Events/Lineage, and IDE Problems.
- User goal: see what the server is doing during a session preview and jump from
  a server diagnostic or mapping lineage link to the exact source field.
- Primary action: Preview, followed by diagnostic/lineage inspection.
- Visual direction: compact clinical workbench; calm neutral progression,
  explicit text-plus-color status, existing tokens and panel language.
- Required states: disabled, connecting, running, success, error, empty,
  reduced-motion, keyboard focus, and narrow-screen wrapping.

## Prove

- Tests: session hub/runner and lineage unit tests; GraphQL resolver and
  authenticated SSE transport tests; integration-session API/store/component
  tests; generated-type checks.
- Lint/static: `gofmt`, `go vet`, targeted race tests, UI typecheck/check/lint,
  scoped security checks.
- Broader gates: `go test ./...`, UI test/build/codegen checks, repository
  documentation validation, and MR/main pipelines.

## Handoff/Harvest

- Docs to update: roadmap, implementation plan, decision log, status,
  changelog, and Integration Session operations guide.
- Agent-context entries: secure SSE choice, canonical lineage path contract,
  validation evidence, and remaining multi-replica limitation.
- Next-slice candidate: Phase 3 Slice 3.3 workflow draft simulation.

## Implementation Status

- Implemented on `codex/phase3-streaming-diagnostics-lineage` and merged as
  `36f2bb8c` through MR `!115`.
- MR pipeline `19464` passed 34/34, including required session job `187950` and
  benchmark job `187953`.
- Main pipeline `19482` passed 37/37 and independently repeated the session
  proof in job `188135`.
- Production feature gates remain disabled.
