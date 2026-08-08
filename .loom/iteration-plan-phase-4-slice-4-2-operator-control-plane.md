# RALPH Iteration Plan — Phase 4 Slice 4.2 Operator Control Plane

## Review

- Roadmap milestone: Phase 4 Slice 4.2 — operator control plane.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  golden journey 2 (failure and replay) and golden journey 5 (operator audit);
  `.loom/30-implementation-plan-integration-engine-ide-completion.md` Phase 4
  Slice 4.2.
- Prior decisions to preserve:
  - Slice 2.3 owns durable delivery attempts, retry schedules, circuit state,
    the DLQ, idempotent operator operations, and the append-only audit table.
    New control actions reuse that machinery instead of forking a second path.
  - Slice 2.1 owns the closed lifecycle state machine, expected-version
    optimistic concurrency, and reason-required human commands. Deployment and
    channel controls expose those commands; they do not add new state logic.
  - Slice 4.1a owns verified caller identity. Tenant, principal, roles, and
    auth method are server-owned; requests cannot assert them.
  - Raw clinical payloads are never returned by an API surface by default.
    Preview and lineage already project PHI-minimal semantics only.

## Align

- Slice name: operator control plane over the existing durable engine records,
  shipped as two sequential merge requests.
- Split:
  - **4.2a (MR 1)** — control-plane GraphQL API: bounded tenant-scoped read
    queries plus reason-required, role-gated, idempotent control mutations.
  - **4.2b (MR 2)** — operator UI in the IDE consuming 4.2a.
- Scope in (4.2a):
  - `internal/integration/operator`: a PostgreSQL read surface over
    `integration_receipts`, `integration_canonical_events`,
    `integration_message_lineage`, `integration_delivery_attempts`,
    `integration_delivery_outbox`, `integration_delivery_dlq`,
    `integration_delivery_circuits`, and `integration_delivery_audit`, plus a
    control service that delegates writes to `internal/integration/delivery`
    and `internal/integration/lifecycle`.
  - Keyset pagination with opaque stable cursors, a bounded default page size
    of 25 and hard maximum of 100, and filters for integration artifact,
    status, correlation ID, source message ID, and a time window.
  - PHI-minimal projections only. Canonical event payloads are rendered as a
    bounded, values-free structural summary: dotted key paths, JSON kinds, and
    array flags. Non-canonical key names collapse to `*`. No value, no length,
    and no raw byte ever leaves the process.
  - Mutations `replayDelivery`, `resubmitMessage`, `discardDeadLetter`,
    `pauseIntegrationDeployment`, `resumeIntegrationDeployment`,
    `retireIntegrationDeployment`, and `deployIntegrationRelease`.
  - DLQ requeue is `replayDelivery`: `delivery.PostgresStore.Replay` already
    requires an active dead letter and requeues the exact failed attempt.
    Discard is the one genuinely new durable operation.
  - Migration `0003_operator_control_plane.sql` extends the delivery operation
    kind and audit event kind check constraints and records a DLQ resolution so
    an inactive dead letter is honest about why it closed.
  - Explicit privileged role checks in the control service, fail closed, with
    role names recorded in the decision log.
  - Schema changes only through `make lint-gqlgen`; generated files are never
    hand-edited.
- Scope in (4.2b):
  - A new `ui/src/lib/features/operator` area: receipt/message browser with
    trace drill-down, delivery attempts with circuit and DLQ views, and
    reason-required control dialogs including expected-version conflict
    surfacing.
  - Pure helper modules with unit tests, inline error homes, `isErrorToasted`
    guards on every catch that toasts, explanatory `title` on disabled
    controls, and honest loading/error/empty states.
- Scope out:
  - Multi-replica fanout, `/health`, `/ready`, `/metrics`, correlation-safe log
    plumbing, and durable subscription fanout (Slice 4.3).
  - Backup, restore, migration compatibility, and performance budgets (4.4).
  - New identity modes, token issuance, batch workload identity (4.1b3),
    GitOps activation, and WebSocket enablement.
  - Raw payload retrieval, export controls, and retention administration.
- Acceptance criteria:
  - An authorized operator browses a durable failed delivery, its trace
    lineage, and its DLQ entry entirely through GraphQL, with no SQL and no
    filesystem access.
  - Replay through the mutation records a new durable attempt plus an
    append-only audit row carrying actor, reason, and idempotency key, and a
    duplicate call with the same key does not double-execute.
  - An unprivileged role fails before resolver data; a cross-tenant operator
    sees nothing and mutates nothing.
  - A raw-PHI sentinel planted in the seeded message never appears in any
    GraphQL response.
  - Lifecycle controls reuse expected-version concurrency and surface conflicts
    rather than silently retrying.
- Dependencies/blockers:
  - The delivery audit and operations tables constrain their kind columns, so a
    genuinely new discard operation needs a migration rather than a new table.
  - `lint:gqlgen` is slow on a cold module cache; codegen must be run and
    committed locally so CI has nothing to reconcile.

## Riskiest assumption + kill-test

**Load-bearing assumption**: the failure/replay and operator-audit golden
journeys can be completed entirely through authenticated GraphQL over the
Slice 2.3 durable records — that is, the existing attempt/outbox/DLQ/operations/
audit tables already contain every fact an operator needs, and the existing
idempotent replay path can be driven by a request-scoped verified identity
instead of a PostgreSQL session `current_user`, without weakening idempotency,
tenant isolation, or the raw-free guarantee.

**Kill test** (required CI job, PostgreSQL 16): seed a durable failed delivery
whose canonical event payload contains a raw-PHI sentinel through the existing
processor fixtures. Then, through the real GraphQL handler with a Slice 4.1a
OIDC token:

1. browse the message, its trace lineage, its delivery attempts, and its DLQ
   entry using only GraphQL queries;
2. replay it with a reason and idempotency key, and assert a requeued durable
   attempt plus an append-only audit row carrying actor, reason, and key;
3. repeat the identical mutation and assert the operation count and attempt
   state do not change;
4. assert an unprivileged role is refused before any resolver data;
5. assert a cross-tenant record is invisible to queries and unmutatable;
6. assert the planted sentinel appears in no GraphQL response body.

**Failure mode if the assumption is wrong**: the operator UI would either need
raw SQL behind it, or a second write path that bypasses the idempotency and
audit guarantees Slice 2.3 established — reintroducing exactly the duplicate
durable work the product spec calls a P0.

**Status**: to be recorded after the required job passes.

## Land

- Planned file areas (4.2a):
  - `internal/integration/operator/` (new package)
  - `internal/integration/delivery/` (discard operation, DLQ resolution)
  - `internal/integration/lifecycle/` (deployment listing query)
  - `internal/integration/processor/migrations/0003_operator_control_plane.sql`
  - `internal/api/graphql/schema.graphql` and regenerated artifacts
  - `internal/api/graphql/resolvers/`
  - `cmd/fi-fhir/` runtime wiring
  - `.gitlab-ci.yml` (one new required job block only)
- Planned file areas (4.2b):
  - `ui/src/lib/features/operator/`, its route, and the IDE shell registration
- Implementation steps:
  1. Add the migration, the discard operation, and the lifecycle listing.
  2. Build the operator read store and the role-gated control service.
  3. Extend the GraphQL schema, run codegen, implement resolvers.
  4. Wire `serve`, write the kill-test, add the required CI job.
  5. Ship 4.2a; then branch 4.2b from fresh main and build the UI.

## Prove

- Tests to run:
  - `go test -race ./internal/integration/operator/... ./internal/integration/delivery/... ./internal/integration/lifecycle/... ./internal/api/graphql/...`
  - `go test -race ./...`
  - `go test -tags=integration -race -run '^TestOperatorControlPlane' ./internal/api/graphql`
  - `cd ui && npm ci && npm test -- --run && npm run typecheck` (4.2b)
- Lint/static checks:
  - `gofmt` on changed Go files, `golangci-lint run`, `go vet ./...`
  - `make lint-gqlgen` with a clean generated diff
  - `npm audit` clean for the UI merge request
- CI checks:
  - Required merge-request pipeline terminal green, including the new operator
    control-plane job, the blocking manual benchmark job, and every security
    gate; auto-merge armed only after a clean self-review.
  - The exact post-merge main pipeline harvested to terminal green.

## Handoff/Harvest

- Docs to update:
  - Slice 4.2 section of the canonical implementation plan, per merge request.
  - `.loom/40-decisions.md` for the role model and the payload-summary policy.
  - `.loom/50-worklog.md` dated entries.
  - `.loom/slice-handoff-phase-4-slice-4-2-operator-control-plane.md`.
- Next-slice candidates:
  - 4.3: truthful observability endpoints and multi-replica subscription fanout.
  - 4.4: backup/restore, rolling upgrade, and the numeric performance budgets.
