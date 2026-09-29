# Connection Catalog Operations

The connection catalog (`internal/integration/connection`, `.loom/38` lanes
C-0 and C-2) is a durable store of source and destination connection drafts,
the immutable revisions compiled from them, and the audit of every sample
intake. This page is for operators: what it stores, who may use it, what it
reports, and how its proofs run. The user journey is in
[Connections](../user-guide/connections.md).

**It activates nothing.** `serve` still mounts MLLP and batch source documents,
the destination registry, and the static integration registry from files and
environment at startup (`cmd/fi-fhir/preview_runtime.go`). The catalog makes
those same documents authorable and exportable, and labels each connection
against what the replica that answered actually mounted (`engineRuntime`).
Package comment, `internal/integration/connection/types.go`: "the catalog
never hot-loads anything".

## When it exists

`runServe` builds the catalog whenever it opened the durable PostgreSQL
submission database (`cmd/fi-fhir/main.go`, the `securePreviewRuntime.submissionDB
!= nil` block), in the same block that builds the operator control plane. That
database opens when any of these is on (`cmd/fi-fhir/preview_runtime.go`):
`FI_FHIR_HTTP_INGRESS_AUTH_MODE`, `FI_FHIR_MLLP_SOURCE_CONFIG_PATH`,
`FI_FHIR_BATCH_SOURCE_CONFIG_PATH`, `FI_FHIR_DELIVERY_WORKER_ENABLED`,
`FI_FHIR_INTEGRATION_SESSION_ENABLED`, or
`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`, with the `FI_FHIR_DATABASE_*`
connection settings. Startup then migrates the catalog's ledger and logs
`connection catalog configured`; a migration failure stops startup.

Sample intake (peek and capture) additionally needs the Integration Session
workspace (`FI_FHIR_INTEGRATION_SESSION_ENABLED=true`): there is nowhere else
to put a sample. Startup logs `connection sample intake` with `enabled`.

## The seventh ledger

| Fact | Value |
|---|---|
| Ledger table | `integration_connection_schema_migrations` |
| Advisory lock key | `5064657639792058909` (`connectionMigrationLockKey`, `internal/integration/connection/postgres.go`), distinct from every other `*MigrationLockKey` |
| Migrations | `0001_connection_catalog.sql` (C-0: drafts, revisions, captures, guards), `0002_connection_capture_intake.sql` (C-2: capture problems, cancellation, object path, one armed stream capture per source), `0003_runtime_observations.sql` (.loom/39 step 1: per-replica runtime observations) |
| `SchemaVersion` | `3` |
| Ledger name | `connection` (`observability.SchemaLedgerConnection`) |

The migrator follows `AGENTS.md` § Migration authoring: it takes
`pg_advisory_xact_lock` on its own key before reading the ledger version, so
two replicas starting together converge. The ledger is forward-only, like the
other six (submission, session, lifecycle, batch, destination, terminology).

Where the version shows:

- `fi-fhir version` prints every ledger; this one is the line
  `connection   3`.
- `/metrics` reports `fi_fhir_schema_ledger_version{ledger="connection"} 3`.
- `engineRuntime.ledgers` (the Engine tab's Ledgers table) lists the same
  seven.

`test:migration-compatibility` holds all seven ledgers together: concurrent
migration, a binary one version behind still writing, and a `pg_dump`/restore
round trip that brings back every row and trigger.

## Tables and their rules

The drafts, revisions, and captures tables are guarded by row-level triggers
in the `0004_audit_immutability.sql` idiom. No row in any of them is ever
deleted. `integration_runtime_observations` is the exception on purpose: it is
a heartbeat table, upserted in place, with no trigger (see
[Runtime observations](#runtime-observations)).

| Table | Shape | Rules the schema enforces |
|---|---|---|
| `integration_connection_drafts` | one row per `(tenant_id, artifact_id)`: direction, kind, name, description, `spec_json`, `secret_bindings_json`, `version`, `archived_at`, created/updated audit | Identity (tenant, id, direction, kind) and creation audit never change; every update raises `version` by exactly one (an expected-version update); an archived row is frozen; direction must match kind. |
| `integration_connection_revisions` | one row per compile: `revision_text` (the exact bytes, returned verbatim as `revisionJson`), `revision_json` (the same document as JSONB, `CHECK revision_text::jsonb = revision_json`), `digest`, `compiled_from_version`, created audit | Append-only (`UPDATE` and `DELETE` raise); `UNIQUE (tenant_id, digest)`; revision ids are `1`, `2`, … per connection. |
| `integration_connection_captures` | one row per peek or stream capture (see [Capture audit](#capture-audit)) | Provenance frozen, `object_path` included; while `armed`, only `status`, `captured`, `completed_at` (and on finish `problems_json`, `cancellation_json`) advance, each by an update that raises `version` by one; `captured` never decreases and never exceeds `max_messages`; a finished row is frozen; at most one armed stream capture per `(tenant_id, source_id)` (unique partial index). |

| `integration_runtime_observations` | one row per `(tenant_id, replica_id, adapter)`: `definition_id`, `artifact_id`, `revision_id`, `digest` (all nullable), `observed_at`, `heartbeat_at`; index on `(tenant_id, digest)` | Upsert-only: every heartbeat rewrites the row in place. `observed_at` moves only when the reported digest or revision changes; `heartbeat_at` moves on every tick. No immutability trigger. |

Two columns hold a revision because JSONB alone cannot return exact bytes (it
reorders keys), and the digest is over exact bytes.

Service rules on top of the schema (`internal/integration/connection/service.go`):

- Every method reads the verified caller from the request context, never from
  an argument, and scopes every read and write to the one deployment tenant the
  service is bound to. Another tenant's connection reads as not found.
- A draft may be incomplete: missing fields, out-of-range values, and wrong
  scalar types are compile problems, not write errors. A write refuses only
  what a draft must never persist: a key the kind does not define, secret
  material in any form (a secret-looking key, a PEM block, a URL with a
  secret-named query parameter or userinfo), a malformed binding reference, or a
  `*_binding` value that names none of the draft's bindings. Refusals carry
  `extensions.problems` with codes and paths.
- Compile runs the document's constructor (`mllp.NewSourceRevision`,
  `batch.NewSourceRevision`, `destination.NewRevision`, or
  `NewHTTPSourceRevision`) and stores its exact output. A compile with blocking
  problems writes nothing. A compile of a draft version that is already compiled
  returns the existing revision and writes nothing. Compile never raises the
  draft version. Racing compiles claim exactly one revision; a compile racing an
  archive waits for it and reports archived.
- The field-level checker restates the constructors' bounds so it can name the
  failing field (the constructors return one coarse error on purpose).
  `TestConnectionChecker_MirrorsConstructorBounds` drives every bound through
  both, so `CONSTRUCTOR_REJECTED` is never produced.

## Runtime observations

`engineRuntime` and a connection's `runtime.mounted` describe only the replica
that answered the request. `integration_runtime_observations` is every
replica's report, so a reader can say "observed on N/N replicas" (.loom/39,
"Convergence" step 1).

- **Who writes.** Every `serve` replica with the durable submission database
  (the same condition that migrates this ledger) runs its own reporter
  (`runtimeObservationReporter`, `cmd/fi-fhir/serve_observability.go`). There is
  no leader: a leader-only report would hide exactly the divergence this table
  exists to show. `replica_id` is `hostname-pid`, the MLLP rate quota's holder
  id. Without the database, or when the replica id cannot be derived, the
  reporter is off and startup logs one INFO line, `runtime observation heartbeat
  disabled` (`component=runtime-observations`).
- **What it writes.** One row per adapter row of the runtime description —
  `http`, `mllp`, `batch`, `delivery`, enabled or not — plus one row per
  destination in the delivery identity registry, with adapter
  `destination:<artifact id>`. A disabled adapter's row has null identity
  columns, so "this replica runs no MLLP listener" and "this replica has not
  reported" are different answers. For the MLLP listener and batch runner,
  `artifact_id` is the runtime source id and `digest` the mounted source
  revision's digest; for the HTTP ingress, `digest` is the source digest of the
  definition it is bound to.
- **Cadence.** On start, then once per `runtimeReportInterval` (one minute), the
  same timer as the lifecycle health report. Rows are fixed at startup, because
  `serve` mounts nothing after it. A failed write logs one WARN per tick,
  `runtime observation heartbeat failed`, and is retried on the next tick; it
  never stops the process.
- **Staleness.** A row is stale when `heartbeat_at` is more than three report
  intervals old (`ObservationStaleFactor`). Stale rows are not deleted; a
  replica that was scaled away stays visible as stale.
- **Where it shows.** `engineRuntime.observations` lists every row of the
  tenant with `stale`. A connection's `runtime.observedReplicas` counts the
  replicas with a fresh heartbeat that report any revision of that connection
  mounted, and `runtime.totalReplicas` counts the replicas with any fresh
  heartbeat. Without the connection catalog, `observations` is an empty list.
  Both fields are gated by the existing `integration.operator` rule for
  `engineRuntime` and `connections`.

To see the fleet from SQL:

```sql
SELECT replica_id, adapter, digest, heartbeat_at,
       heartbeat_at < now() - interval '3 minutes' AS stale
FROM integration_runtime_observations
WHERE tenant_id = '<tenant>'
ORDER BY replica_id, adapter;
```

## Roles

No new role (`.loom/38` Decision 4). The production operator bundle already
carries both.

| Fields | Transport gate (`rootFieldRoles`) | Service re-check |
|---|---|---|
| `connections`, `connection`, `connectionRevisions`, `connectionRevision`, `engineRuntime`, `connectionCaptures` | `integration.operator` | same (`engineRuntime` in its resolver) |
| `createConnection`, `updateConnection`, `archiveConnection`, `compileConnection`, `validateConnectionSpec` | `integration.operator` + `integration.deployment.operator` | same |
| `peekBatchConnection`, `startConnectionCapture`, `cancelConnectionCapture` | `integration.operator` | same, plus a session workspace and a reason |
| `SessionSample.redactedPayload` (a captured sample's text) | session read | `integration.operator`, per field; `null` otherwise |

## Capabilities

`/api/auth/status` (`internal/api/graphql/capabilities.go`) reports:

| Capability | Kind | True when |
|---|---|---|
| `connectionsRead` | role | the caller clears both halves for `connections` (missing roles in `missingRoles.connectionsRead`) |
| `connectionsWrite` | role | the caller clears both halves for `createConnection` (`missingRoles.connectionsWrite`) |
| `controlPlane` | deployment | `serve` built the operator control plane, i.e. the durable database is open (above) |
| `connectionCatalog` | deployment | `serve` built the connection catalog service; same condition today |
| `integrationSessions` | deployment | the session workspace is on; with `connectionCatalog`, this is what enables sample intake |

The deployment capabilities are how the IDE says "not configured on this
deployment" instead of "forbidden": at the GraphQL layer an unconfigured
catalog (`connection catalog unavailable`) is deliberately indistinguishable
from a missing role. `TestAuthCapabilityRepresentativesCoverTheirGroup` pins
that every field of each group shares its representative's roles.

## Error strings

Every failure maps to a stable, inventory-safe message
(`internal/api/graphql/resolvers/connection_catalog.go`,
`connection_intake.go`). Not found and forbidden are distinct; another
tenant's row is not found.

| Message | When |
|---|---|
| `authentication required` | no verified identity |
| `connection catalog action forbidden` | a required role is missing (service half) |
| `invalid connection catalog request` | malformed id, name, reason, version, or direction |
| `connection spec carries secret material` | the write gate refused the spec; `extensions.code` and `extensions.problems[{code,path,message}]` say where |
| `connection not found` | no such connection for this tenant |
| `connection already exists` | create with a taken id |
| `connection version conflict` | stale `expectedVersion` (never retried) |
| `connection is archived` | change to an archived draft |
| `connection catalog unavailable` | the catalog is not configured |
| `engine runtime unavailable` | no runtime description (only `serve` composes one) |
| `connection catalog request failed` | anything else |
| `connection sample intake unavailable` | no session workspace |
| `invalid connection sample intake request` | bad id, reason, bound, or `objectPath` |
| `integration session not found` / `integration session is archived` | the target session |
| `connection capture not found` / `connection capture is already finished` | cancel of an unknown or finished capture |
| `a capture is already armed for this source` | second start on a source |
| `peek requires a compiled batch source connection` | peek of a non-batch or uncompiled connection |
| `capture source unavailable: no mounted or compiled MLLP or HTTP source has this id` (`extensions.code = SOURCE_UNAVAILABLE`) | start on a source no tap can see |
| `connection sample intake request failed` | anything else |

## Capture audit

`integration_connection_captures` holds one row per request, peek or stream,
listings included: tenant, session, mode (`peek` | `stream`), the runtime
`source_id` (stream) or the connection and its revision digest (peek), the
`object_path` a peek read, the verified principal, the reason (1–1024 bytes),
`requested_at`, `expires_at`, `completed_at`, `captured`, `max_messages`,
`problems_json`, and `cancellation_json` (who cancelled and why).

| Status | Reached by |
|---|---|
| `armed` | a stream capture waiting for frames; a peek while its request runs |
| `complete` | `captured == max_messages`, or a peek that did all it was asked |
| `expired` | a stream capture past `expires_at` (moved by the next cache refresh, at most 2 s late; the tap is inert from `expires_at`), or a peek row still armed a minute after its deadline because its replica died |
| `cancelled` | `cancelConnectionCapture` on an armed capture |
| `failed` | a problem: `SECRET_UNRESOLVABLE`, `SOURCE_UNAVAILABLE`, `OBJECT_NOT_FOUND`, `MESSAGE_UNREADABLE`, `SAMPLE_WRITE_FAILED`, or `CAPTURE_COUNT_FAILED` |

How the tap fills a capture (`capture.go`, `capture_store.go`
`FillCaptureSlot`):

- Admission never queries the database for captures. Each replica refreshes
  an in-memory set of armed stream captures every 2 s
  (`CaptureRefreshInterval`); a frame of an unarmed source costs one map lookup.
- The tap runs after the inner processor returned an accepted production
  result, returns exactly that result, and swallows and meters its own
  failures. Its work is bounded by its own 2 s context; only an armed source's
  ACK can be up to that much slower.
- A slot is claimed with `SELECT … FOR UPDATE SKIP LOCKED` on the armed row,
  the sample is written, and the row is advanced by an expected-version update
  in the same transaction (completing it on the last slot). A frame that finds
  the row locked by another frame skips capture rather than waiting, so
  admission never queues behind the tap.
- The sample id is derived from the capture and the slot
  (`sample_capture_<capture id>_<slot>`), so a slot written twice is one sample
  and `captured` never counts a sample that is not in the session.

A peek takes no lease, writes no checkpoint, archives nothing, and deletes
nothing; its provider calls are List, OpenAt, and Close. It is bounded by 60 s.

## Redaction

Captured and peeked samples are stored with `PHIPolicyRedact` through
`session.RedactCapturedHL7v2`, a strict superset of the pasted-sample
redactor: 113 fields across PID, NK1, IN1, IN2, GT1, MRG, and PV1, dates
masked whole. The field table, what stays unmasked, encryption at rest, and
who can read the text are in
[PHI retention, "Captured and peeked samples"](PHI-RETENTION.md#captured-and-peeked-samples).
That table is the authority; `session.CaptureRedactedFields` is held equal to
it by test.

## The `engineRuntime` allowlist

`engineRuntime` is a read-only projection of what this replica composed, built
once at the end of `runServe` (`buildEngineRuntimeDescription`,
`cmd/fi-fhir/engine_runtime.go`) and served from a copy. It is PHI-free and
secret-free by construction: adapters are described from what
`loadIntegrationRuntimeFromEnv` actually built, destination endpoints are
reduced to scheme, host, and path (`connection.EndpointAdvisory`), and process
properties come from a closed allowlist, `serveProperties()`. Nothing reads
`os.Environ()` generically; the one prefix rule below filters it on exactly one
prefix.

Each allowlisted property is `{key, secret, family, defaultValue}`. It renders as:

| Case | `value` | `source` |
|---|---|---|
| secret, set | `set` | `env` |
| secret, unset | `unset` | `default` |
| not secret, set | the value, reduced by `EndpointAdvisory` if it is a URL, bounded to 512 bytes | `env` |
| not secret, unset | the documented default, or `""` | `default` |

`RuntimeDescription.Validate` refuses a secret property rendering anything but
`set`/`unset`, and startup fails if it does.

**The one prefix rule.** `FI_FHIR_CONNECTION_SECRET_*` is the only family
allowlisted by prefix (`family: true`, key `connectionSecretEnvPrefix`), because
its members are named by connection drafts rather than by the code. It renders
one secret row per set variable with that prefix and a non-empty name, sorted by
key, each `set`/`env`; the value is never read beyond whether it is non-empty.
With no member set it renders a single `FI_FHIR_CONNECTION_SECRET_*` row,
`unset`/`default`. `TestServePropertiesAllowlistExactlyOneFamily` keeps the rule
to this one secret family. Other documented families, such as
`FI_FHIR_DATABASE_*`, are allowlisted by their exact member keys.

Keys added after the C-4 close-out (2026-09-27), with their `secret` flags:

| Key | `secret` | Why |
|---|---|---|
| `FI_FHIR_INTEGRATION_SESSION_ENABLED` | no (default `false`) | a boolean |
| `FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE` | yes | path to the session retention key |
| `FI_FHIR_DELIVERY_IDENTITY_MODE` | no | `strict` or `compatibility` |
| `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` | no | destination registry document path |
| `FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT` | no | a grant subject, not a credential |
| `FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR` | yes | directory of destination credentials; its name matches the credential pattern |
| `FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE` | yes | the passphrase |
| `FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE_FILE` | yes | path to the passphrase, like `FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE` |
| `FI_FHIR_CONNECTION_SECRET_*` | yes | the prefix rule above |

The two credential paths are secret even though a path is not a credential:
the repository's rule is that a path to a credential renders only as
`set`/`unset`, and `TestServePropertiesMarkEveryCredentialSecret` enforces it by
name.

**Adding a property**:

1. Document the key in `serve --help` (`serveUsage` in `cmd/fi-fhir/main.go`).
   `[_FILE]` expands to both keys; `NAME_*` declares a family.
2. Add `{key: …}` to `serveProperties()` with `secret: true` if the value is a
   credential or the path to one, and `defaultValue` if the code applies one.
   Do not add a second `family: true` entry; a family whose members the code
   names is allowlisted member by member.
3. Run `go test ./cmd/fi-fhir -run 'TestServeProperties|TestDescribeServeProperties'`.
   `TestServePropertiesAreExactlyTheDocumentedKeys` fails in both directions (an
   allowlisted key `serve --help` does not document, or a documented key the
   Engine tab would not show), `TestServePropertiesMarkEveryCredentialSecret`
   fails a key matching `TOKEN|SECRET|PASSWORD|_KEY(_|$)` that is not secret,
   and `TestDescribeServePropertiesNeverRendersASecretValue` plants values and
   requires none to render.

The allowlist can only be as complete as `serve --help`: a key the help does
not document is not on the Engine tab. The session, delivery identity,
connection secret, and SFTP passphrase keys were missing until they were
documented and allowlisted as above.

## The peek secret allow-list

A peek resolves the bindings of a draft that any
`integration.deployment.operator` can write, and hands the material to a
provider that contacts the endpoint the same draft names; an S3 request
carries the access key in its `Authorization` header. Without a limit, a draft
could bind any process variable or any destination credential and read it back
at an endpoint of its own.

So `serve` wraps the destination identity runtime's env/file resolver in
`connectionSecretResolver` (`cmd/fi-fhir/connection_intake_runtime.go`), which
resolves only:

- `env` references whose key starts `FI_FHIR_CONNECTION_SECRET_` (and has a
  name after it);
- `file` references whose key starts `connections/`, read under
  `FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR` (the destination credentials beside
  that subtree are not a peek's; the inner resolver refuses a path escaping the
  directory; with the directory unset, only env resolves).

Every other reference, including `vault`, `aws-ssm`, and `k8s`, and any pinned
`version`, is `SECRET_UNRESOLVABLE` before anything is read or contacted.
Provision a connection's credentials under these names to make it peekable.
The runtime's own batch runner and MLLP listener do not use this resolver; they
read their fixed keys (`FI_FHIR_BATCH_S3_*`, `FI_FHIR_MLLP_TLS_*`, …).

## Metrics

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `fi_fhir_connection_capture_messages_total` | counter | `mode` (`peek`, `stream`) | messages a peek or capture added to a session |
| `fi_fhir_connection_capture_tap_errors_total` | counter | `reason` (`ledger`, `session_store`, `refresh`, `panic`) | failures the tap and its cache swallowed; none changed an admission result, ACK, or receipt |
| `fi_fhir_schema_ledger_version` | gauge | `ledger` (`connection` among them) | the ledger version this process expects |

A tap failure is also logged at warn with `component=connection-capture` and
the reason; the error text comes from a store or the tap, never from a
message.

## CI proofs

| Job | Make target | Proves |
|---|---|---|
| `test:connection-catalog` (`ci/test-connection-catalog.yml`) | `make connection-catalog` | Ten PostgreSQL proofs (`TestConnectionCatalog_*`; the existence guard asserts the original nine by name): two replicas migrate concurrently; create → stale update refused → compile, and the bytes decode with the kind's own decoder to the stored digest; a lifecycle definition naming a digest shows in `references`; restart preserves every row byte for byte; the schema refuses an `UPDATE` on a revision; secret material, unknown keys, and malformed bindings are refused with code and path and nothing is written; cross-tenant reads are not found; writes without `integration.deployment.operator` are forbidden; racing compiles claim one revision; a runtime observation upserts in place, moving `observed_at` only when the digest changes, and stays per replica and per tenant. |
| `test:connection-capture` (`ci/test-connection-capture.yml`) | `make connection-capture` | Seven proofs over a real MLLP listener, the durable processor, and MinIO: a capture armed at `maxMessages: 2` turns three admitted frames into exactly two redacted, sealed samples while every receipt and ACK matches an unarmed run; another source captures nothing (negative control); a closed session store changes no ACK and counts `reason="session_store"`; a 1 s TTL expires; racing frames never exceed `maxMessages`; a source no tap can see is refused; a peek lists and reads (NK1-2 and IN1-16 masked) while the batch tables and bucket stay byte-identical and the runner still ingests the object. |

Both extend `.integration-proof` and skip without their services, which is why
each carries an existence guard. `TestConnectionChecker_MirrorsConstructorBounds`
needs no database and runs in `test:unit`. Local runs need a PostgreSQL (and
MinIO for the peek) on the `7900xtx` Docker context; the variables are in each
job's `PROOF_LOCAL`.
