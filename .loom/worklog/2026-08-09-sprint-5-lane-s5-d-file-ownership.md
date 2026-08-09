### 2026-08-09 - Sprint 5 Lane S5-D: file ownership, lifecycle migration 0002 claim, and the rate-distribution decision

- What changed:
  - Lane S5-D (slice 4.4e, durable per-deployment MLLP token bucket) opens on
    branch `feat/phase4-slice-4-4e-mllp-token-bucket`, with two day-1 artifacts
    landing ahead of it: this ownership record plus the rate-distribution
    decision, and a standalone test-only MR carrying the day-1 gate.
  - **Files this lane owns** (from `.loom/33-sprint5-execution-specs.md`
    File-Ownership Map; no other lane writes these):
    - `internal/integration/mllp/**` — the capacity gate, its durable backing,
      and the end-to-end NAK contract test.
    - `internal/integration/lifecycle/migrations/0002_*.sql` — the durable
      quota record.
    - `pkg/integration/deployment.go` — the `CapacityPolicy` doc comment, which
      currently documents the per-replica multiple as intended behaviour.
    - `cmd/fi-fhir/main.go` `runServe` **component table only** (`:5232-5356`):
      the new component, the `errCh` capacity bump, `waiting`,
      `componentMetricNames`, and the `markComponent` not-configured list.
      S5-C owns every print/`Fprintf` site in the same file and merges first.
    - `cmd/fi-fhir/serve_observability.go` — one `Observe` adapter, added
      **after** S5-C merges, in S5-C's new shape.
    - `internal/observability/metrics.go` component constants (`:74-93`) and the
      hand-maintained list in `internal/observability/observability_test.go:142-148`
      — appended at the end of each block, not re-sorted. Shared with S5-F.
    - `docs/operations/PRODUCTION-MLLP.md` — the capacity contract section.
    - `.loom/40-decisions.md` (dated append), `.loom/worklog/` (this file).
  - **Migration number claim: lifecycle `0002`.** Re-verified against
    `origin/main` at `2f8b3f609`, not against the spec table: the only file
    under `internal/integration/lifecycle/migrations/` is
    `0001_deployment_lifecycle.sql`, and `internal/integration/lifecycle/postgres.go:28`
    declares `SchemaVersion = lifecycleMigrationVersion` at head 1. The
    `integration_lifecycle_schema_migrations` ledger is unfrozen for this lane
    alone this sprint. The number is re-verified again at rebase; the ledger on
    `origin/main` at commit time is the authority, not this claim.
  - The migration is **not authored until this branch has rebased onto S5-0's
    MR 0c**, which repairs the `ADD COLUMN … NOT NULL` hole in the
    migration-authoring rule (found defect D4). Every `NOT NULL` column carries
    a `DEFAULT`; every new CHECK lands `NOT VALID`; `SchemaVersion` bumps in the
    same commit or `assertEveryLedgerAtDeclaredVersion` fails.
- Why:
  - Sprint 4 twice proved that a migration number taken from a document rather
    than from the ledger is wrong by the time it is committed. Recording the
    claim and the verification source before the first commit is what makes the
    conflict visible early instead of at merge.
  - The lane touches three files that other lanes also touch
    (`cmd/fi-fhir/main.go`, `internal/observability/metrics.go`, and its test
    list). Naming the exact sub-regions here is cheaper than discovering the
    overlap in a rebase.
- Evidence:
  - `find internal -path '*lifecycle/migrations/*.sql'` → one file, `0001_deployment_lifecycle.sql`.
  - `grep -rn 'SchemaVersion' internal/integration/lifecycle/postgres.go` → `:28`, head 1.
  - `grep -rniE 'rate_limit|token_bucket|capacity_counter' --include='*.sql' .` → zero
    matches across all 18 migration files: PostgreSQL records no rate decision today.
- What's next:
  - Day-1 gate `TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday` as a
    standalone test-only MR — it must **pass** on unmodified `main`, quantifying
    the `N ×` over-admission the spec asserts, and it becomes the lane's
    negative control.
  - The end-to-end `RATE_EXCEEDED` NAK contract test, also against the existing
    in-memory gate, because nothing asserts that contract today and a durable
    bucket must not be the first code to establish it.
  - Then the durable quota record, the claim/refill loop, the redeploy-reset
    repair, and the serve wiring.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-D, File-Ownership Map, Schema Freeze Status Per Ledger
  - [S2] `internal/integration/mllp/capacity.go:104-126` — the in-memory continuous-refill bucket
  - [S3] `pkg/integration/deployment.go:60-78` — `CapacityPolicy` and the per-replica note
  - [S4] `internal/integration/lifecycle/postgres.go:25-28` — the lifecycle ledger head
  - [S5] `docs/operations/PRODUCTION-MLLP.md:42-71` — the current documented contract
