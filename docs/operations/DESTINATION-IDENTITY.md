# Destination-Scoped Delivery Identity

Slice 4.1c-a gives a delivery destination an immutable, content-addressed
revision and adds one fail-closed `integration.deliver` authorization decision to
the durable dispatch path.

## What the engine does and does not do

**The engine does not contact destinations.** `Dispatcher.RunOnce` claims one
outbox row and publishes one command to the single constant Kafka topic
`integration.delivery.v1`. An external consumer of that topic performs the actual
destination call. `webhook`, `fhir`, `database`, and `file` are plan-level action
*classes* validated in `internal/integration/processor/workflow_plan.go`; none is
an executed transport on the durable path.

This is asserted, not assumed. `TestDeliveryDispatch_ContactsNoDestination`
(`internal/integration/delivery/destination_contact_integration_test.go`) stands
a live TLS endpoint at the address a webhook destination would be reached on,
runs a full production submission through durable admission, and asserts zero
accepted connections and zero served requests against exactly one Kafka record.
It dials the endpoint itself at the end, so a zero cannot come from a broken
listener.

Before this slice a destination was only
`integration.DestinationRevisionRef{artifact_id, revision_id, digest, class}`
(`pkg/integration/revision.go`), with no resolvable bytes behind the digest, no
transport, and no credential binding. The digest named an artifact that did not
exist. This slice supplies that artifact and the decision that verifies it.

The first durable HTTPS destination consumer is **4.1c-b**, a separate slice.

## Contract

A destination revision (`internal/integration/destination/revision.go`) carries:

| Field | Meaning |
|---|---|
| `schema_version` | Pinned wire contract, currently `"1"` |
| `artifact_id`, `revision_id` | Artifact identity; the attempt reference must match exactly |
| `destination_id` | Runtime destination identity, the analogue of `source_id` |
| `class` | `production` or `sandbox` |
| `transport` | `kafka` or `https` |
| `kafka` / `https` | Non-secret transport policy. HTTPS carries a URL, method, and **binding names** |
| `identity` | Optional client subject and its grants |
| `digest` | SHA-256 over every semantic field above, domain-separated |

Secret **values** never appear. Only binding names travel, resolved out of band —
the same discipline `mllp.SourceRevision` and `batch.SourceRevision` apply.

Any semantic mutation invalidates the digest, so a mutated revision fails
`Validate()` and never resolves.

## The registry

`FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` points at a server-owned document. It
is loaded like the static integration registry and the lifecycle release: never
authored over GraphQL, never sender-supplied.

```json
{
  "schema": "fi-fhir/destination-registry/v1",
  "tenant_id": "tenant-a",
  "integration_revision": {
    "artifact_id": "integration-adt",
    "revision_id": "revision-1",
    "digest": "sha256:…"
  },
  "secret_bindings": [
    {"name": "alpha-token", "reference": {"provider": "file", "key": "alpha/token"}}
  ],
  "destinations": [
    {
      "schema_version": "1",
      "artifact_id": "dest-alpha",
      "revision_id": "destination-1",
      "destination_id": "alpha",
      "class": "production",
      "transport": "https",
      "https": {
        "url": "https://alpha.example/fhir",
        "method": "POST",
        "token_binding": "alpha-token"
      },
      "identity": {
        "subject": "alpha-client",
        "grants": ["integration.destination.client"]
      },
      "digest": "sha256:…"
    }
  ]
}
```

Every binding a destination names must be present in `secret_bindings`, and the
document is rejected at load otherwise.

## Modes

| Setting | Behavior |
|---|---|
| `FI_FHIR_DELIVERY_IDENTITY_MODE` unset | No decision on the dispatch path. Behavior is identical to Slice 2.3. Any other `FI_FHIR_DELIVERY_IDENTITY_*` setting without a mode **refuses startup**. |
| `strict` | Every destination must declare an `identity`. A registry containing an unbound destination fails to load. A compatibility subject is **refused**. |
| `compatibility` | Unbound destinations are authorized under one explicit, server-issued `integration.destination.compatibility` grant. `FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT` is **required**. |

The two modes reject each other's configuration, exactly as the OIDC and static
identity modes do. There is no implicit fallback and no per-destination override.

`serve` prints the active mode after the delivery worker line at startup.

## Secret resolution

`integration.SecretResolver` (`pkg/integration/secret.go`) is the interface;
`cmd/fi-fhir/destination_identity_runtime.go` is the file/env implementation. It
lives in `cmd/` on purpose, so no package under `internal/integration/*` can hold
resolved material in a struct that is later marshaled into a record.

- `provider: "env"` reads an uppercase environment variable.
- `provider: "file"` reads a path under `FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR`,
  rejecting absolute keys, `..` segments, and anything escaping the directory.
- Every read is bounded by `integration.MaxSecretBytes` (64 KiB).
- A pinned `version` fails closed: neither provider stores versions, so silently
  returning whatever is on disk would be a lie.
- Every failure collapses to `ErrSecretUnresolvable`, so failures cannot be used
  to enumerate the secret inventory.
- Providers other than file and env are not implemented in this slice and fail
  closed.

**Startup resolves every declared binding once and discards the material.** A
deployment whose destination names a credential that does not resolve refuses to
start, instead of discovering the gap on the first dispatch.

## The decision

`authorization.AuthorizeDelivery` (`internal/integration/authorization/policy.go`)
evaluates `integration.deliver` over `(tenant, integration revision, destination
revision)`.

It runs in `Dispatcher.RunOnce` **after `Claim` and before the delivery command
is built or published**, so an unauthorized attempt never reaches the broker.

The identity is a function of the destination revision alone. Nothing the work
item asserts can select a subject: the registry resolves by artifact ID and then
requires the deployed revision's own reference to equal the attempt's reference
exactly — revision ID, digest, and class included.

A delivery principal must carry **no** `source_id`. That is the isolation
boundary between the two actions: a source principal, which always names its
source, can never be replayed as a destination client, and a destination client
can never be replayed as a source.

### Refusals

| Code | Cause |
|---|---|
| `DELIVERY_FORBIDDEN` | Destination absent from the deployed set, unbound under strict mode, or a principal without a deliver grant |
| `DELIVERY_DESTINATION_UNVERIFIED` | The attempt reference does not match the deployed destination revision byte for byte |

Both are **non-retryable**: the destination set and the identity binding are
properties of the deployed revision, so the same attempt would be refused
identically on every poll. They enter the existing DLQ through `MarkFailed` with
`attempt_count` unchanged, and are visible to the operator control plane
alongside every other terminal failure.

An **infrastructure** failure in the decision path (for example a provenance
write failure) is surfaced as an ordinary error and retried. It never becomes a
dead letter.

## Grant naming

`integration.deliver` is the action; `integration.destination.client` is the
grant. Both follow the dotted fine-grained convention already used by
`integration.delivery.operator` (Slice 2.3) and `integration.operator` /
`integration.deployment.operator` (Slice 4.2a), rather than the colon-form
compatibility grants (`integration:submit`, `integration:mllp`,
`integration:batch`) that predate it.

## Provenance

Every decision, authorized or refused, is appended to
`integration_delivery_identity_decisions`
(`internal/integration/destination/migrations/0001_delivery_identity.sql`).

Trusted, server-owned columns: `decision`, `identity_mode`, `principal_subject`,
`principal_auth_method`, `granted_role`, `destination_digest_verified`,
`denial_code`, `decided_at`.

Advisory, never a trust input: `destination_endpoint_advisory` — the remote
address the destination revision declares, carried for operator diagnostics only.
Its `COMMENT ON COLUMN` says so.

Dispatches from before this slice have **no row at all**, which is how they stay
visibly distinguishable: absence of a decision row means the decision was never
made, never that it was made and allowed. The provenance CHECK is added
`NOT VALID` so a later backfill cannot silently claim the constraint governed
rows it never saw.

The package owns its own numbered migration set and its own version ledger
(`integration_destination_schema_migrations`), following the per-package
`go:embed` idiom used by `processor`, `lifecycle`, `batch`, and `session`.

## Verification

```bash
export POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=...
make delivery-identity        # both 4.1c-a proofs
make delivery-reliability     # Slice 2.3's proof must still pass
```

CI runs both as the blocking job `test:delivery-identity`, whose first step
asserts **both** test names exist so a renamed or deleted proof turns the job red
rather than green.

## Operator checklist

- Leave `FI_FHIR_DELIVERY_IDENTITY_MODE` unset until a destination registry
  exists. A half-applied configuration refuses startup rather than running with
  the decision silently disabled.
- Prefer `strict`. Use `compatibility` only while migrating a deployment whose
  destination revisions do not yet declare identities, and record why.
- Mount secret material as files under a dedicated directory; do not pass
  credentials through the environment in production.
- A `DELIVERY_FORBIDDEN` or `DELIVERY_DESTINATION_UNVERIFIED` dead letter means
  the deployed revision and the planned attempt disagree. Fix the deployed
  revision and replay; never repair the attempt row with ad hoc SQL.
- The engine still contacts no destination. Until 4.1c-b ships, the identity
  authorizes the Kafka command that an external consumer acts on.
