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

### Slice 1.3: authenticated HL7v2 HTTP ingress — complete

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
and no raw/credential sentinel in responses or persisted JSON. MR `!99`
pipeline `18898` passed 32/32 jobs, including required Golden Path job `182088`.
Merge commit `48d156d2` passed 35/35 jobs in main pipeline `18951`; independent
Golden Path job `182694` repeated the 20-assertion proof.

## Phase 2 — Production channel runtime

### Slice 2.1: expand the integration deployment lifecycle — complete

- Expand the minimal revision with connection validation, schedules, health,
  capacity and deployment state without changing its identity/audit contracts.
- Draft -> validate -> approve -> publish -> deploy -> pause/resume -> retire.
- Optimistic concurrency and immutable release records.

Implementation:

- `IntegrationDefinitionRevision` accepts an optional deployment policy without
  changing legacy Slice 1 JSON or digests. Lifecycle-managed revisions require
  bounded connection-validation freshness, a continuous or cron schedule,
  health thresholds, and capacity limits.
- `internal/integration/lifecycle` persists exact revision JSON, failed/successful
  validation evidence, releases, and lifecycle events as append-only PostgreSQL
  records. A versioned snapshot is the only mutable projection.
- The state machine is closed to
  `draft -> validated -> approved -> published -> deployed -> paused -> deployed`
  with retirement from published/deployed/paused. Human commands require a
  reason and every command uses an expected version.
- Runtime resolution returns the exact immutable release only while deployed.
  Static registry/runtime wiring remains intentionally unchanged until Slice 2.2.
- Required CI job `test:deployment-lifecycle` discovers and runs the PostgreSQL
  16 race/restart/immutable-row proof with `allow_failure: false`.

Evidence:

- MR `!101` pipeline `19014` passed 32/32; required lifecycle job `183463`
  passed, and merge commit `a95bb44f` repeated the proof in main job `183702`.
- The first main pipeline exposed an existing concurrent durable-receipt primary-
  key conflict. MR `!102` made insertion arbitrate either deterministic unique
  key before the authoritative lookup and fingerprint check; pipeline `19045`
  passed 24/24.
- Final main pipeline `19052` passed 26/26, including durable-submission job
  `183938` and lifecycle job `183940`. Production GitOps activation remains a
  separate reviewed operation.

### Slice 2.2: MLLP source adapter — complete and merged

- Configurable framing/timeouts/TLS/client allowlist and ACK/NACK policy.
- Backpressure and bounded concurrency.
- Durable acceptance before positive ACK.
- Real-world framing, split-packet, duplicate, timeout, and reconnect tests.

Implementation:

- `internal/integration/mllp` owns a strict content-addressed UTF-8 source
  revision, configurable single-byte framing, validated ACK projection, TLS 1.3
  mutual auth, CIDR policy, and bounded connections/capacity/rate.
- Each frame resolves `PostgresCatalog.ResolveRunnable`, verifies the exact
  source revision and deployed policy, and uses the shared production processor.
- `PostgresSubmissionStore` accepts an optional transaction authorizer. MLLP
  takes a shared lifecycle snapshot lock through admission commit; pause and
  retire take the conflicting update lock.
- `serve` enables MLLP only when its immutable source path is configured. HTTP
  ingress and preview keep their prior registry behavior; profile/workflow bytes
  remain digest-verified through the static artifact registry.
- `test:mllp-runtime` discovers and runs the PostgreSQL 16/TCP race proof. MR
  `!104` pipeline `19175` passed 33/33, including required job `184996`, and
  merged as `6205fa39`. Main pipeline `19193` passed 36/36 and independently
  repeated the proof in job `185093`.
- CI exposed and closed two proof defects before merge: empty diagnostics now
  persist as JSON `[]`, and the 32-client duplicate fixture declares matching
  bounded TCP/queue capacity. Production GitOps activation remains separate.

### Slice 2.3: delivery reliability

- Durable attempts, retry schedules, circuit status, outbox, DLQ, replay, and
  resubmit with reason/audit.
- Wire one real queue transport in addition to webhook/FHIR/database/file.

Implementation status:

- PostgreSQL migration v2 adds expiring leases, attempt schedules, parent links,
  circuit state, durable DLQ, idempotent operator operations, and append-only
  audit while preserving Slice 1.2's atomic initial unit.
- `internal/integration/delivery` claims with `FOR UPDATE SKIP LOCKED`, applies
  bounded retry/circuit policy, and emits raw-free Kafka commands with the stable
  attempt ID as key and lineage headers.
- Optional `serve` wiring fails closed on partial configuration, requires TLS for
  credentials, and shuts down with GraphQL/MLLP. PostgreSQL-authenticated CLI
  replay/resubmit records `current_user`, reason, and operation idempotency key.
- Unit/race/full-suite gates pass locally. MR `!106` pipeline `19226` passed
  34/34, including PostgreSQL 16/Kafka job `185433`, and merged as `ca968fbf`.
  Main pipeline `19235` passed 37/37 and repeated the proof in job `185505`.

### Slice 2.4: batch sources — complete and merged

- Register S3/SFTP providers into runtime configuration.
- Replace discovery-only completion with streaming parse/checkpoint/resume.
- Secure SFTP host-key verification and implement archive semantics.

Implementation status:

- `internal/integration/batch` defines content-addressed S3/SFTP sources,
  validates the exact deployed lifecycle binding, and streams concatenated
  HL7v2 with a bounded reader.
- PostgreSQL leases exact object versions and advances byte/message checkpoints
  only after durable admission. Deterministic object/offset identity collapses
  the admission-before-checkpoint crash window without storing raw paths or PHI.
- S3 and SFTP copy to digest-addressed archives, verify bytes, commit completion,
  and only then delete the source. S3 targets its exact version ID. SFTP requires
  `known_hosts`, immutable atomic publication, immediate pre-delete digest
  verification, and rejects symlinked inputs/archive targets.
- Optional `serve` wiring is fail closed. The required integration target uses
  PostgreSQL 16, MinIO, and a real SSH/SFTP server to prove replica exclusion,
  lease reclaim, kill/resume, mutation isolation, secure host keys, archive
  integrity, exact durable cardinality, and raw-PHI exclusion.
- MR `!108` pipeline `19331` passed 35/35, including required batch job `186259`,
  and merged as `ed32915f`. Main pipeline `19344` passed 38/38 and repeated the
  provider recovery proof in job `186476`. Production GitOps activation remains
  a separate reviewed operation.

## Phase 3 — Durable IDE lifecycle

### Slice 3.1: restart-safe Integration Session Workspace

- Store interface + PostgreSQL implementation for sessions, samples, immutable
  runs, artifact revisions, accepted decisions, and exports.
- Stable list/create/reopen/archive routes.
- Ephemeral/redacted sample default; explicit retention policy.
- Exact profile revision is applied during preview.

Kill test: restart backend, reopen session, run one sample against two profile
revisions, and observe the expected warning/event delta without raw retention.

Implementation status (2026-07-16): complete, merged, and independently
reverified on main. MR `!111` pipeline `19409` passed 37/37, including required
PostgreSQL 16 restart/raw-leakage job `187425`, and merged as `15746ccd`. Main
pipeline `19424` passed 40/40 and repeated the proof in job `187618`.

### Slice 3.2: streaming diagnostics and server lineage

- Subscribe before runs and render real stage progression.
- Feed diagnostics into Problems; deduplicate debug/session events.
- Navigate server lineage into HL7 inspector fields.

Implementation status (2026-07-16): complete, merged, and independently
reverified on main. The
feature-gated authenticated SSE transport permits only Integration Session
subscriptions on bounded `POST /graphql`; WebSocket remains closed. Mapping
Studio subscribes before runs, reconciles durable terminal snapshots, deduplicates
Problems diagnostics, and navigates canonical repeated-segment lineage. Production
GitOps activation and durable cross-replica fanout remain pending.

MR `!115` pipeline `19464` passed 34/34, including required session job
`187950` and benchmark job `187953`, and merged as `36f2bb8c`. Main pipeline
`19482` passed 37/37 and independently repeated the session proof in job
`188135`.

### Slice 3.3: workflow draft simulation

- Session event set + exact workflow revision -> route/transform/action trace.
- Compare run deltas and promote fixes without browser-local draft drift.

Implementation status (2026-07-18): complete and merged. Workflow Builder can
save the current YAML as an append-only session
revision, plan explicit successful immutable runs through the production pure
planner, render PHI-minimal event/route/transform/action traces, and compare the
result with the prior simulation over the same run set. PostgreSQL migration 2
persists traces and exact revision provenance across restart; no transform,
action, destination, or external call is executed.

Kill test: parse one redacted ADT sample, simulate two exact workflow revisions
over the same immutable run, reconstruct the PostgreSQL store, compare the
restored route/action delta, and prove filesystem-side-effect, action-config,
and raw-PHI sentinels are absent.

MR `!122` pipeline `19872` passed 37/37, including required session job
`191685` and benchmark job `191688`, and merged as `d42f7233`. Main pipeline
`19878` passed 40/40 and independently repeated the session proof in job
`191786` and benchmark proof in job `191789`.

### Slice 3.4: publish and deploy

- Fix profile diff/change-summary correctness.
- Export signed/versioned bundle with fixtures and expected results.
- Approval and deployment actions target the exact tested revisions.

Implementation status (2026-07-18): shipped in MR `!124`; MR pipeline `19939`
and post-merge main pipeline `19944` passed, with merge commit `84d2fab2`.
Integration Session publications are append-only, versioned, and signed with
Ed25519 over a canonical PHI-minimal manifest. The
service independently resolves the exact production profile/workflow bytes,
proves content equivalence under production digest rules, and reuses the closed
lifecycle catalog for optimistic approval, immutable release publication, and
deployment. Workflow Builder exposes the bounded flow, and Source Profile review
now compares an immutable baseline and persists a required actor-attributed
change summary. Production GitOps activation remains a separate reviewed action.

Kill-test status: unit/adversarial, full Go, full UI, and the required CI
PostgreSQL restart/append-only proofs passed before merge.

## Phase 4 — Operations, governance, and scale

### Slice 4.1: enforce identity, authorization, and PHI policy

- OIDC/OAuth service and human identity, tenant scoping, roles, origin policy,
  secret resolution, immutable audit, and export controls.
- Proof: cross-tenant/object access, privilege escalation, secret/PHI logging, and
  audit-immutability tests fail closed across REST, GraphQL, WebSocket, and adapters.

**Correction (2026-08-08, Slice 4.1d C1).** This bullet previously claimed
"retention/TTL/encryption" enforcement and an "expired-retention" proof. Read
against the code, that claim had no subject and the proof could not have been
written:

- The durable committer **rejects** every non-ephemeral raw retention mode with
  `ErrUnsupportedRawRetention`
  (`internal/integration/processor/postgres_submission.go:179-181`), so there is
  no retained production raw PHI whose TTL could expire. An "expired-retention"
  test would assert over an empty set.
- The PHI that *is* retained indefinitely — canonical event payloads, session
  sample ciphertext, and session export snapshots — carried no policy field, no
  expiry column, and no purge job at all
  (`internal/integration/processor/migrations/0001_atomic_submission.sql:19-33`,
  `internal/integration/session/postgres.go:324-333`).

Retention was therefore not an enforcement gap over an existing policy; it was a
policy that did not exist for the data that actually persists. Designing it,
adding the expiry columns, and building the durable purge runtime shipped as
**Slice 4.1e** (`.loom/32-sprint4-execution-specs.md`, Lane S4-B; formerly named
S3-C2 in `.loom/31`).

**Correction (Slice 4.1e, 2026-08-08).** The framing above, and Slice 4.1d C1's
handoff, both described the obstacle as a `DELETE` problem. That was wrong in
three independent ways, proved against unmodified `main` by
`TestPhiRetention_PurgeIsStructurallyBlockedToday`:

1. C1's guard on `integration_canonical_events` is a **blanket** `BEFORE UPDATE
   OR DELETE` (`0004_audit_immutability.sql:29-32`), so it blocked deletion
   **and** payload redaction — both mechanisms a purge could use, not one.
2. Even with that trigger lifted, the row is undeletable:
   `integration_message_lineage` and `integration_delivery_attempts` reference it
   `ON DELETE RESTRICT` and are themselves undeletable, and
   `integration_delivery_outbox` chains off the attempts
   (`0001_atomic_submission.sql:52-54,73-75,90-92`).
3. The same is true of an exported session: the export row is append-only and the
   session row is held by the export's foreign key, so **both** are permanently
   undeletable once an export exists.

The purge that shipped is therefore a **tombstone** under a column-scoped,
schema-enforced exemption, never a deletion, and a tombstone is explicitly **not**
a backup-inclusive deletion. See `.loom/40-decisions.md` (2026-08-08, "Slice
4.1e"). The current posture, with file:line citations, is
`docs/operations/PHI-RETENTION.md`.

#### Slice 4.1a: OIDC-authenticated GraphQL human identity

- Add a long-lived OIDC discovery/JWKS verifier behind the existing GraphQL
  authenticator seam. Require HTTPS discovery/JWKS, rejected redirects, bounded
  remote refresh, a signed `typ=at+jwt` access-token class, an exact issuer, one exact
  audience, an asymmetric algorithm allowlist, valid expiry/not-before,
  nonempty subject, exact deployment-tenant claim, and a strict nonempty role
  array.
- Project verified caller identity into the server-owned security context used
  by GraphQL POST/SSE operation authorization. Keep static bearer/trusted-network
  handling only in an explicit compatibility mode; OIDC mode rejects those
  settings, and static mode rejects OIDC settings.
- Keep browser login/refresh, service identity for production adapters,
  fine-grained policy administration, immutable audit storage, PHI retention,
  export controls, WebSocket enablement, and GitOps activation in later 4.1
  slices.

Implementation status (2026-08-06): merged in MR !131 as `036f7acd`; exact main
pipeline 22300 passed all 32 jobs. The real-handler kill-test proves expected-tenant
preview and operator tokens reach the authorized GraphQL POST/SSE resolvers,
cross-tenant and malformed claims fail authentication, unprivileged roles fail
before resolver data, and JWKS rotation succeeds through a time/rate-bounded
unknown-key refresh. The focused and full repository race suites, lint, vet,
documentation, module-integrity, and reachable-vulnerability gates pass.

#### Slice 4.1b1: OAuth HTTP service identity and submit authorization

- Add a constrained OAuth2 client-credentials resource-server mode for the
  production HL7v2 HTTP ingress. Reuse the Slice 4.1a verifier and require the
  exact trust domain, `typ=at+jwt`, time window, deployment tenant, allowlisted
  canonical `sub == client_id`, and a signed `integration:submit` grant.
- Carry the verified service identity per request while keeping the integration
  revision and source server-owned. Add one explicit `integration.submit`
  authorization decision over the exact tenant, revision, and source.
- Enforce the same decision at the adapter boundary, before processor artifact
  loading, and in transaction-scoped runnable admission. Preserve existing
  HTTP, MLLP, and batch grant names as compatible submit grants.
- Keep token issuance/introspection, MLLP certificate identity, batch workload
  identity, GraphQL control actions, delivery/export policy, immutable audit,
  PHI controls, and GitOps activation in later slices.

Implementation status (2026-08-07): merged in MR !132 as `9d952552` (merge
commit `ea760bc3`); exact main pipeline 22333 passed. The load-bearing kill-test
proves two allowlisted clients through one real handler remain distinct despite
spoofed provenance headers, and proves a no-grant production request stops
before artifact loading or durability. Focused and full-repository race suites,
lint, vet, documentation, module-integrity, and vulnerability/security scans
pass.

#### Slice 4.1b2: MLLP certificate service identity and submit authorization

- Add an optional `clients.identities` allowlist to the immutable, content-
  addressed MLLP source revision. Each entry maps one authority-scoped
  certificate criterion — a URI subject alternative name, a subject public key
  info pin, or both — to one canonical service subject and its grants. Common
  names are never accepted as identity.
- Resolve one verified `ConnectionIdentity` per accepted TLS connection
  immediately after the handshake, before any frame is read, parsed, processed,
  or admitted. A CA-valid certificate matching zero entries or more than one
  entry closes the connection with no acknowledgement.
- Carry the verified subject, auth method, and grants into the same fail-closed
  `integration.submit` decision added in Slice 4.1b1, evaluated at the adapter
  boundary before capacity and envelope construction, again in the shared
  processor before artifact loading, and again in transaction-scoped runnable
  admission. Source, tenant, and integration binding stay server-owned.
- Keep identity mapping all-or-nothing per listener. An empty identity list
  preserves the existing deployment-fixed principal and server-issued
  `integration:mllp` grant, and existing source revisions keep their exact
  digest. Declaring identities requires mutual TLS, and no connection can fall
  back between the two modes. `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY` lets a
  deployment refuse to start in compatibility mode.
- Keep batch/S3/SFTP workload identity, destination-scoped identity, GraphQL
  control actions, token issuance/introspection, certificate revocation
  transport, immutable audit storage, PHI controls, and GitOps activation in
  later slices.

Implementation status (2026-08-08): implemented and locally verified; landing
pipeline evidence recorded in the Slice 4.1b2 handoff. The load-bearing kill-test
`TestPostgresMLLPRuntime_CertificateIdentityAuthorization` runs the real MLLP
listener over real TLS 1.3 mutual authentication against PostgreSQL 16 with the
production durable processor. It proves two mapped certificates stay two
distinct verified subjects at the authorization decision even when the second
sender's MSH provenance impersonates the first, that an unmapped CA-valid
certificate reaches neither artifact loading nor any durable record class, that
an ungranted mapped identity is denied for the exact tenant/revision/source, and
that compatibility mode is unchanged. Three independent negative controls —
silent fallback for unmatched certificates, ignored per-identity grants, and a
deployment-fixed principal in mapped mode — each fail the test.

#### Slice 4.1b3: batch workload identity and trusted receipt provenance

- Add an optional `workload` block to the immutable, content-addressed batch
  source revision. It names one canonical service subject and its grants, and it
  is the identity under which every object that source ingests submits. Object
  keys, remote directories, remote metadata, and MSH content can never select,
  influence, or impersonate it.
- Evaluate the same fail-closed `integration.submit` decision added in Slice
  4.1b1 at the connector boundary in `PollOnce`, immediately after deployed-
  release binding validation and before listing, leasing, opening, reading,
  artifact loading, or any durable write; again per message before the processor
  loads artifacts; and again in transaction-scoped runnable admission. A denied
  source therefore leaves no lease or checkpoint state to poison a later retry.
- Keep binding all-or-nothing per source. An absent `workload` block preserves
  the deployment-fixed `FI_FHIR_BATCH_PRINCIPAL_ID` principal and the
  server-issued `integration:batch` grant, and existing source revisions keep
  their exact digest because the block is omitted from the canonical digest
  input. `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY` lets a deployment refuse to
  start in compatibility mode.
- Replace remote object modification time as trusted receipt provenance. The
  authoritative received-at becomes the server-owned custody timestamp recorded
  when an exact object version is first durably admitted, stable across lease
  reclaim, restart, and checkpoint resume. Content provenance becomes a SHA-256
  digest over the exact bytes streamed during admission, resumed across
  checkpoints from marshaled hash state and cross-checked against a full re-read
  before archive. S3 additionally pins the exact version ID and the entity tag
  observed at listing and re-verified at every read, archive, and delete. The
  remote value survives only as `remote_modified_at_advisory` and takes no part
  in any trust or audit decision.
- Keep destination-scoped identity, token issuance/introspection, cloud workload
  federation transport, GraphQL control actions, immutable audit storage, PHI
  controls, and GitOps activation in later slices.

Implementation status (2026-08-08): implemented and locally verified; landing
pipeline evidence recorded in the Slice 4.1b3 handoff. The load-bearing kill-test
`TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance` drives real MinIO
and a real SSH/SFTP server against PostgreSQL 16 with the production durable
processor and transaction-scoped runnable admission. It proves that two bound
subjects stay distinct at admission while both objects carry identical MSH
sending application/facility naming one of them and the S3 key names the other,
that an ungranted subject halts with every durable record class unchanged and the
same object admits cleanly once the grant is repaired in a new source revision,
that compatibility mode still admits under the deployment-fixed principal and the
server-issued grant, and that a remote modification time spoofed to 1994 never
becomes a canonical `received_at`. Five independent negative controls — a
deployment-fixed principal in bound mode, no connector-boundary decision, remote
modification time as received-at, ignored per-identity grants, and a streaming
digest over normalized rather than raw bytes — each fail the test.

#### Slice 4.1c-a: destination-scoped identity contract

The sprint scope assumed the engine authenticates to destinations, so 4.1c would
be "scope an existing credential". It does not. The durable worker publishes one
command per attempt to the constant Kafka topic `integration.delivery.v1`
(`internal/integration/delivery/dispatcher.go`,
`internal/integration/processor/postgres_submission.go`); an external consumer
performs the destination call. `webhook`, `fhir`, `database`, and `file` are
plan-level action classes validated in
`internal/integration/processor/workflow_plan.go`, never executed transports.
There was no destination artifact behind the digest carried on every attempt, and
no `SecretReference` resolver of any kind — the Slice 1.0 secret contract was a
name-presence check.

4.1c is therefore two slices:

- **4.1c-a (this slice)** — the missing contract and the missing decision.
- **4.1c-b (next)** — the first durable HTTPS destination consumer that presents
  the scoped identity.

4.1c-a delivers:

- `DestinationRevision` in a new `internal/integration/destination` package:
  schema version, artifact/revision/destination identity, class, transport kind,
  a non-secret transport policy carrying binding **names** only, an optional
  client identity block, a domain-separated semantic digest, `Validate()`, and
  `ValidateAgainst(lifecycle.RunnableBinding)` applying the same
  `hasSecretBinding` discipline as `mllp/source.go` and `batch/source.go`.
- A server-owned destination registry that resolves an attempt's reference to the
  exact deployed revision and requires the reference to match byte for byte —
  revision ID, digest, and class included — so an attempt carrying another
  destination's digest cannot be published under the wrong identity.
- `integration.SecretResolver` in `pkg/integration` with one file/env
  implementation wired in `cmd/fi-fhir/`, never inside `internal/integration/*`.
  Every declared binding is resolved once at startup and discarded, so a
  credential that does not resolve refuses startup rather than failing at
  dispatch.
- `ActionDeliver = "integration.deliver"`, `ObjectDestinationRevision`, and the
  dotted grant `integration.destination.client`, matching the convention 2.3 and
  4.2a already use. `Authorize` is restructured into per-action decisions with
  the submit path's conditions unchanged, and the deliver path requires an empty
  `SourceID` so a source principal can never be replayed as a destination client
  or the reverse.
- Enforcement in `Dispatcher.RunOnce` after `Claim` and before the command is
  built or published. A refusal is a non-retryable `DELIVERY_FORBIDDEN` or
  `DELIVERY_DESTINATION_UNVERIFIED` routed through the existing `MarkFailed`, so
  it enters the DLQ with `attempt_count` unchanged and is visible to 4.2a's
  control plane instead of spinning. An infrastructure failure in the decision
  path is surfaced and retried, never converted into a dead letter.
- 4.1b1-style `strict` and `compatibility` modes that reject each other's
  configuration, plus refusal of any `FI_FHIR_DELIVERY_IDENTITY_*` setting
  without a mode, so the decision cannot be half-applied.
- Server-owned decision provenance in the package's own migration set. The
  destination-declared endpoint is labeled `_advisory` with a `COMMENT ON COLUMN`
  stating it is never a trust input, and the provenance CHECK lands `NOT VALID`
  so no later backfill can silently claim it governed rows it never saw. Absence
  of a decision row means the decision was never made, never that it was allowed.

Keep the HTTPS transport, token issuance/introspection, cloud workload
federation, GraphQL destination authoring, and PHI retention in later slices.

Implementation status (2026-08-08): implemented and locally verified; landing
pipeline evidence recorded in the Slice 4.1c-a handoff. The day-1 gate
`TestDeliveryDispatch_ContactsNoDestination` **passed against unmodified main**,
which is what converted the slice from "scope the existing credential" into
"build the missing contract": a live loopback TLS endpoint standing where a
webhook destination would be reached recorded zero accepted connections and zero
served requests across one complete production submission, while Kafka received
exactly one command; no durable record or broker payload carried a scheme, host,
or port; and a URL-named destination was rejected at planning before any durable
row existed. The load-bearing kill-test
`TestDeliveryIdentity_PostgresKafkaScopedDispatch` drives PostgreSQL 16 and Kafka
with two bound destinations, one crossed digest, and one orphan. Two independent
negative controls each fail it: stubbing the decision to return nil publishes all
four attempts and records no provenance, and removing the registry's digest
equality check publishes the crossed-digest attempt.

#### Slice 4.1c-b: first durable HTTPS destination consumer

The day-1 gate `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday`
**passed against unmodified main**, which is what fixed this slice's shape. A
destination declaring `transport: https` with a live TLS endpoint at its URL was
fully resolved, digest-verified, authorized, and provenance-recorded with its
address — and then published to Kafka anyway, with the endpoint recording zero
accepted connections. 4.1c-a resolves the destination revision on the dispatch
path and discards it, and holds no secret resolver at all after startup. So the
slice is not "wire a transport onto an existing resolution"; it is **build the
routing seam, the dispatch-time credential lifetime, and the trust-closed
client**.

4.1c-b delivers:

- `DestinationTransport` in `internal/integration/delivery`: a second
  primitives-only seam **parallel to** `DestinationDecider`, not a widening of
  it. The `destination` package satisfies it structurally; neither package
  imports the other, which is what avoids the import cycle the S3-B handoff
  records solving. A `TransportFailure` interface mirrors `Refusal` and adds the
  retryable bit.
- Transport substitution at the `Publisher` seam in `Dispatcher.RunOnce`, chosen
  over an in-process consumer of `integration.delivery.v1` and recorded in
  `.loom/40-decisions.md` (2026-08-09). It inherits the lease, retry, backoff,
  DLQ, replay, resubmit, discard, and the per-destination-artifact circuit for
  free, and leaves `internal/integration/delivery/store.go` — 4.2a's file —
  untouched. `MarkPublished` now means "handed off successfully" for two
  transports; it is documented, not renamed.
- A trust-closed HTTPS client: a redirect is refused and never followed, TLS is
  pinned to ≥ 1.2 with `CABundleBinding` roots or the system pool,
  `InsecureSkipVerify` appears nowhere, no proxy is honored, response headers are
  read for nothing, and the body is drained to a bound and discarded unparsed.
  The only property of the response that leaves the client is its status class.
- Dispatch-time credential lifetime: the token binding is resolved per dispatch
  through `integration.SecretResolver`, used once, and zeroed. There is no cache,
  because file and env references cannot be version-pinned and a cache would
  silently pin a rotated-out credential. The material never reaches a `Decision`,
  a `DeliveryRecord`, a log line, a metric label, or a `Failure.Detail`.
- The call is bounded by the existing `PublishTimeout`, which `Config.validate`
  already requires to be shorter than the lease, so a slow destination cannot
  outlive its lease and be delivered twice after reclaim. No second timeout knob.
- Delivery provenance in the destination package's own ledger
  (`migrations/0002_https_delivery_provenance.sql`). Server-owned columns plus
  `http_status_class`, which is this process's own closed-vocabulary reduction of
  the response rather than the destination's status line. Destination-derived
  facts carry `_advisory` and a `COMMENT ON COLUMN`; the outcome CHECK lands
  `NOT VALID`.
- `TestDeliveryDispatch_ContactsNoDestination` is **narrowed, not inverted**: same
  name, same place in `make delivery-identity`, same arity-2 guard, and a doc
  comment saying it now proves this of a `kafka`-class destination.
- The Kafka dependency for HTTPS-only deployments is **decided**: kept and
  documented, with a named follow-up, because the registry is one file read at
  boot and relaxing it would trade a startup configuration error for a runtime
  dead letter.

The kill-test
`TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity` drives
PostgreSQL 16, Kafka, five TLS servers, and one redirect-target listener over six
destinations in one registry and one production submission. Its negative control
— a router that unconditionally reports "not mine" — runs in the same invocation
and must fail four named assertions while the kafka-class assertion still passes.

Deferred: mTLS to destinations and per-destination client certificates, a
FHIR-class destination (5.1's prerequisite), multi-tenant destination registries,
and GraphQL destination authoring.

#### Slice 4.1d C1: audit immutability and export attribution

- Extend schema-level immutability from the six already-guarded catalog and
  session tables to the durable-runtime audit records: blanket
  `BEFORE UPDATE OR DELETE` guards on `integration_canonical_events`,
  `integration_message_lineage`, `integration_delivery_audit`,
  `integration_delivery_operations`, and `integration_batch_audit`;
  column-scoped `BEFORE UPDATE` guards plus blanket `BEFORE DELETE` on the two
  state tables `integration_receipts` and `integration_delivery_attempts`, so
  identity, provenance, and attribution freeze while the delivery state machine
  keeps advancing status, attempt count, schedule, and error columns.
- Make every session export an attributed disclosure: `principal_json`,
  `reason`, and `include_raw_payload` become `NOT NULL` on
  `integration_session_exports`, the verified caller identity is threaded from
  `requestsecurity.SecurityContextFromContext`, `reason: String!` is added to
  `ExportIntegrationBundleInput`, and export rows are themselves append-only.
- Gate raw payloads behind the new dotted grant `integration.phi.export`,
  refused before any bundle is assembled and recorded on the export row.
- Publish `docs/operations/PHI-RETENTION.md` — the truthful, citation-backed
  retention posture — and correct this plan's 4.1 bullet above.
- Keep retention/TTL columns and the durable purge component in **S3-C2**.

Implementation status (2026-08-08): implemented and locally verified. The day-1
riskiest-assumption gate passed both assertions before any migration was
written: a fully contract-valid `encrypted` raw-retention submission is rejected
with `ErrUnsupportedRawRetention`, and a successful ephemeral submission leaves a
PHI-classified canonical event in a table with zero ttl/expires/retention/purge
columns. The load-bearing kill-test
`TestPhiAudit_PostgresImmutableRecordsAndAttributedExport` runs on PostgreSQL 16
with `-race` and carries a negative control on a second, independently
provisioned pre-migration schema where every `UPDATE`/`DELETE` succeeds and
exports are written with no principal and no reason. The negative control caught
two real defects in the proof itself: `DELETE` aimed at rows with dependents was
being blocked by pre-existing `ON DELETE RESTRICT` foreign keys rather than by
the new guards, and an `UPDATE` against an empty `integration_delivery_audit`
was passing vacuously. Both are now structurally impossible.

### Slice 4.2: operator control plane

- Real message/trace browser, deployment/channel controls, replay/resubmit/DLQ,
  actor reason capture, and policy-aware semantic payload rendering.
- Proof: failure/replay and operator-audit golden journeys pass without SQL/manual
  filesystem intervention.

Delivered as two sequential merge requests. Plan:
`.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`.

#### Slice 4.2a: control-plane GraphQL API

- `internal/integration/operator` adds a tenant-scoped, PHI-minimal read surface
  over the existing Slice 2.3 delivery tables and Slice 2.1 lifecycle catalog,
  with keyset pagination, opaque cursors, a default page size of 25, and a hard
  server-side maximum of 100.
- Canonical event payloads render structurally. An operator sees dotted field
  coordinates, JSON kinds, and repetition flags; never a stored value, never a
  value length, and never a caller-influenced map key (those collapse to `*`).
- Control mutations `replayDelivery`, `resubmitMessage`, `discardDeadLetter`,
  `pauseIntegrationDeployment`, `resumeIntegrationDeployment`,
  `retireIntegrationDeployment`, and `deployIntegrationRelease` all require a
  nonempty actor reason, use the verified OIDC identity as the actor, enforce
  explicit privileged roles, and delegate to the existing durable machinery.
  DLQ requeue is `replayDelivery`, which already refuses anything that is not an
  active dead letter.
- Roles are least-privilege and additive: `integration.operator` for every read,
  plus `integration.delivery.operator` for delivery recovery (the Slice 2.3
  constant, still enforced a second time inside the durable operation) and
  `integration.deployment.operator` for lifecycle commands.
- Migration `0003_operator_control_plane` extends the delivery operation-kind and
  audit event-kind constraints for the new `discard` decision and records a DLQ
  resolution so a closed dead letter states why it closed.
- Required CI job `test:operator-control-plane` runs the PostgreSQL 16
  race-enabled kill-test with `allow_failure: false`.

Implementation status (2026-08-08): implemented and locally verified; landing
pipeline evidence pending. The kill-test drives the real GraphQL handler with a
Slice 4.1a OIDC operator token and proves: the failure/replay and operator-audit
journeys complete with no SQL; replay records one requeued attempt plus one
append-only audit row carrying actor and reason and one operation-ledger row
carrying the idempotency key; a repeated mutation with the same key leaves
durable state byte-identical and appends nothing; a reused key for a different
action is refused; resubmit forks exactly one idempotent child; discard closes a
dead letter attributably without requeueing; stale expected versions surface as
conflicts while pause/resume record the verified actor and reason; an
unprivileged role is refused before any resolver data; another tenant's failure
is invisible, unmutatable, and unchanged; and a raw-PHI sentinel proven present
in the durable payload appears in no GraphQL response. A negative control
confirmed the sentinel assertion is not vacuous: emitting scalar values from the
payload summarizer makes the job fail.

The required delivery-reliability proof caught one real defect before merge: the
DLQ resolution label for resubmit was derived by string concatenation and wrote
an invalid value. The kill-test now covers that path directly.

#### Slice 4.2a evidence

MR `!136` merged as `7111cca1`. The required merge-request pipeline `22548`
passed 38/38, including the new `test:operator-control-plane` job, the blocking
manual `test:benchmark`, `lint:gqlgen`, and every security gate. Post-merge main
pipeline `22560` passed 42/42.

#### Slice 4.2b: operator UI

- New `/operator` area in the IDE consuming 4.2a: a durable message browser with
  server-owned filters and cursor paging, receipt-to-delivery trace drill-down
  (events rendered as field coordinates only, lineage, attempts, audit), a
  dead-letter and destination-circuit console, and deployment controls.
- Every mutating action routes through one reason-required dialog. Delivery
  recovery derives a stable idempotency key from the action, target, and reason,
  so repeating an identical intent is a server-side no-op while a changed reason
  produces a new key rather than reusing a spent one.
- Toast budget: inline homes for every failure, `isErrorToasted` is unnecessary
  because the feature never re-toasts a GraphQL failure the global net already
  raised; controls the server would refuse are disabled with an explanatory
  `title` (B2) instead of firing and being rejected.
- Expected-version conflicts and resolved dead letters are classified as stale
  views: the panel reloads the durable record and instructs the operator to
  re-decide rather than retrying silently.
- No schema change: the UI consumes exactly what 4.2a shipped. No new npm
  dependency; `npm audit --audit-level=high` is clean.

### Slice 4.3: truthful observability and multi-replica behavior

- Real `/health`, `/ready`, `/metrics`, correlation-safe logs/traces, durable
  subscription fanout, leader/lease rules, and cardinality/PHI budgets.
- Proof: two replicas process/stream without in-memory fanout loss or duplicate
  durable acceptance/outbox records; transport retries preserve one identity.
- **Status: delivered (Lane S3-A).** Spec `.loom/31-sprint3-execution-specs.md`;
  decision `.loom/40-decisions.md` (2026-08-08); handoff
  `.loom/slice-handoff-phase-4-slice-4-3-observability.md`.

**What the plan text got wrong, corrected from code before implementation.** The
riskiest assumption above — "without in-memory fanout loss" — treats the session
hub as the only multi-replica defect. It is one of five. `.loom/31` correction 10
enumerates them; four were fixed and one is now documented instead of surprising:

| Component | Pre-slice behaviour | Outcome |
|---|---|---|
| Session SSE hub | Process-local map; a subscription on replica A never saw a run on replica B | Fixed: envelope-only durable log + per-replica relay |
| Batch worker identity | `FI_FHIR_BATCH_WORKER_ID` required, and `.env.example` plus the operations doc published one shared literal, so two replicas stole each other's live leases | Fixed: derived `hostname-pid`; documentation no longer publishes a value |
| Autoroute notifier | Per-process `seen` map: N replicas paged reviewers N times, every restart re-paged the backlog | Fixed: durable `notified_at` claim |
| Autoroute sweeper | Two replicas run one idempotent guarded `UPDATE` | Accepted: benign duplicate, documented |
| MLLP capacity | One gate per process, so N replicas serve N× the declared `CapacityPolicy` | Documented as per-replica; durable token bucket deferred to 4.4 |

**Also delivered.** Truthful probes in both deployment paths (the Kubernetes
manifests probed `exec ["/fi-fhir","version"]`; the Helm chart pointed both
probes at the same literal `/health`); a rewritten Grafana dashboard and 10
actionable alert rules replacing 32 that targeted metrics nothing emitted; the
`Observe` seams the MLLP service, delivery dispatcher, batch runner, and session
hub never had; the first production caller of `PostgresCatalog.ReportHealth`; and
replacement of the two dead e2e tests that asserted the desired behaviour behind
assertions that could neither run nor fail.

**Deferred to 4.4**, deliberately and in the lane's non-goals: the OpenTelemetry
trace exporter (`FI_FHIR_TRACING_*` stays inert and is now labelled "not
implemented" rather than implying an exporter), structured logging in the serve
path (`log/slog` appears nowhere; correlation IDs are plumbed through records but
not emitted in logs), load-generated cardinality and latency budgets, and a
per-deployment durable MLLP token bucket.

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

**Sequencing dependency (recorded 2026-08-08, Sprint 4 Lane S4-D): 5.1 is blocked
on Slice 4.1c-b.** The durable integration engine produces no FHIR resource
today. `pkg/fhir` has exactly two non-test importers — `internal/workflow/actions.go:23`
(the legacy engine; mapper constructed at `:680`, action registered only at
`internal/workflow/engine.go:127`) and `cmd/fi-fhir/main.go:49` (the `fhir validate`
CLI, `:309`). The durable processor's only use of `internal/workflow` is a planner
that "never invokes transforms or actions" (`internal/workflow/plan.go:144`), and
the dispatcher's command carries the canonical event, not a resource
(`internal/integration/delivery/dispatcher.go:246-259`;
`internal/integration/delivery/store.go:107,128`). Consequently:

- **"Surface conformance policy in artifacts and diagnostics" cannot be specified
  until 4.1c-b defines what a FHIR destination is.** Conformance policy attaches
  to a destination revision, and the destination revision has no FHIR class:
  `TransportKind` is `kafka|https` only
  (`internal/integration/destination/revision.go:54-62`).
- Package pinning and validator integration are unblocked in principle but would
  certify a path no golden journey executes. Golden journey 1 ends "inspect trace
  and FHIR delivery" and journey 6 validates a US Core 9.0.0 R4 payload
  (`.loom/20-product-spec-integration-engine-ide-completion.md:293,302-304`);
  both wait on 4.1c-b.
- Preparation completed in Sprint 4 with no code:
  `docs/planning/FHIR-CONFORMANCE-MATRIX.md` (coverage matrix against
  `pkg/fhir/mapper.go` and `pkg/fhir/validate.go`), the rewritten
  `.loom/28-spec-fhir-ig-bulk-smart.md`, and the proposed validator decision in
  `.loom/40-decisions.md` ("2026-08-08: FHIR conformance validation strategy").
  The decision is **proposed, not ratified**; ratify it before any CI-image
  change.

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

1. Production MLLP adapter consuming the deployed exact release.
2. Durable delivery attempts, replay, and one real queue transport.
3. Runtime-wired S3/SFTP ingestion with checkpoint/resume.
4. Restart-safe Integration Session workspace.

## Scope controls

- One vertical slice per MR.
- Each slice states scope in/out, acceptance criteria, rollback, and exact tests.
- Generated files update only through canonical generation commands.
- Documentation and plan state update with the implementation.
- A slice is not done until local gates and terminal CI are green or a precise
  exception is recorded.
