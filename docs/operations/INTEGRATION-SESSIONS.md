# Restart-Safe Integration Sessions

Phase 3 Slices 3.1 through 3.4 add an opt-in PostgreSQL workspace for Integration Sessions.
Sessions, redacted samples, append-only artifact revisions, immutable terminal
runs, accepted decisions, and export records survive a backend restart. Preview
runs record the exact profile revision ID and SHA-256 digest they executed, and
authenticated server-sent events (SSE) expose live stage, diagnostic, and
lineage snapshots to Mapping Studio. Workflow Builder can bind the current
workflow draft to those immutable runs and persist a side-effect-free route,
transform, and action plan for one exact workflow revision.

Signed publication can advance an existing validated lifecycle definition, but
it does not create artifacts, validate connections, mutate Kubernetes/GitOps, or
execute workflow actions. Multi-replica stream fanout and GitOps activation
remain separate operations work. GraphQL WebSocket transport stays closed.

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

The UI has its own public build flag, `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED`.
The UI image (`ui/Dockerfile`) is built with it on; a dev server needs it
explicitly:

```bash
VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true npm --prefix ui run dev
```

The build flag only permits the session engine. The UI uses it when the API's
`GET /api/auth/status` also reports `capabilities.integrationSessions: true`
(`resolveIntegrationSessionEngine` in
`ui/src/lib/features/integration-session/api.ts`). Against an API with the
workspace off, HL7 intake keeps the stateless `previewIntegrationMessage` path,
Dry Run does not offer the **Session** source, and the session surfaces show
"Live streaming for Integration Session runs is not available on this
deployment" instead of an error. The API environment is the switch; a UI built
with `=false` never offers the engine.

Startup opens PostgreSQL, takes a migration advisory lock, applies the session
schema once, and wires the GraphQL session routes to the durable store. Startup
fails closed when the database or migration is unavailable.

The authenticated GraphQL server still requires the existing deployment tenant,
origin, and bearer configuration. Session operations require the
`graphql:operator` compatibility grant; the narrower `integration:preview` role
remains limited to the typed stateless preview operation. Local operators
enabling the session workspace must include `graphql:operator` in
`FI_FHIR_GRAPHQL_ROLES`.

Sprint 4 narrowed the GraphQL transport gate from a blanket allow to a
per-root-field role map, but the session workspace is not yet part of the
narrowed surface: all nine session queries, all twelve session mutations, and
both session subscriptions are still in the compatibility bucket. Nothing about
this configuration changed — `graphql:operator` continues to expand to every
root field. See [`docs/planning/GRAPHQL-API.md`](../planning/GRAPHQL-API.md) for
the roles that now gate the operator control plane, and for the follow-up that
will give the session workspace its own grant.

### Production

`fi-fhir.flexinfer.ai` runs the UI image with the build flag on, so the
workspace is turned on by one API environment entry. In
`platform/gitops/k3s/fi-fhir/fi-fhir-api.yaml`, add it beside the other
`FI_FHIR_*` feature switches (after `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`):

```yaml
            - name: FI_FHIR_INTEGRATION_SESSION_ENABLED
              value: "true"
```

Nothing else changes: no new secret, database, or UI rebuild.

**Database.** The session store uses the durable connection that
`openSubmissionDatabaseFromEnv` (`cmd/fi-fhir/preview_runtime.go`) already opens
for the operator control plane. It is built from `FI_FHIR_DATABASE_HOST`,
`_PORT`, `_NAME`, `_USER` (or `_USERNAME`), `_PASSWORD` and `_SSL_MODE`, which
in production point at `postgres.fi-fhir.svc` and the `fi-fhir-postgres`
secret. It does not read `FI_FHIR_DATABASE_URL`, which is the terminology
store's connection string (the same server in production). This connection
defaults to `sslmode=require`, unlike the legacy profile, event and workflow
stores, which fall back to `disable` when `FI_FHIR_DATABASE_SSL_MODE` is unset.
The in-cluster PostgreSQL (`k3s/fi-fhir/postgres.yaml`) is not configured for
TLS, so the Deployment must keep
`FI_FHIR_DATABASE_SSL_MODE=disable`. Production already sets it. A deployment
that enables sessions as its first durable feature must set it too, or startup
fails at the database ping.

**Migrations.** On the first start with the flag on,
`integrationsession.PostgresStore.Migrate` takes a transaction-scoped advisory
lock and applies the session ledger (`integration_session_schema_migrations`,
versions 1–7, from `internal/integration/session/migrations/`):
`0001_session_workspace`, `0002_workflow_simulations`, `0003_publications`,
`0004_export_attribution`, `0005_session_stream_events`,
`0006_retention_expiry`, `0007_export_attribution_defaults`. They create and
alter only `integration_session_*` tables. The submission, lifecycle and
destination migrations already ran for the operator control plane, and
sessions do not add to them. With no signing keys set, publish, approve and
deploy stay unavailable. With no retention key set, samples are stored redacted
and explicit raw retention is refused.

**Roles.** Session operations and both session subscriptions pass the
transport gate through `graphql:operator`. Every production identity (the
static bearer, the trusted network, both Access principals) already holds it.

**Verify** after Flux rolls the Deployment (from the LAN, where the trusted
network authenticates `curl` without a token):

```bash
kubectl -n fi-fhir rollout status deployment/fi-fhir-api
#   a stalled rollout leaves the old pod serving; the new pod's log names the
#   failure ("configure durable integration database" or
#   "migrate Integration Session store")

curl -s https://fi-fhir.flexinfer.ai/api/auth/status | python3 -m json.tool
#   "integrationSessions": true,
#   "streaming": true,
#   "subscriptions": ["integrationSessionEvents", "sessionRunEvents"]

curl -sS -N --max-time 5 -o /dev/null \
  -w 'http=%{http_code} type=%{content_type}\n' \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -H 'Origin: https://fi-fhir.flexinfer.ai' \
  --data '{"query":"subscription Probe { integrationSessionEvents(sessionId: \"probe\") { id type } }"}' \
  https://fi-fhir.flexinfer.ai/graphql
#   http=200 type=text/event-stream   (before the flip: http=404)
```

The SSE probe names a session that does not exist. It passes the transport and
the allowlist, the resolver's read-only lookup answers not-found as an event,
and the stream completes without creating anything. In the IDE, reload the page. HL7
intake's **Preview** now shows the Integration Session run progress in place of
the "not available" note, and Workflow Builder's Dry Run offers the **Session**
source. Events → Live Stream, Workflow Monitor, Debug and Runtime Output still
show the honest unavailable state, because their subscriptions are not on the
SSE allowlist (see [RUNBOOK](RUNBOOK.md#live-streaming-is-unavailable)).

**Roll back** by removing the `FI_FHIR_INTEGRATION_SESSION_ENABLED` entry and
letting Flux roll the Deployment
(see [Rollback](#rollback) for the tables). `/api/auth/status` then reports
`integrationSessions: false`, `streaming: false` and `subscriptions: []`. The
same UI image falls back to the stateless preview path because the capability
went false. HL7 intake previews through `previewIntegrationMessage`, the Dry
Run **Session** source disappears, and the session surfaces show the honest
state. No UI rebuild or image change is needed. Reload any IDE tab that was
open during the rollback. A tab reads its capabilities once, when it loads.
Until it reloads, it keeps choosing the session engine, and its **Preview**
fails with `legacy integration execution is unavailable`.

Signed publication is separately disabled unless all three key settings are
present:

```bash
export FI_FHIR_INTEGRATION_SESSION_SIGNING_KEY_ID=release-key-2026-07
export FI_FHIR_INTEGRATION_SESSION_SIGNING_KEY_FILE=/var/run/secrets/fi-fhir/session-signing-key.pem
export FI_FHIR_INTEGRATION_SESSION_TRUST_ROOT_FILE=/var/run/secrets/fi-fhir/session-signing-public.pem
```

The private key must be one PEM PKCS#8 Ed25519 key and the trust root must be
its matching PEM PKIX public key. Partial or mismatched configuration fails
startup. When all settings are absent, authoring and simulation remain available
but publish/approve/deploy return unavailable.

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

The planner reports transforms as `planned`; the session path does not claim
transform execution semantics.

## Signed publication and deployment

After a successful simulation, Workflow Builder can target one exact production
definition revision that is already in lifecycle state `validated`. Publication:

1. reloads the selected session profile, workflow simulation, ordered successful
   runs, and redacted-policy samples;
2. loads the immutable production definition and resolves its exact profile and
   workflow bytes through the production artifact loader;
3. recomputes production-domain references from the tested session bytes and
   rejects any content mismatch;
4. creates a canonical manifest containing fixture digests and bounded expected
   event/diagnostic/route/transform/action identities, never payloads or action
   configuration; and
5. appends the manifest and detached Ed25519 signature as a versioned publication.

Approval verifies the signature against the configured trust root before calling
the existing lifecycle `Approve` transition. Deployment verifies it again,
creates the immutable lifecycle release from `approved`, and advances that exact
definition revision to `deployed`. Expected snapshot versions are mandatory. A
retry from `published` safely resumes at deploy; no current profile/workflow
pointer is consulted.

Publication rejects samples explicitly marked for raw retention. It does not
copy session artifacts into production stores and performs no network,
destination, transform, action, or GitOps call.

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
- Publication and workflow-simulation rows reject update/delete; exact manifest
  bytes and detached signatures survive restart and are included in exports.
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
restores both simulations and a signed publication, compares the expected route/action delta, and proves
that raw-PHI, action-config, and filesystem-side-effect sentinels are absent.

The normal GraphQL and UI suites additionally prove stream-before-run ordering,
exact revision/digest reconciliation, canonical repeated-OBX lineage, operation
authorization, raw-preview exclusion, diagnostic deduplication, and inspector
navigation. Workflow Builder tests additionally prove that durable simulation
sends only revision/run identities, renders server trace provenance/deltas, and
gates signed publish/approve/deploy on exact immutable identities.

## Rollback

Unset `FI_FHIR_INTEGRATION_SESSION_ENABLED` and restart the API. Other ingestion,
delivery, and preview paths do not depend on the session tables. Do not drop the
tables during an incident; preserve them for recovery and audit. Schema removal
requires a separately reviewed data-retention operation.
