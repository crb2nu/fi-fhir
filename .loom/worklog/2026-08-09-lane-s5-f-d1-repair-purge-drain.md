### 2026-08-09 - Lane S5-F D1 repair: purge drain loop and backlog gauge

Found defect D1 — HIGH, release blocker — repaired, with the S3 first-error fix
that compounds it and the gauge whose absence made both invisible. The role
topology is ratified and deferred; see the day-1 gate entry
(`2026-08-09-lane-s5-f-day-1-gates.md`) and `.loom/40-decisions.md`
(2026-08-09, "Purge role topology").

- The defect, restated from the reproduction:
  `Purger.Run` called `PurgeOnce` once per ticker tick and then blocked. Every
  purge and stamp statement carries `LIMIT $3` bound to `defaultBatchSize = 200`,
  on a `defaultRetentionCadence` of one hour. Sustained ceiling **200 records per
  class per hour = 0.056/sec**, on the table
  `internal/integration/retention/store.go:31-33` itself calls "the busiest table
  in the system", with no catch-up. Retention published counters only, so a purge
  falling a thousand records an hour behind and one keeping up produced identical
  exposition.

- What changed:
  - `internal/integration/retention/purger.go` — `PurgeOnce` is now a **drain**:
    while a bounded statement comes back full there is more backlog, so the tick
    keeps going rather than waiting a whole interval per batch. That is the shape
    `internal/integration/session/stream.go:174-179` already used. Bounded by a
    **wall-clock budget per tick** (`DrainBudget`, `defaultDrainBudget = 5m`,
    clamped to half the interval), checked *between* passes so a tick always
    makes at least one. `PurgeResult` gains `Passes`, `BudgetExhausted`, and
    `Backlog`.
  - `internal/integration/retention/store.go` — `PurgeExpired` attempts **every**
    class and joins the failures (`errors.Join`) instead of returning on the
    first. That is S3: one poisoned class used to skip every remaining class for
    the pass, which on an hourly cadence is an hour of retention not enforced for
    classes that were healthy. `PurgeCounts` gains `Saturated`, the
    full-batch signal the drain runs on; the three stamp helpers now report rows
    affected so **stamping counts toward saturation too** — a canonical event is
    unpurgeable until `purge_after` is stamped and the stamp carries the same
    `LIMIT`, so a stamp-bound backlog was exactly as invisible as a purge-bound
    one. A failing pass deliberately reports `Saturated = false` so the loop does
    not spin against a broken class for the whole budget.
  - `internal/integration/retention/store.go` — new `Backlog(ctx)` returning
    `BacklogCounts`. It counts what the purge is **eligible to act on right now**,
    delivery interlock included character-for-character, so a gauge of zero and a
    purge that acts on nothing are the same statement. It recomputes each
    deadline from the record's own timestamp and the policy window rather than
    reading `purge_after`, so an unstamped row still counts.
  - `internal/observability/metrics.go` — new metric family
    `fi_fhir_retention_backlog_records{record_class}`, a **gauge**, plus the
    `record_class` label allowlist (`allRetentionClasses`, `KnownRetentionClass`)
    matching the four classes the durable purge audit already writes. Appended at
    the end of each block; nothing re-sorted.
  - `internal/observability/observability_test.go` — the bounded-label assertion
    now also allows the record classes, via a loop over `allRetentionClasses`
    rather than a second hand-maintained list. **The hand-maintained component
    list at `:142-148` is unchanged** — this lane adds a metric family, not a
    component — which leaves that list free for Lane S5-D to append to without a
    conflict.
  - `cmd/fi-fhir/retention_runtime.go` — the observer publishes all four gauge
    series on **every** tick including the zeroes (a gauge written only when
    non-zero goes stale rather than going to zero, making "the backlog cleared"
    indistinguishable from "the purge died"), publishes them on the error path
    too, and warns with the remaining backlog when a tick exhausts its budget.
  - `internal/integration/retention/purger_{drain,nodrain}.go` — the negative
    control, in the `transport_gate_{narrowed,blanket}.go` idiom. The
    `retentionnodrain` build tag restores the single-pass loop.
  - `Makefile` — `phi-retention-throughput` and
    `phi-retention-throughput-negative-control`, on **one new `.PHONY` line of
    this lane's own** (correction 58). The existing `phi-retention-purge` target
    and its CI job are untouched, so no other job's `-list` arity moves.
  - `docs/operations/PHI-RETENTION.md` — new "Throughput" and "The backlog gauge"
    sections, the cadence/batch table corrected (they were documented as
    independent knobs and never multiplied — that framing is what let the ceiling
    ship), two new status rows, two new operational-guidance items, and the
    role-separation row rewritten to the ratified-and-deferred answer.

- Evidence:
  - `TestPurgeThroughput_TenThousandRecordBacklogDrainsWithinTheDocumentedTickBound`
    — the acceptance criterion at its stated scale:

    ```
    backlog drained in 1 tick(s), 51 store passes, 10000 records tombstoned
    --- PASS (3.03s)
    ```

    with the gauge asserted at 10,000 before the tick and 0 after, all 10,000
    audit rows present, and a non-tombstone mutation still raising afterwards —
    a drain that purged 10,000 records by widening the 4.1e exemption would be a
    regression, not a repair.
  - **Negative control**, `-tags 'integration retentionnodrain'`:

    ```
    after 1 tick(s) the backlog is 9800, not zero: 200 records tombstoned across
    1 store passes. At one batch per tick this would take 50 hourly ticks, which is D1
    ```

    and the day-1 gate re-reproduces at exactly 200 of 500.
    `make phi-retention-throughput-negative-control` inverts the exit status:
    *"negative control OK: the drain kill-test fails at one batch per tick with
    the single-pass loop restored."* Failing **at the batch boundary** is what
    makes the control attributable to the drain rather than to any breakage.
  - `TestPurgeThroughput_OnePoisonedClassDoesNotStopTheOthers` — the poison is a
    renamed table, so the failure arrives through the same path a revoked grant
    would. One class fails, the healthy class still purges all 10 of its records
    in the same pass, and the failing pass reports `Saturated = false`.
  - The day-1 gate `TestPurgeThroughput_BacklogExceedsOneBatchPerTick` is now
    **GREEN** — "one pass tombstoned all 500 events" — promoted from
    reproduction to regression guard without changing an assertion.
  - Unit: `TestPurgeOnceDrainsTheBacklogWithinOneTick` (asserts the *pass count*,
    six for a 1,000-record backlog at batch 200 — a single unbounded statement
    would report one pass and would not be this fix),
    `TestPurgeOnceStopsDrainingWhenTheWallClockBudgetIsSpent` (injected clock),
    `TestDrainBudgetIsClampedToHalfTheInterval`,
    `TestRetentionPurgeObserverPublishesTheBacklogGaugeEveryTick`.

- Acceptance criteria, all four proofs run locally against PostgreSQL 16 with
  `-race`:
  - `make phi-retention-purge` — ok, unchanged
  - `make phi-audit` — ok, unchanged
  - `make migration-compatibility` — ok, unchanged
  - `make observability-replicas` — ok, unchanged
  - `go test ./...` — clean; `golangci-lint` clean on every file this lane
    touches (three pre-existing findings remain in
    `internal/observability/replicas_integration_test.go` and
    `cmd/fi-fhir/integration_helpers_test.go`, neither owned by this lane, and
    neither linted by CI's `lint:go`, which does not set the integration tag)
  - An unconfigured deployment still purges nothing: `loadRetentionPurgerFromEnv`
    is unchanged, and both `PurgeExpired` and the new `Backlog` return zero on a
    nil policy record.

- One correction to a claim in the code itself:
  `docs/operations/PHI-RETENTION.md` documented cadence and batch size as
  independent knobs and never multiplied them, and `store.go:31-33`'s rationale
  for the batch bound is transaction size, never aggregate rate. Both are now
  accurate: the batch is a transaction-size decision, throughput is the drain
  loop's job, and the operational guidance says to watch the gauge rather than
  the batch size to decide whether the purge is keeping up.

- Not in this MR, deliberately:
  - **No `.gitlab-ci.yml` change.** Lane S5-0 MR 0a has not merged
    (`origin/main` has no `ci/` directory as of `31d61fe94`), and the sprint's
    one hard sequencing rule is that no lane appends before it does. The job for
    `phi-retention-throughput` — one file under `ci/`, one `include:` line, with
    the `-list | rg -x | awk` existence guard and the negative control in the
    same invocation — lands as a follow-up commit on this branch once 0a merges,
    which it must before this lane merges fourth.
  - **No migration.** Lane S5-F released its processor `0006` claim; the gauge is
    a query over the existing partial indexes. See the day-1 gate worklog entry.
