### 2026-08-08 - Lane C2 pending-autoroute review notifications

- What changed:
  - Added `internal/terminology/autoroute/notify.go`: a `ReviewNotifier` that
    scans the pending-autoroute review queue on an interval, filters by a
    configurable confidence floor, de-duplicates by row ID, and dispatches a
    PHI-minimal JSON digest through a `NotificationSink`. `WebhookSink` is the
    only implementation (generic webhook, no Slack-specific logic).
  - Serve wiring in `cmd/fi-fhir/main.go` under the existing `serveCtx` /
    `errCh` / `waitForBackgroundStops` boundary, beside the C1 sweeper. `errCh`
    buffer 5 -> 6 for the extra component.
  - Config in `pkg/config/config.go`:
    `terminology.autoroute_notify.{notification_webhook,interval,min_confidence,timeout}`
    with `FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_*` env equivalents. Empty webhook
    disables the feature; validation only runs when a webhook is configured.
  - Kill-test in the external `db_test` package:
    `pkg/terminology/db/notify_integration_test.go`.
  - Docs: `docs/planning/TERMINOLOGY-MAPPING.md` (new "Pending Autoroute Review
    Notifications (implemented)" section, Phase 5 checkbox), and
    `docs/user-guide/terminology.md` (env table + "Review Notifications").
- Why:
  - Lane C tasks 3-5, the remaining half of `.loom/24-parallel-execution-specs.md`
    Lane C after C1 shipped the expiry sweep on 2026-08-03.
- Decisions:
  - **Periodic digest, not a per-event hook in the creation path.** The lane
    allowed either. A digest makes "notification failures never affect
    resolution" structural — nothing on the resolution or
    `CreatePendingAutoroute` path calls the notifier — rather than a discipline
    each call site has to maintain. It is also the more useful signal
    (`eligible_count` makes queue depth alertable), and it avoids firing on every
    re-resolution of the same code, since creation upserts on the natural key.
    Freshness is bounded by the scan interval, which is immaterial against a
    30-day review expiry window.
  - The per-event door is open and not dead code: `Notify` is the non-blocking
    bounded-queue entry point and the scan loop itself calls it, so a future
    per-event hook inherits the same drop-rather-than-block contract.
  - Same narrow-interface discipline as C1: `PendingAutorouteLister`, not
    `*db.MappingStore`.
  - Payload excludes every free-text / LLM-authored column
    (`source_display`, `suggested_display`, `reasoning`, `decision_trace`,
    `alternates`, `reviewed_by`, `rejection_reason`) because they can quote
    source message content and a webhook is untrusted egress.
  - Drop, do not grow: 8-deep queue, one delivery attempt plus one bounded
    retry. Each digest restates the backlog, so a drop loses nothing durable.
- Findings:
  - `httptest.Server.Close` waits for outstanding requests, and a client-side
    `http.Client` timeout does not reliably cancel `r.Context()` first. A
    "hanging receiver" handler that only selects on a release channel plus
    `r.Context().Done()` deadlocks the test binary (hit once, 10m timeout). Both
    hang tests now give the handler an absolute escape timer and release before
    `Close`.
  - macOS `cp` prompts on overwrite even in a script; use `yes | cp` when
    restoring a file after a negative-control experiment, and verify with grep.
- Verification:
  - `go test ./internal/terminology/autoroute/ ./pkg/config/ ./cmd/fi-fhir/ -race`
    -> passed.
  - `go test -race ./...` -> passed.
  - Kill-test against real Postgres 16:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/ -run ReviewNotif`
    -> both tests passed.
    **Negative control**: with the confidence floor removed from both the store
    filter and the in-Go re-check, the same test fails with
    "NOTIFY_LOW_1 (below threshold) was delivered" / "eligible_count = 4, want 2",
    proving it detects an absent threshold rather than passing because the
    low-confidence rows were never created.
  - Full `./pkg/terminology/db/` integration package against the same Postgres
    -> passed (35s), so the new schema-dropping test preserves Lane D's
    isolation property.
  - `golangci-lint run` on changed packages -> 0 issues. `go vet ./...`,
    `gofmt -l` -> clean.
- What's next:
  - Lane E: `allow_failure` inventory. `test:integration` now carries two Lane C
    kill-tests worth promoting.
  - `.loom/27` governance hardening: audit contract on approve/reject, rejection
    context retention, bulk-approval limits, role expectations. Also open there:
    whether notification recipients need per-source-system routing (today it is
    one global webhook; splitting is a config + payload change, not a redesign).
- Sources:
  - [S1] `internal/terminology/autoroute/notify.go`
  - [S2] `pkg/terminology/db/notify_integration_test.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `pkg/config/config.go`
  - [S5] `.loom/iteration-plan-lane-c2-autoroute-notifications.md`
