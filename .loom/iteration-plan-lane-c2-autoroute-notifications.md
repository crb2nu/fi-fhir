# RALPH Iteration Plan — Lane C2: Pending-Autoroute Review Notifications

**Created**: 2026-08-08
**Program**: `.loom/24-parallel-execution-specs.md` Lane C (Wave P2)
**Branch**: `feat/autoroute-notifications`

## Review

- Roadmap milestone: Lane C tasks 3-5. C1 (expiry sweep, tasks 1-2 plus the
  Lane C kill-test) shipped 2026-08-03 on `feat/autoroute-expiry-sweep`.
- Prior decisions to preserve, from the "C1 as shipped" block
  (`.loom/24-parallel-execution-specs.md`):
  - Narrow interfaces, not `*db.MappingStore`. C1's sweeper depends on
    `PendingAutorouteExpirer`. C2 mirrors it with `PendingAutorouteLister`, so
    notification policy stays out of the database package (the generalized Lane C
    non-goal) and is unit-testable without Postgres.
  - Serve wiring goes under the existing `serveCtx` / `errCh` /
    `waitForBackgroundStops` boundary, alongside MLLP, delivery, batch, and the
    sweep. Cancellation returns `nil`; a failing iteration logs and continues.
  - Observability is a typed result plus an `Observe` hook that serve prints.
    There is still no serve-wide Prometheus registry; do not invent one.
  - Kill-tests that need real store rows live in the **external** `db_test`
    package (`pkg/terminology/db/`), because `internal/terminology/autoroute`
    imports `pkg/terminology/db` and an in-package test would be an import
    cycle. CI needs no `.gitlab-ci.yml` change: it already runs that package.
- `.loom/27-spec-terminology-governance.md` asks this lane to own operational
  automation only (review queue SLA signal, notification policy) and explicitly
  tells the governance slice not to duplicate it.
- `docs/planning/TERMINOLOGY-MAPPING.md:1491-1494` documents the planning key
  `review.notification_webhook`, so the shipped config key takes that name.

## Align

- Slice name: **Lane C2 — webhook notification for pending autoroute review**.

- Scope in:
  1. A PHI-minimal notification payload and a `NotificationSink` interface.
  2. A webhook sink: bounded per-attempt timeout, one bounded retry, scheme and
     host validated at construction.
  3. A `ReviewNotifier` that scans the review queue on an interval, filters by a
     configurable confidence floor, de-duplicates, and dispatches asynchronously
     through a bounded queue.
  4. Config: webhook URL (empty disables), interval, min confidence, timeout —
     env plus YAML, with validation only when the webhook is set.
  5. Serve wiring under the existing background-component boundary.
  6. Unit tests (httptest) and an integration kill-test against real store rows.
  7. Docs: `docs/planning/TERMINOLOGY-MAPPING.md` and
     `docs/user-guide/terminology.md`.

- Scope out:
  - Slack-specific formatting anywhere. Generic JSON webhook only (Lane C
    non-goal).
  - Auto-approval of mappings (Lane C non-goal).
  - Rebuilding the approval UI (Lane C non-goal).
  - GraphQL schema changes; nothing here is a schema surface, so `lint:gqlgen`
    has nothing new to generate.
  - A serve-wide Prometheus registry (still absent; C1's reasoning holds).
  - The remaining `.loom/27` governance hardening: audit contract, rejection
    context retention, bulk-approval limits, role expectations.

## Key decision: periodic digest, not a per-event hook

The lane spec allows either "new pending autoroute created" (a hook in the
creation path) or "high-confidence pending review awaiting action" (a poll).
Shipped: **the periodic digest**, with the per-event door left open.

Rationale:

1. **Isolation is structural, not disciplined.** The acceptance criterion is
   that notification failures never affect mapping resolution. With a digest,
   nothing on the resolution or creation path calls the notifier at all, so the
   guarantee does not depend on getting error handling right at each call site.
   A per-event hook would put a new failure domain inside
   `Activities.CreatePendingAutoroute` and the GraphQL resolve path.
2. **A digest is what a reviewer needs.** The useful signal is "4 high-confidence
   mappings are waiting", not four separate pages. `eligible_count` makes queue
   depth alertable, which is the review-queue SLA framing `.loom/27` asks for.
3. **Creation is an upsert.** `CreatePendingAutoroute` upserts on
   `(source_system, source_code, target_system, suggested_code)`, so a per-event
   hook would fire on every re-resolution of the same unresolved code. The
   digest de-duplicates by row ID and announces each row once.
4. **The interface stays open.** `ReviewNotifier.Notify` is the non-blocking,
   bounded-queue entry point, and it is what the scan loop itself calls — so it
   is live in production, not speculative scaffolding. A future per-event hook
   calls the same method and inherits the same drop-rather-than-block contract.

Cost accepted: freshness is bounded by the scan interval (default 15m) rather
than immediate. For a human review queue with a 30-day expiry window that is not
a meaningful delay.

Secondary decisions:

- **De-duplication by row ID**, in a bounded (1024) FIFO set. Simpler than a
  `created_at` watermark and correct under the creation upsert. Eviction can at
  worst re-announce a very old row.
- **First scan announces the current backlog.** Mirrors C1's boot-time sweep:
  rows that arrived while the process was down are surfaced at startup.
- **Drop, do not grow.** The dispatch queue is 8 deep. Because each digest
  restates the backlog, dropping a stale digest loses nothing durable; queuing
  them would only deliver stale duplicates later.
- **Threshold re-checked in the notifier** after the store filter, so a store
  implementation that ignored `MinConfidence` still cannot leak low-confidence
  rows to an external endpoint.

## Acceptance criteria

1. High-confidence pending autoroutes reach the configured webhook; rows below
   the threshold do not.
2. The payload contains no free-text or LLM-authored content from the row.
3. A hanging or erroring webhook cannot slow or fail pending-autoroute creation;
   it logs a warning and drops.
4. Webhook URL, threshold, cadence, and timeout are configurable; an unset URL
   disables the feature with zero overhead.
5. The notifier stops cleanly on server shutdown and does not report
   cancellation as a component failure.

## Kill-test

`pkg/terminology/db/notify_integration_test.go` (build tag `integration`, external
`db_test` package):

1. `TestReviewNotifier_HighConfidenceRowsReachWebhook` — four real rows, two
   above and two below a 0.90 floor, one carrying poisoned free text in
   `source_display`, `reasoning`, and `decision_trace`. Asserts exactly the two
   above-threshold rows are delivered, `eligible_count == 2`, none of the
   poisoned strings appear in the body, and the next tick sends no repeat.
2. `TestReviewNotifier_HangingWebhookDoesNotSlowCreation` — a receiver that
   blocks until the test releases it, with a 30s client timeout. Once the
   webhook is confirmed wedged, 20 `CreatePendingAutoroute` calls must all
   succeed with a worst case well under the webhook timeout, and the notifier
   must still shut down cleanly out from under the hung request.

Negative control for the threshold assertion: removing the confidence floor from
`ScanOnce` makes the below-threshold codes appear in the delivered payload and
the test fails — it is not passing because the rows were never created.

## Verification

- `go test ./internal/terminology/autoroute/ ./pkg/config/ ./cmd/fi-fhir/ -race`
- `go test -race ./...`
- `golangci-lint run`, `go vet ./...`, `gofmt -l`
- Integration: `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/ -run ReviewNotif`

## What this leaves for `.loom/27`

- Reviewer identity, rationale, and audit-trail contract on approve/reject.
- Rejection/modification context retention for later analytics.
- Bulk-approval limits and role/permission expectations.
- Whether notification recipients need per-source-system routing. The current
  design is one global webhook; splitting by source system would be a config
  and payload change, not a redesign.
