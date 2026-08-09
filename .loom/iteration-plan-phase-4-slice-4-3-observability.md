# Iteration Plan — Phase 4 Slice 4.3: Truthful Observability and Multi-Replica Behavior

**Lane**: S3-A (`.loom/31-sprint3-execution-specs.md`)
**Branch**: `feat/phase4-slice-4-3-observability`
**Base**: `main` @ `7111cca1` (after 4.1b2, 4.1b3, 4.2a)
**Status**: in progress

## Goal

Make `/health`, `/ready`, and `/metrics` report real component state; make the
deployed probes and scrape config match what the process actually serves; and
make every background component correct under the `replicas: 2` the checked-in
manifests already declare.

## What is actually broken (verified, not assumed)

`.loom/31` corrections 1-12 are the ground truth. Re-verified against
`7111cca1` before editing:

| # | Finding | Verified at |
|---|---|---|
| 1 | `/health` is a hardcoded string literal | `internal/api/graphql/server.go:180-184` |
| 2 | `/ready` is mounted nowhere; `workflow.NewHealthService` has zero non-test callers | `internal/workflow/health.go:67-68,88` |
| 3 | GraphQL `health` resolver is hardcoded and is what CI smoke-tests | `schema.resolvers.go:2153-2180`, `scripts/smoke-test.sh:68-72` |
| 4 | `/metrics` does not exist behind a complete deployment façade | `deploy/**`, `dashboards/**`, `docker-compose.yaml`, `pkg/config/config.go:181-195` |
| 5 | K8s probes are `exec: ["/fi-fhir","version"]`; Helm hits the literal `/health` | `deploy/kubernetes/base/deployment.yaml:136-150` |
| 6 | Dead e2e tests assert the desired behavior and cannot run or fail | `test/e2e/integration_test.go:8,324-385` |
| 7 | Only the autoroute sweeper and notifier have `Observe` seams | `sweeper.go:44`, `notify.go:258-264` |
| 10 | Five multi-replica defects, not one | shared batch worker ID, notifier double-fire, MLLP per-process capacity, sweeper benign duplicate, SSE hub |
| 11 | SSE fanout is process-local with a `default:` drop | `internal/integration/session/hub.go:34-95` |
| 12 | `replicas: 2` is already checked in | `deploy/kubernetes/base/deployment.yaml:10` |

## Riskiest assumption and how it is killed

> **"In-process fanout is the only thing that breaks with two replicas."**

Wrong: correction 10 finds four *additional* defects, one of which (the shared
`FI_FHIR_BATCH_WORKER_ID`) is actively prescribed by our own `.env.example` and
operations doc, and is invisible to the existing CI proof because that proof
uses distinct worker IDs `worker-a`/`worker-b`/`worker-c`.

The kill-test refuses to construct a favorable configuration: it boots two
replicas from the documented environment block **read out of `.env.example` at
test time**, so re-introducing a shared literal to the docs fails the build.

## Order of work

1. **Docs day 1** — metrics decision in `.loom/40-decisions.md`, file/migration
   claim in `.loom/50-worklog.md`, `.loom/31` corrections. *(this commit)*
2. **Real `/health` + `/ready` + probes** — `internal/observability` package;
   wire `workflow.HealthService`; mount on the GraphQL listener; fix the GraphQL
   `health` resolver body; replace the `exec` probes; align Helm.
3. **Metrics** — one registry, second listener, bounded labels; bind the
   `Observe` seams; `PostgresCatalog.ReportHealth` from the runtime health path.
4. **Observe seams** — MLLP service, delivery `Dispatcher`, batch `Runner`,
   session `Hub`, following `SweeperConfig.Observe` exactly.
5. **Durable session fanout** — `integration_session_stream_events` envelope
   table + `LISTEN`/`NOTIFY` + poll backstop.
6. **Multi-replica fixes** — batch worker identity, durable notifier claim,
   documented per-replica MLLP capacity.
7. **Façade cleanup** — e2e tests, alert rules, Grafana dashboard, `.env.example`,
   `docs/operations/*`, `scripts/*`.
8. **Kill-test + CI job** — `TestServeObservability_TwoReplicasUnderDocumentedConfiguration`
   with the `pre` negative control, `make observability-replicas`,
   `test:observability-replicas` with the `-list | rg -x | awk` existence guard.

## Acceptance criteria (from `.loom/31`)

- `/ready` returns `503` when the submission database is unreachable and `200`
  when it is; `/health` stays `200` in both cases.
- `/metrics` serves a Prometheus exposition containing at least one counter that
  increments on a real durable submission, and every deployment artifact points
  at what is actually served.
- `grep -rn '\.Observability' internal/ cmd/` returns a production consumer.
- Two `serve` processes on one PostgreSQL: an SSE subscription on A receives the
  ordered `run_started → stage_* → run_completed` sequence for a run on B.
- Two batch runners from the **documented** configuration do not both claim the
  same object.
- Two notifiers against one receiver deliver each pending row once.
- No PHI in any metric label, `NOTIFY` payload, or log line added by this lane.
- `docs/operations/README.md`, `.env.example`, `deploy/**`, and `fi-fhir config
  env` agree with the process.

## Non-goals (deferred)

- OpenTelemetry exporter wiring — 4.4. `FI_FHIR_TRACING_*` stays inert and is
  now labelled "not implemented" instead of implying an exporter.
- Load-generated cardinality/latency budgets — 4.4.
- Durable per-deployment MLLP token bucket — 4.4; per-replica semantics are
  documented instead.
- Leader election for anything already lease-correct (delivery, batch store,
  lifecycle).
- Any GraphQL schema change. S3-C owns `schema.graphql` this sprint.

## Sources

- [S1] `.loom/31-sprint3-execution-specs.md` Lane S3-A
- [S2] `.loom/40-decisions.md` 2026-08-08 metrics decision
- [S3] `.loom/30-implementation-plan-integration-engine-ide-completion.md` §4.3
