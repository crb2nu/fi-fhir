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
