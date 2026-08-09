# Slice Handoff — Phase 4 Slice 4.4a: Migration Compatibility, Rollback Safety, Restore Round-Trip

**Lane**: S4-C · **Branch**: `feat/phase4-slice-4-4a-migration-compatibility`
**Day-1 gate**: MR !151, merged `4b7279e1` (landed red on purpose)
**Merge position**: first of four Sprint 4 lanes (C → A → B → E)

## What is now true that was not

| Claim | Before | After |
|---|---|---|
| "One version" has a definition | Zero git tags; the binary version string carries no schema information | The per-package migration ledger version, exported as `SchemaVersion` by all six packages |
| A one-version binary rollback can write | **No** — every session export died on `SQLSTATE 23502` | Yes, and the row is visibly unattributed rather than plausible |
| Two replicas can start simultaneously against a fresh database | Five of six migrators were safe; terminology raced to a `pg_namespace_nspname_index` duplicate key | All six take an advisory lock and re-read their version inside it |
| Replicas at different ledger versions are distinguishable | No — every replica reported the same `fi_fhir_build_info` | `fi_fhir_schema_ledger_version{ledger}` and `fi-fhir version` |
| A `pg_dump` restore preserves the PHI guarantees | Unproven | Proved every run: rows, C1 triggers, the `NOT VALID` CHECK, and resumable delivery work |
| The documented backup meets the 5-minute RPO | Claimed | Stated as unachievable with logical dumps; PITR filed as 4.4c |
| Deployment artifacts advertise tracing | Five did | None do, and `make check-runtime-config` fails if one comes back |
| The chart defaults and the reference profile agree | They contradicted each other | Reconciled: defaults are labelled scheduling defaults, the profile has its own values file |

## Read this first if you are Lane S4-B

1. **Your session migration is `0007`, not `0006`.** Slice 4.4a took session
   `0006` because task 2's rollback fix is a session schema change, which
   `.loom/32`'s ownership map did not anticipate its own task list requiring.
   The map has been corrected. Re-verify against `origin/main` at rebase anyway
   — the ledger is the authority, not this document.
2. **`internal/integration/session/postgres.go` has one more embed and one more
   ledger step.** Both are appends in exactly the shape yours will take. Rebase
   onto merged main before judging your MR diff.
3. **The `DEFAULT`-on-`NOT NULL` rule is now enforced in `test:unit`**, not in
   review. `TestMigrationRule_NotNullOnExistingColumnCarriesADefault` reads the
   migration `.sql` files directly and needs no database. If one of your
   `purge_after` / `purged_at` columns is `NOT NULL`, it needs a `DEFAULT` or a
   dated entry in `knownRollbackUnsafeColumns` plus a decision in
   `.loom/40-decisions.md`. Your columns are specced `NULL`-able with no
   backfill, so this should cost you nothing — but the test will tell you
   immediately if that changes.
4. **`internal/observability/metrics.go` gained an additive gauge** and a
   `SchemaLedger*` constant block near the `Component*` block. Your
   `ComponentRetentionPurge` append lands in a different region.

## Read this first if you are Lane S4-A

`internal/integration/destination/postgres.go` gained five lines — an exported
`SchemaVersion = 1` constant next to the existing `destinationMigrationLockKey`
block. That file is yours in `.loom/32`'s map; the constant is here because the
version has to live in the package that owns the ledger, or it drifts.

**When you add `migrations/0002_*.sql`, bump `SchemaVersion` to 2 in the same
commit.** `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`
asserts every declared version equals the version actually applied, so
forgetting fails `test:migration-compatibility` rather than shipping a binary
that lies about what schema it expects. Your new migration also has to satisfy
the `DEFAULT`-on-`NOT NULL` rule, now enforced in `test:unit`.

## Notes for every lane

- **Provision a database, not a `search_path` schema**, for anything that
  touches the terminology ledger. `pkg/terminology/db` creates a PostgreSQL
  schema literally named `terminology`, which is database-wide; sharing a
  database means a later proof runs against a ledger an earlier one already
  migrated. This silently made one of 4.4a's own assertions vacuous until a
  negative control exposed it.
- **`pg_dump` client major must equal server major.** pg_dump 17+ writes
  `SET transaction_timeout = 0`, which PostgreSQL 16 rejects, and the dump exits
  0 regardless. Locally: `brew install postgresql@16` and
  `FI_FHIR_PG_BIN_DIR=/opt/homebrew/opt/postgresql@16/bin`. In CI the job
  installs `postgresql-client-16` from PGDG, because Debian trixie — the
  `golang:1.26.5` base — has only 17.
- **`test:migration-compatibility` runs its negative controls in the same
  invocation.** A control that *passes* fails the job. If you see
  `CONTROL PASSED, WHICH MEANS THE PROOF IS BROKEN`, the proof stopped
  exercising its mechanism; do not "fix" it by deleting the control.

## Next Actions

### 4.4b — performance budget harness (Sprint 5)

Budgets 1-3 (ACK latency p95 ≤ 250 ms / p99 ≤ 500 ms at 100 msg/s; one-hour
steady state ≥ 250 2-KiB msg/s; 1-GiB batch peak RSS ≤ 512 MiB above idle).

**Blocked on an infrastructure decision, not on code.** CI's k3s pool spans
hardware differing by more than 5×, so a latency gate there is either
permanently red or calibrated into meaninglessness. Needs, in order: a pinned
runner or dedicated host; `deploy/helm/fi-fhir/values-reference-profile.yaml`
(4.4a shipped it); and 4.4e's MLLP capacity decision, because a two-replica run
against a revision declaring 250 msg/s admits up to 500 and is not measuring
the declared policy.

Also in scope: `test:benchmark` measures `./internal/workflow/...`, the legacy
engine the durable path never executes, so it is not partial credit toward any
budget. Decide whether to repoint it or retire it.

### 4.4c — chaos, DR, and Kubernetes upgrade/rollback (Sprint 5)

Budgets 4 and 7, plus the remaining half of budget 5. Needs a cluster; 4.4a's
N-1 definition is the prerequisite it was waiting on.

- **WAL archiving / PITR.** 4.4a established the documented logical dump cannot
  meet a 5-minute RPO no matter how often it runs.
- Destination recovery under fault injection.
- Kubernetes 1.36 install, upgrade, rollback, uninstall through Helm and
  Kustomize, with live golden-journey evidence. This is where the *rolling*
  half of budget 6 gets proved — 4.4a proved the schema and writer are
  compatible, not that a real rollout survives.
- The first real CI job for `./test/e2e/...`, which runs in no job today.
- Repair the two rollback-unsafe columns 4.4a recorded as a dated baseline:
  `integration_delivery_attempts.scheduled_at` and
  `integration_delivery_outbox.updated_at` (processor ledger 2; outside the
  one-version window; needs a processor migration whose number belonged to Lane
  S4-B in Sprint 4). Remove both entries from `knownRollbackUnsafeColumns` in
  the same change, so the allowlist shrinks to empty.

### 4.4d — structured logging, then the tracing exporter (Sprint 5)

**In that order.** `log/slog` appears nowhere, so correlation-safe logging is a
build item and a prerequisite rather than a companion. 4.4a resolved only the
artifact half: `FI_FHIR_TRACING_*` is gone from every deployment artifact and
`make check-runtime-config` fails if it returns unlabelled. When the exporter
lands, restore `tracingEnabled` / `tracingSampler` to the Helm values and
remove the guard's justification, not the guard.

The `jaeger` container remains in `docker-compose.yaml` with a comment saying
nothing exports to it. Either wire it in 4.4d or remove it.

### 4.4e — durable per-deployment MLLP token bucket (Sprint 5+)

`docs/operations/PRODUCTION-MLLP.md:42-71` already documents that
`CapacityPolicy` is per-replica and that N replicas admit N× the declared rate.
Until a durable bucket exists, no throughput number can be attributed to a
declared policy — which is why 4.4b depends on this.

### Not filed as 4.4 work, but surfaced by it

- **Git tags.** The N-1 decision deliberately does *not* rest on tags, but the
  repository still has none, so "which artifact is running" has no stable
  answer either. That belongs to the release-gate work, not to 4.4.
- **`make build` passes no `-ldflags`**, so a developer build reports
  `0.0.0-dev` while CI builds carry a commit SHA. Harmless once the ledger
  versions are the compatibility signal, but worth aligning.

## Proof inventory

| Test | Kind | Where |
|---|---|---|
| `TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback` | Day-1 gate, now a permanent rollback-safety guard | `test:migration-compatibility` |
| `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore` | Primary kill-test, 5 assertion groups | `test:migration-compatibility` |
| `TestMigrationCompatibility_NegativeControls` | 3 controls; a passing control fails the job | `test:migration-compatibility` |
| `TestMigrationRule_NotNullOnExistingColumnCarriesADefault` | Static rule enforcement, no database | `test:unit` |
| `no deployment artifact advertises unimplemented tracing` | Config assertion | `make check-runtime-config` |
