# Worklog

Chronological notes while executing the plan (useful for handoffs and debugging).

## Template

### YYYY-MM-DD

- What changed:
- Why:
- What’s next:
- Sources:
  - [S1] …

### 2026-02-11

- What changed:
  - Initialized `.loom/` context pack and generated workspace snapshot.
  - Recorded MCP inventory for this session (no resources/templates returned).
  - Authored initial research brief, product spec, implementation plan, and decision log for ETL/parsing-transform/API-contract/auditability program.
- Why:
  - User requested a concrete plan foundation to start backend enhancement work.
- What’s next:
  - Resolve open API-surface decision (GraphQL-only runtime vs GraphQL+REST).
  - Start M0 contract baseline implementation (compatibility matrix + CI gate).
  - Draft schema for persistent ETL run/checkpoint + audit envelope.
- Sources:
  - [S1] Command: `python /Users/cblevins/.codex/skills/plan-loom-core/scripts/init_loom_context.py --root .`
  - [S2] Command: `python /Users/cblevins/.codex/skills/plan-loom-core/scripts/workspace_snapshot.py --root .`
  - [S3] Tool output: `functions.list_mcp_resources` → `{\"resources\":[]}`
  - [S4] Tool output: `functions.list_mcp_resource_templates` → `{\"resourceTemplates\":[]}`

### 2026-03-01

- What changed:
  - Reviewed commit stream since 2026-02-01 and identified integration-heavy slices (`4a6048d`, `8b58964`, `6e1c5e7`, `96550d1`, `843ba26`).
  - Replaced stale MCP inventory with loom-mode evidence (`44` servers, `456` tools) and documented codebase index constraint (`total_chunks=0`, stuck index job).
  - Updated `.loom` planning docs to a platform-integration program covering backend↔frontend completion and sibling-service integration with `flexinfer`, `mentatlab`, and `loom-core`.
  - Created GitLab tracking issues for delivery milestones:
    - [#9](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/9) M1 backend↔frontend parity
    - [#10](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/10) M2 flexinfer integration
    - [#11](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/11) M3 mentatlab integration
- Why:
  - User requested a current-state review plus planning that integrates the full platform and sibling repos.
- What’s next:
  - Execute M0 tasks: promote contract gate to blocking, add endpoint smoke checks, and finalize cross-service auth/timeout defaults.
  - Start M1 execution for backend↔frontend CI parity using live GraphQL operations.
  - Open implementation issues for M2/M3 adapters (`flexinfer`, `mentatlab`) with explicit acceptance tests.
- Sources:
  - [S1] Command: `git log --since='2026-02-01' --date=short --pretty=format:'%h %ad %s' -n 80`
  - [S2] Commands: `git show --stat --oneline ... 4a6048d 8b58964 6e1c5e7 96550d1 843ba26`
  - [S3] Tool output: `read_mcp_resource(server='loom', uri='loom://config')`
  - [S4] Tool output: `read_mcp_resource(server='loom', uri='loom://tools/index')`
  - [S5] Tool output: `mcp__loom__codebase_memory__codebase_stats(repo_id='fi-fhir')`
  - [S6] `internal/api/graphql/server.go:126`
  - [S7] `ui/nginx/default.conf.template:40`
  - [S8] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
  - [S9] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
  - [S10] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
  - [S11] Command output: `glab issue create --repo libs/fi-fhir --title \"M1: Complete backend↔frontend integration parity (GraphQL + UI runtime contracts)\" ...` → `https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/9`
  - [S12] Command output: `glab issue create --repo libs/fi-fhir --title \"M2: Integrate flexinfer inference path with timeout/retry/error contracts\" ...` → `https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/10`
  - [S13] Command output: `glab issue create --repo libs/fi-fhir --title \"M3: Integrate mentatlab run orchestration and SSE lifecycle events\" ...` → `https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/11`

### 2026-03-04

- What changed:
  - Promoted `lint:contracts` from `allow_failure: true` to blocking merge gate (M0 exit criterion).
  - Added `scripts/smoke-test.sh` covering `/health`, `/graphql`, `/graphql/ws` endpoint reachability.
  - Added `smoke-test` and `smoke-test-local` Makefile targets.
  - Moved 15 root-level `ROADMAP_RECONCILIATION_*.md` files into `docs/` for consistency.
- Why:
  - M0 milestone requires enforced contract governance and endpoint smoke baseline.
  - Reconciliation files were cluttering the repo root and inconsistent with the `docs/` convention.
- What's next:
  - Validate full CI pipeline passes with blocking contract gate on `main`.
  - Begin M1 execution: backend↔frontend integration parity (issue #9).
  - Open implementation tasks for M2/M3 adapters.
- Sources:
  - [S1] Contract check: `make contract-check-strict` → 36/36 events, zero drift.
  - [S2] `.gitlab-ci.yml:324-325` — removed `allow_failure: true`.
  - [S3] New file: `scripts/smoke-test.sh`.
  - [S4] `Makefile:218-225` — added `smoke-test` / `smoke-test-local`.

### 2026-03-16

- What changed:
  - Added CLI telemetry coverage for terminology mapping decisions:
    - `fi-fhir terminology mapping decisions`
    - `fi-fhir terminology mapping decision`
    - `fi-fhir terminology mapping decision-stats`
  - Updated `terminology mapping resolve` to record CLI decisions into `terminology.mapping_decisions` for persistent hits, autoroute results, and no-match outcomes.
  - Added focused CLI tests plus an integration test scaffold for the new telemetry commands.
  - Updated terminology planning docs and added a dedicated iteration plan for this slice.
- Why:
  - The roadmap/spec still showed terminology CLI/telemetry gaps even though the persistence layer already existed. This closes a high-value auditability gap with a small, shippable increment.
- What’s next:
  - Re-run focused Go tests after freeing disk space on the workstation.
  - Continue Phase 3/5 telemetry follow-ups: OpenTelemetry attributes, analytics query/UI, and retention/partitioning work.
- Sources:
  - [S1] `cmd/fi-fhir/terminology.go`
  - [S2] `cmd/fi-fhir/terminology_telemetry_cli_test.go`
  - [S3] `cmd/fi-fhir/terminology_telemetry_integration_test.go`
  - [S4] `.loom/iteration-plan-m2-terminology-telemetry.md`
