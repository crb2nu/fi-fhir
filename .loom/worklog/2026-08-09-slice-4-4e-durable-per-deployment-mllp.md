### 2026-08-09 - Slice 4.4e: durable per-deployment MLLP rate quota

- What changed:
  - `max_messages_per_second` is now a deployment-wide budget. Each replica
    leases a share of it from `integration_mllp_rate_claims` (lifecycle
    migration **0002**), refills its in-memory token bucket from that share, and
    releases the share on shutdown. Two replicas of a deployment declaring
    100 msg/s admit 100 in aggregate; before this slice they admitted 200.
  - **Admission never touches the database.** The claim loop runs every 2s on a
    6s lease; the per-frame decision is the same in-memory token take it always
    was. Asserted, not asserted-about: the kill-test counts store round trips
    during the measured window and requires zero.
  - `capacity.go` no longer refills the bucket when the revision digest changes
    (correction 36). It is seeded full once, on the process's first frame, and
    the balance then carries, clamped to the current share.
  - Serve wiring: one new background component (`mllp-rate-quota`), the
    `errCh` capacity repair 10 → 12 (correction 59), `markComponent`'s
    not-configured list, `waiting`, `componentMetricNames`, the
    `ComponentMLLPRateQuota` constant and the hand-maintained list in
    `observability_test.go`, and one `Observe` adapter in
    `serve_observability.go`.
  - New metric `fi_fhir_mllp_rate_claims_total{outcome}` and one new bounded
    `Outcome`, `degraded` — a component still serving, but on a documented
    fallback rather than its authoritative state. None of the existing 14 names
    that, and it is the condition an operator alerts on.
  - `ci/test-mllp-rate-quota.yml` plus one `include:` line, and
    `make mllp-rate-quota`. Proof and negative control run in one invocation.
- Why:
  - `docs/operations/PRODUCTION-MLLP.md` documented `N ×` over-admission as
    intended behaviour. Budget 2 of the product spec is a 250 msg/s figure on
    the two-replica reference profile; measuring it before this landed would
    have certified nothing, which is why S5-D merges before S5-A.
- Evidence:
  - `make mllp-rate-quota` against PostgreSQL 16: *two replicas admitted 100
    msg/s against a declared 100 msg/s (control: 200) at a total cost of 8
    database round trips.*
  - `TestMLLPQuotaStore_ConcurrentClaimsCannotOverGrant`: ten replicas claiming
    at once hold exactly 100 of a declared 100. The bound is transactional, not
    advisory.
  - `TestMLLPQuotaStore_RejectsAnOverGrant`: a share larger than the rate it is
    a share of is refused with SQLSTATE 23514 — the migration's CHECK does work.
  - `make mllp-runtime` and `make migration-compatibility` both pass unchanged
    with the lifecycle ledger at version 2, including the restore round-trip and
    the concurrent-replica migration.
  - `scripts/ci-job-inventory.sh --check`: 56 jobs, exactly one added, at
    `stage: test` with `allow_failure: false`.
- Two things the tests found that review would not have:
  - **The quota pool cannot be keyed on the revision digest**, though `.loom/33`
    task 3 specified it that way. A rolling redeploy runs two digests at once;
    two digest-keyed pools each admit the full declared rate, so the deployment
    would burst to twice it for the length of every rollout — the exact failure
    the durable bucket exists to prevent. `QuotaKey` is `(tenant_id,
    definition_id)`; the digest rides on the claim row as attribution. Corrected
    in `.loom/33` and `.loom/40` before the code was written.
  - **`release()` budgeted its context by the claim interval**, which can be
    shorter than a round trip. Every graceful release silently timed out and
    stranded the share until the lease expired. Found by an integration test
    with a 1ms interval; now budgeted by the lease TTL.
- Owned files (unchanged from the ownership entry, plus the ones the migration
  necessarily brought with it): `internal/integration/mllp/**`,
  `internal/integration/lifecycle/migrations/0002_mllp_rate_claims.sql`,
  `internal/integration/lifecycle/postgres.go` (the numbered-migration form and
  `SchemaVersion = 2` — authoring the migration is what requires this, and no
  other lane writes the lifecycle ledger this sprint), `pkg/integration/deployment.go`,
  the `runServe` component table only, `serve_observability.go`,
  `internal/observability/metrics.go` + `observability_test.go` (appended, not
  re-sorted; shared with S5-F), `docs/operations/PRODUCTION-MLLP.md`,
  `ci/test-mllp-rate-quota.yml`, `ci/job-inventory.txt`, one `.gitlab-ci.yml`
  include line, one Makefile `.PHONY` lane line and one target.
- What's next:
  - Rebase over S5-C's `!178` once it merges: `serve_observability.go` and the
    `runServe` print statements are its surface, and the `Observe` adapter here
    should end up in its shape rather than the current one.
  - Slice 4.4b (Lane S5-A) can now measure budget 2 on the two-replica
    reference profile against a number that means something.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-D; corrections 11, 35-39, 59, 60
  - [S2] `.loom/40-decisions.md` (2026-08-09) — the distribution decision and its keying correction
  - [S3] `internal/integration/lifecycle/migrations/0002_mllp_rate_claims.sql`
  - [S4] `docs/operations/PRODUCTION-MLLP.md` — the rewritten capacity contract
- Correction (2026-09-05, found by CI): the "ten replicas hold exactly 100"
  evidence above was a local interleaving, not a property. `test:mllp-rate-quota`
  failed on every CI run of this branch from its first (pipeline 22968, 2026-08-09)
  on `TestMLLPQuotaStore_ConcurrentClaimsCannotOverGrant` — ten concurrent
  replicas granted 190 of a declared 100. Nothing in `Claim` serialised
  concurrent claimants: each replica writes its own row, so no row lock is
  shared, and under READ COMMITTED each transaction's `count(*)` sees only
  committed rows plus its own. Two changes in `quota_postgres.go`: (1) a
  transaction-scoped `pg_advisory_xact_lock` keyed on the deployment, so claims
  for one pool serialise and the count is exact; (2) the claim rebalances every
  live holder's share, not only the caller's, so the persisted shares sum to the
  declared rate after every commit rather than only once every holder has
  renewed since the last arrival. The pool rate is the smallest declaration
  among live holders, because rows carry their own `declared_rate` and the CHECK
  bounds each by its own — a rolling redeploy that changes the rate runs at the
  lower one until the older revision drains. Neither change touches the
  admission path; `partitionShare` stays the single formula.
