# 23 — IDE + Backend Functionality: Gap-Fill Program

**Status**: Planning (created 2026-06-18)
**Owner**: platform
**Predecessor**: UX-redesign initiative (`.loom/21`, `.loom/22`) — COMPLETE. That program was cosmetic
(toast budget, inline errors, disabled-control honesty). This one is **functional**: wire capabilities
that exist but aren't reachable, make misleading surfaces honest, and close end-to-end loops.
**Mode**: Sequenced 3-wave program (operator-selected 2026-06-18).

---

## Riskiest assumption + kill-test

**Load-bearing assumption**: The fi-fhir backend's LLM features (`explainWarnings`, `generateWorkflow`,
`explainWorkflow`, `extractEntities`, `analyzeQuality`, `classifyMessage`) return *real, useful* output
end-to-end when `FI_FHIR_LLM_ENABLED=true` and an OpenAI-compatible provider is reachable — i.e. the only
thing standing between the UI Copilot and real AI is the UI's own `simulateStream()`, not a missing or
broken backend path.

**Why this is the bet**: Wave 2 (the highest-visibility win — "make the Copilot real") is a *wiring*
slice, not a *build*. Evidence the pipe already exists end-to-end except the last hop:
- UI operations are defined + codegen'd: `ui/src/lib/graphql/llm.graphql` (ExtractEntities, AnalyzeQuality,
  GenerateWorkflow, ExplainWorkflow, ClassifyMessage), `ui/src/lib/graphql/explainWarnings.graphql`,
  typed in `ui/src/lib/gen/graphql.ts:2234-2259` and `:899` (MutationGenerateWorkflowArgs).
- Backend resolvers delegate to real LLM modules with graceful-degrade (return placeholder if the
  module is nil): `internal/api/graphql/resolvers/schema.resolvers.go` (generateWorkflow → `r.WorkflowCopilot.Generate()`,
  explainWarnings → `explainWarningsBatch()`, etc.).
- Go LLM modules are Production-tested: `internal/llm/explain` (99.7%), `pkg/llm/copilot` (98%),
  `internal/llm/extract` (84.3%), `internal/llm/quality` (93.8%).
- The UI Copilot bypasses all of it: `ui/src/lib/features/copilot/copilotStore.ts:91-164` (`SIMULATED_RESPONSES`)
  + `:166-237` (`simulateStream`); `sendAction()` calls the simulator directly (`:265-322`).
- Default provider is in-cluster litellm: `pkg/config/config.go:309` →
  `http://litellm.ai.svc.cluster.local:8000/v1`, overridable via `FI_FHIR_LLM_BASE_URL`
  (`pkg/config/config.go:468`), gated by `FI_FHIR_LLM_ENABLED` (`:467`, default false).

**Kill test** (≤30 min, run BEFORE committing Wave-2 UI wiring):
1. Start the backend locally with LLM enabled against a reachable OpenAI-compatible endpoint
   (flexinfer proxy or litellm). Minimal:
   ```bash
   FI_FHIR_LLM_ENABLED=true \
   FI_FHIR_LLM_BASE_URL=<reachable-openai-compatible-base-url>/v1 \
   FI_FHIR_LLM_API_KEY=<key> \
   go run ./cmd/fi-fhir serve --addr :8080
   ```
2. POST a GraphQL `generateWorkflow` mutation (and an `explainWorkflow` query) with a real description, e.g.:
   ```bash
   curl -s localhost:8080/graphql -H 'content-type: application/json' -d '{
     "query":"mutation($i:GenerateWorkflowInput!){generateWorkflow(input:$i){yaml explanation model}}",
     "variables":{"i":{"description":"route ADT A01 admits to the FHIR server and alert on missing MRN"}}
   }'
   ```
3. **Observable pass criterion**: response contains non-placeholder `yaml` that varies with the prompt
   AND a non-empty `model` field naming the actual model used. Re-run with a different description and
   confirm the output changes (proves it's not canned).

**Pair with disconfirming search** (per workspace rule): also confirm the *negative* —
verify what the resolver returns when `WorkflowCopilot` is nil (the graceful-degrade placeholder string),
so we can tell "LLM off" apart from "LLM on but unreachable" in the UI. Both must be distinguishable.

**Failure mode if wrong**: If the provider is unreachable in the target deploy (or the resolver path is
broken), wiring the UI Copilot ships a feature that looks identical to the simulator — or worse, errors on
every keystroke. We'd be building UI polish on a dead backend path. If the kill-test FAILS, Wave 2
re-scopes to "graceful LLM-unavailable UX" (honest disabled state + provider-config surfacing) and the
copilot-wiring work parks until a provider is provisioned.

**Status**: not run — run as Slice 0 of Wave 2.

---

## Gap inventory (evidence-backed)

Two parallel audits (UI + backend, 2026-06-18). Headline: **backend is mature — 0 `panic("not implemented")`,
no stubbed business logic.** The functional holes are in the IDE (faked/unwired surfaces) and in a few
backend *reach* gaps (schema field with no resolver, CLI↔GraphQL parity, deferred Phase-5 items).

### Class A — Mock-masquerade (UI presents fake data as real) — TRUST defect

| Surface | Evidence | Defect | Verified? |
|---------|----------|--------|-----------|
| Observability | `ui/src/lib/features/observability/observabilityStore.ts` fetchers | Alerts (the only mounted observability surface — `AlertBadge` in StatusBar) silently fall back to realistic *fake* data when the platform MCP is disconnected — no "simulated" signal. Operator may act on fake firing alerts. | ✅ CONFIRMED → **FIXED (Slice 1)** |
| Debug panel | `ui/src/lib/features/debug/DebugPanel.svelte:71-75` (`loadMockData()` guarded by `useMockData && import.meta.env.DEV`) | Audit claimed "fake session on load" — **FALSE in production.** Mock only loads when `useMockData={true}` (prop defaults `false`; zero `useMockData={true}` mounts in-app; only mount is `IDEShell.svelte:450 <DebugPanel/>`). Prod already shows the honest Debug Event Input + empty state. | ❌ NOT A BUG (audit overstated) |
| Collaboration | `ui/src/lib/features/collaboration/collaborationStore.ts:5-6` | Entirely in-memory mock — BUT **not mounted anywhere** (no refs to `PresenceBar`/`TaskPanel`/`collaborationStore` outside its own dir). Users never see it; no masquerade until it's wired in. | ⚠️ MOOT (not user-visible) |

> **Verification note (2026-06-18)**: the UI audit's Class-A claims were re-checked at the mount level
> before acting (riskiest-assumption rule applied per-slice). Only **observability** is a genuine,
> user-visible trust defect. Debug is already honest in prod; collaboration is dead/unmounted. This is
> exactly the failure mode the kill-test discipline guards against — a green test (`DebugPanel.test.ts`
> "should load mock data on mount") exercised a non-production prop value (`useMockData: true`).

### Class B — Unwired capability (backend exists, UI fakes it) — REACH gap

| Surface | Evidence | Defect |
|---------|----------|--------|
| Copilot assistant | `copilotStore.ts:91-237`, `sendAction()` `:265-322` | Explain/suggest/generate/review return canned text; ignores the codegen'd `llm.graphql` ops + live resolvers. |
| Workflow generate | `ui/src/lib/features/workflows/components/GenerateFromDescription.svelte` | "Generate from description" routes through the stubbed copilot, not `GenerateWorkflow` mutation. |
| Workflow explain | `ui/src/lib/features/workflows/workflowApi.ts` (ExplainWorkflow defined, leaks to copilot) | Explanation not wired to the real `explainWorkflow` query. |

### Class C — Backend capability/reach fills

| Gap | Evidence | Effort | Blocks UI? |
|-----|----------|--------|-----------|
| `MappingStats` query missing resolver | schema field present; no resolver; `docs/planning/TERMINOLOGY-MAPPING.md:1540-1545` | M | Yes — blocks mapping-stats dashboard |
| Pending-autoroute auto-expiry | `expiresAt` column exists; no cleanup task | S | Yes — review queue pollutes |
| Pending-autoroute notifications (webhook/Slack) | deferred Phase-5 (`TERMINOLOGY-MAPPING.md:1493-1494`) | M | Partial — users must poll |
| OTel span emission for mapping decisions | stored as JSONB, not emitted | M | No |
| CLI↔GraphQL parity | ETL/eventstore/terminology-load = CLI only; workflow lifecycle/FHIR-subscription/profile-CRUD = GraphQL only | M | No (ops/CI ergonomics) |
| `classifyMessage` is rule-based only | `schema.resolvers.go` classify path | M | No |

### Already solid (do NOT touch — wired + tested)
HL7 preview/parse/submit, Source Profile editor + YAML revisions, Terminology mapping CRUD + CSV upload +
pending-review approve/reject/bulk, Workflow CRUD + dry-run + approvals, Events browse/detail/timeline/stream,
Temporal workflow list/signal, System health. (UI audit ranked all 🟢.)

---

## Sequenced program

Principle: **honesty before capability** — stop lying first (cheap, removes risk of acting on fake data),
then wire what already exists, then build new reach. Each slice ships independently (own MR, auto-merge,
green CI) following the workspace auto-ship policy. Each carries a kill/done criterion.

### Wave 1 — Honesty pass (make the IDE tell the truth)

Removes mock-masquerade. Low effort, no backend dependency, immediately de-risks operator trust.
**Re-scoped after mount-level verification (2026-06-18): only observability was a real, user-visible defect.**

- **Slice 1 — Observability: "Demo data" indicator on simulated alerts. ✅ SHIPPED (2026-06-18, branch `feat/funcgap-w1-observability-honesty`).**
  Added `isSimulated` store to `observabilityStore.ts` (set false on a real backend result, true on a mock
  fallback, in all three fetchers). `AlertBadge` (the only mounted observability surface) now shows a
  "Demo data" pill in its dropdown header and annotates the badge `title`/`aria-label` when `$isSimulated`,
  so demo data is perceivable without opening. Mock fallback retained intentionally (demo value) — the
  defect was that it was *unlabeled*, not that it existed (matches the "mock-with-badge" decision below).
  *Done*: 6 new tests (3 AlertBadge + 3 store), 533 vitest pass (527→533), lint clean, typecheck clean
  (1 pre-existing vite/rollup dep `.d.ts` error only).
  *Deferred within scope*: `MetricsPanel`/`LogViewer` are not mounted anywhere — when they are surfaced,
  drive the same `isSimulated` flag into them (1-line each). Logged here so it isn't silently dropped.

- **~~Slice 1a — Debug panel~~**: WITHDRAWN — already honest in production (see Class-A verification note).

- **~~Slice 1c — Collaboration~~**: DEFERRED — feature is not mounted, so there's no live masquerade.
  Revisit only if/when collaboration is wired into the IDE shell; at that point label-as-preview (default).

### Wave 2 — Make the Copilot real (wire UI → existing backend LLM)

Gated by the **kill-test above (Slice 0)**. Only proceeds if the LLM path is proven live.

- **Slice 0 — LLM reachability kill-test. ✅ PASSED (2026-06-18) → Wave 2 UNBLOCKED.**
  Operator authorized `flexinfer-proxy` + gemma4/qwen3 models. Procedure run:
  `kubectl port-forward -n flexinfer-system svc/flexinfer-proxy 8000:80` (k3s kubeconfig), then local
  `fi-fhir serve` with `LLM_BASE_URL=http://localhost:8000/v1`, `LLM_QUALITY_MODEL=gemma4-26b-a4b-gptq`,
  `LLM_DEFAULT_MODEL=gemma4-e4b-radeonvii`, `FI_FHIR_LLM_ENABLED=true`. Results:
  - `generateWorkflow` returned **real, prompt-specific** YAML for two distinct prompts (ADT→FHIR + missing-MRN
    warn route; ORU→Kafka + critical-result email) — the disconfirming "varies with prompt" check passed.
  - `explainWorkflow` returned a real business-audience summary (same client → same proxy).
  - Negative check confirmed: with the wrong base URL the resolver errors against the default
    `litellm.ai.svc.cluster.local` — i.e. "off/misconfigured" is distinguishable from "live".
  - Working models on the proxy: `gemma4-26b-a4b-gptq`, `gemma4-e4b-radeonvii`, `qwen3-1p7b-tools-radeonvii`.
    Broken/unactivatable: `qwen3-8b-radeonvii` (RuntimeFailed). Avoid the latter in config.

  **⚠ Config-namespace gap found (record before 2a deploy):** the serve GraphQL LLM client reads the
  `pkg/llm` env namespace — `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_DEFAULT_MODEL`, `LLM_QUALITY_MODEL`
  (`cmd/fi-fhir/main.go:4724` → `llm.DefaultConfig().WithEnv()`, `pkg/llm/config.go:100`). But *enablement*
  reads `FI_FHIR_LLM_ENABLED` (`pkg/config`). So the `FI_FHIR_LLM_BASE_URL`/`_MODEL` keys documented in
  `docs/planning/README.md` and this spec's kill-test draft DO NOT configure the copilot/explainers — only
  the `LLM_*` keys do. Deploys must set `LLM_BASE_URL` (or unify the two namespaces). Candidate Wave-3
  cleanup slice: collapse `pkg/llm` env loading onto the `FI_FHIR_LLM_*` namespace, or document both.
- **Slice 2a — Replace `simulateStream()` with real GraphQL. ✅ SHIPPED (2026-06-18, branch `feat/funcgap-w2-copilot-real`).**
  `copilotStore.sendAction()` now dispatches real codegen'd ops via a new pure, unit-tested
  `copilotDispatch.ts` (no simulator left). Operator-selected mapping "wire all 4 best-effort":
  generate→`GenerateWorkflow{description}`, explain→`ExplainWorkflow{workflowYaml}`,
  suggest→`SuggestMappings{…}` (best-effort free-text coercion: parses a trailing `… to <system>`
  clause, context-metadata hints, LOINC default), review→`AnalyzeQuality{event,eventType}` (JSON input
  passed through, else wrapped `{raw}`; eventType from a validated context hint else `DOCUMENT`).
  **Mapping reality vs. the plan sketch**: only `AnalyzeQuality`/`ExtractEntities` carry a `model` field in
  the schema — `GeneratedWorkflow`/`WorkflowExplanation` do not — so per the operator decision the model
  badge surfaces *only* for `review` (the others render `model: null`, no fabrication). The streaming UX
  shell (placeholder + spinner + cancel) is preserved as an honest in-flight indicator (single round-trip;
  no fake token typewriter re-added). B4 error path defers to the global toast net via `isErrorToasted`
  (`.loom/22 §5i`); cancel discards an in-flight result.
  *Done*: 22 new vitest (`copilotDispatch.test.ts` builders/formatters/dispatch + `copilotStore.test.ts`
  dispatch/model/error/cancel), 555 pass (533→555), lint clean, typecheck clean (1 pre-existing
  vite/rollup `.d.ts` error only). `model` chip added to `CopilotPanel`.
  *Deferred within scope*: no GraphQL capability field exposes "LLM enabled", so the UI can only
  distinguish *unreachable* (op errors → inline + net toast) from *connected*; a true "LLM off" badge needs
  a backend capability query (candidate Wave-3, alongside the `LLM_*`/`FI_FHIR_LLM_*` namespace collapse).
- **Slice 2b — Workflow generate/explain wired through.**
  `GenerateFromDescription.svelte` → `GenerateWorkflow` mutation; explain → `explainWorkflow` query.
  Generated YAML lands in the WorkflowBuilder draft (not a toast).
  *Done*: "Generate from description" produces a real, prompt-specific draft; round-trips into the builder.

### Wave 3 — Backend capability fills (close end-to-end loops)

Pulled forward because Wave 2 is blocked on an LLM provider (no backend dependency here).

- **Slice 3b — Pending-autoroute expiry honesty. ✅ SHIPPED (2026-06-18, branch `feat/funcgap-w3-pending-autoroute-expiry`).**
  Verification corrected the plan: the DB transition `ExpirePendingAutoroutes` (SQL `status='pending' AND
  expires_at < NOW()` → expired) already existed at `mappings.go:1009` — but **nothing called it**, and
  `ListPendingAutoroutes`/`CountPendingAutoroutes` filtered only on the literal `status` column, so a
  time-expired-but-unswept row still appeared in the review queue. Fix (query-time, sweep-independent):
  `ListPendingAutoroutes` + count now exclude `status='pending' AND expires_at < NOW()` (count remaps
  those to `expired` via CASE), so expired suggestions never surface regardless of sweep timing.
  **Bonus latent-bug fix**: `CreatePendingAutoroute` passed `nullJSON(DecisionTrace)` = NULL into the
  `decision_trace JSONB NOT NULL` column → insert failure whenever a caller omits the trace; now defaults
  to `'{}'` (`jsonObjectOrEmpty`). *Done*: 1 new integration test; verified via local testcontainers —
  branch FAIL set ⊊ clean-main FAIL set (no new failures) and the `decision_trace` fix **repaired 9
  previously-red** pending-autoroute integration tests. gofmt+vet clean.
  *Deferred within scope*: the background **sweep caller** (periodic `ExpirePendingAutoroutes` in the serve
  path, for DB hygiene/analytics + status-column truth) — next slice; query-time exclusion already makes
  the UI honest, so it's not urgent.
  *Pre-existing rot found (out of scope, flagged separately)*: 7 `TestMappingStore_*` integration tests
  red on main in unrelated areas (`custom_mappings` create/lookup, `mapping_decisions` telemetry, a
  separate `ApprovePendingAutoroute` path bug). These integration tests aren't in CI's path
  (`test:integration` runs `./cmd/fi-fhir/...` only), so they rotted unnoticed.

- **~~Slice 3a — `MappingStats` query + dashboard~~**: RE-SCOPED. Audit claim ("schema field with no
  resolver") is **false** — there is no `MappingStats` field in `schema.graphql`; `pendingAutorouteStats`
  already exists, is implemented, and is UI-consumed. A broader mapping-stats query would be a net-new
  feature *build* (not a wiring fix). Park until a concrete dashboard need pulls it.
- **Slice 3c — Pending-autoroute notifications.**
  Webhook/Slack dispatch when a high-confidence autoroute lands in review. (M effort.)
- **Slice 3d (stretch) — CLI↔GraphQL parity + OTel mapping spans.**
  Pick per real need: workflow-lifecycle CLI for CI automation, or OTel emission for the decision telemetry
  dashboards. Defer unless pulled by a concrete consumer.

---

## Test & rollout strategy

- **Per slice**: unit tests (vitest for UI, `go test` for backend) — workspace policy >80% on new code,
  regression test for any fix. Run `devbox_quality_gate` before commit.
- **UI**: maintain the green vitest baseline (527 passing at program start, `.loom/22 §5i`). New copilot
  wiring needs tests that mock the GraphQL client and assert real-op dispatch (not simulator).
- **Backend**: `MappingStats` resolver needs table-driven tests; auto-expiry needs a time-injected test.
- **Wave 2 gate**: do not merge any copilot-wiring MR until the Slice-0 kill-test is recorded as passed.
- **CI gotchas** (from memory / `.loom/22`): `lint:gqlgen` cold-GOMODCACHE run takes 16-24 min and looks
  hung but isn't — don't cancel-retry; it blocks MWPS (not allow_failure). `test:ui` silently skips vitest
  on UI-only MRs. `lint:go` used to compile golangci-lint from source and timed out on a cold branch cache;
  as of 2026-06-18 it uses the pinned `golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine` image,
  2 CPU / 4 GiB runner limits, and a 30-minute lint timeout for cold-cache package loading.
  For MR create/merge against the public host, use curl with `--resolve gitlab.flexinfer.ai:443:192.168.50.227`
  + `$GITLAB_PAT` (MCP gitlab write POSTs 403 via Cloudflare).

## Open questions (decisions needed)

- [ ] **Collaboration (Slice 1c)**: feature-flag-hide vs. label-as-preview? (Default: label-as-preview —
  cheaper, keeps the surface visible for the eventual agent-context MCP wiring.)
- [ ] **Observability (Slice 1b)**: honest-empty vs. mock-with-badge when disconnected? (Default:
  mock-with-badge for demos, but badge must be unmissable.)
- [ ] **LLM provider for kill-test**: flexinfer proxy or in-cluster litellm? Needs a reachable endpoint +
  key in the dev/test environment.

## Risks

- [ ] Kill-test depends on an external LLM provider being reachable from the test environment — the single
  biggest schedule risk (Wave 2 blocks on it).
- [ ] Codebase-memory index was reported empty (`total_chunks: 0`, `00-index.md` risks) — semantic search
  assist is degraded; rely on text search + these audits.
- [ ] Collaboration wiring (real MCP agent-context) is explicitly OUT of scope here (Wave 1 only makes it
  honest); a future program owns the real integration.

## See also
- `.loom/21-ux-professional-redesign.md`, `.loom/22-toast-budget-policy.md` (predecessor cosmetic program)
- `.loom/30-implementation-plan.md` (earlier IDE-connectivity slice sketch — partially superseded)
- `docs/planning/TERMINOLOGY-MAPPING.md:1529-1545` (Phase-5 deferrals → Wave 3)
- `docs/STATUS.md` (component maturity matrix)
