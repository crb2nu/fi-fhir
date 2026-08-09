### 2026-08-08 — Slice 4.1e: retention policy and purge runtime

- What changed:
  - **Schema.** `internal/integration/processor/migrations/0005_retention_expiry.sql`
    adds `integration_retention_policies` (mutable, attributed, versioned,
    per-tenant), `integration_retention_policy_audit` and
    `integration_retention_purge_audit` (both blanket-guarded append-only),
    `purge_after`/`purged_at` on `integration_canonical_events` with a partial
    index, the canonical tombstone function, and the exemption.
    `internal/integration/session/migrations/0006_retention_expiry.sql` adds
    `purge_after` to samples, `purge_after`/`purged_at` to exports with the same
    exemption shape over `record_json`, and turns the fanout log's blanket guard
    into blanket `BEFORE UPDATE` plus a `BEFORE DELETE` scoped past a 24 hour
    schema floor.
  - **Runtime.** New `internal/integration/retention` package: policy document
    decoding, `PostgresStore` (versioned policy upsert, expiry stamping, purge),
    and `Purger` mirroring `autoroute.SweeperConfig`'s shape exactly.
    `cmd/fi-fhir/retention_runtime.go` loads the policy the way the destination
    registry is loaded. `cmd/fi-fhir/main.go` gains **appends only** to Slice
    4.3's component table: `errCh` 9 → 10, `ComponentRetentionPurge` in the
    not-configured list, one component after the autoroute block, one `waiting`
    entry, one `componentMetricNames` entry.
  - **Observability.** `internal/observability/metrics.go` appends
    `ComponentRetentionPurge`, `OutcomePurged`, `fi_fhir_retention_purges_total`,
    `fi_fhir_retention_records_purged_total`, and the label-allowlist entry; the
    PHI-label test was extended in the same commit.
  - **Docs.** `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6 rewritten;
    both citations S3-A drifted repaired (`:303-305` → `:321-323`, `:306-317` →
    `:324-333`, `:889-893` → `:907-911`). The posture gate's
    `information_schema.columns` assertion inverted in the same commit.
- Why:
  - The day-1 gate proved a purge could be **neither** a `DELETE` nor a redaction
    `UPDATE`: Slice 4.1d C1's guard is blanket, and three `ON DELETE RESTRICT`
    chains terminating in undeletable state tables make row removal impossible
    regardless. The purge is therefore a tombstone under a column-scoped,
    schema-enforced exemption, decided in `.loom/40-decisions.md` before any
    migration was written.
  - Retention policy went into a mutable, audited, per-tenant record rather than
    the immutable revision contract or deployment config alone, so a retention
    change neither mints a revision nor loses its audit trail.
- Evidence:
  - `TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone` — PostgreSQL 16,
    `-race`, **two purge components concurrently**: exactly one tombstone and
    exactly one audit row per record, both PHI sentinels gone, the tombstoned row
    keeping its identity and classification, the export snapshot tombstoned with
    its disclosure attribution intact, and the sample row plus its ciphertext
    gone.
  - **Delivery interlock proved directly**: the event whose attempt was still
    queued was not purged, and `delivery.PostgresStore.Claim` afterwards returned
    that event with its real payload — never a tombstone — and published it.
  - **Two negative controls.** Pre-4.1e schema: the purge fails and no shape of
    `UPDATE` tombstones a payload. Pre-C1 schema: every mutation the primary
    proof requires to raise **succeeds**. See the correction below.
  - `make phi-audit` green against the migrated schema (C1's kill-test **and** the
    rewritten posture gate); `make integration-session`,
    `make observability-replicas`, `make check-runtime-config` all green.
    `make delivery-reliability` cannot run locally — it needs Kafka through
    testcontainers and there is no local Docker Desktop; CI is its proof.
- Correction filed to the plan:
  - **`.loom/32` correction 41** — the lane's negative control cannot come from
    one schema. The spec asked for one pre-migration database where the purge
    fails *and* step 4's mutations succeed; on the pre-4.1e schema C1's guards
    are active, so the mutations raise there too. Split into pre-4.1e and pre-C1,
    which is strictly stronger.
  - `.loom/30-implementation-plan…:413-418` and
    `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195` both framed
    the obstacle as a `DELETE` problem; both corrected in place.
- What's next:
  - Purge role separation (filed, not built — correction 16).
  - Backup-copy purge interaction for Slice 4.4c: a tombstone does not reach a
    backup, so effective retention is `max(policy window, backup retention)`.
  - Operator-facing purge status, once the GraphQL schema lock lifts.
- Sources:
  - [S1] `.loom/iteration-plan-phase-4-slice-4-1e-retention-purge.md`
  - [S2] `.loom/slice-handoff-phase-4-slice-4-1e-retention-purge.md`
  - [S3] `.loom/40-decisions.md` — 2026-08-08, "Slice 4.1e"
  - [S4] `docs/operations/PHI-RETENTION.md`
