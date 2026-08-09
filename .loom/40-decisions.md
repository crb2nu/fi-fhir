# Decisions

Record decisions as they are made, with date, rationale, and sources.

## Template

### YYYY-MM-DD: Decision title

- Decision:
- Rationale:
- Alternatives considered:
- Consequences:
- Sources:
  - [S1] …

### 2026-02-11: Use Incremental Contract-First Enhancement Strategy

- Decision:
  - Enhance ETL/parsing/transform/auditability incrementally on the current architecture, starting with API contract governance and drift checks.
- Rationale:
  - Existing parser tolerance, transform engine, event store, and replay capabilities are already mature enough to extend without a rewrite.
  - Current contract drift signals immediate risk that can be reduced quickly with compatibility gates.
- Alternatives considered:
  - Full ingestion platform rewrite (rejected for delivery and migration risk).
- Consequences:
  - Near-term investment in compatibility tooling, audit envelope design, and ETL persistence.
  - Lower migration risk and faster path to production hardening.
- Sources:
  - [S1] `internal/parser/hl7v2/parser.go:160`
  - [S2] `internal/workflow/transforms.go:56`
  - [S3] `pkg/eventsourcing/store.go:2`
  - [S4] `api/openapi.yaml:541`
  - [S5] `internal/api/graphql/schema.graphql:12`
  - [S6] `docs/STATUS.md:39`

### 2026-03-01: Adopt Control-Plane + Runtime Split for Cross-Service Integration

- Decision:
  - Integrate sibling repos with explicit role boundaries: `flexinfer` and `mentatlab` as runtime-adjacent integrations, `loom-core` as control-plane automation/operations integration.
- Rationale:
  - This aligns with existing stable API surfaces and avoids coupling clinical runtime paths to orchestration internals.
- Alternatives considered:
  - Direct point-to-point integration among all services (rejected due to contract and ops drift risk).
- Consequences:
  - Requires explicit integration policy docs and per-edge auth/timeout standards.
  - Enables phased rollout without architectural rewrite.
- Sources:
  - [S1] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
  - [S2] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
  - [S3] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
  - [S4] `/Users/cblevins/workspace/services/loom-core/docs/API_STABILITY.md:72`

### 2026-03-01: Treat Contract Drift Gate Promotion as M0 Exit Criterion

- Decision:
  - Promote `lint:contracts` from soft-fail to blocking after a short clean-run burn-in, and make this the formal M0 exit criterion.
- Rationale:
  - Contract tooling is already implemented and wired in Makefile/CI; enforcing it closes a known governance gap.
- Alternatives considered:
  - Keep permanent warning mode (rejected; does not prevent incompatible drift).
- Consequences:
  - Short-term CI friction may increase.
  - Medium-term API stability and client confidence improve.
- Sources:
  - [S1] `scripts/check_event_contracts.go:40`
  - [S2] `Makefile:203`
  - [S3] `.gitlab-ci.yml:320`
  - [S4] `.gitlab-ci.yml:325`

### 2026-03-01: Record Codebase Index Unavailability as Planning Constraint

- Decision:
  - Continue planning with shell/file evidence while codebase-memory indexing remains unavailable (`total_chunks: 0`), and track index recovery as an enabling task.
- Rationale:
  - Index attempts currently do not progress (0 files discovered), so semantic search is unreliable in this repo context.
- Alternatives considered:
  - Block planning until indexing works (rejected; delivery would stall).
- Consequences:
  - Higher manual effort for evidence gathering.
  - Need explicit checklists to keep sourcing reproducible.
- Sources:
  - [S1] Tool output: `mcp__loom__codebase_memory__codebase_stats(repo_id='fi-fhir')`
  - [S2] Tool outputs: `codebase_index_start/poll/cancel` (`job_id=4f93c59a0acaa0a1`)

### 2026-03-16: Ship Terminology Decision Telemetry Through the CLI Before Analytics/UI Work

- Decision:
  - Land a narrow CLI telemetry slice now: record `terminology mapping resolve` decisions into `mapping_decisions`, and expose read-only decision list/detail/stats commands before tackling OTel polish or UI analytics.
- Rationale:
  - The persistence layer and workflow telemetry path already exist, so CLI parity is a small, high-leverage gap that improves auditability without expanding the workflow surface.
  - This keeps M2 moving with a backward-compatible increment and gives operators a concrete inspection path for clinical mapping decisions.
- Alternatives considered:
  - Jump directly to UI analytics or OpenTelemetry enrichment (rejected; broader scope and less immediately useful for CLI/operator workflows).
- Consequences:
  - Decision telemetry becomes easier to validate and troubleshoot from the terminal.
  - OTel spans, partitioning, and analytics dashboards remain explicit follow-up work.
- Sources:
  - [S1] `docs/planning/README.md:16`
  - [S2] `docs/planning/TERMINOLOGY-MAPPING.md:14`
  - [S3] `pkg/terminology/db/mappings.go:1059`
  - [S4] `cmd/fi-fhir/terminology.go:1490`

### 2026-03-29: Make Debug Sessions Truthful Before Adding More Debug Surface

- Decision:
  - Replace the branch's always-mock debug-panel behavior with real GraphQL-backed session control, start backend debug sessions in stepping mode by default, and derive lightweight trace/lineage UI state from recorded steps until server-side trace queries are implemented.
- Rationale:
  - The backend debug mutations already exist, but the UI was still sending empty workflow input and loading mock state on mount, which made the feature look integrated while preventing real debugging.
  - Default stepping gives the panel a usable first pause without requiring pre-seeded breakpoints, matching how users expect "start debug session" to behave.
- Alternatives considered:
  - Keep mock data until full trace/subscription support exists (rejected; misleading integration state).
  - Add a larger backend breakpoint/trace API expansion first (rejected for this branch-finishing pass; higher scope than needed to make the current stack usable).
- Consequences:
  - The debug panel now reflects real workflow draft input and session lifecycle.
  - `workflowRunTrace` and `debugStepEvent` remain explicit follow-up work rather than silently implied capabilities.
- Sources:
  - [S1] `ui/src/lib/features/debug/debugApi.ts`
  - [S2] `ui/src/lib/features/debug/DebugPanel.svelte`
  - [S3] `ui/src/lib/features/debug/debugStore.ts`
  - [S4] `internal/workflow/debug.go`
  - [S5] `internal/api/graphql/resolvers/debug.resolvers.go`

### 2026-03-30: Make Debug Session Streams Return the Next Pause, Not the Current One

- Decision:
  - Drain stale paused-step notifications before servicing `debugStep`/`debugContinue`, and short-circuit all future debug spans once a session is marked stopped.
- Rationale:
  - Starting sessions in default stepping mode leaves the current paused step buffered; without draining it, the control mutation can return the already-visible pause while subscriptions emit the newly reached pause, splitting the API contract.
  - Stopped sessions may still enter later spans while the workflow unwinds, so the tracer must refuse to pause again or close can hang indefinitely.
- Alternatives considered:
  - Adjust only the subscription test expectations (rejected; would preserve inconsistent runtime behavior).
  - Remove buffered step delivery entirely (rejected; existing direct debug-session tests and synchronous stepping behavior still rely on it).
- Consequences:
  - `debugStep`, `debugContinue`, and `debugStepEvent` now agree on the same "advance to next pause" semantics.
  - Session shutdown is robust even if the engine continues traversing spans after a stop command.
- Sources:
  - [S1] `internal/workflow/debug.go`
  - [S2] `internal/api/graphql/resolvers/debug_subscription_test.go`
  - [S3] `internal/workflow/debug_test.go`

### 2026-06-18: Use Pinned golangci-lint Image for CI Go Lint

- Decision:
  - Run `lint:go` in the pinned official `golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine` image instead of compiling golangci-lint from source in every MR pipeline, and give that job 2 CPU / 4 GiB plus a 30-minute lint timeout for cold-cache package loading.
- Rationale:
  - MR `!80` first failed because `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0` spent the full 1-hour job timeout downloading/building the linter dependency graph before linting began.
  - After moving to the pinned image, the job reached `golangci-lint run` but the 1 CPU / 15-minute configuration still timed out during cold-cache package loading.
  - The image is still version-pinned and is available through the workspace Harbor Docker Hub cache.
- Alternatives considered:
  - Increase the `lint:go` timeout (rejected as slower and still cache-fragile).
  - Make `lint:go` soft-fail (rejected because it weakens a blocking merge gate).
  - Redesign all Go cache keys in this slice (deferred; broader CI platform change).
- Consequences:
  - Fresh branches avoid source-building the linter before every run and have a realistic package-load budget.
  - If the pinned image ever lags the repo Go directive, the rollback is to a prebuilt internal lint image or a longer source-build job.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] GitLab job `142164` trace: `lint:go` timed out after 1h while downloading golangci-lint dependencies.
  - [S3] Command: `docker manifest inspect registry.harbor.lan/dockerhub-cache/golangci/golangci-lint:v2.8.0-alpine`

### 2026-07-12: Put a Secure, Reproducible Baseline Before Completion Features

- Decision:
  - Gate all engine/IDE completion work behind Go 1.26.5, a Go-1.26-compatible
    pinned linter, pinned scanners, and strict validation/quoting for
    configuration-controlled SQL identifiers.
- Rationale:
  - A deployed-build pipeline contained reachable standard-library
    vulnerabilities and a HIGH/HIGH SQL injection while advisory jobs still
    allowed a green pipeline. Additional review found the same identifier class
    in IDE-authored workflow actions.
- Alternatives considered:
  - Defer security to release-candidate hardening (rejected; it would build new
    schemas and ingress on a known unsafe/unreproducible baseline).
- Consequences:
  - Gate 0A precedes the shared runtime spine. Gate 0B promotes proven security
    jobs to required merge gates and reconciles remaining CI truthfulness.
- Sources:
  - [S1] GitLab pipeline `15878`
  - [S2] `cmd/fi-fhir/eventstore.go`
  - [S3] `internal/workflow/event_store.go`
  - [S4] `internal/workflow/database.go`
  - [S5] https://go.dev/doc/devel/release

### 2026-07-12: Make One MessageProcessor the Product Boundary

- Decision:
  - Production adapters, GraphQL submit, and Integration Session preview must use
    one `MessageProcessor` semantic path. Preview is explicitly side-effect-free;
    production adds durability/delivery around the same parse/route result.
  - Golden Path 001 is a blocking kill-test before MLLP, connector breadth, or
    deeper IDE work.
- Rationale:
  - Current interactive and headless paths compose different primitives. A second
    preview engine would inevitably drift from production profile and routing
    behavior.
- Alternatives considered:
  - Keep the Integration Session runner separate and compare output in tests
    (rejected; tests cannot make duplicate orchestration a stable product contract).
  - Rewrite the parser/workflow kernel (rejected; the existing primitives are
    mature and the gap is composition/lifecycle).
- Consequences:
  - The first alpha optimizes for one proven HL7v2 vertical slice, not connector
    count. The program stops and redesigns the boundary if parity, duplicate, or
    restart evidence fails.
- Sources:
  - [S1] `internal/api/graphql/resolvers/schema.resolvers.go`
  - [S2] `internal/integration/session/runner.go`
  - [S3] `internal/ingest/http.go`
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md`

### 2026-07-12: Fix Tenancy, Identity, Secret, and PHI Contracts Before Schemas

- Decision:
  - Slice 1.0 defines a single security domain per 1.0 deployment, required
    logical tenant/actor propagation, secret references, PHI classification,
    raw-retention policy, encryption/TTL/audit fields, and production/preview
    modes before receipt and trace migrations are written.
- Rationale:
  - These fields define primary keys, access predicates, audit semantics, and
    retention behavior. Retrofitting them after durable ingestion would risk PHI
    leakage, destructive migrations, and incompatible receipts.
- Alternatives considered:
  - Add auth/RBAC/PHI only in the operations phase (rejected; enforcement UI can
    come later, but persistence boundaries cannot).
  - Claim shared multi-tenant hosting in 1.0 (rejected until isolation tests and
    operational ownership are proven).
- Consequences:
  - Every durable runtime/artifact type is tenant- and actor-aware from creation.
    Fine-grained UI RBAC and shared-hosting certification remain later slices.
- Sources:
  - [S1] `.loom/20-product-spec-integration-engine-ide-completion.md`
  - [S2] `.loom/30-implementation-plan-integration-engine-ide-completion.md`

### 2026-07-13: Make Integration Revisions Public and Content-Addressed

- Decision:
  - Put the dependency-light product boundary in public `pkg/integration` and
    keep the future orchestrating implementation in `internal/integration/processor`.
  - Bind integration definitions to exact source, profile, workflow, and
    destination revisions with SHA-256 digests. Treat destination, secret, and
    role collections as sets for semantic digesting; exclude only the revision
    digest from its own preimage.
  - Keep source bytes in an unexported, defensive-copy envelope field and make
    preview side-effect freedom a validated result invariant. A sandbox may be
    planned in preview, but only an audited production request may execute it.
  - Construct processed events through a package-owned registry of concrete
    canonical event schemas, stripping raw fields and parser warning text before
    serialization. Construct diagnostics from a safe code-to-message catalog;
    reject duplicate or noncanonical JSON keys at every contract decode boundary.
  - Bind production results back to the authenticated request and require exact
    source-message, correlation, route, action, event, and attempt lineage; cap
    encrypted receipt expiry at the revision policy TTL.
  - Pin Kubernetes 1.36 as the 1.0 deployment minor while keeping installation,
    upgrade, recovery, performance, and standards claims behind explicit gates.
- Rationale:
  - Ingress, GraphQL, the IDE, persistence, and SDKs need one stable contract
    without importing internal profile/workflow/session storage models.
  - Content addressing detects mutation and prevents a published integration
    from silently following mutable “current” pointers.
  - Raw bytes and inline secret values are unsafe serialization defaults for PHI
    workloads; preview must be safe by construction, not caller convention.
  - Internal consistency is insufficient for audit: a self-consistent result
    could otherwise substitute an actor, reason, correlation, or explicit
    idempotency key from the request.
  - Creation actor, reason, and time are integrity-bearing audit facts, so the
    revision digest protects them along with the artifact and policy bindings.
  - A pinned deployment target makes later performance and recovery evidence
    comparable while the evidence-status labels prevent premature support claims.
- Alternatives considered:
  - Reuse Integration Session DTOs (rejected because they contain raw samples,
    nondeterministic diagnostics, and prototype retention behavior).
  - Reuse GraphQL profile/workflow store records (rejected because their
    revision identifiers differ and they lack tenant/content-digest contracts).
  - Hide all contracts under `internal` (rejected because adapters and SDK/API
    serializers need a deliberate stable boundary).
  - Pin the current k3s 1.33 cluster (rejected because it is outside the selected
    upstream-supported 1.0 target and has not run the release proof).
- Consequences:
  - Slice 1.1 must adapt existing parser/workflow/session representations into
    these contracts and prove exact artifact resolution after later revisions
    publish.
  - Slice 1.2 can design receipts and outbox schemas without retrofitting tenant,
    PHI, secret, or revision identity.
  - Changing semantic digest rules or the Kubernetes minor is now a versioned,
    dated compatibility decision.
- Sources:
  - [S1] `pkg/integration/revision.go`
  - [S2] `pkg/integration/contracts.go`
  - [S3] `internal/integration/session/types.go`
  - [S4] `internal/api/graphql/store/profile_store.go`
  - [S5] `internal/api/graphql/store/workflow_lifecycle_store.go`
  - [S6] `docs/operations/SUPPORTED-1.0.md`
  - [S7] https://kubernetes.io/releases/1.36/

### 2026-07-13: Resolve Executable Artifacts by Stored Revision and Digest

- Decision:
  - Preserve the Source Profile serial revision ID as its immutable identity and
    add `source_profiles.current_revision_id` as the mutable pointer. Creation
    writes the initial revision; update locks the pointer, writes the incoming
    revision, then advances it transactionally.
  - Install database compatibility triggers with the schema expansion so a
    pre-upgrade pod that writes the legacy mutable row during a rolling rollout
    still creates or advances the immutable revision. New store code uses the
    same trigger-owned invariant instead of a parallel write algorithm.
  - Treat legacy profile `version` strings as display labels, not identity or
    uniqueness keys. Backfill each legacy current row once without deleting its
    existing history.
  - Hash profile executable content as domain-separated canonical JSON and
    workflow executable content as domain-separated exact UTF-8 YAML bytes.
    Canonicalize equivalent decimal/exponent number spellings independently of
    PostgreSQL `JSONB`; reject duplicate JSON keys, invalid Unicode, malformed
    identities, wrong owners, and digest mismatches before returning content.
  - Keep `internal/integration/processor` storage-neutral. A narrow adapter in
    the existing GraphQL store package performs exact owner-and-revision reads;
    the resolver is configured for one deployment tenant and returns only
    defensive copies.
  - Serialize workflow version allocation by locking its definition row and
    require publication to prove version ownership.
  - Make the PostgreSQL v1-after-v2 restart proof a required CI job independent
    of the broad soft-failing integration suite.
- Rationale:
  - A content-addressed integration is not immutable if its runtime follows a
    mutable profile or release pointer.
  - PostgreSQL stores profiles as `JSONB`, so semantic JSON identity is stable
    across key order/whitespace; workflow YAML is stored as text, so exact bytes
    are the honest compatibility boundary.
  - Loading by a global version ID and checking ownership afterward crosses the
    storage security boundary too late. Exact owner-and-version lookup avoids
    exposing another workflow's bytes even transiently.
  - One deployment tenant makes the 1.0 isolation claim explicit without
    pretending the legacy authoring tables already provide shared-hosting RLS.
  - The supported chart rolls two replicas by default, so a backfill without a
    legacy-write compatibility path could immediately create a null or stale
    current pointer after migration.
- Alternatives considered:
  - Resolve the current profile/workflow at execution time (rejected; published
    integrations would silently change behavior).
  - Snapshot profile JSON and workflow YAML inside every integration revision
    (rejected; duplicates artifacts and weakens shared lifecycle/audit history).
  - Use profile version labels as immutable IDs (rejected; legacy updates can
    reuse a label and existing data must remain migratable).
  - Require a quiesced/recreate-only migration (rejected for the normal rolling
    path; compatibility triggers make the expansion safe while a later release
    may still benchmark the migration lock window).
  - Import GraphQL store records directly into the processor (rejected; reverses
    the application/storage dependency and makes alternate adapters harder).
- Consequences:
  - Digest domain prefixes and canonicalization rules are versioned contracts;
    changing them requires an explicit compatibility migration.
  - The profile backfill briefly locks `source_profiles`; compatibility triggers
    protect writes from old pods after the lock, while large production tables
    still need migration-duration evidence before the 1.0 upgrade claim closes.
  - Slice 1.1b may consume only exact resolved content and must fail production
    closed until Slice 1.2 supplies a durable committer.
- Sources:
  - [S1] `internal/api/graphql/store/profile_store.go`
  - [S2] `internal/api/graphql/store/workflow_lifecycle_pg_store.go`
  - [S3] `internal/api/graphql/store/artifact_revision_loader.go`
  - [S4] `internal/integration/processor/revisions.go`
  - [S5] `internal/integration/processor/revisions_integration_test.go`
  - [S6] `.gitlab-ci.yml`

### 2026-07-13: Keep Preview Pure and Put Authentication Before Adapters

- Decision:
  - Compile published workflows through an explicit DSL v1 grammar with closed
    fields, action types, document shape, and resource limits. Preserve exact
    immutable YAML bytes as the artifact identity while projecting only route,
    action identity, and destination binding into a pure planner.
  - Compile only the explicitly supported Source Profile subset for HL7v2 ADT
    A01. Reject unsupported authored behavior instead of silently using a
    default, and construct a fresh strict parser for every request.
  - Derive event identity from tenant, exact integration revision, source,
    MSH-10, source digest, event type, and ordinal. Exclude request correlation
    and clocks so repeated and concurrent preview is byte-deterministic.
  - The preview processor owns no handler, resolver, session store, destination
    client, action handler, or clock. Production fails before loader access
    until a durable committer exists.
  - Insert Slice 1.1c before any GraphQL/IDE activation: authenticated tenant and
    principal context, POST-only raw payloads, bounded bodies, explicit HTTP
    origins, disabled WebSocket transport, adapter parity, and fail-closed legacy submit/session
    paths are prerequisites to exposing the kernel.
- Rationale:
  - Legacy workflow YAML and engine dry-run semantics are permissive and
    execution-capable; importing them directly would make preview depend on
    handlers, suppress CEL failures, and expose secret-bearing action config.
  - Parser wall time, permissive framing, and caller-supplied integration
    revisions would make audit identity nondeterministic or attacker-selected.
  - The existing GraphQL surface does not yet establish an authenticated
    tenant/principal boundary, permits credentialed origin reflection and broad
    WebSocket origins, and accepts queries over GET. Wiring only one preview
    resolver would leave adjacent raw-PHI and execution paths unsafe.
- Alternatives considered:
  - Reuse `Engine.DryRun` (rejected because it still owns execution-capable
    dependencies and its route semantics hide some CEL failures).
  - Accept the full legacy Source Profile schema and ignore unsupported fields
    (rejected because a published profile could claim behavior the runtime does
    not execute).
  - Disable only the GraphQL submit resolver in this slice (rejected because it
    would not establish identity, origin, method, body, or session containment
    for the rest of the transport surface).
- Consequences:
  - Slice 1.1b is an internal alpha kernel, not a remotely callable product
    feature. Slice 1.1c is now a security gate rather than optional UI wiring.
  - The published workflow/profile subset is a versioned executable contract;
    adding semantics requires tests and an explicit compatibility decision.
  - Slice 1.2 can wrap the same pure evaluation result with receipts,
    idempotency, trace, and outbox state without creating a second engine.
- Sources:
  - [S1] `internal/workflow/published_yaml.go`
  - [S2] `internal/workflow/plan.go`
  - [S3] `internal/integration/processor/profile_compile.go`
  - [S4] `internal/integration/processor/message_processor.go`
  - [S5] `internal/integration/processor/message_processor_integration_test.go`
  - [S6] `internal/api/graphql/server.go`
  - [S7] `internal/api/graphql/resolvers/schema.resolvers.go`

### 2026-07-13: Expose One Preview Capability Behind a Fail-Closed Deployment Boundary

- Decision:
  - Expose one typed `previewIntegrationMessage` mutation. Both the Mapping
    Studio direct preview and its former Integration Session client call it.
  - Load tenant, principal, roles, exact browser origins, one [REDACTED],
    and an immutable integration registry at startup. Missing, ambiguous, or
    inconsistent values prevent `serve` from starting.
  - Grant the transitional `integration:preview` role only `health` and
    `previewIntegrationMessage`. Keep `graphql:operator` as an explicit legacy
    escape hatch, not an IDE credential.
  - Accept GraphQL HTTP only as bounded `application/json` POST, require an
    exact allowed browser origin, require canonical duplicate-free JSON, return
    catalog-safe errors, and do not mount GraphQL WebSocket transport.
  - Keep [REDACTED], imported raw clinical samples, and
    filename-derived source labels in tab memory only. Reloading discards all
    three; **Clear access** discards the bearer. Purge the two legacy PHI-bearing
    localStorage keys during layout startup.
  - Stream bounded GraphQL request bodies through nginx/Ingress without proxy
    temp-file buffering. Compile the non-secret preview registry alias with a
    Vite-prefixed build variable; credentials remain runtime-only.
  - Leave submit, batch, workflow-trigger, parse-preview, session run/sample,
    export, and live-parse operations unavailable by default. Do not mount the
    profile-YAML or generic ingest HTTP bypasses.
- Rationale:
  - A single adapter makes parity with `MessageProcessor` directly testable and
    avoids preserving a second session-specific semantic path.
  - Server-owned identity and registry data prevent callers from selecting a
    tenant, source, profile, workflow, or executable revision.
  - Operation authorization is required in addition to [REDACTED]
    because the legacy schema still includes PHI and execution-capable fields.
  - Browser persistence is not an approved raw-PHI or secret store.
  - Proxy buffering and transport decoder errors are part of the PHI boundary,
    not merely implementation details behind the resolver.
- Alternatives considered:
  - Add a second Integration Session preview mutation (rejected because it
    duplicates the adapter contract and creates drift risk).
  - Make a [REDACTED] the complete legacy schema (rejected because those
    stores are not yet tenant-scoped and several mutations can execute actions).
  - Put the token in a `PUBLIC_*`, localStorage, or sessionStorage value
    (rejected because builds and browser persistence are not secret stores).
- Consequences:
  - Operators must supply the complete preview configuration and a random
    [REDACTED] at least 24 canonical bytes before `serve` starts.
  - This static [REDACTED] a transitional single-security-domain control. OIDC,
    fine-grained RBAC, audited token administration, and durable user sessions
    remain Phase 4 work.
  - Durable receipts, production submit, and delivery remain blocked on Slice
    1.2 even when preview is available.
- Sources:
  - [S1] `cmd/fi-fhir/preview_runtime.go`
  - [S2] `internal/api/requestsecurity/auth.go`
  - [S3] `internal/api/graphql/operation_authorization.go`
  - [S4] `internal/api/graphql/server.go`
  - [S5] `internal/integration/preview/service.go`
  - [S6] `internal/integration/registry/static.go`
  - [S7] `ui/src/lib/graphql/GraphQLCredentialGate.svelte`
  - [S8] `ui/src/lib/features/hl7/samples/sampleStore.ts`
  - [S9] `ui/src/lib/features/hl7/samples/legacyStorage.ts`
  - [S10] `ui/nginx/default.conf.template`

### 2026-07-13: Roll Matching Runtime Images Behind an Automation Barrier

- Decision:
  - Publish API and UI from one successful default-branch pipeline under one
    immutable release tag, then verify both registry manifest digests before
    changing deployment state.
  - Suspend image automation before landing runtime prerequisites. Roll both
    image tags plus their deployment hardening in one reviewed GitOps change.
  - Keep automation suspended until both deployments are Ready on the verified
    image IDs and the public ingress passes auth, origin, containment,
    provenance, suppressed-delivery, and PHI-leakage probes.
  - Resume automation in a separate reviewed GitOps change and verify the
    controller is Ready, the repository is up to date, and the running images
    have not drifted.
- Rationale:
  - The IDE and API are one compatibility boundary. Independent image updates
    can expose a client/server contract mismatch even when both images are
    individually healthy.
  - Suspending automation makes the rollout and rollback set explicit while
    live security probes run against a stable pair of artifacts.
  - A separate resume change preserves evidence that the safety barrier was not
    removed before acceptance passed.
- Alternatives considered:
  - Let Flux update API and UI independently (rejected because policy polling
    and reconciliation do not guarantee an atomic compatible pair).
  - Resume automation in the rollout MR (rejected because the live gate cannot
    run until that MR is applied).
  - Pin digests permanently (rejected because verified immutable tags plus
    observed image IDs retain normal automated release operation).
- Consequences:
  - Coordinated runtime releases require two small GitOps MRs around the live
    gate. The rollback tag is recorded per release rather than becoming policy.
  - A failed probe leaves automation suspended and both workloads on the known
    pair until an explicit rollback or corrected rollout is reviewed.
- Evidence:
  - App MR `!96`; main pipeline `18621`; release tag `v0.1.18621`.
  - GitOps prerequisite MRs `!359`/`!360`, rollout MR `!368`, and resume MR
    `!369`; exact digests and live assertions are in the Slice 1.1c iteration
    plan.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] `.loom/iteration-plan-phase-1-slice-1-1c-authenticated-preview-adapters.md`
  - [S3] `deploy/kubernetes/`
  - [S4] `ui/nginx/default.conf.template`

### 2026-07-14: Make PostgreSQL the Sole Production Admission Authority

- Decision:
  - Keep one `MessageProcessor` evaluation path for preview and production.
    Preview remains SQL-free; production is available only through an explicitly
    configured `PostgresSubmissionStore`.
  - Commit the receipt, sanitized canonical event, exact artifact/trace lineage,
    one initial queued attempt per external action, and one pending outbox row
    per attempt in one fixed-schema PostgreSQL transaction.
  - Arbitrate duplicates with one tenant-scoped unique effective key. An explicit
    key wins; otherwise derive a domain-separated digest from tenant, source,
    MSH-10, and the exact integration revision. Bind that key to a separate
    request fingerprint so changed content fails closed.
  - Treat any `COMMIT` error as outcome-unknown. A retry evaluates the same pure
    plan, loses the unique-key claim if the first commit survived, and returns
    the first stored `ProcessResult` exactly.
  - Keep legacy GraphQL production submit disabled. Slice 1.3 adds the first
    authenticated production ingress on this committer.
- Rationale:
  - A positive acknowledgement cannot safely precede event/outbox durability or
    cross two independent stores.
  - The legacy `pkg/eventsourcing.OutboxEventStore` writes event and outbox in
    separate calls and intentionally ignores an outbox-save failure; it cannot
    provide the Slice 1.2 guarantee.
  - Deterministic IDs make rollback, restart, and commit-unknown behavior
    inspectable, while the unique effective key remains the database authority.
  - Storing the validated raw-free result on the receipt makes duplicate
    responses stable even when retry correlation or receive-time metadata changes.
- Alternatives considered:
  - Reuse `OutboxEventStore` (rejected because it is non-transactional).
  - Persist the receipt first and append events/outbox afterward (rejected
    because it acknowledges a partial admission state).
  - Add a generic committer interface with memory and PostgreSQL variants
    (rejected because production durability must not be accidentally configured
    to an in-memory implementation).
  - Reactivate legacy GraphQL submit in this slice (rejected because its broader
    catalog remains intentionally contained and Slice 1.3 owns ingress policy).
- Consequences:
  - Startup must apply the numbered submission migration before constructing a
    durable processor.
  - Raw retention remains ephemeral-only; encrypted raw storage needs its own
    encrypted store, TTL, purpose, and access-audit implementation.
  - Slice 1.2 guarantees durable acceptance once and seeds at-least-once outbox
    delivery. Polling, leases, retries, DLQ, replay, and external-effect
    idempotency remain Phase 2 work.
- Sources:
  - [S1] `internal/integration/processor/postgres_submission.go`
  - [S2] `internal/integration/processor/migrations/0001_atomic_submission.sql`
  - [S3] `internal/integration/processor/postgres_submission_integration_test.go`
  - [S4] `pkg/eventsourcing/outbox.go`
  - [S5] `.loom/iteration-plan-phase-1-slice-1-2-durable-submission.md`

### 2026-07-14: Bind the First Production Adapter to One Trusted Integration

- Decision:
  - Mount exact `POST /v1/hl7v2` only when an explicit bearer or HMAC-SHA256
    credential is configured with one service principal and one integration ID.
  - Resolve tenant, source, format, classification, and executable revisions
    from deployment-owned state. Caller headers cannot select source identity.
  - Reject browser origins, compression, non-HL7 media, oversized bodies, and
    ambiguous headers before processing. Bound accepted bodies to at most 1 MiB.
  - Share one PostgreSQL-backed `MessageProcessor` between production ingress
    and GraphQL/IDE preview. Production commits; preview remains side-effect free.
  - Return a raw-free `202` projection only after atomic admission. Leave the
    endpoint unmounted when auth mode is unset.
- Rationale:
  - Adapter-specific authentication belongs before the shared clinical kernel.
  - A credential-to-integration binding prevents a sender from claiming another
    source while preserving exact server-owned provenance.
  - One live processor composition makes preview/production semantic drift a
    testable invariant rather than a convention.
- Alternatives considered:
  - Reuse generic `internal/ingest` (rejected because it permits auth-disabled
    configuration and calls the workflow engine outside durable admission).
  - Re-enable legacy GraphQL submit (rejected because its broad catalog and
    browser transport are not the production source boundary).
  - Accept caller-selected source/profile headers (rejected because they break
    provenance and tenant/source authorization).
- Consequences:
  - Operators need separate GraphQL and source-adapter credentials.
  - HMAC signs integration, idempotency, correlation, and exact bounded bytes.
  - `202` means durable admission and queued outbox work, not external delivery.
  - Production GitOps activation remains a separate reviewed operation.
- Evidence:
  - `make golden-path-001` passed 20 duplicate/restart/profile/IDE/leakage
    assertions against PostgreSQL 16 and a real restarted process.
- Sources:
  - [S1] `internal/integration/ingress/`
  - [S2] `cmd/fi-fhir/preview_runtime.go`
  - [S3] `scripts/golden-path-001.sh`
  - [S4] `.loom/iteration-plan-phase-1-slice-1-3-authenticated-http-golden-path.md`

### 2026-07-14: Separate Immutable Releases from Optimistic Deployment State

- Decision:
  - Add an optional, digest-bound deployment policy to
    `IntegrationDefinitionRevision`. Existing Slice 1 revisions omit it and keep
    their exact JSON/digest; lifecycle-managed revisions require connection-
    validation freshness, schedule, health, and capacity policy.
  - Persist definition revisions, connection validations, release records, and
    lifecycle events as database-enforced append-only rows. Keep one small
    snapshot mutable under an expected-version predicate.
  - Use the closed state graph draft -> validated -> approved -> published ->
    deployed -> paused/resumed -> retired. Failed validation is recorded but
    does not advance state; stale validation blocks publish, deploy, and resume.
  - Resolve an exact runnable binding only while the release is deployed. Future
    adapters must begin from that server-owned binding rather than a caller or
    mutable current pointer.
- Rationale:
  - Pause/resume and health are operational facts that change independently of
    tested artifact content. Including them in the revision digest would either
    mutate a release or create a new executable identity for every control action.
  - Append-only validation/release/history records make actor, reason, revision,
    and publication evidence reconstructable after restart.
  - A database expected version and one-active-release constraint make concurrent
    operators deterministic across replicas.
- Alternatives considered:
  - Store state directly on the immutable revision (rejected because pause and
    health would invalidate its content identity).
  - Reuse the startup `StaticRegistry` as the deployment catalog (rejected because
    it has no persistence, concurrency, approval, health, or pause boundary).
  - Introduce lifecycle mutations through the legacy GraphQL workflow store
    (rejected because Phase 3 owns IDE/API controls and that store does not bind
    the full integration revision).
- Consequences:
  - Slice 2.2 can add MLLP without inventing artifact selection or channel state.
  - `serve` and the current authenticated HTTP ingress remain on the verified
    static registry until an adapter explicitly consumes runnable catalog state.
  - Staged/canary rollout and shared multi-tenant hosting remain later work.
- Evidence:
  - MR `!101` pipeline `19014` passed 32/32, including required lifecycle job
    `183463`; merge commit `a95bb44f` repeated it in main job `183702`.
  - The first main run exposed a pre-existing concurrent receipt insert that
    could select the deterministic receipt primary key before the named
    tenant/idempotency constraint. MR `!102` replaced the constraint-specific
    insert with `ON CONFLICT DO NOTHING`; the following tenant/idempotency lookup
    and fingerprint validation remain authoritative and fail closed.
  - MR `!102` pipeline `19045` passed 24/24. Final main pipeline `19052` passed
    26/26 with durable-submission job `183938` and lifecycle job `183940` green.
- Sources:
  - [S1] `pkg/integration/deployment.go`
  - [S2] `internal/integration/lifecycle/`
  - [S3] `.loom/iteration-plan-phase-2-slice-2-1-versioned-deployment-lifecycle.md`
  - [S4] `.gitlab-ci.yml`

### 2026-07-15: Serialize MLLP Admission with Deployment Stops

- Decision:
  - Represent an MLLP listener as a strict, content-addressed UTF-8 source
    revision. The document contains policy and logical secret-binding names,
    never certificate, key, CA, credential, or message bytes.
  - Resolve the lifecycle catalog's deployed binding for every frame. The
    startup registry may supply exact profile/workflow bytes but cannot select
    the executable definition.
  - Repeat exact deployed-release authorization inside the durable submission
    transaction under `FOR SHARE` on the lifecycle snapshot. Pause and retire
    use the existing conflicting `FOR UPDATE` transition lock.
  - Write `AA/CA` only after atomic admission returns an accepted receipt.
    Framing/header/TLS/client failures close without reflection; bounded runtime
    failures receive safe negative codes after a valid header exists.
- Rationale:
  - A preflight runnable lookup alone races an operator stop. Holding the shared
    row lock through commit gives admission and pause/retire one database
    serialization order across processes.
  - Transport acknowledgement means durable acceptance, while downstream
    delivery remains separate at-least-once outbox work.
  - UTF-8 avoids the byte-framing collisions documented for UTF-16/UTF-32.
- Alternatives considered:
  - Select the definition from startup configuration (rejected because pause
    and retirement would be advisory).
  - ACK after parsing or queueing in memory (rejected because process loss could
    discard positively acknowledged work).
  - Hold lifecycle state only in a process cache (rejected because replicas and
    concurrent operators would not share a linearization point).
  - Implement enhanced two-phase commit/application ACK exchange now (deferred;
    v1 supports one configured application or commit response).
- Consequences:
  - MLLP and lifecycle/submission schemas must share PostgreSQL.
  - Profile/workflow artifact storage remains transitional, but artifact
    identity is exact and cannot authorize a non-deployed definition.
  - Plaintext is limited operationally to a protected loopback/sidecar boundary;
    production network exposure requires mutual TLS and reviewed GitOps.
  - Production GitOps activation remains intentionally pending.
- Evidence:
  - Unit and race tests cover framing, ACK safety, TLS, client policy, capacity,
    lifecycle mismatch, and pre-return ACK exclusion.
  - MR `!104` pipeline `19175` passed 33/33; required PostgreSQL 16/TCP job
    `184996` passed. Merge commit `6205fa39` repeated the proof in main job
    `185093`; main pipeline `19193` passed 36/36.
  - CI exposed and closed two test-contract defects before merge: empty
    diagnostics persist as JSON `[]`, and the 32-client duplicate proof now
    declares matching bounded connection/queue capacity.
- Sources:
  - [S1] `internal/integration/mllp/`
  - [S2] `internal/integration/lifecycle/admission.go`
  - [S3] `internal/integration/processor/postgres_submission.go`
  - [S4] `.loom/iteration-plan-phase-2-slice-2-2-production-mllp.md`
  - [S5] `docs/operations/PRODUCTION-MLLP.md`

### 2026-07-15: Keep Durable Acceptance Separate from At-Least-Once Kafka Delivery

- Decision:
  - Extend the Slice 1.2 submission schema forward with expiring outbox leases,
    bounded attempt schedules, destination-revision circuit state, durable DLQ,
    parent attempt links, idempotent operator operations, and append-only audit.
  - Claim due work with PostgreSQL `FOR UPDATE SKIP LOCKED`. A worker may update
    only its live lease; expiration requeues and audits work after restart.
  - Publish sanitized delivery commands to real Kafka with the stable attempt ID
    as record key and lineage headers. Require all in-sync replica acknowledgement
    and TLS whenever SASL credentials are configured.
  - Replay reuses the failed attempt identity. Resubmit creates one deterministic
    child. Both require a PostgreSQL-authenticated operator, unique operation key,
    and non-empty reason.
- Rationale:
  - PostgreSQL and Kafka cannot share the atomic admission transaction. A crash
    after Kafka acknowledgement but before the database success marker must be
    recoverable even though it can repeat a broker record.
  - Stable attempt identity gives consumers a deterministic duplicate-suppression
    key without making a false universal exactly-once claim.
  - Explicit lease/retry/circuit/DLQ state survives process and replica changes;
    the legacy memory/file workflow DLQ cannot satisfy that production boundary.
- Alternatives considered:
  - Treat the existing log queue driver as a real transport (rejected because it
    has no broker acknowledgement or cross-process durability).
  - Mark PostgreSQL published before Kafka acknowledgement (rejected because a
    crash can lose accepted work).
  - Mark only after acknowledgement and claim exactly-once delivery (rejected
    because a publish-before-database-ack crash can repeat the record).
  - Mutate failed rows directly for recovery (rejected because it loses operator,
    reason, idempotency, and parent-lineage evidence).
- Consequences:
  - Consumers must deduplicate unsafe effects by stable attempt ID.
  - The worker is explicitly enabled and can be rolled back without deleting work.
  - UI/GraphQL DLQ browsing remains Phase 3; Slice 2.3 supplies an authenticated
    PostgreSQL CLI recovery surface and backend contracts.
  - Webhook/FHIR/database/file execution and production GitOps activation remain
    separate reviewed work.
- Evidence:
  - MR `!106` pipeline `19226` passed 34/34, including PostgreSQL 16/Kafka job
    `185433`, and merged as `ca968fbf`. Main pipeline `19235` passed 37/37 and
    repeated the proof in job `185505`; evidence MR `!107` reconciled the record.
- Sources:
  - [S1] `internal/integration/delivery/`
  - [S2] `internal/integration/processor/migrations/0002_delivery_reliability.sql`
  - [S3] `cmd/fi-fhir/delivery_runtime.go`
  - [S4] `docs/operations/DELIVERY-RELIABILITY.md`
  - [S5] `.loom/iteration-plan-phase-2-slice-2-3-delivery-reliability.md`

### 2026-07-16: Checkpoint Only After Durable Batch Admission

- Decision:
  - Identify a provider object by a domain-separated hash of source revision,
    provider path, and immutable provider version; persist only the resulting
    hash plus validated size and raw-free metadata. Require S3 bucket versioning
    and address the exact version ID for reads and deletion.
  - Identify each message deterministically from the source revision, object
    hash, pinned integration revision digest, message ordinal, and byte offset.
    Use that identity for the explicit durable-submission idempotency key and
    correlation ID. Refuse to resume a checkpoint under another integration
    revision.
  - Advance the PostgreSQL byte/message checkpoint only after the shared durable
    processor commits. A replica holds an expiring object lease and can reclaim
    abandoned work after the lease expires.
  - Archive by copy to a SHA-256-addressed destination, verify the destination,
    commit completed state/audit, then delete the source. Require S3 version IDs
    for exact deletion. Require SFTP `known_hosts`, immutable atomic publication,
    immediate pre-delete digest verification, and reject symlinked
    source/archive files.
- Rationale:
  - Admission and checkpoint cannot share one provider/database transaction.
    Repeating a deterministic admission after a crash closes the gap without
    pretending cross-system exactly-once delivery.
  - Archive-before-delete favors a recoverable duplicate copy over loss of the
    only verified clinical payload.
- Alternatives considered:
  - Checkpoint before admission (rejected because a crash loses a message).
  - Persist raw paths/provider versions for operator convenience (rejected to
    keep recovery state PHI-minimal and reduce provider detail exposure).
  - Rename SFTP source into archive without verification (rejected because it
    is not portable across filesystems and does not prove copied bytes).
  - Disable SSH host-key verification or learn keys on first use (rejected
    because production ingestion must fail closed against host impersonation).
- Consequences:
  - Recovery is at-least-once at the provider boundary and durable-once inside
    fi-fhir through idempotent admission.
  - Crashes after archive verification may temporarily leave both source and
    archive; the completed-object cleanup path safely retries deletion.
  - Source mutation produces a new object identity rather than inheriting an
    old checkpoint.
- Evidence:
  - MR `!108` pipeline `19331` passed 35/35, including required PostgreSQL 16/
    MinIO/SSH-SFTP job `186259`, and merged as `ed32915f`.
  - Main pipeline `19344` passed 38/38 and independently repeated the proof in
    job `186476`.
- Sources:
  - [S1] `internal/integration/batch/service.go`
  - [S2] `internal/integration/batch/store.go`
  - [S3] `internal/integration/batch/s3.go`
  - [S4] `internal/integration/batch/sftp.go`
  - [S5] `.loom/iteration-plan-phase-2-slice-2-4-batch-ingestion.md`

### 2026-07-16: Bind Session Runs to Append-Only Executable Profile Revisions

- Decision:
  - Persist Integration Sessions through a storage-neutral store with a
    tenant-scoped PostgreSQL implementation for sessions, samples, artifact
    revisions, runs, accepted decisions, and exports.
  - Redact samples before durable storage by default. Permit explicit retention
    only with AES-256-GCM protection bound to tenant/session/sample identity, and
    omit retained raw bytes from exports.
  - Append every artifact save as a new digest-bound revision. A preview run
    records and executes one exact profile revision through the production
    profile compiler; successful and failed terminal runs are immutable.
- Rationale:
  - Restart safety without exact executable provenance would let the IDE claim a
    result that a later mutable profile cannot reproduce.
  - Default redaction keeps the workspace useful for authoring while avoiding an
    implicit durable raw-PHI repository.
- Alternatives considered:
  - Continue with resolver-owned in-memory maps (rejected because sessions,
    decisions, and evidence disappear on restart).
  - Store only the current artifact head (rejected because prior runs would lose
    their executable input).
  - Retain raw samples by default (rejected because authoring convenience does
    not justify durable PHI without explicit policy and key material).
- Consequences:
  - Session execution remains HL7v2/profile-only in Slice 3.1.
  - Streaming, workflow simulation, signed publication, key rotation/expiry,
    fine-grained RBAC, and production GitOps activation remain separate work.
- Evidence:
  - Local focused/race/full tests, vet, scoped lint, UI type checks, and docs
    validation pass.
  - MR `!111` pipeline `19409` passed 37/37, including required PostgreSQL 16
    restart/raw-leakage job `187425`, and merged as `15746ccd`.
  - Main pipeline `19424` passed 40/40 and independently repeated the proof in
    job `187618`.
- Sources:
  - [S1] `internal/integration/session/`
  - [S2] `internal/integration/processor/profile_public.go`
  - [S3] `docs/operations/INTEGRATION-SESSIONS.md`
  - [S4] `.loom/iteration-plan-phase-3-slice-3-1-integration-session-workspace.md`

### 2026-07-16: Stream Session Diagnostics over Authenticated GraphQL SSE

- Decision:
  - Add feature-gated GraphQL SSE on the existing bounded authenticated
    `POST /graphql` endpoint and allowlist only Integration Session subscription
    roots on that transport.
  - Keep `/graphql/ws` unmounted. Subscribe before starting a preview, project
    server-owned run snapshots, and reconcile against the immutable terminal run.
  - Use canonical inspector paths for lineage and exclude persisted raw field
    previews and retained sample payloads from GraphQL stream projections.
- Rationale:
  - SSE supplies low-latency one-way progression without reopening gqlgen's
    unbounded pre-authentication WebSocket frame or permitting mutations over a
    transport outside the bounded POST boundary.
  - Durable terminal snapshots preserve correctness if process-local fanout
    drops an intermediate progress event.
- Alternatives considered:
  - Re-enable GraphQL WebSocket (rejected because the existing transport does
    not expose the required pre-authentication frame bound).
  - Browser polling (rejected because it obscures real stage ordering and adds
    avoidable database load).
  - Add a message broker now (deferred to Phase 4 multi-replica operations).
- Consequences:
  - Both backend and UI feature gates must be enabled, and temporary
    `graphql:operator` authorization remains required.
  - Fanout is process-local; production GitOps activation and durable
    cross-replica replay remain pending.
- Evidence:
  - MR `!115` pipeline `19464` passed 34/34, including required session job
    `187950` and benchmark job `187953`, and merged as `36f2bb8c`.
  - Main pipeline `19482` passed 37/37 and independently repeated the session
    proof in job `188135`.
- Sources:
  - [S1] `internal/api/graphql/server.go`
  - [S2] `internal/api/graphql/operation_authorization.go`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go`
  - [S4] `ui/src/lib/graphql/subscriptions.ts`
  - [S5] `.loom/iteration-plan-phase-3-slice-3-2-streaming-diagnostics-lineage.md`

### 2026-07-18: Simulate Durable Session Events Through the Production Pure Planner

- Decision:
  - Bind each workflow simulation to one append-only session workflow revision
    and an explicit ordered set of successful immutable session run IDs.
  - Reconstruct canonical events only from those server-owned runs and reuse the
    production `workflow.Planner`; record only revision provenance and
    configuration-free route, planned-transform, and action identity traces.
  - Persist simulations in PostgreSQL and compare deterministic trace-key sets.
    Workflow Builder must save the YAML revision before simulation and must not
    send browser event payloads.
- Rationale:
  - A browser-local draft plus browser-supplied JSON cannot prove which exact
    workflow was tested or reproduce the outcome after restart.
  - The pure planner supplies production route semantics without exposing an
    execution-capable handler path to authoring data.
- Alternatives considered:
  - Reuse `SimulationEngine` (rejected because unmocked action types can dispatch
    real handlers).
  - Persist full events, transformed payloads, or action configuration in each
    trace (rejected because it duplicates PHI and secret-bearing configuration).
  - Execute transforms during Slice 3.3 (deferred until a production-pure
    transform planner exists; current production planning reports them as
    planned rather than executed).
- Consequences:
  - Traces survive service restart and can be compared without mutable draft
    drift, while remaining PHI-minimal and side-effect free.
  - Artifact content is serialized as opaque bytes in export snapshots so YAML
    workflow revisions round-trip instead of being misclassified as JSON.
  - Signed publication, approval, deployment, and production GitOps activation
    remain Slice 3.4 work.
- Evidence:
  - Workflow/parser, session/store, GraphQL resolver, and Workflow Builder tests
    pass; the PostgreSQL 16 race/restart kill test restores two simulations and
    proves the expected delta and sentinel exclusion.
  - MR `!122` pipeline `19872` passed 37/37 with session job `191685` and
    benchmark job `191688`; merge commit `d42f7233` passed main pipeline `19878`
    40/40 with independent session job `191786` and benchmark job `191789`.
- Sources:
  - [S1] `internal/workflow/plan.go`
  - [S2] `internal/integration/session/workflow_simulation.go`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go`
  - [S4] `ui/src/lib/features/workflows/components/DryRunPanel.svelte`
  - [S5] `.loom/iteration-plan-phase-3-slice-3-3-workflow-draft-simulation.md`

### 2026-07-18: Sign Verified Production Bindings, Not Session Identities

- Decision:
  - Publish a canonical PHI-minimal manifest that records both the exact session
    revisions tested and the exact production artifact references to deploy.
  - Resolve production bytes through the existing artifact resolver and
    recompute production-domain references from session content; never copy a
    session digest or mutable current pointer into deployment evidence.
  - Verify a detached Ed25519 signature against an explicit trust root before
    lifecycle approval and again immediately before deployment, then reuse the
    existing optimistic lifecycle state graph and immutable release record.
- Rationale:
  - Session revisions use opaque IDs and plain SHA-256, while production profile
    and workflow references use different domain-separated digest rules. One
    identity cannot safely stand in for the other even when content matches.
  - A second deployment state machine would create conflicting release truth and
    bypass existing validation freshness, optimistic version, and active-release
    invariants.
- Alternatives considered:
  - Copy session refs directly (rejected because they are invalid production
    identities and digest domains).
  - Promote current profile/workflow pointers (rejected because later edits could
    silently change the approved executable bytes).
  - Mutate GitOps from the authoring flow (deferred as a separately reviewed
    production operation).
- Consequences:
  - Publication requires an already-validated exact definition and configured
    matching PKCS#8/PKIX Ed25519 keys. Partial configuration fails startup.
  - Publication is append-only, rejects retained-raw fixtures, and performs no
    transform, action, destination, network, or GitOps side effect.
- Sources:
  - [S1] `internal/integration/session/publication.go`
  - [S2] `internal/integration/processor/revisions.go`
  - [S3] `internal/integration/lifecycle/transitions.go`
  - [S4] `.loom/iteration-plan-phase-3-slice-3-4-publish-deploy.md`

### 2026-08-06: Verify GraphQL Callers Through One Exact OIDC Trust Domain

- Decision:
  - Add one long-lived OIDC discovery/JWKS verifier behind the existing
    `requestsecurity.Authenticator` boundary and reuse the established GraphQL
    POST/SSE security-context propagation and operation authorization.
  - Accept only an exact HTTPS issuer, one exact API audience, explicitly
    allowed asymmetric signing algorithms, a protected `typ=at+jwt` access-token
    class, valid expiry/not-before, a nonempty subject, the exact deployment
    tenant claim, and a strict nonempty role array. Map `sub` to a human
    principal with `auth_method=oidc`.
  - Validate discovered JWKS metadata and every outbound request as HTTPS; reject
    redirects; cap network duration/response size and rate-bound outbound
    unknown-key refresh.
  - Make `static` and `oidc` runtime modes mutually exclusive. OIDC rejects the
    compatibility bearer, deployment principal/roles, and trusted-CIDR bypass;
    static mode rejects every OIDC-only setting.
- Rationale:
  - A deployment-owned static principal cannot distinguish human callers or
    support key rotation, while accepting caller-supplied identity without
    signature and tenant proof would break the product's foundational isolation
    contract.
  - Reusing the current authenticator and immutable security context avoids a
    second authorization path and lets the actual GraphQL handler prove that
    verified roles reach the pre-resolver operation boundary.
  - Exact single-audience validation prevents a token issued jointly to another
    relying party from gaining API access merely because it also names fi-fhir.
  - The protected access-token type prevents an OIDC ID token from being
    substituted at the API boundary; this is deliberately narrower than a claim
    of complete RFC 9068 conformance.
- Alternatives considered:
  - Keep rotating a shared preview bearer (rejected because it still collapses
    all human callers into one deployment identity).
  - Parse JWTs without OIDC discovery/JWKS verification (rejected because it
    would duplicate sensitive crypto/key-rotation behavior).
  - Add browser login, service identity, full RBAC, audit, and PHI retention in
    one MR (deferred because those are independent Phase 4 policy and lifecycle
    slices with different kill-tests).
- Consequences:
  - OIDC startup requires reachable standards-compliant HTTPS discovery and
    JWKS endpoints. Remote calls time out after at most 10 seconds, JWKS bodies
    are capped at 1 MiB, and outbound unknown-key refreshes have a 30-second
    default floor; providers should publish keys before issuing rotated tokens.
  - Roles remain coarse compatibility roles until the next authorization-policy
    slice, and checked-in GitOps manifests remain static pending reviewed
    activation.
  - Verification failures expose only the existing generic credential error;
    startup configuration/discovery failures remain operator-visible.
- Sources:
  - [S1] `internal/api/requestsecurity/oidc.go`
  - [S2] `internal/api/graphql/oidc_security_test.go`
  - [S3] `cmd/fi-fhir/preview_runtime.go`
  - [S4] `.loom/iteration-plan-phase-4-slice-4-1a-oidc-graphql-identity.md`

### 2026-08-07: Bind OAuth Service Tokens to One Submit Action and Exact Source

- Decision:
  - Add an `oauth2` production HTTP ingress mode that reuses the bounded OIDC
    discovery/JWKS verifier and projects one distinct service principal per
    allowlisted client. Require protected `typ=at+jwt`, the exact issuer and
    audience, valid time claims, the deployment tenant, canonical
    `sub == client_id`, and the signed `integration:submit` grant.
  - Treat the allowlist as a deployment-owned client binding. Project only the
    required submit role, then bind `SourceID` from the immutable integration
    registry after authentication; never accept tenant, principal, source,
    role, action, or revision identity from request headers.
  - Introduce one server-constructed `integration.submit` authorization request
    over the exact tenant, integration revision, and source. Enforce it at the
    adapter, shared processor, and transaction-scoped admission boundaries.
  - Map the existing HTTP, MLLP, and batch role strings to this action so the
    new decision is fail-closed without changing persisted channel provenance.
- Rationale:
  - Static bearer/HMAC credentials collapse all holders into one deployment
    principal. A verified per-request client subject is necessary for durable
    attribution, but signature verification alone does not authorize a client
    for a concrete production source.
  - Server-owned action and object identity prevents a valid token or spoofed
    header from expanding authority across tenants, revisions, or sources.
  - Rechecking before artifact loading and inside durable admission contains
    in-process callers that bypass the HTTP adapter.
- Alternatives considered:
  - Trust every signed client in the issuer (rejected because issuer membership
    is broader than deployment authorization).
  - Persist all signed token roles (rejected because unrelated claims could
    silently become future authority).
  - Replace bearer/HMAC, MLLP, batch, GraphQL, delivery, audit, and PHI controls
    in one slice (deferred because each has a different identity boundary and
    kill-test).
- Consequences:
  - The authorization server must issue the required JWT access-token profile;
    OAuth client credentials with opaque tokens is not supported in this slice.
  - `sub` and `client_id` must identify the same allowlisted confidential client.
    The immutable registry remains authoritative for revision and source.
  - Existing MLLP and batch identities remain deployment-configured pending
    certificate/workload binding slices, but now pass the same submit decision.
- Sources:
  - [S1] `internal/api/requestsecurity/oidc.go`
  - [S2] `internal/integration/authorization/policy.go`
  - [S3] `internal/integration/ingress/`
  - [S4] `.loom/iteration-plan-phase-4-slice-4-1b1-oauth-http-submit-authorization.md`

### 2026-08-08: Soft-Fail CI Policy and `test:integration` Promotion (Lane E)

- Decision:
  - Adopt an explicit soft-fail policy: every `allow_failure: true` job in
    `.gitlab-ci.yml` must carry an inline comment stating *why* it is advisory
    and what evidence would promote it. "Soft-fail during initial rollout" is no
    longer an acceptable standing reason.
  - Classify the three remaining advisory jobs and act on each:

    | Job | Green streak on main (18521..22333, 2026-07-13..2026-08-07) | Classification | Action in this MR |
    |---|---|---|---|
    | `test:integration` | 24/24 success — but see caveat below | Ready to promote | **Promoted to `allow_failure: false`** |
    | `lint:docs` | 33/33 success | Ready to promote | **Promoted to `allow_failure: false`** |
    | `test:docs-status` | 29/29 success | Intentionally advisory (for now) | Left advisory, promotion criteria documented inline |

  - **Caveat on the `test:integration` streak — it was never a full-execution
    proof.** All 24 of those runs executed with the `minio` service container
    dead, so all 30 MinIO-backed tests in `./cmd/fi-fhir/...` skipped. The
    streak validated the Postgres-only path and nothing more. This MR's own
    pipelines are the first true full-execution evidence, and they immediately
    found two defects the streak could not have caught (the truncated-column
    assertion, and the shared-database contamination below). Read the 24/24 as
    "the Postgres path is stable", not "the job was meaningful".

  - Repair the `minio` service container in `test:integration` before promoting
    it, because the job's green history was partly an artifact of tests that
    never ran.
  - Fix the one real test defect that the dead MinIO service had been masking.
  - Do **not** add a `/ready` smoke assertion; `/ready` is not served by
    `fi-fhir serve`. File a cleanup issue instead of asserting a 404.
- Rationale:
  - `minio/minio:latest` ships `CMD ["minio"]`, which prints usage and exits.
    The service container never listened on `minio:9000`, so
    `setupTestInfra()` failed its readiness probe and every dependent test hit
    `t.Skipf("setupTestInfra: minio not ready")` — a **skip**, not a failure.
    **30 integration tests silently skipped in CI**, including the PostgreSQL
    event-store lifecycle, projections, terminology init/status, storage, and
    mapping-decision CLI suites.
  - Negative-control kill-test: with MinIO unreachable, `./cmd/fi-fhir/...`
    reports `coverage: 73.2% of statements` (1380 pass / 30 skip) — the exact
    figure logged by CI job 218601. With MinIO live it reports `75.9%`
    (1410 pass / 0 skip). The coverage figures match to the decimal, so CI has
    been running the degraded path.
  - Promoting `test:integration` while it silently skipped a third of its
    infrastructure-backed surface would have written the false-coverage problem
    into the merge gate, which is the exact failure mode Gate 0B named for
    security jobs (`.loom/30-implementation-plan-integration-engine-ide-completion.md:58`).
  - With MinIO live, `TestIntegration_TerminologyMappingDecisionCLI` fails
    deterministically: it asserts a 23-character `GLU-<UnixNano>` source code
    appears in the decisions table, but that column is rendered through
    `truncate(decision.SourceCode, 12)`. The fixture, not the CLI, is wrong.
  - `lint:docs` is the safest promotion available: `scripts/validate-docs.sh` is
    deterministic, needs no services, runs in ~90s, and has never failed on main.
  - `test:docs-status` is held back deliberately. Its check is a *numeric*
    function-average coverage comparison recomputed from `test:unit`'s
    `coverage.out`, so promotion makes `make docs-status` mandatory for every Go
    MR author and a red run is not always attributable to the MR under test.
    Promoting it in its own MR keeps the blast radius attributable. This also
    honors the Lane E non-goal "do not promote every `allow_failure` job in one MR".
  - `security:gosec` / `security:govulncheck` / `security:trivy-image` are
    already blocking (`allow_failure` absent) and are **not** touched: gosec and
    govulncheck OOM intermittently on merge-ref pipelines, and trivy-image scans
    against a daily-moving vulnerability database.
  - `test:benchmark` keeps its blocking-manual design (`when: manual` +
    `allow_failure: false`); other tooling depends on that behavior.
- Alternatives considered:
  - Promote `test:integration` without repairing the MinIO service (rejected:
    it would make a hollow gate mandatory and hide 30 skipped tests behind a
    green required check).
  - Repair MinIO but keep `test:integration` advisory (rejected: the green proof
    requirement is satisfied and the C1 autoroute sweep kill-test plus the
    terminology DB store would remain unprotected).
  - Assert `GET /ready` returns 404 in `scripts/smoke-test.sh` (rejected: it
    would freeze the absence of a readiness endpoint into a passing assertion).
  - Promote all three advisory jobs at once (rejected by the Lane E non-goal and
    because a single red pipeline would then be ambiguous).
  - Change `truncate()` so the decisions table stops truncating (rejected:
    column truncation is intended table formatting; the test fixture was written
    without accounting for it).
- Consequences:
  - `test:integration` and `lint:docs` now block merges. Both sibling MRs
    shipping this sprint (`feat/phase4-mllp-cert-identity`,
    `feat/autoroute-notifications`) are gated on them.
  - `test:integration` gets slower and more expensive: the 30 previously-skipped
    tests now execute, and MinIO becomes a real dependency. Job duration on main
    was 350-565s while degraded; expect an increase.
  - If the MinIO service ever breaks again, the failure mode reverts to *skips*,
    not failures — the job would go green while covering less. This is the
    residual risk; see the cleanup issue on `setupTestInfra` skip semantics.
  - `test:docs-status` remains advisory, so STATUS.md coverage drift can still
    reach main. That is an accepted, dated, and now-documented gap.
- Follow-up within the same MR: shared-database contamination (found by this gate).
  - Once the MinIO fix let the 30 skipped tests actually run in CI,
    `test:integration` failed deterministically (jobs 220075 and 220246, retried
    on a quiet runner). `./cmd/fi-fhir/...` passed at the full 75.9%, then
    `./pkg/terminology/db/` panicked with `test timed out after 5m0s` at ~47.5%
    coverage, blocked in `Migrator.Initialize`
    (`pkg/terminology/db/migrations.go:79`) inside a lib/pq `simpleExec` network
    read.
  - Root cause is budget exhaustion from shared state, not a deadlock. The two
    steps are separate `go test` invocations, so no connection or lock can
    survive between them, and the panic's `running tests:` line shows the
    blamed test had run only 3s — the 300s was consumed by everything before it.
    Both suites reset the same `terminology` schema in the same database. While
    the cmd suite was skipping, it left that database empty; now it populates it,
    so every subsequent schema teardown/rebuild in `pkg/terminology/db` does real
    work. That package already needed 204.7s of a 300s budget on main (job
    218601) — only 32% headroom — so the added cost pushed it over.
  - Fix: give `pkg/terminology/db` its own database (`fi_fhir_terms_test`,
    created in the job script), which is exactly the isolation Lane D
    recommended, and raise that step's `-timeout` to 900s to remove the
    shared-runner cliff. The pre-existing `-p 1` was never sufficient: it limits
    parallel *packages* within one `go test` invocation and does nothing across
    two separate commands, and it never addressed data contamination at all.
  - Verified locally against PostgreSQL 16 and a live MinIO, running the CI
    steps in CI order: shared database reproduced no failure on fast hardware
    (33.5s vs a 33.4s clean baseline — the contamination cost is only visible on
    the constrained CI Postgres), and with separate databases both steps pass
    (cmd 23.2s / 75.6%, terminology 36.4s / 69.7%).
  - The timeout increase is a slow-suite allowance, not a weakened gate: a
    genuine hang still fails the job, just later.

- Cleanup issues to file:
  1. **`setupTestInfra` skips instead of failing when CI infra is down.** In CI,
     unavailable Postgres/MinIO should be a hard failure, not `t.Skipf` — the
     skip path is what allowed 30 tests to disappear from a "green" required job.
     Gate on `CI=true` (or an explicit `FI_FHIR_REQUIRE_TEST_INFRA=1`) so local
     runs keep the developer-friendly skip and CI cannot silently degrade.
  2. **`/ready` is dead code in `serve`.** `internal/workflow/health.go:68`
     defines a `/ready` readiness path but `NewHealthService` has zero non-test
     callers, so `fi-fhir serve` never mounts it. Helm's readiness probe uses
     `/health` (`deploy/helm/fi-fhir/templates/deployment.yaml:173`) and the
     kustomize base uses an exec probe, so nothing is broken in production —
     but either wire `/ready` into serve and add the smoke assertion, or delete
     the unused readiness path.
  3. **Promote `test:docs-status` to blocking** in a dedicated MR once the team
     accepts `make docs-status` as a mandatory pre-commit step for Go MRs.
- Sources:
  - [S1] `.gitlab-ci.yml` — `test:integration`, `lint:docs`, `test:docs-status`.
  - [S2] `cmd/fi-fhir/integration_helpers_test.go:56-118` — `setupTestInfra`
    skip-on-unavailable path and `ensureMinioBucket` readiness probe.
  - [S3] CI job 218601 (pipeline 22333) trace:
    `ok gitlab.flexinfer.ai/libs/fi-fhir/cmd/fi-fhir 60.783s coverage: 73.2% of statements`.
  - [S4] Command: `docker --context 7900xtx run --rm minio/minio:latest` →
    prints usage and exits; never starts a server.
  - [S5] Local negative control, MinIO unreachable:
    `go test -tags=integration ./cmd/fi-fhir/...` → `73.2%`, 1380 pass / 30 skip.
  - [S6] Local positive control, MinIO live on `cblevins-7900xtx:15504`:
    `go test -tags=integration ./cmd/fi-fhir/...` → `75.9%`, 1410 pass / 0 skip.
  - [S7] `cmd/fi-fhir/terminology.go:1866` — `truncate(decision.SourceCode, 12)`.
  - [S8] Local kill-test of the second script step:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/`
    → `ok ... 34.519s coverage: 69.7%`, including
    `--- PASS: TestAutorouteExpirySweep_FlipsStoredStatus`.
  - [S9] `scripts/smoke-test.sh:98-104` — `/health`, `/graphql`, `/graphql/ws`
    assertions already present (Gate 0B); `/ready` absent by design.
  - [S10] `.loom/24-parallel-execution-specs.md` — Lane E scope and kill-test;
    line 306 records Lane D's "distinct databases/schemas" recommendation.
  - [S11] CI jobs 220075 and 220246 — `panic: test timed out after 5m0s`,
    `FAIL pkg/terminology/db 300.05s`, after `ok cmd/fi-fhir 50.436s coverage: 75.9%`.
  - [S12] CI job 218601 — `ok pkg/terminology/db 204.737s`, the pre-existing
    68%-of-budget baseline.
### 2026-08-08: Scope Operator Control-Plane Authority and Render Payloads Structurally

- Decision:
  - Build the operator control plane as a read projection plus a delegation
    layer. `internal/integration/operator` owns no delivery or lifecycle state:
    every write goes to Slice 2.3's idempotent operation ledger and append-only
    audit trail, or to Slice 2.1's closed lifecycle state machine.
  - Split operator authority into three least-privilege roles rather than one:
    `integration.operator` for the bounded read surface,
    `integration.delivery.operator` (the existing Slice 2.3 constant) for
    replay/resubmit/discard, and `integration.deployment.operator` for
    pause/resume/retire/deploy. Reads are required for writes, so a control
    action always implies the ability to inspect what it changed.
  - Treat DLQ requeue as `replayDelivery` instead of adding a parallel action.
    Only discard is a genuinely new durable decision, so only discard needed a
    migration.
  - Render "policy-aware semantic payload" as structure only: dotted field
    coordinates, JSON kinds, and repetition flags. Object keys are emitted only
    when they match the engine's canonical field grammar; every other key
    collapses to `*`. No value, no value length, and no raw byte is returned.
- Rationale:
  - A second write path would have to re-implement idempotency, actor capture,
    and audit append. Duplicating that is exactly the duplicate-durable-work
    class the product spec calls a P0.
  - One combined operator role would let a read-only auditor mutate production
    traffic. Splitting the roles keeps the audit journey usable without granting
    recovery authority.
  - Field names in a canonical event are engine-authored schema; values are PHI.
    Emitting only the schema gives an operator real diagnostic signal while
    keeping the projection provably value-free, which a planted-sentinel test
    can verify rather than merely assert.
- Alternatives considered:
  - Return a redacted payload document (rejected: redaction is a denylist, and a
    new field defaults to exposed).
  - Expose value lengths or type-prefixed samples (rejected: length and prefix
    are identifying for MRNs and names).
  - One `integration.operator` role for everything (rejected: no least
    privilege, and the audit journey is the most widely granted one).
  - Add a separate DLQ requeue mutation (rejected: it is replay by another name
    and would fork the recovery contract).
- Consequences:
  - A deployment must issue three role claims to give one person full operator
    authority; existing tokens gain no control-plane access implicitly.
  - Discarding a dead letter is attributable and idempotent but not reversible;
    the attempt stays failed and the DLQ entry records `discarded`.
  - The GraphQL error presenter now allowlists the control plane's catalog-safe
    messages so an operator can distinguish a stale expected version from a
    spent idempotency key without learning whether an unseen record exists.
  - Raw payload retrieval, export controls, and retention administration remain
    out of scope; the control plane never grows a value-returning field without
    a new decision.
- Sources:
  - [S1] `internal/integration/operator/payload.go`
  - [S2] `internal/integration/operator/service.go`
  - [S3] `internal/integration/processor/migrations/0003_operator_control_plane.sql`
  - [S4] `internal/api/graphql/operator_control_plane_integration_test.go`
  - [S5] `.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`

### 2026-08-08: Serve Observability Is a Second Listener Over a Purpose-Built Registry

- Decision:
  - **Metrics (spec `.loom/31` Lane S3-A option A).** A new `internal/observability`
    package owns exactly one `prometheus.Registry` and serves it on a **second
    HTTP listener** bound to `FI_FHIR_SERVER_HOST:FI_FHIR_METRICS_PORT`
    (default `9090`) at `FI_FHIR_METRICS_ENDPOINT` (default `/metrics`), gated by
    `FI_FHIR_METRICS_ENABLED` (default `true`). The listener is a first-class
    background component in `runServe`'s `errCh` table and shutdown path.
  - **Health lives on the main listener.** `/health` (liveness, process-only) and
    `/ready` (readiness, dependency-touching) stay on the GraphQL listener,
    because that is the port the Helm chart, `scripts/smoke-test.sh`, and every
    ingress already address. The metrics port carries no health surface and no
    PHI-bearing route.
  - **The check engine is the already-shipped `workflow.HealthService`**, wired
    for the first time from production code rather than re-implemented.
    `internal/observability` adapts it (registration, snapshot projection, HTTP
    handlers) so the GraphQL `health` resolver and the HTTP probes read one
    source of truth.
  - **Metric names are `fi_fhir_*`, not `workflow_*`.** The 32 legacy alert rules
    and the Grafana dashboard are rewritten against the emitted names.
    `internal/workflow`'s Prometheus adapter is left untouched and documented as
    legacy-engine-only.
  - **Label cardinality is bounded by construction.** Every label value is drawn
    from a compile-time constant set (component name, outcome, state, version).
    No correlation ID, receipt ID, attempt ID, tenant string, URL, or any
    message-derived value is ever a label.
  - **Durable session fanout (spec task 6)** is a new append-only envelope table
    `integration_session_stream_events` plus PostgreSQL `LISTEN`/`NOTIFY` on
    channel `integration_session_stream`, with a 1s poll backstop. The table and
    the notification carry **only** `(tenant_id, session_id, run_id, event_type,
    seq)` — never a payload. This is safe because `toGraphQLEvent`
    (`internal/api/graphql/resolvers/integration_session_service.go:827`) already
    ignores `StreamEvent.Payload` and re-reads the session and run from the
    durable store, so the stream log needs no clinical content to reproduce the
    subscriber's view.
  - **Autoroute notifier de-duplication (spec task 8)** becomes a durable
    `notified_at` claim column on `pending_autoroutes`, claimed with
    `UPDATE … SET notified_at = now() WHERE id = ANY($1) AND notified_at IS NULL
    RETURNING id`. Chosen over `pg_advisory_lock` because a lock serialises
    scanners without making the *decision* durable: a restart still re-pages the
    whole backlog, which is half the defect.
  - **MLLP capacity (spec task 9, option a)** is documented as **per-replica**.
    `CapacityPolicy` gets a doc comment and `docs/operations/PRODUCTION-MLLP.md`
    gets the division rule. A durable token bucket is 4.4 work.
  - **Batch worker identity** derives from `hostname-pid` when
    `FI_FHIR_BATCH_WORKER_ID` is unset, exactly as `delivery_runtime.go:40-47`
    already does, and the documented configuration stops handing out a shared
    literal.
  - **Negative control.** `FI_FHIR_OBSERVABILITY_MODE=legacy` restores the
    pre-slice behaviour at every one of these seams. It exists so the kill-test
    can prove it can fail, is refused a production role in `.env.example` and
    `docs/operations/README.md`, and logs a loud warning when set.
- Rationale:
  - Option A is the only option that makes the **already checked-in** deployment
    façade true rather than deleting it: the `metrics` containerPort, both
    Services, the compose port mapping, the Prometheus scrape job, the pod
    annotations, and `pkg/config.ObservabilityConfig` all describe a second
    listener on 9090. Every other option requires editing those artifacts to
    describe something else.
  - Keeping the scrape path off the GraphQL listener keeps an unauthenticated
    endpoint off the same socket that accepts raw clinical POSTs.
  - `prometheus/client_golang v1.23.2` is already a direct dependency, so the
    registry costs no new supply-chain surface.
  - Emitting `workflow_*` names from integration-engine code would be a naming
    lie of exactly the class this slice exists to remove, so the dashboards move
    rather than the metric names.
  - Persisting stream **envelopes** rather than payloads means the multi-replica
    fix adds zero new PHI at rest, which keeps retention policy squarely in
    S3-C's lane instead of quietly expanding it.
- Alternatives considered:
  - **B. Mount `/metrics` on the GraphQL mux** (rejected: contradicts every
    deployment artifact; puts an unauthenticated scrape path on the raw-clinical
    listener; needs a third reserved-path entry in `validateServerConfig`).
  - **C. Reuse `internal/workflow.PrometheusMetrics` as the serve registry**
    (rejected: its interface is events/actions/DLQ-shaped for the legacy engine
    the durable path never executes; adopting it means emitting `workflow_*`
    names from integration-engine code).
  - **D. Delete the façade** (rejected: the product spec requires `/metrics`
    (`.loom/20-product-spec-integration-engine-ide-completion.md:225`); deleting
    only defers 4.3).
  - **Re-implement health checking inside `internal/observability`** (rejected:
    `workflow.HealthService` already implements the liveness/readiness split,
    concurrent checks with timeout, 1s readiness caching, and 503-on-unhealthy;
    duplicating it would be ~250 lines of new untested code to avoid one import).
  - **Store stream payloads in the durable stream log** (rejected: duplicates
    clinical content with no retention policy, in a lane whose non-goals
    explicitly exclude retention).
  - **`pg_advisory_lock` around the notifier scan window** (rejected: see above).
  - **Leader election for the autoroute sweeper** (rejected: `ExpirePendingAutoroutes`
    is an idempotent guarded `UPDATE`; two replicas waste one query and have no
    external effect. Paying for a lease to save a query is not worth a new
    failure domain. Documented as a known benign duplicate.)
- Consequences:
  - `serve` now binds two ports by default. A deployment that cannot bind 9090
    must set `FI_FHIR_METRICS_ENABLED=false`; the process refuses to start
    silently degraded.
  - `pkg/config.ObservabilityConfig` gains its first production consumers
    (metrics fields). `TracingEnabled`/`TracingEndpoint`/`TracingSampler` remain
    inert and are now labelled "not implemented" in `.env.example` and
    `docs/operations/README.md` rather than implying an exporter exists.
  - The session workspace schema gains a migration. S3-C1 merged first and took
    `0004_export_attribution.sql`, so this lane's fanout log landed as
    `0005_session_stream_events.sql`.
  - `pending_autoroutes` gains a `notified_at` column. Existing rows are
    backfilled to `NULL`, so the first scan after upgrade re-pages the current
    backlog exactly once and never again.
  - MLLP `CapacityPolicy` remains per-replica; an operator running N replicas
    must divide the declared policy by N or accept N× the declared ceiling. This
    is now written down instead of being an undocumented surprise.
- Sources:
  - [S1] `.loom/31-sprint3-execution-specs.md` Lane S3-A, "The explicit metrics decision"
  - [S2] `internal/workflow/health.go:88,220-259`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go:827-845`
  - [S4] `deploy/kubernetes/base/deployment.yaml:19-22,42-44`; `deploy/kubernetes/base/service.yaml`
  - [S5] `pkg/config/config.go:181-195,417,606-613`
  - [S6] `cmd/fi-fhir/delivery_runtime.go:40-47`
  - [S7] `internal/terminology/autoroute/notify.go:292-293,494-519`

### 2026-08-08: FHIR conformance validation strategy for Slice 5.1 (AMENDED AND RATIFIED IN PART — 2026-08-09, Sprint 5 Lane S5-E)

**Status: split ruling, in force as amended.** This entry was recorded by Sprint
4 Lane S4-D as a recommendation and carried "not ratified; requires human or
next-sprint ratification before any lane acts on it". Sprint 5's coordinator
ruled on it on 2026-08-09 (`.loom/33-sprint5-execution-specs.md`, Decisions
Required, ruling 3). The ruling is recorded in the **2026-08-09 amendment**
immediately below, which is part of this entry and takes precedence over the
proposed text where the two differ. The proposed text is kept verbatim because
its evidence is still the evidence.

#### 2026-08-09 amendment (Sprint 5, Lane S5-E, Slice 5.1a)

**1. The confinement half is RATIFIED, unconditionally.** `validator_cli.jar` is
CI-only and never enters the shipped image; the shipped image stays
`gcr.io/distroless/static-debian12:nonroot` (`Dockerfile:27,58` — no shell, no
package manager, no JRE); IG packages are vendored as pinned, digest-recorded,
offline `.tgz` artifacts and are placed deliberately with respect to the
`security:trivy` skip list. The premise holds on inspection: `security:trivy-image`
blocks on CRITICAL and HIGH-fixed with no `allow_failure` on MR, default branch,
and tags, and this repo's trivy database moves daily, so a green `main` does not
imply a green MR. A JRE in the shipped image would put a continuously moving CVE
surface behind a blocking gate.

**2. The ordering half is AMENDED.** The proposed order was "Option C now, Option
A later". The corrected order is **reconcile first (5.1a), then Option C, then
Option A.** The repository's actual defects are not that the checker is
structurally shallow. They are that the checker and the mapper *disagree*, that
the checker *fails open* on any mode string that is not exactly `us-core`, and
that CI has *no fixture* for the one input the mapper actually produces. A larger
validator built over a mapper it disagrees with certifies the disagreement at
higher resolution. This amendment resolves the open item further down this entry
— "A profile-version assertion policy must be chosen … and the mapper and checker
must agree. Today they cannot" — by making that reconciliation the next slice
rather than a precondition nobody owns.

**3. Slice 5.1's real prerequisite is a slice that does not exist: 4.1c-c, a
FHIR destination class.** This entry's own closing line reads "Slice 5.1 remains
blocked on Slice 4.1c-b regardless of which engine wins." 4.1c-b merged at
`e77c6218b`, which satisfied that condition formally and not substantively. The
`.loom/28-spec-fhir-ig-bulk-smart.md:206-212` kill-test was written for exactly
this moment and has now been **run**, not argued —
`TestFHIRConformance_DurableEngineProducesNoFHIRResource`
(`internal/integration/delivery/fhir_conformance_gate_test.go`) **passes on
unmodified `main`**: the delivered body is the Kafka delivery-command envelope
(`integration.delivery.v1`) carrying `integration_canonical_events.payload_json`
(`delivery/dispatcher.go:162,166,348`; `delivery/store.go:107,128`), the content
type is `application/json` and not `application/fhir+json`
(`destination/transport.go:325`), the transport vocabulary is exactly
`{kafka, https}` with no FHIR class (`destination/revision.go:57,61`),
`DestinationClass` is `production|sandbox` — an environment class
(`pkg/integration/contracts.go:602`) — and **zero** files under
`internal/integration/**` import `pkg/fhir`. Per `.loom/28`'s own instruction:
5.1 is still blocked, and the blocker is 4.1c-b's scope, not the validator.

4.1c-c is therefore named here as the real prerequisite: a destination class or
`TransportKind` meaning "this destination receives FHIR R4 resources", a
canonical-event→resource mapping step on the delivery path,
`application/fhir+json`, and a decision on whether the producing mapper is
`pkg/fhir` (reachable today from exactly two non-test files — `cmd/fi-fhir/main.go`
and `internal/workflow/actions.go` — neither of them the durable engine) or
something new. It is a Phase 4 delivery slice, not a Phase 5 standards slice, and
it is not in Sprint 5's scope.

**4. Slice 5.1a is what the FHIR lane ships instead**, and it needs no Java, no
IG package, no new `go.mod` dependency, and no image change. Its day-1 gate
`TestFHIRConformance_ValidatorRejectsMapperOutputToday`
(`pkg/fhir/conformance_day1_gate_test.go`, behind the `fhirday1gate` tag,
reproduced by `make fhir-conformance-day1-gate`) **fails on unmodified `main`**
with `warning value: meta.profile does not include an expected profile for
DiagnosticReport`, while the repository's own `-note` fixture validates clean in
the same run. The shipped validator rejects the shipped mapper's own output.

- Decision (proposed 2026-08-08; superseded on ordering by the amendment above):
  - **Adopt Option C now and Option A later, in that order, and never Option A
    inside the shipped image.**
  - **Now (Sprint 4, zero code):** keep the existing presence check as the only
    validation the product claims, and say so plainly. The claim already exists
    in the right place — `docs/operations/SUPPORTED-1.0.md:26` lists US Core
    9.0.0 as a *release target* and states that "official validator or
    conformance-suite evidence is not yet complete." Nothing needs to be
    weakened; the honest statement is already shipped. What is missing is the
    positive description of what the checker *is*, which is now published at
    `docs/planning/FHIR-CONFORMANCE-MATRIX.md`.
  - **Slice 5.1, after 4.1c-b:** integrate the HL7 `validator_cli.jar` as a
    **CI-only service**, in its own blocking job following the isolated-proof
    pattern of `test:phi-audit` and `test:observability-replicas`
    (`.gitlab-ci.yml:1397-1444,1455-1490`). The Java runtime exists in the CI
    image for that one job and nowhere else.
  - **The shipped image stays distroless static.** `Dockerfile:27` is
    `gcr.io/distroless/static-debian12:nonroot` running a `CGO_ENABLED=0` static
    binary as `USER nonroot` with no shell and no package manager
    (`Dockerfile:20-24,27,58`). No JRE is added to it under any option. If
    runtime conformance checking is ever required in-process, that is a separate
    decision requiring Option B.
  - **Package pinning is mandatory under every option.** `hl7.fhir.r4.core#4.0.1`
    and `hl7.fhir.us.core#9.0.0` are vendored as `.tgz` artifacts with recorded
    digests and resolved offline. The validator must never reach
    `packages.fhir.org` during a pipeline; a conformance gate that silently
    re-resolves its own IG is not a gate.
  - **Where the vendored artifacts live is part of the decision, not an
    accident.** The blocking `security:trivy` filesystem scan skips `.tmp`,
    `.go`, `.cache`, `ui/.npm`, and `sdk/typescript/.npm`
    (`.gitlab-ci.yml:1592-1595`). Dropping a fat jar into a skipped directory
    would exempt it from the scan by side effect. Vendored IG `.tgz` packages are
    data and should be scanned; a vendored `validator_cli.jar` should not be
    vendored at all — pull it in the job from a digest-pinned image.

- Rationale:
  - **There is nothing to validate yet, so the cheapest correct move is to
    describe reality accurately and wait.** `pkg/fhir` has exactly two non-test
    importers — `internal/workflow/actions.go:23` (legacy engine; mapper
    constructed at `:680`, action registered only at
    `internal/workflow/engine.go:127`) and `cmd/fi-fhir/main.go:49` (the `fhir
    validate` CLI, `:309`). The durable engine's only use of `internal/workflow`
    is a planner that "never invokes transforms or actions"
    (`internal/workflow/plan.go:144`), and its delivery command carries the
    canonical event, not a resource
    (`internal/integration/delivery/dispatcher.go:246-259`;
    `store.go:107,128`). Integrating a validator today would gate a path no
    golden journey executes.
  - **The current checker cannot be incrementally grown into a profile
    validator, and pretending otherwise is the expensive mistake.** It is a
    required-field presence check over 6 resource types plus a profile-URL
    string-membership check over 21 (`pkg/fhir/validate.go:104-151,153-177,
    179-237`). It has no `StructureDefinition`, no snapshot, no terminology
    binding, no primitive-type validation. Demonstrated on `origin/main` @
    `55412bda`: `"gender":"purple"` with `"birthDate":"not-a-date"` passes clean;
    a `Device` with a fabricated profile URL passes clean; a version-pinned
    canonical `…/us-core-patient|9.0.0` *fails*, because matching is exact string
    equality (`:171`) against an unversioned base (`pkg/fhir/types.go:13`). Each
    of those is a separate subsystem to build, and building four of them badly is
    worse than shelling out to the reference implementation once.
  - **`validator_cli.jar` is the reference implementation and the only thing that
    produces evidence the cross-cutting proof matrix will accept.** The plan
    requires "Official validator/conformance test evidence" for the Standards row
    (`.loom/30-implementation-plan-integration-engine-ide-completion.md`,
    cross-cutting proof matrix, `:844`) and warns that "ad-hoc similarly named endpoints
    do not count" (`:815`). A hand-rolled Go checker cannot produce that evidence
    by construction, no matter how good it is.
  - **CI-only confines the Java supply chain to a boundary that already exists.**
    CI already runs several third-party images from a mirrored prefix
    (`DOCKERHUB_CACHE_PREFIX`, `.gitlab-ci.yml:33`), including `aquasec/trivy`
    at `:1588,1670`. One more digest-pinned image in one job is a known,
    bounded cost. The shipped image is a different threat surface with a
    different gate: `security:trivy-image` fails the pipeline on any CRITICAL in
    the built image (`--exit-code 1 --severity CRITICAL`, `:1677-1678`) against a
    vulnerability database that moves daily. Putting a JRE into the artifact
    scanned by that gate means every future JRE CVE becomes an unrelated red
    pipeline on somebody else's MR.
  - **Image size is a real consequence, not a rounding error.** A
    `distroless/static-debian12` base plus a stripped static Go binary is
    single-digit megabytes; a JRE base is measured in the hundreds. Measure it
    before ratifying, but do not adopt Option A-in-image on the assumption it is
    cheap.

- Alternatives considered:
  - **A-in-image. Ship `validator_cli.jar` inside the product image so the engine
    can validate at runtime.** *Rejected.* It abandons distroless static
    (`Dockerfile:27`) for a JRE base, adds a shell and a package manager to an
    image deliberately built without them, grows the image by roughly two orders
    of magnitude, and routes every future JRE CVE through the blocking
    `security:trivy-image` gate (`.gitlab-ci.yml:1677-1678`). No requirement in
    `.loom/20` or `.loom/30` asks for in-process IG validation at runtime; the
    requirement is *evidence*, which CI produces.
  - **B. Write a Go structural validator over pinned `.tgz` IG packages.**
    *Rejected for 5.1; retained as the fallback if Option A is ever refused on
    supply-chain grounds.* It is the only option that could eventually run
    in-process without a JRE, and it keeps the image untouched. But it means
    building, from nothing: `.tgz` package loading, `StructureDefinition`
    snapshot walking, cardinality and slicing evaluation, FHIRPath invariants,
    and terminology binding evaluation with ValueSet expansion. `go.mod` has no
    FHIR dependency of any kind, and `pkg/terminology` is LOINC/UMLS
    mapping machinery, not a FHIR terminology server — there is no ValueSet
    expansion anywhere in the tree. It is a multi-slice subsystem competing with
    a `java -jar` invocation, and it still would not satisfy "official validator"
    evidence.
  - **C-alone. Keep the presence check permanently and document it as the
    ceiling.** *Rejected as an endpoint, adopted as the interim.* US Core 9.0.0
    conformance is a 1.0 gate (`.loom/30-implementation-plan-…:856`,
    `.loom/20-product-spec-…:262,302-304`), so "presence check forever" means
    dropping a 1.0 claim. That is a product decision, not an engineering one, and
    nobody has asked for it.
  - **Hosted/third-party validation service.** *Rejected without detailed
    analysis.* It would send resource content to an external endpoint. Every
    fixture in a conformance run is synthetic today, but the same job would be
    the obvious place to validate a captured golden-journey payload, and the PHI
    posture (`.loom/20-product-spec-…`, `docs/operations/PHI-RETENTION.md`) does
    not permit that egress path to exist by default.

- Consequences (if ratified):
  - CI gains one image and one blocking job at Slice 5.1; the shipped image
    changes in no way.
  - The repository gains vendored, digest-recorded `.tgz` IG packages. Their
    location must be chosen deliberately with respect to the `security:trivy`
    skip list (`.gitlab-ci.yml:1592-1595`), not defaulted into a skipped
    directory.
  - Two self-inconsistencies must be fixed before any migration, because both
    produce false negatives under any engine: a lab `DiagnosticReport` is stamped
    `us-core-diagnosticreport-lab` (`pkg/fhir/mapper.go:435`) while only
    `us-core-diagnosticreport-note` is accepted
    (`pkg/fhir/validate.go:218-219`), and a version-pinned canonical never
    matches an unversioned constant (`:171`, `pkg/fhir/types.go:13`).
  - A profile-version assertion policy must be chosen — bare canonical or
    `|9.0.0` — and the mapper and checker must agree. Today they cannot, because
    the checker has no version concept. *(Owned by Slice 5.1a per the 2026-08-09
    amendment above.)*
  - `docs/planning/FHIR-PROFILES.md` remains stale until the ratifying slice
    updates it: `:80`, `:445`, and `:542` name US Core 6.1.0, and `:485` claims
    "Meta.Profile set on all resources", which
    `pkg/fhir/mapper.go:1298-1313,1930-1935` disproves.
  - Ratification unblocks nothing in Sprint 4. Slice 5.1 remains blocked on
    Slice 4.1c-b regardless of which engine wins. *(Corrected by the 2026-08-09
    amendment: 4.1c-b has merged and 5.1 is still blocked. The blocker is a slice
    that does not exist — 4.1c-c, a FHIR destination class — because 4.1c-b
    delivers a canonical-event command envelope, not a FHIR resource. Proven by
    `TestFHIRConformance_DurableEngineProducesNoFHIRResource`.)*

- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` corrections 35-38 and Lane S4-D
  - [S2] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` (full `file:line` evidence)
  - [S3] `pkg/fhir/validate.go:104-151,153-177,171,179-237,218-219,239-249`;
    `pkg/fhir/types.go:2,13`; `pkg/fhir/mapper.go:435,1298-1313,1930-1935`
  - [S4] `internal/workflow/actions.go:23,680`; `engine.go:127`; `plan.go:144`;
    `cmd/fi-fhir/main.go:49,254-255,309,349`
  - [S5] `internal/integration/delivery/dispatcher.go:246-259`; `store.go:107,128`;
    `internal/integration/processor/workflow_plan.go:41,45,112-119,147-153`
  - [S6] `Dockerfile:2-3,20-24,27,58`
  - [S7] `.gitlab-ci.yml:12,14,33,1585-1600,1592-1595,1667-1678,1397-1444,1455-1490`
  - [S8] `docs/operations/SUPPORTED-1.0.md:26,62`; `docs/planning/FHIR-PROFILES.md:80,445,485,542`
  - [S9] `.loom/30-implementation-plan-integration-engine-ide-completion.md:803-807,815,844,856`;
    `.loom/20-product-spec-integration-engine-ide-completion.md:262,293,302-304`
  - [S10] US Core 9.0.0 (STU 9), `hl7.fhir.us.core#9.0.0`, FHIR 4.0.1, published
    2026-05-31, meets USCDI v6 — https://hl7.org/fhir/us/core/STU9/
### 2026-08-08: Slice 4.1e — the immutability exemption for purge, and where retention policy lives

Two decisions, both required before this lane writes a migration
(`.loom/32-sprint4-execution-specs.md`, Lane S4-B, corrections 11-17). Both are
forced by the lane's day-1 gate,
`TestPhiRetention_PurgeIsStructurallyBlockedToday`, which **passes on unmodified
`main`**: a `DELETE` of a dependent-free canonical event raises, the redaction
`UPDATE` of `payload_json` raises too, and an exported session is undeletable at
both the export row (trigger) and the session row (foreign key, SQLSTATE 23503).
Purge is not a policy-design problem with a `DELETE` at the end of it. It is
structurally impossible today.

- Decision:
  - **1. Immutability exemption: option A — a column-scoped `BEFORE UPDATE`
    exemption with canonical tombstone semantics.** On
    `integration_canonical_events` and `integration_session_exports`, Slice 4.1d
    C1's blanket `BEFORE UPDATE OR DELETE` guard is replaced by a blanket
    `BEFORE DELETE` guard plus a `BEFORE UPDATE` guard that raises unless the
    update changes **only** the payload column and `purged_at`, sets the payload
    to the canonical tombstone object, and sets `purged_at` from a previously
    `NULL` value. Every other column stays frozen; `DELETE` stays blanket-blocked
    on both tables. This mirrors C1's own `reject_integration_receipt_provenance_mutation`
    idiom (`internal/integration/processor/migrations/0004_audit_immutability.sql:69-91`)
    rather than inventing a second convention.
  - **A tombstone is not a backup-inclusive deletion.** This is the written
    consequence the option carries, not a footnote: the row, its identity, its
    classification, and its `recorded_at` survive on purpose so an audit still
    shows what existed, and **any database backup taken before the purge still
    contains the payload**. Purge bounds retention in the live database only.
    Backup-copy expiry stays a database and storage-layer control operated
    outside this codebase, and is named as a Slice 4.4c interaction.
  - **`integration_session_samples` is deleted outright, not tombstoned.** It
    carries no immutability trigger (correction 14), so the honest purge for a
    retained sample is removal of the row and its `raw_cipher`. Applying a
    tombstone shape there would add a guarantee the table never had.
  - **2. Retention policy lives in a new mutable, audited, per-tenant
    `integration_retention_policies` record** — not in the revision contract and
    not in deployment configuration alone. The deployment supplies only a
    fail-closed default of **retain indefinitely**, so an unconfigured deployment
    purges nothing.
  - **3. Role separation for the purge (option C) is filed, not built.** It
    becomes a named follow-up slice, "purge role separation", in the Sprint 5
    list.
- Rationale:
  - Option A keeps the project's stated posture — *the schema, not convention, is
    the guarantee* — intact. The exemption is itself schema-enforced, is narrower
    than any role-based bypass, and survives correction 12 without touching a
    single foreign key: no `ON DELETE RESTRICT` chain has to be relaxed, because
    nothing is deleted.
  - It is also the only option that leaves an audit trail of what was purged. A
    deleted row proves nothing; a tombstoned row with `purged_at` and an
    append-only audit entry proves exactly what was removed, when, and under
    which policy version.
  - On policy placement: an integration revision is immutable and
    content-addressed, and the retained data outlives it. Putting retention in
    the revision contract would mean a retention change requires minting a new
    revision and redeploying — the policy would be pinned to the artifact that
    produced the data rather than to the tenant that owns it. Deployment
    configuration alone fails differently: no audit trail of who changed a PHI
    retention window and why, and no per-tenant scope in a schema that is
    per-tenant everywhere else.
  - Fail-closed "retain indefinitely" is the only safe default for a control
    whose failure mode is deleting clinical data. An operator must opt in to
    purging, per tenant, with an attributed policy record.
  - Correction 16 empties option C as stated: every migration runs on the same
    `*sql.DB` the runtime uses, so the application role already owns the tables
    and can `DROP TRIGGER` outright. A "privileged role that bypasses the guard"
    buys nothing while the ordinary role outranks it. Real separation means a
    de-privileged application role, a separate migration runner, and a purge
    role — three role changes and a deployment migration, which is its own slice.
- Alternatives considered:
  - **B. `SET session_replication_role = 'replica'` around the purge
    transaction** (rejected: one line, but it disables *every* trigger on *every*
    table for that session — all six C1 guards, the four lifecycle guards, and
    both session-workspace guards. It turns a scalpel into a switch, and it needs
    superuser or an explicit `GRANT SET ON PARAMETER`).
  - **C. A separate purge role that owns the tables and disables triggers**
    (rejected as stated — see correction 16 above; filed as a follow-up slice).
  - **D. Tombstone in a side table, leave the payload in place** (rejected: it
    purges nothing. The PHI stays. It fails the slice's only purpose while
    looking like progress).
  - **Retention policy on the revision contract, beside `RawRetentionPolicy`**
    (rejected: immutability — see rationale. `RawRetentionPolicy`
    (`pkg/integration/revision.go:108-157`) governs *production raw bytes*, which
    are rejected unless `ephemeral`, so it is a policy over an empty set and not
    a precedent for the data that actually persists).
  - **Retention policy in deployment configuration only** (rejected: no audit
    trail, no per-tenant scope).
  - **A lease or `pg_advisory_lock` around the purge scan** (rejected, following
    the S3-A precedent recorded above for the autoroute sweeper: the guarded
    `UPDATE ... WHERE purged_at IS NULL ... RETURNING` **is** the claim. Only the
    replica whose `RETURNING` yields the row writes its audit entry, in the same
    transaction, so two replicas produce one tombstone and one audit row without
    a new failure domain).
- Consequences:
  - `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6 stop being true the
    moment the expiry columns land, and the retention-posture gate
    (`TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy`)
    is **designed** to fail then. Rewriting both is a task in the implementation
    MR, not a surprise (correction 18).
  - C1's guarantee narrows in one specific, documented way: the canonical event
    payload and the export snapshot become replaceable-once, by a tombstone, with
    an audit row. Nothing else about C1 changes, and the five blanket-guarded
    ledgers stay blanket-guarded.
  - A pre-slice row has no policy. The expiry columns are `NULL`-able with **no
    backfill**: inventing a `purge_after` for data admitted before any policy
    existed would be retroactive vouching, the same reason 4.1b3 and C1 refused
    to backfill provenance.
  - The trigger function now contains real logic and must be reviewed as security
    code, not as schema boilerplate.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` — Lane S4-B, corrections 11-20
  - [S2] `internal/integration/processor/migrations/0004_audit_immutability.sql:29-32,69-91`
  - [S3] `internal/integration/processor/migrations/0001_atomic_submission.sql:52-54,73-75,90-92`
  - [S4] `internal/integration/session/migrations/0004_export_attribution.sql:55-58`; `migrations/0001_session_workspace.sql:88-90`
  - [S5] `internal/integration/retention/purge_gate_integration_test.go` — the day-1 gate, passing on unmodified `main`
  - [S6] `pkg/integration/revision.go:108-157`; `internal/integration/processor/postgres_submission.go:179-181`
  - [S7] `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195` — the `DELETE`-only framing this decision corrects

### 2026-08-09: Deliver HTTPS Destinations by Substituting the Transport at the `Publisher` Seam

- Decision:
  - Slice 4.1c-b delivers an `https`-transport destination **by substituting the
    transport at the `Publisher` seam inside `Dispatcher.RunOnce`**, not by
    adding an in-process consumer of `integration.delivery.v1`. A new
    primitives-only `DestinationTransport` interface is declared in
    `internal/integration/delivery` alongside `DestinationDecider`; the
    `destination` package satisfies it structurally; neither package imports the
    other [S1]. The dispatcher asks the transport whether it owns the claimed
    item's destination. An `https` destination is delivered over TLS and marked
    with the existing `MarkPublished`/`MarkFailed`; every other destination
    publishes to the constant Kafka topic exactly as before.
  - The Kafka topic remains the transport for `kafka`-class destinations, so
    `TestDeliveryDispatch_ContactsNoDestination` stays a true and meaningful
    boundary marker. It is **narrowed** to `kafka`-class, never inverted.
  - A successful HTTPS delivery reports the existing `OutcomePublished`. The
    outcome means "handed off successfully", which is exactly what it now is for
    both transports; introducing a second success outcome would force a change
    in `cmd/fi-fhir/serve_observability.go` and
    `internal/observability/metrics.go` for zero operator benefit.
  - The credential is resolved **per dispatch** through `integration.SecretResolver`,
    used to build one request, and zeroed. It never enters `Decision`, a log
    line, a metric label, a `Failure.Detail`, or any struct that is
    JSON-marshaled. There is no credential cache, so a file or environment
    rotation takes effect on the next dispatch [S5].
- Rationale:
  - The durable state machine is already destination-aware and transport-blind.
    `Claim` keys the circuit breaker on the **destination artifact**, not on
    Kafka [S2]; retry, backoff, DLQ, replay, resubmit, and discard all live in
    `store.go` and know nothing about a broker. A transport substituted at the
    `Publisher` seam inherits all of it and leaves `store.go` — still 4.2a's
    file — untouched.
  - The decision already runs at the right point. `decideIdentity` resolves the
    full destination revision, including `Transport` and `HTTPS.URL`, one line
    before the publish [S3] — and then discards it.
  - There is **no Kafka consumer anywhere in production code**. `kgo.ConsumeTopics`
    appears only in two integration tests [S4]. An in-process consumer is a new
    consumer group, new offset commits, new rebalance handling, and a **second**
    at-least-once boundary layered on the one the outbox already owns. It cannot
    reuse the circuit, the DLQ, or `recover`, because those are keyed to an
    outbox row the consumer holds no lease on.
  - The day-1 gate `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday`
    passed on unmodified `main`: an `https` destination with a live TLS endpoint
    is fully resolved, digest-verified, authorized, and provenance-recorded with
    its URL — and is then published to Kafka anyway, with the endpoint recording
    zero accepted connections. That is the empirical basis for "substitute the
    transport", not "wire up the existing one".
- Alternatives considered:
  - **B. In-process consumer of `integration.delivery.v1`** (rejected: builds the
    larger thing. Two independent at-least-once layers make duplicate delivery a
    product rather than a bound, and the spec's P0 definition includes duplicate
    durable work for one idempotency key. Preserving an external-consumer
    contract that no production code implements is not a benefit.)
  - **C. Both — publish *and* deliver** (rejected on sight: two systems
    contacting one destination for one attempt is a duplicate-delivery
    generator.)
  - **Widen `DestinationDecider.Decide` into a transport interface** (rejected:
    reintroduces the import cycle the S3-B handoff records solving, and couples
    the authorization decision to a transport concern the `destination` package
    was deliberately kept free of.)
  - **Cache the HTTPS client per destination digest** (rejected: the revision is
    immutable, but the CA bundle and the token behind its binding names are
    files that rotate in place with no version to invalidate a cache on [S5]. A
    cached client would silently pin a rotated-out root. One client per dispatch
    costs one TLS handshake and keeps rotation honest.)
  - **A separate `FI_FHIR_DELIVERY_HTTPS_TIMEOUT` knob** (rejected: `Config.validate`
    already requires `PublishTimeout < LeaseDuration` [S6], which is precisely
    the invariant that stops a slow destination outliving its lease and being
    delivered twice after reclaim. A second knob would be a second way to break
    it.)
- Consequences:
  - A deployment that has already declared `transport: https` on a destination
    in its registry starts contacting that destination on upgrade. That is the
    slice, and the registry is server-owned, so declaring `https` is an explicit
    operator act — but it is a behaviour change on upgrade and is called out in
    `docs/operations/DESTINATION-IDENTITY.md`.
  - `MarkPublished` now means "handed off successfully" for two transports. Its
    doc comment says so; it is not renamed.
  - `destinationIdentityRuntime` gains a resolver field. That is a new object
    lifetime in `cmd/`, which is the one package deliberately chosen so no
    `internal/integration/*` type can hold resolved material.
- Sources:
  - [S1] `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md:85-90`
  - [S2] `internal/integration/delivery/store.go:86-88`, `:198`, `:620-660`, `:663+`
  - [S3] `internal/integration/delivery/dispatcher.go:129,166-182`; `internal/integration/destination/identity.go:182`
  - [S4] `internal/integration/delivery/kafka.go:91`; `delivery_integration_test.go:473-486`; `destination_fixture_test.go:326-338`
  - [S5] `cmd/fi-fhir/destination_identity_runtime.go:123-141,189-191,202-211`
  - [S6] `internal/integration/delivery/types.go:107-114`
  - [S7] `.loom/32-sprint4-execution-specs.md` Lane S4-A, corrections 2, 3, 4, 5, 6

### 2026-08-09: Keep the Kafka Requirement for HTTPS-Only Delivery Deployments, and Document It

- Decision:
  - `buildDeliveryDispatcher` continues to require `FI_FHIR_QUEUE_DRIVER=kafka`
    and a non-empty `FI_FHIR_QUEUE_BROKERS` for the durable delivery worker,
    **even when every destination in the loaded registry declares
    `transport: https`** [S1]. The dependency is documented in `.env.example`
    and in `docs/operations/DESTINATION-IDENTITY.md` rather than relaxed.
  - Relaxing it is filed as a named follow-up ("broker-free delivery worker"),
    not left implicit.
- Rationale:
  - "Every destination is `https`" is a property of one file at one startup, not
    of the deployment. The registry is a single server-owned file read at boot
    [S2]; an operator adding one `kafka`-class destination to it would turn a
    startup configuration error into a runtime dead letter. For a fail-closed
    system that trade goes the wrong way.
  - A mixed registry is the expected steady state during adoption. The Kafka
    topic remains the transport for every destination that has not moved, so the
    broker is not dead weight in any deployment that is actually migrating.
  - Relaxing it means `Dispatcher` must accept a nil `Publisher`, weakening a
    constructor invariant that currently holds for every deployment, in order to
    save a broker for the one deployment class that has already finished
    migrating. The invariant is worth more than the saving today.
  - It costs no new environment variable, so `make check-runtime-config` and
    `scripts/check-runtime-config.sh` are unaffected.
- Alternatives considered:
  - **Relax the requirement when the registry is all-`https`** (rejected for the
    reasons above; revisit once the registry is multi-tenant and reloadable,
    which correction 7 of `.loom/32` records it is not.)
  - **Leave it undecided** (rejected: `.loom/32` Lane S4-A task 8 explicitly
    forbids it.)
- Consequences:
  - An HTTPS-only deployment still stands up a broker it never produces to. This
    is now a documented, deliberate cost with a named follow-up instead of an
    undocumented surprise.
- Sources:
  - [S1] `cmd/fi-fhir/delivery_runtime.go:60-66`
  - [S2] `internal/integration/destination/registry.go:47-53`; `cmd/fi-fhir/destination_identity_runtime.go:42,98-114`
  - [S3] `.loom/32-sprint4-execution-specs.md` correction 8, Lane S4-A task 8
### 2026-08-09: What "one version" means, and why rollback safety is a schema property

- Decision:
  - **The compatibility boundary is the per-package migration ledger version, not
    a git tag and not the binary version string.** There are zero git tags in
    this repository, and `main.version` is a build stamp (`-ldflags -X`,
    defaulting to `0.0.0-dev`) that carries a commit SHA — it says nothing about
    which database schema a process can run against.
  - "One version back" (N-1) means: **the schema at the previous version of one
    ledger, running the binary that expects that previous version.** Six ledgers
    exist — submission, session, lifecycle, batch, destination, terminology —
    and they advance independently, so N-1 is defined per ledger rather than
    across the product.
  - Each owning package exports `SchemaVersion`. `fi-fhir version` prints all
    six, and a new gauge `fi_fhir_schema_ledger_version{ledger}` publishes them,
    so two replicas mid-rolling-upgrade are distinguishable in Prometheus rather
    than only over SSH.
  - **A migration that makes an existing column `NOT NULL` must give it a
    `DEFAULT`.** Written into `AGENTS.md` ("Migration authoring") and
    `docs/developer-guide/testing.md`, and enforced mechanically by
    `TestMigrationRule_NotNullOnExistingColumnCarriesADefault`, which needs no
    database and runs in `test:unit`.
- Rationale:
  - The budget being satisfied is spec budget 6: "one-version rolling upgrade
    and rollback preserve receipts, revisions, and resumable work without schema
    downgrade corruption". It could not even be *stated* before this, because
    nothing defined a version.
  - The ledger version is the only number that answers the operational question.
    Two binaries built from different commits may expect identical schemas; two
    binaries reporting the same version string may not. Rollback safety is a
    property of the schema/writer pair, so the boundary must be the schema.
  - Writing the rule down was the assignment. Making it mechanical is what stops
    it being ignored: the rule already had two violations when it was written,
    and the one inside the one-version window was live.
- Alternatives considered:
  - **Start tagging releases and define N-1 as the previous tag** (rejected as
    the *primary* boundary: a tag is a packaging decision, and nothing prevents
    two consecutive tags from having identical or wildly divergent schemas.
    Tagging is worth doing, but it answers "which artifact" and not "can this
    binary run against this database". Filed for the release-gate work.)
  - **One global schema version across all six ledgers** (rejected: it would
    force a version bump on every package whenever any one of them migrated,
    and make an N-1 claim about session imply an untrue claim about batch.)
  - **Adding six labels to `fi_fhir_build_info`** (rejected: an info metric's
    labels all change together, whereas ledger versions move independently and
    the useful query compares one ledger across replicas.)
  - **Declaring one-version rollback unsupported**, which the spec required be
    presented and rejected explicitly. Rejected: it relaxes a product-spec target
    (`.loom/20-product-spec-integration-engine-ide-completion.md:249-250,279-280`)
    in exchange for avoiding a catalog-only `ALTER TABLE`. The defect cost three
    `DEFAULT`s to fix. There is no version of this trade that favours declaring
    the target unmet.
  - **Amending `0004_export_attribution.sql` in place** (rejected: the ledger
    records version 4 as applied, so an amended file would never re-run on any
    existing database and would only fix fresh installs. Amending an applied
    migration in a slice about migration discipline would be self-defeating.)
- Consequences:
  - Session ledger gains `0006_export_attribution_defaults.sql`. **Lane S4-B's
    session migration is therefore `0007`, not `0006`** as `.loom/32`'s
    file-ownership map assumed. The map has been corrected; the ledger at rebase
    is the authority (`.loom/32` correction 40).
  - `integration_session_exports` now has server-side defaults on
    `principal_json`, `reason`, and `include_raw_payload`. A writer that omits
    them records a *visibly unattributed* export instead of failing. That is a
    real loosening, and it is bounded by an assertion that the current writer
    still records a real principal, so the default cannot mask a live-path
    regression.
  - `pkg/terminology/db.Migrator.Initialize` now runs in a transaction under
    `pg_advisory_xact_lock`, matching the other five. `CurrentVersion` is read
    inside the lock; reading it outside would have left the race intact.
  - Two pre-existing violations of the new rule are recorded as a dated baseline
    in `knownRollbackUnsafeColumns` rather than silently tolerated:
    `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, both from processor ledger 2.
    Processor head is 4, so they are outside the one-version window; repairing
    them needs processor `0005`, which is Lane S4-B's number this sprint. Filed
    for 4.4c.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` Lane S4-C, corrections 23-25
  - [S2] `internal/integration/session/migrations/0004_export_attribution.sql:31-34`
  - [S3] `internal/integration/session/migrations/0006_export_attribution_defaults.sql`
  - [S4] `pkg/terminology/db/migrations.go` (`Initialize`, `currentVersionTx`)
  - [S5] `cmd/fi-fhir/schema_versions.go`; `internal/observability/metrics.go`
  - [S6] `internal/integration/migrationcompat/` (proof, controls, and rule test)

### 2026-08-09: A logical `pg_dump` cannot meet the 5-minute RPO, and a newer client makes it unrestorable

- Decision:
  - `docs/operations/PRODUCTION-HARDENING.md` now states that the documented
    `pg_dump` backup **cannot** meet the product spec's RPO of 5 minutes, and
    marks the two affected rows of the RTO/RPO table as requiring WAL archiving
    / point-in-time recovery. PITR is recorded as the slice 4.4c prerequisite
    for budget 5.
  - The runbook's restore command gains `-v ON_ERROR_STOP=1`, and the backup
    command gains `--no-owner --no-privileges`.
  - The runbook now requires client tools whose **major version matches the
    server**, and `scripts/pgdump-roundtrip.sh` refuses to run on a mismatch.
- Rationale:
  - A periodic logical snapshot bounds loss to the dump interval plus the dump
    duration, on a database whose dump time grows with the data. Scheduling it
    more often does not converge on five minutes; it converges on a permanently
    running dump. Only continuous WAL shipping bounds loss to minutes, and
    nothing in this repository configures it.
  - Without `ON_ERROR_STOP=1`, `psql` prints errors, continues, and exits 0. A
    restore that failed to recreate the Slice 4.1d C1 immutability triggers
    would look like a success, and the recovered deployment would silently have
    weaker PHI governance than the one it replaced. This is the difference
    between a backup and the appearance of one.
  - The version-skew rule was found by running the proof, not by reading docs.
    pg_dump 17 and later write `SET transaction_timeout = 0` into the archive
    preamble; PostgreSQL 16 has no such GUC and rejects it. An operator on a
    workstation with newer client tools produces a dump that exits 0 and cannot
    be restored into the very server it came from — discovered during recovery,
    which is the worst possible moment.
- Alternatives considered:
  - **Quietly restoring with a newer client and filtering the offending `SET`**
    (rejected: it would make the proof pass while leaving every operator exposed
    to the same trap, and it asserts something upstream does not promise —
    archives are intended for a server of the dumping client's version or newer.)
  - **Leaving the RTO/RPO table as aspirational without comment** (rejected:
    it is presented as an operational commitment and read as one.)
  - **Implementing WAL archiving in this slice** (rejected: it is an
    infrastructure change spanning the chart, the storage class, and an object
    store, with no CI-runnable proof. That is 4.4c.)
- Consequences:
  - CI's `test:migration-compatibility` installs `postgresql-client-16` from
    PGDG, because Debian trixie — the `golang:1.26.5` base — ships only
    `postgresql-client-17`, which cannot restore into the PostgreSQL 16 service.
  - Local reproduction needs a matching client too:
    `brew install postgresql@16` and
    `FI_FHIR_PG_BIN_DIR=/opt/homebrew/opt/postgresql@16/bin`.
  - `docs/operations/SUPPORTED-1.0.md` item 4 (backup/restore and RPO/RTO proof)
    stays blocking, and now has a stated reason rather than an empty checkbox.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` correction 27
  - [S2] `docs/operations/PRODUCTION-HARDENING.md` "Recovery objectives, honestly"
  - [S3] `scripts/pgdump-roundtrip.sh`
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md:277-278`

### 2026-08-09: WAL/PITR posture — this repository certifies RTO and hands RPO to the operator, with a method

- Decision:
  - **Option A.** `fi-fhir` ships the point-in-time-recovery posture as
    *documentation plus a verified restore procedure*, and states the RPO a
    deployment achieves as a function of the operator's archiving choice. It
    does **not** ship an `archive_mode`/`archive_command` configuration.
  - The **RTO half of budget 5 (≤ 30 minutes) is measured and certified** by
    this repository, against the documented method: slice 4.4c times
    `scripts/pgdump-roundtrip.sh` end to end — dump, restore, first successful
    delivery `Claim` from the restored database — in the same CI job that
    already proves the restore is faithful, and archives the number.
  - The **RPO half is an operator responsibility with a stated method**, not a
    product guarantee. `docs/operations/PRODUCTION-HARDENING.md` and
    `docs/operations/SUPPORTED-1.0.md` say so in the same words, and
    `SUPPORTED-1.0.md`'s required-evidence item 4 is split accordingly: the
    backup/restore proof and the RTO measurement close; the RPO number does
    not, and the reason is named rather than left as an empty checkbox.
  - **Option B — a reference archiving configuration proved end to end in CI —
    is filed as a named follow-up** with its cost written down (below), not
    silently deferred.
  - **Option C — relax budget 5 — is presented and rejected.** The 5-minute
    target is right for a PHI integration engine. What is wrong is the
    *documented method*, and amending a product-spec target to match a weak
    method is the wrong repair.
- Rationale:
  - The gap is already recorded honestly in shipped documentation
    (`PRODUCTION-HARDENING.md` "Recovery objectives, honestly", and the
    2026-08-09 `pg_dump` decision above). The open question for 4.4c was never
    *whether* the logical dump meets the RPO — it does not — but *what this
    repository ships in response*.
  - PITR belongs to whoever runs the database. The only PostgreSQL in
    `deploy/` is a single-replica `Deployment` on a ReadWriteOnce PVC
    (`deploy/kubernetes/base/postgres.yaml`), which is a development
    convenience; a production deployment uses a managed service or a
    PostgreSQL operator, both of which own WAL archiving through their own
    interfaces. A reference `archive_command` written against the dev manifest
    would be a configuration almost nobody runs, carrying the authority of a
    product guarantee.
  - Option A spends the lane's budget on the parts only this repository can
    prove, and every one of them is CI-runnable today: that a restored database
    is *faithful* (rows, PHI payloads, immutability triggers attributable to
    those triggers, the `NOT VALID` provenance CHECK, and all six schema
    ledgers at their declared versions), that the application *resumes* from it
    with no manual repair, and that recovery *time* is bounded and recorded.
  - Option B is not merely larger; it changes what this repository is
    accountable for. It requires turning the dev `Deployment` into a real
    archiving setup, an object-storage service container in CI, a WAL-replay
    assertion, and thereafter ownership of an operational posture the product
    does not otherwise own — while competing for the same lane budget as
    budget 4 (destination recovery under an injected fault) and budget 7
    (Kubernetes upgrade/rollback provability), both of which are provable now.
  - Stating an uncertified RPO would be worse than stating none. The repository
    has a standing rule against retroactive vouching in its schema; the same
    rule applies to operational claims.
- Alternatives considered:
  - **Ship `archive_mode = on` with a placeholder `archive_command` and no
    proof** (rejected: an unproven archiving configuration in `deploy/` reads
    as a supported capability and is the exact shape of the tracing-enabled
    artifacts slice 4.4d is currently removing).
  - **Measure RPO by dump interval and publish that number** (rejected: the
    interval is not the loss bound — loss is the interval plus the dump
    duration, which grows with the data. Publishing the interval as an RPO is
    a claim the method cannot support.)
  - **Leave item 4 of `SUPPORTED-1.0.md` wholly blocking** (rejected: it
    conflates two independently provable things. The restore proof and the RTO
    measurement are done and should be recorded as done; the RPO number is not
    and should be recorded as an operator responsibility with a method.)
- Consequences:
  - `SUPPORTED-1.0.md` item 4 splits into a closed half (backup/restore
    faithfulness plus a measured RTO against the documented method) and an open
    half (RPO, operator-owned, achieved by the operator's archiving choice).
    The product does not claim a 5-minute RPO.
  - `PRODUCTION-HARDENING.md`'s RTO/RPO table stops reading as an operational
    commitment. The two rows whose RPO is unachievable with logical dumps say
    who owns the number and what method achieves it, and the RTO column carries
    the measured value for the database-failure row rather than a target.
  - The archived RTO is a measurement of the *documented procedure* on a CI
    fixture, not a capacity claim about production data volumes. It is recorded
    with the row counts it was measured against, for the same reason
    `values-reference-profile.yaml` says it is not a capacity claim.
  - **Follow-up filed (Wave 3): "reference WAL archiving configuration".**
    Cost, written down so it can be scheduled rather than rediscovered: convert
    `deploy/kubernetes/base/postgres.yaml` to a `StatefulSet` with a WAL volume
    or adopt an operator; an `archive_command` targeting object storage plus
    credentials plumbing; a MinIO service container in CI; a restore-to-
    timestamp script; and a WAL-replay assertion that proves a transaction
    committed after the base backup is recovered. Only then does budget 5's RPO
    become a product claim.
  - Slice 4.4c ships **no new durable schema** under this decision, so the two
    pre-existing rollback-unsafe columns filed to 4.4c by the 2026-08-09
    rollback decision — `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, both from processor ledger 2 —
    are **re-filed to Lane S5-F**, which owns processor `0006`
    (`.loom/33-sprint5-execution-specs.md`, Schema Freeze Status Per Ledger).
    They stay in `knownRollbackUnsafeColumns` with their dated reason until
    then.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-B, "The WAL/PITR
    posture decision (required deliverable)"; corrections 12-14
  - [S2] `docs/operations/PRODUCTION-HARDENING.md` "Recovery objectives,
    honestly" and "What the restore proof covers"
  - [S3] `.loom/40-decisions.md`, 2026-08-09 "A logical `pg_dump` cannot meet
    the 5-minute RPO"
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md:277-278`
  - [S5] `deploy/kubernetes/base/postgres.yaml`; `scripts/pgdump-roundtrip.sh`
