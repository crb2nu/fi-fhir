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
