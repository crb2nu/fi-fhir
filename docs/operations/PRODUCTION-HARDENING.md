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
11. [Batch Source Security](#batch-source-security)
12. [MLLP Client Identity](#mllp-client-identity)

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

## MLLP Client Identity

The deployed HL7v2 MLLP listener is opt-in. Follow the complete operator
contract in [`PRODUCTION-MLLP.md`](PRODUCTION-MLLP.md).

- Require TLS 1.3 mutual authentication for every network-reachable listener.
  Plaintext MLLP is acceptable only on an independently protected loopback or
  same-pod sidecar boundary.
- Treat a CA-valid certificate as authentication, not authorization. Declare a
  `clients.identities` map in the immutable source revision so every connection
  resolves to one canonical service subject before any frame is read.
- Key identity on authority-scoped values only: a URI subject alternative name
  (`uri_san`) and/or a subject public key info pin (`spki_sha256`). Do not rely
  on common names; RFC 6125 deprecates common-name identity matching.
- Issue one certificate per sending system. Sharing one certificate across
  senders collapses them into a single audited subject and defeats per-sender
  revocation.
- Grant the minimum submit authority per subject. An identity provisioned for
  observation should carry no recognized submit grant; it then authenticates but
  never reaches artifact loading or durable admission.
- Set `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY=true` in production so the process
  refuses to start if the mounted source document ever drops the identity map.
- Rotate senders by publishing a new source revision, a new integration
  definition revision, and a lifecycle redeploy. The deployed release pins the
  exact source digest, so editing the mounted document alone fails closed.
- Keep the client CA bundle scoped to the MLLP trust domain. A shared corporate
  CA turns every certificate it issues into a candidate peer, leaving the
  identity map as the only remaining boundary.
- Alert on repeated unmapped-certificate rejections and on ungranted-identity
  denials. Do not include certificate subjects, URI SANs, or message bytes in
  alert labels.

---

## Batch Source Security

S3/SFTP ingestion is opt-in and must be activated through reviewed deployment
configuration. Follow the complete operator contract in
[`BATCH-INGESTION.md`](BATCH-INGESTION.md).

- Mount S3 credentials, SFTP private keys/passwords, and SFTP `known_hosts` as
  separate read-only secrets. Never embed secret values in the immutable source
  revision.
- Require TLS for remote S3 endpoints. Plaintext S3 is accepted only on
  loopback for local testing or a same-pod sidecar.
- Enable S3 bucket versioning. Polling fails closed without it so source cleanup
  can delete the immutable version that was actually admitted and archived.
- Build `known_hosts` out of band from a verified host-key fingerprint. Do not
  populate it with an unauthenticated runtime `ssh-keyscan`.
- Grant the source principal list/read/delete only under its input prefix and
  create/read only under its archive prefix. Deny bucket-wide administrative
  operations.
- Keep the input and archive prefixes/directories disjoint. SFTP symlinks are
  rejected; preserve that boundary in server-side filesystem permissions.
- Require SFTP producers to upload under a temporary name and atomically rename
  complete files into the input directory. Deny overwrite/truncate after
  publication; SFTP lacks a conditional unlink primitive, so immutable-drop
  ACLs are part of the pre-delete digest-verification boundary.
- Encrypt PostgreSQL and archive storage at rest and restrict checkpoint/audit
  table access. Although checkpoint state is raw-free, it remains operational
  metadata for a PHI-bearing data flow.
- Alert on repeated lease reclaim, invalid-stream quarantine, archive collision,
  host-key failure, or provider unavailability. Do not include source paths or
  message bytes in alert labels.
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

### GraphQL Human Authentication

Phase 4 Slice 4.1a supports per-request OIDC human identity for GraphQL POST and
SSE. Configure `FI_FHIR_GRAPHQL_AUTH_MODE=oidc`, an exact HTTPS issuer URL and
audience, the deployment tenant, and exact allowed origins. The default claims
are `sub`, `tenant_id`, and a strict `roles` string array; claim names and the
default `RS256` algorithm allowlist are deployment configurable. The runtime
requires a signed JWT access token with protected `typ=at+jwt`, then validates
issuer, one exact audience, signature, time window, subject, and exact tenant
before operation authorization. This token-class boundary is not a claim of
complete RFC 9068 conformance.

Discovery metadata, `jwks_uri`, and outbound requests are HTTPS-only; redirects
are rejected.
The runtime bounds each request to at most 10 seconds, caps discovery and JWKS
responses at 1 MiB, and allows at most one outbound JWKS refresh per 30-second
default window. Publish rotated keys before issuing tokens that use them so the
abuse bound does not create an avoidable authentication delay.

OIDC mode rejects static bearer, principal, roles, and trusted-CIDR settings.
The checked-in Helm/Kustomize deployment remains on the static compatibility
path until a separate production GitOps activation review.

Static mode preserves the Slice 1.1c single-deployment bearer for local and
preview compatibility. Harden that boundary as follows:

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

### HL7v2 Production Ingress

The endpoint is absent unless `FI_FHIR_HTTP_INGRESS_AUTH_MODE` is `bearer`,
`hmac-sha256`, or `oauth2`. Enabling it requires PostgreSQL and applies the fixed
submission migration during startup. It never falls back to an in-memory
committer.

OAuth2 mode authenticates each confidential client as a distinct service
principal. Configure an exact HTTPS issuer and audience, the deployment tenant,
and a deployment-owned allowlist in
`FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS`. The authorization server must
issue a signed JWT access token with protected `typ=at+jwt`, exact issuer and
single audience, valid time window, the deployment tenant, a strict roles array
containing `integration:submit`, and canonical `sub` and `client_id` claims that
are equal. fi-fhir projects only the required submit grant; extra token roles do
not expand authority. The immutable registry, not the token or request headers,
owns the integration revision and source identity.

This is a constrained resource-server profile, not a token endpoint or a claim
of universal OAuth support. OAuth 2.0 client credentials does not prescribe the
access-token representation, so providers that issue opaque tokens require a
future introspection slice. See [RFC 6749 section 4.4](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.4)
and the [RFC 9068 JWT access-token profile](https://www.rfc-editor.org/rfc/rfc9068.html).
Discovery/JWKS transport uses the same HTTPS-only, bounded, redirect-rejecting,
refresh-rate-limited verifier as GraphQL human OIDC.

OAuth settings are mutually exclusive with the static ingress principal and
secret. Bearer/HMAC modes remain compatibility paths: each deployment secret
still maps to one configured service principal rather than distinguishing
callers.

- In OAuth2 mode, allow only reviewed client IDs and rotate keys by publishing
  the new JWKS entry before issuing tokens that use it.
- In bearer/HMAC mode, bind each credential to one service principal and one
  server-owned integration.
- Prefer `FI_FHIR_HTTP_INGRESS_SECRET_FILE`; never reuse the GraphQL preview bearer.
- Keep the body limit at or below 1 MiB and terminate TLS at the approved proxy.
- Do not send browser `Origin` headers or compressed bodies; both fail closed.
- Treat `202` as durable admission, not downstream delivery completion.
- Retry `503`/`504` with the same idempotency key. A changed valid body under a
  committed key returns `409`.
- Leave the mode unset to roll back exposure without affecting GraphQL preview.

`make golden-path-001` reproduces the duplicate/restart/profile/IDE parity and
leakage gate. It writes disposable evidence under `.tmp/golden-path-001/`.

### Durable Kafka delivery

The production delivery worker is opt-in and shares the PostgreSQL submission
database. Keep it disabled until broker ACLs, TLS trust, topic policy, retry
bounds, and consumer duplicate suppression have been reviewed.

- Require TLS 1.3 when credentials are present and mount the password/CA as
  files. SASL without TLS fails startup.
- Grant the producer only write/describe access to the configured delivery topic.
- Treat the stable attempt ID as the consumer idempotency key. PostgreSQL and
  Kafka cannot commit atomically, so a publish-before-database-ack crash may
  repeat one record.
- Bound lease, publish timeout, retry delay/count, and circuit-open duration.
- Use individual PostgreSQL operator credentials for replay/resubmit so
  `current_user`, reason, and operation key form useful immutable audit evidence.
- Never repair delivery state with ad hoc SQL. Disable the worker to stop sends;
  preserve outbox/DLQ/audit rows for controlled recovery.

See [`DELIVERY-RELIABILITY.md`](DELIVERY-RELIABILITY.md) for exact configuration,
state transitions, inspection queries, and recovery commands.

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
