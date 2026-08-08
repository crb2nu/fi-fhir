# Slice Handoff — Phase 4 Slice 4.2 Operator Control Plane

**Date**: 2026-08-08
**Plan**: `.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`
**Shipped as**: two sequential merge requests (4.2a backend, 4.2b UI)

## What shipped

### 4.2a — control-plane GraphQL API

`internal/integration/operator` is a read projection plus a delegation layer. It
owns no delivery or lifecycle state: every write goes to Slice 2.3's idempotent
operation ledger and append-only audit trail, or to Slice 2.1's closed lifecycle
state machine.

- **Read**: nine tenant-scoped queries over receipts, canonical events,
  receipt-to-delivery lineage, delivery attempts (joined with outbox and DLQ
  state), dead letters, destination circuits, delivery audit, the deployment
  inventory, and lifecycle history. Keyset pagination with opaque cursors,
  default page size 25, hard maximum 100. Filters for integration/destination
  artifact, status, correlation ID, MSH-10 source message ID, and time window.
- **Semantic payload rendering**: canonical event payloads return field
  coordinates, JSON kinds, and repetition flags — never a stored value, never a
  value length, and never a caller-influenced map key (non-canonical keys
  collapse to `*`).
- **Write**: `replayDelivery`, `resubmitMessage`, `discardDeadLetter`, and the
  lifecycle `pause`/`resume`/`retire`/`deploy` commands. Each requires a nonempty
  actor reason, uses the verified OIDC identity as the actor, enforces an
  explicit privileged role, and applies the underlying idempotency or
  expected-version guarantee unchanged.
- **DLQ requeue is `replayDelivery`** — Slice 2.3's `Replay` already refuses
  anything that is not an active dead letter. `discard` is the only genuinely
  new durable decision, so it is the only one that needed a migration.

### 4.2b — operator UI

`/operator` in the IDE: message browser with server-owned filters and cursors,
trace drill-down, dead-letter and circuit console, and deployment controls. One
reason-required dialog fronts every mutating action.

## Roles enforced

| Role | Grants |
|---|---|
| `integration.operator` | the entire bounded read surface |
| `integration.delivery.operator` | replay / resubmit / discard (existing Slice 2.3 constant, enforced again inside the durable operation) |
| `integration.deployment.operator` | pause / resume / retire / deploy |

Reads are required for writes, so a control action always implies the ability to
inspect what it changed. An operator token also needs the pre-existing
`graphql:operator` role to clear the legacy operation-authorization gate.

## Kill-test result

`TestOperatorControlPlane_FailureReplayAndAuditGoldenJourneys` (required CI job
`test:operator-control-plane`, PostgreSQL 16, `-race`) drives the real GraphQL
handler with a Slice 4.1a OIDC operator token.

| Assertion | What proved it |
|---|---|
| Failure/replay and operator-audit journeys complete without SQL | Every browse and mutation in the test body is a GraphQL request through `server.Handler()`; SQL appears only in fixtures and post-hoc verification |
| Replay produces one durable attempt plus audit and ledger rows | `readDeliveryState` shows `queued`/`pending`/DLQ-inactive; `readAudit(..., "replayed")` has exactly 1 row carrying actor + reason; `readOperations` has 1 row carrying the idempotency key |
| A duplicate mutation does not double-execute | Identical second call returns the same attempt; durable state struct compares equal; audit and ledger row counts unchanged |
| A spent key cannot be reused for another action | `resubmitMessage` with the replay key returns `operator operation idempotency conflict` |
| Resubmit forks exactly one idempotent child | Child attempt has `parentAttemptId` = source, source DLQ closes as `resubmitted`, duplicate resubmit returns the same child, ledger holds 2 rows |
| Discard closes a dead letter attributably | Resolution `discarded`, attempt stays `failed`, audit row carries actor + reason |
| Optimistic concurrency surfaces, never silently retries | Stale `expectedVersion` returns `integration deployment version conflict`; pause/resume then succeed and record actor + reason |
| Unprivileged roles never reach resolver data | Preview role gets `FORBIDDEN` at operation authorization; a `graphql:operator`-only caller passes that gate and is refused by the service; a read-only operator cannot mutate and writes no ledger row |
| Cross-tenant isolation is data-scoped | Tenant-b's receipt/attempt return null, its replay is refused, its durable state is unchanged, and a tenant-b token fails authentication with 401 |
| No raw PHI leaves the process | A sentinel proven present in the durable payload (`assertSentinelIsDurablyStored`) appears in none of the recorded response bodies |

**Negative control**: making the payload summarizer emit scalar values makes the
job fail on the sentinel assertion, so the leak check is not vacuous.

**Defect caught before merge**: the required delivery-reliability proof rejected
a DLQ resolution label built by string concatenation (`"resubmit" + "ed"` →
`resubmited`, an invalid CHECK value). Fixed, and the kill-test now exercises
resubmit directly so the path is guarded.

## Evidence

| Item | 4.2a |
|---|---|
| MR | `!136` |
| Merge commit | `7111cca1` |
| MR pipeline | `22548` — 38/38 passed |
| Post-merge main pipeline | `22560` — 42/42 passed |

4.2b evidence is recorded in `.loom/50-worklog.md` and the 4.2 section of the
canonical implementation plan when its pipeline lands.

## Decisions recorded

`.loom/40-decisions.md`, 2026-08-08: "Scope Operator Control-Plane Authority and
Render Payloads Structurally" — delegation over a second write path, three
least-privilege roles, DLQ requeue as replay, and structure-only payload
rendering.

## Deliberately deferred

- **4.3**: multi-replica fanout, `/health`, `/ready`, `/metrics`,
  correlation-safe log plumbing, durable subscription fanout. The operator UI
  polls and reloads after each action rather than subscribing.
- **4.4**: backup/restore, migration compatibility, rolling upgrade, and the
  numeric performance budgets.
- Raw payload retrieval, export controls, and retention administration. The
  control plane never grows a value-returning field without a new decision.
- Attempt-level filtering UI for the delivery tab (the read surface supports it;
  the console currently browses the DLQ and circuits, which is what the failure
  journey needs).

## Gotchas for the next agent

- `lint:ui` runs `npm run codegen:check`, which regenerates
  `ui/src/lib/gen/graphql.ts` from the **Go** schema. Any `schema.graphql` change
  therefore requires committing the regenerated UI types, even in a
  backend-only MR — and that in turn activates the `ui-changes` rule, so the
  blocking `security:npm-audit-ui` job starts running on your MR.
- `make lint-gqlgen` and `npm run codegen:check` both compare against `HEAD`, so
  they report your own uncommitted work as "out of sync". Commit first, then run
  them the way CI does.
- The GraphQL error presenter allowlists exact message strings. A new
  catalog-safe error must be added there or it collapses to the generic
  "GraphQL request failed", which silently breaks conflict handling in the UI.
- Svelte 5: `component.$on(...)` is gone. Use
  `render(Component, { events: { name: fn } })`.
