# Operations Guide

Documentation for deploying and operating fi-fhir in production environments.

## Contents

- [Production Hardening](PRODUCTION-HARDENING.md) - Security best practices
- [Operations Runbook](RUNBOOK.md) - Troubleshooting and procedures
- [Supported 1.0 Baseline](SUPPORTED-1.0.md) - Pinned release target and evidence gaps

## Quick Links

| Task | Document |
|------|----------|
| Secure a deployment | [Production Hardening](PRODUCTION-HARDENING.md) |
| Troubleshoot issues | [Operations Runbook](RUNBOOK.md) |
| Review the 1.0 support target | [Supported 1.0 Baseline](SUPPORTED-1.0.md) |
| Monitor performance | [Observability](#observability) |
| Configure health checks | [Health Endpoints](#health-endpoints) |

## Deployment Options

### Docker Compose (Development)

```bash
docker-compose up -d
```

### Kubernetes (Production)

```bash
# Using Kustomize
kubectl apply -k deploy/kubernetes/overlays/production/

# Using Helm
helm install fi-fhir deploy/helm/fi-fhir/ -f values-prod.yaml
```

## Observability

### Metrics

Prometheus metrics are exposed at `/metrics`:

```
workflow_events_processed_total
workflow_action_duration_seconds
workflow_action_errors_total
workflow_dlq_size
workflow_circuit_breaker_state
```

### Tracing

OpenTelemetry tracing is configured via environment:

```bash
FI_FHIR_TRACING_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
```

### Logging

Structured JSON logs with trace correlation:

```json
{
  "level": "info",
  "msg": "event processed",
  "event_id": "evt_abc123",
  "trace_id": "abc123def456",
  "span_id": "span789"
}
```

## Health Endpoints

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `/health` | Liveness | `{"status": "ok"}` |
| `/ready` | Readiness | Checks dependencies |
| `/metrics` | Prometheus | Metrics in text format |

## See Also

- [User Guide](../user-guide/README.md)
- [Developer Guide](../developer-guide/README.md)
