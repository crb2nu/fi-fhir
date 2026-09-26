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

### Dedicated Operator and Preview Service Access

For a backend client such as MentatLab, keep the existing static IDE credential
and `FI_FHIR_GRAPHQL_ROLES` unchanged. Mount a separate managed secret file and
add these settings through the deployment's GitOps configuration:

```text
FI_FHIR_GRAPHQL_AUTH_MODE=static
FI_FHIR_GRAPHQL_SERVICE_BEARER_TOKEN_FILE=/var/run/secrets/fi-fhir-service/bearer-token
FI_FHIR_GRAPHQL_SERVICE_PRINCIPAL_ID=mentatlab
FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true
```

The two service credential settings must appear together. Startup rejects a
reused IDE token or principal, malformed/oversized secret files, and service
settings in OIDC mode. The service always uses the deployment tenant and exactly
`integration.operator,integration:preview`; neither `clinical:read` nor
`graphql:operator` is granted. Secret rotation requires a controlled restart.
Keep the credential in the client backend, never browser configuration.

The operator flag requires the existing PostgreSQL connection settings and
applies the existing submission/delivery, lifecycle, and destination migrations. It makes the
operator reads available without enabling an ingress, delivery dispatcher, or
session workspace. Its database identity needs schema migration privileges;
the HTTP credential still cannot replay, resubmit, discard, or deploy. The
default remains disabled, while deployments already running durable integration
features keep their existing operator initialization.

Validate with the client's bounded `operatorDeployments`, `operatorCircuits`,
`operatorDeliveryAttempts`, and `operatorDeadLetters` queries, then a fixed
synthetic `previewIntegrationMessage`. Choose the deployed registry's
`integrations[].integration_id`; it is not necessarily a definition or source
ID. An empty operator inventory is valid and must not be replaced with fixtures.
Never use real patient payloads for this connection check.

Explicit bearer headers now take precedence over trusted-network and Access
identity. A stale, empty, or repeated bearer header returns 401 even on the LAN.
Remove a stale header to use an existing headerless LAN/Access session; valid
IDE credentials are unchanged. This prevents service callers from inheriting
the broader IDE roles through network trust.

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

### S3/SFTP Batch Ingestion

Check whether polling is intentionally enabled without printing secret values:

```bash
kubectl -n fi-fhir get deployment fi-fhir \
  -o jsonpath='{.spec.template.spec.containers[0].env}' | \
  jq '.[] | select(.name == "FI_FHIR_BATCH_SOURCE_CONFIG_PATH") | .name'
```

Startup fails closed when the immutable source, deployed lifecycle binding,
PostgreSQL, provider credentials, TLS policy, or SFTP host key is invalid. For an
outage:

1. Preserve the source object and PostgreSQL state; do not rename, rewrite, or
   manually archive an in-flight file.
2. Restore PostgreSQL and provider connectivity while retaining the same source
   revision and credentials/host-key binding.
3. Restart the worker. The expired lease is reclaimed and processing resumes
   from the last checkpoint; the first replayed admission is idempotent.
4. Confirm the digest-addressed archive exists before any manual source cleanup.
   The normal worker commits completion before deletion; S3 targets its exact
   version ID and SFTP re-verifies the unchanged path's content digest.

Safe diagnostic state is available in `integration_batch_objects` and
`integration_batch_audit`. Query only object hashes, phase, checkpoint counters,
lease timestamps, and safe error codes. Do not copy source paths, credentials,
or message content into logs or incident tickets.

To disable only polling, remove `FI_FHIR_BATCH_SOURCE_CONFIG_PATH` through
GitOps and reconcile. PostgreSQL checkpoints remain available for a later
restart. See [`BATCH-INGESTION.md`](BATCH-INGESTION.md) for configuration and
the recovery proof.

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

### Operator page says the role is missing

**Symptoms**: the IDE's Operator page says the account does not hold the
operator role, or operator queries answer `operator control-plane action
forbidden` — from the LAN, through Cloudflare Access, and with the static
bearer alike.

**Cause**: the identity holds `graphql:operator`, the transport gate's
compatibility grant, but not `integration.operator`, the operator control
plane's own role. The gate admits the request and `operator.Service` refuses
it. This was the production outage from 2026-09-05 to 2026-09-25: the API
Deployment granted `integration:preview,graphql:operator,clinical:read` to the
bearer, the trusted network, and both Access principals, so nobody could reach
the operator plane. The trusted network was never the problem — it inherits
the same roles as everything else.

**Check**:
```bash
# What the server thinks this browser/LAN client is, and what it lacks.
curl -s https://<ui-host>/api/auth/status
#   capabilities.operatorRead=false, missingRoles.operatorRead=["integration.operator"]

# serve names every misconfigured identity once at startup.
kubectl -n fi-fhir logs deployment/fi-fhir | \
  grep 'transport grant without control-plane role'
```

**Resolution**: grant the operator bundle where that identity's roles come
from. There are three places, and a deployment usually needs all of them:

1. `FI_FHIR_GRAPHQL_ROLES` — the static bearer's roles.
2. The trusted network (`FI_FHIR_GRAPHQL_TRUSTED_CIDRS`) — it has no roles of
   its own and **inherits `FI_FHIR_GRAPHQL_ROLES`**, so fixing (1) fixes it.
3. `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS` — each operator's `email=roles` entry.

```text
integration:preview,graphql:operator,clinical:read,integration.operator,integration.delivery.operator,integration.deployment.operator
```

In OIDC mode the same list belongs in the identity provider's roles claim. The
service bearer's roles are fixed (`integration.operator,integration:preview`)
and are not the IDE's. Change the Deployment's environment through GitOps and
let the rollout restart the pods; roles are read at startup.

**Verify**: `/api/auth/status` reports `operatorRead: true` with an empty
`missingRoles.operatorRead`, the startup warning is gone, and
`{ operatorCircuits { state } }` returns data. Do not "fix" this by making
`graphql:operator` imply the service roles in code; the two vocabularies are
deliberate (decision 2026-09-25, "Grant the operator bundle rather than alias
the transport grant"). The contract is in
[GRAPHQL-API.md](../planning/GRAPHQL-API.md#the-apiauthstatus-contract).

### Live streaming is unavailable

**Symptoms**: an IDE panel shows "Live streaming for … is not available on this
deployment" (`data-testid="streaming-unavailable"`, with `data-stream` naming
the subscription and `data-reason` giving the cause) instead of a live feed. No
toast accompanies it. A direct SSE request (`POST /graphql` with
`Accept: text/event-stream`) fails in one of two ways:

| Answer | `data-reason` | Meaning |
|---|---|---|
| HTTP 404, body `Integration Session streaming is unavailable` | `streaming-off` | The API has the session workspace off (`FI_FHIR_INTEGRATION_SESSION_ENABLED` unset), so no subscription can open. `/api/auth/status` reports `streaming: false` and `subscriptions: []` |
| HTTP 200 `text/event-stream` whose event is `GraphQL stream operation forbidden` (`"code":"FORBIDDEN"`) | `not-allowlisted` | Streaming is on, but the subscription is not on the SSE allowlist, or the caller's roles do not clear the transport gate for it |

**Which panels can stream.** The SSE transport admits exactly two subscription
roots, `integrationSessionEvents` and `sessionRunEvents`
(`integrationSessionStreamRoots` in
`internal/api/graphql/operation_authorization.go`). They carry redacted
Integration Session run stages, diagnostics and lineage. Every other
subscription is refused on the stream, even for `graphql:operator`:

| Panel | Subscription | On this deployment |
|---|---|---|
| HL7 intake: Integration Session run progress | `integrationSessionEvents` | Streams when the session workspace is on |
| Events → Live Stream | `eventStream` | Never streams |
| Workflows → Monitor | `workflowEvents` | Never streams |
| Debug | `debugStepEvent` | Never streams |
| Runtime Output (bottom panel) | `workflowEvents` or `eventStream` | Never streams |

The four panels that never stream are working as designed. The allowlist is
the durable API's PHI-minimal stream surface by design (IDE repair program,
`.loom/36` corrections). The session roots carry redacted run stages,
diagnostics and lineage. The legacy roots carry event payloads and runtime
state from the pre-durable engine. Do not widen the allowlist to make a panel
light up. Each honest state names the alternative on that surface: recorded
events in the Events browser, completed runs under Run Diagnostics in the
Workflows monitor, Dry Run in the workflow builder, and step-on-request in
Debug.

**How the UI decides.** The credential gate reads `/api/auth/status` once per
page load. Each panel resolves its own subscription root from `streaming` and
`subscriptions` (`ui/src/lib/graphql/streamAvailability.ts`), renders the
honest state, and never opens a stream it cannot get. If capabilities are
unknown (an API older than the status contract), a panel tries once. An HTTP
404 or a "stream operation forbidden" answer then marks that root unavailable
for the rest of the page session, so a status that went stale after a redeploy
corrects itself the first time a panel tries. HL7 intake is different. It
shows the note only when the UI build enabled the session engine
(`VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true`, the image default). Its
**Preview** keeps working on the stateless path. A build with `=false` shows no
note because it never offers session runs.

**Check**:
```bash
# What the server says for this caller (from the LAN, the trusted network).
curl -s https://<ui-host>/api/auth/status | python3 -m json.tool
#   capabilities.streaming, capabilities.integrationSessions, capabilities.subscriptions

# Probe the one stream that should open (does not create a session).
curl -sS -N --max-time 5 -o /dev/null -w 'http=%{http_code}\n' \
  -H 'Content-Type: application/json' -H 'Accept: text/event-stream' \
  -H 'Origin: https://<ui-host>' \
  --data '{"query":"subscription Probe { integrationSessionEvents(sessionId: \"probe\") { id } }"}' \
  https://<ui-host>/graphql
#   http=404 → session workspace off; http=200 → on
```

**Resolution**:
- `streaming-off` on the HL7 session note: enable the session workspace. See
  [INTEGRATION-SESSIONS.md, Production](INTEGRATION-SESSIONS.md#production) for
  the exact environment entry, the database it migrates, and rollback.
- `not-allowlisted` on `integrationSessionEvents` / `sessionRunEvents`: the
  identity lacks `graphql:operator`, which the session roots require at the
  transport gate. Grant it where that identity's roles come from (see
  [Operator page says the role is missing](#operator-page-says-the-role-is-missing)
  for the three places).
- Any reason on Live Stream, Workflow Monitor, Debug or Runtime Output: no
  action. The state is correct for this deployment.

**Verify**: `/api/auth/status` lists both session roots in `subscriptions`,
the probe answers `http=200`, and after a page reload HL7 intake's **Preview**
shows Integration Session run progress. Tabs opened before a change keep the
capabilities they loaded with. Reload them.

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

For the production integration outbox, use
[`DELIVERY-RELIABILITY.md`](DELIVERY-RELIABILITY.md). Inspect only bounded DLQ
metadata, repair Kafka first, then use the audited `fi-fhir delivery replay` or
`resubmit` command with a unique operation key and reason. Disable
`FI_FHIR_DELIVERY_WORKER_ENABLED` to stop publication without deleting work.

The commands below apply only to the legacy in-process workflow DLQ.

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
