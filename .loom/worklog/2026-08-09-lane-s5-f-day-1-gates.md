### 2026-08-09 - Lane S5-F day-1 gates

Lane S5-F's two day-1 gates, plus the lane's role-topology decision and its
file-ownership and migration-number claims. This entry is the record
`.loom/33-sprint5-execution-specs.md` requires before the lane writes production
code ("Before the first commit, each lane records its owned files in a worklog
entry and, if it needs one, re-verifies its migration number against
`origin/main`").

- What changed:
  - New `integration`-tagged test file
    `internal/integration/retention/purge_throughput_gate_integration_test.go`.
    No production code. It holds both gates and their fixtures; it reuses the
    package's existing `newRetentionGateSchema`, `migrateDurableSchema`,
    `newRetentionStore`, `seedRawCanonicalEvent`, and `assertRaised` helpers so
    the gates assert against the schema the runtime actually creates.
  - `TestPurgeThroughput_BacklogExceedsOneBatchPerTick` — expected to **FAIL**.
  - `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday` — expected
    to **PASS**.
  - A dated entry in `.loom/40-decisions.md`: the role-topology decision.
  - **No `.gitlab-ci.yml` change.** No lane appends to that file before Lane
    S5-0 MR 0a merges. Neither gate is wired to a job yet, so neither runs in
    CI on this MR; `test:phi-retention-purge` names its two tests explicitly in
    both its `-list` existence guard and its `make` target, so the new tests are
    invisible to it. The non-blocking job and the `make` target land with the
    implementation MR, after 0a, as one file under `ci/` plus one `include:`
    line.

- Owned files (Lane S5-F, per the File-Ownership Map):
  - `internal/integration/retention/**` — sole owner.
  - `internal/integration/processor/migrations/0006_*.sql` — **claim released**,
    see below.
  - `docs/operations/PHI-RETENTION.md` — sole owner.
  - `internal/observability/metrics.go` and
    `internal/observability/observability_test.go:142-148` — **shared with Lane
    S5-D**. S5-F adds a backlog *gauge*, which is a new metric family rather
    than a new label value, and extends the hand-maintained component list in
    the same commit. Both lanes append at the end of their blocks and neither
    re-sorts.
  - `Makefile` — one new proof target appended after `phi-retention-purge`, and
    one new `.PHONY` line of its own.
  - `ci/` — one new file plus one `include:` line, after S5-0 MR 0a merges.
  - `.loom/40-decisions.md`, `.loom/worklog/` — append-only, one file per entry.

- Migration numbers, re-verified against `origin/main` at `2f8b3f609`, from the
  constants rather than from any document:
  - processor `SchemaVersion = 5` (`internal/integration/processor/postgres_submission.go:80`),
    highest file `0005_retention_expiry.sql`. Next free: **`0006`**.
  - session `SchemaVersion = 7` (`internal/integration/session/postgres.go:49`),
    highest file `0007_export_attribution_defaults.sql`. Next free: **`0008`**.
  - Both match `.loom/33` correction 60.
  - **Lane S5-F releases its `0006` claim and will author no migration.** The
    role topology is deferred (decision below), so there are no GRANTs to write,
    and D1's backlog gauge is a `count(*)` over the partial index
    `integration_canonical_events_purge_idx`
    (`internal/integration/processor/migrations/0005_retention_expiry.sql:154-156`)
    and its two session-side equivalents
    (`internal/integration/session/migrations/0006_retention_expiry.sql:34,51`) —
    a query, not a schema change. Processor `0006` returns to the free pool. The
    session ledger is untouched and stays frozen; S5-F does not take `0008`.
  - The lane therefore has no dependency on Lane S5-0 MR 0c's `ADD COLUMN …
    NOT NULL` rule repair for migration authoring. It still rebases onto S5-0
    before the implementation MR, for MR 0a's CI include split.

- Evidence, gate 1 — predicted **FAIL** at exactly one batch:

  ```
  === RUN   TestPurgeThroughput_BacklogExceedsOneBatchPerTick/one_purge_pass_drains_the_backlog_it_is_given
      DAY-1 GATE CONFIRMED — D1 reproduced. One purge pass tombstoned exactly
      200 of 500 seeded events: one batch, then the pass returned and
      Purger.Run blocks on a 1h0m0s tick (purger.go:148-158). Sustained ceiling
      200 records/class/hour on integration_canonical_events.
      reported counts={CanonicalEvents:200 SessionSamples:0 SessionExports:0
      StreamEvents:0} duration=41.453209ms
  === RUN   TestPurgeThroughput_BacklogExceedsOneBatchPerTick/the_bound_is_per_call_not_a_property_of_the_rows
      backlog reached zero; per-pass tombstone sequence was [200 100]
  --- FAIL: TestPurgeThroughput_BacklogExceedsOneBatchPerTick (0.69s)
  ```

  Predicted result: **fail**, at exactly 200 of 500 tombstoned. Observed
  result: **fail**, at exactly 200 of 500 tombstoned. The full drain sequence
  across all three passes is 200, 200, 100 — bit-for-bit the reproduction
  `.loom/33` records for D1.

  Mechanism, confirmed in source: `Purger.Run`
  (`internal/integration/retention/purger.go:148-158`) calls `PurgeOnce` once
  and then blocks on the ticker; every purge and stamp statement carries
  `LIMIT $3` (`store.go:311,339,363,409,441,474,512`) bound to
  `defaultBatchSize = 200` (`store.go:33`); the shipped cadence is
  `defaultRetentionCadence = time.Hour`
  (`cmd/fi-fhir/retention_runtime.go:22-23`). There is no
  `continue`-on-full-batch. The house pattern one package over does the
  opposite — `internal/integration/session/stream.go:174-179`, "A full batch
  means there is more backlog; keep going rather than waiting a whole tick per
  batch."

  Non-vacuity: the gate asserts `purger.BatchSize() == defaultBatchSize` before
  it purges, and that the fixture seeds strictly more than one batch. Without
  the first check a hypothetical `defaultBatchSize = 1` would also fail this
  test, for a different defect, and the reproduction would be worthless — which
  is the trap `.loom/33` names ("`defaultBatchSize = 1` would leave the suite
  green"). The second subtest separates "the bound is per call" from "these rows
  are unpurgeable" and passes in both worlds.

- Evidence, gate 2 — predicted **PASS**:

  ```
  --- PASS: TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday (0.63s)
      --- PASS: .../the_role_is_ordinary_and_owns_the_guarded_table
      --- PASS: .../a_the_guard_is_armed_before_the_disarm
      --- PASS: .../b_the_application_role_drops_its_own_guard
      --- PASS: .../c_and_the_mutation_the_guard_forbade_now_succeeds
      --- PASS: .../d_disable_trigger_is_also_available_to_the_application_role
      --- PASS: .../e_replacing_the_shared_guard_function_disarms_every_trigger_using_it
      --- PASS: .../f_and_so_is_taking_the_table_away_entirely
  ```

  Predicted result: **pass** — the `DROP TRIGGER` succeeds and the forbidden
  mutation then succeeds. Observed result: **pass**, both, plus three further
  disarm shapes. Correction 54 and `PHI-RETENTION.md:293` stand.

  Non-vacuity, and this is the part that matters. The CI service container's
  `POSTGRES_USER` is a superuser, and a superuser can drop anything, so running
  this as `POSTGRES_TEST_URL`'s own role would prove nothing about ownership.
  The fixture provisions an ordinary role instead — `NOSUPERUSER NOCREATEDB
  NOCREATEROLE`, granted nothing but `USAGE` and `CREATE` on one scratch schema
  — runs the shipped processor and session migrators through it exactly as
  `runServe` does on the runtime connection, and asserts both that the role is
  not a superuser and that it ended up owning `integration_canonical_events`,
  before it disarms anything.

- New finding, not in `.loom/33` — a fourth disarm shape, and it constrains the
  deferred slice's GRANTs:

  Subtest `e` was added after the first run. As the application role,

  ```sql
  CREATE OR REPLACE FUNCTION reject_integration_submission_mutation()
  RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END; $$;
  ```

  succeeds, and disarms **all four** triggers that share that function
  (`internal/integration/processor/migrations/0004_audit_immutability.sql:30-47`)
  in one statement, without touching a single trigger. A role topology that
  revokes only `ALTER TABLE` would leave this route open. Function ownership has
  to move to the migrator with `EXECUTE` granted to the app role — which falls
  out of Option A for free, since the migrator creates the functions, and which
  no lesser option delivers. `.loom/33` does not mention it; the sprint spec MR
  should pick it up before it merges. This is an addition to the spec, not an
  inversion of it, so the lane has not rewritten `.loom/33`.

  One fixture note, recorded because it looked like a finding and is not:
  `ALTER TABLE … OWNER TO` first failed with "permission denied for schema".
  PostgreSQL requires the **new owner** to hold `CREATE` on the schema; that is
  a property of the target, not a privilege check against the issuing role. The
  fixture now grants it, so the assertion measures what it claims to.

- Decision: `.loom/40-decisions.md`, "2026-08-09: Purge role topology — ratify
  three roles with migrations out of `serve`, and ship D1 first (Lane S5-F)".
  Option A is ratified as the target topology and deferred as its own costed
  slice; Sprint 5 ships D1. This is not Option C — the follow-up is decided, not
  closed, and `PHI-RETENTION.md:293` is rewritten accordingly in the
  implementation MR. Gate 2 becomes that slice's negative control, already
  written: every `succeeded` inverts to `must be refused by PostgreSQL`.

- Verification:
  - `go vet -tags=integration ./internal/integration/retention/` — clean.
  - `golangci-lint run --build-tags=integration ./internal/integration/retention/...`
    — 0 issues.
  - `make phi-retention-purge` — `ok`, unchanged. The new file adds no helper
    that collides with the existing proofs.
  - PostgreSQL 16 on the remote Docker context, `-race`, `-count=1`.

- Next: the implementation MR. Task 1 is D1 — the drain loop bounded by a
  wall-clock budget per tick, plus the `PurgeExpired` first-error fix (S3) so one
  poisoned class cannot stop the rest of the pass — then the backlog gauge, the
  multi-batch drain test, and the `PHI-RETENTION.md` rewrite. It rebases onto
  Lane S5-0 first.
