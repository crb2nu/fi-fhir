# Product Spec: fi-fhir Integration Engine + IDE Completion

**Status**: Active completion contract
**Date**: 2026-07-12
**Project**: `libs/fi-fhir`
**Canonical roadmap**: `ROADMAP.md`
**Plan store namespace**: `fi-fhir/integration-engine-ide-completion`
**Plan ID**: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98`

## Outcome

Finish fi-fhir as a production healthcare integration engine with an integrated
authoring, testing, deployment, and operations IDE. The finished product must
let an implementation engineer take a feed from connection setup through a
versioned, observable deployment without leaving the product or relying on a
different execution path than production.

The repository already contains a strong capability kernel: profile-driven
parsers, canonical events, workflow routing/actions, FHIR mapping, terminology,
event-sourcing primitives, GraphQL, and a substantial SvelteKit Mapping Studio.
Completion is primarily an assembly and lifecycle problem, not a parser rewrite.

## Product truth at the start of this program

### Working kernel

- HL7v2, CSV, EDI, CDA/CCDA, and FHIR parsing/mapping packages are implemented.
- Source Profiles can control the HL7v2 parser directly.
- The workflow engine performs CEL filtering, transforms, actions, retries,
  circuit breaking, replay, simulation, DLQ logic, metrics, and tracing.
- GraphQL and the Mapping Studio provide real profile, workflow, terminology,
  event, debug, and LLM-backed authoring surfaces.
- PostgreSQL event/profile/workflow stores and broader event-sourcing building
  blocks exist.
- The June Integration Session Engine is merged and tested.

### Missing product assembly

- No headless feed runtime composes secure ingress, profile selection, parsing,
  durable receipt/idempotency, persistence, workflow delivery, and traceability.
- `serve` loads at most one workflow and does not manage deployable integration
  definitions or connection lifecycles.
- Generic webhook ingest wraps JSON instead of parsing healthcare payloads;
  S3/SFTP polling is not registered into the production runtime; no MLLP source
  exists.
- Integration Sessions are in-memory, HL7-only, ignore the selected profile and
  workflow drafts, and disappear on restart.
- The session UI is feature-flagged off in shipped builds and does not call its
  subscription helper.
- Deployment, readiness, metrics, auth/RBAC, PHI policy, and CI claims are not
  consistently truthful.

## Users

- **Integration engineers**: connect sources/destinations, author profiles and
  workflows, test samples, publish versions, and diagnose failures.
- **Clinical/terminology SMEs**: review warnings, mappings, and governed fixes
  with rationale and audit history.
- **Operators**: monitor channels, search traces, replay/resubmit messages,
  manage DLQs, and prove delivery.
- **Developers/platform teams**: extend adapters and actions through stable
  contracts, test locally, and ship reproducible releases.
- **Agents/copilots**: operate on the same versioned artifacts, diagnostics, and
  session context as humans without bypassing policy or audit controls.

## Foundation contracts that precede durable schemas

- **Isolation**: 1.0 supports one healthcare-organization security domain per
  deployment. A required logical tenant/security-domain identifier still scopes
  every durable record and trace; shared multi-tenant hosting is not a 1.0 claim.
- **Identity**: human actions carry authenticated actor, tenant, roles, and reason;
  adapters carry service identity, source identity, tenant, and auth method. No
  ingress or IDE mutation creates an anonymous production record.
- **Secrets**: integration artifacts contain typed secret references only. Secret
  values never appear in revisions, exports, logs, diagnostics, or GraphQL output.
- **Data classification**: canonical events, identifiers, diagnostics, samples,
  receipts, and traces are PHI-capable. Raw payload is ephemeral by default;
  retention requires an encrypted store, TTL, purpose, actor, and access audit.
- **Execution modes**: production mode may persist and deliver; preview mode uses
  the same parsing/routing semantics but never persists or delivers. Sandbox
  destinations may be selected for planning and simulation, but an executed
  sandbox action requires a separately audited production request.
- **Collaboration**: 1.0 requires optimistic concurrency, immutable revisions, and
  explicit conflict resolution. Real-time character-level co-editing is not a
  1.0 requirement.

## Riskiest assumption + kill-test

**Load-bearing assumption**: The existing parser, Source Profile, workflow, and
event-store primitives can be composed behind one shared `MessageProcessor`
application service that serves both headless production adapters and IDE
preview/debug flows without duplicating parsing or routing semantics.

**Kill-test recipe**: Golden Path 001 must add
`testdata/golden/integration/adt-http/` with `input.hl7`, strict and tolerant
profile revisions, one workflow revision, and `expected.json`, plus a single
`make golden-path-001` entry point. From a clean checkout, that command owns
Compose build/start, migrations, artifact publication, execution, restart, and
cleanup and must finish within 30 wall-clock minutes. It sends the same
authenticated ADT A01 twice with one idempotency key and verifies:

1. the selected Source Profile changes at least one parse outcome compared with
   the default profile;
2. both requests return the same durable receipt;
3. one canonical event and one downstream delivery are recorded;
4. warnings, profile revision, workflow revision, route/action results, and
   correlation identifiers are queryable after a process restart; and
5. Integration Session preview uses the same artifact revisions and produces a
   semantically equivalent canonical event and diagnostics without a production
   delivery.

Phase 3 Slices 3.1 through 3.3 implement the restart-safe workspace and workflow
simulation foundation: PostgreSQL
persists redacted samples, append-only artifact revisions, immutable terminal
runs, accepted decisions, and exports; each profile-aware run records the exact
revision ID and digest executed by the shared production profile compiler.
Authenticated streaming projects server diagnostics and canonical lineage, while
Workflow Builder plans explicit durable run events against one exact workflow
revision and persists configuration-free route/transform/action traces and
deterministic deltas. Publish/deploy remains a subsequent slice.

Semantic equivalence ignores generated timestamps/transport IDs but requires the
same event type, business payload, warning/error codes and paths, profile/workflow
digests, and normalized lineage. The command writes machine-readable assertions,
JUnit, logs, and receipt/trace exports under `.tmp/golden-path-001/`. Any
duplicate durable receipt/event/outbox record, unexpected second delivery in the
no-failure test, missing post-restart evidence, revision mismatch, preview side
effect, or default/profile-identical result is disconfirming evidence and fails
the gate.

**Failure mode if the assumption is wrong**: Continuing would create parallel
"preview" and "production" engines whose behavior drifts. The program must stop
and redesign the runtime boundary before MLLP, connector expansion, or deeper IDE
work ships.

**Status**: passed. MR `!99` pipeline `18898` job `182088` and merge-commit
pipeline `18951` job `182694` each passed all 20 duplicate/restart/profile/IDE
parity and leakage assertions. The riskiest assumption is confirmed for Slice
1.3; production channel expansion may proceed without creating a parallel
processor.

## Definition of complete

### 1. Secure production data plane

- Versioned `IntegrationDefinition` binds source connection, format, Source
  Profile revision, workflow revision, destinations, secrets, and policies.
- A shared processor performs normalize -> parse -> canonicalize -> persist ->
  route -> deliver with one trace/receipt model.
- At minimum, production-ready HL7v2 MLLP and authenticated raw HTTP ingress are
  shipped; S3/SFTP batch ingestion is registered and completes parsing rather
  than stopping at file discovery.
- At minimum, webhook, FHIR, database, file, and one real queue transport are
  production-wired destinations.
- Idempotency, bounded retries, outbox delivery, DLQ, replay, and resubmit have
  documented semantics and integration tests.
- Acceptance is durable once. Downstream delivery is at-least-once; duplicate
  suppression and downstream idempotency are required where the destination
  protocol supports them. The product does not claim universal exactly-once I/O.
- ACK/NACK and HTTP responses reflect durable acceptance, not merely receipt in
  process memory.

Slice 2.1 implements the versioned backend lifecycle portion of this section:
digest-bound deployment policy, append-only validation/release/history records,
optimistic deployment state, and exact deployed-only revision resolution.
MR `!101` pipeline `19014` passed all 32 jobs, including required PostgreSQL 16
lifecycle job `183463`; final main pipeline `19052` passed all 26 jobs with
durable-submission job `183938` and lifecycle job `183940` independently green.
Slice 2.2 implements the production MLLP portion with UTF-8 byte framing,
TLS/client/capacity bounds, lifecycle-catalog selection, transaction-scoped
deployment authorization, and positive ACK only after durable admission.
MR `!104` pipeline `19175` passed all 33 jobs, including required PostgreSQL
16/TCP MLLP job `184996`; merge commit `6205fa39` repeated the proof in main job
`185093`, and main pipeline `19193` passed all 36 jobs.
Slice 2.3 implements the backend delivery-reliability portion with PostgreSQL
leases, bounded retries/circuits, a durable DLQ, audited idempotent replay and
resubmit, and a real Kafka publisher using stable attempt IDs for downstream
duplicate suppression. Local unit/race/full-suite gates pass; required
PostgreSQL 16/Kafka CI and merge evidence passed in MR `!106` pipeline `19226`
and main pipeline `19235`.
Slice 2.4 implements runtime-wired S3/SFTP batch ingestion: an immutable source
is matched to the exact deployed release, a bounded reader streams concatenated
HL7v2 through the shared durable processor, and PostgreSQL leases/checkpoints
resume with deterministic admission identity. SFTP requires pinned host keys;
both providers verify a digest-addressed archive before deletion. S3 addresses
the exact version ID; SFTP requires immutable atomic publication and repeats the
content digest immediately before removal.
MR `!108` pipeline `19331` passed all 35 jobs, including required PostgreSQL 16/
MinIO/SSH-SFTP job `186259`, and merged as `ed32915f`. Main pipeline `19344`
passed all 38 jobs and independently repeated the proof in job `186476`.
HTTP catalog migration, GitOps exposure, and IDE
lifecycle controls remain open, so the secure data plane is not yet complete.

### 2. Durable integration IDE

- Stable project/session URLs support create, reopen, archive, and collaboration.
- Samples, profile/workflow/mapping drafts, immutable run inputs, diagnostics,
  accepted decisions, and export records survive restart.
- Preview uses the exact selected artifact revisions and shows warning/event
  deltas between revisions.
- Run stages stream live; diagnostics feed the global Problems panel; server
  lineage navigates back to the source inspector.
- Workflow dry-run uses session events and renders route/transform/action traces.
- Publish produces a reviewable, signed/versioned bundle with fixtures and test
  expectations, then promotes the same artifacts used by production.
- Message browser, trace view, deployment status, replay/resubmit, and DLQ tools
  operate on real runtime data.

### 3. Healthcare-grade governance

- Raw PHI is ephemeral/redacted by default. Retention requires explicit policy,
  encryption, TTL, access audit, and export controls.
- Authentication, tenant boundaries, RBAC, secret references, and immutable
  audit events cover GraphQL, REST, WebSocket, adapters, and IDE operations.
- CORS and WebSocket origin checks are explicit; no production wildcard policy.
- Terminology approvals, artifact publication, deployment, replay, and data
  export record actor, reason, timestamp, and revision.

### 4. Truthful reliability and operations

- `/health`, `/ready`, and `/metrics` report real component state and match
  deployed probes and documentation.
- Container, Compose, Kustomize, and Helm entrypoints start the intended runtime.
- Structured logs/traces correlate receipt, source message, canonical event,
  workflow run, and delivery attempts without exposing PHI.
- Backups, migrations, disaster recovery, scaling, and rolling-upgrade behavior
  are tested. Multi-replica operation does not depend on in-process fanout.
- CI has no false-green test or security jobs. Reachable critical/high
  vulnerabilities block shipping.

### 5. Standards and ecosystem readiness

- FHIR R4 validation supports declared Implementation Guides and makes
  conformance policy visible in artifacts and diagnostics.
- SMART App Launch and Bulk Data support conform to their published IGs rather
  than ad-hoc lookalike endpoints.
- Adapter/action extension contracts are documented and tested without making
  the core a generic plugin framework prematurely.
- SDK, CLI, API, examples, runbooks, migration docs, and a reproducible demo
  environment cover the golden journeys.

## Supported 1.0 matrix and measurable budgets

These are the initial release targets. Golden Path Slice 1.0 records the exact
reference hardware and Kubernetes minor before Engine Alpha; relaxing a target
requires a dated decision rather than an undocumented test change.

| Area | 1.0 target |
|---|---|
| Runtime | Linux amd64/arm64; Go 1.26 current security patch |
| Persistence | PostgreSQL 16 reference profile |
| Authoring UI | SvelteKit/npm; latest two Chrome, Edge, and Firefox releases; current Safari |
| Deployment | Docker Compose reference environment and one pinned, upstream-supported Kubernetes minor via Helm/Kustomize |
| Interactive sources | HL7v2 over MLLP and authenticated raw HTTP |
| Batch sources | S3 and SFTP with checkpoint/resume |
| Destinations | webhook, FHIR R4, PostgreSQL, file, and Kafka |
| Formats | HL7v2, CSV, X12 837/835/270/271/276/277, CDA/CCDA, FHIR R4 |
| FHIR standards | FHIR R4 4.0.1; US Core 9.0.0; SMART App Launch 2.2.0; Bulk Data 3.0.0 |
| Accessibility | WCAG 2.2 AA on all golden-journey surfaces |

Reference performance/recovery gates:

- authenticated MLLP/HTTP durable-accept latency: p95 <= 250 ms and p99 <=
  500 ms at 100 messages/second on 4 vCPU/8 GiB with destinations decoupled;
- one-hour steady-state: >= 250 2-KiB HL7 messages/second with no loss and zero
  duplicate receipt/event/outbox records for one idempotency key; transport
  retries retain that identity and may repeat delivery only under the declared
  at-least-once contract;
- 1-GiB batch import: peak RSS <= 512 MiB above idle and successful restart from
  the last durable checkpoint;
- destination recovery: queued attempts resume without manual repair and without
  unbounded retry growth;
- PostgreSQL-backed RPO <= 5 minutes and service RTO <= 30 minutes in the tested
  backup/restore exercise;
- one-version rolling upgrade and rollback preserve receipts, revisions, and
  resumable work without schema downgrade corruption.

A signed bundle means an immutable manifest containing artifact/fixture digests
plus a detached signature verified against a configured trust root before deploy;
a digest alone is not called a signature. P0 means exploitable security, PHI
exposure, data loss/corruption, or duplicate durable acceptance/event/outbox work
for one idempotency key. P1 means a supported
golden journey, upgrade, recovery, or required governance control is broken.

## Golden journeys

1. **ADT feed to FHIR**: create connections -> select/fork Source Profile ->
   paste samples -> resolve warnings -> author workflow -> dry-run -> publish ->
   send over MLLP -> receive ACK -> inspect trace and FHIR delivery.
2. **Failure and replay**: destination outage -> bounded retries -> DLQ -> inspect
   exact failure -> repair configuration -> replay once -> prove delivery.
3. **Profile drift**: new vendor sample -> warning delta -> update profile draft ->
   regression suite -> approval -> staged rollout -> compare diagnostics.
4. **Batch clinical import**: poll SFTP/S3 -> stream/chunk input -> parse -> route ->
   checkpoint -> resume idempotently without duplicate durable records.
5. **Operator audit**: search by MSH-10/patient-safe correlation -> view lineage,
   revisions, delivery attempts, actor decisions, and retained-data policy.
6. **Standards exchange**: authorize through SMART Backend Services -> validate a
   US Core 9.0.0 R4 payload -> start/poll/download a Bulk Data 3.0.0 export ->
   verify scopes, NDJSON, manifests, errors, and audit evidence.

## Non-goals

- Rewriting working parsers, canonical events, or workflow primitives.
- Competing feature-for-feature with every legacy engine before the golden
  journeys are reliable.
- Putting Loom/agent control-plane dependencies in the clinical data plane.
- Creating a second workflow DSL or a generic graph runtime for its own sake.
- Claiming HIPAA compliance from code alone; the product supplies controls and
  evidence, while deployment/process obligations remain explicit.

## Release gates

- **Gate 0A — secure baseline**: current Go security release; SQL identifier
  injection closed; security scan green.
- **Gate 0B — truthful delivery**: UI, live GraphQL/WebSocket, smoke, security,
  codegen, and contract jobs execute and block when applicable.
- **Engine Alpha**: Golden Path 001 passes through the shared processor.
- **Engine Beta**: MLLP + durable delivery/replay + real operational trace.
- **IDE Beta**: restart-safe session authoring through publish/deploy.
- **Release Candidate**: RBAC/PHI/DR/performance/upgrade gates pass.
- **1.0**: all six golden journeys pass in the supported deployment profile,
  documentation is current, and no P0/P1 completion issue remains open.

## Sources

### Repository

- `cmd/fi-fhir/main.go:4606` — `serve` loads a single workflow.
- `cmd/fi-fhir/main.go:4878` — generic webhook mount.
- `internal/ingest/http.go:57` — webhook wraps generic JSON.
- `internal/ingest/temporal.go:47` — unregistered S3/SFTP discovery activity.
- `internal/api/graphql/resolvers/schema.resolvers.go:38` — current interactive
  parse/persist/workflow composition.
- `internal/integration/session/runner.go:13` — concrete in-memory session runner.
- `internal/api/graphql/resolvers/integration_session_service.go:23` — in-memory
  session service construction.
- `ui/src/lib/features/integration-session/api.ts:43` — feature-flagged session
  preview and forced raw retention.
- `ui/src/lib/graphql/integration.test.ts:5` — HTTP-only live integration tests.
- `.gitlab-ci.yml:422` and `.gitlab-ci.yml:529` — binary/UI gate mismatch.
- `Dockerfile:53`, `docker-compose.yaml:13`, and
  `deploy/kubernetes/base/deployment.yaml:31` — runtime entrypoint mismatch.

### External primary sources

- NextGen Connect describes the baseline integration functions as filtering,
  transformation, extraction, and routing:
  https://github.com/nextgenhealthcare/connect
- InterSystems documents production adapters, persistent messages, visual trace,
  testing, resend, monitoring, and lifecycle management:
  https://docs.intersystems.com/irislatest/csp/docbook/DocBook.UI.Page.cls?KEY=EGIN_intro
  and
  https://docs.intersystems.com/irislatest/csp/docbook/DocBook.UI.Page.cls/documatic/changes/DocBook.UI.Page.cls?KEY=EGDV_testing
- SMART App Launch 2.2.0:
  https://hl7.org/fhir/smart-app-launch/
- HL7 FHIR Bulk Data Access:
  https://hl7.org/fhir/uv/bulkdata/
- US Core 9.0.0:
  https://hl7.org/fhir/us/core/STU9/
