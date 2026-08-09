### 2026-08-08 — Slice 4.1e lane claim and day-1 gate (retention policy + purge runtime)

- Lane: **S4-B** (`.loom/32-sprint4-execution-specs.md`, "Lane S4-B").
  Branch `feat/phase4-slice-4-1e-retention-purge`, worktree
  `.worktrees/phase4-slice-4-1e-retention-purge`, branched from `origin/main`
  @ `55412bdaa`.
- **Claimed migration numbers** (re-verified against `origin/main`'s migration
  directories, not against the spec — correction 40; re-verified again at every
  rebase):
  - `internal/integration/processor/migrations/0005_retention_expiry.sql`
    (`0001`-`0004` used; C1 took `0004_audit_immutability.sql`).
  - `internal/integration/session/migrations/0006_retention_expiry.sql`
    (`0001`-`0005` used; S3-A took `0005_session_stream_events.sql`).
- **Owned files this sprint** (one-owner rule from the spec's shared-file table):
  - `internal/integration/retention/**` (new package: purge component, store,
    both proofs).
  - `internal/integration/processor/migrations/0005_*` and
    `internal/integration/processor/postgres_submission.go` (migration embed and
    ledger entry only).
  - `internal/integration/session/migrations/0006_*` and
    `internal/integration/session/postgres.go` (migration embed and ledger entry
    only).
  - `internal/observability/metrics.go` — **appends only**:
    `ComponentRetentionPurge`, one bounded `Outcome`, and the matching entry in
    the PHI-label allowlist.
  - `cmd/fi-fhir/main.go` — **appends only** to S3-A's background-component
    table: one component after the autoroute block, `errCh` capacity, the
    not-configured list, `waiting`, and `componentMetricNames`. No restructuring.
  - `docs/operations/PHI-RETENTION.md` (rewrite of sections 2, 3, 6 plus the two
    drifted citations, correction 19).
  - `.gitlab-ci.yml`: appends `test:phi-retention-purge` at the end of the `test`
    stage only. `Makefile`: appends the `phi-retention-purge` target only.
- **Not touched** (sibling lanes): `internal/integration/delivery/store.go` and
  `dispatcher.go` and `internal/integration/destination/**` (S4-A);
  `pkg/terminology/db/**`, `test/e2e/**`, `deploy/**` (S4-C);
  `internal/api/graphql/operation_authorization.go` (S4-E);
  `internal/api/graphql/schema.graphql` and every generated artifact — **frozen
  for Sprint 4**; this lane's policy record is server-owned configuration and
  needs no root field.
- **Day-1 gate result: `TestPhiRetention_PurgeIsStructurallyBlockedToday` PASSES
  on unmodified `main`**, all three assertions, so corrections 11-13 stand and no
  re-scope of `.loom/32` is needed:
  - (a) `DELETE FROM integration_canonical_events` on a **dependent-free** row
    raises `integration submission audit records are append-only`.
  - (b) `UPDATE integration_canonical_events SET payload_json = '{}'` — the only
    other shape a purge can take — **also** raises the same guard. No planning
    document said so before this gate ran.
  - (c) For an exported session, `DELETE FROM integration_session_exports` raises
    `integration session exports are append-only`, and
    `DELETE FROM integration_sessions` raises SQLSTATE `23503` naming
    `integration_session_exports`. Both rows are permanently undeletable while
    `PHI-RETENTION.md:191` promises export TTL in the next slice.
  - Row counts identical before and after every attempt.
- **Both day-1 decisions recorded** in `.loom/40-decisions.md` (2026-08-08,
  "Slice 4.1e") **before any migration was written**: the immutability exemption
  is **option A**, a column-scoped `BEFORE UPDATE` exemption with canonical
  tombstone semantics and `DELETE` still blanket-blocked, carrying the explicit
  written consequence that *a tombstone is not a backup-inclusive deletion*; and
  retention policy lives in a new mutable, audited, per-tenant
  `integration_retention_policies` record with a fail-closed deployment default
  of retain-indefinitely — neither the immutable revision contract nor deployment
  config alone. Purge role separation (option C) is **filed, not built**.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` — Lane S4-B, corrections 11-20, 40
  - [S2] `.loom/40-decisions.md` — 2026-08-08, Slice 4.1e
  - [S3] `internal/integration/retention/purge_gate_integration_test.go`
