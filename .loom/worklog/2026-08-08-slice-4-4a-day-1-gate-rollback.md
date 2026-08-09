### 2026-08-08 - Slice 4.4a day-1 gate: rollback not-null

Lane S4-C's day-1 gate, landed deliberately red. This entry is the record the
lane spec requires before the fix may be written
(`.loom/32-sprint4-execution-specs.md`, "Lane S4-C — Kill-Test").

- What changed:
  - New test-only package `internal/integration/migrationcompat` (`doc.go`
    plus two `integration`-tagged test files). It holds no production code: the
    properties 4.4a must prove are statements about the six migration ledgers
    *together*, so they need a package that may import all of them without any
    of them importing it.
  - `TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback` —
    migrate the session ledger to head, then issue the exact pre-4.1d
    five-column `INSERT INTO integration_session_exports`.
  - `make migration-compatibility` and CI job `test:migration-compatibility`
    (dedicated `fi_fhir_migration_compat_test` PostgreSQL 16 service,
    `postgresql-client` installed for the round-trip proof that lands next),
    with an arity-1 `-list | rg -x | awk` existence guard.
  - The job is **`allow_failure: true` and that is temporary.** The
    implementation MR lands the fix and flips it to `allow_failure: false` in
    the same commit.

- Why:
  Reproducing a found defect *before* fixing it is what separates a regression
  guard from a test written after the code. `.loom/32` correction 23 claims
  one-version rollback is broken today by Slice 4.1d C1's own migration. If the
  gate had passed, or failed for any other reason, correction 23 would be wrong
  and the lane would have re-scoped before writing a line of production code.

- Evidence:
  Local run against PostgreSQL 16 (remote Docker context, `-race`,
  `POSTGRES_TEST_URL` pointed at a dedicated database):

  ```
  --- FAIL: TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback (1.23s)
      DAY-1 GATE CONFIRMED — one-version rollback is broken today, exactly as
      .loom/32-sprint4-execution-specs.md correction 23 predicts.
        SQLSTATE 23502 (not_null_violation) on column "principal_json"
          of relation "integration_session_exports"
        null value in column "principal_json" of relation
          "integration_session_exports" violates not-null constraint
  ```

  Predicted result: **fail**, on a not-null violation on `principal_json`.
  Observed result: **fail**, on a not-null violation on `principal_json`.
  Correction 23 stands; the lane proceeds as specified.

  Mechanism, confirmed in source:
  `internal/integration/session/migrations/0004_export_attribution.sql:31-34`
  runs `ALTER COLUMN principal_json/reason/include_raw_payload SET NOT NULL`
  with no `DEFAULT`. The current writer names all eight columns
  (`internal/integration/session/postgres.go:949-954`); a binary one version
  behind names five. Every `exportIntegrationBundle` from an N-1 replica fails
  during a rolling upgrade or after a rollback, so the product spec's budget 6
  — "one-version rolling upgrade and rollback preserve receipts, revisions, and
  resumable work without schema downgrade corruption" — is currently false.

- What's next:
  The Slice 4.4a implementation MR, which lands in one change:
  1. server-side `DEFAULT`s on the three columns, reusing the
     `unattributed_legacy_export` sentinel the migration already backfills with,
     so an N-1 insert succeeds and is *visibly* unattributed rather than
     failing (the 4.1b3 no-retroactive-vouching idiom, applied forward);
  2. `pg_advisory_xact_lock` in `pkg/terminology/db.Migrator.Initialize`, the
     one migrator of six that takes none (`.loom/32` correction 25);
  3. `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`
     (concurrent replica startup across all six ledgers, plus a
     `pg_dump`/restore round-trip that proves the C1 triggers and the 4.1c-a
     `NOT VALID` CHECK survive and a queued outbox row still publishes);
  4. the migration-authoring rule (a new `NOT NULL` column on an existing table
     carries a `DEFAULT`), which Lane S4-B's two migrations are the first
     consumers of;
  5. `test:migration-compatibility` → `allow_failure: false`, arity 2.

- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` — Lane S4-C, corrections 23-25
  - [S2] `internal/integration/session/migrations/0004_export_attribution.sql:20-38`
  - [S3] `internal/integration/session/postgres.go:949-954`
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md:279-280`
