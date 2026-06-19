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

### 2026-03-29

- What changed:
  - Reviewed `feat/ide-integration` against `origin/main` and narrowed the unfinished branch work to the workflow debug surface rather than the IDE chrome itself.
  - Wired the debug panel to the real GraphQL debug mutations instead of always loading mocks.
  - Updated backend debug sessions to pause on the first executable span by default and to preserve a stable `createdAt` timestamp.
  - Added frontend-derived trace/lineage synchronization from real debug steps so the bottom-panel debug/trace views stay useful even while server trace queries remain stubbed.
  - Validated the backend debug surface with focused Go tests and the frontend debug/editor surface with focused Vitest plus UI typecheck.
- Why:
  - The branch looked feature-complete visually, but the actual debug session flow was not integrated end to end: the UI started sessions with empty workflow YAML, relied on mock state, and could not truthfully drive the backend debugger.
- What’s next:
  - Implement real server-backed `workflowRunTrace` query results instead of frontend-derived placeholders.
  - Implement a real `debugStepEvent` subscription or another broadcast mechanism if live push updates are still desired.
  - Triage the unrelated `ui/src/lib/ui/ide/ideStore.test.ts` runner-local `localStorage.clear` failure separately from this branch work.
- Sources:
  - [S1] Command: `git diff --stat 9cf0bf4006218b143c4184559d955c6f0428ddcf..HEAD`
  - [S2] `ui/src/lib/features/debug/debugApi.ts`
  - [S3] `ui/src/lib/features/debug/DebugPanel.svelte`
  - [S4] `ui/src/lib/features/debug/debugStore.ts`
  - [S5] `internal/workflow/debug.go`
  - [S6] `internal/api/graphql/resolvers/debug.resolvers.go`
  - [S7] Command: `GOCACHE=$PWD/.tmp/go-build-cache GOMODCACHE=$PWD/.tmp/go-mod-cache go test ./internal/workflow ./internal/parser/hl7v2 ./internal/api/graphql/...`
  - [S8] Command: `npm run typecheck`
  - [S9] Command: `npm test -- --run src/lib/features/debug/DebugPanel.test.ts src/lib/features/debug/debugStore.test.ts src/lib/features/debug/TraceTimeline.test.ts src/lib/features/debug/VariableInspector.test.ts src/lib/features/debug/StepControls.test.ts src/lib/ui/editor/CodeEditor.test.ts`

### 2026-03-30

- What changed:
  - Finished the remaining backend integration for the debug surface by implementing `workflowRunTrace` from recorded runtime spans and wiring `debugStepEvent` to live debug-session pause broadcasts.
  - Fixed debugger control semantics so `debugStep`/`debugContinue` discard the already-buffered current pause before waiting for the next one.
  - Fixed debug-session shutdown so `Close()` cannot hang if the workflow engine continues traversing spans while unwinding after a stop command.
  - Added focused resolver coverage for trace retrieval and live step subscriptions, then re-ran the workflow and GraphQL package tests on the changed backend surface.
- Why:
  - The March 29 UI integration made the debugger usable, but two backend contracts were still placeholders, and the first live subscription test exposed that stepping and stop behavior were not yet internally consistent.
- What’s next:
  - Re-run the frontend debug suite once more if we touch the UI again, but no additional client changes were required for this backend completion pass.
  - If the branch is meant to ship immediately, the next practical step is a broader branch review plus commit/merge prep rather than more debugger feature work.
- Sources:
  - [S1] `internal/workflow/tracing.go`
  - [S2] `internal/workflow/debug.go`
  - [S3] `internal/api/graphql/resolvers/schema.resolvers.go`
  - [S4] `internal/api/graphql/resolvers/debug.resolvers.go`
  - [S5] `internal/api/graphql/resolvers/debug_subscription_test.go`
  - [S6] `internal/api/graphql/resolvers/workflow_lifecycle_test.go`
  - [S7] Command: `GOCACHE=$PWD/.tmp/go-build-cache GOMODCACHE=$PWD/.tmp/go-mod-cache go test ./internal/workflow ./internal/api/graphql/...`

### 2026-06-18

- What changed:
  - Diagnosed MR `!80` after auto-merge stalled and found `lint:go` failed with `job_execution_timeout`.
  - Switched `lint:go` from source-building golangci-lint to the pinned `golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine` image available through Harbor.
  - Increased `lint:go` to 2 CPU / 4 GiB and a 30-minute golangci-lint timeout after the image-based job exposed cold-cache package loading as the next bottleneck.
  - Added a RALPH iteration plan for the CI repair slice.
- Why:
  - The previous source-profile template slice could not satisfy RALPH exit criteria while CI was red, and the failure was a CI bootstrap problem rather than a product-code lint finding.
- What’s next:
  - Push the CI repair commit to MR `!80`, re-arm auto-merge, and monitor the replacement pipeline.
  - Resume Wave 2 Slice 0 LLM reachability kill-test after the MR is green/queued.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] `.loom/iteration-plan-ci-golangci-lint-image.md`
  - [S3] GitLab pipeline `14326`, job `142164`

### 2026-06-19

- What changed:
  - Converted the latest Loom brainstorm/spec docs into `.loom/24-parallel-execution-specs.md`.
  - Split follow-up work into parallel lanes: Workflow AI verification, LLM config/capability, pending-autoroute automation, terminology DB integration recovery, integration CI hardening, and product speclets.
  - Updated `.loom/00-index.md` and `.loom/30-implementation-plan.md` to point future agents at the new handoff map.
  - Refreshed the workspace snapshot per `plan-loom-core`, then reverted that generated change because the local worktree remote URLs include credentials and should not be persisted in planning docs.
- Why:
  - Several brainstorm assumptions have changed since the docs were written. In particular, workflow generate/explain is already wired in current code, so the remaining work should be verification/hardening instead of duplicate wiring.
- What's next:
  - Start Wave P1 lanes in parallel: A, B, D, and F from `.loom/24-parallel-execution-specs.md`.
  - Defer pending-autoroute sweep/notifications until Lane D records a stable terminology DB integration-test baseline.
- Sources:
  - [S1] `.loom/24-parallel-execution-specs.md`
  - [S2] `.loom/23-functionality-gaps-plan.md`
  - [S3] `ui/src/lib/features/workflows/components/GenerateFromDescription.svelte`
  - [S4] `ui/src/lib/features/workflows/components/WorkflowPreview.svelte`
  - [S5] `pkg/llm/config.go`
  - [S6] `pkg/terminology/db/mappings.go`

### 2026-06-19 - Lane A workflow AI verification

- What changed:
  - Added component coverage for Workflow Builder "Generate from Description":
    real `generateWorkflow(description, ALL_EVENT_TYPES, ACTION_TYPES)` dispatch, generated YAML/warnings/explanation rendering, valid-YAML-only draft loading, and the invalid-YAML kill-test.
  - Added component coverage for YAML Preview "Explain with AI":
    real `explainWorkflow(yamlOutput, "business")` dispatch, top-level + route explanation rendering, and no duplicate local toast when the GraphQL net already toasted the error.
  - Corrected `.loom/23-functionality-gaps-plan.md` so Wave 2b is recorded as verified rather than an old generic unwired gap.
- Why:
  - `.loom/24-parallel-execution-specs.md` identified this lane as verification/polish: the product path was already wired, but lacked focused tests and the old plan text was stale.
- Verification:
  - `cd ui && npm test -- --run src/lib/features/workflows` → 87 pass.
- Sources:
  - [S1] `ui/src/lib/features/workflows/components/GenerateFromDescription.test.ts`
  - [S2] `ui/src/lib/features/workflows/components/WorkflowPreview.test.ts`
  - [S3] `.loom/23-functionality-gaps-plan.md`

### 2026-06-19

- What changed:
  - Lane F split the product expansion backlog into five independent speclets:
    - `.loom/25-spec-cda-section-expansion.md`
    - `.loom/26-spec-storage-provider-tests.md`
    - `.loom/27-spec-terminology-governance.md`
    - `.loom/28-spec-fhir-ig-bulk-smart.md`
    - `.loom/29-spec-profile-management-observability.md`
  - Added `.loom/00-index.md` links for the new speclets.
- Why:
  - The product spec and P3 backlog were too broad for one implementation lane. Each child spec now has explicit goals, non-goals, acceptance criteria, kill-test, dependencies, sources, and an independent assignment note.
- What's next:
  - Downstream agents can pick up a single speclet and start with its kill-test before implementation.
  - Terminology governance should wait for Lane D's terminology DB integration-test baseline and avoid duplicating Lane C's expiry/notification automation.
- Sources:
  - [S1] `.loom/24-parallel-execution-specs.md` Lane F
  - [S2] `.loom/20-product-spec.md`
  - [S3] `docs/planning/README.md` P2/P3 backlog
  - [S4] `docs/planning/CDA-CCDA.md`
  - [S5] `docs/planning/TERMINOLOGY-MAPPING.md`
  - [S6] `docs/planning/FHIR-PROFILES.md`
  - [S7] `docs/planning/SOURCE-PROFILES.md`

### 2026-06-19 - Lane B LLM config namespace + capability surface

- What changed:
  - Made `pkg/llm.Config.WithEnv()` prefer canonical `FI_FHIR_LLM_*` variables before legacy `LLM_*`, preserving `OPENAI_API_KEY` as the final API-key fallback.
  - Mirrored the same base URL/API key/default model/quality model precedence in `pkg/config.ApplyEnv()`.
  - Added a safe GraphQL `llmCapability` query with `enabled`, `configured`, provider host, model names, `status`, and warnings only.
  - Changed serve-time LLM wiring to honor `FI_FHIR_LLM_ENABLED` and report disabled/unavailable/degraded/available from actual initialization.
  - Updated LLM docs with canonical and legacy variable names.
- Why:
  - Lane B needed runtime LLM configuration to be unambiguous while keeping existing `LLM_*` deployments working.
- Verification:
  - `go test ./pkg/config ./pkg/llm ./cmd/fi-fhir ./internal/api/graphql/...` → passed.
  - `go run github.com/99designs/gqlgen generate --config gqlgen.yml` → passed.
- Sources:
  - [S1] `pkg/llm/config.go`
  - [S2] `pkg/config/config.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `internal/api/graphql/schema.graphql`
  - [S5] `docs/user-guide/llm-features.md`

### 2026-06-19 - Lane D terminology DB integration CI

- What changed:
  - Ran the requested `pkg/terminology/db` integration baseline. The no-env testcontainers path failed in `TestICD10Loader_Integration_LoadCSV` with `port "5432/tcp" not found`.
  - Verified the full package is green through the CI-compatible `POSTGRES_TEST_URL` path against an isolated Postgres service.
  - Wired `.gitlab-ci.yml` `test:integration` to run `./pkg/terminology/db/` after `./cmd/fi-fhir/...`, with `POSTGRES_TEST_URL` pointing at the existing CI Postgres service and `-p 1` serialization to avoid shared `terminology` schema collisions.
  - Updated terminology DB test comments and planning docs with the exact CI/local command.
- Why:
  - Lane D needs approval/autoroute store behavior protected in CI without relying on Docker-in-Docker or parallel packages that can both `DROP SCHEMA terminology CASCADE`.
- What's next:
  - Keep loader fixtures in the full package path; no exclusions were needed after the external-DSN run passed.
  - Lane C can build pending-autoroute sweep/notification work on this stable store-test base.
- Sources:
  - [S1] Command: `go test -tags=integration ./pkg/terminology/db/` -> failed via testcontainers port discovery.
  - [S2] Command: `POSTGRES_TEST_URL=postgres://testuser:testpass@localhost:55433/fi_fhir_test?sslmode=disable go test -tags=integration ./pkg/terminology/db/` -> passed.
  - [S3] `.gitlab-ci.yml`
  - [S4] `pkg/terminology/db/migrations_integration_test.go`
  - [S5] `pkg/terminology/db/mappings_integration_test.go`
  - [S6] `docs/planning/README.md`
