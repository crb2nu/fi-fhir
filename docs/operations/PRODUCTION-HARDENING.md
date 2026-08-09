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

### Batch workload identity

- Declare a `workload` block in the immutable source revision so each source
  submits under its own canonical service subject. One shared connector
  principal across every source collapses attribution and defeats per-source
  revocation.
- Give each subject the minimum submit authority. A subject provisioned for
  observation should carry no recognized submit grant; it then halts at the
  connector boundary before any lease, checkpoint, artifact load, or durable
  record exists.
- Set `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY=true` in production so the process
  refuses to start if the mounted source document ever drops its `workload`
  block. Identity binding is all-or-nothing per source; there is no per-object
  fallback to the deployment-fixed principal.
- Rotate a subject by publishing a new source revision, a new integration
  definition revision, and a lifecycle redeploy. The deployed release pins the
  exact source digest, so editing the mounted document alone fails closed.
- Alert on repeated ungranted-subject denials. Do not include subjects, object
  keys, or message bytes in alert labels.

### Batch receipt provenance

- Treat the remote object modification time as untrusted. SFTP exposes
  `SSH_FXP_SETSTAT`, so a producer can set any value it likes. The column is
  named `remote_modified_at_advisory` for that reason and must never be used for
  retention windows, ordering guarantees, audit timelines, or alerting
  thresholds.
- The authoritative received-at is the server-owned custody timestamp recorded
  when an exact object version is first durably admitted. It is stable across
  lease reclaim, worker restart, and checkpoint resume, so replays do not shift
  a receipt's clinical timeline.
- Content provenance is a SHA-256 digest computed over the exact bytes admitted,
  resumed across checkpoints and cross-checked against a full re-read before
  archive. A disagreement quarantines the object with `DIGEST_MISMATCH`; alert
  on that code, because it means an object was rewritten while its exact-version
  identity was preserved.
- Keep S3 bucket versioning enabled. Version ID plus entity tag is the
  exact-object identity re-verified before every read, archive, and delete.
- Rows admitted before this revision keep empty `object_version`/`object_etag`
  defaults; the provenance constraint is `NOT VALID` so historical rows are
  visibly distinguishable rather than retroactively given invented provenance.
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
    logLevel: info   # debug | info | warn | error
    logFormat: json  # json (default) or text; json is the only aggregator-parsable form
```

`tracingEnabled` is deliberately absent. The chart carried it until slice 4.4a
removed the key and 4.4d removed the last snippet that still showed it: it set
`FI_FHIR_TRACING_ENABLED`, which `pkg/config` parses and validates and which no
OpenTelemetry exporter consumes. See "Tracing" below.

`logLevel` and `logFormat` became load-bearing in slice 4.4d. Before that they
were parsed, validated, and read by nothing, and `serve` wrote unstructured
single-line text regardless of what they said.

### Log Fields for Compliance

Every line `fi-fhir serve` writes is a JSON object on stderr. The field keys are
a **closed set** — `internal/observability/logging.go` defines `LogField`, and
the handler drops any attribute whose key is not in it rather than writing it,
reporting the refusal as `dropped_fields`. This is the same posture the metric
labels take (`internal/observability/metrics.go` coerces an unrecognised outcome
to `error` rather than emitting it), and for the same reason: an unbounded key
space is how PHI reaches a log aggregator.

A representative line:

```json
{
  "time": "2026-08-09T10:30:00.123456-04:00",
  "level": "WARN",
  "msg": "delivery scheduled for retry",
  "tenant_id": "tenant-a",
  "component": "delivery-worker",
  "outcome": "retried",
  "duration_ms": 45
}
```

`time`, `level`, and `msg` come from `log/slog`. `tenant_id` is bound to the
logger at startup and is therefore on every line. Everything else is drawn from
the allowlist; `observability.PermittedLogFields()` enumerates it, and
`TestEveryPermittedLogFieldIsBounded` fails if the set stops being enumerable.

**Correlation, honestly.** There is no single correlation identifier to filter
on, by design: `pkg/integration/contracts.go` declares an eight-field lineage
bundle, and the durable schema follows it —
`integration_receipts`, `integration_canonical_events`, and
`integration_message_lineage` carry `correlation_id NOT NULL`, while
`integration_delivery_attempts` carries `trace_id NOT NULL` and joins back by
foreign key. So a log line and a submission are joined **through the durable
records**, not through a field the log line carries.

Two consequences an operator should know before writing a query:

- Lines emitted from the component-observation seam
  (`cmd/fi-fhir/serve_observability.go`) carry `tenant_id`, `component`,
  `outcome`, and `duration_ms` and **no** lineage identifier. The four
  observation callbacks receive `(Result, error)` and no context, and none of
  the `Result` types carries one. Emitting `correlation_id` from a component
  line requires widening those types, which is filed rather than done.
- `trace_id` and `span_id` appear only when a valid OpenTelemetry span context
  is on the request context. With tracing not exported, that is the GraphQL
  request path only. A zeroed trace ID is never emitted: a line that looks
  correlated and is not is worse than a line that admits it is not.

To join a component line to a message, filter by `tenant_id` and `component`,
take the window from `time`, and resolve the message through
`operatorMessageTrace` or the ledger tables directly.

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
that role is the deprecated compatibility grant and it expands to all 131
GraphQL root fields.

Since Sprint 4 the transport gate enumerates every root field and refuses any it
does not have a role for, instead of allowing everything to a `graphql:operator`
holder. Sixteen operator control-plane fields now have fine-grained
requirements, so a recovery or deployment operator can be issued a token that
reaches the control plane and nothing else:

| Token roles | Reaches |
|---|---|
| `integration.operator` | the nine `operator*` control-plane reads |
| `integration.operator` + `integration.delivery.operator` | the reads, plus `replayDelivery` / `resubmitMessage` / `discardDeadLetter` |
| `integration.operator` + `integration.deployment.operator` | the reads, plus pause / resume / retire / deploy |
| `graphql:operator` | everything, as before |

Each pair is an AND: the transport gate requires exactly the roles the service
behind it requires, so it can never be more permissive than
`operator.Service.authorize`. `integration.phi.export` is deliberately not a
transport-gate role — it gates the `includeRawPayload` argument of
`exportIntegrationBundle`, and a token holding only that grant reaches nothing.

The remaining 115 root fields — the event/patient browser, the legacy workflow
catalog, FHIR subscriptions, the session workspace, profiles, LLM, terminology,
and every subscription — are still reachable only through `graphql:operator`.
Do not replace an existing operator token's `graphql:operator` with the
fine-grained roles: it would keep the control plane and lose the entire IDE.
`serve` prints the mapping's shape at startup so a deployment relying on the
compatibility grant is visible in its own log.

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

### Destination-scoped delivery identity

The engine contacts no destination: the durable worker publishes one command per
attempt to the constant topic `integration.delivery.v1`, and an external consumer
performs the destination call. Slice 4.1c-a adds one fail-closed
`integration.deliver` decision on that dispatch path, evaluated after the outbox
row is claimed and before the broker is contacted.

- Leave `FI_FHIR_DELIVERY_IDENTITY_MODE` unset until a destination registry
  exists. Any other `FI_FHIR_DELIVERY_IDENTITY_*` setting without a mode refuses
  startup, so the decision cannot be half-applied.
- `strict` and `compatibility` reject each other's configuration. Prefer strict.
- Every declared secret binding is resolved once at startup and discarded; a
  credential that does not resolve refuses startup rather than failing at
  dispatch. Secret values enter no revision, record, log, metric, or broker field.
- `DELIVERY_FORBIDDEN` and `DELIVERY_DESTINATION_UNVERIFIED` are non-retryable
  dead letters. They mean the deployed revision and the planned attempt disagree;
  fix the revision and replay rather than editing attempt rows.

See [`DESTINATION-IDENTITY.md`](DESTINATION-IDENTITY.md) for the contract, the
registry document, mode semantics, secret resolution, and provenance columns.

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
# Database backup (PostgreSQL).
#
# --no-owner --no-privileges so the archive restores into an instance with a
# different role name, which is the normal recovery case.
#
# Use client tools whose MAJOR version matches the server (PostgreSQL 16, per
# docs/operations/SUPPORTED-1.0.md). This is load-bearing, not hygiene:
# pg_dump 17 and later write `SET transaction_timeout = 0` into the archive
# preamble, PostgreSQL 16 has no such setting and rejects it, and the failure
# appears only at restore time. A dump taken with newer client tools exits 0
# and is unrestorable into the very server it came from.
pg_dump --no-owner --no-privileges -h $DB_HOST -U $DB_USER -d fi_fhir | \
  gzip | \
  aws s3 cp - s3://backups/fi-fhir/$(date +%Y%m%dT%H%M%S).sql.gz

# Workflow configuration backup
kubectl get configmap fi-fhir -n fi-fhir -o yaml > workflow-config-backup.yaml

# Secrets backup (encrypted)
kubectl get secret fi-fhir -n fi-fhir -o yaml | \
  kubeseal --format yaml > sealed-secret-backup.yaml
```

`scripts/pgdump-roundtrip.sh` performs the dump and a restore into a scratch
database with exactly these options, and refuses on a client/server major
mismatch. `make migration-compatibility` runs it against a populated database
in CI and asserts the restored copy is complete — see "What the restore proof
covers" below.

### Recovery Procedures

1. **Database Recovery**:
   ```bash
   # -v ON_ERROR_STOP=1 is required. Without it psql prints errors, continues,
   # and exits 0 — so a restore that failed to recreate the audit-immutability
   # triggers looks like a success, and the recovered deployment silently has
   # weaker PHI governance than the one it replaced.
   aws s3 cp s3://backups/fi-fhir/latest.sql.gz - | \
     gunzip | \
     psql -v ON_ERROR_STOP=1 -h $DB_HOST -U $DB_USER -d fi_fhir
   ```

   After any restore, confirm the guarantees came back with the rows:

   ```bash
   # Every append-only ledger must still refuse mutation.
   psql -h $DB_HOST -U $DB_USER -d fi_fhir \
     -c "SELECT tgname FROM pg_trigger WHERE NOT tgisinternal ORDER BY tgname"

   # The 4.1c-a delivery-identity provenance CHECK must be present and NOT VALID.
   psql -h $DB_HOST -U $DB_USER -d fi_fhir \
     -c "SELECT conname, convalidated FROM pg_constraint
         WHERE conname = 'integration_delivery_identity_decisions_provenance_chk'"

   # Every migration ledger must be at the version the binary expects
   # (compare against `fi-fhir version`).
   psql -h $DB_HOST -U $DB_USER -d fi_fhir \
     -c "SELECT max(version) FROM integration_submission_schema_migrations"
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

### Recovery objectives, honestly

The product spec fixes **RPO ≤ 5 minutes and RTO ≤ 30 minutes**
(`.loom/20-product-spec-integration-engine-ide-completion.md:277-278`).

**The backup method documented above cannot meet that RPO, and no amount of
scheduling will make it.** A periodic `pg_dump` is a point-in-time logical
snapshot: everything written between the last successful dump and the failure
is gone. Running it every five minutes does not bound loss to five minutes — it
bounds loss to the interval *plus* the dump duration, on a database whose dump
time grows with the data, while holding a long transaction that keeps
`autovacuum` from cleaning up. Bounding data loss to minutes requires
continuous WAL archiving and point-in-time recovery, which **no chart,
manifest, or document in this repository configures today**.

### What this repository claims, and what it hands to the operator

Slice 4.4c decided the posture rather than restating the gap
(`.loom/40-decisions.md`, 2026-08-09, "WAL/PITR posture"). It splits budget 5
along the line where the evidence actually falls.

**RTO is measured and certified here.** `test:migration-compatibility` times the
documented procedure end to end on every merge request — `pg_dump`, restore, and
the first successful delivery `Claim` from the restored database — and archives
the number as the `recovery-rto.json` job artifact. The report carries the row
counts it was measured against, because a recovery time measured on a CI fixture
is evidence about the *procedure* and not about a production data volume.

**RPO is an operator responsibility, with a stated method.** This product does
not claim a 5-minute RPO. Bounding data loss to minutes requires continuous WAL
archiving and point-in-time recovery, and that belongs to whoever runs the
database: the only PostgreSQL in `deploy/` is a single-replica `Deployment` on a
ReadWriteOnce PVC, which is a development convenience, and a production
deployment uses a managed service or a PostgreSQL operator that owns archiving
through its own interface. Shipping a reference `archive_command` written
against the dev manifest would be a configuration almost nobody runs, carrying
the authority of a product guarantee. A reference archiving configuration proved
end to end in CI is filed as a follow-up, with its cost written down, in the
decision above.

**The RPO an operator achieves is a function of the method they choose:**

| Method | Achievable RPO | Who configures it |
|--------|---------------|-------------------|
| Periodic `pg_dump` (documented above) | The dump interval plus the dump duration. Minutes is not reachable. | This repository documents it; the operator schedules it |
| Continuous WAL archiving + PITR | Seconds to low minutes | The operator, through their managed service or PostgreSQL operator |
| Synchronous replication to a standby | Near zero for a single-site failure | The operator |

| Scenario | RTO | RPO | Status |
|----------|-----|-----|--------|
| Pod failure | 30s target | 0 | Kubernetes restart; not measured on the reference profile |
| Node failure | 5m target | 0 | Depends on cluster capacity; not measured |
| Database failure | **Measured** against the documented restore; see the archived `recovery-rto.json` | Operator-owned; see the method table above | Restore faithfulness and resumption proven on every merge request |
| Full cluster failure | 1h target | Operator-owned | Not measured; needs a cluster |

Treat the rows marked *target* as targets, not capabilities.
`docs/operations/SUPPORTED-1.0.md` item 4 is split accordingly: the
backup/restore proof and the RTO measurement close, and the RPO number stays
open as an operator responsibility rather than as an empty checkbox with no
stated reason.

### What the restore proof covers

`TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`
(`internal/integration/migrationcompat`, CI job `test:migration-compatibility`)
runs the documented dump and restore against a populated PostgreSQL 16 database
on every merge request and asserts:

- every row in the durable classes survives — receipts, canonical events,
  lineage, delivery attempts, the outbox, sessions, exports, and delivery
  identity decisions;
- the canonical event payload survives intact, so the comparison is about
  content and not row counts;
- every Slice 4.1d C1 immutability guard still raises on the restored copy —
  a dump that silently drops a trigger is a PHI-governance regression, not a
  backup;
- the 4.1c-a provenance CHECK is present and still `NOT VALID`, so a restore
  cannot quietly convert a forward-only guarantee into a retroactive claim;
- a queued delivery attempt is claimed and published from the restored state
  with no manual repair, and is not claimed twice.

Slice 4.4c strengthened three of those and added two:

- every immutability refusal is now asserted **by SQLSTATE**. A trigger refusal
  is `P0001`; before 4.4c the proof asserted only that *an* error came back, and
  three of its six mutations were in fact refused by a foreign key — so those
  three would have stayed green with their guards dropped. They now target rows
  with no dependents, and
  `TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed` drops every
  non-internal trigger on the restored copy and requires every guarded mutation
  to then succeed, which is the control that keeps the attribution honest;
- the durable set now includes the whole Slice 4.1e surface — session samples,
  the session fanout log, and all three retention tables. Their absence was not
  a row-count gap: an empty table has no rows to mutate, so five of the newest
  immutability triggers were never exercised after a restore at all;
- the **restored** database's six schema ledgers are asserted at their declared
  versions. A restore that lost them used to pass every other assertion;
- the recovery time is measured and archived (above).

**What it still does not cover**: WAL archiving, point-in-time recovery,
failover, or a restore onto a different host. The proof establishes that a
logical backup is *complete and faithful*, that the application resumes from it,
and how long the documented procedure takes. It says nothing about how much data
a real failure would lose — that is the operator-owned RPO above.

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
