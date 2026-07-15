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
timeouts, TLS binding names, client CIDRs, acknowledgement mode, and bounds.
Secret values never appear in the source document.

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

## Configuration

The GraphQL preview variables and immutable profile/workflow registry remain
required by `serve`. Add:

```bash
export FI_FHIR_MLLP_SOURCE_CONFIG_PATH=/etc/fi-fhir/mllp/source-revision.json
export FI_FHIR_MLLP_DEFINITION_ID=integration-mllp
export FI_FHIR_MLLP_PRINCIPAL_ID=mllp-listener
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

The required `test:mllp-runtime` job first discovers the exact test name, then
runs the real TCP, transaction block, concurrent pause, 32-client duplicate,
resume, retirement, restart, cardinality, and leakage proof with the race
detector.

## Rollback

Unset `FI_FHIR_MLLP_SOURCE_CONFIG_PATH` and restart `serve`. The listener is then
absent while GraphQL preview and optional authenticated HTTP ingress retain
their existing behavior. Preserve lifecycle and submission records for audit.
