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
