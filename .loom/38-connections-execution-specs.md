# Connections and engine properties — execution specs (2026-09-26)

> **Delivered 2026-09-27.** Spec MR !240; C-0 MR !241 (merged 2026-09-27,
> `eef1907ed`); C-1 MR !243 (2026-09-27, `907281c70`); C-2 MR !242
> (2026-09-27, `78f1e385f`); C-3 MR !245 (2026-09-27, `b365f9511`); C-4 MR !C4_IID. Where the merged
> code differs from this spec, `docs/operations/CONNECTION-CATALOG.md` and
> `docs/user-guide/connections.md` follow the code (worklog 2026-09-27,
> "Connections program delivered"). Decision 1 is reopened by `.loom/39`.

Brief (Cody, 2026-09-26, after the IDE design uplift shipped and verified):
"now we need to build out the UI to view/configure the integration engine
properties and define source and destination connections. We should be able
to source messages to build profiles from these connection sources, build the
profile for transformation/mapping, and define the destination connection.
So we'll want a concept like a connection profile for source and
destinations. Being able to preview/shape the semantic events from the
source stream/batches is the intuitive tooling I imagine."

This is the first step of golden journey 1 ("create connections → select/fork
Source Profile → paste samples → …", `.loom/20` § Golden journeys). It has had
no IDE surface since the program began. Coordinator designs and reviews;
lanes implement and open MRs for review; the coordinator arms.

## What exists (read against the code, 2026-09-26)

Sources, destinations, and the binding between them are three kinds of
immutable, content-addressed JSON documents that `serve` mounts at startup.
None has a durable catalog or an API.

| Thing | Contract | How `serve` gets it | Runtime limit |
|---|---|---|---|
| MLLP source | `internal/integration/mllp/source.go` `SourceRevision`: listen address, encoding, framing, timeouts, TLS mode plus cert/key/CA *bindings*, client CIDRs and identities, ACK policy, max bytes and connections. Digest domain `fi-fhir/mllp-source/v1`. | `FI_FHIR_MLLP_SOURCE_CONFIG_PATH` + `FI_FHIR_MLLP_DEFINITION_ID` (`cmd/fi-fhir/preview_runtime.go:255-300`) | one listener per process |
| Batch source | `internal/integration/batch/source.go` `SourceRevision`: provider `s3`/`sftp`, endpoint/bucket/prefixes or host/directories, credential *bindings*, poll/lease/process seconds, max files and bytes, optional workload identity. Digest domain `fi-fhir/batch-source/v1`. | `FI_FHIR_BATCH_SOURCE_CONFIG_PATH` + `FI_FHIR_BATCH_DEFINITION_ID` (`cmd/fi-fhir/batch_runtime.go`) | one runner per process |
| HTTP ingress | no document. `FI_FHIR_HTTP_INGRESS_*` env: auth mode `bearer`/`hmac-sha256`/`oauth2`, principal, bound integration id, max body, OAuth issuer/audience/claims/allowed clients (`preview_runtime.go:560-690`) | env | one, bound to one integration id |
| Destination | `internal/integration/destination/revision.go` `Revision`: transport `kafka`/`https`/`fhir`, class `production`/`sandbox`, URL or base URL or topic, token and CA *bindings*, client identity subject and grants. Digest domain `fi-fhir/destination-revision/v1`. | `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` registry document (`destination/registry.go`, `cmd/fi-fhir/destination_identity_runtime.go`) | one registry per process |
| Integration definition | `pkg/integration/revision.go` `IntegrationDefinitionRevision`: source ref + format + profile ref + workflow ref + destination refs + secret bindings (name → `{provider,key,version}`, never a value) + PHI policy + deployment policy (`deployment.go`: connection-validation freshness, schedule, health, capacity). | `FI_FHIR_INTEGRATION_REGISTRY_PATH` static registry (`internal/integration/registry/static.go`) for preview and HTTP; the PostgreSQL lifecycle catalog (`internal/integration/lifecycle`) for MLLP and batch runnable resolution. Profile and workflow bytes always come from the static registry (`preview_runtime.go:139,264`). | — |

Worked examples of every document, for fixtures and for the form generator:
`testdata/golden/integration/adt-mllp/source-revision.json` (MLLP),
`testdata/golden/integration/adt-batch-s3/source-revision.json` (batch),
`docs/operations/DESTINATION-IDENTITY.md` (a full destination registry
document, schema `fi-fhir/destination-registry/v1`:
`{tenant_id, integration_revision, secret_bindings, destinations}`),
`testdata/golden/integration/adt-http/integration-revision.json` (definition).

Consequences the design respects:

- **The lifecycle catalog is written by nothing in production.**
  `lifecycle.CreateDraft` and `ValidateConnection` are called only from
  tests, and every production catalog is built with a nil validator
  (`cmd/fi-fhir/main.go:4615`, `preview_runtime.go:247`,
  `batch_runtime.go:65`). `integration_definition_revisions` is therefore
  empty on a deployment nobody seeded, and the Referenced/Deployed states
  below will honestly read as absent until the definition editor
  (Decision 5) ships. The Mounted-here state does not depend on it.
- **The kernel's production and preview path accepts HL7v2 `ADT^A01` only**
  (`processor/adt_a01.go:43-47`); the session runner parses with the full
  parser (`session/runner.go`). Sample intake therefore lands messages in a
  session, where any HL7v2 message type previews, and never in the
  production kernel.
- **Only `env` and `file` secret references resolve today**, and a pinned
  `version` is refused (`destination_identity_runtime.go:186-213`). The
  catalog accepts all five providers because the contract does; a peek can
  resolve only those two and reports `SECRET_UNRESOLVABLE` otherwise.
- **Nothing resolves source bytes by digest.** The golden registry fixture's
  source ref is `sha256:aaaa…` (`testdata/golden/integration/adt-http/
  preview-registry.json`): the same "a digest that names an artifact that does
  not exist" gap the destination package closed for destinations in 4.1c-a
  (`destination/revision.go:5-10`).
- **Secrets are bindings.** Every document names bindings; the definition
  maps names to `SecretReference{provider,key,version}`; values are resolved
  in `cmd/` only and never enter a struct that is persisted
  (`destination_identity_runtime.go`, `destinationSecretResolver`). The
  catalog and the UI never hold, transmit, or display a value.
- **Runtime activation is GitOps.** `serve` composes adapters from env at
  startup; "production activation remains a separate reviewed action" on
  every slice of the roadmap. This program does not hot-load connections. It
  makes them authorable, compilable, exportable, and honestly labelled
  against what the running replica actually mounted.
- **Raw PHI is ephemeral in production.** The durable committer refuses every
  non-ephemeral raw retention mode (`processor/postgres_submission.go`), so
  "replay a received message into a session" is impossible by construction.
  A real message can reach the IDE only at admission time (a tap) or from a
  batch object that has not been consumed yet.
- **Sessions already hold samples redacted.** `session.Store.AddSample` with
  `PHIPolicyRedact` masks PID-3/5/7/11/13/19 (`session/store.go:669`) and
  stores no ciphertext; runs, diagnostics, lineage, and Problems already work
  on those samples (`runSessionPreview`). Production sessions run with no
  retention key, so retain is refused there.
- **The operator control plane lists deployments** (`operatorDeployments` →
  lifecycle snapshots: definition revision ref, state, health, validation)
  but not definition content. Home's "Integrations" panel reads it.
- **The capabilities contract** (`internal/api/graphql/capabilities.go`)
  derives role capabilities from `rootFieldRoles`
  (`operation_authorization_roles.go:93`; an unmapped root field is refused
  at the transport gate) and deployment facts from `ServerConfig`. ROADMAP
  "Now" already asks for "control plane not configured" to become a
  capability distinct from "forbidden"; the connections page needs exactly
  that distinction, so this program adds it.

## Design

### Vocabulary

A **connection** is a named, reusable, editable declaration of one endpoint
the engine listens on (a **source**: `mllp`, `http`, `batch_s3`, `batch_sftp`)
or delivers to (a **destination**: `https`, `fhir`, `kafka`). Editing changes
a mutable **draft** with an optimistic version. **Compiling** a draft produces
an immutable, content-addressed **connection revision** whose bytes are
exactly what `serve` mounts today (`mllp.NewSourceRevision`,
`batch.NewSourceRevision`, `destination.NewRevision`; HTTP gains an equivalent
document in Lane C-0). An integration definition references connection
revisions by `{artifact_id, revision_id, digest}`; the catalog reports which
definitions reference each revision and whether this replica has it mounted.

"Connection profile" in the brief = a connection draft plus its revisions.
The word *profile* stays reserved for Source Profiles in every user-facing
string.

The connection's `id` is its artifact id (`source.artifact_id` /
`destinations[].artifact_id` in a definition). Revision ids are `"1"`,
`"2"`, … per connection, like Source Profile revisions.

### Honest states (the Status column)

| State | Meaning | Evidence |
|---|---|---|
| Draft | never compiled, or edited since the latest revision | `draft.version > latest.compiledFromVersion` |
| Compiled r*N* | the latest revision was compiled from the current draft version | catalog |
| Referenced | ≥ 1 lifecycle definition revision names the digest | `integration_definition_revisions.revision_json` |
| Deployed | a referencing definition's snapshot is `deployed` or `paused` | lifecycle snapshots |
| Mounted here | this replica's MLLP listener, batch runner, or delivery registry runs a document with this digest; for HTTP, the ingress is bound to a definition that references it | `engineRuntime` |

These are additive: a row can be "Compiled r3 · Referenced · Mounted here".
A draft that was edited after r3 shows "Draft (r3 mounted)". No state is
ever inferred from a name matching a name.

### GraphQL contract (C-0 ships it; C-1 and C-3 consume it verbatim)

Root fields, in `internal/api/graphql/schema.graphql` as one `extend type`
block per root under a new `# Connection catalog (.loom/38 C-0)` section,
mapped in `operation_authorization_roles.go:89-93` (an unmapped root field
is refused at the transport gate), regenerated with `make lint-gqlgen`, and
covered by `lint:contracts` and the UI client codegen (`cd ui && npm run
codegen`, checked by `npm run codegen:check` inside `make lint-ui`; the
operation documents live in `ui/src/lib/graphql/*.graphql`).

```graphql
enum ConnectionDirection { SOURCE DESTINATION }
enum ConnectionKind { MLLP HTTP BATCH_S3 BATCH_SFTP HTTPS FHIR KAFKA }

type Connection {
  id: ID!                         # artifact id
  direction: ConnectionDirection!
  kind: ConnectionKind!
  name: String!
  description: String!
  spec: JSON!                     # kind-specific, non-secret; schema below
  secretBindings: [ConnectionSecretBinding!]!
  version: Int!                   # optimistic draft version, starts at 1
  archived: Boolean!
  latestRevision: ConnectionRevision
  references: [ConnectionReference!]!
  runtime: ConnectionRuntimeState!
  createdBy: OperatorPrincipal!
  createdAt: DateTime!
  updatedBy: OperatorPrincipal!
  updatedReason: String!
  updatedAt: DateTime!
}
type ConnectionSecretBinding { name: String!, provider: String!, key: String!, version: String }
type ConnectionRevision {
  artifactId: ID!, revisionId: ID!, digest: String!
  direction: ConnectionDirection!, kind: ConnectionKind!
  revisionJson: String!           # the exact bytes serve mounts
  compiledFromVersion: Int!
  createdBy: OperatorPrincipal!, createdReason: String!, createdAt: DateTime!
}
type ConnectionReference { definitionId: ID!, revisionId: ID!, digest: String!, state: String!, health: String! }
type ConnectionRuntimeState { mounted: Boolean!, role: String, detail: String }
  # role ∈ mllp-listener | batch-runner | http-ingress | delivery-registry
type ConnectionProblem { code: String!, path: String!, message: String! }
type ConnectionCompileResult { connection: Connection!, revision: ConnectionRevision, problems: [ConnectionProblem!]! }

input CreateConnectionInput { id: ID!, direction: ConnectionDirection!, kind: ConnectionKind!, name: String!, description: String, spec: JSON!, secretBindings: [ConnectionSecretBindingInput!], reason: String! }
input UpdateConnectionInput { id: ID!, expectedVersion: Int!, name: String, description: String, spec: JSON, secretBindings: [ConnectionSecretBindingInput!], reason: String! }
input ConnectionSecretBindingInput { name: String!, provider: String!, key: String!, version: String }
input ConnectionCommandInput { id: ID!, expectedVersion: Int!, reason: String! }
input ValidateConnectionSpecInput { kind: ConnectionKind!, spec: JSON!, secretBindings: [ConnectionSecretBindingInput!] }

extend type Query {
  connections(direction: ConnectionDirection, includeArchived: Boolean): [Connection!]!
  connection(id: ID!): Connection
  connectionRevisions(id: ID!): [ConnectionRevision!]!
  connectionRevision(artifactId: ID!, revisionId: ID!): ConnectionRevision
  engineRuntime: EngineRuntime!
}
extend type Mutation {
  createConnection(input: CreateConnectionInput!): Connection!
  updateConnection(input: UpdateConnectionInput!): Connection!
  archiveConnection(input: ConnectionCommandInput!): Connection!
  compileConnection(input: ConnectionCommandInput!): ConnectionCompileResult!
  validateConnectionSpec(input: ValidateConnectionSpecInput!): [ConnectionProblem!]!
}
```

Rules:

- `spec` is the kind's JSON shape below, snake_case, decoded server-side with
  `DisallowUnknownFields`; unknown keys are a `ConnectionProblem`
  (`UNKNOWN_FIELD`, path), never a GraphQL error. `validateConnectionSpec`
  runs the same pre-validation with no write; `compileConnection` returns
  `problems` non-empty and `revision: null` on failure and writes nothing.
- Pre-validation yields `{code, path, message}` per field, mirroring the
  bounds the constructors enforce (`mllp.SourceRevision.validateSemanticFields`
  returns the coarse `ErrInvalidSourceRevision` on purpose, for inventory
  safety, so the field-level checker lives in the catalog). The constructor
  stays the authority: a table test pins that every catalog-valid fixture
  compiles and every constructor-rejected fixture is caught by the checker
  first (`TestConnectionChecker_MirrorsConstructorBounds`).
- Secret binding **values** never exist in this surface. A binding is
  `{name, provider ∈ env|file|vault|aws-ssm|k8s, key, version?}`
  (`pkg/integration.SecretReference`). Every `*_binding` field in a spec must
  name a binding in `secretBindings` (`UNBOUND_SECRET`), and every binding
  must be named by some field (`UNUSED_BINDING`, a warning-level problem that
  does not block compile).
- Errors are inventory-safe as in `operator_control_plane.go`
  (`catalogOperatorError`): not-found and forbidden are distinct; cross-tenant
  reads are not-found; an unconfigured catalog is
  `ErrConnectionCatalogUnavailable`, indistinguishable from a missing
  capability at the GraphQL layer, and reported honestly through
  `/api/auth/status` instead.

### Per-kind `spec` (mirrors the Go documents field for field; C-1 builds the forms from this)

- `mllp` (→ `mllp.SourceRevisionInput`): `source_id`, `listen_address`
  (`host:port`), `encoding` (`utf-8` only today), `framing{start_byte,
  end_byte, trailer_byte}` (defaults 11/28/13), `timeouts{read_seconds 1–300,
  write_seconds 1–60, idle_seconds ≥ read ≤ 3600, process_seconds 1–300}`,
  `tls{mode disabled|mutual, server_certificate_binding, server_private_key_binding,
  client_ca_binding}` (all three bindings required when mutual),
  `clients{allowed_cidrs[], identities[]?}`, `acknowledgements{mode
  application|commit, include_error_segment}`, `max_message_bytes ≤ 1048576`,
  `max_connections 1–10000`.
- `batch_s3` (→ `batch.SourceRevisionInput`, `Provider: s3`): `source_id`,
  `s3{endpoint, region?, bucket, input_prefix, archive_prefix, use_tls,
  access_key_binding, secret_access_key_binding}`, `workload?`,
  `poll_seconds`, `lease_seconds`, `process_seconds`, `max_files_per_poll`,
  `max_message_bytes`. Bounds: read them from `batch/source.go`
  `validateSemanticFields` and pin them in the checker test.
- `batch_sftp`: as above with `sftp{host, port, username, input_directory,
  archive_directory, known_hosts_binding, password_binding?,
  private_key_binding?, private_key_passphrase_binding?}` (exactly one of
  password/private key).
- `http` (new document, `internal/integration/connection/http_source.go`,
  digest domain `fi-fhir/http-source/v1`, `schema_version "1"`): `source_id`,
  `path` (default `/v1/hl7v2`), `auth_mode bearer|hmac-sha256|oauth2`,
  `principal_id`, `credential_binding` (required for bearer/hmac, forbidden
  for oauth2), `oauth{issuer_url, audience, tenant_claim, roles_claim,
  client_id_claim, signing_algs[], allowed_client_ids[]}` (required for
  oauth2, forbidden otherwise), `max_body_bytes 1–1048576`. It declares what
  `FI_FHIR_HTTP_INGRESS_*` configures; the runtime does not consume it in
  this program, and the Engine tab says so.
- `https` (→ `destination.RevisionInput`): `destination_id`, `class
  production|sandbox`, `https{url (https scheme), method POST|PUT,
  token_binding, ca_bundle_binding?}`, `identity{subject, grants[]}?`.
- `fhir`: `destination_id`, `class`, `fhir{base_url, token_binding,
  ca_bundle_binding?, interaction: "transaction"}`, `identity?`.
- `kafka`: `destination_id`, `class`, `kafka{topic}`, `identity?`.

The catalog stores `spec` as given (validated shape) and stores the
compiled revision's exact bytes separately; it never re-derives one from the
other after compile.

### Engine properties (`engineRuntime`)

A read-only projection of what this replica composed at startup, PHI-free and
secret-free by construction (an **allowlist** of properties in Go, each with
a `secret` flag whose value is rendered only as `set`/`unset`; nothing is
read from `os.Environ()` generically).

```graphql
type EngineRuntime {
  version: String!, tenantId: ID!, replicaId: String!   # hostname-pid, as the rate quota uses
  authMode: String!, trustedNetwork: Boolean!, accessIdentity: Boolean!
  controlPlane: Boolean!, integrationSessions: Boolean!, streaming: Boolean!, retentionPurge: Boolean!, llmConfigured: Boolean!
  registry: EngineRegistry!            # { integrationCount, integrations: [{integrationId, definitionId, revisionId, digest, sourceId, format}] }
  adapters: [EngineAdapter!]!          # exactly four rows, enabled or not: http, mllp, batch, delivery
  destinationIdentity: EngineDestinationIdentity  # { mode, destinations: [{artifactId, revisionId, digest, transport, class, endpointAdvisory}] } or null
  ledgers: [EngineLedger!]!            # { name, version } from each package's SchemaVersion (submission, session, lifecycle, batch, destination, terminology, connection)
  properties: [EngineProperty!]!       # { key, value, secret, source: env|default } — the serve usage's documented keys only
}
type EngineAdapter {
  kind: String!, enabled: Boolean!, definitionId: ID, integrationId: ID, sourceId: ID
  sourceRevisionId: ID, sourceDigest: String, listenAddress: String, path: String
  authMode: String, tlsMode: String, provider: String, pollSeconds: Int, maxConnections: Int
  maxMessageBytes: Int, maxBodyBytes: Int, requireClientIdentity: Boolean, requireWorkloadIdentity: Boolean
  queueDriver: String, maxAttempts: Int, workerId: String
}
```

`engineRuntime` is transport-gated by `integration.operator`. It reads the
`previewRuntime` composition in `cmd/fi-fhir` (the only place that knows what
was mounted), so C-0 adds a small `internal/integration/runtime` description
type that `cmd/` fills at startup and the resolver returns; the resolver
never reads env itself.

"Configure" the engine, honestly: process-level properties are immutable
per process and the Engine tab shows each one's env key so the change is
made in GitOps; per-integration runtime policy (capacity, schedule, health,
connection validation) is part of the immutable definition revision and is
authored as a definition draft, which is outside this program (see Decisions).

### Sample intake from connections (C-2 backend, C-3 UI)

Two mechanisms, one audit table, both gated and reason-required, both
redact-only.

**Batch peek** — `peekBatchConnection(input: {connectionId: ID!, sessionId:
ID!, objectPath: String, maxObjects: Int = 10, maxMessages: Int = 5, reason:
String!}): BatchPeekResult!` (`{objects: [{path, size, version, modifiedAt}],
samples: [SessionSample!]!, capture: ConnectionCapture!}`). Without
`objectPath` it lists up to `maxObjects` objects under the input prefix and
adds nothing; with it, it streams that object through the bounded batch
reader (`batch/reader.go`) and adds the first `maxMessages` HL7v2 messages to
the session with `PHIPolicyRedact`. It takes **no lease, writes no
checkpoint, archives nothing, deletes nothing** — a kill-test proves the
batch tables are byte-identical before and after a peek and the object is
still listed by the runner afterwards. Credentials are resolved through the
same `integration.SecretResolver` the destination identity runtime uses
(env/file), from the draft's bindings; an unresolvable binding is a
`ConnectionProblem` (`SECRET_UNRESOLVABLE`) and nothing is contacted.

**Stream capture** — `startConnectionCapture(input: {sourceId: ID!, sessionId:
ID!, maxMessages: Int = 5 (≤ 100), ttlSeconds: Int = 300 (≤ 900), reason:
String!}): ConnectionCapture!`, `cancelConnectionCapture(id: ID!):
ConnectionCapture!`, `connectionCaptures(sessionId: ID!):
[ConnectionCapture!]!` (`{id, sessionId, sourceId, mode peek|stream, status
armed|complete|expired|cancelled|failed, captured: Int!, maxMessages: Int!,
expiresAt, requestedBy, reason, requestedAt, completedAt}`). A **tap**
decorates the `Processor` the MLLP server and HTTP ingress service already
call (`internal/integration/ingress/service.go:37`, the MLLP equivalent),
composed in `cmd/fi-fhir/preview_runtime.go`: after the inner `Process`
returns an accepted production result, if a capture is armed for
`request.Envelope.Metadata.SourceID`, it adds the payload to the session
(`AddSample`, redact) and increments the row. Rules: the tap never changes
the result, the ACK, or the receipt (its own errors are logged and metered,
never returned); admission never queries the database — each replica
refreshes an in-memory armed-capture cache from the captures table at most
every 2 s, so multi-replica capture is correct within 2 s and needs no
per-frame I/O; a capture completes at `maxMessages`, expires at TTL, and is
cancellable.

**Redaction for captures is a superset.** Pasted samples keep
`redactHL7v2` unchanged (behaviour pin). Captured and peeked messages go
through `redactCapturedHL7v2`, which masks every HIPAA Safe Harbor
identifier field in PID, NK1, IN1, IN2, GT1 (names, addresses, dates other
than year, phone/fax/email, SSN, MRN, account and plan identifiers; the exact
table is documented in `docs/operations/PHI-RETENTION.md` by C-2). The
sample's `source` field records provenance
(`capture:<captureId>` / `peek:<connectionId>@<digest>:<objectPath>#<n>`).

**Audit.** `integration_connection_captures` (C-0's ledger) records every
peek and capture: tenant, session, source/connection artifact id and digest,
mode, principal, reason, requested/completed/expires, counts, status; rows
are append-only except `status`/`captured`/`completed_at` advancing under an
expected-version update, and the immutability trigger idiom from
`0004_audit_immutability.sql` applies.

### Roles and capabilities

| Field group | Transport gate (`rootFieldRoles`) | Service half |
|---|---|---|
| `connections`, `connection`, `connectionRevisions`, `connectionRevision`, `engineRuntime`, `connectionCaptures` | `operatorRead` (`integration.operator`) | none |
| `createConnection`, `updateConnection`, `archiveConnection`, `compileConnection`, `validateConnectionSpec` | `operatorDeployment` (`integration.operator` + `integration.deployment.operator`) | catalog re-checks the same |
| `peekBatchConnection`, `startConnectionCapture`, `cancelConnectionCapture` | `operatorRead` + sessions enabled | tap/peek re-check `integration.operator`; reason required |

No new role: the production operator bundle already carries these
(`platform/gitops` MR 805), so the program needs no GitOps change to be
usable. Splitting an authoring role out later is a one-line change per
field. `/api/auth/status` gains role capabilities `connectionsRead`,
`connectionsWrite` (with `missingRoles` entries) and deployment capabilities
`controlPlane` and `connectionCatalog` (both true iff
`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`, which is what applies the catalog
migration); `TestAuthCapabilityRepresentativesCoverTheirGroup` covers the
new groups.

### Persistence (C-0)

New package `internal/integration/connection` with its own forward-only
ledger `integration_connection_schema_migrations` (seventh ledger: distinct
advisory lock key, exported `SchemaVersion`, `pg_advisory_xact_lock` taken
before the version read, `AGENTS.md` § Migration authoring), tables
`integration_connection_drafts` (expected-version updates only),
`integration_connection_revisions` (append-only, `UNIQUE (tenant_id,
digest)`), `integration_connection_captures`. Migrated where the other
control-plane ledgers are, under `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`.
The `migrationcompat` proof's ledger list grows from six to seven, and the
`fi_fhir_schema_ledger_version` metric and `fi-fhir version` report it.

## UI conventions the lanes follow (from the 2026-09-26 survey of `ui/`)

- **Environment.** `cd ui && npm ci && npx svelte-kit sync`; `npm run
  test:run` (vitest, jsdom, `$app` mocked from `src/test/mocks/app/`),
  `npm run lint`, `npm run lint:css` (stylelint bans hex colours in
  `.svelte`), `npm run check` (svelte-check), `npm run typecheck` (strict
  with `exactOptionalPropertyTypes`: an optional prop is declared
  `| undefined`), `npm run codegen:graphql` after any schema change (writes
  the committed `src/lib/gen/graphql.ts`; CI runs `codegen:check`). CI is
  Node 22, the host Node 20. No Prettier, no pnpm.
- **Registering a route** takes all eight places or the page silently
  becomes Home's tab: `src/routes/connections/+page.svelte` (thin wrapper
  around `$lib/features/connections/ConnectionsPage.svelte`);
  `src/lib/ui/ide/types.ts` (`IDEView`, `IDEAppRoute`);
  `src/lib/ui/ide/IDEShell.svelte` (`viewRoutes`, `routeToView`, palette
  `navCommands`); `src/lib/ui/ide/ActivityBar.svelte` (label);
  `src/lib/ui/ide/viewIcons.ts` (a Lucide icon, deep import
  `@lucide/svelte/icons/<name>`); `src/lib/ui/ide/ideStore.ts`
  (`VALID_VIEWS`, `WORKSPACE_ROUTE_TITLES`, `WORKSPACE_VIEW_ROUTES`,
  `workspaceViewForPath`); `src/lib/ui/ide/sidebar/sidebarContent.ts` and
  `Sidebar.svelte` (`SidebarView`, `viewLinks`, `contexts`,
  `getSidebarView`, `sidebarToIDEView`). Connections sits **outside the five
  stages**, like Operator (it spans Source Intake and Delivery), so
  `journey.ts` is untouched. The list-pinning tests (`sidebarContent.test.ts`,
  `ActivityBar.test.ts`, `IDEShell.test.ts`) are updated, not weakened.
- **Page root.** A full-height flex column: `Toolbar` (title, `tabs`
  snippet with `Tabs`, `actions` snippet), then a content area with
  `flex: 1 1 auto; min-height: 0` (`src/routes/events/+page.svelte` is the
  model; the document region drops its padding when a page renders a
  Toolbar). `<svelte:head><title>Connections | fi-fhir</title>`. Table +
  details is a fixed CSS grid like Operator's Messages split, with the
  details column wide enough for a two-column form; `SplitPane` is for the
  HL7 editor only.
- **Primitives only**, from `$lib/ui/primitives` (runes components: handlers
  are props such as `onclick`; `class` and `data-testid` pass through;
  named regions are snippets). `Table`/`Th`/`Td`/`Tr` (`Tr selectable
  selected onselect`, `Td mono truncate`), `Field` + `Input`/`Select`/
  `Textarea` (`Field error` replaces the hint and marks the control
  invalid), `Badge` for status (as `MessageBrowser` does), `EmptyState`
  (children allow inline `<code>`), `KeyValue` (`items` of `{key, value,
  mono}`, "—" for empty), `Popover`, `Icon`. Dialogs follow
  `features/operator/ControlReasonDialog.svelte` (40 px header, `Field`s,
  inline `submitError`, md buttons, focus from `domain/a11yDialog.ts`), never
  the old `ConfirmModal`. JSON is a read-only `CodeEditor language="json"`
  or the `<pre>` inset from `EventDetail.svelte`; `JsonViewer.svelte` is
  dead. New components are Svelte 5 runes (`$props`, `$state`, `$derived`);
  do not add old-style components.
- **Data layer.** Hand-written operation documents in
  `src/lib/graphql/connections.graphql` (queries, mutations, fragments),
  typed through codegen; one feature API module
  `features/connections/connectionsApi.ts` shaped like
  `features/operator/operatorApi.ts` (row types derived from the generated
  types, one async function per operation, `INLINE_ERRORS` reads pass
  `{ showErrorToast: false }`), and a message catalog like
  `operatorErrors.ts` mapping the inventory-safe server strings to operator
  guidance. No REST: only `/health` and `/api/auth/status` exist.
- **Capabilities.** Extend `src/lib/graphql/accessCapabilities.ts`
  (`parseAuthStatus`, the derived per-capability stores, `capabilityOf`)
  with `connectionsRead`, `connectionsWrite`, `controlPlane`,
  `connectionCatalog`; an older API body parses to "unknown", and **unknown
  never blocks** — the page tries the call and renders the failure inline.
  The pre-flight follows `OperatorPage.svelte:259-290` + `operatorAccess.ts`
  (`data-testid`, `data-missing-roles`).
- **Toast budget** (`.loom/22`). Reads render their error inline
  (`EmptyState` with Retry, a `role="alert"` note, or a `Field error`);
  mutations the user waits on pass `showSuccessToast` and show failure
  inside their dialog; an unmet precondition disables the control and puts
  the reason in its `title`; never toast what another layer shows. The e2e
  gate records every `.toast.error` and asserts none.
- **State.** Component-local `$state` for tabs, filters, and selection
  (no URL state anywhere in the IDE); shared state only as `svelte/store`
  singletons; PHI never in browser storage (samples live in tab memory:
  `features/hl7/sampleStore.ts`).
- **Honest states** as the existing surfaces do them: not configured →
  `EmptyState` naming the env key in `<code>`; missing role → `EmptyState`
  naming `missingRoles` in mono without issuing the query; stream not
  available → `StreamingUnavailable`; nothing yet → "—", never an invented
  default. No synthetic data outside the built-in HL7 sample, "Load
  examples", and the dev Gallery.
- **e2e.** Specs in `ui/e2e/*.spec.ts` per stack (`operator-bundle` :3000,
  `missing-operator-role` :3001, `sessions-off` :3002, `visual` at
  1440×900 on :3000); a new capture is a `CAPTURES` entry in
  `visual.spec.ts` (`reach` must end on an element that exists only once
  data arrived; `expectSettled` forbids `aria-busy`, "Loading…" text, and
  open requests) plus its id in `check-report.mjs`; `FORBIDDEN_COPY` lives
  only in `support.ts`. The stacks mount the `adt-http` registry, the
  operator plane, sessions, no LLM, no terminology store, and no MLLP,
  batch, HTTP ingress, or delivery worker — the Engine tab there shows one
  registry integration and four disabled adapters, which is the honest
  capture. Local run: `make ui-e2e` (docker context `7900xtx`),
  `UI_E2E_ARGS="--project visual"`, `UI_E2E_KEEP=1`.
- **Intake and sessions** (for C-3). `features/hl7/HL7PreviewPage.svelte`
  runs Preview through `features/integration-session/api.ts`: with the
  session engine on it creates a session on the first run
  (`createSession()`), adds the editor text as a sample, updates the profile
  draft, subscribes, and runs; the page keeps the session id in its
  `IntegrationSessionPreviewMeta` and reuses it. The Samples panel is
  `features/hl7/components/SampleInbox.svelte` ("Load examples" is the
  entry to sit beside), backed by `createHL7SampleStore()` in tab memory.

## Lanes

Merge order: **C-0 → C-1 and C-2 in parallel, stacked on C-0's branch → C-3
stacked on C-1 + C-2 → C-4 last.** Each lane attaches evidence (see
Evidence).

### C-0 — Connection catalog, engine runtime, capabilities (backend, first, alone)

**Outcome.** `internal/integration/connection`: draft/revision/capture types,
the per-kind checker, the HTTP source document, the PostgreSQL store with its
ledger, a `Service` with the inventory-safe error set and the role checks,
the reference projection over the lifecycle catalog, and the runtime
description type `cmd/` fills. GraphQL: the contract above, resolvers in
`internal/api/graphql/resolvers/connection_catalog.go` following
`operator_control_plane.go`, `rootFieldRoles` entries, `gqlgen` regenerated,
`capabilities.go` extended. `cmd/fi-fhir`: catalog wired under the control
plane flag, `engineRuntime` description filled from `previewRuntime`, the
version command and metric report the seventh ledger. `make
connection-catalog` proof target and `ci/test-connection-catalog.yml`
(`extends: .integration-proof`, existence guards, one `- local:` line in
`.gitlab-ci.yml`, `ci/job-inventory.txt` regenerated). **Not in scope**: the
tap, peek, any UI. **Acceptance**: PostgreSQL proof — two replicas migrate
concurrently; create → update (stale version refused) → compile → the revision
bytes decode with `mllp.DecodeSourceRevision` / `batch.DecodeSourceRevision`
/ `destination.DecodeRevision` to the same digest; a definition draft created
in the lifecycle catalog referencing the digest shows in `references`;
restart preserves everything; an `UPDATE` on a revision row is refused by
trigger; a spec carrying a value-looking key (`token`, `password`,
`secret`) is refused (`SECRET_VALUE_FORBIDDEN`); `go test ./...`,
`lint:gqlgen`, `lint:contracts`, `npm run codegen:check` green.

### C-1 — `/connections` (UI)

**Outcome.** New route `/connections` with toolbar tabs **Sources ·
Destinations · Engine**, built only from `ui/src/lib/ui/primitives/`.
Sources and Destinations: filter input, `Show archived`, `New ▾` (the kinds),
`Table` (Name · Kind · ID · Endpoint · Revision (mono, digest truncated with
title) · Status (the honest states, text not pills) · Updated) with a details
pane: tabs **Settings · Secrets · Revisions · Usage**. Settings is the
kind's form on the 2-column `Field` grid at 28 px, generated from the spec
schema above (numbers as `Input type=number` with the documented bounds as
`min`/`max`, enums as `Select`, lists as one-per-line `Textarea`), inline
`ConnectionProblem`s by path after `validateConnectionSpec` (debounced) and
after a failed compile; Save and Compile open the reason dialog restyled
from Operator; Secrets is a table of bindings (name · provider · key ·
version) with add/remove and **no value column, ever**; Revisions lists
revisions with a "Copy JSON" and "Download" (`<id>-r<N>.json`) — the exact
`revisionJson`; Usage lists `references` and the runtime state. Engine:
`KeyValue` groups (Identity and access · Control plane · Registry) and one
`Panel` per adapter (HTTP ingress · MLLP listener · Batch runner · Delivery
worker · Destination identity) with the env key beside each property, then
Ledgers and Properties tables; secrets render `set`/`unset`. Honest states,
in this precedence: `capabilities.controlPlane === false` → `EmptyState`
"The connection catalog is not configured on this deployment" with the env
key in mono; `connectionsRead === false` → `EmptyState` naming
`missingRoles.connectionsRead` in mono; `connectionsWrite === false` → forms
read-only with one status line. Navigation: an activity-bar entry and stage
placement per the UI conventions section; the command palette lists "New
source connection", "New destination connection", "Engine properties".
`data-testid`s: `connections-preflight`, `connections-table`,
`connection-form`, `connection-problems`, `connection-secrets`,
`connection-revisions`, `engine-runtime`. **Acceptance**: vitest for the
form generator (every kind renders every field; a problem lands on its
field) and the status derivation (table-driven over the five states);
`test:ui-e2e` operator-bundle spec: create an MLLP source → compile → the
digest is shown and `Download` yields bytes equal to `revisionJson`; the
Engine tab shows four adapter rows with honest enabled/disabled states; the
missing-operator-role stack shows `connections-preflight`; `visual` project
captures the three tabs; screenshots attached.

### C-2 — Sample intake backend: peek and capture

**Outcome.** The tap (`internal/integration/connection/capture.go`, composed
in `cmd/fi-fhir/preview_runtime.go` around the MLLP and HTTP processors), the
armed-capture cache with its 2 s refresh, `peekBatchConnection` over a
provider built from a catalog revision's bindings, `redactCapturedHL7v2`
with its documented field table, the GraphQL fields and `rootFieldRoles`
entries, Prometheus counters (`fi_fhir_connection_capture_messages_total`,
`fi_fhir_connection_capture_tap_errors_total`). **Kill-test**
(`make connection-capture`, `ci/test-connection-capture.yml`): with a capture
armed for `adt-east` at `maxMessages: 2`, three frames over the MLLP
integration harness produce 2 session samples whose text contains no PID-5
name, NK1-2 name, or IN1-16 insured name from the fixture while all 3
receipts are durable and ACKed; a capture armed for another source id
captures nothing (negative control); with the session store closed, the ACK
and receipt are unchanged and the tap error counter increments; after
`ttlSeconds`, the row is `expired` and the tap is inert. Peek proof on the
MinIO harness from `test:batch`: peek lists and reads, batch tables are
byte-identical, no archive object exists, the runner processes the object
afterwards. **Not in scope**: UI.

### C-3 — Sample intake UI

**Outcome.** On `/hl7` with the session engine on, the samples panel gains
**From connection…**: a dialog listing sources from `engineRuntime.adapters`
(enabled, with their mounted digest) and catalog sources, each with its
state; a batch source opens the object list (`peekBatchConnection` without
`objectPath`) → choose → messages `N` → the samples appear; a stream source
arms a capture (`N`, TTL, reason) and the panel shows a capture row
("2 / 5 captured · expires in 4:12 · Cancel") polling `sessionSamples` and
`connectionCaptures` every 2 s while a capture is armed and stopping when it
is not. The dialog captures into the page's current session, creating one
through the exported `createSession()` when no Preview has run yet, and
pulls `sessionSamples` into the tab-memory inbox with their `sampleId` and
provenance. A sample that came from the session runs Preview by its
`sampleId` (`RunSessionPreviewInput.sampleId`) instead of being re-added
as a new sample; the editor path is unchanged. Captured samples open in
the editor like pasted ones. Honest states:
no session engine → the entry is absent; no enabled source and no catalog
source → `EmptyState` "No source connection is mounted on this deployment";
capability missing → the reason in mono. **Acceptance**: vitest on the
dialog's state derivation; `test:ui-e2e` operator-bundle spec drives the
dialog to its honest empty state (the e2e stack mounts no MLLP or batch
source) and asserts the entry is absent on the sessions-off stack;
screenshots.

### C-4 — Gate, docs, close-out (last)

**Outcome.** `docs/user-guide/connections.md` (the journey: define → compile
→ export or reference → mount in GitOps → see it mounted; sample from a
connection; what is and is not live-reloaded), `docs/operations/
CONNECTION-CATALOG.md` (ledger, roles, capture audit, the redaction table
pointer, the `engineRuntime` allowlist and how to add a property), one line
in `docs/operations/INTEGRATION-DEPLOYMENT-LIFECYCLE.md` replacing "Lifecycle
controls are not yet exposed", `ROADMAP.md` (a Delivered block for this
program and the "Now" items it closes), one `changelog.d/` fragment per lane
if the lanes did not add theirs, the decision entry (`make decisions-new`)
and the worklog entry (`make worklog-new`), and the `visual` copy register
extended if any lane introduced copy to retire.

## Evidence every lane attaches

Before/after PNGs at 1440×900 for each route the MR touches (C-1, C-3), taken
with Playwright against a local stack (`bin/fi-fhir serve` with
`FI_FHIR_GRAPHQL_TRUSTED_CIDRS=127.0.0.1/32`, the operator bundle in
`FI_FHIR_GRAPHQL_ROLES`, `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true`, a
Postgres on the 7900xtx docker context, sessions on; or `make ui-e2e
UI_E2E_KEEP=1` and `npx playwright screenshot` inside the kept container),
uploaded with `POST /projects/19/uploads` and embedded in the MR description.
Backend lanes attach the proof job's log tail and the negative control's
failing line. The coordinator reviews screenshots first, then the diff.

## Shared lane policy (verbatim in every prompt)

- Branch from `origin/main` for C-0; C-1 and C-2 branch from `origin/main`
  and merge C-0's branch with `--no-ff` until C-0 merges, then
  `rebase --onto origin/main <stacking-merge>`; C-3 branches from C-1's
  branch and merges C-2's; C-4 branches from main after C-1..C-3 merge.
  Names: `feat/connections-0-catalog`, `feat/connections-1-ui`,
  `feat/connections-2-intake-api`, `feat/connections-3-intake-ui`,
  `docs/connections-4-close-out`.
- **Review gate**: open the MR, write `State: REVIEW-READY` with the iid to
  your status file, **do not arm auto-merge**; never push after
  REVIEW-READY unless the coordinator sends findings.
- **Git and API from this LAN**: push with
  `git -c http.sslVerify=false -c http.extraHeader="Host: gitlab.flexinfer.ai" push https://oauth2:${GITLAB_PAT}@192.168.50.227/libs/fi-fhir.git <branch>`
  (plain `git push origin` also works for small pushes); API with
  `curl --resolve gitlab.flexinfer.ai:443:192.168.50.227 -H "PRIVATE-TOKEN: $GITLAB_PAT"`;
  print `http=%{http_code}` on every write. Never the `gitlab` MCP tools.
  Never print the token.
- **Per-lane scratch directory** `<scratchpad>/<lane>/`; status file
  `status.md` with `## State` / `## MR` / `## Pipeline` / `## Notes`.
- **Node**: the host has no `ui/node_modules`; `npm ci` (and `npm install`
  for a new dependency) inside YOUR worktree's `ui/` only; `npx`, never
  pnpm; `npx svelte-kit sync` before vitest.
- **Go**: `go build ./... && go test ./...` before every push; the
  PostgreSQL proofs run against a Postgres on `--context 7900xtx`
  (`AGENTS.md` § Integration tests gives the recipe; connect via
  `cblevins-7900xtx:PORT`); never set `GOMODCACHE` under `.tmp/`.
- **Migrations**: `AGENTS.md` § Migration authoring, all three rules; the
  seventh ledger's lock key is new and distinct; re-verify numbers against
  `origin/main` at every rebase.
- **CI layout**: a new proof is `ci/test-<name>.yml` plus one `- local:`
  line and a regenerated `ci/job-inventory.txt`; never append a job to the
  root file; one `.PHONY` line per lane in the Makefile.
- **Pipelines**: poll ≤ every 180 s; retry a job once only for a known
  flake (`lint:ui` heap, `build:docker-ui` BuildKit "context canceled",
  "Getting source" curl 56 resets, `test:observability-replicas`,
  `security:trivy-image` DB drift); never cancel-retry `lint:gqlgen`; after
  two failures of different jobs or 3 h on one pipeline, STOP and write
  status. `test:ui-e2e` failures are yours to read (artifacts); restyling
  must not change what the honest surfaces assert.
- **Never touch** `CHANGELOG.md`, `ROADMAP.md`, `.loom/30-*.md`,
  `.loom/50-worklog.md`, `.loom/40-decisions.md`, `platform/gitops`,
  `ci/_shared.yml`, or another lane's files (C-4 excepted for ROADMAP,
  CHANGELOG fragments, decision and worklog entries). Worklog via
  `make worklog-new TITLE="..."`; decisions via `make decisions-new`.
- **zsh**: unquoted `$VAR` does not word-split; `noclobber` (`>|`);
  `mv`/`rm`/`cp` are interactive (`command mv -f`); python f-strings cannot
  contain backslashes.
- **PHI/secrets**: synthetic samples only; no production credential
  anywhere; no secret value in any struct, log, fixture, screenshot, or MR.
- Commit messages conventional, scoped, trailer
  `Co-Authored-By: <the model you are> <noreply@anthropic.com>`.

## Decisions taken by the coordinator

1. **A durable catalog of drafts and immutable revisions, not hot-loading.**
   The runtime keeps mounting content-addressed documents at startup and the
   lifecycle catalog keeps governing what runs; the catalog makes the same
   documents authorable and exportable, and labels them against the mounted
   digest. Hot-loading listeners and credentials is a data-plane change with
   its own safety story and is not in this program.
2. **Compile with the existing constructors.** A catalog revision's bytes
   are produced by `mllp.NewSourceRevision`, `batch.NewSourceRevision`, and
   `destination.NewRevision`, so a compiled MLLP source is bit-identical to
   what an operator would have hand-written, and its digest is the one the
   lifecycle catalog validates. HTTP gets a document of the same idiom
   because a source with no bytes behind its digest is the gap this program
   closes.
3. **Secrets stay references end to end.** The catalog stores binding names
   and references only; the GraphQL surface has no field that could carry a
   value; a spec key that looks like a value is refused.
4. **No new role.** Reads ride `integration.operator`, writes ride
   `integration.deployment.operator`; the production bundle already holds
   both. `controlPlane` and `connectionCatalog` become capabilities so the
   page can say "not configured" instead of "forbidden".
5. **The integration definition editor is a follow-up.** Binding a source,
   profile, workflow, and destinations into a deployable definition draft
   needs profile and workflow bytes the runtime today loads only from the
   static registry; doing it honestly means also moving artifact resolution
   onto the lifecycle catalog. This program ships the connections and the
   references view; the definition editor is the next slice and is recorded
   in ROADMAP "Then".
6. **Capture is a tap after admission, redact-only, superset redaction.**
   The tap never touches the ACK or the receipt, never queries the database
   per frame, and stores only a copy masked more aggressively than pasted
   samples, because a live feed carries PHI an engineer's synthetic sample
   does not. Every peek and capture is a reason-required, audited row.
7. **Evidence over pixel tests**, unchanged from `.loom/37`: screenshots are
   review artifacts; the gate asserts honest `data-testid`s and the copy
   register.
