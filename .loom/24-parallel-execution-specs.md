# 24 - Parallel Execution Specs

**Status**: Ready for agent pickup (created 2026-06-19)
**Owner**: platform
**Inputs**: latest Loom planning/brainstorm artifacts: `.loom/20-product-spec.md`, `.loom/21-ux-professional-redesign.md`, `.loom/22-toast-budget-policy.md`, `.loom/23-functionality-gaps-plan.md`, `.loom/30-implementation-plan.md`

## Goal

Turn the latest brainstorm/planning material into execution-ready specs that can be assigned to parallel agents without duplicating work, re-opening shipped slices, or turning verification items into unnecessary rebuilds.

## Non-Goals

- Do not re-run the completed UX redesign/toast budget program; `.loom/21-ux-professional-redesign.md` and `.loom/22-toast-budget-policy.md` record it as shipped.
- Do not implement these specs in this planning slice.
- Do not change GitLab issues from this doc alone; issue updates should happen when an execution lane starts.
- Do not create sibling worktrees outside the owning repo; follow `AGENTS.md` worktree policy.

## Current-State Corrections From Code

These corrections matter before parallelization:

1. **Workflow generate/explain is already wired in current code.**
   - `GenerateFromDescription.svelte` imports `generateWorkflow`, calls it, stores `yaml`/`explanation`/`warnings`, and loads YAML into `workflowDraft`: `ui/src/lib/features/workflows/components/GenerateFromDescription.svelte:6`, `:24-43`.
   - `WorkflowPreview.svelte` imports `explainWorkflow`, calls it, and renders route explanations: `ui/src/lib/features/workflows/components/WorkflowPreview.svelte:6`, `:17-29`.
   - The GraphQL operations exist in `ui/src/lib/graphql/workflows.graphql:267-288`.
   - Therefore the old Wave 2b should now be a **verification + UX hardening** lane, not a blind wiring lane.
2. **LLM config naming is still a real execution risk.**
   - Serve initializes LLM features with `llm.DefaultConfig().WithEnv()`: `cmd/fi-fhir/main.go:4723-4725`.
   - `pkg/llm.Config.WithEnv()` reads `LLM_*` variables: `pkg/llm/config.go:91-120`.
   - App config reads `FI_FHIR_LLM_*` variables: `pkg/config/config.go:466-473`.
   - The functionality plan found the same split during the LLM reachability kill-test: `.loom/23-functionality-gaps-plan.md:160-166`.
3. **Pending autoroute expiry has query-time honesty but still lacks a background sweep caller.**
   - `ListPendingAutoroutes` excludes time-expired pending rows at query time: `pkg/terminology/db/mappings.go:747-763`.
   - `ExpirePendingAutoroutes` exists but is just a store method: `pkg/terminology/db/mappings.go:1025-1037`.
   - `.loom/23-functionality-gaps-plan.md:195-209` records the shipped query-time fix and explicitly defers the background sweep caller.
4. **Terminology DB integration tests are the fragile prerequisite for more terminology-store work.**
   - The integration test helper supports `POSTGRES_TEST_URL`: `pkg/terminology/db/migrations_integration_test.go:35-63`.
   - CI `test:integration` already provisions Postgres but only runs `./cmd/fi-fhir/...`: `.gitlab-ci.yml:469-506`.
   - The project AGENTS notes explicitly say `pkg/terminology/db/...` integration tests are not exercised by CI and can rot.
5. **Contract lint is no longer a soft M0 blocker, but broader integration CI still is.**
   - `lint:contracts` has no `allow_failure` and comments say it was promoted to blocking: `.gitlab-ci.yml:333-366`.
   - `test:integration` still has `allow_failure: true`: `.gitlab-ci.yml:469-519`.

## Parallelization Map

| Lane | Can Start Now? | Parallel With | Must Coordinate With | Primary Risk |
|---|---:|---|---|---|
| A. Workflow AI verification + polish | Yes | B, D, E | B if adding capability UI | Shipping tests for behavior already implemented |
| B. LLM config namespace + capability surface | Yes | A, D, E | A if UI consumes capability query | Breaking existing `LLM_*` users |
| C. Pending autoroute sweep + notifications | After D triage, or carefully isolated | A, B, E | D because same DB package | Adding automation on top of red integration tests |
| D. Terminology DB integration recovery + CI path | Yes | A, B | C, E | CI/schema isolation and existing red tests |
| E. Integration/contract CI hardening | Yes, but split narrowly | A, B | D for terminology DB inclusion | Turning soft jobs blocking before stable |
| F. Product expansion speclets (CDA, storage, FHIR IG) | Yes, spec-first | A-E | None unless implementation begins | Too broad for one agent |

Recommended first wave: A, B, D, and F can run in parallel. C should wait for D's initial red/green audit unless its agent touches only serve-time scheduling and notification interfaces. E should avoid changing the terminology DB CI path until D has a clean package-level story.

## Lane A - Workflow AI Verification + Polish

**Branch suggestion**: `codex/workflow-ai-verify`

### Goal

Prove the current Workflow Builder "Generate from Description" and "Explain with AI" path is truly live, covered, and honest when LLM features are enabled or unavailable.

### Non-Goals

- Do not reintroduce simulated Copilot text.
- Do not add a new LLM provider abstraction.
- Do not change GraphQL schema unless the capability-query work from Lane B has landed.

### Tasks

1. Add or refresh UI tests for `GenerateFromDescription.svelte`:
   - Calls `generateWorkflow(...)` with the current description, event types, and action types.
   - Renders generated YAML, warnings, and explanation.
   - `Load into Builder` calls `workflowDraft.loadDraft` only after YAML parses.
   - GraphQL errors follow the `isErrorToasted` dedupe contract.
2. Add or refresh UI tests for `WorkflowPreview.svelte`:
   - Calls `explainWorkflow(yamlOutput, "business")`.
   - Renders top-level description and route explanation rows.
   - Handles failures without double-toasting.
3. Run a local GraphQL smoke with an LLM-enabled backend if credentials are available; otherwise record the unavailable-provider result as an explicit limitation.
4. Update `.loom/23-functionality-gaps-plan.md` to mark Wave 2b as verified or to replace it with the exact remaining defect.

### Acceptance Criteria

- Tests prove current code uses `GenerateWorkflow` and `ExplainWorkflow`, not the old Copilot simulator.
- Generated YAML can be parsed into the builder draft in a test.
- Failure UI is honest: no fake output, no duplicate toast.
- `.loom/23-functionality-gaps-plan.md` no longer lists Wave 2b as a generic unwired item if verification passes.

### Kill-Test

Before broad UI polish, run one targeted test that mocks `generateWorkflow` with invalid YAML. The component must show a parse failure only when `Load into Builder` is clicked, not silently mutate the draft.

### Verification

- `cd ui && npm test -- --run src/lib/features/workflows`
- `cd ui && npm run typecheck`

## Lane B - LLM Config Namespace + Capability Surface

**Branch suggestion**: `codex/llm-config-capability`

### Goal

Make LLM runtime configuration unambiguous for operators and give UI/GraphQL callers a truthful capability surface for "enabled, configured, unavailable, degraded".

### Non-Goals

- Do not remove legacy `LLM_*` variables in the first slice.
- Do not require an external provider for unit tests.
- Do not expose API keys or provider secrets through GraphQL.

### Tasks

1. Decide precedence and document it:
   - Recommended: `FI_FHIR_LLM_*` is canonical for app/runtime config; `LLM_*` remains backward-compatible fallback for package-level clients.
   - Preserve `OPENAI_API_KEY` as API-key fallback where already supported.
2. Update `pkg/llm.Config.WithEnv()` to read `FI_FHIR_LLM_BASE_URL`, `FI_FHIR_LLM_API_KEY`, `FI_FHIR_LLM_DEFAULT_MODEL`, and `FI_FHIR_LLM_QUALITY_MODEL` before legacy `LLM_*`.
3. Add unit tests in `pkg/llm/config_test.go` covering canonical envs, fallback envs, and precedence.
4. Add a small GraphQL capability query if needed by UI:
   - Minimum fields: `enabled`, `configured`, `providerBaseURLHost`, `defaultModel`, `qualityModel`, `status`, `warnings`.
   - Never return API keys or full secret-bearing URLs.
5. Wire serve-time resolver state from actual initialization, not from env guessing alone.
6. Update `docs/planning/README.md` and `docs/user-guide/llm-features.md` so operators see both canonical and legacy variable names.

### Acceptance Criteria

- Both `FI_FHIR_LLM_*` and existing `LLM_*` configurations work, with documented precedence.
- Serve-time warnings clearly distinguish "disabled", "misconfigured", and "provider unreachable" where practical.
- UI can display a truthful disabled/degraded state without probing by failing user actions if the capability query is added.
- Existing LLM command tests still pass.

### Kill-Test

Run a unit test where both `FI_FHIR_LLM_BASE_URL` and `LLM_BASE_URL` are set to different values. The resulting config must use the documented canonical value.

### Verification

- `go test ./pkg/llm ./cmd/fi-fhir ./internal/api/graphql/...`
- If schema changes: regenerate GraphQL artifacts and run the repo's GraphQL/codegen checks.

## Lane C - Pending Autoroute Sweep + Notifications

**Status**: **COMPLETE** — split into C1 and C2, both shipped.
- **C1 — expiry sweep: SHIPPED (2026-08-03, branch `feat/autoroute-expiry-sweep`)**.
  Covers tasks 1-2 and the Lane C kill-test.
- **C2 — notifications: SHIPPED (2026-08-08, branch `feat/autoroute-notifications`)**.
  Covers tasks 3-5.

### C1 as shipped

`internal/terminology/autoroute/sweeper.go` adds a `Sweeper` with
`SweepOnce(ctx)` / `Run(ctx)`, wired into `serve` as a background component
under the existing `serveCtx` / `errCh` / `waitForBackgroundStops` boundary, so
it cancels with the server like the MLLP, delivery, and batch runners.

Decisions worth preserving:
- The sweeper depends on a narrow `PendingAutorouteExpirer` interface, not on
  `*db.MappingStore`. Scheduling stays out of the DB package (the Lane C
  non-goal, generalized) and sweep logic is unit-testable without Postgres.
- Cadence is `FI_FHIR_TERMINOLOGY_AUTOROUTE_SWEEP_INTERVAL`, default `15m`,
  `0` disables. Config owns the default; the constructor rejects a
  non-positive interval rather than silently no-opping.
- `Run` sweeps immediately on boot (reconciling rows that expired while the
  process was down), then on each tick. A failing iteration is reported and the
  loop continues — a database blip must not take down serve. Cancellation
  returns `nil` so normal shutdown is not reported as a component failure.
- Observability is a typed `SweepResult` plus an `Observe` hook that serve
  prints. There is no serve-wide Prometheus registry to hook today: the only
  Prometheus code is `internal/workflow/metrics_prometheus.go`, whose interface
  is events/actions/DLQ shaped. The hook keeps a metrics adapter cheap later.
- The kill-test lives in the **external** `db_test` package at
  `pkg/terminology/db/sweeper_integration_test.go`, because
  `internal/terminology/autoroute` transitively imports `pkg/terminology/db`
  and an in-package test would be an import cycle. It needs no
  `.gitlab-ci.yml` change: CI already runs `./pkg/terminology/db/`.

C1 limitation to carry into Lane E: `test:integration` is still
`allow_failure: true`, so the kill-test runs in CI but does not block.

### C2 as shipped

`internal/terminology/autoroute/notify.go` adds a `ReviewNotifier` with
`ScanOnce(ctx)` / `Run(ctx)` and a `WebhookSink`, wired into `serve` as a
background component beside the C1 sweeper, under the same `serveCtx` / `errCh`
/ `waitForBackgroundStops` boundary.

Decisions worth preserving:
- **Periodic digest, not a per-event hook in the creation path.** Nothing on the
  resolution or `CreatePendingAutoroute` path calls the notifier, so "a failing
  notification cannot affect resolution" is structural rather than a discipline
  every call site has to maintain. A digest is also the more useful signal (the
  current queue, with `eligible_count` for depth alerting) and it avoids firing
  on every re-resolution of the same code, since creation upserts on the natural
  key. Rationale and the rejected alternative are recorded in
  `.loom/iteration-plan-lane-c2-autoroute-notifications.md`.
- The per-event door is left open and is not dead code: `Notify` is the
  non-blocking bounded-queue entry point, and the scan loop itself calls it. A
  future per-event hook calls the same method and inherits drop-rather-than-block.
- Same narrow-interface discipline as C1: the notifier depends on
  `PendingAutorouteLister`, not `*db.MappingStore`. Notification policy stays out
  of the DB package and is unit-testable without Postgres.
- Config mirrors C1's shape: `FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_WEBHOOK`
  (empty disables — no component, no network), plus `_INTERVAL` (15m),
  `_MIN_CONFIDENCE` (0.90), `_TIMEOUT` (5s). The YAML key is
  `terminology.autoroute_notify.notification_webhook`, matching the planning key
  at `docs/planning/TERMINOLOGY-MAPPING.md`. Validation only runs when a webhook
  is set, so a disabled feature cannot fail startup.
- The payload is PHI-minimal by construction: coded identity, confidence, and
  lifecycle timestamps only. `source_display`, `suggested_display`, `reasoning`,
  `decision_trace`, `alternates`, `reviewed_by`, and `rejection_reason` are all
  excluded because they can quote source message content, and a webhook is an
  untrusted egress point. Both a unit test and the integration kill-test assert
  poisoned free text never reaches the wire.
- De-duplication is by pending row ID in a bounded (1024) FIFO set, so a quiet
  queue produces no traffic. The first scan announces the current backlog, which
  mirrors C1's boot-time sweep.
- Dispatch drops rather than grows: an 8-deep queue, and a full queue logs a
  warning and discards. Each digest restates the backlog, so a drop loses nothing
  durable. Delivery is one attempt plus one bounded retry — notifications are
  advisory and a determined retry loop would be a second failure domain.
- Observability follows C1: typed `NotifyResult` / `DeliveryResult` plus
  `Observe` / `ObserveDelivery` hooks that serve prints. Still no serve-wide
  Prometheus registry; still not invented here.
- The kill-test lives in the external `db_test` package at
  `pkg/terminology/db/notify_integration_test.go`, for the same import-cycle
  reason as C1, and needs no `.gitlab-ci.yml` change.

C2 inherits C1's Lane E limitation: `test:integration` is still
`allow_failure: true`.

### Goal

Finish the deferred operational loop for pending autoroutes: keep the status column truthful over time and notify humans when high-confidence mappings need review.

### Non-Goals

- Do not rebuild the approval UI; it already consumes pending review stats and list APIs.
- Do not add Slack-only logic directly into the DB package.
- Do not auto-approve mappings in this lane.

### Tasks

1. **[C1 done]** Add a serve-time background sweep that periodically calls `MappingStore.ExpirePendingAutoroutes(ctx)`.
   - Scope it to serve/runtime initialization where `mappingStore` exists.
   - Use context cancellation on server shutdown.
   - Make interval configurable, with a conservative default.
2. **[C1 done, logging only]** Add structured logging/metrics for sweep count and failures.
   Metrics deferred: no shared serve-wide registry exists yet (see decisions above).
3. **[C2 done]** Design a notification interface around "new pending autoroute created" or "high-confidence pending review".
   - Shipped: "high-confidence pending review", as a periodic digest. Webhook-only,
     via a small HTTP client behind a `NotificationSink` interface.
   - Config aligns with the planning key `notification_webhook`.
4. **[C2 done]** Ensure notification dispatch cannot block creation of a pending autoroute.
   Guaranteed structurally (nothing on the creation path notifies) and by a
   bounded, drop-on-full dispatch queue behind `Notify`.
5. **[C1 done for sweep; C2 done for notifications]** Add tests for sweep invocation and notification error handling.

### Acceptance Criteria

- Expired pending rows eventually transition to `expired` without relying only on query-time filtering.
- Notification failures are logged/warned but do not fail the mapping resolution path.
- High-confidence thresholds and webhook URL are configurable.
- The review queue remains truthful if the sweep has not run yet, preserving the shipped query-time guard.

### Kill-Test

**C1**: Create an expired pending autoroute in an integration test, start only the sweep runner with a short interval, and assert status becomes `expired` without calling `ListPendingAutoroutes`. Shipped as `TestAutorouteExpirySweep_FlipsStoredStatus`.

**C2**: Create pending autoroutes above and below the confidence floor, run the notifier against a local HTTP receiver, and assert exactly the above-threshold rows are delivered with a PHI-minimal payload; then wedge the receiver and assert pending-autoroute creation keeps succeeding promptly. Shipped as `TestReviewNotifier_HighConfidenceRowsReachWebhook` and `TestReviewNotifier_HangingWebhookDoesNotSlowCreation` in `pkg/terminology/db/notify_integration_test.go`.

### Verification

- `go test ./pkg/terminology/db ./cmd/fi-fhir ./internal/terminology/...`
- Integration path after Lane D: `POSTGRES_TEST_URL=... go test -tags=integration ./pkg/terminology/db/`

## Lane D - Terminology DB Integration Recovery + CI Path

**Branch suggestion**: `codex/terminology-db-integration-ci`

### Goal

Make `pkg/terminology/db` integration tests reliable enough to protect the approval/autoroute store, then wire them into CI without schema collisions.

### Non-Goals

- Do not make unrelated loader fixture tests blocking until their data dependency is understood.
- Do not share a destructive schema between concurrent CI packages.
- Do not hide red tests with blanket skips.

### Tasks

1. Run and record the current package baseline:
   - `go test -tags=integration ./pkg/terminology/db/`
   - If Docker is unavailable, use the CI-compatible `POSTGRES_TEST_URL` path.
2. Split failures into:
   - Store logic regressions.
   - Fixture/loader dependency failures.
   - Schema isolation or test-order failures.
3. Fix store logic regressions first, especially pending autoroute approve/reject/count behavior noted in `.loom/23-functionality-gaps-plan.md:210-213`.
4. Add schema isolation for CI:
   - Either serialize destructive packages with `-p 1`, or give `cmd/fi-fhir` and `pkg/terminology/db` distinct databases/schemas.
   - The AGENTS note says both paths can `DROP SCHEMA terminology CASCADE`, so treat parallel sharing as unsafe.
5. Update `.gitlab-ci.yml` to run the clean subset/package using the existing Postgres service and `POSTGRES_TEST_URL`.
6. Update project docs with the new CI/local command.

### Acceptance Criteria

- `pkg/terminology/db` integration tests have a known green subset or a fully green package.
- CI runs that subset/package against the existing Postgres service.
- Any remaining loader fixture failures are explicitly excluded with issue-backed rationale, not silently skipped.
- Lane C has a stable store test base to build on.

### Kill-Test

Run the new CI command twice against the same Postgres service path. It must pass both times, proving cleanup/schema isolation is sufficient.

### Verification

- `POSTGRES_TEST_URL=postgres://testuser:testpass@localhost:5432/fi_fhir_test?sslmode=disable go test -tags=integration ./pkg/terminology/db/`
- CI lint for `.gitlab-ci.yml` if available.

## Lane E - Integration CI Hardening

**Branch suggestion**: `codex/integration-ci-hardening`

### Goal

Reduce CI blind spots without destabilizing the pipeline: integration tests should be meaningful, and soft-fail jobs should have explicit promotion criteria.

### Non-Goals

- Do not promote every existing `allow_failure` job in one MR.
- Do not change `lint:contracts`; it is already blocking according to current CI config.
- Do not require Docker-in-Docker.

### Tasks

1. Inventory remaining `allow_failure: true` jobs and classify them as:
   - Intentionally advisory.
   - Ready to promote.
   - Needs cleanup issue.
2. For `test:integration`, decide whether this lane only documents promotion criteria or also removes `allow_failure`.
3. Add smoke assertions for `/graphql`, `/graphql/ws`, `/health`, and `/ready` only if they are missing from current CI.
4. Coordinate with Lane D before adding terminology DB package tests.

### Acceptance Criteria

- CI soft-fail policy is documented in `.loom/40-decisions.md` or CI comments.
- Any promoted job has a recent green proof.
- `test:integration` no longer gives a false sense of coverage in docs.

### Kill-Test

Before promoting `test:integration`, run the exact CI script locally or in a branch pipeline and confirm the job is green without relying on `allow_failure`.

## Lane F - Product Expansion Speclets

**Branch suggestion**: `codex/product-speclets`

### Goal

Break `.loom/20-product-spec.md` and the P3 backlog into independent child specs, not broad implementation blobs.

### Non-Goals

- Do not implement new ingestion, FHIR IG, or SMART flows in this lane.
- Do not treat all P3 items as equal priority; each speclet should name its trigger/customer pull.

### Speclets To Produce

1. **CDA/CCDA section expansion**
   - Source: `.loom/20-product-spec.md:22-27`, `docs/planning/README.md:268-269`.
   - Output: `.loom/25-spec-cda-section-expansion.md`.
   - Acceptance focus: Medications, Allergies, Social History mapping to canonical events.
2. **Storage/provider integration tests**
   - Source: `docs/planning/README.md:280-281`.
   - Output: `.loom/26-spec-storage-provider-tests.md`.
   - Acceptance focus: S3/MinIO test harness, no production credential coupling.
3. **Terminology approval workflow hardening**
   - Source: `.loom/20-product-spec.md:28-33`, `docs/planning/README.md:274`.
   - Output: `.loom/27-spec-terminology-governance.md`.
   - Acceptance focus: review queue SLA, audit trail, notification policy; avoid duplicating Lane C implementation details.
4. **FHIR IG/Bulk/SMART scoping**
   - Source: `.loom/20-product-spec.md:34-39`, `docs/planning/README.md:275`.
   - Output: `.loom/28-spec-fhir-ig-bulk-smart.md`.
   - Acceptance focus: standards matrix and incremental compliance order.
5. **Dynamic Source Profile management**
   - Source: `.loom/20-product-spec.md:40-43`.
   - Output: `.loom/29-spec-profile-management-observability.md`.
   - Acceptance focus: profile CRUD, diff/publish safety, trace correlation.

### Acceptance Criteria

- Each speclet has Goal, Non-Goals, Acceptance Criteria, Kill-Test, Dependencies, and Sources.
- Each speclet can be assigned independently after review.
- Cross-spec dependencies are explicit.

## Coordination Rules For Parallel Agents

- Use separate branches/worktrees per lane.
- Lane agents must update only their lane section and the relevant implementation/worklog entries.
- If two lanes need the same file, the earlier lane records the ownership note in `.loom/50-worklog.md`.
- Any agent finding a false premise must correct the source planning doc before coding. The Workflow AI wiring correction above is the model: verify code first, then re-scope.
- Before MR/commit, each lane runs targeted tests and records exact commands in the relevant spec/worklog.

## Suggested Execution Order

1. ~~**Wave P1 (parallel)**: Lane A, Lane B, Lane D, Lane F.~~ **Done 2026-06-19.**
2. **Wave P2 (after D baseline)**: Lane C, Lane E CI promotion changes.
   - Lane C1 (expiry sweep) **done 2026-08-03**.
   - **Next: Lane C2** (notifications) and **Lane E**. Lane E now has stronger
     promotion evidence than when this doc was written: `test:integration`
     covers the terminology DB store (Lane D) and the sweep kill-test (C1), but
     is still `allow_failure: true`.
3. **Wave P3**: Child speclet implementation MRs selected from Lane F based on customer pull or issue priority.

## Sources

- `.loom/20-product-spec.md:7-13`, `.loom/20-product-spec.md:22-43`
- `.loom/21-ux-professional-redesign.md:231-248`, `.loom/21-ux-professional-redesign.md:250-267`
- `.loom/22-toast-budget-policy.md:131-144`, `.loom/22-toast-budget-policy.md:292-342`
- `.loom/23-functionality-gaps-plan.md:12-67`, `.loom/23-functionality-gaps-plan.md:143-223`, `.loom/23-functionality-gaps-plan.md:227-241`
- `.loom/30-implementation-plan.md:6-41`
- `ui/src/lib/features/workflows/components/GenerateFromDescription.svelte:6`, `ui/src/lib/features/workflows/components/GenerateFromDescription.svelte:24-43`
- `ui/src/lib/features/workflows/components/WorkflowPreview.svelte:6`, `ui/src/lib/features/workflows/components/WorkflowPreview.svelte:17-29`
- `ui/src/lib/graphql/workflows.graphql:267-288`
- `cmd/fi-fhir/main.go:4723-4725`
- `pkg/llm/config.go:91-120`
- `pkg/config/config.go:466-473`
- `pkg/terminology/db/mappings.go:747-763`, `pkg/terminology/db/mappings.go:1025-1037`
- `pkg/terminology/db/migrations_integration_test.go:35-63`
- `.gitlab-ci.yml:333-366`, `.gitlab-ci.yml:469-519`
- `docs/planning/README.md:241-281`
