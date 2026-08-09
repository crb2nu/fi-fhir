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

## Capacity is enforced per replica, not per deployment

`CapacityPolicy` (`MaxInFlight`, `MaxQueued`, `MaxMessagesPerSecond`) is
declared once on the integration revision, but the listener enforces it with
one in-process gate per `Service`
(`internal/integration/mllp/service.go:46-55`,
`pkg/integration/deployment.go:61-73`). That gate has no cross-process state:
it bounds only the connections and messages this one replica handles.

Running `N` replicas of the same deployment admits up to `N × MaxInFlight`
concurrently and up to `N × MaxMessagesPerSecond` in aggregate. The revision
itself still declares only a single policy value. A revision with
`max_messages_per_second: 100` and 4 replicas can accept 400
messages/second in total.

This is documented behavior, not a pending bug fix. A durable,
per-deployment token bucket that enforces the policy across replicas is
future work (Slice 4.4+), not shipped today. Until it lands, an operator has
two choices:

- **Divide the declared policy by the replica count** — set
  `max_messages_per_second`, `max_in_flight`, and `max_queued` on the
  revision to the deployment-wide target divided by the replica count, so
  the aggregate across replicas matches the intended ceiling.
- **Accept the multiple deliberately** — size the declared policy assuming
  it will be multiplied by the replica count, and document that assumption
  in the deployment's own change record.

Either way, changing the replica count without revisiting the capacity
policy silently changes the deployment's effective ceiling.

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
