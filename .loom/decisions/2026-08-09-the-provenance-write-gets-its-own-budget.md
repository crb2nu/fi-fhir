### 2026-08-09: The provenance write gets its own budget, and a provenance outage still stops the delivery worker

- Decision:
  - `Transport.DeliverDestination` derives the recorder's context with
    `context.WithTimeout(context.WithoutCancel(ctx), provenanceWriteBudget)`
    rather than passing the caller's context. The destination-facing request
    keeps the dispatcher's `PublishTimeout` window unchanged; the durable ledger
    write gets an independent five seconds that begins after that window has
    already expired.
  - `provenanceWriteBudget` is a package constant, not configuration.
  - A provenance write that fails **within its own budget** continues to return
    an unclassified error, which `transportFailure()` cannot classify, which
    stops the delivery worker component. That behaviour is retained deliberately
    and is now written down and covered by a test.
- Rationale:
  - Found defect D2, a release blocker. Both the request and the write shared
    `context.WithTimeout(ctx, PublishTimeout)`, so a destination that answered
    slowly left no budget for the ledger. Reproduced: destination contacted
    once, HTTP handler returned 200, **zero** provenance rows,
    `context.DeadlineExceeded`.
  - That is precisely the state
    `migrations/0002_https_delivery_provenance.sql:23-26` says cannot occur —
    "absence of a row means this process contacted no destination for that
    attempt, never that it did so and the record was lost." The ledger's own
    contract was false in merged code.
  - The blast radius is larger than a missing row. `context.DeadlineExceeded` is
    neither a `TransportFailure` nor a `Refusal`, so the dispatcher surfaces it
    raw, `RunOnce` returns with `MarkPublished = 0` and `MarkFailed = 0`, `Run`
    returns, and the delivery worker exits. The attempt stays leased, the lease
    expires, and the payload is redelivered to a destination that already
    accepted it — duplicate delivery for one idempotency key, inside the product
    spec's P0 definition.
  - `WithoutCancel` also detaches the write from worker shutdown. That is the
    intended trade: at most five seconds of extra shutdown latency per in-flight
    delivery, against never dropping a row that says a destination was
    contacted. A governance ledger should be the last thing abandoned.
  - Keeping the worker-stopping escalation is defensible for a governance
    ledger, and the objection to it — that one failed insert stops the whole
    component — was really an objection to how easily the branch could be
    reached. With the self-inflicted timeout removed, reaching it means the
    ledger did not accept a single-row insert in five seconds, which is an
    outage an operator should see immediately.
- Alternatives considered:
  - **Change the `DestinationTransport` contract so the dispatcher passes its
    parent context and the transport applies `PublishTimeout` itself** (rejected:
    it moves a timeout across a package boundary and rewrites an interface four
    call sites implement, to buy the same lifetime `WithoutCancel` gives in one
    expression.)
  - **Make the provenance budget configurable** (rejected: `delivery.Config`
    deliberately has no second timeout knob, because a second knob is a second
    way to break `PublishTimeout < LeaseDuration`. A constant can be reasoned
    about once.)
  - **Reclassify a provenance failure as a retryable `TransportError`**
    (rejected: `MarkFailed` writes a delivery outcome, and writing "this
    delivery failed" when the truth is "we do not know what happened" is the
    same category error the ledger exists to prevent. It would also let a
    permanently unavailable ledger drain the outbox into the DLQ silently.)
  - **Zero budget — write the row before contacting the destination** (rejected:
    the row records an executed delivery, including its status class and served
    certificate. Writing it first would make it an intent record, which is a
    different table and a different contract.)
- Consequences:
  - Worst-case wall clock for one dispatch rises from `PublishTimeout` to
    `PublishTimeout + 5s`. Against `DefaultConfig` (10s publish, 30s lease) that
    is 15s, half the lease. A deployment that narrows `PublishTimeout` to just
    under `LeaseDuration` can exceed the lease — that is the pre-existing gap in
    `Config.validate` recorded as S1 in `.loom/33`, not something this budget
    introduces, and it is filed rather than fixed here.
  - `TestTransportRecordsProvenanceWhenTheDestinationIsSlow` is the regression
    gate; its negative control is re-deriving `recordCtx` from `ctx` instead of
    `context.WithoutCancel(ctx)`, which reproduces the identical failure.
  - `TestDispatcherExitsWhenTheTransportReturnsAnUnclassifiedError` pins the
    retained escalation, so a future lane that wants to change it has to change
    a test that says why.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Found Defects D2; Lane S5-0 MR 0b
  - [S2] `internal/integration/destination/migrations/0002_https_delivery_provenance.sql:23-26`
  - [S3] `internal/integration/delivery/dispatcher.go` `deliverToDestination`;
    `internal/integration/delivery/transport.go` `transportFailure`
  - [S4] `internal/integration/destination/postgres.go` `RecordDelivery`
---
