### 2026-09-27 - Session preview run persists once and publishes one stream batch

- What changed:
  - `internal/integration/session/runner.go`: `RunHL7v2` keeps stage progress
    and the run's stream envelopes in memory (`runProgress`), writes the
    terminal record with one `UpdateRun`, and then publishes every envelope of
    the run in order as one batch. The per-transition `UpdateRun` (a
    `SELECT ... FOR UPDATE` plus an `UPDATE` in its own transaction, eight per
    run) and the "running" write are gone; the durable record goes
    pending → succeeded|failed. If the terminal write fails nothing is
    published, so the stream never describes a run the record does not hold.
  - `hub.go`: `Hub.PublishAll` — in process it delivers in slice order; through
    a `StreamBatchLog` it takes contiguous seqs from one append; through a
    plain `StreamLog` it falls back to `Publish` per event. A failed batch
    degrades to in-process delivery exactly as `Publish` does.
  - `stream.go`: `StreamBatchLog` (optional, detected by type assertion, so the
    observability stub log and any older log keep working).
  - `postgres_stream.go`: `AppendStreamEvents` — one `INSERT ... SELECT FROM
    unnest(...) WITH ORDINALITY ORDER BY ord RETURNING seq, event_id`, mapped
    back by `event_id`; refuses a batch with a missing session, type or id, or
    a repeated id, and writes nothing in that case.
  - Tests: `runner_roundtrips_test.go` (a counting `Store` decorator and an
    in-memory batch log: 3 store round trips per run, 4 with a profile
    revision, 1 batch and 0 single appends, exact envelope order including the
    `diagnostic` between `normalize_diagnostics` start and end, `ErrImmutable`
    after the terminal write, nothing published when the terminal write
    fails), `hub_test.go` (five `PublishAll` proofs), and
    `postgres_stream_integration_test.go` (batch seqs contiguous and
    index-aligned on PostgreSQL 16, interleaving with single appends, refused
    batches leave the tail unchanged, a real run over the real store is one
    batch).
- Why: the browser smoke gate's session Preview flaked under runner saturation
  (jobs 322094 and 322278, pipelines 29733 and 29713): Playwright traces showed
  every store round trip at ~3 s, and a run was ~25 sequential committed round
  trips (`CreateRun`, one `UpdateRun` per stage start and end, one
  `AppendStreamEvent` per publish) around a sub-millisecond parse. MR !253
  gave the gate a 60 s run budget; this is the engine side, so the run's
  wall-clock no longer tracks per-commit latency at all. What the SSE stream
  delivers is unchanged: the relay polls the log every 250 ms, so subscribers
  already saw a run's envelopes as one burst, and the GraphQL projection
  re-reads the run at delivery time, so intermediate snapshots were never
  deterministic.
- Evidence:
  - `go test -race ./internal/integration/session/...`,
    `./internal/api/graphql/resolvers/...` (exact stream-order test),
    `./internal/integration/kernel/...` (parity), `./cmd/fi-fhir/...` (stub
    log): green. `golangci-lint run ./internal/integration/session/...`: 0
    issues (the first draft's VALUES-list concatenation tripped gosec G202;
    the `unnest` form is a fixed query and states the insertion order
    explicitly).
  - `go test -tags=integration ./internal/integration/session/` against
    PostgreSQL 16 on docker context 7900xtx: green, including the two new
    stream proofs.
  - Timing probe (scratch, not committed): 20 runs of `RunHL7v2` over the real
    store on 7900xtx from the LAN, durable hub on. `main`: min 355 ms, median
    378 ms, p90 431 ms. This branch: min 52 ms, median 55 ms, p90 59 ms, max
    61 ms. At ~14 ms per LAN round trip that is ~25 round trips versus ~4.
  - `UI_E2E_NAME=fi-fhir-ui-e2e-roundtrips make ui-e2e` on 7900xtx: 29 checks
    passed across the four projects in 2.1 min. Check 3 (stream open + honest
    state) 5.1 s; V3 `hl7-preview` 2.7 s end to end, against 15 s, 3 s and 2 s
    for the run alone on a busy host before this change.
- What's next: `CreateRun` still re-reads the sample the runner just loaded
  (one round trip, the store's own validation); a `CreateRun` that also sets
  the running state would make it 3. The two-replica soak from Sprint 7's
  budget 2 is where the batch append's contiguous-seq contract earns its keep.
- Sources:
  - [S1] `.claude` memory `ui-e2e-preview-run-lifecycle` (2026-09-27 diagnosis
    of jobs 322094 and 322278).
  - [S2] MR !253 `fix/ui-e2e-preview-lifecycle-wait`.
  - [S3] `migrations/0005_session_stream_events.sql` (envelope-only log, seq
    is the only ordering authority).
