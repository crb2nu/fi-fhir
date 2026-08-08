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

### 2026-08-03 - Lane C1 pending-autoroute expiry sweep

- What changed:
  - Added `internal/terminology/autoroute/sweeper.go`: a cancellable,
    interval-driven `Sweeper` (`SweepOnce` / `Run`) over a narrow
    `PendingAutorouteExpirer` interface, so scheduling stays out of the DB
    package and sweep logic is unit-testable without Postgres.
  - Wired it into `serve` as a background component beside the MLLP, delivery,
    and batch runners: same `serveCtx` cancellation boundary, same `errCh`
    reporting, added to `waitForBackgroundStops`, `errCh` buffer 4 -> 5.
  - Added `FI_FHIR_TERMINOLOGY_AUTOROUTE_SWEEP_INTERVAL` (default `15m`, `0`
    disables) to `pkg/config`, the `fi-fhir config env` table, and
    `.env.example`.
  - Documented the two-layer expiry model in
    `docs/planning/TERMINOLOGY-MAPPING.md` and synced the `Terminology
    Autoroute` row in `docs/STATUS.md` (88.5% -> 91.6%).
- Why:
  - `ExpirePendingAutoroutes` had existed since the approval-workflow slice with
    zero production callers; only the shipped query-time guard kept the review
    queue truthful. Lane C was gated on Lane D's terminology DB integration
    baseline, which shipped 2026-06-19.
- Decisions:
  - Config owns the default cadence; the sweeper constructor rejects a
    non-positive interval instead of silently no-opping, so "disabled" is a
    deployment choice rather than a hidden fallback.
  - `Run` sweeps immediately on boot then on each tick, continues past iteration
    failures, and returns `nil` on cancellation so shutdown is not reported as a
    component failure.
  - Observability is a typed `SweepResult` + `Observe` hook printed by serve.
    Metrics deferred: there is no shared serve-wide Prometheus registry, and
    `internal/workflow`'s events/actions/DLQ-shaped `Metrics` interface is the
    wrong abstraction to import into terminology.
- Findings:
  - `internal/terminology/autoroute` transitively imports `pkg/terminology/db`,
    so the kill-test had to go in the **external** `db_test` package. That
    placement also means CI needs no change: `test:integration` already runs
    `./pkg/terminology/db/`.
  - `test:integration` is still `allow_failure: true`, so the kill-test runs but
    does not block. Hand this to Lane E as concrete promotion evidence.
  - The `FI_FHIR_MAPPING_*` env block in `docs/planning/TERMINOLOGY-MAPPING.md`
    documents variables that do not exist in code. Left in place (out of scope)
    but the new section is explicitly marked as implemented to avoid inheriting
    the ambiguity. Worth a dedicated docs slice.
- Verification:
  - `go test ./internal/terminology/autoroute/ ./pkg/config/ -race` -> passed.
  - `go test -race -coverprofile=coverage.out -covermode=atomic -cover ./...`
    -> passed (exit 0), same command CI's `test:unit` runs.
  - Kill-test against real Postgres:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/ -run ExpirySweep`
    -> passed. **Negative control**: with the store call removed from
    `SweepOnce`, the same test fails with "stored status after sweep = pending,
    want expired", proving it detects an absent sweep rather than passing on the
    query-time guard.
  - Full package twice against the same Postgres -> passed both times (47.8s,
    46.6s), preserving Lane D's schema-isolation property with the new
    schema-dropping test added.
  - `bash scripts/docs-status.sh --check-drift` -> exit 0, no coverage drift.
  - `golangci-lint run` on changed packages -> 0 issues (it caught an unused
    `//nolint` directive that would have failed CI `lint:go`).
  - `go vet ./...`, `gofmt -l` -> clean.
- What's next:
  - Lane C2: notification interface for new / high-confidence pending
    autoroutes (webhook config, thresholds, non-blocking dispatch).
  - Lane E: `allow_failure` inventory; `test:integration` now has real
    store-and-sweep coverage worth promoting.
- Sources:
  - [S1] `internal/terminology/autoroute/sweeper.go`
  - [S2] `pkg/terminology/db/sweeper_integration_test.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `pkg/config/config.go`
  - [S5] `.loom/iteration-plan-lane-c1-autoroute-expiry-sweep.md`

### 2026-08-08 - Lane C2 pending-autoroute review notifications

- What changed:
  - Added `internal/terminology/autoroute/notify.go`: a `ReviewNotifier` that
    scans the pending-autoroute review queue on an interval, filters by a
    configurable confidence floor, de-duplicates by row ID, and dispatches a
    PHI-minimal JSON digest through a `NotificationSink`. `WebhookSink` is the
    only implementation (generic webhook, no Slack-specific logic).
  - Serve wiring in `cmd/fi-fhir/main.go` under the existing `serveCtx` /
    `errCh` / `waitForBackgroundStops` boundary, beside the C1 sweeper. `errCh`
    buffer 5 -> 6 for the extra component.
  - Config in `pkg/config/config.go`:
    `terminology.autoroute_notify.{notification_webhook,interval,min_confidence,timeout}`
    with `FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_*` env equivalents. Empty webhook
    disables the feature; validation only runs when a webhook is configured.
  - Kill-test in the external `db_test` package:
    `pkg/terminology/db/notify_integration_test.go`.
  - Docs: `docs/planning/TERMINOLOGY-MAPPING.md` (new "Pending Autoroute Review
    Notifications (implemented)" section, Phase 5 checkbox), and
    `docs/user-guide/terminology.md` (env table + "Review Notifications").
- Why:
  - Lane C tasks 3-5, the remaining half of `.loom/24-parallel-execution-specs.md`
    Lane C after C1 shipped the expiry sweep on 2026-08-03.
- Decisions:
  - **Periodic digest, not a per-event hook in the creation path.** The lane
    allowed either. A digest makes "notification failures never affect
    resolution" structural — nothing on the resolution or
    `CreatePendingAutoroute` path calls the notifier — rather than a discipline
    each call site has to maintain. It is also the more useful signal
    (`eligible_count` makes queue depth alertable), and it avoids firing on every
    re-resolution of the same code, since creation upserts on the natural key.
    Freshness is bounded by the scan interval, which is immaterial against a
    30-day review expiry window.
  - The per-event door is open and not dead code: `Notify` is the non-blocking
    bounded-queue entry point and the scan loop itself calls it, so a future
    per-event hook inherits the same drop-rather-than-block contract.
  - Same narrow-interface discipline as C1: `PendingAutorouteLister`, not
    `*db.MappingStore`.
  - Payload excludes every free-text / LLM-authored column
    (`source_display`, `suggested_display`, `reasoning`, `decision_trace`,
    `alternates`, `reviewed_by`, `rejection_reason`) because they can quote
    source message content and a webhook is untrusted egress.
  - Drop, do not grow: 8-deep queue, one delivery attempt plus one bounded
    retry. Each digest restates the backlog, so a drop loses nothing durable.
- Findings:
  - `httptest.Server.Close` waits for outstanding requests, and a client-side
    `http.Client` timeout does not reliably cancel `r.Context()` first. A
    "hanging receiver" handler that only selects on a release channel plus
    `r.Context().Done()` deadlocks the test binary (hit once, 10m timeout). Both
    hang tests now give the handler an absolute escape timer and release before
    `Close`.
  - macOS `cp` prompts on overwrite even in a script; use `yes | cp` when
    restoring a file after a negative-control experiment, and verify with grep.
- Verification:
  - `go test ./internal/terminology/autoroute/ ./pkg/config/ ./cmd/fi-fhir/ -race`
    -> passed.
  - `go test -race ./...` -> passed.
  - Kill-test against real Postgres 16:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/ -run ReviewNotif`
    -> both tests passed.
    **Negative control**: with the confidence floor removed from both the store
    filter and the in-Go re-check, the same test fails with
    "NOTIFY_LOW_1 (below threshold) was delivered" / "eligible_count = 4, want 2",
    proving it detects an absent threshold rather than passing because the
    low-confidence rows were never created.
  - Full `./pkg/terminology/db/` integration package against the same Postgres
    -> passed (35s), so the new schema-dropping test preserves Lane D's
    isolation property.
  - `golangci-lint run` on changed packages -> 0 issues. `go vet ./...`,
    `gofmt -l` -> clean.
- What's next:
  - Lane E: `allow_failure` inventory. `test:integration` now carries two Lane C
    kill-tests worth promoting.
  - `.loom/27` governance hardening: audit contract on approve/reject, rejection
    context retention, bulk-approval limits, role expectations. Also open there:
    whether notification recipients need per-source-system routing (today it is
    one global webhook; splitting is a config + payload change, not a redesign).
- Sources:
  - [S1] `internal/terminology/autoroute/notify.go`
  - [S2] `pkg/terminology/db/notify_integration_test.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `pkg/config/config.go`
  - [S5] `.loom/iteration-plan-lane-c2-autoroute-notifications.md`

### 2026-08-08 - Lane E integration CI hardening

- What changed:
  - **Lane E (integration/contract CI hardening) shipped.** Branch
    `ci/integration-ci-hardening`.
  - Inventoried all three `allow_failure: true` jobs in `.gitlab-ci.yml` and
    classified them against 40 pipelines of main history
    (18521..22333, 2026-07-13..2026-08-07):
    `test:integration` 24/24 green → promoted; `lint:docs` 33/33 green →
    promoted; `test:docs-status` 29/29 green → deliberately held advisory with
    inline promotion criteria.
  - **Repaired the `minio` service container before promoting.**
    `minio/minio:latest` ships `CMD ["minio"]`, which prints usage and exits, so
    the service never listened on `minio:9000`. `setupTestInfra()` responded with
    `t.Skipf`, not a failure, so **30 integration tests silently skipped in CI** —
    event store, projections, terminology init/status, storage, mapping-decision
    CLI. Added `command: ["server", "/data", "--console-address", ":9001"]`.
  - Fixed the one real defect the dead service had been masking:
    `TestIntegration_TerminologyMappingDecisionCLI` asserted a 23-character
    `GLU-<UnixNano>` code appeared in a column rendered through
    `truncate(decision.SourceCode, 12)`. The fixture now fits the column width.
  - Removed the stale `MINIO_DEFAULT_BUCKETS` variable (a bitnami-only
    convention, inert against `minio/minio`; the bucket is created by
    `ensureMinioBucket`).
  - Corrected docs that described CI inaccurately: `AGENTS.md` still claimed
    `pkg/terminology/db` is not exercised by CI (Lane D wired it in),
    `docs/DOCUMENTATION-CONVENTIONS.md` said `lint:docs` is advisory, and
    `docs/developer-guide/testing.md` showed a fabricated `test:integration`
    snippet (postgres:14 / hapiproject / `./test/e2e/...`).
- Why:
  - Lane E's acceptance criteria require a recent green proof for any promoted
    job and that `test:integration` "no longer gives a false sense of coverage".
    The kill-test proved the green history was partly an artifact of skipped
    tests, so promoting without the MinIO repair would have made a hollow gate
    mandatory — the exact failure mode Gate 0B named for security jobs.
  - Negative-control evidence: with MinIO unreachable `./cmd/fi-fhir/...` reports
    `coverage: 73.2%` (1380 pass / 30 skip), matching CI job 218601 to the
    decimal. With MinIO live it reports `75.9%` (1410 pass / 0 skip).
- What's next:
  - File the three cleanup issues recorded in `.loom/40-decisions.md`:
    (1) make `setupTestInfra` fail rather than skip when CI infra is down;
    (2) resolve `/ready` — either mount it in `serve` and assert it in
    `scripts/smoke-test.sh`, or delete the unused readiness path;
    (3) promote `test:docs-status` to blocking in a dedicated MR.
  - Lane C2 (pending-autoroute notifications) remains the last open lane.
- Sources:
  - [S1] `.gitlab-ci.yml` — `test:integration`, `lint:docs`, `test:docs-status`.
  - [S2] `cmd/fi-fhir/integration_helpers_test.go:56-118`.
  - [S3] CI job 218601 trace (pipeline 22333) — `coverage: 73.2% of statements`.
  - [S4] Command: `docker --context 7900xtx run --rm minio/minio:latest`.
  - [S5] `.loom/40-decisions.md` — 2026-08-08 soft-fail policy entry.
### 2026-08-08

- What changed:
  - Landed Phase 4 Slice 4.1b2: verified MLLP client-certificate service identity
    and fail-closed submit authorization.
  - Added `clients.identities` to the immutable, content-addressed MLLP source
    revision (`internal/integration/mllp/identity.go`, `source.go`). Entries map an
    authority-scoped URI SAN and/or SPKI SHA-256 pin to one canonical service
    subject plus grants. Existing revisions keep their exact digest because the
    field is omitted from the canonical digest input when empty.
  - Resolved one `ConnectionIdentity` per connection immediately after the TLS
    handshake and before any frame read (`server.go`); zero-match and ambiguous
    multi-match certificates close without an acknowledgement.
  - Carried the verified subject/auth-method/grants into the existing
    `authorization.AuthorizeSubmission` decision and moved that decision ahead of
    capacity acquisition and envelope construction (`service.go`).
  - Wired `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY`, forwarded the full MLLP env
    contract through Docker Compose, and extended
    `scripts/check-runtime-config.sh` with a required MLLP compose check.
  - Extended the required `test:mllp-runtime` job and `make mllp-runtime` to
    discover and run both MLLP runtime proofs (minimal `.gitlab-ci.yml` diff: the
    `-list` regex, the `rg` pattern, the expected count, and the job comment;
    `ci/integration-ci-hardening` owns that file this sprint).
- Why:
  - Slice 4.1b1 left MLLP attributing every connection to one deployment-fixed
    principal, so a CA-valid certificate from any sender the client CA trusts was
    indistinguishable at the authorization decision.
- What's next:
  - 4.1b3: bind batch connector/workload identity and replace remote object
    modification time as trusted receipt provenance.
  - 4.1c: destination-scoped identity and secret resolution for the first durable
    HTTPS consumer, then audit and PHI controls.
- Sources:
  - [S1] Kill test: `POSTGRES_TEST_URL=... make mllp-runtime` (both
    `TestPostgresMLLPRuntime_DurableACKPauseRestart` and
    `TestPostgresMLLPRuntime_CertificateIdentityAuthorization` passed with `-race`)
  - [S2] Negative controls: silent fallback for unmatched certificates, ignored
    per-identity grants, and a deployment-fixed principal in mapped mode each
    failed the kill test at assertions 2, 3, and 1 respectively
  - [S3] `.loom/iteration-plan-phase-4-slice-4-1b2-mllp-certificate-identity.md`
  - [S4] RFC 5280 §4.2.1.6 (URI SAN), RFC 6125 §6.4.4 (common-name deprecation)

### 2026-08-08 (Slice 4.1b3)

- What changed:
  - Landed Phase 4 Slice 4.1b3: batch (S3/SFTP) workload identity bound to the
    shared submit decision, plus trusted receipt provenance.
  - Added an optional `workload` block to the immutable, content-addressed batch
    source revision (`internal/integration/batch/identity.go`, `source.go`). It
    names one canonical service subject plus its grants. Existing revisions keep
    their exact digest because the block is omitted from the canonical digest
    input when absent, and grant order is canonicalized so it cannot move a
    digest.
  - Moved the fail-closed `authorization.AuthorizeSubmission` decision to the
    connector boundary in `PollOnce`, ahead of `provider.List`, the PostgreSQL
    lease claim, artifact loading, and every durable write (`service.go`). The
    same decision still runs per message before the processor and inside
    transaction-scoped runnable admission.
  - Replaced remote object modification time as receipt provenance. Authoritative
    `received_at` is now the server-owned custody timestamp
    (`integration_batch_objects.created_at`), and content provenance is a
    SHA-256 digest computed over the exact admitted bytes, resumed across
    checkpoints from marshaled hash state (`provenance.go`,
    `MessageReader.Raw`) and cross-checked against the pre-archive re-read.
  - Renamed `object_modified_at` to `remote_modified_at_advisory` and added
    `object_version`, `object_etag`, and `digest_state` in migration
    `0002_batch_provenance.sql`; the provenance CHECK is `NOT VALID` so rows
    admitted before this revision are visibly distinguishable rather than given
    invented provenance. S3 now pins and re-verifies the entity tag alongside the
    version ID at every read, archive, and delete.
  - Wired `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY`, forwarded the full batch env
    contract through Docker Compose, and extended
    `scripts/check-runtime-config.sh` with a required batch compose check.
  - Extended the required `test:batch-ingestion` job and `make batch-ingestion`
    to discover and run both batch runtime proofs (minimal `.gitlab-ci.yml` diff:
    the `-list` regex, the `rg` pattern, the expected count, and the job comment).
- Why:
  - Slice 4.1b1's handoff flagged two defects in the batch path: every source
    submitted under one deployment-fixed principal, and the receipt's
    `received_at` came from remote object modification time, which an SFTP
    producer can set freely via `SSH_FXP_SETSTAT`.
- What's next:
  - 4.1c: destination-scoped identity and secret resolution for the first durable
    HTTPS consumer.
  - Then control-plane authorization, PHI retention/export controls, and
    immutable audit storage.
- Sources:
  - [S1] Kill test: `POSTGRES_TEST_URL=... BATCH_S3_* =... make batch-ingestion`
    (both `TestBatchIngestion_PostgresS3SFTPKillResumeArchive` and
    `TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance` passed with
    `-race`)
  - [S2] Negative controls, each failing the kill test: deployment-fixed
    principal in bound mode (assertion 1), no connector-boundary decision
    (assertion 2, `integration_batch_objects` grew from 2 to 3 after denial),
    remote modification time as received-at (assertion 4), ignored per-identity
    grants (assertion 2, ungranted poll returned 1/nil), and a streaming digest
    over normalized rather than raw bytes (assertion 4, object quarantined with
    an empty content digest)
  - [S3] `.loom/iteration-plan-phase-4-slice-4-1b3-batch-workload-identity.md`
  - [S4] SFTP draft §6.7 `SSH_FXP_SETSTAT` (client-settable modification time)
### 2026-08-08 - Phase 4 Slice 4.2a operator control plane

- What changed:
  - Implemented Phase 4 Slice 4.2a, the operator control-plane GraphQL API.
  - Added `internal/integration/operator`: a tenant-scoped PostgreSQL read
    projection (receipts, canonical events, lineage, delivery attempts, DLQ,
    circuits, delivery audit) with keyset pagination and opaque cursors, plus a
    role-gated control service that delegates writes to
    `internal/integration/delivery` and `internal/integration/lifecycle`.
  - Added `PostgresStore.Discard` and a DLQ `resolution`/`resolved_at` column via
    submission migration `0003_operator_control_plane`, and
    `PostgresCatalog.ListSnapshots` for the deployment inventory.
  - Extended `schema.graphql` with nine operator queries and seven control
    mutations, regenerated gqlgen artifacts, and implemented the resolvers.
  - Allowlisted the control plane's catalog-safe messages in the GraphQL error
    presenter so conflicts are distinguishable without leaking inventory.
  - Wired the control plane into `serve` behind the existing durable submission
    database, sharing one lifecycle catalog with session publication.
  - Added required CI job `test:operator-control-plane` and `make
    operator-control-plane`.
- Why:
  - Slice 4.2 requires the failure/replay and operator-audit golden journeys to
    pass without SQL or manual filesystem intervention, over the durable records
    Slices 2.1 and 2.3 already own.
- Evidence:
  - Kill-test `TestOperatorControlPlane_FailureReplayAndAuditGoldenJourneys`
    passes on PostgreSQL 16 with `-race`.
  - Negative control: emitting scalar values from the payload summarizer makes
    the kill-test fail on the raw-PHI sentinel assertion, so the leak check is
    not vacuous.
  - The required delivery-reliability proof caught a real defect: the resubmit
    DLQ resolution label was built by string concatenation and produced an
    invalid value. Fixed, and the kill-test now exercises resubmit directly.
  - `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` clean,
    `go test -race ./...` green, `go mod verify` and `go mod tidy -diff` clean.
  - Sibling PostgreSQL suites re-verified: lifecycle, processor, session, and
    the Kafka-backed delivery-reliability proof.
- What's next:
  - Ship 4.2a, then branch 4.2b from fresh main for the operator UI.
- Sources:
  - [S1] `.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`
  - [S2] `internal/api/graphql/operator_control_plane_integration_test.go`
  - [S3] Command: `make operator-control-plane` with `POSTGRES_TEST_URL` set

### 2026-08-08 - Sprint 3 Lane S3-B file ownership and day-1 gate (Slice 4.1c-a)

- Lane: S3-B, branch `feat/phase4-slice-4-1c-destination-identity`, spec
  `.loom/31-sprint3-execution-specs.md`.
- Owned files (declared before first commit, per the spec's coordination rules):
  - `internal/integration/destination/**` (new package, including its own
    migration set `internal/integration/destination/migrations/0001_*.sql`)
  - `internal/integration/authorization/policy.go` and its tests
  - `internal/integration/delivery/dispatcher.go`,
    `internal/integration/delivery/types.go`,
    `internal/integration/delivery/identity.go` (new), and this lane's delivery
    test files
  - `pkg/integration/secret.go` (new)
  - `cmd/fi-fhir/destination_identity_runtime.go` (new) plus one appended
    destination-identity block in `cmd/fi-fhir/main.go` after the delivery block
  - `.loom/iteration-plan-phase-4-slice-4-1c-a-destination-identity.md`,
    `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md`
  - Appended-only edits: `.gitlab-ci.yml` (`test:delivery-identity` at the end of
    the test stage), `Makefile` (`delivery-identity` target), `.env.example` and
    `docker-compose.yaml` (`FI_FHIR_DELIVERY_IDENTITY_*` block),
    `docs/operations/*`
- Migration numbers claimed: **none in `processor/` or `session/`** — S3-C1 owns
  `0004` in both. This lane creates its own package-local set starting at
  `internal/integration/destination/migrations/0001_delivery_identity.sql` with
  its own `integration_destination_schema_migrations` version table, following
  the per-package `go:embed` idiom used by `processor`, `lifecycle`, `batch`, and
  `session`.
- Not touched by this lane: `internal/integration/delivery/store.go` (4.2a),
  `internal/api/graphql/schema.graphql` and every regenerated artifact (S3-C1),
  `scripts/smoke-test.sh`, `scripts/check-runtime-config.sh` assertions,
  `deploy/**`, and the `runServe` component table (S3-A).
- Day-1 gate: `TestDeliveryDispatch_ContactsNoDestination` **passed against
  unmodified main @ `7111cca1`**, confirming correction 13. A live loopback TLS
  endpoint standing where a webhook destination would be reached recorded zero
  accepted connections and zero served requests across one complete production
  submission, while Kafka received exactly one command on the constant topic
  `integration.delivery.v1`. No durable record or broker payload carried a
  scheme, host, or port, and a URL-named destination was rejected at planning
  with `ErrWorkflowPlanningFailed` before any durable row existed. The test dials
  the endpoint itself at the end so the zeros are proven to be facts about the
  engine rather than about a broken listener.
- Consequence: 4.1c stays split. Sprint 3 ships 4.1c-a (contract + decision);
  the HTTPS consumer is 4.1c-b. No correction to `.loom/31` was required.
