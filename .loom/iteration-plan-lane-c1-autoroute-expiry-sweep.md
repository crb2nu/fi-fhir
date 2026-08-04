# RALPH Iteration Plan — Lane C1: Pending-Autoroute Expiry Sweep

**Created**: 2026-08-03
**Program**: `.loom/24-parallel-execution-specs.md` Lane C (Wave P2)
**Branch**: `feat/autoroute-expiry-sweep`

## Review

- Roadmap milestone: Wave P2 of `.loom/24-parallel-execution-specs.md`. Wave P1
  (Lanes A, B, D, F) is shipped per `.loom/50-worklog.md` 2026-06-19 entries.
- Spec section(s): Lane C "Pending Autoroute Sweep + Notifications"
  (`.loom/24-parallel-execution-specs.md:144-185`).
- Prior decisions to preserve:
  - Lane C was gated on Lane D's terminology DB integration baseline. Lane D
    shipped: `pkg/terminology/db/` is green via `POSTGRES_TEST_URL` and now runs
    in CI `test:integration` with `-p 1`. The gate is satisfied.
  - The shipped query-time expiry guard is authoritative and must stay.
    `ListPendingAutoroutes` (`pkg/terminology/db/mappings.go:747-763`) and
    `CountPendingAutoroutes` (`:1043-1055`) already treat time-expired pending
    rows as expired. The sweep makes the stored column agree with the reads; it
    must not become the only thing keeping the queue truthful.
  - Lane C non-goal: do not add notification/Slack logic to the DB package.
    Same principle applied here: no scheduling logic in the DB package either.

## Align

- Slice name: **Lane C1 — serve-time pending-autoroute expiry sweep**.

- Scope in:
  1. A cancellable, interval-driven sweeper that calls
     `MappingStore.ExpirePendingAutoroutes(ctx)`.
  2. Serve-time wiring under the existing background-component boundary
     (`serveCtx` / `errCh` / `waitForBackgroundStops`).
  3. Configurable interval with a conservative default and an explicit
     disable path.
  4. Structured per-sweep result (count, duration, error) surfaced through an
     observer hook, printed by serve in the existing output style.
  5. Unit tests for the sweeper; integration kill-test for real expiry.

- Scope out (deferred to **Lane C2**):
  - Notification interface for new / high-confidence pending autoroutes
    (Lane C tasks 3-5). That is a separate design surface: webhook config,
    threshold policy, non-blocking dispatch, failure isolation.
  - Auto-approval of mappings (explicit Lane C non-goal).
  - Rebuilding the approval UI (explicit Lane C non-goal).
  - A shared serve-wide Prometheus registry. There is none today: the only
    Prometheus code is `internal/workflow/metrics_prometheus.go`, whose
    `Metrics` interface is events/actions/circuit-breaker/DLQ shaped and is
    wrong for terminology. Importing `internal/workflow` into
    `internal/terminology/autoroute` would be bad coupling for one counter.
    The observer hook keeps a metrics adapter cheap to add later.

- Acceptance criteria (from Lane C, restricted to C1):
  1. Expired pending rows transition to `expired` in the database without any
     read call — i.e. not relying only on query-time filtering.
  2. The sweep interval is configurable; a conservative default applies; a
     non-positive interval disables the sweep.
  3. The sweep stops cleanly on server shutdown and does not stall or fail the
     shutdown path.
  4. A failing sweep iteration does not crash serve or terminate the loop; it is
     reported and the next tick still runs.
  5. The shipped query-time guard still holds when the sweep has not yet run.
  6. Serve without a mapping store (no `FI_FHIR_TERMINOLOGY_DB_URL`) is
     unchanged — no sweeper, no new output.

- Dependencies/blockers:
  - Lane D terminology DB CI path: satisfied.
  - Integration kill-test needs Postgres via `POSTGRES_TEST_URL`; if unavailable
    locally it must be recorded as a CI-verified item, not silently skipped.

## Land

- Planned file areas:
  - `internal/terminology/autoroute/sweeper.go` (new)
  - `internal/terminology/autoroute/sweeper_test.go` (new)
  - `pkg/config/config.go` (add `TerminologyConfig.AutorouteSweepInterval`)
  - `pkg/config/config_test.go` (env precedence/default coverage)
  - `cmd/fi-fhir/main.go` (serve wiring + shutdown bookkeeping)
  - `pkg/terminology/db/mappings_integration_test.go` (sweep kill-test)
  - docs: `.env.example`, `docs/planning/README.md` or terminology docs
  - `.loom/24-parallel-execution-specs.md`, `.loom/50-worklog.md`,
    `.loom/00-index.md`

- Implementation steps:
  1. Define a narrow `PendingAutorouteExpirer` interface in the autoroute
     package so the sweeper is unit-testable without Postgres.
     `*termdb.MappingStore` satisfies it structurally.
  2. Implement `Sweeper` with `SweepOnce(ctx) (SweepResult, error)` and
     `Run(ctx) error`, mirroring the `internal/integration/batch` Runner shape
     (ticker, `select` on `ctx.Done()`, return `nil` on cancellation).
  3. Make `Run` resilient: an iteration error is reported to the observer and
     the loop continues; only context cancellation ends it.
  4. Add config plumbing with `getEnvDuration` and default `15m`.
  5. Wire into serve next to the other background components; bump the `errCh`
     buffer and add the component to the `waitForBackgroundStops` map.
  6. Add tests, then docs, then plan/worklog updates.

## Prove

- Tests to run:
  - `go test ./internal/terminology/autoroute/ ./pkg/config/ ./cmd/fi-fhir/`
  - `go test ./...`
  - Kill-test (needs Postgres):
    `POSTGRES_TEST_URL=... go test -tags=integration ./pkg/terminology/db/ -run ExpirySweep`
- Lint/static checks: `go vet ./...`, `gofmt -l`, `golangci-lint` if available.
- CI checks: `lint:go`, `test:unit`, `test:integration` (now covers
  `pkg/terminology/db`), `security:gosec`, `test:coverage-merge`,
  `test:docs-status`.

## Kill-Test

From Lane C, unchanged: create an expired pending autoroute in an integration
test, run **only** the sweep runner with a short interval, and assert the stored
`status` becomes `expired` **without** calling `ListPendingAutoroutes`. This is
what distinguishes a real sweep from the already-shipped query-time filter — a
test that reads through `ListPendingAutoroutes` would pass even with no sweeper
at all.

## Handoff/Harvest

- Docs to update: env reference, terminology operations docs, Lane C section
  marked C1-done / C2-open.
- Agent-context entries: decision on sweeper placement and observability shape;
  finding on the metrics-registry gap.
- Next-slice candidates:
  1. **Lane C2** — pending-autoroute notifications (webhook, thresholds,
     non-blocking dispatch).
  2. **Lane E** — integration CI hardening / `allow_failure` promotion
     inventory.
  3. Lane F speclet implementation chosen by customer pull.
