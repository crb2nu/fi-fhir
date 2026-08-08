# Slice Handoff — Phase 4 Slice 4.3: Truthful Observability and Multi-Replica Behavior

**Lane**: S3-A (`.loom/31-sprint3-execution-specs.md`)
**Branch**: `feat/phase4-slice-4-3-observability`
**Base**: `main` @ `7111cca1`
**Status**: complete

## What shipped

### Observability is real, and the deployment artifacts now describe it

| Surface | Before | After |
|---|---|---|
| `GET /health` | One handler writing the literal `{"status":"healthy","service":"graphql"}`, checking nothing | Process-only liveness projecting named components; still 200 during a dependency outage, deliberately |
| `GET /ready` | Not mounted anywhere. `workflow.NewHealthService` implemented everything and had zero non-test callers | Mounted on the GraphQL listener; 503 when a readiness component is unhealthy; an absent dependency reports `not_configured`, never `healthy` |
| `GET /metrics` | Did not exist, behind pod annotations, a named containerPort, two Services, a scrape job, a dashboard, and 32 alert rules | Second listener on `FI_FHIR_METRICS_PORT`, one `prometheus.Registry`, `fi_fhir_*` names |
| GraphQL `health` | `Status: "healthy"` and `{event_store, healthy}` without touching the database — and it is what `scripts/smoke-test.sh` asserts on | Projects the same component set `/ready` evaluates. Resolver body only; `generated.go` byte-identical (`make lint-gqlgen` ✓) |
| K8s probes | `exec: ["/fi-fhir","version"]` for both — a wedged handler, a dead pool, and a crashed MLLP listener all passed | `httpGet /health` and `httpGet /ready` |
| Helm probes | Both hit `/health`, so readiness could never fail | Liveness `/health`, readiness `/ready` |

### Four multi-replica defects fixed, one documented

The plan text assumed in-process fanout was the only one. It was one of five.

1. **Session SSE fanout** — new envelope-only log (session migration `0004`) plus a per-replica relay. The log carries `(tenant_id, session_id, run_id, event_type, seq)` and **never a payload**, because `toGraphQLEvent` already ignores `StreamEvent.Payload` and re-reads the session and run from the durable store. The multi-replica fix therefore adds zero new PHI at rest, leaving retention entirely to S3-C.
2. **Batch worker identity** — `FI_FHIR_BATCH_WORKER_ID` was required and our own `.env.example` and `docs/operations/BATCH-INGESTION.md` handed out `fi-fhir-batch-1`. The batch store treats a matching owner as a lease renewal, so two replicas on the documented configuration stole each other's live leases. Now derived `hostname-pid` when unset, exactly as the delivery worker already did, and the documentation publishes no value.
3. **Autoroute notifier** — de-duplication moved from a per-process `seen` map to a durable `notified_at` claim (terminology schema v3) taken with `UPDATE … WHERE id = ANY($1) AND notified_at IS NULL RETURNING id`.
4. **Autoroute sweeper** — accepted as a benign duplicate and documented: `ExpirePendingAutoroutes` is an idempotent guarded `UPDATE`; two replicas waste one query and have no external effect.
5. **MLLP capacity** — documented as per-replica on `CapacityPolicy` itself and in `docs/operations/PRODUCTION-MLLP.md`. A durable per-deployment token bucket is 4.4.

### Everything else

- `Observe` seams on the four blind components, following `SweeperConfig.Observe` exactly: typed result struct, optional non-blocking hook, no metrics dependency inside the component package. The delivery dispatcher's seam finally surfaces the typed `Outcome` that `RunOnce` computed and `Run` discarded.
- First production caller of `PostgresCatalog.ReportHealth`, so `integration_lifecycle_snapshots.health` stops being written once at deploy time and never again. It reports only for definitions this replica actually serves.
- `pkg/config.ObservabilityConfig` gained its first production consumers.
- 32 alert rules → 10 actionable ones; the Grafana dashboard rewritten against emitted metrics.
- The two dead e2e tests replaced. `TestHealthEndpoints` asserted `health["status"] == "ok"` against a handler writing `"healthy"`; `TestMetricsEndpoint` asserted an endpoint that did not exist and downgraded its own content check to `t.Logf`. Both `t.Skipf`ed on connection error and no CI job passed `-tags=e2e`.
- `scripts/check-runtime-config.sh` gained five blocking checks that stop the façade regrowing, including one that fails if `.env.example` ever republishes a shared batch worker ID.

## Kill-test evidence

`TestServeObservability_TwoReplicasUnderDocumentedConfiguration`
(`internal/observability/replicas_integration_test.go`, CI job
`test:observability-replicas`, `make observability-replicas`).

Two real `fi-fhir serve` processes — not two hand-built object graphs, because
the failure this slice fixed was *wiring* — against one PostgreSQL 16, started
from the environment block the documentation prescribes.

| # | Assertion | `current` | `legacy` negative control |
|---|---|---|---|
| 1 | `/ready` 200 → 503 when PostgreSQL is unreachable → 200 again; `/health` stays 200 throughout | PASS | FAIL — `/ready` = 404; the endpoint does not exist |
| 2 | SSE subscribe on A, run a sample on B, receive the ordered `run_started → run_completed` within 2s | PASS | FAIL — replica A received `[]` |
| 3 | Two batch runners from the documented configuration do not share a lease owner | PASS | FAIL — both claim `fi-fhir-batch-1` |
| 4 | Two real `ReviewNotifier`s against one recording HTTP receiver page each pending row once | PASS | FAIL — 6 pages for 3 rows |
| 5 | A real durable submission on A increments A's counter and not B's; no PHI sentinel in any label | PASS (HTTP 202 with receipt) | FAIL — no metrics listener to scrape |

The control is a subtest, so one `go test` invocation produces both the proof and
the evidence that the proof can fail. Assertion 3 reads
`FI_FHIR_BATCH_WORKER_ID` out of `.env.example` at run time rather than crafting
a value, so re-publishing a shared literal fails the build.

## Two corrections made to `.loom/31` before coding

1. **Session migration ownership.** The file-ownership table assigned session migration `0004_*` to S3-C without noticing that S3-A task 6 needs a session migration and S3-A merges first. **S3-A took `0004_session_stream_events.sql`; S3-C1 takes `0005_*`.** Claimed in `.loom/50-worklog.md`.
2. **Kill-test database outage method.** The spec said "stop PostgreSQL via the remote Docker context". That is not runnable inside `test:observability-replicas`, whose PostgreSQL is a GitLab service container with no Docker socket, so the assertion would have been forced to skip in the one place it must block — the same cannot-fail shape corrections 6 and 28 exist to remove. The proof interposes an in-test TCP proxy instead: identical locally and in CI, no Docker dependency, and it exercises pool-level reconnect rather than container restart timing.

## Notes for the sibling lanes

- **S3-B and S3-C:** `runServe`'s component table changed shape. Rebase onto this
  merge before judging your MR diff. Append your component after your own
  subsystem's block and add a matching entry to `markComponent` and
  `waitForBackgroundStops`; do not restructure the table.
- Both `serveHealth` and `serveMetrics` are in scope at that point. A new
  background component should call `markComponent(<name>, ComponentRunning)` so
  `/ready` and `fi_fhir_component_up` cannot disagree with what the process
  started.
- `internal/api/graphql/schema.graphql` was **not** touched. S3-C keeps the
  schema lock.

## Deferred to 4.4

- OpenTelemetry trace exporter. `FI_FHIR_TRACING_*` stays inert and is now
  labelled "not implemented" in `.env.example` and `docs/operations/README.md`
  rather than implying an exporter exists.
- Structured logging in the serve path. `log/slog` appears nowhere in
  `internal/ pkg/ cmd/`; correlation IDs are plumbed end-to-end through records,
  receipts, events, and lineage, but there is no logger to correlate with.
  Emission, not plumbing, is the remaining work.
- Load-generated cardinality and latency budgets.
- Durable per-deployment MLLP capacity (token bucket).
- Client-resumable session streams. Replaying from a client-supplied last-seen
  `seq` needs a `schema.graphql` field, and S3-C owns the schema this sprint.

## Sources

- [S1] `.loom/31-sprint3-execution-specs.md` Lane S3-A
- [S2] `.loom/40-decisions.md` 2026-08-08 metrics decision
- [S3] `internal/observability/replicas_integration_test.go`
- [S4] Command: `make observability-replicas` with `POSTGRES_TEST_URL` set
- [S5] `.loom/iteration-plan-phase-4-slice-4-3-observability.md`
