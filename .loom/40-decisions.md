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
    principal context, POST-only raw payloads, bounded bodies, explicit HTTP and
    WebSocket origins, adapter parity, and fail-closed legacy submit/session
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
