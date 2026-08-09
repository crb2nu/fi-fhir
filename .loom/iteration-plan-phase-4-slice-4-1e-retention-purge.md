# Iteration Plan — Phase 4, Slice 4.1e: Retention Policy and Purge Runtime

**Lane**: S4-B (`.loom/32-sprint4-execution-specs.md`)
**Branch**: `feat/phase4-slice-4-1e-retention-purge`
**Base**: `origin/main` @ `55412bdaa`
**Status**: implemented; day-1 gate shipped separately as MR !152
**Formerly**: "S3-C2" in `.loom/31:406,501`

---

## 1. Day-1 gate: the result that set the shape of the slice

`TestPhiRetention_PurgeIsStructurallyBlockedToday`, a standalone test-only MR
against unmodified `main` (`internal/integration/retention/purge_gate_integration_test.go`).

**Expected: pass. Actual: pass, all three assertions.**

| # | Assertion | Observed |
|---|---|---|
| a | `DELETE FROM integration_canonical_events` on a dependent-free row | raises `integration submission audit records are append-only` |
| b | `UPDATE integration_canonical_events SET payload_json = '{}'` | **also** raises the same guard |
| c | exported session: `DELETE FROM integration_session_exports` | raises `integration session exports are append-only` |
| c | exported session: `DELETE FROM integration_sessions` | raises SQLSTATE `23503`, naming `integration_session_exports` |

Row counts identical before and after every attempt.

(b) is the finding that re-shaped the slice. Slice 4.1d C1's guard is blanket
`BEFORE UPDATE OR DELETE` (`0004_audit_immutability.sql:29-32`), so it removed
**both** mechanisms a purge could use. Every planning document in front of this
lane — `docs/operations/PHI-RETENTION.md:189-192`, the C1 handoff at `:187-195`,
and `.loom/30:413-418` — described it as a `DELETE` problem. Corrections 11-13 of
`.loom/32` stand; nothing needed re-scoping, and all three documents were
corrected in this slice.

Assertions (a) and (c)'s first half deliberately aim at dependent-free rows: a
row with dependents would be refused by the existing `ON DELETE RESTRICT`
foreign keys, which proves referential integrity rather than immutability.

## 2. The two decisions, taken before any migration

Recorded in `.loom/40-decisions.md` (2026-08-08, "Slice 4.1e"), in the same MR as
the gate.

**Exemption: option A** — a column-scoped `BEFORE UPDATE` exemption permitting
only the canonical-tombstone update of the payload column plus `purged_at`;
`DELETE` stays blanket-blocked. Chosen because it keeps the project's posture
that the schema, not convention, is the guarantee; because it mirrors C1's own
`reject_integration_receipt_provenance_mutation` idiom; and because it survives
correction 12 without relaxing a single foreign key — nothing is deleted, so no
`ON DELETE RESTRICT` chain is in the way. **A tombstone is not a
backup-inclusive deletion**, and that is written into the decision, the
migration, the operator doc, and `.env.example` rather than left implied.

**Policy placement**: a new mutable, audited, per-tenant
`integration_retention_policies` record with a fail-closed deployment default of
retain-indefinitely. Not the revision contract (immutable and content-addressed;
the data outlives it, and a retention change must not mint a revision). Not
deployment config alone (no audit trail, no per-tenant scope).

**Filed, not built**: purge role separation (option C). Correction 16 empties it
as stated — the application role already owns the tables it guards.

## 3. What shipped

| Area | Change |
|---|---|
| `internal/integration/processor/migrations/0005_retention_expiry.sql` | `integration_retention_policies`, `integration_retention_policy_audit`, `integration_retention_purge_audit`, `purge_after`/`purged_at` on canonical events, partial index, the tombstone function, and the exemption |
| `internal/integration/session/migrations/0006_retention_expiry.sql` | `purge_after` on samples, `purge_after`/`purged_at` on exports, the export exemption, and the fanout-log prune with a 24 hour schema floor |
| `internal/integration/retention/` (new) | `Policy` + document decoding, `PostgresStore` (policy upsert, stamping, purge), `Purger` mirroring `autoroute.SweeperConfig`'s shape |
| `cmd/fi-fhir/retention_runtime.go` (new) | Env loading, policy upsert at startup, the metrics observer |
| `cmd/fi-fhir/main.go` | Appends only: `errCh` 9 → 10, `ComponentRetentionPurge` in the not-configured list, the component after the autoroute block, `waiting`, `componentMetricNames` |
| `internal/observability/metrics.go` | Appends only: `ComponentRetentionPurge`, `OutcomePurged`, two counters, the label allowlist |
| `docs/operations/PHI-RETENTION.md` | Sections 2, 3, and 6 rewritten; both drifted citations repaired (correction 19) |
| `internal/integration/processor/phi_retention_posture_integration_test.go` | Posture gate assertion inverted in the same commit (correction 18) |

### Per-class purge shapes

| Class | Shape | Why |
|---|---|---|
| `integration_canonical_events` | tombstone `payload_json`, set `purged_at` | Row deletion is structurally impossible (corrections 11-12), and the surviving row keeps the audit truthful about what existed |
| `integration_session_samples` | delete the row, ciphertext included | No immutability trigger exists or ever did; a tombstone would invent a guarantee |
| `integration_session_exports` | tombstone `record_json`, attribution frozen | The disclosure record is evidence; correction 13 makes the row undeletable anyway |
| `integration_session_stream_events` | prune, 24 hour schema floor, no audit row | Envelope log, no PHI: a growth control, and one audit row per envelope would replace one unbounded table with another |

### Task 4, decided rather than left open

The fanout log **is** pruned. The deployment window comes from the policy
record's `stream_event_retain`; the schema refuses to delete any envelope younger
than 24 hours no matter what a deployment configures, because the log is a
subscriber's resume cursor and a one-minute window would turn every reconnect
into a gap. `UPDATE` stays blanket-blocked.

## 4. Why no lease

The claim is the guarded statement. `UPDATE ... WHERE purged_at IS NULL ...
RETURNING`, with the audit `INSERT` reading from that `RETURNING` in the **same
statement**, means a purge without an audit row cannot be expressed and a second
replica's `UPDATE` matches nothing. `UNIQUE (tenant_id, record_class, record_id)`
on the audit is the schema-level backstop. This follows S3-A's rejection of
`pg_advisory_lock` for the autoroute notifier (`.loom/40-decisions.md`).

## 5. Kill-test and negative controls

`TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone`, PostgreSQL 16,
`-race`, two purge components against one database.

**A correction to the spec's control (recorded as correction 41 in `.loom/32`).**
The spec asks for one control on "the pre-migration database" where the purge
fails *and* step 4's mutations succeed. Those cannot come from one schema: on the
pre-4.1e schema C1's guards are active, so the mutations raise there too. The
control is split, which is strictly stronger:

- **pre-4.1e** (processor `0001`-`0004`, session `0001`-`0005`): the purge fails
  outright, and no shape of `UPDATE` tombstones a payload — so the primary
  proof's tombstone is attributable to this migration.
- **pre-C1** (processor `0001`-`0003`): every mutation the primary proof requires
  to raise **succeeds** — so the refusals are attributable to a guard, not to
  referential integrity or a malformed statement.

## 6. Verification

```bash
export POSTGRES_TEST_URL=...
make phi-retention-purge     # both proofs plus both negative controls
make phi-audit               # C1's kill-test and the rewritten posture gate
make integration-session
make delivery-reliability
make observability-replicas
make check-runtime-config
go test -race ./...
```

`test:phi-retention-purge` runs `make phi-retention-purge` **and** `make
phi-audit` in one job, because this lane amends C1's triggers on two tables and a
green purge beside a red C1 would mean the exemption ate the guarantee.

## 7. Follow-ups filed

1. **Purge role separation** — a de-privileged application role, a separate
   migration runner, and a purge role. The schema-enforced exemption guards
   against programmatic error, not against a hostile database role.
2. **Backup-copy purge interaction (Slice 4.4c)** — a tombstone does not reach a
   backup. Effective retention is `max(policy window, backup retention)` until DR
   policy accounts for it.
3. **Operator-facing purge status** — the GraphQL schema was frozen for Sprint 4.
   The purge audit is queryable in the database; a read model belongs to a later
   control-plane slice.
