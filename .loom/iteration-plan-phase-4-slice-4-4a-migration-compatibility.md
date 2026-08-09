# Iteration Plan — Phase 4 Slice 4.4a: Migration Compatibility, Rollback Safety, Restore Round-Trip

**Lane**: S4-C (`.loom/32-sprint4-execution-specs.md`)
**Branch**: `feat/phase4-slice-4-4a-migration-compatibility`
**Merge position**: first of four Sprint 4 lanes (C → A → B → E)
**Day-1 gate**: MR !151 (landed red, deliberately)

## Why this lane merges first

Its `DEFAULT`-on-`NOT NULL` rule has to exist before Lane S4-B writes two
migrations, or S4-B repeats correction 23 in code that is being written this
sprint. Its footprint — `pkg/terminology/db`, a new test-only package, `deploy/**`,
two operations documents — collides with nothing.

## What was actually broken

Three findings, all confirmed against `origin/main` before any code was written.

1. **One-version rollback fails.** Slice 4.1d C1's
   `0004_export_attribution.sql:31-34` made `principal_json`, `reason`, and
   `include_raw_payload` `NOT NULL` with no `DEFAULT`. The current writer names
   all eight columns; a binary one version behind names five. Product spec
   budget 6 — "one-version rolling upgrade and rollback preserve receipts,
   revisions, and resumable work without schema downgrade corruption" — was
   **false in code that had already merged**.
2. **The terminology migrator takes no advisory lock**, while the other five do.
   Two replicas starting simultaneously against a fresh database both execute
   the full schema; `IF NOT EXISTS` is not atomic and the loser gets a
   duplicate-key error on a `pg_catalog` index.
3. **The repository cannot say what "one version" means.** Zero git tags, and
   the binary version string is a build stamp that carries no information about
   schema compatibility.

Two findings appeared *while* building the proof, neither anticipated by the
spec:

4. **A newer `pg_dump` produces an unrestorable backup.** pg_dump 17+ writes
   `SET transaction_timeout = 0` into the archive preamble; PostgreSQL 16 has no
   such GUC and rejects it. The dump exits 0. The failure surfaces during
   recovery.
5. **`search_path` cannot isolate the terminology ledger.** `pkg/terminology/db`
   creates a PostgreSQL schema literally named `terminology`, which is
   database-wide. The first version of the proof shared one database across
   subtests, so "two replicas against a fresh database" silently stopped testing
   the fresh-install path for the one migrator this slice exists to fix. **The
   negative control caught it** — which is the entire argument for writing
   controls.

## Sequence

| Step | Deliverable | State |
|---|---|---|
| 0 | Day-1 gate: `TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback` lands **red** in a non-blocking job, expected failure recorded in the worklog | MR !151 |
| 1 | Decision: what "one version" means (`.loom/40-decisions.md`) | this MR |
| 2 | `SchemaVersion` on all six ledgers; `fi-fhir version` and `fi_fhir_schema_ledger_version{ledger}` report them | this MR |
| 3 | `0006_export_attribution_defaults.sql` — the rollback fix | this MR |
| 4 | `pg_advisory_xact_lock` in `pkg/terminology/db.Migrator.Initialize`, version re-read inside the lock | this MR |
| 5 | Migration-authoring rule in `AGENTS.md` + `docs/developer-guide/testing.md`, enforced by a database-free unit test | this MR |
| 6 | `scripts/pgdump-roundtrip.sh` + the restore round-trip proof | this MR |
| 7 | RPO honesty in `PRODUCTION-HARDENING.md`; PITR filed as 4.4c | this MR |
| 8 | Tracing façade stripped from five deployment artifacts; `check-runtime-config` assertion | this MR |
| 9 | Reference-profile reconciliation: `values-reference-profile.yaml` + `SUPPORTED-1.0.md` | this MR |
| 10 | 4.4 split into a/b/c/d/e in `.loom/30`; release-gate row corrected | this MR |
| 11 | `test:migration-compatibility` promoted to blocking, arity 3 | this MR |

## Design decisions worth stating

**A new session migration, not an amendment to `0004`.** The ledger records
version 4 as applied, so an amended file never re-runs on an existing database
and would only fix fresh installs. Amending an applied migration inside a slice
about migration discipline would be self-defeating. Consequence: S4-C takes
session `0006`, so **S4-B takes `0007`**; `.loom/32`'s map is corrected.

**The DEFAULT makes a rollback-era row visibly unattributed, not plausible.**
It reuses the exact `unattributed_legacy_export` sentinel `0004` already
backfills historical rows with, so one predicate finds both classes. This
loosens the attribution guarantee — a future writer that forgets the column now
records an unattributed export instead of failing loudly. The loosening is
bounded by an assertion that the *current* writer still records a real
principal, so the DEFAULT cannot mask a live-path regression.

**A separate gauge, not six more labels on `fi_fhir_build_info`.** An info
metric's labels all change together; ledger versions move independently, and the
useful query compares one ledger across replicas.

**The proof shells out to the operator's script.** A restore proof that
reimplements `pg_dump` in Go proves something about the test, not about the
runbook.

**A fake publisher, not Kafka, for the resume assertion.** `Publisher` is
broker-neutral by contract, and the claim under test is that restored durable
state still yields a claimable, publishable work item. Requiring Kafka would add
a service container to prove something about PostgreSQL.

## Non-goals held

No performance measurement of any kind. No Kubernetes cluster work, no chaos
injection, no Helm upgrade/rollback exercise. No OpenTelemetry exporter and no
`log/slog` logger — only the artifacts that advertised tracing. No touching
`internal/integration/delivery/store.go` or `internal/api/graphql/schema.graphql`
(both frozen). No promotion of `test:benchmark`.

## File ownership claimed

`pkg/terminology/db/migrations.go`; `internal/integration/migrationcompat/**`
(new); `internal/integration/session/migrations/0006_*` and the embed + ledger
step in `session/postgres.go`; one-line `SchemaVersion` constants in
`processor/postgres_submission.go`, `lifecycle/postgres.go`, `batch/store.go`,
`destination/postgres.go`; an additive gauge in `internal/observability/metrics.go`;
`cmd/fi-fhir/schema_versions.go` (new) plus three lines in `main.go`;
`scripts/pgdump-roundtrip.sh` (new) and one check in `check-runtime-config.sh`;
`deploy/**`, `docker-compose.yaml`, `configs/full-stack.env`, `README.md`;
`docs/operations/{SUPPORTED-1.0,PRODUCTION-HARDENING}.md`,
`docs/developer-guide/testing.md`, `AGENTS.md`; append-only additions to
`Makefile` and `.gitlab-ci.yml`.
