# Implementation Plan: Integration Engine + IDE Completion

**Date**: 2026-07-12
**Spec**: `.loom/20-product-spec-integration-engine-ide-completion.md`
**Execution model**: RALPH — one independently proven vertical slice at a time

## Sequencing rule

Security and truthful verification precede feature expansion. Golden Path 001
must pass the product spec's riskiest-assumption kill-test before MLLP, connector
expansion, or deeper IDE lifecycle slices ship.

## Phase 0 — Make the baseline trustworthy

### Gate 0A: secure build/runtime baseline — complete

MR !89 pipeline 18379 passed on 2026-07-12. The prebuilt `lint:go`, pinned
`security:govulncheck`, and pinned `security:gosec` jobs each finished
`success`; the latest evidence-only commit remains subject to the same MR gate.

Scope:

- Upgrade Go `1.25.7` to current stable `1.26.5` in `go.mod`, CI, and Docker;
  move golangci-lint to a Go-1.26 build and pin the security scanners.
- Centralize strict PostgreSQL identifier validation for the event-store CLI and
  IDE-authored `event_store`/`database` action tables and columns, then quote
  identifiers at direct PostgreSQL query boundaries.
- Add regression tests for malicious, empty, NUL, case, and length boundaries.
- Re-run lint, `govulncheck`, gosec, race tests, build/vet, and the full Go suite.

Exit:

- No reachable vulnerability caused by the pinned Go standard library.
- No unwaived HIGH/HIGH SQL identifier finding remains at a discovered runtime
  configuration boundary; validation and suppression rationales have tests.

### Gate 0B: truthful CI and local reproducibility — complete

The first Gate 0B tranche is complete in MR !91: binary production, real
readiness, frozen npm install plus SvelteKit sync, full Vitest with live HTTP
queries, aggregate smoke checks, and transport-level GraphQL WebSocket event
delivery all passed required pipeline 18494 and main pipeline 18498. MR !92
closed the final tranche: required pipeline 18520 passed, then post-merge main
pipeline 18521 passed all 31 jobs. Its eight security/image gates had
`allow_failure=false`, and both deploy jobs remained stage-blocked until every
required test, security, build, and image-scan prerequisite was green.

Scope:

- Align `test:binary` rules with every UI/runtime contract job that consumes it.
- Always execute component Vitest; conditionally enrich with a live backend only
  when explicitly designed, never by early successful exit.
- Fix `scripts/smoke-test.sh` counter behavior under `set -e` and prove all
  health, GraphQL, and WebSocket checks execute.
- Add the live Integration Session subscription/run proof to CI.
- Standardize CI, Docker, Makefile, and developer flows on npm; remove or
  deliberately regenerate stale pnpm lock state so frozen installs are unambiguous.
- Make security job labels and `allow_failure` behavior truthful; promote green
  security gates to blocking.

Exit:

- A deliberately failing UI test, smoke check, or reachable vulnerability makes
  the relevant pipeline fail.
- The positive path also proves frozen install, full Vitest, live WebSocket event
  order, every smoke assertion, clean contract/codegen diffs, and a merge policy
  that rejects a failed required pipeline.

## Phase 1 — Golden Path 001: shared runtime spine

### Slice 1.0: foundation and minimal integration revision — complete

Lock the decisions that would be expensive or unsafe to retrofit after schemas
and ingress ship:

- one logical tenant/security domain per 1.0 deployment and identity propagation
  on every receipt, trace, audit, and artifact operation;
- PHI classification, raw-retention/TTL/encryption fields, audit envelope, and
  secret-reference contract;
- a minimal immutable `IntegrationDefinitionRevision` that binds tenant, source,
  format, profile revision, workflow revision, destination revisions, and policy;
- stable `RawEnvelope`, `ProcessRequest`, `ProcessResult`, diagnostic, receipt,
  delivery-result, and correlation contracts;
- explicit `production` and side-effect-free `preview` execution modes;
- reference hardware/deployment minor and the supported-1.0 matrix.

Exit: migrations and processor implementation cannot begin until a fixture can
construct and validate one revision without embedding secrets or retained raw PHI.

### Slice 1.1a: immutable artifact resolution — complete

Repair the persistence boundary before runtime composition:

- Source Profile creation/update owns an immutable current-revision pointer and
  exact historical lookup.
- Workflow versions allocate serially per workflow and publication rejects
  cross-workflow version ownership.
- A storage-neutral, single-deployment-tenant resolver verifies domain-separated
  profile/workflow content digests before returning defensive executable bytes.
- A required PostgreSQL CI test proves v1 remains resolvable after v2 becomes
  current/published and fresh store/resolver objects are constructed.

Exit: Slice 1.1b remains blocked until wrong tenant, owner, digest, nonexistent
revision, malformed content, and concurrent version-allocation cases fail closed
and the live v1-after-v2 kill-test passes.

MR !94 and required pipeline 18533 passed that boundary; post-merge main
pipeline 18542 passed all 33 jobs, including the isolated PostgreSQL proof.

### Slice 1.1b: canonical MessageProcessor preview semantics — complete

Introduce a small application service—not another parser abstraction—that owns:

`RawEnvelope -> integration/profile resolution -> parse -> canonical events -> route plan -> ProcessResult`

Acceptance:

- HL7v2 first; the selected published profile demonstrably changes behavior.
- Resolve only a server-owned integration revision and exact immutable profile
  and workflow bytes; compile a bounded published grammar rather than the
  permissive authoring representation.
- Preview owns the shared parsing/routing semantics and cannot invoke any
  destination. Production remains explicitly unavailable until Slice 1.2 wraps
  the same evaluator with durable receipt/idempotency/outbox work.
- Event, diagnostics, artifact revisions, planned routes/actions, and correlation
  identifiers are deterministic for the same request.
- Raw payload, parser text, workflow configuration, secrets, and executable
  clients are absent from the result and processor boundary.

### Slice 1.1c: authenticated preview adapters and legacy containment — complete

- Establish one deployment-owned tenant/principal request context before the
  GraphQL and IDE boundary can call the processor.
- Require POST for raw clinical payloads, enforce an explicit HTTP origin
  allowlist and bounded bodies, and leave WebSocket transport unmounted.
- Route GraphQL and the former Integration Session preview client through one
  typed `previewIntegrationMessage` mutation and the exact 1.1b kernel.
- Hold the browser bearer, raw samples, and filename-derived source labels only
  in tab memory. Reload discards all three; **Clear access** discards the
  bearer. Purge the two legacy localStorage keys on startup. No preview path
  writes raw sample/run state.
- Fail legacy direct submit/session execution closed. Production GraphQL submit
  remains unavailable until Slice 1.2 supplies the durable committer.
- Restrict the temporary `integration:preview` role to `health` and
  `previewIntegrationMessage`; it cannot use the legacy GraphQL catalog.
- Prove adapter/kernel parity and wrong-tenant/origin/method/body rejection with
  transport-level tests before any IDE activation.

The combined 1.1b/1.1c release shipped in MR `!96`. Default-branch pipeline
`18621` passed all 33 jobs and published matching `v0.1.18621` API/UI images.
GitOps MRs `!368` and `!369` rolled out the verified digests behind a suspended
automation barrier, passed the public auth/origin/containment/provenance/PHI
gate, and resumed healthy image automation. Exact evidence is recorded in
`.loom/iteration-plan-phase-1-slice-1-1c-authenticated-preview-adapters.md`.

### Slice 1.2: durable receipt and idempotency — complete

- Add PostgreSQL receipt, event, trace, attempt, and outbox storage and migrations.
- Add the durable production committer to the shared evaluator while keeping
  legacy GraphQL submit disabled; Slice 1.3 owns the first authenticated
  production ingress rather than reviving the direct parse/store/execute path.
- Define idempotency key precedence: explicit key, then source + MSH-10 + active
  integration revision.
- Persist before acknowledging; duplicate submissions reuse the durable receipt
  and do not create duplicate outbox work.
- State the guarantee precisely: durable acceptance once; at-least-once outbox
  delivery with duplicate suppression/idempotency where supported.

MR `!98` implements this slice. Pipeline `18854` job `181669` passed the
blocking PostgreSQL 16 race/fault/restart proof: six pre-commit fault positions
left all five record classes empty, one post-commit-unknown result recovered by
idempotent retry after restart, and 64 callers converged on one raw-free durable
admission unit with byte-identical results.

### Slice 1.3: authenticated HL7v2 HTTP ingress — implementation complete

- Bounded body, bearer/HMAC auth, explicit integration/source identity.
- Structured 4xx diagnostics and durable 5xx/retry semantics.
- Response carries receipt, event summary, warnings, and delivery status.
- Add `make golden-path-001` and its fixture/evidence contract, then run the full
  restart/duplicate/profile-delta/IDE-parity kill-test. Block later phases if it
  fails or produces disconfirming evidence.

The local gate passed 20 assertions against PostgreSQL 16 and a real restarted
`serve` process. It proved one durable receipt/event/lineage/attempt/outbox unit,
byte-identical duplicate responses before and after restart, selected-profile
divergence, exact production/IDE semantic parity, suppressed preview delivery,
and no raw/credential sentinel in responses or persisted JSON. The required
`test:golden-path-001` CI job remains the merge authority.

## Phase 2 — Production channel runtime

### Slice 2.1: expand the integration deployment lifecycle

- Expand the minimal revision with connection validation, schedules, health,
  capacity and deployment state without changing its identity/audit contracts.
- Draft -> validate -> approve -> publish -> deploy -> pause/resume -> retire.
- Optimistic concurrency and immutable release records.

### Slice 2.2: MLLP source adapter

- Configurable framing/timeouts/TLS/client allowlist and ACK/NACK policy.
- Backpressure and bounded concurrency.
- Durable acceptance before positive ACK.
- Real-world framing, split-packet, duplicate, timeout, and reconnect tests.

### Slice 2.3: delivery reliability

- Durable attempts, retry schedules, circuit status, outbox, DLQ, replay, and
  resubmit with reason/audit.
- Wire one real queue transport in addition to webhook/FHIR/database/file.

### Slice 2.4: batch sources

- Register S3/SFTP providers into runtime configuration.
- Replace discovery-only completion with streaming parse/checkpoint/resume.
- Secure SFTP host-key verification and implement archive semantics.

## Phase 3 — Durable IDE lifecycle

### Slice 3.1: restart-safe Integration Session Workspace

- Store interface + PostgreSQL implementation for sessions, samples, immutable
  runs, artifact revisions, accepted decisions, and exports.
- Stable list/create/reopen/archive routes.
- Ephemeral/redacted sample default; explicit retention policy.
- Exact profile revision is applied during preview.

Kill test: restart backend, reopen session, run one sample against two profile
revisions, and observe the expected warning/event delta without raw retention.

### Slice 3.2: streaming diagnostics and server lineage

- Subscribe before runs and render real stage progression.
- Feed diagnostics into Problems; deduplicate debug/session events.
- Navigate server lineage into HL7 inspector fields.

### Slice 3.3: workflow draft simulation

- Session event set + exact workflow revision -> route/transform/action trace.
- Compare run deltas and promote fixes without browser-local draft drift.

### Slice 3.4: publish and deploy

- Fix profile diff/change-summary correctness.
- Export signed/versioned bundle with fixtures and expected results.
- Approval and deployment actions target the exact tested revisions.

## Phase 4 — Operations, governance, and scale

### Slice 4.1: enforce identity, authorization, and PHI policy

- OIDC/OAuth service and human identity, tenant scoping, roles, origin policy,
  secret resolution, immutable audit, retention/TTL/encryption, and export controls.
- Proof: cross-tenant/object access, privilege escalation, secret/PHI logging, and
  expired-retention tests fail closed across REST, GraphQL, WebSocket, and adapters.

### Slice 4.2: operator control plane

- Real message/trace browser, deployment/channel controls, replay/resubmit/DLQ,
  actor reason capture, and policy-aware semantic payload rendering.
- Proof: failure/replay and operator-audit golden journeys pass without SQL/manual
  filesystem intervention.

### Slice 4.3: truthful observability and multi-replica behavior

- Real `/health`, `/ready`, `/metrics`, correlation-safe logs/traces, durable
  subscription fanout, leader/lease rules, and cardinality/PHI budgets.
- Proof: two replicas process/stream without in-memory fanout loss or duplicate
  durable acceptance/outbox records; transport retries preserve one identity.

### Slice 4.4: recovery, upgrade, and performance

- Backup/restore, migration compatibility, rolling upgrade/rollback, chaos and DR,
  ACK latency, throughput, queue recovery, and batch-memory gates.
- Proof: every numeric budget in the product spec passes on the pinned reference
  profile with archived reports.

## Phase 5 — Standards and ecosystem

### Slice 5.1: FHIR R4 and US Core conformance

- Pin FHIR R4 4.0.1 and US Core 9.0.0 packages, integrate an official validator,
  publish the USCDI/profile coverage matrix, and surface conformance policy in
  artifacts and diagnostics.

### Slice 5.2: SMART and Bulk Data conformance journey

- Implement/test SMART App Launch 2.2.0 backend services and Bulk Data 3.0.0
  asynchronous export semantics, authorization, NDJSON, manifests, cancellation,
  error handling, and audit.
- Proof: the standards-exchange golden journey and applicable official test suites
  pass; ad-hoc similarly named endpoints do not count.

### Slice 5.3: extension and compatibility contract

- Adapter/action contracts, SDK parity, examples, signed bundles, compatibility
  windows, deprecation, and migration policy.

## Phase 6 — 1.0 release evidence

- All golden journeys pass in Compose and supported Kubernetes deployment.
- Install/upgrade/uninstall and backup/restore are reproducible.
- Security, privacy, accessibility, performance, and disaster recovery gates are
  blocking and green.
- Docs/status/roadmap match executable behavior.
- No open P0/P1 completion issue.

## Cross-cutting proof matrix

| Concern | Required proof |
|---|---|
| Profile-driven behavior | Golden fixture where selected profile changes output |
| Delivery guarantee | Durable acceptance plus at-least-once delivery, duplicate/restart/concurrency tests |
| PHI posture | Raw absent by default; retention policy + TTL/audit tests |
| Transport | Protocol-level tests, not endpoint-exists probes |
| IDE/runtime parity | Same input/revisions produce equivalent event/diagnostics |
| Durability | Restart and multi-replica tests |
| Observability | Receipt-to-delivery trace and metric assertions |
| Security | govulncheck, gosec, auth/RBAC/origin/secret tests |
| Deployment | Rendered manifests plus live startup/readiness smoke |
| Standards | Official validator/conformance test evidence |

## Release-gate mapping

| Gate | Required slices |
|---|---|
| Gate 0A | 0A secure baseline |
| Gate 0B | 0B truthful CI/reproducibility |
| Engine Alpha | 1.0-1.3 and Golden Path 001 |
| Engine Beta | 2.1-2.4 plus engine-only MLLP and failure/replay proofs |
| IDE Beta | 3.1-3.4 and journeys 1 and 3 |
| Release Candidate | 4.1-4.4, journeys 1-5, and numeric/accessibility gates |
| 1.0 | 5.1-5.3, Phase 6 release evidence, and all six journeys |

## Immediate backlog

1. Expanded IntegrationDefinition publication and deployment lifecycle.
2. Production MLLP adapter.
3. Durable delivery attempts, replay, and one real queue transport.
4. Runtime-wired S3/SFTP ingestion with checkpoint/resume.
5. Restart-safe Integration Session workspace.

## Scope controls

- One vertical slice per MR.
- Each slice states scope in/out, acceptance criteria, rollback, and exact tests.
- Generated files update only through canonical generation commands.
- Documentation and plan state update with the implementation.
- A slice is not done until local gates and terminal CI are green or a precise
  exception is recorded.
