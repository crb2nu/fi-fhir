# Restart-Safe Integration Sessions

Phase 3 Slices 3.1 through 3.3 add an opt-in PostgreSQL workspace for Integration Sessions.
Sessions, redacted samples, append-only artifact revisions, immutable terminal
runs, accepted decisions, and export records survive a backend restart. Preview
runs record the exact profile revision ID and SHA-256 digest they executed, and
authenticated server-sent events (SSE) expose live stage, diagnostic, and
lineage snapshots to Mapping Studio. Workflow Builder can bind the current
workflow draft to those immutable runs and persist a side-effect-free route,
transform, and action plan for one exact workflow revision.

This is an author/test foundation, not a production deployment control plane.
Bundle publication, multi-replica stream fanout, and GitOps activation remain
separate slices. GraphQL WebSocket transport stays closed.

## Enable the workspace

The workspace is disabled unless `fi-fhir serve` receives:

```bash
export FI_FHIR_INTEGRATION_SESSION_ENABLED=true
export FI_FHIR_DATABASE_DRIVER=postgres
export FI_FHIR_DATABASE_HOST=postgres
export FI_FHIR_DATABASE_NAME=fi_fhir
export FI_FHIR_DATABASE_USERNAME=fi_fhir
export FI_FHIR_DATABASE_PASSWORD='use-a-secret-provider'
export FI_FHIR_DATABASE_SSL_MODE=verify-full
```

Build or start the UI with its separate public feature gate:

```bash
VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true npm --prefix ui run dev
```

Startup opens PostgreSQL, takes a migration advisory lock, applies the session
schema once, and wires the GraphQL session routes to the durable store. Startup
fails closed when the database or migration is unavailable.

The authenticated GraphQL server still requires the existing deployment tenant,
origin, and bearer configuration. Until fine-grained Phase 4 authorization is
implemented, session operations require the temporary `graphql:operator` role;
the narrower `integration:preview` role remains limited to the typed stateless
preview operation. Local operators enabling the session workspace must include
`graphql:operator` in `FI_FHIR_GRAPHQL_ROLES`.

Production GitOps does not enable either feature gate in Slice 3.3.

## Streaming diagnostics and lineage

When both feature gates are enabled, Mapping Studio creates or reuses a durable
session, adds a redacted sample, saves the current executable profile revision,
and opens the `integrationSessionEvents` subscription before starting the run.
The UI renders connecting/running/complete/error states and reconciles streamed
progress with the immutable terminal run returned by the mutation.

The stream is an authenticated `POST /graphql` request with
`Accept: text/event-stream`. It retains the existing request body, origin,
tenant, bearer-token, timeout, depth, and complexity checks. A transport-level
allowlist permits only `integrationSessionEvents` and `sessionRunEvents` on SSE;
legacy subscriptions and mutations fail closed even for `graphql:operator`.
`/graphql/ws` remains a 404.

Run snapshots include canonical source paths such as `PID-5`, `OBX[0]-3`, and
`OBX[1]-5`. Problems-panel diagnostics are deduplicated by run and diagnostic
identity, and selecting a diagnostic or lineage link focuses that exact field
in the HL7 inspector. Raw retained samples and persisted lineage value previews
do not cross the GraphQL stream boundary.

Fanout is process-local in this slice. The terminal mutation response remains
the reconciliation source if an intermediate stage event is missed. Durable
cross-replica fanout/replay is Phase 4 work.

## Workflow draft simulation

With both feature gates enabled, Workflow Builder exposes a **Session** event
source in Dry Run Simulation. A run performs this server-owned sequence:

1. append the current YAML as a new `workflow_draft` artifact revision;
2. select the active session's explicit successful run IDs;
3. load canonical event payloads from those immutable runs on the server;
4. evaluate the production pure route planner against the exact workflow
   revision; and
5. persist and render revision provenance plus event, route, planned-transform,
   and action identity traces.

The browser sends session, workflow revision, source run, and optional baseline
simulation IDs. It does not send event JSON on this path. The server never calls
transform or action handlers, resolves destinations, or performs terminology,
LLM, network, database, queue, file, or process side effects. Simulation records
omit event payloads, raw samples, transformed values, action configuration, and
secrets.

Running another draft over the same ordered run set automatically compares it
with the latest prior simulation. Added and removed event, matched-route,
transform, and action keys are sorted deterministically. Simulations and their
exact workflow revision ID/digest survive a backend restart and are included in
session exports. YAML and JSON artifact bodies are stored as exact opaque bytes
so both formats round-trip through an export snapshot.

The planner reports transforms as `planned`; Slice 3.3 does not claim transform
execution semantics. Publication, signing, approval, or deployment of a tested
revision remains Slice 3.4 work.

## Raw sample policy

Durable samples default to `redact`. For HL7v2 this replaces selected
identifier, name, birth date, address, phone, and SSN fields in PID before the
record reaches PostgreSQL. Other formats are stored as a redaction marker until
format-specific redactors are implemented.

Explicit raw retention additionally requires an AES-256 key file:

```bash
install -m 0400 /secure/generated/session-aes-key \
  /var/run/secrets/fi-fhir-session/aes-256-key
export FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE=\
/var/run/secrets/fi-fhir-session/aes-256-key
```

The file must contain exactly 32 binary bytes. Retained payloads are encrypted
with AES-256-GCM using a random nonce and tenant/session/sample identity as
authenticated additional data. The key is never stored in PostgreSQL or session
exports. Exports omit explicitly retained raw bytes by default even when the
caller requests a session containing them.

Back up the key through the deployment's secret-management process whenever
retained samples exist. Losing it makes those samples intentionally unreadable.
Key rotation and retention expiry are Phase 4 work; prefer redacted samples.

## Immutability and replay

- Each artifact save appends a revision with a stable artifact ID, unique
  revision ID, increasing version, and content digest.
- A run loads one exact mapping-profile revision, verifies its digest, and uses
  the same profile compiler as the production processor.
- A workflow simulation loads one exact workflow-draft revision, verifies its
  digest, and uses the production pure planner over explicit immutable runs.
- Successful and failed terminal runs cannot be changed through the store.
- Workflow simulation records are append-only and configuration-free.
- Accepted diagnostic decisions and exports are separate durable audit records.
- Archive is a state transition. Archived sessions are hidden from the default
  list but remain reopenable by stable ID and visible when explicitly requested.

## Verification

The required restart proof uses PostgreSQL 16:

```bash
make integration-session
```

Locally, the target starts PostgreSQL with testcontainers when
`POSTGRES_TEST_URL` is unset. In CI it requires the supplied PostgreSQL service.
The test reconstructs every store/runner object, reopens the session, executes
one redacted sample against strict and tolerant profile revisions, checks the
warning/event delta and exact provenance, and scans session records for its raw
PHI sentinel. It also proves encrypted explicit retention, terminal-run
immutability, durable decisions/exports, archive/list/reopen behavior, and two
workflow revisions over the same run. After another store reconstruction it
restores both simulations, compares the expected route/action delta, and proves
that raw-PHI, action-config, and filesystem-side-effect sentinels are absent.

The normal GraphQL and UI suites additionally prove stream-before-run ordering,
exact revision/digest reconciliation, canonical repeated-OBX lineage, operation
authorization, raw-preview exclusion, diagnostic deduplication, and inspector
navigation. Workflow Builder tests additionally prove that durable simulation
sends only revision/run identities and renders server trace provenance/deltas.

## Rollback

Unset `FI_FHIR_INTEGRATION_SESSION_ENABLED` and restart the API. Other ingestion,
delivery, and preview paths do not depend on the session tables. Do not drop the
tables during an incident; preserve them for recovery and audit. Schema removal
requires a separately reviewed data-retention operation.
