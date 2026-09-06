### 2026-08-08: Serve Observability Is a Second Listener Over a Purpose-Built Registry

- Decision:
  - **Metrics (spec `.loom/31` Lane S3-A option A).** A new `internal/observability`
    package owns exactly one `prometheus.Registry` and serves it on a **second
    HTTP listener** bound to `FI_FHIR_SERVER_HOST:FI_FHIR_METRICS_PORT`
    (default `9090`) at `FI_FHIR_METRICS_ENDPOINT` (default `/metrics`), gated by
    `FI_FHIR_METRICS_ENABLED` (default `true`). The listener is a first-class
    background component in `runServe`'s `errCh` table and shutdown path.
  - **Health lives on the main listener.** `/health` (liveness, process-only) and
    `/ready` (readiness, dependency-touching) stay on the GraphQL listener,
    because that is the port the Helm chart, `scripts/smoke-test.sh`, and every
    ingress already address. The metrics port carries no health surface and no
    PHI-bearing route.
  - **The check engine is the already-shipped `workflow.HealthService`**, wired
    for the first time from production code rather than re-implemented.
    `internal/observability` adapts it (registration, snapshot projection, HTTP
    handlers) so the GraphQL `health` resolver and the HTTP probes read one
    source of truth.
  - **Metric names are `fi_fhir_*`, not `workflow_*`.** The 32 legacy alert rules
    and the Grafana dashboard are rewritten against the emitted names.
    `internal/workflow`'s Prometheus adapter is left untouched and documented as
    legacy-engine-only.
  - **Label cardinality is bounded by construction.** Every label value is drawn
    from a compile-time constant set (component name, outcome, state, version).
    No correlation ID, receipt ID, attempt ID, tenant string, URL, or any
    message-derived value is ever a label.
  - **Durable session fanout (spec task 6)** is a new append-only envelope table
    `integration_session_stream_events` plus PostgreSQL `LISTEN`/`NOTIFY` on
    channel `integration_session_stream`, with a 1s poll backstop. The table and
    the notification carry **only** `(tenant_id, session_id, run_id, event_type,
    seq)` — never a payload. This is safe because `toGraphQLEvent`
    (`internal/api/graphql/resolvers/integration_session_service.go:827`) already
    ignores `StreamEvent.Payload` and re-reads the session and run from the
    durable store, so the stream log needs no clinical content to reproduce the
    subscriber's view.
  - **Autoroute notifier de-duplication (spec task 8)** becomes a durable
    `notified_at` claim column on `pending_autoroutes`, claimed with
    `UPDATE … SET notified_at = now() WHERE id = ANY($1) AND notified_at IS NULL
    RETURNING id`. Chosen over `pg_advisory_lock` because a lock serialises
    scanners without making the *decision* durable: a restart still re-pages the
    whole backlog, which is half the defect.
  - **MLLP capacity (spec task 9, option a)** is documented as **per-replica**.
    `CapacityPolicy` gets a doc comment and `docs/operations/PRODUCTION-MLLP.md`
    gets the division rule. A durable token bucket is 4.4 work.
  - **Batch worker identity** derives from `hostname-pid` when
    `FI_FHIR_BATCH_WORKER_ID` is unset, exactly as `delivery_runtime.go:40-47`
    already does, and the documented configuration stops handing out a shared
    literal.
  - **Negative control.** `FI_FHIR_OBSERVABILITY_MODE=legacy` restores the
    pre-slice behaviour at every one of these seams. It exists so the kill-test
    can prove it can fail, is refused a production role in `.env.example` and
    `docs/operations/README.md`, and logs a loud warning when set.
- Rationale:
  - Option A is the only option that makes the **already checked-in** deployment
    façade true rather than deleting it: the `metrics` containerPort, both
    Services, the compose port mapping, the Prometheus scrape job, the pod
    annotations, and `pkg/config.ObservabilityConfig` all describe a second
    listener on 9090. Every other option requires editing those artifacts to
    describe something else.
  - Keeping the scrape path off the GraphQL listener keeps an unauthenticated
    endpoint off the same socket that accepts raw clinical POSTs.
  - `prometheus/client_golang v1.23.2` is already a direct dependency, so the
    registry costs no new supply-chain surface.
  - Emitting `workflow_*` names from integration-engine code would be a naming
    lie of exactly the class this slice exists to remove, so the dashboards move
    rather than the metric names.
  - Persisting stream **envelopes** rather than payloads means the multi-replica
    fix adds zero new PHI at rest, which keeps retention policy squarely in
    S3-C's lane instead of quietly expanding it.
- Alternatives considered:
  - **B. Mount `/metrics` on the GraphQL mux** (rejected: contradicts every
    deployment artifact; puts an unauthenticated scrape path on the raw-clinical
    listener; needs a third reserved-path entry in `validateServerConfig`).
  - **C. Reuse `internal/workflow.PrometheusMetrics` as the serve registry**
    (rejected: its interface is events/actions/DLQ-shaped for the legacy engine
    the durable path never executes; adopting it means emitting `workflow_*`
    names from integration-engine code).
  - **D. Delete the façade** (rejected: the product spec requires `/metrics`
    (`.loom/20-product-spec-integration-engine-ide-completion.md:225`); deleting
    only defers 4.3).
  - **Re-implement health checking inside `internal/observability`** (rejected:
    `workflow.HealthService` already implements the liveness/readiness split,
    concurrent checks with timeout, 1s readiness caching, and 503-on-unhealthy;
    duplicating it would be ~250 lines of new untested code to avoid one import).
  - **Store stream payloads in the durable stream log** (rejected: duplicates
    clinical content with no retention policy, in a lane whose non-goals
    explicitly exclude retention).
  - **`pg_advisory_lock` around the notifier scan window** (rejected: see above).
  - **Leader election for the autoroute sweeper** (rejected: `ExpirePendingAutoroutes`
    is an idempotent guarded `UPDATE`; two replicas waste one query and have no
    external effect. Paying for a lease to save a query is not worth a new
    failure domain. Documented as a known benign duplicate.)
- Consequences:
  - `serve` now binds two ports by default. A deployment that cannot bind 9090
    must set `FI_FHIR_METRICS_ENABLED=false`; the process refuses to start
    silently degraded.
  - `pkg/config.ObservabilityConfig` gains its first production consumers
    (metrics fields). `TracingEnabled`/`TracingEndpoint`/`TracingSampler` remain
    inert and are now labelled "not implemented" in `.env.example` and
    `docs/operations/README.md` rather than implying an exporter exists.
  - The session workspace schema gains a migration. S3-C1 merged first and took
    `0004_export_attribution.sql`, so this lane's fanout log landed as
    `0005_session_stream_events.sql`.
  - `pending_autoroutes` gains a `notified_at` column. Existing rows are
    backfilled to `NULL`, so the first scan after upgrade re-pages the current
    backlog exactly once and never again.
  - MLLP `CapacityPolicy` remains per-replica; an operator running N replicas
    must divide the declared policy by N or accept N× the declared ceiling. This
    is now written down instead of being an undocumented surprise.
- Sources:
  - [S1] `.loom/31-sprint3-execution-specs.md` Lane S3-A, "The explicit metrics decision"
  - [S2] `internal/workflow/health.go:88,220-259`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go:827-845`
  - [S4] `deploy/kubernetes/base/deployment.yaml:19-22,42-44`; `deploy/kubernetes/base/service.yaml`
  - [S5] `pkg/config/config.go:181-195,417,606-613`
  - [S6] `cmd/fi-fhir/delivery_runtime.go:40-47`
  - [S7] `internal/terminology/autoroute/notify.go:292-293,494-519`
