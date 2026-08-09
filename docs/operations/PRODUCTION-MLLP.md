# Production MLLP

## Runtime contract

Slice 2.2 adds an optional HL7v2 Minimal Lower Layer Protocol listener to
`fi-fhir serve`. The listener is disabled unless
`FI_FHIR_MLLP_SOURCE_CONFIG_PATH` is set. Enabling it does not activate a
Kubernetes Service or production sender route; GitOps exposure remains a
separate reviewed operation.

Each connection uses a strict, content-addressed UTF-8 source revision. Every
frame resolves the configured definition through the PostgreSQL lifecycle
catalog and runs only its exact deployed release. Paused, retired, missing, or
source-mismatched releases receive a retryable negative acknowledgement.

A positive application ACK (`AA`) or commit ACK (`CA`) is written only after
the receipt, canonical event, lineage, initial attempt, and outbox record commit
atomically. It means durable admission, not successful downstream delivery.

## Source revision

Use [the checked-in mutual-TLS example](../../testdata/golden/integration/adt-mllp/source-revision.json)
as the schema reference. The digest covers listener identity, address, framing,
timeouts, TLS binding names, client CIDRs, the optional client identity map,
acknowledgement mode, and bounds. Secret values never appear in the source
document.

The v1 adapter supports:

- UTF-8 only;
- configurable distinct single-byte start, end, and trailer framing, defaulting
  to `VT (11)`, `FS (28)`, and `CR (13)`;
- application `AA/AE/AR` or commit `CA/CE/CR` response codes;
- TLS 1.3 mutual authentication and exact canonical CIDR allowlists;
- bounded message bytes, connections, in-flight work, queued work, and rate;
- fragmented frames and multiple sequential frames per connection.

Plaintext mode is intended only for an independently protected loopback or
sidecar trust boundary. Do not expose plaintext MLLP through a node port,
ingress, load balancer, or untrusted network.

## Capacity: the message rate is deployment-wide, the rest is per replica

`CapacityPolicy` is declared once on the integration revision
(`pkg/integration/deployment.go`) and carries three numbers. Since slice 4.4e
they are not all enforced at the same scope, and the difference matters when
you size a deployment.

| Setting | Scope | Why |
|---|---|---|
| `max_messages_per_second` | **the deployment** | A throughput commitment to the sending facility. It means the same thing at one replica or six. |
| `max_in_flight` | per replica | A bound on concurrent work inside one process. |
| `max_queued` | per replica | A bound on one process's admission queue depth. |
| `max_connections` | per replica | From the mounted source JSON, not the revision. A per-listener socket bound. |

`max_in_flight` and `max_queued` are deliberately per-replica: they bound a
process's memory and concurrency, and dividing them across replicas would
describe nothing real. Running `N` replicas therefore gives the deployment
`N × max_in_flight` concurrent work, and that is intended.

### How the deployment-wide rate is enforced

Each replica leases a **share** of the declared rate from a durable
per-deployment record (`integration_mllp_rate_claims`, lifecycle migration
0002), refills its in-memory token bucket from that share, and releases the
share when it shuts down. The live shares sum to exactly the declared rate, so
the aggregate is bounded by construction rather than by convention.

**Admission itself never touches the database.** The claim loop runs on an
interval measured in seconds; the per-frame decision is the same in-memory
token take it always was. At 250 msg/s that is roughly one query per replica
per 500 frames. A per-frame durable counter would have been exact and would
have turned the rate limiter into a throughput ceiling; it was rejected for
that reason. See `.loom/40-decisions.md` (2026-08-09).

| Parameter | Value | Meaning |
|---|---|---|
| Claim interval | 2s | How often a replica renews its share and picks up a new split. |
| Lease TTL | 6s | How long a share survives without renewal. Three intervals of headroom, so two lost round trips are a non-event. |
| Degraded share | `max(1, declared ÷ 10)` | What a replica falls back to when it cannot reach PostgreSQL. |

Neither parameter is environment-configurable. Capacity lives on the deployed
revision, and this is its distribution mechanism rather than a separate knob.

### What an operator should expect

- **Scaling up or down needs no config change.** Add a replica and the split
  re-computes within one claim interval. This is the behaviour that replaces
  the old advice to divide `max_messages_per_second` by the replica count —
  **do not do that any more**; it now under-provisions the deployment by the
  replica count.
- **Uneven load is bursty at the margin.** An idle replica holds its share for
  up to one claim interval after a busy one could have used it. That is the
  price of keeping admission in memory, and it is the tuning knob if the
  trade-off ever needs revisiting.
- **A rolling redeploy does not burst.** The quota pool is keyed on the
  deployment, not on the revision digest, so the old and new revisions draw
  from one budget while both are live. The token bucket is also no longer
  refilled when the deployed revision changes — before 4.4e it was, which
  handed every new replica a fresh full burst.
- **A PostgreSQL outage degrades, it does not stop.** A replica keeps its
  current share until the lease expires, then drops to the conservative share
  and reports `fi_fhir_mllp_rate_claims_total{outcome="degraded"}`. It never
  falls back to the full declared rate, and never to zero. The residual: if a
  deployment runs more than ten replicas *and* every one of them loses
  PostgreSQL at once, the aggregate can exceed the declared rate. It is bounded
  and it is strictly better than the pre-4.4e behaviour, which was unbounded.
- **More replicas than messages per second is a misconfiguration.** Every
  holder is granted at least 1 msg/s, so a deployment declaring 4 msg/s across
  6 replicas admits up to 6. Bounding it further would black-hole replicas.

### Observability

- `fi_fhir_mllp_rate_claims_total{outcome}` — one increment per claim attempt
  per replica. `processed` is a healthy renewal, `degraded` is a replica on the
  fallback share, `error` is a failed attempt inside a still-valid lease. If
  this ever rises in step with `fi_fhir_mllp_messages_total`, admission has
  started taking a round trip per frame and the design has regressed.
- `fi_fhir_component_up{component="mllp_rate_quota"}` — the claim loop is a
  first-class background component of `serve`.

### Rate-limited frames

A frame refused by the rate gate gets a **transient** negative acknowledgement —
`AE` in application mode, `CE` in commit mode — carrying `RATE_EXCEEDED` in the
ERR segment, and **the connection stays open**. `AR`/`CR` are reserved for
permanent rejects; a throttled sender is expected to retry. This is unchanged
by 4.4e, and it is asserted end to end for the first time by
`TestMLLPCapacity_RateLimitedFrameGetsATransientNAKAndTheConnectionSurvives`.

## Client certificate service identity

Slice 4.1b2 adds an optional `clients.identities` allowlist that maps one
verified client certificate to one canonical service subject and its grants.

```json
"clients": {
  "allowed_cidrs": ["10.0.0.0/8"],
  "identities": [
    {
      "subject": "svc-adt-sender-east",
      "uri_san": "spiffe://hospital-a/mllp/sender-east",
      "spki_sha256": "sha256:<64 lowercase hex characters>",
      "grants": ["integration:mllp"]
    }
  ]
}
```

Rules the adapter enforces:

- Identity mapping requires `tls.mode = mutual`. A plaintext listener that
  declares identities is an invalid source revision.
- Each entry needs a canonical `subject` and at least one authority-scoped
  criterion: `uri_san`, `spki_sha256`, or both. When both are present, both must
  match. Common names are never accepted as identity.
- Subjects, URI SANs, and SPKI pins must each be unique across the map.
- Identity resolution runs immediately after the TLS handshake and before any
  frame is read. A CA-valid certificate that matches zero entries, or more than
  one entry, is closed without an acknowledgement.
- The resolved subject and grants become the per-connection service principal.
  The `integration.submit` decision then runs over the exact tenant, integration
  revision, and registry-owned source before capacity, envelope construction,
  processor artifact loading, and transaction-scoped durable admission. An entry
  whose `grants` omit a recognized submit grant (`integration:submit`,
  `integration:mllp`, or `integration:batch`) authenticates but never admits.
- Source identity is always bound from the deployed release. Certificates and
  MSH fields cannot assert tenant, source, or integration.

Mapping is all-or-nothing per listener. Omitting `identities` selects
compatibility mode, where every verified connection submits under the
`FI_FHIR_MLLP_PRINCIPAL_ID` principal with the server-issued `integration:mllp`
grant. There is no per-connection fallback between the two modes.

Because the identity map is part of the content-addressed source revision,
adding or removing a sender requires a new source revision, a new integration
definition revision, and a lifecycle redeploy. Set
`FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY=true` so a deployment refuses to start if
the mounted source document ever drops back to compatibility mode.

Rotating a sender key while pinning `spki_sha256` requires publishing the new
pin as a new revision before the sender switches keys; prefer a `uri_san`-only
entry when the issuing authority already scopes the URI.

## Configuration

The GraphQL preview variables and immutable profile/workflow registry remain
required by `serve`. Add:

```bash
export FI_FHIR_MLLP_SOURCE_CONFIG_PATH=/etc/fi-fhir/mllp/source-revision.json
export FI_FHIR_MLLP_DEFINITION_ID=integration-mllp
export FI_FHIR_MLLP_PRINCIPAL_ID=mllp-listener
export FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY=true
export FI_FHIR_MLLP_TLS_CERT_FILE=/var/run/secrets/mllp/tls.crt
export FI_FHIR_MLLP_TLS_KEY_FILE=/var/run/secrets/mllp/tls.key
export FI_FHIR_MLLP_TLS_CLIENT_CA_FILE=/var/run/secrets/mllp/client-ca.crt
```

The PostgreSQL lifecycle catalog must already contain the selected immutable
definition in `deployed` state. Its source reference, source ID, HL7v2 format,
deployment policy, and TLS secret-binding names must exactly match the source
revision. The registry supplies only the digest-verified profile and workflow
bytes; it does not select which definition MLLP executes.

## Failure behavior

| Condition | Response |
|---|---|
| Durable accepted result or exact duplicate | `AA` or `CA` |
| Paused/retired/unavailable release | `AE` or `CE` |
| Capacity, rate, timeout, or storage failure | `AE` or `CE` |
| Invalid supported HL7v2 message or idempotency conflict | `AR` or `CR` |
| Unmapped or ambiguous client certificate identity | Close without ACK |
| Mapped identity without a recognized submit grant | `AE` or `CE` |
| Invalid framing/header, oversize frame, disallowed CIDR, failed TLS | Close without ACK |

ACK construction swaps the validated sending/receiving application/facility
fields and echoes the validated MSH-10 control ID. It never reflects the
clinical message body or unvalidated header fields.

## Verification

Run unit and race coverage:

```bash
go test -race -count=1 ./internal/integration/mllp
```

Run the PostgreSQL 16/TCP proof:

```bash
POSTGRES_TEST_URL='postgres://user:pass@host:5432/db?sslmode=disable' \
  make mllp-runtime
```

The required `test:mllp-runtime` job first discovers both exact test names, then
runs them with the race detector:

- `TestPostgresMLLPRuntime_DurableACKPauseRestart` covers real TCP, the
  transaction block, a concurrent pause, 32-client duplicates, resume,
  retirement, restart, cardinality, and leakage.
- `TestPostgresMLLPRuntime_CertificateIdentityAuthorization` covers real TLS 1.3
  mutual authentication: two mapped certificates stay two distinct verified
  subjects despite spoofed MSH provenance, an unmapped CA-valid certificate
  reaches neither artifact loading nor durability, an ungranted mapped identity
  is denied for the exact tenant/revision/source, and compatibility mode is
  unchanged.

## Rollback

Unset `FI_FHIR_MLLP_SOURCE_CONFIG_PATH` and restart `serve`. The listener is then
absent while GraphQL preview and optional authenticated HTTP ingress retain
their existing behavior. Preserve lifecycle and submission records for audit.

To roll back only the identity mapping, redeploy the previous source revision
and definition revision through the lifecycle and unset
`FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY`. Editing the mounted source document
alone is not a rollback: the deployed release pins the exact digest, so a
mismatched document produces a retryable negative acknowledgement.
