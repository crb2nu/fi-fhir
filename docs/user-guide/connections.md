# Connections

A **connection** is a named, reusable declaration of one endpoint the engine
listens on (a **source**: MLLP, HTTP, S3 batch, SFTP batch) or delivers to (a
**destination**: HTTPS, FHIR, Kafka). The Mapping Studio's `/connections` page
lets you author them, compile them into the exact documents `fi-fhir serve`
mounts, and see which of them this replica is actually running. From HL7
intake you can also pull real messages from a source connection into an
Integration Session to build a profile against.

The catalog **authors and labels; it never activates anything.** `serve`
still composes its listeners, runners, and destinations from environment and
mounted files once, at startup. Putting a compiled connection into service is a
reviewed GitOps change, exactly as it was before the catalog existed. Whether
that should change is an open design question, recorded in
`.loom/39-brainstorm-config-plane-2026-09-27.md` (see
[What may change](#what-may-change)).

Operators: the ledger, roles, audit table, and allow-lists are in
[Connection catalog operations](../operations/CONNECTION-CATALOG.md).

## Before you start

The page needs two things, and says which one is missing instead of failing:

| You see | Meaning | Fix |
|---|---|---|
| "The connection catalog is not configured on this deployment." | `capabilities.connectionCatalog` is false: `serve` did not open its durable PostgreSQL database, so the catalog has nowhere to live. | Turn on any durable feature (`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true` is the usual one) with `FI_FHIR_DATABASE_*` set. |
| "This identity (…) does not hold `integration.operator` …" | `capabilities.connectionsRead` is false. No query was sent. | Grant `integration.operator`. |
| A status line saying the forms are read-only | `capabilities.connectionsWrite` is false. | Grant `integration.deployment.operator` as well. |

The production operator bundle already carries both roles, so no new role
exists for connections.

## The journey

### 1. Define

Open **Connections** from the activity bar, or run **New source connection**,
**New destination connection**, or **Engine properties** from the command
palette. On the Sources or Destinations tab, **New ▾** offers the kinds for
that direction.

| Direction | Kinds | Compiles to |
|---|---|---|
| Source | `mllp` | an MLLP source revision (`internal/integration/mllp/source.go`) |
| Source | `batch_s3`, `batch_sftp` | a batch source revision (`internal/integration/batch/source.go`) |
| Source | `http` | an HTTP source document (`internal/integration/connection/http_source.go`, new with this catalog) |
| Destination | `https`, `fhir`, `kafka` | a destination revision (`internal/integration/destination/revision.go`) |

A connection's **ID** becomes the `artifact_id` of every revision compiled from
it and the stem of the downloaded file name, so it is restricted to
`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$` and cannot be changed later. The **name**
and **description** are labels and can be edited.

The **Settings** tab is a form generated from the kind's spec (field for field
the document above, in snake_case). Numbers carry their documented bounds,
enums are menus, and lists are one entry per line. A draft may be incomplete:
you can save a half-filled form and come back to it.

The **Secrets** tab lists **bindings**, never values. A binding is a name the
spec uses in a `*_binding` field plus a reference: `{provider, key,
version?}`, where the provider is one of `env`, `file`, `vault`, `aws-ssm`, or
`k8s`. There is no column for a value and no field anywhere in the API that
could carry one. A save is refused, and nothing is written, when the spec
carries a key its kind does not define, or anything that looks like secret
material: a key whose name contains `token`, `password`, `secret`, `api_key`
and the like, a PEM block, a URL with credentials or a secret-named query
parameter, or a `*_binding` value that does not name one of the draft's own
bindings. The refused paths are marked on the form.

A binding the spec never uses is a warning (`UNUSED_BINDING`) and does not
block compile.

Every save, compile, and archive asks for a **reason** (1–1024 bytes). It is
recorded with your verified identity on the draft or revision.

An example `batch_s3` spec (synthetic values):

```json
{
  "source_id": "adt-east",
  "s3": {
    "endpoint": "objects.example.com:443",
    "region": "us-east-1",
    "bucket": "adt-drop",
    "input_prefix": "incoming",
    "archive_prefix": "archive",
    "use_tls": true,
    "access_key_binding": "adt-east-access-key",
    "secret_access_key_binding": "adt-east-secret-key"
  },
  "poll_seconds": 15,
  "lease_seconds": 120,
  "process_seconds": 60,
  "max_files_per_poll": 100,
  "max_message_bytes": 1048576
}
```

with two bindings on the Secrets tab, for example
`adt-east-access-key → env FI_FHIR_CONNECTION_SECRET_ADT_EAST_ACCESS_KEY` and
`adt-east-secret-key → env FI_FHIR_CONNECTION_SECRET_ADT_EAST_SECRET_KEY`
(that naming is what lets a [peek](#batch-peek) use them). The checked-in
fixtures for every kind are in `internal/integration/connection/testdata/`.

### 2. Validate

While you edit, the form runs `validateConnectionSpec` (debounced) and puts
each problem on its field by path, for example `timeouts.read_seconds` or
`s3.bucket`. Problems carry a stable code: `REQUIRED`, `OUT_OF_RANGE`,
`INVALID_ENUM`, `UNKNOWN_FIELD`, `UNBOUND_SECRET`, `UNUSED_BINDING`,
`SECRET_VALUE_FORBIDDEN`, `INVALID_URL`, `INVALID_CIDR`, `INVALID_ADDRESS`,
`INVALID_VALUE`, `INVALID_TYPE`, `INVALID_JSON`, `DUPLICATE`, `CONFLICT`, and
`FORBIDDEN`. Validation writes nothing.

### 3. Compile

**Compile** turns the saved draft into an immutable, numbered **revision**
(`r1`, `r2`, …). It runs the document's own constructor, so a compiled MLLP
source is byte-for-byte what an operator would have written by hand, and its
digest is the one the lifecycle catalog checks. If any blocking problem
remains, compile shows them on the form and writes nothing. Compiling a draft
that has not changed since its last revision returns that revision and writes
nothing. Compile reads the **saved** version, so save your edits first.

### 4. Export or reference

On the **Revisions** tab, select a revision:

- **Download** saves `<id>-r<N>.json`, and **Copy JSON** copies it. Both hand
  over the stored `revisionJson` bytes exactly as `serve` will mount them; the
  indented view on screen is for reading only.
- To **reference** a revision from an integration definition, use its
  `{artifact_id, revision_id, digest}` as the definition's source ref (or one
  of its destination refs).

There is no definition editor yet (see [What may change](#what-may-change)).
Definitions are still written by hand: in the static registry
(`FI_FHIR_INTEGRATION_REGISTRY_PATH`) for HTTP ingress and preview, and in the
PostgreSQL lifecycle catalog for MLLP and batch.

### 5. Mount it in GitOps

Where each kind goes (`fi-fhir serve --help` documents every key):

| Kind | The revision bytes go | It also needs |
|---|---|---|
| `mllp` | a mounted file named by `FI_FHIR_MLLP_SOURCE_CONFIG_PATH` | `FI_FHIR_MLLP_DEFINITION_ID` naming a lifecycle definition that is `deployed` and whose source ref is this revision; when `tls.mode` is `mutual`, the certificate, key, and client CA files at `FI_FHIR_MLLP_TLS_CERT_FILE`, `FI_FHIR_MLLP_TLS_KEY_FILE`, `FI_FHIR_MLLP_TLS_CLIENT_CA_FILE`. See [Production MLLP](../operations/PRODUCTION-MLLP.md). |
| `batch_s3`, `batch_sftp` | a mounted file named by `FI_FHIR_BATCH_SOURCE_CONFIG_PATH` | `FI_FHIR_BATCH_DEFINITION_ID` (as above); the runner's credentials from `FI_FHIR_BATCH_S3_ACCESS_KEY[_FILE]` and `FI_FHIR_BATCH_S3_SECRET_KEY[_FILE]`, or `FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE` plus `FI_FHIR_BATCH_SFTP_PASSWORD[_FILE]` or `FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE`. See [Batch ingestion](../operations/BATCH-INGESTION.md). |
| `http` | nowhere: the runtime does not read this document | HTTP ingress is configured by `FI_FHIR_HTTP_INGRESS_*`, which the document declares field for field. It mounts the source named by the definition that its bound integration (`FI_FHIR_HTTP_INGRESS_INTEGRATION_ID`) resolves to in the static registry, so put this revision's ref there. |
| `https`, `fhir`, `kafka` | one entry of `destinations[]` in the registry document named by `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` (schema `fi-fhir/destination-registry/v1`) | every binding the revision names declared in the registry's `secret_bindings`; `FI_FHIR_DELIVERY_IDENTITY_MODE`; the delivery worker (`FI_FHIR_DELIVERY_WORKER_ENABLED=true`, `FI_FHIR_QUEUE_DRIVER=kafka`). See [Destination identity](../operations/DESTINATION-IDENTITY.md). |

Mount the bytes unchanged (an immutable, hash-suffixed ConfigMap is how the
static registry is shipped today); editing the file changes its digest.

The connection's own secret bindings are **not** how the runtime finds its
credentials: the MLLP listener and batch runner read the fixed keys above, and
the delivery registry reads its own `secret_bindings`. The bindings on the
Secrets tab name what the document needs, and a batch peek resolves them (see
below).

### 6. See it mounted

After the rollout, the connection's **Status** column says what this replica
runs. The states are additive; a row can read "Compiled r3 · Referenced ·
Mounted here":

| State | Meaning | Evidence |
|---|---|---|
| Draft | never compiled, or edited since the latest revision | `draft.version > latest.compiledFromVersion` |
| Compiled r*N* | the latest revision was compiled from the current draft version | catalog |
| Referenced | ≥ 1 lifecycle definition revision names the digest | `integration_definition_revisions.revision_json` |
| Deployed | a referencing definition's snapshot is `deployed` or `paused` | lifecycle snapshots |
| Mounted here | this replica's MLLP listener, batch runner, or delivery registry runs a document with this digest; for HTTP, the ingress is bound to a definition that references it | `engineRuntime` |

A draft edited after the mounted revision reads "Draft (r3 mounted)"; when the
replica runs an older revision than the latest, the token says which ("r2
mounted here"). An archived connection says "Archived" first. No state is ever
inferred from a name matching a name: only digests count.

Two things to expect:

- **Referenced and Deployed read from the lifecycle catalog only**, and nothing
  in production writes definition drafts to it yet. On most deployments these
  two states are honestly absent. A definition in the static registry does not
  make a connection "Referenced"; it can still make an HTTP source "Mounted
  here".
- **Mounted here is per replica.** It reflects the replica that answered the
  request, as it was composed at startup.

The **Usage** tab lists every referencing definition revision (definition,
revision, the digest it names, lifecycle state, health) and this replica's
runtime role for the connection. The **Engine** tab shows the whole replica:
identity and access, the control plane, the static registry's integrations,
one panel per adapter (HTTP ingress, MLLP listener, batch runner, delivery
worker, destination identity) with the environment key behind each property,
the schema ledgers, and every documented `serve` property. A secret property
shows only `set` or `unset`. To change any of them, change the key in GitOps
and roll the deployment.

**Archive** freezes a draft. Its revisions stay readable, so a definition that
references one keeps resolving it. Nothing is ever deleted.

## Sampling from a connection

With the Integration Session engine on, the HL7 intake page's **Samples**
panel has **From connection…**. It puts real messages from a source into the
page's current Integration Session (creating one if no Preview has run yet),
where any HL7v2 message type previews, so you can shape a profile against a
live feed. Messages never enter the production kernel this way.

**From connection…** is absent when the session engine is off, and disabled
when the connection catalog is not configured (its tooltip names
`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`). Without `integration.operator` the
dialog names the missing role and queries nothing. With no mounted source and
no catalog source it says "No source connection is mounted on this
deployment."

The dialog lists one row per source you can sample: this replica's enabled
MLLP, HTTP, and batch adapters (from the Engine tab) and the catalog's source
connections. A catalog MLLP or HTTP connection whose `source_id` a mounted
adapter admits shares that adapter's row, because captures are keyed by source
ID. Each row states what it is:

| Row state | Meaning |
|---|---|
| Mounted here r*N* | this replica runs the source |
| Compiled r*N* · not mounted here | a compiled catalog connection no adapter here runs; a stream one can still be captured on the replicas that admit it |
| Draft | never compiled; it cannot be sampled (the button's tooltip says why) |
| Mounted here · no catalog connection | a batch runner with no catalog connection; a peek needs a compiled catalog connection for its bindings |

A stream source offers **Capture…**; a batch source offers **Browse
objects…**.

### Stream capture

A capture taps the next messages one source admits, on any replica:

- You choose how many messages (1–100, default 5), how long it stays armed
  (1–900 seconds, default 300), and a reason.
- Only messages the engine **durably accepted in production** are captured, in
  arrival order. Preview traffic, rejected frames, and other sources never are.
  A capture never changes an ACK, a receipt, or an admission result; an armed
  source's ACK may be up to about 2 seconds slower.
- **One armed capture per source**, across all sessions. A second one is
  refused with "a capture is already armed for this source"; cancel the armed
  one from its row in Samples.
- The Samples panel shows a row such as "2 / 5 captured · expires in 4:12 ·
  Cancel" and polls every 2.5 seconds while any capture is armed, pulling new
  samples into the inbox as the count moves; it stops polling when none is
  armed (or after three failed polls, with Retry on the row). Past its
  deadline the row reads "expiring" until the server reports it `expired`. It
  finishes as `complete` at the count, `expired` at the deadline, or
  `cancelled` (Cancel asks for a reason; samples already captured stay).
  Another replica picks up a new capture within about 2 seconds.
- The source is the **runtime** source ID a replica admits frames under, or the
  `source_id` of a compiled MLLP or HTTP connection. A batch source cannot be
  captured; peek it instead.

**Known limitation.** The v1 production kernel admits only ADT^A01-shaped
frames with MSH, EVN, PID, and PV1 segments (plus NTE). A frame with NK1, IN1,
or any other segment is answered `AR` and never durably accepted, so it is
never captured: a capture of a real ADT feed that carries NK1 or IN1 stays
armed with 0 captured until it expires. The capture dialog says so, and a row
armed for 30 seconds with nothing captured repeats it. For such feeds, use a
batch peek.

### Batch peek

A peek reads a compiled `batch_s3` or `batch_sftp` connection's source the way
you would look at it by hand:

1. **List objects** lists 1–50 objects (default 10) under the input prefix or
   directory, in the provider's listing order. **Listing is itself an audited
   peek**: it needs a reason and writes an audit row, and it reads no message.
2. Choose an object, then **Read messages** adds its first 1–50 messages
   (default 5) to the session; they appear in Samples and the peek becomes
   the active sample. Samples a background capture adds never replace what is
   in the editor.

A peek takes no lease, writes no checkpoint, archives nothing, and deletes
nothing: the batch runner later ingests the object exactly as it would have. It
cannot be cancelled and gives up after 60 seconds. Problems after the audit row
exists are reported in the dialog, not as errors: `SECRET_UNRESOLVABLE`,
`SOURCE_UNAVAILABLE`, `OBJECT_NOT_FOUND`, `MESSAGE_UNREADABLE`,
`SAMPLE_WRITE_FAILED`. Samples read before a problem stay in the session.

**Credentials.** A peek resolves the connection's own bindings, and only two
shapes of reference: an `env` binding whose key starts
`FI_FHIR_CONNECTION_SECRET_`, or a `file` binding whose key starts
`connections/` (under `FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR`). Anything else is
`SECRET_UNRESOLVABLE` and nothing is contacted. Ask your operator to provision
the connection's credentials under those names; the reason is in
[Connection catalog operations](../operations/CONNECTION-CATALOG.md#the-peek-secret-allow-list).

### What a captured or peeked sample holds

Every captured and peeked message is redacted before it is stored, with a
redactor that masks **more** than the one pasted samples use: every HIPAA Safe
Harbor identifier field of PID, NK1, IN1, IN2, and GT1, all of MRG, and PV1's
visit numbers, 113 fields in all, each replaced whole by `REDACTED`. Dates are
masked whole, year included. The table is in
[PHI retention, "Captured and peeked samples"](../operations/PHI-RETENTION.md#captured-and-peeked-samples).
Expect `REDACTED` where names, identifiers, and dates were.

A captured sample is still PHI: free text, clinical content, and every segment
outside the table pass through. It lives in the session under the session's own
retention and is never exported with its text. It opens in the editor only for
a caller holding `integration.operator`; anyone else sees the role named in
its details instead of the text. Its `source` records where it came from:
`capture:<capture id>` or `peek:<capture id>`, shown with the session sample
ID in the details pane. Previewing such a sample unchanged runs the session's
stored copy by its sample ID rather than adding it again; once you edit it,
Preview sends the editor text like any pasted sample.

Every capture and peek, including a listing, is one audited row: who, why,
which session, which source or connection and digest, which object, the limits,
the count, and how it finished.

## What is and is not live-reloaded

**Nothing in the catalog is.** Saving, compiling, or archiving a connection
changes no running listener, runner, or destination on any replica. A replica
reports what it mounted at startup until it restarts.

What does change without a restart today:

- **Lifecycle state.** Deploy, pause, resume, and retire of an integration
  definition (the Operator page) take effect on the next MLLP frame or batch
  poll.
- **Destination credentials and CA bundles** for `https` and `fhir`
  destinations are re-read on every dispatch, so a rotation in place takes
  effect on the next delivery
  ([rotation contract](../operations/DESTINATION-IDENTITY.md#the-rotation-contract)).
- **Stream captures** arm and disarm on every replica within about 2 seconds.

Everything else, including every `FI_FHIR_*` property on the Engine tab, the
mounted source documents, the destination registry, and the static registry,
needs a rollout.

## What may change

Two follow-ups are recorded in the roadmap:

- **An integration definition editor**: binding a source, a profile, a workflow,
  and destinations into a definition draft in the lifecycle catalog, which would
  make "Referenced" and "Deployed" meaningful on every deployment.
- **A database-backed configuration plane**: whether a compiled connection could
  ever be activated from the catalog rather than by a rollout.
  `.loom/39-brainstorm-config-plane-2026-09-27.md` recommends a provisioned
  GitOps baseline with a catalog overlay for the hot-reloadable class
  (destinations, routing, enable/disable), listeners and TLS staying in GitOps,
  and per-replica observed digests first. It is not decided: its kill-test,
  swapping a destination into a running delivery registry on two replicas
  without a restart, has not been run.

Until then, the catalog is where connections are authored and checked, and
GitOps is where they run.
