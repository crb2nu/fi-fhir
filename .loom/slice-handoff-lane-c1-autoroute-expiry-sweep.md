# RALPH Slice Handoff — Lane C1: Pending-Autoroute Expiry Sweep

## Slice Summary

- Milestone: Wave P2 of `.loom/24-parallel-execution-specs.md`
- Slice: Lane C1 — serve-time pending-autoroute expiry sweep
- Status: **complete**

## What Landed

### Key changes

A serve-time background sweep that reconciles the stored `status` of pending
autoroutes whose expiry has passed. `ExpirePendingAutoroutes` had existed with
zero production callers since the approval-workflow slice; only the shipped
query-time guard kept the review queue truthful.

Behavioral notes worth carrying forward:
- The sweep is **reconciliation, not correctness**. `ListPendingAutoroutes` and
  `CountPendingAutoroutes` already treat time-expired pending rows as expired.
  Disabling the sweep leaves the stored column stale for reporting and direct
  SQL, but does not corrupt the review queue.
- `Run` sweeps immediately on boot, so rows that expired while the process was
  down are reconciled at startup, then ticks.
- An iteration failure is reported and the loop continues; a database blip must
  not take down serve. Cancellation returns `nil`, so a normal shutdown is not
  reported as a component failure.

### Key files

- `internal/terminology/autoroute/sweeper.go` — `Sweeper`, `SweepOnce`, `Run`,
  and the narrow `PendingAutorouteExpirer` interface.
- `internal/terminology/autoroute/sweeper_test.go` — 8 unit tests, race-clean.
- `pkg/terminology/db/sweeper_integration_test.go` — the Lane C kill-test, in
  the **external** `db_test` package.
- `cmd/fi-fhir/main.go` — construction beside `mappingStore`, start beside the
  MLLP/delivery/batch runners, `waitForBackgroundStops` entry, `errCh` 4 → 5,
  and the `config env` table row.
- `pkg/config/config.go` + `config_test.go` — `AutorouteSweepInterval`,
  `DefaultAutorouteSweepInterval`, env binding, 5 table-driven cases.
- `.env.example`, `docs/planning/TERMINOLOGY-MAPPING.md`, `docs/STATUS.md`.

### Validation results

| Check | Result |
|---|---|
| `go test ./internal/terminology/autoroute/ ./pkg/config/ -race` | passed |
| `go test -race -coverprofile=coverage.out -covermode=atomic -cover ./...` (CI `test:unit` command) | passed, exit 0 |
| Kill-test vs real Postgres (`-run ExpirySweep`) | passed |
| **Kill-test negative control** (store call removed from `SweepOnce`) | **failed as required** |
| `./pkg/terminology/db/` integration, twice vs same Postgres | passed both (47.8s, 46.6s) |
| `scripts/docs-status.sh --check-drift` | exit 0, no coverage drift |
| `golangci-lint run` on changed packages | 0 issues |
| `go vet ./...`, `gofmt -l` | clean |

The negative control is the load-bearing evidence. A kill-test asserting through
`ListPendingAutoroutes` would pass with no sweeper at all, because that read
already hides time-expired rows. This test asserts through
`GetPendingAutoroute`, which reads the raw column, and it was proven to fail
with the sweep neutered ("stored status after sweep = pending, want expired").

## What Is Still Open

### Remaining acceptance criteria

All C1 acceptance criteria are met. Lane C tasks 3-5 (notifications) were
deliberately scoped out as **C2** — see the iteration plan for the rationale.

### Known issues

1. **The kill-test does not block.** `test:integration` is still
   `allow_failure: true` (`.gitlab-ci.yml`). The sweep test runs in CI — no
   pipeline change was needed, since the job already runs
   `./pkg/terminology/db/` — but a regression would not fail the pipeline. This
   is concrete promotion evidence for Lane E.
2. **No metrics, only logging.** Lane C task 2 asked for "logging/metrics".
   There is no shared serve-wide Prometheus registry to hook: the only
   Prometheus code is `internal/workflow/metrics_prometheus.go`, whose interface
   is events/actions/circuit-breaker/DLQ shaped and wrong to import into
   terminology. The `Observe func(SweepResult, error)` hook is the seam — a
   metrics adapter is a small change once a registry exists.
3. **Stale planning doc, left alone.** The `FI_FHIR_MAPPING_*` env block in
   `docs/planning/TERMINOLOGY-MAPPING.md` documents variables that do not exist
   in code (verified by grep). Correcting it is a docs slice, not this one; the
   new section is explicitly marked "(implemented)" so it does not inherit the
   ambiguity.

### Dependencies

None outstanding. Lane D's terminology DB CI baseline was the gate and it held —
the package passes twice in a row against one Postgres with the new
schema-dropping test added.

## Next Actions

1. **Lane C2 — pending-autoroute notifications.** Tasks 3-5: notification
   interface for new / high-confidence pending autoroutes, webhook-first, config
   aligned to the planning key `notification_webhook`, dispatch that cannot
   block or fail pending-autoroute creation. The `Observe` hook shape is a
   reasonable model for the dispatch seam.
2. **Lane E — CI hardening.** Inventory `allow_failure: true` jobs; the sweep
   kill-test plus Lane D's store coverage make `test:integration` a stronger
   promotion candidate than when Lane E was written.
3. **Docs slice** — split `docs/planning/TERMINOLOGY-MAPPING.md` env
   documentation into implemented vs design-intent.

## Context Links

- Iteration plan: `.loom/iteration-plan-lane-c1-autoroute-expiry-sweep.md`
- Lane spec: `.loom/24-parallel-execution-specs.md` (Lane C section, updated)
- Worklog: `.loom/50-worklog.md` (2026-08-03 entry)
- Branch: `feat/autoroute-expiry-sweep`
