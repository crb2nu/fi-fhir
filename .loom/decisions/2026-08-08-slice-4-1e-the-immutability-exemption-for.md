### 2026-08-08: Slice 4.1e — the immutability exemption for purge, and where retention policy lives

Two decisions, both required before this lane writes a migration
(`.loom/32-sprint4-execution-specs.md`, Lane S4-B, corrections 11-17). Both are
forced by the lane's day-1 gate,
`TestPhiRetention_PurgeIsStructurallyBlockedToday`, which **passes on unmodified
`main`**: a `DELETE` of a dependent-free canonical event raises, the redaction
`UPDATE` of `payload_json` raises too, and an exported session is undeletable at
both the export row (trigger) and the session row (foreign key, SQLSTATE 23503).
Purge is not a policy-design problem with a `DELETE` at the end of it. It is
structurally impossible today.

- Decision:
  - **1. Immutability exemption: option A — a column-scoped `BEFORE UPDATE`
    exemption with canonical tombstone semantics.** On
    `integration_canonical_events` and `integration_session_exports`, Slice 4.1d
    C1's blanket `BEFORE UPDATE OR DELETE` guard is replaced by a blanket
    `BEFORE DELETE` guard plus a `BEFORE UPDATE` guard that raises unless the
    update changes **only** the payload column and `purged_at`, sets the payload
    to the canonical tombstone object, and sets `purged_at` from a previously
    `NULL` value. Every other column stays frozen; `DELETE` stays blanket-blocked
    on both tables. This mirrors C1's own `reject_integration_receipt_provenance_mutation`
    idiom (`internal/integration/processor/migrations/0004_audit_immutability.sql:69-91`)
    rather than inventing a second convention.
  - **A tombstone is not a backup-inclusive deletion.** This is the written
    consequence the option carries, not a footnote: the row, its identity, its
    classification, and its `recorded_at` survive on purpose so an audit still
    shows what existed, and **any database backup taken before the purge still
    contains the payload**. Purge bounds retention in the live database only.
    Backup-copy expiry stays a database and storage-layer control operated
    outside this codebase, and is named as a Slice 4.4c interaction.
  - **`integration_session_samples` is deleted outright, not tombstoned.** It
    carries no immutability trigger (correction 14), so the honest purge for a
    retained sample is removal of the row and its `raw_cipher`. Applying a
    tombstone shape there would add a guarantee the table never had.
  - **2. Retention policy lives in a new mutable, audited, per-tenant
    `integration_retention_policies` record** — not in the revision contract and
    not in deployment configuration alone. The deployment supplies only a
    fail-closed default of **retain indefinitely**, so an unconfigured deployment
    purges nothing.
  - **3. Role separation for the purge (option C) is filed, not built.** It
    becomes a named follow-up slice, "purge role separation", in the Sprint 5
    list.
- Rationale:
  - Option A keeps the project's stated posture — *the schema, not convention, is
    the guarantee* — intact. The exemption is itself schema-enforced, is narrower
    than any role-based bypass, and survives correction 12 without touching a
    single foreign key: no `ON DELETE RESTRICT` chain has to be relaxed, because
    nothing is deleted.
  - It is also the only option that leaves an audit trail of what was purged. A
    deleted row proves nothing; a tombstoned row with `purged_at` and an
    append-only audit entry proves exactly what was removed, when, and under
    which policy version.
  - On policy placement: an integration revision is immutable and
    content-addressed, and the retained data outlives it. Putting retention in
    the revision contract would mean a retention change requires minting a new
    revision and redeploying — the policy would be pinned to the artifact that
    produced the data rather than to the tenant that owns it. Deployment
    configuration alone fails differently: no audit trail of who changed a PHI
    retention window and why, and no per-tenant scope in a schema that is
    per-tenant everywhere else.
  - Fail-closed "retain indefinitely" is the only safe default for a control
    whose failure mode is deleting clinical data. An operator must opt in to
    purging, per tenant, with an attributed policy record.
  - Correction 16 empties option C as stated: every migration runs on the same
    `*sql.DB` the runtime uses, so the application role already owns the tables
    and can `DROP TRIGGER` outright. A "privileged role that bypasses the guard"
    buys nothing while the ordinary role outranks it. Real separation means a
    de-privileged application role, a separate migration runner, and a purge
    role — three role changes and a deployment migration, which is its own slice.
- Alternatives considered:
  - **B. `SET session_replication_role = 'replica'` around the purge
    transaction** (rejected: one line, but it disables *every* trigger on *every*
    table for that session — all six C1 guards, the four lifecycle guards, and
    both session-workspace guards. It turns a scalpel into a switch, and it needs
    superuser or an explicit `GRANT SET ON PARAMETER`).
  - **C. A separate purge role that owns the tables and disables triggers**
    (rejected as stated — see correction 16 above; filed as a follow-up slice).
  - **D. Tombstone in a side table, leave the payload in place** (rejected: it
    purges nothing. The PHI stays. It fails the slice's only purpose while
    looking like progress).
  - **Retention policy on the revision contract, beside `RawRetentionPolicy`**
    (rejected: immutability — see rationale. `RawRetentionPolicy`
    (`pkg/integration/revision.go:108-157`) governs *production raw bytes*, which
    are rejected unless `ephemeral`, so it is a policy over an empty set and not
    a precedent for the data that actually persists).
  - **Retention policy in deployment configuration only** (rejected: no audit
    trail, no per-tenant scope).
  - **A lease or `pg_advisory_lock` around the purge scan** (rejected, following
    the S3-A precedent recorded above for the autoroute sweeper: the guarded
    `UPDATE ... WHERE purged_at IS NULL ... RETURNING` **is** the claim. Only the
    replica whose `RETURNING` yields the row writes its audit entry, in the same
    transaction, so two replicas produce one tombstone and one audit row without
    a new failure domain).
- Consequences:
  - `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6 stop being true the
    moment the expiry columns land, and the retention-posture gate
    (`TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy`)
    is **designed** to fail then. Rewriting both is a task in the implementation
    MR, not a surprise (correction 18).
  - C1's guarantee narrows in one specific, documented way: the canonical event
    payload and the export snapshot become replaceable-once, by a tombstone, with
    an audit row. Nothing else about C1 changes, and the five blanket-guarded
    ledgers stay blanket-guarded.
  - A pre-slice row has no policy. The expiry columns are `NULL`-able with **no
    backfill**: inventing a `purge_after` for data admitted before any policy
    existed would be retroactive vouching, the same reason 4.1b3 and C1 refused
    to backfill provenance.
  - The trigger function now contains real logic and must be reviewed as security
    code, not as schema boilerplate.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` — Lane S4-B, corrections 11-20
  - [S2] `internal/integration/processor/migrations/0004_audit_immutability.sql:29-32,69-91`
  - [S3] `internal/integration/processor/migrations/0001_atomic_submission.sql:52-54,73-75,90-92`
  - [S4] `internal/integration/session/migrations/0004_export_attribution.sql:55-58`; `migrations/0001_session_workspace.sql:88-90`
  - [S5] `internal/integration/retention/purge_gate_integration_test.go` — the day-1 gate, passing on unmodified `main`
  - [S6] `pkg/integration/revision.go:108-157`; `internal/integration/processor/postgres_submission.go:179-181`
  - [S7] `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195` — the `DELETE`-only framing this decision corrects
