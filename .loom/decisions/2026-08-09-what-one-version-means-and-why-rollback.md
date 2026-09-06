### 2026-08-09: What "one version" means, and why rollback safety is a schema property

- Decision:
  - **The compatibility boundary is the per-package migration ledger version, not
    a git tag and not the binary version string.** There are zero git tags in
    this repository, and `main.version` is a build stamp (`-ldflags -X`,
    defaulting to `0.0.0-dev`) that carries a commit SHA — it says nothing about
    which database schema a process can run against.
  - "One version back" (N-1) means: **the schema at the previous version of one
    ledger, running the binary that expects that previous version.** Six ledgers
    exist — submission, session, lifecycle, batch, destination, terminology —
    and they advance independently, so N-1 is defined per ledger rather than
    across the product.
  - Each owning package exports `SchemaVersion`. `fi-fhir version` prints all
    six, and a new gauge `fi_fhir_schema_ledger_version{ledger}` publishes them,
    so two replicas mid-rolling-upgrade are distinguishable in Prometheus rather
    than only over SSH.
  - **A migration that makes an existing column `NOT NULL` must give it a
    `DEFAULT`.** Written into `AGENTS.md` ("Migration authoring") and
    `docs/developer-guide/testing.md`, and enforced mechanically by
    `TestMigrationRule_NotNullOnExistingColumnCarriesADefault`, which needs no
    database and runs in `test:unit`.
- Rationale:
  - The budget being satisfied is spec budget 6: "one-version rolling upgrade
    and rollback preserve receipts, revisions, and resumable work without schema
    downgrade corruption". It could not even be *stated* before this, because
    nothing defined a version.
  - The ledger version is the only number that answers the operational question.
    Two binaries built from different commits may expect identical schemas; two
    binaries reporting the same version string may not. Rollback safety is a
    property of the schema/writer pair, so the boundary must be the schema.
  - Writing the rule down was the assignment. Making it mechanical is what stops
    it being ignored: the rule already had two violations when it was written,
    and the one inside the one-version window was live.
- Alternatives considered:
  - **Start tagging releases and define N-1 as the previous tag** (rejected as
    the *primary* boundary: a tag is a packaging decision, and nothing prevents
    two consecutive tags from having identical or wildly divergent schemas.
    Tagging is worth doing, but it answers "which artifact" and not "can this
    binary run against this database". Filed for the release-gate work.)
  - **One global schema version across all six ledgers** (rejected: it would
    force a version bump on every package whenever any one of them migrated,
    and make an N-1 claim about session imply an untrue claim about batch.)
  - **Adding six labels to `fi_fhir_build_info`** (rejected: an info metric's
    labels all change together, whereas ledger versions move independently and
    the useful query compares one ledger across replicas.)
  - **Declaring one-version rollback unsupported**, which the spec required be
    presented and rejected explicitly. Rejected: it relaxes a product-spec target
    (`.loom/20-product-spec-integration-engine-ide-completion.md:249-250,279-280`)
    in exchange for avoiding a catalog-only `ALTER TABLE`. The defect cost three
    `DEFAULT`s to fix. There is no version of this trade that favours declaring
    the target unmet.
  - **Amending `0004_export_attribution.sql` in place** (rejected: the ledger
    records version 4 as applied, so an amended file would never re-run on any
    existing database and would only fix fresh installs. Amending an applied
    migration in a slice about migration discipline would be self-defeating.)
- Consequences:
  - Session ledger gains `0006_export_attribution_defaults.sql`. **Lane S4-B's
    session migration is therefore `0007`, not `0006`** as `.loom/32`'s
    file-ownership map assumed. The map has been corrected; the ledger at rebase
    is the authority (`.loom/32` correction 40).
  - `integration_session_exports` now has server-side defaults on
    `principal_json`, `reason`, and `include_raw_payload`. A writer that omits
    them records a *visibly unattributed* export instead of failing. That is a
    real loosening, and it is bounded by an assertion that the current writer
    still records a real principal, so the default cannot mask a live-path
    regression.
  - `pkg/terminology/db.Migrator.Initialize` now runs in a transaction under
    `pg_advisory_xact_lock`, matching the other five. `CurrentVersion` is read
    inside the lock; reading it outside would have left the race intact.
  - Two pre-existing violations of the new rule are recorded as a dated baseline
    in `knownRollbackUnsafeColumns` rather than silently tolerated:
    `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, both from processor ledger 2.
    Processor head is 4, so they are outside the one-version window; repairing
    them needs processor `0005`, which is Lane S4-B's number this sprint. Filed
    for 4.4c.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` Lane S4-C, corrections 23-25
  - [S2] `internal/integration/session/migrations/0004_export_attribution.sql:31-34`
  - [S3] `internal/integration/session/migrations/0006_export_attribution_defaults.sql`
  - [S4] `pkg/terminology/db/migrations.go` (`Initialize`, `currentVersionTx`)
  - [S5] `cmd/fi-fhir/schema_versions.go`; `internal/observability/metrics.go`
  - [S6] `internal/integration/migrationcompat/` (proof, controls, and rule test)
