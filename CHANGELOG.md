# Changelog

All notable changes to fi-fhir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### IDE repair — honest surfaces, the operator bundle, production streaming (2026-09-25 → 2026-09-26)

- **Root causes (spec `.loom/36`, MRs !223 and !224)** — measured from the LAN against the live deployment, none of them in the trusted-network path: the operator plane was forbidden for every identity because the deployment granted only the transport grant `graphql:operator` and never the service roles `integration.operator`, `integration.delivery.operator` and `integration.deployment.operator` (fixed in platform/gitops MR 805 with no code change; decision "Grant the operator bundle rather than alias the transport grant"); SSE streaming answered 404 because the Integration Session workspace was never enabled in production; the Copilot was gated on an unset loom platform endpoint; the Problems badge counted the diagnostics of the empty default draft; `/api/auth/status` said nothing about roles.
- **R-A (MR !225)** — `/api/auth/status` reports `principal`, `roles`, derived `capabilities` (`operatorRead`, `operatorDelivery`, `operatorDeployment`, `clinicalRead`, `integrationSessions`, `streaming`, `subscriptions`, `llm.configured`) and `missingRoles`, evaluated through the same root-field role table the transport gate uses and checked in tests against the real operator service; the route and the probe share one authenticator, so a bearer presented from the LAN now reports `bearer`; `serve` logs one WARN per configured identity that holds the grant without `integration.operator`; `scripts/check-runtime-config.sh`, `.env.example` and `docker-compose.yaml` require the bundle for the local IDE.
- **R-B (MR !226)** — the IDE reads that contract: the operator page pre-flights on `capabilities.operatorRead` and names the missing roles instead of issuing a query the service will refuse; every streaming surface is keyed per subscription root (`streaming-unavailable` with `data-stream` and `data-reason`), and the four legacy panels (`eventStream`, `workflowEvents`, `debugStepEvent`, Runtime Output) render the honest state and never subscribe, because the SSE allowlist admits only `integrationSessionEvents` and `sessionRunEvents` by design; the session engine runs only when the build flag is on **and** `capabilities.integrationSessions` is true; the Copilot runs on the backend `llmCapability` (`copilot-llm-state`) and the loom platform chrome is hidden unless `PUBLIC_LOOM_ENDPOINT` is set; the Problems badge counts only a live draft; the `/health` poller keeps one request in flight and pauses while the tab is hidden.
- **R-C (MR !228)** — `ui/Dockerfile` builds with `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true` (the API decides at runtime); `INTEGRATION-SESSIONS.md` gains the production section (the one env entry, the database it migrates, verification, rollback) and `RUNBOOK.md` the "Live streaming is unavailable" section; the SSE client keeps each in-stream error's `extensions.code` (`GraphQLStreamError.codes`) and the classifier keys on `FORBIDDEN`, because the server's catalog-safe presenter rewrites every forbidden message to "GraphQL operation forbidden"; `buildOnApiOff.test.ts` proves "build flag on, API sessions off" end to end through the real gate, store, pages and clients with only `fetch` faked.
- **R-D (MR !227)** — blocking `test:ui-e2e`: Playwright (Chromium) against the built UI served by the production nginx template in front of three real `fi-fhir serve` stacks on PostgreSQL 16 (the operator bundle with sessions on; the bundle minus `integration.operator`; sessions off), asserting the status contract, the operator list with no pre-flight and no "forbidden", an `integrationSessionEvents` stream answering 200 `text/event-stream` while Live Stream shows the honest state, the Copilot's LLM state, the absent Problems badge and Platform indicator, and both negative controls; `check-report.mjs` fails the job if any check did not run and pass; `make ui-e2e` mirrors it on the docker host. First CI run: 7/7 in 249 s.
- **Production (platform/gitops MRs 805 and 811)** — every IDE identity (static bearer, trusted network, both Access principals) holds the operator bundle, and `FI_FHIR_INTEGRATION_SESSION_ENABLED=true` on `fi-fhir-api` turned the Integration Session workspace on: the store migrated itself into the durable PostgreSQL already open for the operator control plane; no retention key is mounted, so raw retention stays refused; signed publication stays off. Verified from the LAN: `integrationSessionEvents` answers `200 text/event-stream`, and `eventStream` answers the `FORBIDDEN` event by design.
- **Close-out** — this roadmap and changelog, the decision entry, and `ci/test-ui-e2e.yml` now runs when its own definition changes (`test:binary` with it, through the runtime-verification anchor).

### Sprint 7 — FHIR conformance, operator trace, harness truth (2026-09-24 → 2026-09-25)

- **Slice 5.1c-α (MR !216)** — the seven US Core cardinality violations recorded by the structural validator are closed in the mapper: `CareTeam.participant` is projected from the event (no members → no CareTeam), `Coverage.relationship` is `self` or `unknown`, a `DocumentReference` with no attachment is no longer emitted (and `content` can no longer serialise as JSON `null`), `Encounter.identifier.system` is qualified under the deployment-owned `urn:fi-fhir:source:<source>` via the new `USCoreMapper.Source`, `Encounter.type` derives from PV1-2 through the pinned US Core binding (SNOMED CT for inpatient and emergency, HL7 table 0004 otherwise), and vital-sign `effective[x]` is the event timestamp. Missing data becomes US Core's `unknown` data-absent reason, never an invented value. `recordedCardinalityGaps()` is empty; all 25 fixtures validate clean; the negative control still turns red on exactly `patient.json`.
- **Slice 5.1c-β (MR !219)** — the HL7 official validator (`validator_cli.jar` 6.10.4, sha256-pinned, never vendored) runs **offline** in the new blocking `test:fhir-official` job (with `test:fhir-official-capture` emitting the 11 delivered Bundles as artifacts) against R4 4.0.1 and US Core 9.0.0. The validator demands 23 archives, not 2: its own terminology and extensions defaults plus US Core's transitive closure. All 21 additional archives are pinned under `testdata/fhir/packages/` with digests reproduced against the registry; an online run with an empty cache produced the identical package summary and identical findings. Findings (157 over 36 inputs: 28 errors, 86 warnings, 43 information) are held by exact equality in `testdata/fhir/official/findings.ledger.txt`; a new or vanished row fails the build. `make fhir-official` and `make fhir-official-negative-control` mirror the job. Matrix §5 row 2 is ratified in full; `SUPPORTED-1.0.md` states what the evidence establishes.
- **Slice 4.2c (MR !217)** — `OperatorDeliveryAttempt.deliveries: [OperatorDestinationDelivery!]!` projects `integration_destination_deliveries` into the operator control plane: transport, destination revision and class, verified digest, outcome, failure code, HTTP status class, endpoint and served-certificate advisories, completion time, and the `fhir` facts (resource types, entry count, OperationOutcome issue codes). Newest first, tenant-scoped, bounded (5 per attempt on lists, 25 on a single attempt). `operator` reads it through a `DestinationDeliveryReader` seam; `cmd/fi-fhir` applies the existing provenance ledger migrations; no new migration and no new root field, so the transport gate is unchanged. The message trace renders it per attempt and the delivery console exposes it on demand; both show ledger fields only. Day-1 gate proved main exposed nothing from the ledger; inverted at ship.
- **Performance harness (MR !215)** — `scripts/performance-report.sh` set `certified: true` whenever `FI_FHIR_PERF_RUNNER` was `1`, comparing no measurement to any budget. It now requires budget 1's p95/p99 to be met on a runner reporting the `fi-fhir-perf` tag in a non-regression build; the benchmarks report p50/p95/p99; a `-tags perfregress` negative control (test files only) sleeps 300 ms per measured accept and must fail budget 1 (exit 3 otherwise); report schema 2 records `negative_control` and `runner_id`; budget 3 reads `not_measured`. A unit test runs the real script and fails against the old one. **No budget is certified**: runner 8's 11 GiB pod cannot schedule on its pinned node today.
- **CI (MR !218)** — quay.io and Docker Hub withdrew `minio/minio`; `test:integration` and `test:batch-ingestion` now pull both pinned MinIO releases from the internal registry by digest, mirrored byte-for-byte.
- **Close-out** — `events.EventMeta` gains a promoted `Meta()` accessor and `fhirout.mapEvent` sets `USCoreMapper.Source` from it, so the legacy workflow `fhir` action's raw Encounter identifier is qualified under `urn:fi-fhir:source:<source>` exactly as the durable path's `ensureIdentifier` already made it; a typed-nil event still yields the invalid-payload error rather than a panic. Delivered Bundles and the official-validator ledger are unchanged.

### FHIR delivery

- Deliver condition, procedure, immunization, vital sign, medication request, and allergy intolerance events through the shared workflow and durable FHIR projector.
- Honor workflow resource selection and resolve transaction references; unsupported JSON events now fail instead of silently producing only a Patient (issue #20).
- Preserve medication substitution `allowedBoolean: false` in JSON and require live HAPI FHIR read-back in the legacy E2E CI gate.

### Security

- Add the `clinical:read` GraphQL transport role for the ten PHI-reading event, patient, and projection queries while preserving the `graphql:operator` compatibility grant.

### Added

- CI job `lint:edi-fixtures` (`ci/lint-edi-fixtures.yml`) runs edilint, pinned to `v0.1.0`, over `testdata/edi/*.edi` against the committed `testdata/edi/.edilint.yml` on every merge request, and publishes `edilint-report.xml` as a JUnit report so envelope defects show up in the MR test panel. It runs beside the existing `lint:edi`, which is unchanged and still checks the same corpus on edilint's stock rules with no config file. `testdata/edi/.edilint.yml` waives EL3009 (duplicate interchange control number): the fixtures are independent interchanges and control-number uniqueness across the corpus is not an invariant the suite maintains. `TestFixtureTrailerCountsMatchTheirTransactionSets` pins every fixture's SE01 against the segments it actually contains in `go test ./internal/parser/edi/...`, without needing the network — the three miscounts edilint originally found were already repaired on `main` in `df45f683`, and nothing until now kept them repaired. `make lint-edi-fixtures` mirrors the job locally at the same pin and config, beside `make lint-edi`, which still mirrors `lint:edi`.

- Kafka, Redis Streams, and Google Cloud Pub/Sub event backends with acknowledgment-aware consumers, workflow queue drivers, `workflow consume`, shared handlers, and an event-sourcing outbox adapter.

- GraphQL callers can be authenticated by the identity Cloudflare Access verified at the edge: with `FI_FHIR_GRAPHQL_ACCESS_TEAM_DOMAIN`, `_AUDIENCE`, and `_PRINCIPALS` set, the runtime verifies the `Cf-Access-Jwt-Assertion` token (or the `CF_Authorization` cookie) against the team domain's keys and exact application audience, and grants each listed email exactly the roles the deployment maps to it. Works beside either bearer mode; an `Authorization` header keeps precedence. `/api/auth/status` reports `authVia: "cloudflare-access"` with the principal, and the IDE's credential gate steps aside for it, so a Google sign-in through Access carries straight into the IDE without a pasted token.

#### Integration Runtime Foundation
- Public `pkg/integration` contracts for content-addressed integration revisions, tenant/actor identity, typed secret references, PHI/raw-retention policy, production/preview requests, and stable processing results
- Golden Path 001 revision fixture with strict decoding, deterministic semantic digest validation, non-serializable raw payload bytes, and preview side-effect invariants
- Immutable Source Profile revision pointers plus exact profile/workflow artifact resolution with domain-separated content digests and single-deployment tenant enforcement
- Required PostgreSQL CI proof that pinned profile/workflow v1 artifacts survive v2 publication, process reconstruction, owner checks, and digest verification
- Internal preview-only `MessageProcessor` that resolves a server-owned integration revision and exact immutable artifacts, then produces deterministic, raw-free ADT A01 events, route plans, diagnostics, and suppressed deliveries
- Strict published-workflow DSL v1 parser and pure CEL route planner with bounded YAML resources, closed action types, safe diagnostics, stable action identity, and no execution-capable dependencies
- Strict executable Source Profile compiler plus one-message HL7v2 validation, standards-correct DTM offsets and precision, source-time precedence, and deterministic event identity
- Blocking PostgreSQL preview-kernel proof that reconstructs fresh stores after v2 publication while preserving byte-identical v1 behavior and exact v2 semantics
- PostgreSQL-only production admission on the same `MessageProcessor` semantics,
  atomically committing the durable receipt, sanitized canonical event, exact
  lineage, initial delivery attempts, and pending transactional outbox rows
- Deterministic tenant-scoped idempotency with explicit-key precedence,
  source/MSH-10/revision derivation, request-fingerprint conflicts, and
  commit-unknown recovery through the first durable result
- Blocking PostgreSQL 16 race gate that injects every transaction-boundary fault,
  restarts all handles, and collapses 64 concurrent submissions to one durable
  admission unit without persisting raw source bytes
- Authenticated, bounded `POST /v1/hl7v2` production ingress with bearer or
  domain-separated HMAC-SHA256 credentials, service-principal attribution,
  server-owned integration/source identity, structured retry semantics, and a
  PHI-free receipt/event/warning/provenance/delivery response
- `make golden-path-001` Compose/CI gate with PostgreSQL 16 migrations, valid
  duplicate and idempotency-conflict probes, real process restart, strict versus
  tolerant profile proof, production/IDE semantic parity, durable cardinality,
  JUnit/JSON/SQL evidence, and raw/credential leakage scans
- One typed `previewIntegrationMessage` adapter backed by a strict server-owned
  integration registry and the canonical `MessageProcessor`, plus a Mapping
  Studio credential gate that keeps the [REDACTED] raw samples in tab memory
- Supported 1.0 target matrix with a pinned Kubernetes 1.36 minor and explicit phase release gates
- Backward-compatible integration deployment policy for connection-validation
  freshness, continuous/cron schedules, health thresholds, and capacity limits
- PostgreSQL versioned integration lifecycle with optimistic commands, auditable
  failed validation, immutable releases/history, pause/resume/retire, health
  projection, and exact revision resolution only while deployed
- Blocking PostgreSQL 16 lifecycle gate covering the full state journey,
  32-caller concurrency, immutable-row rejection, restart reconstruction, and
  raw/secret leakage scans
- Content-addressed UTF-8 MLLP source revisions with bounded framing, timeouts,
  TLS 1.3 mutual authentication, canonical client CIDRs, capacity, and
  application/commit acknowledgement policy
- Optional `serve` MLLP runtime that resolves only the lifecycle catalog's exact
  deployed release and serializes pause/retire authorization inside the durable
  PostgreSQL admission transaction before a positive ACK
- PostgreSQL delivery leases, bounded exponential retry, destination-revision
  circuit state, durable DLQ, append-only audit, and idempotent replay/resubmit
- Optional `serve` Kafka outbox worker with stable attempt keys, sanitized
  canonical-event commands, all-ISR acknowledgement, TLS 1.3, and TLS-required
  SASL credentials
- Authenticated PostgreSQL operator commands for audited `delivery replay` and
  `delivery resubmit`, plus a blocking PostgreSQL 16/Kafka failure-recovery gate
- Content-addressed S3/SFTP batch sources with exact deployed-release binding,
  bounded concatenated-HL7v2 streaming, PostgreSQL leases/checkpoints, and
  deterministic crash-safe durable admission identity
- Optional `serve` batch worker with TLS-protected S3 credentials, mandatory
  SFTP `known_hosts`, symlink rejection, and verified SHA-256-addressed
  archive-before-delete semantics with S3 version IDs and an immutable SFTP
  drop contract plus immediate pre-delete digest verification
- Blocking PostgreSQL 16/MinIO/SSH-SFTP gate covering replica exclusion, lease
  reclaim, the admission/checkpoint kill window, source mutation, host-key
  rejection, archive integrity, exact durable cardinality, and raw-PHI exclusion
- Slice 2.4 evidence: MR `!108` pipeline `19331` passed 35/35, required batch job
  `186259` passed, merge commit `ed32915f` repeated the change on main, and main
  pipeline `19344` passed 38/38 with independent batch job `186476`
- Opt-in PostgreSQL Integration Session workspace with stable create/list/reopen/
  archive routes, redacted samples by default, AES-256-GCM explicit retention,
  append-only artifact revisions, immutable terminal runs, durable accepted
  decisions/exports, and exact profile revision/digest preview provenance
- Required PostgreSQL 16 restart gate that reconstructs the workspace service,
  compares strict/tolerant profile outcomes, and proves no raw-PHI sentinel is
  persisted in session records
- Slice 3.1 evidence: MR `!111` pipeline `19409` passed 37/37, required session
  job `187425` passed, merge commit `15746ccd` repeated the change on main, and
  main pipeline `19424` passed 40/40 with independent session job `187618`
- Feature-gated authenticated GraphQL SSE for Integration Session run/stage/
  diagnostic snapshots while WebSocket remains closed, with a transport-level
  session-subscription allowlist and durable terminal-run reconciliation
- Mapping Studio live server progression, deduplicated Problems diagnostics,
  and canonical HL7 inspector lineage navigation including repeated OBX fields;
  raw retained samples and persisted lineage value previews stay server-side
- Slice 3.2 evidence: MR `!115` pipeline `19464` passed 34/34, required session
  job `187950` and benchmark job `187953` passed, merge commit `36f2bb8c`
  repeated the change on main, and main pipeline `19482` passed 37/37 with
  independent session job `188135`
- Durable Workflow Builder simulation against explicit immutable Integration
  Session runs and one exact append-only workflow revision, with production-pure
  route planning, configuration-free event/route/transform/action traces,
  deterministic prior-run deltas, restart-safe PostgreSQL storage, and no
  browser-supplied event payloads or action execution
- Append-only, versioned Integration Session publication with PHI-minimal
  fixture/expectation manifests, domain-separated digests, detached Ed25519
  signatures, exact session-to-production content verification, trust-root-gated
  approval/deployment through the existing lifecycle catalog, and safe resume
  from an already-published release
- Source Profile review now compares the loaded immutable baseline with the
  edited draft using aligned line diffs and stores a required authenticated
  change summary with each immutable revision
- Integration Session exports now preserve arbitrary artifact content as opaque
  bytes, allowing YAML workflow revisions to round-trip safely alongside JSON
  profiles
- Slice 3.3 evidence: MR `!122` pipeline `19872` passed 37/37 with required
  session job `191685` and benchmark job `191688`, merge commit `d42f7233`
  repeated the change on main, and main pipeline `19878` passed 40/40 with
  independent session job `191786` and benchmark job `191789`
- Blocking PostgreSQL 16/TCP MLLP gate covering pre-commit ACK exclusion,
  concurrent pause serialization, 32 reconnecting duplicates, resume,
  retirement, restart, durable cardinality, and raw-message leakage
- Slice 2.2 evidence: MR `!104` pipeline `19175` passed 33/33, required MLLP job
  `184996` passed, merge commit `6205fa39` repeated the proof in main job
  `185093`, and main pipeline `19193` passed 36/36
- Verified MLLP client-certificate service identity: an optional
  `clients.identities` allowlist in the immutable source revision maps an
  authority-scoped URI SAN and/or SPKI SHA-256 pin to one canonical service
  subject and its grants, resolved per connection immediately after the TLS
  handshake and before any frame is read
- CA-valid MLLP certificates that map to zero or to multiple configured
  identities are closed without an acknowledgement, before artifact loading and
  before any durable record exists
- Mapped MLLP identities flow into the same fail-closed `integration.submit`
  decision as the HTTP ingress, over the exact tenant, integration revision, and
  registry-owned source, so an identity without a recognized submit grant
  authenticates but never admits
- `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY` refuses to start a listener in
  certificate-identity compatibility mode; omitting the identity map preserves
  the existing deployment-fixed principal, server-issued `integration:mllp`
  grant, and exact source-revision digests
- Batch (S3/SFTP) workload identity: an optional `workload` block in the
  immutable source revision names one canonical service subject and its grants,
  and nothing observed on the remote side — object keys, remote metadata, or MSH
  content — can select or influence it
- The batch connector evaluates the shared fail-closed `integration.submit`
  decision before listing, leasing, opening, reading, artifact loading, or any
  durable write, so an ungranted subject leaves no lease or checkpoint state to
  poison a later retry
- `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY` refuses to start a batch source in
  compatibility mode; omitting the `workload` block preserves the existing
  deployment-fixed principal, server-issued `integration:batch` grant, and exact
  source-revision digests
- Per-root-field GraphQL transport-gate roles over all 131 schema root fields
  with default-deny, a compile-time exhaustiveness test against the schema the
  server executes, and a `transportgateblanket` build tag that restores the old
  blanket allow as the kill-test's negative control
- `test:transport-gate` CI job and `make transport-gate` /
  `make transport-gate-negative-control`
- FHIR conformance proof (Slice 5.1a): every one of the mapper's 26 `Map*` entry
  points is driven with a representative event and every resource it produces is
  fed back through `ValidateJSON` at `--mode us-core --strict` with zero issues,
  bound to the type by reflection so a new entry point without a row turns it
  red. Generated golden fixtures under `testdata/fhir/mapper/` hold the mapper's
  exact bytes for all 21 checked resource types, plus
  `testdata/fhir/diagnosticreport_uscore_lab.json` in the curated set.
  `make fhir-conformance` and `make fhir-conformance-negative-control`, the
  latter restoring the pre-slice DiagnosticReport accepted set and requiring
  exactly the `MapLabResult` row to fail
- `TestFHIRConformance_DurableEngineProducesNoFHIRResource`: the durable engine
  delivers a Kafka delivery-command envelope at `application/json`, the
  destination transport vocabulary is `{kafka, https}` with no FHIR class, and no
  package under `internal/integration` imports `pkg/fhir`. Executes the kill-test
  `.loom/28-spec-fhir-ig-bulk-smart.md` defined for the moment Slice 4.1c-b
  merged; its answer is that Slice 5.1 remains blocked on a FHIR destination
  class (4.1c-c) that does not yet exist
- `fhir.ParseValidationMode`, `fhir.ValidationModes`, `fhir.ProfileCanonical`,
  and `fhir.USCoreDiagnosticReportLabProfile`

### Changed
- The decision journal is one file per entry under `.loom/decisions/` (`make decisions-new TITLE=...`, `make decisions`), and `.loom/40-decisions.md` is a pointer page, mirroring the August worklog split. `scripts/decisions.sh check` runs in `lint:docs` and rejects a dated entry appended to the pointer page or a second heading in an entry file. The single append-only journal had re-conflicted every open Sprint 5 merge request on each sibling merge.
- `build:docker` and `build:docker-ui` pass `--network=host` to `docker build`. Inside the Docker-in-Docker service the default per-build bridge network intermittently blackholed Alpine package fetches (`RUN apk ...` hung until the one-hour job timeout four times on 2026-09-02, while image pulls through the daemon and the same build on another runner slot succeeded); host networking routes RUN steps through the daemon's own egress.
- `test:benchmark` is non-blocking on merge-request pipelines (`allow_failure: true` on the manual rule). It stays manual so it can be played for performance-sensitive changes, and stays blocking on tags and the default branch. Previously the unplayed manual job held every Go-touching MR pipeline at status `manual`, so merge-when-pipeline-succeeds never fired and MRs could not merge without a human playing the job.

- Batch receipt provenance no longer trusts remote object modification time. The
  authoritative `received_at` is now the server-owned custody timestamp recorded
  when an exact object version is first durably admitted, stable across lease
  reclaim, worker restart, and checkpoint resume
- Batch content provenance is now a SHA-256 digest computed over the exact bytes
  streamed during admission, resumed across checkpoints from marshaled hash state
  and cross-checked against a full re-read before archive; a disagreement
  quarantines the object with `DIGEST_MISMATCH` instead of archiving it
- S3 batch objects now pin the entity tag alongside the exact version ID and
  re-verify both at every read, archive, and delete
- `integration_batch_objects.object_modified_at` is renamed
  `remote_modified_at_advisory` and joined by `object_version`, `object_etag`,
  and `digest_state` (migration `0002_batch_provenance`). The provenance CHECK is
  `NOT VALID` so rows admitted before this revision stay visibly distinguishable
  rather than being given invented provenance
- Operator control-plane GraphQL API over the existing durable delivery and
  lifecycle records: tenant-scoped, keyset-paginated browsing of receipts,
  canonical events, receipt-to-delivery lineage, delivery attempts, dead
  letters, destination circuits, delivery audit, and deployment inventory
- Policy-aware semantic payload rendering that returns a canonical event's
  field coordinates, JSON kinds, and repetition flags while never returning a
  stored value, a value length, or a caller-influenced map key
- Reason-required, role-gated, idempotent operator mutations for delivery
  replay, resubmit, and dead-letter discard, plus lifecycle pause, resume,
  retire, and deploy with expected-version optimistic concurrency, all
  delegating to the existing operation ledger and append-only audit trail
- Durable `discard` recovery decision with a dead-letter resolution column, so a
  closed dead letter records whether it was replayed, resubmitted, or abandoned
- Blocking PostgreSQL 16 operator gate that completes the failure/replay and
  operator-audit golden journeys over the real GraphQL handler with a verified
  OIDC operator identity, proves duplicate control actions do not double-execute,
  proves unprivileged and cross-tenant callers reach no data and change no state,
  and proves a planted raw-PHI sentinel never leaves the process
- Operator control-plane workspace at `/operator` in the IDE: durable message
  browser with server-owned filters and cursor paging, receipt-to-delivery trace
  drill-down, dead-letter and destination-circuit console, and deployment
  controls, with one reason-required dialog fronting every mutating action
- Operator actions the engine would refuse are disabled with an explanatory
  reason instead of failing after the click, and optimistic-concurrency
  conflicts reload the durable record and ask the operator to re-decide rather
  than retrying silently

#### Format Adapters
- CDA/CCDA clinical document parser with namespace-aware XML handling (`internal/parser/cda/`)
- CDA section handlers for structured data extraction (`internal/parser/cda/sections/`)
- CDA-to-canonical event mapper (`internal/parser/cda/mapper.go`)
- FHIR R4 resource parser for inbound FHIR ingestion (`internal/parser/fhir/`)
- HL7v2 MDM messages (T01–T11) — Medical Document Management with TXA/OBX support
- HL7v2 DFT messages (P03, P11) — Detail Financial Transactions with FT1/DG1/PR1/IN1
- HL7v2 VXU immunization messages
- HL7v2 RDE pharmacy messages
- EDI X12 270/271 eligibility inquiry and response transactions
- EDI X12 276/277 claim status inquiry and response transactions
- EDI companion guide framework with built-in payer guides (`internal/parser/edi/companion/`)
- Built-in companion guides: Medicare, BlueCross, United Healthcare (`companion/builtin/`)
- Companion guide validator and path-based field resolution

#### Event Sourcing / CQRS
- Event store interface with append-only semantics (`pkg/eventsourcing/store.go`)
- In-memory event store for testing (`pkg/eventsourcing/memory_store.go`)
- PostgreSQL event store for production (`pkg/eventsourcing/postgres_store.go`)
- Projection framework with checkpointing (`pkg/eventsourcing/projection.go`)
- Snapshot store interface with memory and PostgreSQL implementations
- Healthcare projections: patient timeline, event statistics, active encounters (`pkg/eventsourcing/projections/`)
- Event replay tooling with ProjectionRebuilder (progress, dry-run, snapshot-aware)
- Time range queries for point-in-time recovery (`pkg/eventsourcing/time_range.go`)
- Event archival and HIPAA-aware retention policies (`pkg/eventsourcing/archive.go`)
- Event stream compaction with aggregate snapshots (`pkg/eventsourcing/compaction.go`)
- Saga orchestration for multi-step transactions with compensation (`pkg/eventsourcing/saga.go`)
- Outbox pattern for reliable event publishing (`pkg/eventsourcing/outbox.go`)
- CLI commands: `eventstore init|stats|streams|read|append`, `projection list|status|run|rebuild`

#### FHIR Resources
- US Core Patient, Encounter, Observation, DiagnosticReport (enhanced)
- US Core Condition, Procedure, Immunization, MedicationRequest
- US Core AllergyIntolerance, CarePlan, Goal, CareTeam, ServiceRequest
- US Core DocumentReference, DiagnosticReport (clinical notes), Provenance
- US Core Location, Organization, Practitioner, PractitionerRole, RelatedPerson
- Observation (Vital Signs) with 8 specific US Core profiles
- Da Vinci PAS Claim resource (837P → FHIR)
- PDex ExplanationOfBenefit resource (835 → FHIR)
- CoverageEligibilityResponse resource (271 → FHIR)
- FHIR Coverage resource (US Core)
- FHIR resource validation with configurable failure policy (warn vs error per profile)
- FHIR validation golden fixtures for high-volume resources

#### GraphQL API
- GraphQL schema with queries, mutations, and subscriptions (`internal/api/graphql/schema.graphql`)
- GraphQL schema retains legacy query, mutation, and subscription types; the
  deployed preview-role configuration authorizes only authenticated POST
  health/preview and leaves WebSocket unmounted
- Resolver implementations: event queries, workflow triggers, FHIR subscription CRUD
- Batch event submission endpoint (`submitBatch` mutation with parallel/sequential modes)
- DataLoaders for N+1 query prevention (`internal/api/graphql/dataloaders/`)
- Projection resolvers wired to event sourcing service layer
- GraphQL codegen with gqlgen and CI validation (`lint:gqlgen`)

#### Terminology System
- LOINC file loader with panel expansion (`pkg/terminology/loinc.go`)
- ICD-10-CM loader with ETL pipeline integration (`pkg/terminology/db/icd10.go`)
- RxNorm loader and cross-walk queries (`pkg/terminology/db/rxnorm.go`)
- UMLS API integration with rate limiting, caching, ticket auth (`pkg/terminology/umls.go`)
- Cross-walk queries: ICD-10 ↔ SNOMED, RxNorm ↔ NDC
- Fuzzy terminology matching with confidence scoring (`pkg/terminology/fuzzy.go`)
- Terminology version pinning and registry/index (`fi-fhir terminology status|use`)
- Version-aware validation modes: pass / warn / error
- Semantic search engine (`pkg/terminology/semantic/`)
- Suggestion engine with feedback loop (`pkg/terminology/suggest/`)
- Full-text terminology indexing (`pkg/terminology/index/`)
- Mapping file upload pipeline (`pkg/terminology/upload/`)
- Automatic terminology routing engine (`internal/terminology/autoroute/`)
- Temporal workflow activities for terminology operations (`internal/terminology/workflow/`)

#### LLM Features
- Multi-provider LLM client with retry and rate limiting (`pkg/llm/`)
- Embedding generation for semantic search (`pkg/llm/embeddings.go`)
- Natural language explanation generation (`internal/llm/explain/`)
- Structured data extraction from documents (`internal/llm/extract/`)
- Data quality analysis with scoring (`internal/llm/quality/`)
- CEL-based copilot actions (`pkg/llm/copilot/`)
- LLM-integrated workflow actions (`internal/workflow/actions_llm.go`)

#### Patient Matching
- Deterministic matching rules: SSN, MBI, MRN exact match (`pkg/matching/deterministic.go`)
- Probabilistic scoring: Jaro-Winkler, Soundex, Levenshtein (`pkg/matching/similarity.go`)
- Combined matcher with configurable thresholds (`pkg/matching/matcher.go`)
- Master Patient Index (MPI) interface with in-memory implementation (`pkg/matching/mpi.go`)
- Batch matching with blocking keys for performance

#### UI / Mapping Studio
- SvelteKit 5 frontend with feature-based architecture (`ui/src/`)
- HL7 Inspector with segment/field viewer (`ui/src/lib/features/hl7/`)
- Profile Selector and Profile Draft Panel
- Sample Inbox for test message management
- Terminology Editor with mapping browser and uploader
- Autoroute Resolver and Pending Review List
- Workflow Builder with visual route/action/transform editors
- Workflow Monitor and Dry Run Panel
- Event Stream Panel for real-time event viewing
- System Status Panel
- LLM Extraction Panel
- Generate-from-description (natural language → workflow)
- Reusable UI component library: Badge, Button, Toast, Tooltip, Tabs, etc.
- Authenticated GraphQL HTTP preview client; subscription consumers fail locally
  while production WebSocket transport is disabled
- OpenAPI-generated type-safe API client

#### Source Profiles
- `fi-fhir profile infer` — generate profile skeleton from sample messages
- `fi-fhir profile lint` — schema validation + opinionated warnings
- Vendor profile templates: Epic, Cerner, Meditech, Allscripts
- Template selection guide and feed-specific fork workflow
- Inference fixtures and golden outputs

#### Workflow Engine
- Email action (SMTP/SES; templated subject/body; retries + circuit breaker)
- File action (templated paths; atomic writes; rotation/retention)
- Exec action (allowlist + timeouts for custom scripts)
- LLM action for AI-powered workflow steps
- Event replay and simulation tooling (`internal/workflow/replay.go`, `simulation.go`)
- Performance benchmarking (`internal/workflow/benchmark_test.go`)
- Load testing utilities (`internal/workflow/loadtest.go`)

#### ETL Pipeline
- Source/sink framework with provider abstraction (`pkg/etl/source/`, `pkg/etl/sink/`)
- CLI commands: `etl fetch`, `etl load`, `etl validate`
- Storage provider abstraction (file, S3, MinIO) (`pkg/storage/`)

#### Observability
- Prometheus metrics adapter (`internal/workflow/metrics_prometheus.go`)
- OpenTelemetry distributed tracing adapter (`internal/workflow/tracing_otel.go`)
- Structured JSON logging with trace correlation (trace_id, span_id)
- Grafana dashboard templates (`dashboards/grafana/`)
- Prometheus alerting rules: standalone + Kubernetes PrometheusRule CRD (`dashboards/alerting/`)
- Health check endpoints: /health, /ready (`internal/workflow/health.go`)
- Log correlation with trace IDs (`internal/workflow/logging.go`)

#### Reliability
- Retry with exponential backoff for HTTP actions (`internal/workflow/retry.go`)
- Circuit breaker pattern for failing external services (`internal/workflow/circuit_breaker.go`)
- Dead letter queue for failed events (`internal/workflow/dlq.go`)
- Rate limiting (token bucket) for high-volume streams (`internal/workflow/ratelimit.go`)
- Configuration validation (`internal/workflow/validate.go`)

#### Testing
- End-to-end test framework with Docker Compose integration (`test/e2e/`)
- PostgreSQL integration tests with testcontainers
- CLI offline stubs + live tests (`-tags=live`)
- Performance benchmarks for workflow engine
- Load testing runner with event generators
- FHIR validation golden fixtures

#### Deployment
- Multi-stage Dockerfile with distroless base (enhanced)
- Kubernetes manifests with Kustomize overlays
- Helm chart with full templating: HPA, PDB, ServiceMonitor (`deploy/helm/fi-fhir/`)
- GitLab CI/CD pipeline with blocking lint, test, benchmark, security, build,
  image-scan, and API/UI publish gates
- Harbor container registry integration with automated pushes
- UI Docker image with Nginx serving
- Coordinated Kubernetes rollout of matching API/UI images behind suspended
  Flux automation, with live auth, origin, containment, provenance, and
  PHI-leakage probes before a reviewed automation resume
- Cross-platform release binaries (linux/darwin/windows × amd64/arm64)
- Helm OCI + npm registry publishing on tags

#### SDK
- TypeScript SDK with CLI wrapper (`sdk/typescript/`)
- Type definitions for events, workflow, and profiles
- Platform-specific optional dependency packaging (darwin/linux/windows)
- npm publish pipeline in CI

#### Documentation
- OpenAPI 3.1 specification for REST API (`api/openapi.yaml`)
- Production hardening guide for HIPAA compliance (`docs/operations/PRODUCTION-HARDENING.md`)
- Operations runbook with troubleshooting procedures (`docs/operations/RUNBOOK.md`)
- User guide: getting started, core concepts, CLI reference, playground tutorial
- Developer guide: architecture, setup, testing, adding parsers
- Mermaid architecture diagrams (overview, parsing phases, CLI flow, UI mapping)
- Planning documents for all major features (14 design docs)
- Component status matrix (`docs/STATUS.md`)
- Documentation conventions (`docs/DOCUMENTATION-CONVENTIONS.md`)

### Changed

- CI `test:integration` promoted from `allow_failure: true` to a blocking merge
  gate (24/24 green on `main`, pipelines 18521..22333). It now protects the
  terminology DB store and the Lane C1 autoroute expiry-sweep kill-test on every
  merge request.
- CI `lint:docs` promoted to a blocking merge gate (33/33 green on `main`). Run
  `make docs-validate` before pushing.
- CI `test:docs-status` deliberately remains advisory; promotion criteria are
  documented inline in `.gitlab-ci.yml` and in `.loom/40-decisions.md`.

### Fixed

- The X12 fixtures declare the segment count they actually contain.
  `271_rejected.edi` said 13 where it had 12, `271_response.edi` said 23 where it
  had 22, and `277_denied.edi` said 18 where it had 19 — trailers a receiving
  trading partner rejects, repaired in the corpus by `df45f683` and never
  recorded here. The parser recomputes `SegmentCount` from the segments it
  reads and never compares it with the declared SE01, so nothing caught the
  drift; `TestFixtureTrailerCountsMatchTheirTransactionSets` and the
  `lint:edi-fixtures` CI job now both do.

- `workflow validate`, `workflow run`, and `workflow dry-run` reject invalid CEL,
  transform, and built-in action configuration before reading events. Validation
  diagnostics include severity, code, and path; warnings remain non-blocking (#21).

- FHIR validation no longer fails open on the mode string. `ValidationOptions.Mode`
  was compared byte-exactly against `us-core`, so any other value — including
  `US-Core`, `uscore`, and `""` — silently disabled both the required-element and
  the profile-presence checks and reported a non-conformant resource as clean;
  `fi-fhir fhir validate --mode US-Core` printed "FHIR validation passed" and
  exited 0 on bytes that `--mode us-core` rejects. The mode is now a closed,
  case-insensitive, whitespace-trimmed set (`none`, `us-core`) and anything
  outside it is `ErrUnknownValidationMode`. **Breaking for callers that relied on
  an unrecognised or empty mode meaning "validate structurally"** — pass `none`
  explicitly. The CLI rejects the flag before reading input
- The shipped FHIR validator no longer rejects the shipped FHIR mapper's own
  output. `MapLabResult` stamps `us-core-diagnosticreport-lab` (correct: US Core
  defines separate `-lab` and `-note` DiagnosticReport profiles and this package
  produces both), but `-lab` was a bare literal that was never declared as a
  constant and the checker accepted only `-note`
- A version-pinned profile canonical (`…/us-core-patient|9.0.0`) no longer fails
  the profile-presence check. Policy: the mapper asserts bare canonicals, the
  checker accepts either form
- `Patient.MRN` is no longer dropped. The mapper read identifiers only from
  `Patient.Identifiers`, so an MRN-only patient produced zero identifiers and a
  hard `Patient.identifier is required (US Core)` error. It is backfilled as an
  `MR`-typed identifier, value-deduplicated against identifiers already present
- `DiagnosticReport.code.coding` no longer repeats a `(system, code)` pair when a
  parser populates both `LabTest.LOINCCode` and `LabTest.Code.Coding`
- CI `test:integration` MinIO service container never started: `minio/minio`
  ships `CMD ["minio"]`, which prints usage and exits, so the service never
  listened on `minio:9000`. `setupTestInfra()` responded with `t.Skipf`, silently
  skipping **30 integration tests** (event store, projections, terminology
  init/status, storage, mapping-decision CLI) behind a green job. The service now
  runs `server /data`. Verified by coverage delta: 73.2% degraded vs 75.9% live.
- `TestIntegration_TerminologyMappingDecisionCLI` asserted an untruncated
  23-character source code appeared in a decisions-table column rendered through
  `truncate(decision.SourceCode, 12)`. The fixture now fits the column width.
- README action templates now use the JSON key paths the workflow engine
  actually evaluates (`{{.patient.family_name}}`), replacing Go struct field
  paths (`{{.Patient.Name.Family}}`) that silently rendered `<no value>`.
- README dry-run example now calls the `fi-fhir workflow dry-run` subcommand;
  `fi-fhir workflow run` has no `--dry-run` flag.
- README Prometheus metric names corrected to the emitted `fi_fhir_workflow_*`
  namespace, replacing names that matched no registered metric.
- README project structure, format tables, semantic event names, and Helm
  install flag corrected against the current code.
- README FHIR output bullet corrected from a four-resource description
  (Patient, Encounter, Observation, DiagnosticReport) to the 26 exported
  `Map*` methods on the US Core R4 mapper in `pkg/fhir/mapper.go`. README
  Observability bullet no longer claims OpenTelemetry tracing is shipped; it
  is scaffolded at the workflow layer but not wired into `serve` (see the
  Tracing section).
- Mermaid diagram sources regenerated to reflect the integration engine,
  deployment lifecycle, and IDE journey added in phases 1 through 3.

- Concurrent durable receipt insertion now arbitrates both the deterministic
  receipt primary key and tenant/idempotency key before the authoritative stored-
  result lookup and request-fingerprint validation, preventing valid duplicate
  callers from surfacing a primary-key error.

- Runtime verification CI now requires the fi-fhir binary for UI, TypeScript
  SDK, and smoke consumers; waits for the configured server port; runs the
  complete SvelteKit/Vitest suite; aggregates every smoke assertion safely
  under strict shell mode; and proves the production handler rejects GraphQL
  WebSocket upgrades and legacy routes. npm 10.9.3 is the canonical UI package
  manager and the stale pnpm lock has been removed.

- Workflow benchmarks now replace terminal log actions with a benchmark-only
  no-op handler, parse `events/sec`, and fail when a thresholded result is
  missing; benchmark test failures now propagate through the artifact-capture
  step, and shared-x86 latency ceilings are calibrated from default-branch
  evidence so the gate measures engine performance instead of console I/O or
  silently skipped records. The calibrated benchmark job is now blocking.

### Security
- UI image runs `apk upgrade` in the nginx stage so libssl3/libcrypto3 carry the fix for CVE-2026-14456 (HIGH, `security:trivy-ui-image`). `npm audit fix` in `ui/` moved browserslist past GHSA-c83g-rgw3-j3cx / GHSA-73wf-gq98-2v4g and postcss-selector-parser past GHSA-w9m9-85wc-3x92 (`security:npm-audit-ui` high gate); three low-severity `cookie` advisories remain behind the SvelteKit major and are below the gate.
- golang.org/x/crypto bumped v0.53.0 → v0.55.0 for CVE-2026-56854 (x/crypto/ssh authentication bypass via unenforced source-address restrictions; used by the SFTP batch provider). Pulled x/mod, x/net, x/sync, x/sys, x/text and x/tools forward one minor each via `go mod tidy`. Surfaced by `security:trivy` on the first MR pipeline after the advisory published.
- golang.org/x/crypto bumped v0.53.0 → v0.55.0 for CVE-2026-56854 (x/crypto/ssh authentication bypass via unenforced source-address restrictions; used by the SFTP batch provider). Pulled x/mod, x/net, x/sync, x/sys, x/text and x/tools forward one minor each via `go mod tidy`. Surfaced by `security:trivy` on the first MR pipeline after the advisory published. Same MR: google.golang.org/grpc v1.82.1 → v1.83.1 for CVE-2026-84304 (HIGH), surfaced by `security:trivy-image` on the built binary.
- GraphQL now fails startup closed without a deployment tenant, principal,
  preview role, exact HTTP origins, one canonical [REDACTED], and a matching
  immutable integration registry. HTTP accepts only bounded JSON POST requests;
  WebSocket transport is unmounted and UI subscription consumers fail locally.
- GraphQL rejects duplicate, case-aliased, malformed, wrongly typed, or trailing
  JSON before gqlgen and presents catalog-safe errors without reflecting raw
  request/query content. nginx and Kubernetes ingress stream bounded request
  bodies without proxy temp-file buffering.
- The `integration:preview` role can invoke only `health` and
  `previewIntegrationMessage`. Legacy submit, batch, workflow-trigger, parse,
  session execution/raw retention, export, and live-parse paths are unavailable
  by default. Profile-YAML and unauthenticated generic-ingest HTTP bypasses are
  no longer mounted by `serve`; canonical UI and cluster proxies expose no
  legacy `/api` fallback.
- The GraphQL transport gate no longer allows every operation to any
  `graphql:operator` holder. It enumerates all 131 root fields and refuses any
  it has no role for. The sixteen operator control-plane fields require the same
  roles as the service behind them — `integration.operator` for the nine reads,
  plus `integration.delivery.operator` for replay/resubmit/discard and
  `integration.deployment.operator` for pause/resume/retire/deploy — so a
  control-plane operator can be issued a token that reaches nothing else.
  `graphql:operator` is retained as a named, deprecated compatibility grant that
  expands to the full set, so every existing operator token is unaffected; the
  remaining 115 root fields are still reachable only through it and each carries
  a `TODO` naming the slice that should narrow it. `serve` prints the mapping's
  shape at startup. `integration:preview` and the SSE stream-context allowlist
  are unchanged, and every service-layer authorization check is unchanged: this
  is defence in depth, not a relocation.
- Mapping Studio preview now compiles its public registry alias through the
  Vite environment namespace, validates complete tenant/provenance/correlation
  lineage, keeps raw samples and filename-derived labels in tab memory, and
  purges their two legacy localStorage keys during startup.
- Security evidence is now enforced: govulncheck, high-confidence/high-severity
  gosec, Trivy filesystem critical/secret checks, UI and TypeScript SDK npm
  audits, pinned go-licenses policy checks, and both runtime image scans are
  required merge-request jobs with their reports retained as artifacts.
- Refreshed the UI dependency lock within declared ranges, pinned the patched
  same-major Lodash resolution required by the current GraphQL Codegen
  toolchain, moved the UI to the compatible Vite 7/Svelte plugin 6 pair, and
  upgraded the TypeScript SDK to Vitest 4.1.10; both frozen npm 10.9.3 trees now
  contain no HIGH or CRITICAL audit findings.
- Replaced the mutable full nginx UI runtime base with a digest-pinned nginx
  Alpine slim image that removes the four vulnerable unused packages; backend
  and UI images are now built and scanned before merge and reject every
  CRITICAL plus every fixed HIGH finding. Main deploys wait for those scans,
  and tagged releases retag the exact scanned artifacts instead of rebuilding
  mutable inputs. The backend Docker context now excludes UI dependencies and
  local build/tool scratch data.
- Upgraded the Go build/runtime baseline to 1.26.5 and the Go-1.26-compatible
  golangci-lint baseline to 2.12.2; govulncheck and gosec versions are now pinned.
- Event-store and database workflow actions now reject configuration-controlled
  SQL identifiers outside lowercase PostgreSQL identifiers (`[a-z_][a-z0-9_]*`,
  maximum 63 characters) and quote identifiers at direct query boundaries.
- Public PostgreSQL event, checkpoint, projection-snapshot, and stream-snapshot
  stores now quote raw unqualified table and derived index names internally;
  embedded NUL bytes and names over PostgreSQL's 63-byte limit receive a
  deterministic hash suffix.
- Non-root container execution
- Read-only root filesystem
- Secret provider interface (env, file, Vault, AWS SSM, K8s secrets)
- TLS 1.3 support
- Pod security standards (restricted)
- Network policy templates
- govulncheck + gosec in CI pipeline
- Trivy filesystem and image scanning
- Required npm audits for UI and TypeScript SDK dependencies
- Required pinned license compliance checking (go-licenses)

## [0.1.0] - 2024-01-15

### Added

#### Core Functionality
- HL7v2 message parsing (ADT A01-A04, A08, ORU R01, SIU S12-S15, S26)
- CSV/flatfile parsing with schema inference
- EDI X12 parsing (837P claims, 835 remittance, 270/271 eligibility, 276/277 status)
- Canonical semantic event model (`pkg/events/`)
- Source Profile system for per-interface configuration

#### Workflow Engine
- YAML-based workflow DSL for event routing
- CEL (Common Expression Language) filter conditions
- Transform pipeline (set_field, map_terminology, redact)
- Action types: log, webhook, fhir, database, queue
- Dry-run mode for testing workflows

#### FHIR Integration
- FHIR R4 resource generation (Patient, Encounter, Observation, DiagnosticReport)
- US Core profile mapper
- OAuth2 client credentials flow with token caching
- Automatic 401 retry with token refresh

#### Reliability Features
- Retry with exponential backoff for HTTP actions
- Circuit breaker pattern for failing external services
- Dead letter queue (DLQ) for failed events
- Rate limiting for high-volume event streams
- Event replay from DLQ or recordings

#### Observability
- Prometheus metrics (`workflow_events_processed_total`, etc.)
- OpenTelemetry distributed tracing
- Structured JSON logging with trace correlation
- Grafana dashboard templates
- Prometheus alerting rules

#### CLI
- `parse` - Parse messages (HL7v2, CSV, EDI)
- `workflow run` - Process events through workflow
- `workflow validate` - Validate workflow configuration
- `config show/validate/env/init` - Configuration management
- `version` - Version information

#### Deployment
- Multi-stage Dockerfile with distroless base
- Docker Compose for local development
- Kubernetes manifests with Kustomize overlays
- Helm chart with full templating
- GitLab CI/CD pipeline (lint, test, security, build, release)

#### SDK
- TypeScript SDK with CLI wrapper
- Type definitions for events and workflow

#### Validation
- NPI (National Provider Identifier) validation with Luhn check
- MBI (Medicare Beneficiary Identifier) validation
- SSN format validation
- DEA number validation

### Security
- Non-root container execution
- Read-only root filesystem
- Secret provider interface (env, file, Vault, AWS SSM, K8s secrets)
- TLS 1.3 support
- Pod security standards (restricted)
- Network policy templates

## Types of Changes

- `Added` for new features
- `Changed` for changes in existing functionality
- `Deprecated` for soon-to-be removed features
- `Removed` for now removed features
- `Fixed` for any bug fixes
- `Security` for vulnerability fixes

[Unreleased]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/compare/v0.1.0...main
[0.1.0]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/releases/v0.1.0
