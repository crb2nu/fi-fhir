### 2026-08-09 - Sprint 5 Lane S5-0 MR 0b: D2 provenance write budget

Release blocker. Found defect D2 in `.loom/33`: a duplicate-delivery generator
inside the product spec's P0 definition, merged with a green pipeline two days
before the sprint opened.

- Owned files (recorded before first commit, per `.loom/33` coordination rules):
  - `internal/integration/destination/transport.go` — the ten-line context
    lifetime change and the comment it invalidated. No other Sprint 5 lane
    touches this file.
  - `internal/integration/destination/transport_test.go` — the day-1 gate and a
    context-honouring recorder.
  - `internal/integration/delivery/dispatcher_test.go` — one characterization
    test plus a stub transport. No lane owns this package this sprint.
  - `.loom/40-decisions.md` — one appended dated entry (MR 0b task 3).
  - No migration. D2's fix is a context lifetime; the destination ledger stays
    FROZEN at head 2.

- Day-1 gate — `TestTransportRecordsProvenanceWhenTheDestinationIsSlow`.
  **Expected: FAIL with zero provenance rows and `context.DeadlineExceeded`.
  Result: exactly that**, on unmodified `main`, in the gate-only commit
  (`49b65672c`, which is red on purpose and precedes the fix):

  ```
  --- FAIL: TestTransportRecordsProvenanceWhenTheDestinationIsSlow (0.50s)
      the destination was contacted once and 0 provenance rows were written, want 1.
        DeliverDestination returned: record destination delivery: context deadline exceeded
        recorder saw context error:   context deadline exceeded
  ```

  Not a harness error and not a skip: the destination handler was entered
  exactly once and returned 200. The row is missing because the write had no
  budget left, which is the state
  `migrations/0002_https_delivery_provenance.sql:23-26` says cannot happen.

- Why the fake recorder is fair: `PostgresProvenance.RecordDelivery` ends in
  `db.ExecContext`, and `database/sql` returns `ctx.Err()` before reaching the
  driver when the context is already done. `contextBoundDeliveryRecorder` does
  the same and wraps with the same string `postgres.go` uses. The existing
  `recordingDeliveryRecorder` ignores its context entirely, which is why
  `TestTransportSurfacesAProvenanceOutage` — a recorder that *returns* an error
  — never covered a recorder *starved of budget*.

- What changed:
  - **The provenance write gets its own budget.**
    `context.WithTimeout(context.WithoutCancel(ctx), provenanceWriteBudget)`.
    The destination-facing request keeps `PublishTimeout` unchanged; the ledger
    write gets an independent five seconds that begins after that deadline has
    already expired. `WithoutCancel` keeps the caller's values and drops its
    deadline and cancellation.
  - **`provenanceWriteBudget = 5 * time.Second`**, a constant rather than a
    knob — `delivery.Config` deliberately has no second timeout, because a
    second knob is a second way to break `PublishTimeout < LeaseDuration`.
  - **The comment at the `record` call is now true.** Reaching the error branch
    means the ledger did not accept a single-row insert in five seconds, which
    is the genuine provenance outage the migration already describes. Before
    this, the branch was also reachable by a destination merely being slow.
  - **MR 0b task 3, answered and recorded.** A provenance failure still returns
    the unclassified error that stops the delivery worker. That is retained
    deliberately: `MarkFailed` writes a delivery outcome, and recording "this
    delivery failed" when the truth is "we do not know what happened" is the
    category error the ledger exists to prevent. The objection to the
    escalation was really an objection to how easily it fired. Full reasoning
    in `.loom/40-decisions.md` (2026-08-09).

- Evidence:
  - Gate red at `49b65672c`, green after the fix.
  - **Negative control**: re-deriving `recordCtx` from `ctx` instead of
    `context.WithoutCancel(ctx)` reproduces the identical failure — zero rows,
    same message. The gate watches the mechanism, not a coincidence.
  - `TestDispatcherExitsWhenTheTransportReturnsAnUnclassifiedError` pins the
    second half of D2 in the `delivery` package: an unclassified transport
    error leaves `published = 0`, `failed = 0`, and propagates out of `RunOnce`,
    so the worker stops and the attempt stays leased. Green before and after —
    a characterization of retained behaviour, not a regression gate.
  - `go build ./...`, `go vet ./internal/integration/...`, and
    `go test -race ./internal/integration/destination/...
    ./internal/integration/delivery/...` all pass.

- What is not covered here, and why: the lease-arithmetic gap.
  `Config.validate` enforces `PublishTimeout < LeaseDuration` but real elapsed
  time is now `Claim + decideIdentity + PublishTimeout + provenanceWriteBudget +
  MarkPublished`. Against `DefaultConfig` (10s/30s) the worst case is 15s, half
  the lease. A deployment narrowing `PublishTimeout` to just under
  `LeaseDuration` can exceed it. That is `.loom/33`'s suspected defect S1, which
  predates this budget and is unowned; it is filed rather than fixed, because
  fixing it means changing `Config.validate` and that is not this MR's file.

- What's next: MR 0c (D4 — the migration rule enforces the rule it documents),
  which must merge before S5-D and S5-F author migrations.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Found Defects D2; Lane S5-0 MR 0b
  - [S2] `internal/integration/destination/migrations/0002_https_delivery_provenance.sql:23-26`
  - [S3] `internal/integration/delivery/dispatcher.go` `deliverToDestination`;
    `internal/integration/delivery/transport.go` `transportFailure`
  - [S4] `internal/integration/destination/postgres.go` `RecordDelivery`
