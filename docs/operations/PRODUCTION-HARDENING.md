# Production Hardening Guide

This guide covers security hardening for fi-fhir deployments in healthcare environments requiring HIPAA compliance.

## Table of Contents

1. [Security Overview](#security-overview)
2. [Container Security](#container-security)
3. [Kubernetes Security](#kubernetes-security)
4. [Network Security](#network-security)
5. [Secrets Management](#secrets-management)
6. [Encryption](#encryption)
7. [Audit Logging](#audit-logging)
8. [Access Control](#access-control)
9. [Monitoring & Alerting](#monitoring--alerting)
10. [Disaster Recovery](#disaster-recovery)

---

## Security Overview

### HIPAA Technical Safeguards

fi-fhir deployments handling PHI must implement:

| Safeguard | Implementation |
|-----------|----------------|
| **Access Control** | RBAC, service accounts, network policies |
| **Audit Controls** | Structured logging, trace correlation, event recording |
| **Integrity Controls** | Image signing, checksum verification, immutable infrastructure |
| **Transmission Security** | TLS 1.3, mTLS between services |
| **Encryption** | At-rest (database, secrets), in-transit (TLS) |

### Security Checklist

```
[ ] Container runs as non-root user
[ ] Read-only root filesystem
[ ] No privileged containers
[ ] Resource limits configured
[ ] Network policies applied
[ ] Secrets encrypted at rest
[ ] TLS enabled for all endpoints
[ ] Audit logging enabled
[ ] Health checks configured
[ ] Pod disruption budget set
[ ] Vulnerability scanning in CI
[ ] Image signatures verified
```

---

## Container Security

### Dockerfile Best Practices

The fi-fhir Dockerfile follows security best practices:

```dockerfile
# Multi-stage build minimizes attack surface
FROM golang:1.22-alpine AS builder
# ... build stage ...

# Distroless base - no shell, no package manager
FROM gcr.io/distroless/static-debian12:nonroot

# Run as non-root user (UID 65532)
USER nonroot:nonroot

# Binary only - minimal attack surface
COPY --from=builder --chown=nonroot:nonroot /fi-fhir /fi-fhir
```

### Image Scanning

Scan images before deployment:

```bash
# Trivy scan
trivy image fi-fhir:latest --severity CRITICAL,HIGH

# Grype scan
grype fi-fhir:latest

# Snyk scan
snyk container test fi-fhir:latest
```

### Image Signing (Cosign)

Sign images for supply chain security:

```bash
# Generate key pair
cosign generate-key-pair

# Sign image
cosign sign --key cosign.key registry.gitlab.flexinfer.ai/libs/fi-fhir:v1.0.0

# Verify signature
cosign verify --key cosign.pub registry.gitlab.flexinfer.ai/libs/fi-fhir:v1.0.0
```

---

## Kubernetes Security

### Pod Security Standards

Apply restrictive pod security:

```yaml
# namespace-security.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: fi-fhir
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### Security Context

The Helm chart applies these security contexts by default:

```yaml
# Pod-level security
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault

# Container-level security
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

### Resource Limits

Always set resource limits to prevent resource exhaustion:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
    ephemeral-storage: 100Mi
  requests:
    cpu: 100m
    memory: 128Mi
    ephemeral-storage: 50Mi
```

### Pod Disruption Budget

Ensure availability during cluster operations:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: fi-fhir
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: fi-fhir
```

---

## Network Security

### Network Policies

Default-deny with explicit allow rules:

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: fi-fhir-default-deny
  namespace: fi-fhir
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: fi-fhir-allow-ingress
  namespace: fi-fhir
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: fi-fhir
  policyTypes:
    - Ingress
  ingress:
    # Allow from ingress controller
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
    # Allow Prometheus scraping
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - protocol: TCP
          port: 9090
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: fi-fhir-allow-egress
  namespace: fi-fhir
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: fi-fhir
  policyTypes:
    - Egress
  egress:
    # Allow DNS
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53
    # Allow FHIR server
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8  # Internal network
      ports:
        - protocol: TCP
          port: 443
    # Allow database
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: database
      ports:
        - protocol: TCP
          port: 5432
```

### Service Mesh (Istio)

For mTLS between services:

```yaml
# peer-authentication.yaml
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: fi-fhir-mtls
  namespace: fi-fhir
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: fi-fhir
  mtls:
    mode: STRICT
---
# authorization-policy.yaml
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: fi-fhir-authz
  namespace: fi-fhir
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: fi-fhir
  action: ALLOW
  rules:
    - from:
        - source:
            principals:
              - cluster.local/ns/ingress-nginx/sa/ingress-nginx
      to:
        - operation:
            methods: ["GET", "POST"]
            paths: ["/api/*", "/health", "/ready"]
```

---

## Secrets Management

### Kubernetes Secrets (Encrypted)

Enable encryption at rest for secrets:

```yaml
# encryption-config.yaml (for kube-apiserver)
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <base64-encoded-32-byte-key>
      - identity: {}
```

### External Secrets Operator

For production, use External Secrets with HashiCorp Vault:

```yaml
# secret-store.yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
  namespace: fi-fhir
spec:
  provider:
    vault:
      server: https://vault.example.com
      path: secret
      version: v2
      auth:
        kubernetes:
          mountPath: kubernetes
          role: fi-fhir
          serviceAccountRef:
            name: fi-fhir
---
# external-secret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: fi-fhir-secrets
  namespace: fi-fhir
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: fi-fhir
    creationPolicy: Owner
  data:
    - secretKey: database-password
      remoteRef:
        key: fi-fhir/database
        property: password
    - secretKey: fhir-bearer-token
      remoteRef:
        key: fi-fhir/fhir
        property: bearer_token
```

### Sealed Secrets

Alternative for GitOps workflows:

```bash
# Install kubeseal
brew install kubeseal

# Seal a secret
kubectl create secret generic fi-fhir-secrets \
  --from-literal=database-password=secret \
  --dry-run=client -o yaml | \
  kubeseal --format yaml > sealed-secret.yaml

# Apply sealed secret
kubectl apply -f sealed-secret.yaml
```

---

## Encryption

### TLS Configuration

**Ingress TLS** with cert-manager:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: fi-fhir
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-ssl-protocols: "TLSv1.3"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - fi-fhir.example.com
      secretName: fi-fhir-tls
  rules:
    - host: fi-fhir.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: fi-fhir
                port:
                  name: http
```

**Database TLS**:

```yaml
# In values.yaml
config:
  database:
    enabled: true
    sslMode: verify-full  # Require TLS with certificate verification
```

### Data at Rest

For database encryption:

```sql
-- PostgreSQL: Enable pgcrypto
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Encrypt sensitive columns
ALTER TABLE workflow_events
  ALTER COLUMN payload
  SET DATA TYPE bytea
  USING pgp_sym_encrypt(payload::text, current_setting('app.encryption_key'))::bytea;
```

---

## Audit Logging

### Structured Logging Configuration

```yaml
# In values.yaml
config:
  observability:
    logLevel: info
    logFormat: json  # Structured JSON for log aggregation
    tracingEnabled: true
```

### Log Fields for Compliance

fi-fhir logs include:

```json
{
  "timestamp": "2024-01-15T10:30:00.123Z",
  "level": "info",
  "message": "Event processed",
  "trace_id": "abc123",
  "span_id": "def456",
  "event_type": "patient_admit",
  "source": "epic_adt",
  "action": "fhir",
  "duration_ms": 45,
  "status": "success"
}
```

### Kubernetes Audit Policy

```yaml
# audit-policy.yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all access to secrets
  - level: Metadata
    resources:
      - group: ""
        resources: ["secrets"]
    namespaces: ["fi-fhir"]

  # Log all changes to fi-fhir resources
  - level: RequestResponse
    verbs: ["create", "update", "patch", "delete"]
    namespaces: ["fi-fhir"]
```

---

## Access Control

### Transitional Preview Authentication

Slice 1.1c uses one static deployment bearer to activate stateless preview in a
single security domain. This is a narrow transitional control, not the Phase 4
OIDC, fine-grained RBAC, audited token-administration, or durable-session model.

Harden this boundary as follows:

- mount `FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE` from Vault, External Secrets, or an
  equivalent managed secret; do not put the bearer in Git, Helm values, a
  `PUBLIC_*` build variable, ConfigMap, localStorage, or sessionStorage;
- generate at least 24 canonical random bytes and rotate through the secret
  manager plus a controlled rollout;
- set `FI_FHIR_DEPLOYMENT_TENANT_ID`, `FI_FHIR_GRAPHQL_PRINCIPAL_ID`, and
  `FI_FHIR_GRAPHQL_ROLES=integration:preview` from deployment-owned config;
- list every browser origin exactly in
  `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS`; wildcards, paths, user information, query
  strings, and fragments are invalid;
- mount a strict immutable registry at
  `FI_FHIR_INTEGRATION_REGISTRY_PATH`; its tenant must equal the deployment
  tenant and its profile/workflow digests must match the definition; and
- disable Playground and introspection on internet-reachable deployments even
  though operation authorization still applies.

The `integration:preview` role permits only GraphQL `health` and
`previewIntegrationMessage`. Do not grant `graphql:operator` to an IDE token;
that role is a temporary authenticated escape hatch for legacy operations.

GraphQL HTTP accepts only bounded JSON POST requests and browser requests
require an exact allowed origin. GraphQL WebSocket transport is unmounted; the
UI fails subscription attempts locally without opening a socket. The preview service owns no receipt,
sample/run store, destination, or action client. Legacy submit, workflow,
session retention/export, and live-parse operations fail closed by default.

The Mapping Studio holds its bearer, imported raw HL7 samples, and
filename-derived source labels only in the current tab's JavaScript memory.
This prevents implicit browser persistence but does not make the open tab
PHI-free. Require approved workstations, session locking, and redacted fixtures
where possible.

The layout startup purges the two known legacy localStorage keys that stored raw
HL7 samples and recent source labels. This upgrade cleanup preserves unrelated
UI preferences.

Copy a Helm-managed bearer directly to the macOS clipboard without printing it:

```bash
kubectl -n fi-fhir get secret fi-fhir \
  -o jsonpath='{.data.graphql-bearer-token}' | base64 --decode | pbcopy
```

Adjust the secret name for the Helm release fullname, paste it only into the
credential gate, and clear the clipboard afterward.

### RBAC Configuration

```yaml
# rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: fi-fhir-operator
  namespace: fi-fhir
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: fi-fhir-operators
  namespace: fi-fhir
subjects:
  - kind: Group
    name: fi-fhir-operators
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: fi-fhir-operator
  apiGroup: rbac.authorization.k8s.io
```

### Service Account

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fi-fhir
  namespace: fi-fhir
automountServiceAccountToken: false  # Don't mount unless needed
```

---

## Monitoring & Alerting

### Critical Alerts

See `dashboards/alerting/workflow-alerts-k8s.yaml` for full alert rules:

```yaml
# Key alerts for production
groups:
  - name: fi-fhir-critical
    rules:
      - alert: FiFhirHighErrorRate
        expr: |
          rate(workflow_action_errors_total[5m])
          / rate(workflow_events_processed_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: High error rate in fi-fhir workflow

      - alert: FiFhirDLQBacklog
        expr: workflow_dlq_size > 100
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: Dead letter queue growing

      - alert: FiFhirCircuitBreakerOpen
        expr: workflow_circuit_breaker_state == 2
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: Circuit breaker open - external service failing
```

### SLO Targets

| Metric | Target | Alert Threshold |
|--------|--------|-----------------|
| Availability | 99.9% | < 99.5% |
| Latency (p99) | < 500ms | > 1s |
| Error Rate | < 0.1% | > 1% |
| DLQ Size | 0 | > 100 |

---

## Disaster Recovery

### Backup Strategy

```bash
# Database backup (PostgreSQL)
pg_dump -h $DB_HOST -U $DB_USER -d fi_fhir | \
  gzip | \
  aws s3 cp - s3://backups/fi-fhir/$(date +%Y%m%d).sql.gz

# Workflow configuration backup
kubectl get configmap fi-fhir -n fi-fhir -o yaml > workflow-config-backup.yaml

# Secrets backup (encrypted)
kubectl get secret fi-fhir -n fi-fhir -o yaml | \
  kubeseal --format yaml > sealed-secret-backup.yaml
```

### Recovery Procedures

1. **Database Recovery**:
   ```bash
   aws s3 cp s3://backups/fi-fhir/latest.sql.gz - | \
     gunzip | \
     psql -h $DB_HOST -U $DB_USER -d fi_fhir
   ```

2. **Application Recovery**:
   ```bash
   # Redeploy from Helm
   helm upgrade fi-fhir deploy/helm/fi-fhir/ \
     -f production-values.yaml \
     --namespace fi-fhir
   ```

3. **DLQ Replay** (after recovery):
   ```bash
   # Replay failed events from dead letter queue
   ./fi-fhir workflow replay --dlq --since 24h
   ```

### RTO/RPO Targets

| Scenario | RTO | RPO |
|----------|-----|-----|
| Pod failure | 30s | 0 |
| Node failure | 5m | 0 |
| Database failure | 15m | 5m |
| Full cluster failure | 1h | 15m |

---

## Security Hardening Checklist

### Pre-Deployment

- [ ] Vulnerability scan passed (no CRITICAL/HIGH)
- [ ] Image signed with cosign
- [ ] Secrets stored in Vault/External Secrets
- [ ] Preview bearer is mounted from a managed secret file and is not in Git or Helm release values
- [ ] Deployment tenant, principal, preview role, exact origins, and registry tenant/digests agree
- [ ] Network policies applied
- [ ] RBAC configured (principle of least privilege)
- [ ] TLS certificates provisioned
- [ ] Audit logging enabled

### Post-Deployment

- [ ] Health checks passing (`/health`, `/ready`)
- [ ] Missing/wrong bearer and disallowed origins fail; preview role cannot call legacy operations
- [ ] Mapping Studio reload clears bearer and imported raw samples
- [ ] Metrics being scraped
- [ ] Alerts configured and tested
- [ ] Backup procedures tested
- [ ] Runbook reviewed by operations team
- [ ] Incident response plan documented

### Periodic Review

- [ ] Quarterly: Rotate secrets and certificates
- [ ] Monthly: Review audit logs for anomalies
- [ ] Weekly: Check vulnerability scan results
- [ ] Daily: Monitor alert dashboards
