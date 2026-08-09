### 2026-08-03 - Lane C1 pending-autoroute expiry sweep

- What changed:
  - Added `internal/terminology/autoroute/sweeper.go`: a cancellable,
    interval-driven `Sweeper` (`SweepOnce` / `Run`) over a narrow
    `PendingAutorouteExpirer` interface, so scheduling stays out of the DB
    package and sweep logic is unit-testable without Postgres.
  - Wired it into `serve` as a background component beside the MLLP, delivery,
    and batch runners: same `serveCtx` cancellation boundary, same `errCh`
    reporting, added to `waitForBackgroundStops`, `errCh` buffer 4 -> 5.
  - Added `FI_FHIR_TERMINOLOGY_AUTOROUTE_SWEEP_INTERVAL` (default `15m`, `0`
    disables) to `pkg/config`, the `fi-fhir config env` table, and
    `.env.example`.
  - Documented the two-layer expiry model in
    `docs/planning/TERMINOLOGY-MAPPING.md` and synced the `Terminology
    Autoroute` row in `docs/STATUS.md` (88.5% -> 91.6%).
- Why:
  - `ExpirePendingAutoroutes` had existed since the approval-workflow slice with
    zero production callers; only the shipped query-time guard kept the review
    queue truthful. Lane C was gated on Lane D's terminology DB integration
    baseline, which shipped 2026-06-19.
- Decisions:
  - Config owns the default cadence; the sweeper constructor rejects a
    non-positive interval instead of silently no-opping, so "disabled" is a
    deployment choice rather than a hidden fallback.
  - `Run` sweeps immediately on boot then on each tick, continues past iteration
    failures, and returns `nil` on cancellation so shutdown is not reported as a
    component failure.
  - Observability is a typed `SweepResult` + `Observe` hook printed by serve.
    Metrics deferred: there is no shared serve-wide Prometheus registry, and
    `internal/workflow`'s events/actions/DLQ-shaped `Metrics` interface is the
    wrong abstraction to import into terminology.
- Findings:
  - `internal/terminology/autoroute` transitively imports `pkg/terminology/db`,
    so the kill-test had to go in the **external** `db_test` package. That
    placement also means CI needs no change: `test:integration` already runs
    `./pkg/terminology/db/`.
  - `test:integration` is still `allow_failure: true`, so the kill-test runs but
    does not block. Hand this to Lane E as concrete promotion evidence.
  - The `FI_FHIR_MAPPING_*` env block in `docs/planning/TERMINOLOGY-MAPPING.md`
    documents variables that do not exist in code. Left in place (out of scope)
    but the new section is explicitly marked as implemented to avoid inheriting
    the ambiguity. Worth a dedicated docs slice.
- Verification:
  - `go test ./internal/terminology/autoroute/ ./pkg/config/ -race` -> passed.
  - `go test -race -coverprofile=coverage.out -covermode=atomic -cover ./...`
    -> passed (exit 0), same command CI's `test:unit` runs.
  - Kill-test against real Postgres:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/ -run ExpirySweep`
    -> passed. **Negative control**: with the store call removed from
    `SweepOnce`, the same test fails with "stored status after sweep = pending,
    want expired", proving it detects an absent sweep rather than passing on the
    query-time guard.
  - Full package twice against the same Postgres -> passed both times (47.8s,
    46.6s), preserving Lane D's schema-isolation property with the new
    schema-dropping test added.
  - `bash scripts/docs-status.sh --check-drift` -> exit 0, no coverage drift.
  - `golangci-lint run` on changed packages -> 0 issues (it caught an unused
    `//nolint` directive that would have failed CI `lint:go`).
  - `go vet ./...`, `gofmt -l` -> clean.
- What's next:
  - Lane C2: notification interface for new / high-confidence pending
    autoroutes (webhook config, thresholds, non-blocking dispatch).
  - Lane E: `allow_failure` inventory; `test:integration` now has real
    store-and-sweep coverage worth promoting.
- Sources:
  - [S1] `internal/terminology/autoroute/sweeper.go`
  - [S2] `pkg/terminology/db/sweeper_integration_test.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `pkg/config/config.go`
  - [S5] `.loom/iteration-plan-lane-c1-autoroute-expiry-sweep.md`
