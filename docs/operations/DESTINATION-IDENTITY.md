# Destination-Scoped Delivery Identity

Slice 4.1c-a gives a delivery destination an immutable, content-addressed
revision and adds one fail-closed `integration.deliver` authorization decision to
the durable dispatch path. Slice 4.1c-b makes the `https` transport on that
revision execute.

## What the engine does and does not do

**The engine contacts `https`-transport destinations, and only those.** The
destination revision's `transport` field is the switch:

| `transport` | What `Dispatcher.RunOnce` does |
|---|---|
| `kafka` | Publishes one command to the single constant Kafka topic `integration.delivery.v1`. An external consumer of that topic performs the destination call. The engine contacts nothing. |
| `https` | Contacts the destination itself over TLS, once per claimed attempt, under the identity the destination revision declares, and completes the lease through the same `MarkPublished`/`MarkFailed` the broker path uses. |

`webhook`, `fhir`, `database`, and `file` remain plan-level action *classes*
validated in `internal/integration/processor/workflow_plan.go`. None of them is
a transport, and a workflow cannot name a URL: the published DSL restricts
destination names to `^[a-z][a-z0-9_.-]*$`. **The transport is a property of the
server-owned destination revision, never of the workflow.**

Both halves are asserted, not assumed.

- `TestDeliveryDispatch_ContactsNoDestination`
  (`internal/integration/delivery/destination_contact_integration_test.go`)
  stands a live TLS endpoint at the address a webhook destination would be
  reached on, runs a full production submission through durable admission, and
  asserts zero accepted connections and zero served requests against exactly one
  Kafka record. Since 4.1c-b it proves this of a **`kafka`-class** destination.
- `TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity`
  (`internal/integration/delivery/destination_transport_scenario_test.go`)
  proves the other half over six destinations at once, and runs its own negative
  control against a router that owns nothing.

Both dial their endpoints from the test itself, so a zero cannot come from a
broken listener.

Before 4.1c-a a destination was only
`integration.DestinationRevisionRef{artifact_id, revision_id, digest, class}`
(`pkg/integration/revision.go`), with no resolvable bytes behind the digest, no
transport, and no credential binding. The digest named an artifact that did not
exist. 4.1c-a supplied that artifact and the decision that verifies it; 4.1c-b
supplies the consumer.

> **Upgrade note.** A deployment that already declares `transport: https` on a
> destination in its registry **starts contacting that destination** on upgrade
> to 4.1c-b. Before it, the field was inert. The registry is server-owned, so
> declaring `https` is an explicit operator act — but check the registry before
> upgrading.

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

### The rotation contract

Since 4.1c-b the resolver also runs on the dispatch path, for `https`
destinations only:

- The token binding is resolved **once per dispatch**, used to build exactly one
  request, and the buffer is zeroed before the call returns.
- There is **no cache**. File and environment references cannot be
  version-pinned (a pinned `version` fails closed), so a rotation is a write in
  place with nothing to invalidate. A cache would silently pin a rotated-out
  credential. The cost is one read per delivery; the benefit is that rotation
  takes effect on the next dispatch with no restart and no signal.
- The same applies to `ca_bundle_binding`. The TLS client is rebuilt per dispatch
  for the same reason, so a rotated trust root takes effect immediately.
- Resolved material never enters a `Decision`, a `DeliveryRecord`, a log line, a
  metric label, a `Failure.Detail`, or any struct that is JSON-marshaled. The
  kill-test plants a sentinel in one destination's credential and proves it
  appears in none of nine durable record classes, no broker field, and no
  captured process output.
- A credential that does not resolve at dispatch time produces a **retryable**
  `DELIVERY_DESTINATION_CREDENTIAL_UNRESOLVED` failure and **no request**. The
  engine never contacts a destination without the credential it declared.

The credential is sent as `Authorization: Bearer <material>`, with surrounding
whitespace trimmed (file-backed credentials routinely end with a newline). The
material must be printable ASCII with no interior whitespace, so it cannot
smuggle a second header or a header terminator into the request; anything else
fails closed as a terminal `DELIVERY_DESTINATION_UNCONFIGURED`.

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

## The HTTPS transport (4.1c-b)

The transport is substituted at the `Publisher` seam inside `Dispatcher.RunOnce`,
so it inherits the whole durable state machine rather than duplicating it: the
lease, the bounded retry and backoff, `MaxAttempts`, the DLQ, replay, resubmit,
discard, and the **per-destination-artifact circuit breaker**. `MarkPublished`
now means "handed off successfully" for two transports. See `.loom/40-decisions.md`
(2026-08-09) for the rejected alternatives, chiefly an in-process consumer of
`integration.delivery.v1`.

**What is sent.** The bytes are the same `integration.delivery.v1` command the
broker would have carried — raw-free and address-free by the same construction —
with `Content-Type: application/json`, the `Authorization` header above, and a
server-owned `Idempotency-Key` set to the durable attempt ID. The outbox is
at-least-once by design, so the idempotency key is what lets a destination absorb
a redelivery.

**What is closed.**

- `CheckRedirect` returns an error. **A redirect is a refusal, never a follow** —
  the target is chosen by the destination and is therefore not a trust input.
- `MinVersion: tls.VersionTLS12`. Trust roots come from `ca_bundle_binding` when
  declared and the system pool otherwise. **`InsecureSkipVerify` appears nowhere.**
- No proxy. A proxy read from the process environment would be a trust input the
  destination revision never declared.
- Response **headers are read for nothing**. The body is drained up to 64 KiB and
  discarded unparsed. The only property of the response that leaves the client is
  its status class.
- The call is bounded by the existing `FI_FHIR_DELIVERY_PUBLISH_TIMEOUT`, which
  `Config.validate` already requires to be shorter than the lease. A slow
  destination therefore cannot outlive its lease and be delivered a second time
  by the worker that reclaims it. **There is deliberately no second timeout knob.**

**Outcome mapping.**

| Response | Failure code | Retryable | Durable effect |
|---|---|---|---|
| `2xx` | — | — | `MarkPublished`; the destination circuit closes |
| `408`, `429`, any `5xx` | `DELIVERY_DESTINATION_UNAVAILABLE` | yes | Bounded retry; circuit failure counted |
| Any other `4xx`, or a `3xx` with no `Location` | `DELIVERY_DESTINATION_REJECTED` | no | Dead letter, `attempt_count` unchanged |
| A redirect | `DELIVERY_DESTINATION_REDIRECT_REFUSED` | no | Dead letter; the target is never dialed |
| Dial, TLS handshake, or timeout failure | `DELIVERY_DESTINATION_UNREACHABLE` | yes | Bounded retry; circuit failure counted |
| Credential or trust bundle unresolvable | `DELIVERY_DESTINATION_CREDENTIAL_UNRESOLVED` | yes | Bounded retry; **no request is made** |
| Destination unresolvable or unusable as declared | `DELIVERY_DESTINATION_UNCONFIGURED` | no | Dead letter; never falls through to the broker |

Every code and detail obeys the same bounds a refusal does — a catalog-safe code
of at most 128 bytes and a detail of at most 512 — and none contains
destination-supplied content. That bound is the reason a destination cannot write
its response body into a durable record by way of a failure detail.

### Kafka is still required

The delivery worker requires `FI_FHIR_QUEUE_DRIVER=kafka` and
`FI_FHIR_QUEUE_BROKERS` **even when every destination in the loaded registry
declares `transport: https`**. An HTTPS-only deployment stands up a broker it
never produces to.

This is deliberate. The registry is one server-owned file read at boot, so "all
destinations are https" is a property of one startup rather than of the
deployment; relaxing the requirement would turn adding one `kafka`-class
destination from a startup configuration error into a runtime dead letter.
Recorded in `.loom/40-decisions.md` (2026-08-09) with a named follow-up
("broker-free delivery worker").

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

### Delivery provenance (4.1c-b)

Every delivery this process performs itself is appended to
`integration_destination_deliveries`
(`internal/integration/destination/migrations/0002_https_delivery_provenance.sql`).

The two ledgers answer different questions. `0001` records the **decision** —
whether a dispatch was authorized to reach a destination. `0002` records the
**act** — that the process actually contacted one, under which verified
revision, and how the exchange ended. An authorized decision with no delivery row
means the attempt was authorized and then published to the broker, which is
exactly what every `kafka`-class destination does.

Trusted, server-owned columns: `transport`, `destination_artifact_id`,
`destination_revision_id`, `destination_class`, `destination_digest_verified`,
`outcome`, `failure_code`, `completed_at`, and `http_status_class`.

`http_status_class` is deliberately **not** advisory: it is not the destination's
status line, it is this process's own reduction of the response to the closed
vocabulary `1xx|2xx|3xx|4xx|5xx`, and it is the only property of the response
that is read at all.

Advisory, never a trust input, each with a `COMMENT ON COLUMN` saying so:

- `destination_endpoint_advisory` — the remote address the revision declares.
- `served_certificate_subject_advisory` — the subject of the certificate the
  destination served, bounded to 256 bytes and stripped to printable ASCII.
  Trust came from verifying that certificate against roots the deployment
  declared, which had already happened before this value existed.

The advisory columns of these two ledgers are the **only** place a destination
address lives. `integration_receipts`, `integration_canonical_events`,
`integration_message_lineage`, `integration_delivery_attempts`, and
`integration_delivery_outbox` carry no address of any kind, and the kill-test
scans all five for a scheme, a `host:port`, and a loopback literal.

The outcome CHECK is added `NOT VALID`, so a later backfill cannot silently claim
the constraint governed rows it never saw.

## Verification

```bash
export POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=...
make delivery-identity        # both 4.1c-a proofs
make destination-transport    # both 4.1c-b proofs, plus the negative control
make delivery-reliability     # Slice 2.3's proof must still pass
```

CI runs them as the blocking jobs `test:delivery-identity` and
`test:destination-transport`. Each one's first step asserts that **both** of its
test names exist, so a renamed or deleted proof turns the job red rather than
green.

`test:destination-transport` also runs the kill-test's negative control in the
same invocation: the whole scenario repeats against a router that reports it owns
no destination, and four named assertions must **fail** there while the
kafka-class assertion still passes. A control that passes turns the job red,
because it would mean the router is not on the dispatch path.

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
- **Before upgrading to 4.1c-b, read your registry.** Any destination already
  declaring `transport: https` starts receiving real traffic; the field was inert
  before.
- Rotating a destination credential or trust bundle is a write in place. It takes
  effect on the next dispatch — no restart, no signal, no cache to clear. Write
  atomically (write-then-rename) so a dispatch cannot read a half-written file;
  a torn read fails closed as a retryable
  `DELIVERY_DESTINATION_CREDENTIAL_UNRESOLVED` rather than as an unauthenticated
  request, but it still costs an attempt.
- A `DELIVERY_DESTINATION_REDIRECT_REFUSED` dead letter means the destination
  moved. Publish a new destination revision with the new URL; the engine will
  never follow a destination-chosen address.
- A `DELIVERY_DESTINATION_UNAVAILABLE` storm opens the per-destination circuit
  after `FI_FHIR_DELIVERY_CIRCUIT_FAILURE_THRESHOLD` consecutive failures, which
  stops the worker spinning against one sick destination while others keep
  flowing. The circuit is keyed on the destination artifact, not on the broker.
- The delivery worker still requires a Kafka broker even in an HTTPS-only
  deployment. See "Kafka is still required" above.
