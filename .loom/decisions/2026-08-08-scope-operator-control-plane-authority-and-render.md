### 2026-08-08: Scope Operator Control-Plane Authority and Render Payloads Structurally

- Decision:
  - Build the operator control plane as a read projection plus a delegation
    layer. `internal/integration/operator` owns no delivery or lifecycle state:
    every write goes to Slice 2.3's idempotent operation ledger and append-only
    audit trail, or to Slice 2.1's closed lifecycle state machine.
  - Split operator authority into three least-privilege roles rather than one:
    `integration.operator` for the bounded read surface,
    `integration.delivery.operator` (the existing Slice 2.3 constant) for
    replay/resubmit/discard, and `integration.deployment.operator` for
    pause/resume/retire/deploy. Reads are required for writes, so a control
    action always implies the ability to inspect what it changed.
  - Treat DLQ requeue as `replayDelivery` instead of adding a parallel action.
    Only discard is a genuinely new durable decision, so only discard needed a
    migration.
  - Render "policy-aware semantic payload" as structure only: dotted field
    coordinates, JSON kinds, and repetition flags. Object keys are emitted only
    when they match the engine's canonical field grammar; every other key
    collapses to `*`. No value, no value length, and no raw byte is returned.
- Rationale:
  - A second write path would have to re-implement idempotency, actor capture,
    and audit append. Duplicating that is exactly the duplicate-durable-work
    class the product spec calls a P0.
  - One combined operator role would let a read-only auditor mutate production
    traffic. Splitting the roles keeps the audit journey usable without granting
    recovery authority.
  - Field names in a canonical event are engine-authored schema; values are PHI.
    Emitting only the schema gives an operator real diagnostic signal while
    keeping the projection provably value-free, which a planted-sentinel test
    can verify rather than merely assert.
- Alternatives considered:
  - Return a redacted payload document (rejected: redaction is a denylist, and a
    new field defaults to exposed).
  - Expose value lengths or type-prefixed samples (rejected: length and prefix
    are identifying for MRNs and names).
  - One `integration.operator` role for everything (rejected: no least
    privilege, and the audit journey is the most widely granted one).
  - Add a separate DLQ requeue mutation (rejected: it is replay by another name
    and would fork the recovery contract).
- Consequences:
  - A deployment must issue three role claims to give one person full operator
    authority; existing tokens gain no control-plane access implicitly.
  - Discarding a dead letter is attributable and idempotent but not reversible;
    the attempt stays failed and the DLQ entry records `discarded`.
  - The GraphQL error presenter now allowlists the control plane's catalog-safe
    messages so an operator can distinguish a stale expected version from a
    spent idempotency key without learning whether an unseen record exists.
  - Raw payload retrieval, export controls, and retention administration remain
    out of scope; the control plane never grows a value-returning field without
    a new decision.
- Sources:
  - [S1] `internal/integration/operator/payload.go`
  - [S2] `internal/integration/operator/service.go`
  - [S3] `internal/integration/processor/migrations/0003_operator_control_plane.sql`
  - [S4] `internal/api/graphql/operator_control_plane_integration_test.go`
  - [S5] `.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`
