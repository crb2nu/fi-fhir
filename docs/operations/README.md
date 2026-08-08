# Operations Guide

Documentation for deploying and operating fi-fhir in production environments.

## Contents

- [Production Hardening](PRODUCTION-HARDENING.md) - Security best practices
- [Operations Runbook](RUNBOOK.md) - Troubleshooting and procedures
- [Supported 1.0 Baseline](SUPPORTED-1.0.md) - Pinned release target and evidence gaps
- [Integration Deployment Lifecycle](INTEGRATION-DEPLOYMENT-LIFECYCLE.md) - Versioned catalog and state contract
- [Restart-Safe Integration Sessions](INTEGRATION-SESSIONS.md) - Durable author/test workspace and PHI policy

## Quick Links

| Task | Document |
|------|----------|
| Secure a deployment | [Production Hardening](PRODUCTION-HARDENING.md) |
| Troubleshoot issues | [Operations Runbook](RUNBOOK.md) |
| Review the 1.0 support target | [Supported 1.0 Baseline](SUPPORTED-1.0.md) |
| Review integration lifecycle state | [Integration Deployment Lifecycle](INTEGRATION-DEPLOYMENT-LIFECYCLE.md) |
| Operate durable Integration Sessions | [Restart-Safe Integration Sessions](INTEGRATION-SESSIONS.md) |
| Configure authenticated preview | [Operations Runbook](RUNBOOK.md#authenticated-preview-access) |
| Monitor performance | [Observability](#observability) |
| Configure health checks | [Health Endpoints](#health-endpoints) |

## Deployment Options

### Docker Compose (Development)

```bash
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
docker compose up -d
unset FI_FHIR_GRAPHQL_BEARER_TOKEN
```

Compose deliberately has no default preview bearer. Generate a fresh local
value as above; never commit or print it. Production deployments should mount a
managed secret file instead.

### Kubernetes (Production)

```bash
# Using Kustomize
kubectl apply -k deploy/kubernetes/overlays/production/

# Using Helm
helm install fi-fhir deploy/helm/fi-fhir/ -f values-prod.yaml
```

`fi-fhir serve` requires a deployment tenant, principal, `integration:preview`
role, exact HTTP(S) origins, one bearer source, and an immutable same-tenant
integration registry. Missing or inconsistent values fail startup closed. See
[Production Hardening](PRODUCTION-HARDENING.md#transitional-preview-authentication).

## Observability

`fi-fhir serve` exposes three HTTP surfaces. `/health` and `/ready` share the
main server listener (`FI_FHIR_SERVER_HOST:FI_FHIR_SERVER_PORT`, default
`8080`, `cmd/fi-fhir/main.go:3141`). `/metrics` is a second, independent
listener (`internal/observability/server.go:20-91`).

### Liveness: `GET /health`

Process-only. It never checks a dependency and never returns 503. A liveness
probe that fails on a database outage turns the outage into a pod restart
loop (`internal/observability/health.go:89-91`, `:260-272`).

```bash
curl -s http://localhost:8080/health
```

Expected output shape:

```json
{"status":"healthy","components":[{"name":"process","status":"healthy","message":"process is serving"}],"checked_at":"2026-08-08T00:00:00Z"}
```

### Readiness: `GET /ready`

Dependency-touching. Returns `503` when any readiness component reports
`unhealthy`, `200` otherwise (`internal/observability/health.go:53-61`).

Always-checked dependencies (`cmd/fi-fhir/main.go:4890-4903`):

- `submission_db`, `terminology_db`, `session_store`, `profile_store`,
  `workflow_lifecycle_store`, `event_store`, `mapping_store`

Background components, checked only if this replica started them
(`cmd/fi-fhir/main.go:5201-5207`):

- `mllp`, `delivery`, `batch`, `autoroute_sweep`, `autoroute_notify`,
  `session_stream`, `metrics`, `graphql`

A dependency that was never configured reports `not_configured`, never
`healthy` (`internal/observability/health.go:141-150`).

```bash
curl -s http://localhost:8080/ready
```

Expected output shape (a database outage):

```json
{"status":"unhealthy","components":[{"name":"submission_db","status":"unhealthy","message":"database is unreachable"},{"name":"terminology_db","status":"not_configured","message":"terminology database is not configured"}],"checked_at":"2026-08-08T00:00:00Z"}
```

### Metrics: `GET /metrics`

Prometheus exposition on a **second listener**, gated by
`FI_FHIR_METRICS_ENABLED` (default `true`), bound to `FI_FHIR_METRICS_PORT`
(default `9090`) at `FI_FHIR_METRICS_ENDPOINT` (default `/metrics`)
(`internal/observability/server.go`, `cmd/fi-fhir/main.go:5113-5126`). Every
metric name starts `fi_fhir_` (`internal/observability/metrics.go:131-174`);
`workflow_*` names belong to the legacy engine's separate, unused adapter and
are never emitted here:

| Metric | Type | Labels |
|---|---|---|
| `fi_fhir_build_info` | gauge | `version` |
| `fi_fhir_component_up` | gauge | `component` |
| `fi_fhir_readiness_up` | gauge | `component` |
| `fi_fhir_http_ingress_submissions_total` | counter | `outcome` |
| `fi_fhir_mllp_messages_total` | counter | `outcome` |
| `fi_fhir_delivery_attempts_total` | counter | `outcome` |
| `fi_fhir_batch_objects_total` | counter | `outcome` |
| `fi_fhir_session_stream_events_total` | counter | `outcome` |
| `fi_fhir_autoroute_sweeps_total` | counter | `outcome` |
| `fi_fhir_autoroute_expired_total` | counter | none |
| `fi_fhir_autoroute_notifications_total` | counter | `outcome` |

The registry also includes the standard `go_*` and `process_*` collectors
(`internal/observability/metrics.go:124-127`).

```bash
curl -s http://localhost:9090/metrics | head -n 3
```

Expected output shape:

```
# HELP fi_fhir_build_info Build information for the running fi-fhir process; always 1.
# TYPE fi_fhir_build_info gauge
fi_fhir_build_info{version="dev"} 1
```

Assumption: the `version` label value above (`dev`) depends on how the
binary was built; treat it as illustrative, not a guaranteed value.

### Tracing — not implemented

`FI_FHIR_TRACING_ENABLED`, `FI_FHIR_TRACING_ENDPOINT`, and
`FI_FHIR_TRACING_SAMPLER` are parsed and validated
(`pkg/config/config.go:609-611`, `:759`), but nothing consumes them. There is
no OpenTelemetry exporter wired into `serve`. Setting these variables changes
no runtime behavior. They are reserved for a future slice; do not rely on
trace export today.

### Logging — not implemented

`FI_FHIR_LOG_LEVEL` and `FI_FHIR_LOG_FORMAT` are likewise parsed but nothing
reads them. There is no structured logger in the serve path, verified with:

```bash
grep -rn '"log/slog"' internal/ pkg/ cmd/
```

Expected output: no matches. `serve` writes plain `fmt.Print*`/`fmt.Fprintf`
lines to stdout/stderr, not JSON, and carries no `trace_id`/`span_id`
correlation. Reserved for a future slice.

### `FI_FHIR_OBSERVABILITY_MODE=legacy` — not a production configuration

Setting `FI_FHIR_OBSERVABILITY_MODE=legacy` restores the pre-Slice-4.3
behaviour at every seam this slice touches
(`internal/observability/mode.go:34-53`):

- `/health` is a literal `{"status":"healthy","service":"graphql"}`,
  regardless of dependency state.
- `/ready` does not exist.
- No metrics listener starts.
- Session-stream fanout and notification de-duplication both go
  process-local instead of cross-replica.
- `FI_FHIR_BATCH_WORKER_ID` becomes required again, with no derived default.

It exists only so an automated negative-control test can prove itself
capable of failing. `serve` prints a warning to stderr when it is set
(`cmd/fi-fhir/main.go:4877-4883`). Do not set it outside that test.

## Health Endpoints

| Endpoint | Port | Purpose | Failure mode |
|----------|------|---------|--------------|
| `/health` | `FI_FHIR_SERVER_PORT` (default `8080`) | Liveness, process-only | Never returns 503 |
| `/ready` | `FI_FHIR_SERVER_PORT` (default `8080`) | Readiness, dependency-touching | `503` when any component is `unhealthy` |
| `/metrics` | `FI_FHIR_METRICS_PORT` (default `9090`) | Prometheus exposition | `404` outside the configured path |

## See Also

- [User Guide](../user-guide/README.md)
- [Developer Guide](../developer-guide/README.md)
