### 2026-08-09 - Slice 4.4a migration compatibility and rollback safety

Lane S4-C, first of Sprint 4's four lanes to merge. Follows the day-1 gate
recorded in `2026-08-08-slice-4-4a-day-1-gate-rollback.md`, which landed red on
purpose and merged as MR !151 (`4b7279e1`).

- Owned files (recorded before first commit, per `.loom/32` coordination rules):
  - `pkg/terminology/db/migrations.go`
  - `internal/integration/migrationcompat/**` (new, test-only)
  - `internal/integration/session/migrations/0006_export_attribution_defaults.sql`
    (**claimed migration number**, re-verified against `origin/main` at rebase:
    session head was `0005`, so `0006` was free) plus the embed and ledger step
    in `internal/integration/session/postgres.go`
  - one-line `SchemaVersion` constants in `processor/postgres_submission.go`,
    `lifecycle/postgres.go`, `batch/store.go`, `destination/postgres.go`
  - `cmd/fi-fhir/schema_versions.go` (new), three lines in `cmd/fi-fhir/main.go`
  - an additive gauge in `internal/observability/metrics.go`
  - `scripts/pgdump-roundtrip.sh` (new), one check in `scripts/check-runtime-config.sh`
  - `deploy/**`, `docker-compose.yaml`, `configs/full-stack.env`, `README.md`
  - `docs/operations/{SUPPORTED-1.0,PRODUCTION-HARDENING}.md`,
    `docs/developer-guide/testing.md`, `AGENTS.md`
  - append-only: `Makefile`, `.gitlab-ci.yml`

- What changed:
  - **Defined N-1.** The compatibility boundary is the per-package migration
    ledger version. Six ledgers, each exporting `SchemaVersion`; `fi-fhir
    version` prints all six and a new `fi_fhir_schema_ledger_version{ledger}`
    gauge publishes them. `main.version` becomes a var defaulting to
    `0.0.0-dev` instead of a release-looking `0.1.0`.
  - **Fixed the rollback defect.** `0006_export_attribution_defaults.sql` adds
    server-side defaults to the three columns `0004_export_attribution.sql` made
    `NOT NULL` without one, reusing the same `unattributed_legacy_export`
    sentinel `0004` backfills historical rows with.
  - **Locked the terminology migrator.** `Initialize` now runs in a transaction
    under `pg_advisory_xact_lock`, with the version re-read *inside* the lock.
  - **Made the migration rule mechanical.** `AGENTS.md` "Migration authoring"
    plus `TestMigrationRule_NotNullOnExistingColumnCarriesADefault`, which needs
    no database and runs in `test:unit`.
  - **Proved the restore round-trip.** `scripts/pgdump-roundtrip.sh` plus a
    proof that every durable row, every C1 immutability trigger, the 4.1c-a
    `NOT VALID` CHECK, and resumable delivery work all survive.
  - **Stripped the tracing façade** from all five deployment artifacts, with a
    `check-runtime-config` assertion that it cannot come back unlabelled.
  - **Stopped two documents contradicting each other** about the reference
    profile, and stated the RPO the documented backup actually achieves.
  - Promoted `test:migration-compatibility` to `allow_failure: false`, arity 3.

- Why:
  Product spec budget 6 — "one-version rolling upgrade and rollback preserve
  receipts, revisions, and resumable work without schema downgrade corruption" —
  was **false in already-merged code**, and the repository had no definition of
  "one version" with which to even state it.

- Evidence:
  - Day-1 gate reproduced the defect before the fix, locally and in CI:
    `SQLSTATE 23502 (not_null_violation) on column "principal_json"`. After
    `0006`, the same insert succeeds and produces a visibly unattributed row.
  - `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`:
    all assertions pass on PostgreSQL 16 with `-race`.
  - `TestMigrationCompatibility_NegativeControls`: all three confirm.
    - Defaults removed → the rollback insert fails again with 23502.
    - Lock held externally → `Initialize` waits 2.01s and applies nothing.
    - Pre-slice unlocked migrator under 4 concurrent replicas → reproduced on
      the first attempt: `pq: duplicate key value violates unique constraint
      "pg_namespace_nspname_index"`. Correction 25 is real, not theoretical.
  - `go test -race ./...` clean; `golangci-lint run` 0 issues;
    `make check-runtime-config` 21/21; `helm lint` and `kubectl kustomize` clean.

- Two things the spec did not anticipate, both found by running the proof:
  1. **A newer `pg_dump` produces an unrestorable backup.** pg_dump 17+ writes
     `SET transaction_timeout = 0` into the archive; PostgreSQL 16 rejects it.
     The dump exits 0 and fails only at restore. `pgdump-roundtrip.sh` now
     refuses on a client/server major mismatch, the runbook says so, and CI
     installs `postgresql-client-16` from PGDG because Debian trixie ships only
     17.
  2. **`search_path` cannot isolate the terminology ledger.**
     `pkg/terminology/db` creates a PostgreSQL schema literally named
     `terminology`, which is database-wide. The first version of the proof
     shared one database across subtests, so "two replicas against a fresh
     database" silently stopped exercising the fresh-install path for the one
     migrator this slice exists to fix. **The negative control caught it** — the
     lock control reported a schema that no migrator in that subtest created.
     Every proof now provisions its own database.

- What's next:
  - **Lane S4-B: your session migration is `0007`, not `0006`.** `.loom/32`'s
    ownership map has been corrected. Both of your migrations must satisfy the
    new `DEFAULT`-on-`NOT NULL` rule, which now fails `test:unit` rather than a
    review.
  - 4.4b/c/d/e filed in `.loom/30` with the blocker behind each; the
    Release-Candidate gate row corrected from "4.1-4.4" to "4.1-4.3 and 4.4a".
  - Filed for 4.4c: repair `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, recorded as a dated baseline in
    `knownRollbackUnsafeColumns`.

- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` Lane S4-C, corrections 21-30
  - [S2] `.loom/40-decisions.md` (2026-08-09, two entries)
  - [S3] `.loom/iteration-plan-phase-4-slice-4-4a-migration-compatibility.md`
  - [S4] `.loom/slice-handoff-phase-4-slice-4-4a-migration-compatibility.md`
