# fi-fhir Operations Runbook

Operational procedures for running fi-fhir in production.

## Service Information

| Field | Value |
|-------|-------|
| **Service Name** | fi-fhir |
| **Repository** | https://gitlab.flexinfer.ai/libs/fi-fhir |
| **Primary Language** | Go |
| **Default Port** | 8080 (API), 9090 (metrics) |
| **Health Endpoint** | `/health` |
| **Ready Endpoint** | `/ready` |

---

## Quick Reference

### Common Commands

```bash
# Check deployment status
kubectl -n fi-fhir get pods
kubectl -n fi-fhir describe deployment fi-fhir

# View logs
kubectl -n fi-fhir logs -f deployment/fi-fhir

# View logs with trace ID filter
kubectl -n fi-fhir logs deployment/fi-fhir | jq 'select(.trace_id == "abc123")'

# Port forward for debugging
kubectl -n fi-fhir port-forward svc/fi-fhir 8080:80

# Check metrics
curl http://localhost:9090/metrics | grep workflow_

# Restart deployment
kubectl -n fi-fhir rollout restart deployment/fi-fhir

# Scale deployment
kubectl -n fi-fhir scale deployment/fi-fhir --replicas=5
```

### CLI Commands

```bash
# Parse a message
./fi-fhir parse --format hl7v2 --pretty < message.hl7

# Validate workflow configuration
./fi-fhir workflow validate workflow.yaml

# Run workflow in dry-run mode
./fi-fhir workflow run --dry-run --config workflow.yaml < events.json

# Check configuration
./fi-fhir config show
./fi-fhir config validate

# View version
./fi-fhir version
```

---

## Monitoring

### Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `workflow_events_processed_total` | Total events processed | N/A (counter) |
| `workflow_events_in_progress` | Currently processing | > 100 |
| `workflow_action_duration_seconds` | Action latency | p99 > 1s |
| `workflow_action_errors_total` | Action failures | rate > 0.01 |
| `workflow_dlq_size` | Dead letter queue depth | > 100 |
| `workflow_circuit_breaker_state` | Circuit breaker status | == 2 (open) |
| `workflow_rate_limiter_rejected_total` | Rate limited requests | rate > 10/s |

### Dashboards

- **Grafana**: `dashboards/grafana/workflow-overview.json`
- Import dashboard via Grafana UI or provision via ConfigMap

### Log Queries (Loki/Elasticsearch)

```
# Find errors in last hour
{namespace="fi-fhir"} |= "error" | json | level="error"

# Find slow actions (> 500ms)
{namespace="fi-fhir"} | json | duration_ms > 500

# Find by trace ID
{namespace="fi-fhir"} | json | trace_id="abc123"

# Find DLQ events
{namespace="fi-fhir"} |= "dlq" | json
```

---

## Common Operations

### Scaling

**Manual scaling**:
```bash
kubectl -n fi-fhir scale deployment/fi-fhir --replicas=5
```

**Enable autoscaling**:
```bash
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10 \
  --reuse-values
```

### Configuration Updates

**Update workflow configuration**:
```bash
# Edit configmap
kubectl -n fi-fhir edit configmap fi-fhir

# Or via Helm
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set-file workflowConfig=new-workflow.yaml \
  --reuse-values

# Pods will automatically restart (checksum annotation)
```

**Update environment variables**:
```bash
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set config.observability.logLevel=debug \
  --reuse-values
```

### Authenticated Preview Access

`fi-fhir serve` fails startup closed unless all preview inputs are present:

- deployment tenant and server-owned principal IDs;
- roles containing `integration:preview`;
- one or more comma-separated exact HTTP(S) origins;
- exactly one bearer source with at least 24 canonical bytes; and
- an immutable integration registry for the same deployment tenant.

Use `FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE` for production. The direct
`FI_FHIR_GRAPHQL_BEARER_TOKEN` variable is intended for local development.
Never set both.

To copy a Helm-managed token into the macOS clipboard without writing it to
terminal output, shell history, or a temporary file:

```bash
NAMESPACE=fi-fhir
SECRET_NAME=fi-fhir
kubectl --namespace "$NAMESPACE" get secret "$SECRET_NAME" \
  -o jsonpath='{.data.graphql-bearer-token}' | base64 --decode | pbcopy
unset NAMESPACE SECRET_NAME
```

Adjust `SECRET_NAME` when the Helm release fullname differs. Paste the value
into the Mapping Studio credential gate, then clear the clipboard. The UI holds
the bearer only in tab memory and clears it on reload or **Clear access**. Raw
HL7 samples are also tab-memory only and disappear on reload.

The transitional `integration:preview` role can call only GraphQL `health` and
`previewIntegrationMessage`. A successful credential does not unlock legacy
submit, workflow execution, session retention/export, or subscriptions. Those
paths remain unavailable until their production security and durability
boundaries ship.

When preview startup fails, inspect only catalog-safe logs:

```bash
kubectl -n fi-fhir logs deployment/fi-fhir --tail=100 | \
  grep -E 'GraphQL|integration registry|deployment tenant'
```

Do not log, echo, or add the bearer or raw clinical message to an incident
ticket. Validate exact origins, registry tenant/digests, secret mount, and role
configuration before rotating the credential.

### Durable HL7v2 Ingress

Check whether the endpoint is intentionally enabled:

```bash
kubectl -n fi-fhir get deployment fi-fhir \
  -o jsonpath='{.spec.template.spec.containers[0].env}' | \
  jq '.[] | select(.name | startswith("FI_FHIR_HTTP_INGRESS")) | .name'
```

Startup fails closed when the credential, integration binding, or PostgreSQL is
invalid. For request failures, use the structured response code:

- `401`: rotate or correct the bound bearer/HMAC credential.
- `404 INTEGRATION_UNAVAILABLE`: verify the credential-bound integration exists.
- `409 IDEMPOTENCY_CONFLICT`: stop retries; the key was committed for other bytes.
- `422 INVALID_HL7V2_MESSAGE`: correct the message or selected Source Profile.
- `503`/`504`: retry with the same idempotency key after database recovery.

Never place raw HL7, credentials, or persisted JSON in logs or tickets. To
disable only production ingress, remove `FI_FHIR_HTTP_INGRESS_AUTH_MODE` through
GitOps and reconcile; authenticated GraphQL preview remains available.

### Deployment

**Rolling update**:
```bash
# Update image tag
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set image.tag=v1.2.0 \
  --reuse-values

# Monitor rollout
kubectl -n fi-fhir rollout status deployment/fi-fhir
```

**Rollback**:
```bash
# View history
kubectl -n fi-fhir rollout history deployment/fi-fhir

# Rollback to previous
kubectl -n fi-fhir rollout undo deployment/fi-fhir

# Rollback to specific revision
kubectl -n fi-fhir rollout undo deployment/fi-fhir --to-revision=3

# Helm rollback
helm rollback fi-fhir 1
```

---

## Incident Response

### Severity Levels

| Level | Description | Response Time | Examples |
|-------|-------------|---------------|----------|
| **P1** | Complete outage | Immediate | All pods down, no events processing |
| **P2** | Degraded service | 15 minutes | High error rate, circuit breaker open |
| **P3** | Minor issue | 1 hour | Elevated latency, DLQ growing |
| **P4** | Low impact | Next business day | Single failed event, log warnings |

### Triage Steps

1. **Check service health**:
   ```bash
   kubectl -n fi-fhir get pods
   kubectl -n fi-fhir describe pod <pod-name>
   ```

2. **Check recent logs**:
   ```bash
   kubectl -n fi-fhir logs deployment/fi-fhir --since=10m | tail -100
   ```

3. **Check metrics**:
   ```bash
   curl -s http://localhost:9090/metrics | grep -E "workflow_(errors|dlq|circuit)"
   ```

4. **Check dependencies**:
   ```bash
   # Database connectivity
   kubectl -n fi-fhir exec deployment/fi-fhir -- nc -zv postgres 5432

   # FHIR server connectivity
   kubectl -n fi-fhir exec deployment/fi-fhir -- nc -zv fhir-server 443
   ```

---

## Troubleshooting

### Pod Not Starting

**Symptoms**: Pod in `CrashLoopBackOff` or `Error` state

**Check**:
```bash
kubectl -n fi-fhir describe pod <pod-name>
kubectl -n fi-fhir logs <pod-name> --previous
```

**Common causes**:
- Invalid configuration: Check `config validate` output
- Missing secrets: Verify secret exists and has required keys
- Resource limits: Check if OOMKilled

**Resolution**:
```bash
# Fix configuration
./fi-fhir config validate

# Check secrets
kubectl -n fi-fhir get secret fi-fhir -o yaml

# Increase resources
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set resources.limits.memory=1Gi \
  --reuse-values
```

### High Error Rate

**Symptoms**: `FiFhirHighErrorRate` alert firing

**Check**:
```bash
# Find error patterns
kubectl -n fi-fhir logs deployment/fi-fhir | jq 'select(.level=="error")' | head -20

# Check specific action errors
curl -s http://localhost:9090/metrics | grep workflow_action_errors
```

**Common causes**:
- External service down (FHIR server, database)
- Authentication expired (OAuth token)
- Rate limiting by external service

**Resolution**:
```bash
# Check circuit breaker state
curl -s http://localhost:9090/metrics | grep circuit_breaker

# If circuit breaker is open, wait for half-open or force reset
kubectl -n fi-fhir delete pod -l app.kubernetes.io/name=fi-fhir

# Check OAuth token
kubectl -n fi-fhir logs deployment/fi-fhir | grep -i oauth
```

### Dead Letter Queue Growing

**Symptoms**: `FiFhirDLQBacklog` alert firing

**Check**:
```bash
# Check DLQ size
curl -s http://localhost:9090/metrics | grep workflow_dlq_size

# View DLQ entries in logs
kubectl -n fi-fhir logs deployment/fi-fhir | jq 'select(.message | contains("dlq"))'
```

**Resolution**:
```bash
# Investigate root cause first
# Then replay when issue is resolved
./fi-fhir workflow replay --dlq --since 24h --dry-run  # Preview
./fi-fhir workflow replay --dlq --since 24h            # Execute
```

### High Latency

**Symptoms**: `FiFhirHighLatency` alert firing, p99 > 1s

**Check**:
```bash
# Check latency metrics
curl -s http://localhost:9090/metrics | grep workflow_action_duration

# Find slow requests in logs
kubectl -n fi-fhir logs deployment/fi-fhir | jq 'select(.duration_ms > 500)'
```

**Common causes**:
- Database slow queries
- External service latency
- Insufficient resources

**Resolution**:
```bash
# Scale up
kubectl -n fi-fhir scale deployment/fi-fhir --replicas=5

# Check database
kubectl -n fi-fhir exec deployment/fi-fhir -- \
  psql -c "SELECT * FROM pg_stat_activity WHERE state='active'"

# Increase connection pool
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set config.database.maxOpenConns=50 \
  --reuse-values
```

### Circuit Breaker Open

**Symptoms**: `FiFhirCircuitBreakerOpen` alert firing

**Check**:
```bash
curl -s http://localhost:9090/metrics | grep circuit_breaker_state
# 0 = closed, 1 = half-open, 2 = open
```

**Resolution**:
1. Identify failing external service from logs
2. Verify external service is healthy
3. Wait for circuit breaker to transition to half-open
4. If urgent, restart pods to reset circuit breaker state

```bash
# Force circuit breaker reset
kubectl -n fi-fhir rollout restart deployment/fi-fhir
```

### Memory Issues (OOMKilled)

**Symptoms**: Pod restarts with reason `OOMKilled`

**Check**:
```bash
kubectl -n fi-fhir describe pod <pod-name> | grep -A5 "Last State"
kubectl top pods -n fi-fhir
```

**Resolution**:
```bash
# Increase memory limits
helm upgrade fi-fhir deploy/helm/fi-fhir/ \
  --set resources.limits.memory=1Gi \
  --set resources.requests.memory=512Mi \
  --reuse-values
```

---

## Maintenance

### Certificate Rotation

```bash
# Check certificate expiry
kubectl -n fi-fhir get certificate fi-fhir-tls -o yaml | grep -A5 status

# Force renewal (cert-manager)
kubectl -n fi-fhir delete certificate fi-fhir-tls
# cert-manager will automatically create new certificate
```

### Secret Rotation

```bash
# Update database password
kubectl -n fi-fhir create secret generic fi-fhir-new \
  --from-literal=database-password=newpassword

# Update deployment to use new secret
# Then delete old secret after verification
```

### Database Maintenance

```bash
# Vacuum and analyze
kubectl -n fi-fhir exec deployment/fi-fhir -- \
  psql -c "VACUUM ANALYZE workflow_events;"

# Check table sizes
kubectl -n fi-fhir exec deployment/fi-fhir -- \
  psql -c "SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) FROM pg_stat_user_tables ORDER BY pg_total_relation_size(relid) DESC;"
```

### Log Rotation

Logs are managed by Kubernetes. For long-term retention:
- Configure log aggregation (Loki, Elasticsearch)
- Set retention policies appropriate for HIPAA (typically 6 years)

---

## Emergency Procedures

### Complete Service Outage

1. **Verify outage scope**:
   ```bash
   kubectl -n fi-fhir get all
   ```

2. **Check cluster health**:
   ```bash
   kubectl get nodes
   kubectl get events --all-namespaces --sort-by='.lastTimestamp' | tail -20
   ```

3. **Attempt restart**:
   ```bash
   kubectl -n fi-fhir rollout restart deployment/fi-fhir
   ```

4. **If restart fails, redeploy**:
   ```bash
   helm upgrade fi-fhir deploy/helm/fi-fhir/ -f production-values.yaml
   ```

5. **If namespace is corrupted**:
   ```bash
   kubectl delete namespace fi-fhir
   helm install fi-fhir deploy/helm/fi-fhir/ -f production-values.yaml -n fi-fhir --create-namespace
   ```

### Data Recovery

See [PRODUCTION-HARDENING.md](PRODUCTION-HARDENING.md#disaster-recovery) for backup/restore procedures.

### Rollback Bad Release

```bash
# Identify last good release
helm history fi-fhir

# Rollback
helm rollback fi-fhir <revision>

# Verify
kubectl -n fi-fhir rollout status deployment/fi-fhir
```

---

## Contact Information

| Role | Contact |
|------|---------|
| On-call Engineer | PagerDuty: fi-fhir-oncall |
| Team Lead | @team-lead |
| Security | security@example.com |
| Database Admin | dba@example.com |

---

## Appendix

### Environment Variables

See `fi-fhir config env` for complete list:

```bash
./fi-fhir config env
```

### Helm Values Reference

```bash
# View all configurable values
helm show values deploy/helm/fi-fhir/
```

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Liveness check |
| `/ready` | GET | Readiness check |
| `/metrics` | GET | Prometheus metrics |
| `/api/v1/parse` | POST | Parse message |
| `/api/v1/workflow` | POST | Process event |
