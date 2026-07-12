# Research Brief

## Problem

Re-plan fi-fhir as a fully integrated platform slice from backend to frontend, then define practical integration seams to sibling services (`flexinfer`, `loom-core`, `mentatlab`) based on what actually shipped recently.

## Questions

- Q1: What did recent commits (since 2026-02-01) materially change?
- Q2: What backend↔frontend integration is already real vs still implied?
- Q3: Which sibling-service contracts are mature enough to integrate now?
- Q4: What operational constraints could block execution?

## Method

- Reviewed commit history and changed-path concentration.
- Inspected key commits tied to UI integration, persistent EventStore, terminology indexing, and contract governance.
- Inspected fi-fhir runtime endpoints, UI proxy/client wiring, workflow action/validation surfaces, env and compose defaults.
- Inspected sibling service docs in `~/workspace/services/{flexinfer,loom-core,mentatlab}` for concrete API/protocol contracts.
- Verified MCP/loom inventory and codebase indexing readiness.

## Findings

1. Recent work concentrated on tests + feature completion, with strong UI and backend movement.
- Prefix counts from commit subjects since 2026-02-01: `test` (26), `feat` (22), `fix` (21), `ui` (19), `ci` (11).
- Top changed roots: `ui` (202), `cmd` (105), `pkg` (98), `internal` (85).
- Key shipped slices:
  - `4a6048d`: full UI-backend integration across event/terminology/dashboard routes.
  - `8b58964`: Postgres-backed GraphQL EventStore wiring.
  - `6e1c5e7`: terminology index CLI and coverage uplift.
  - `96550d1` + `843ba26`: event contract alignment + drift checks.

2. fi-fhir backend→frontend wiring is now concrete and production-shaped.
- Backend serves GraphQL + WebSocket + health endpoints (`/graphql`, `/graphql/ws`, `/health`).
- `serve` CLI help documents GraphQL runtime, not REST `/api/v1/*` runtime handlers.
- UI GraphQL clients call `/graphql` over HTTP and `/graphql/ws` for subscriptions.
- UI nginx template proxies `/graphql`, `/graphql/ws`, `/health`, `/ready`, and `/api/` to backend origin.
- Compose ships full-stack services (fi-fhir, postgres, kafka, qdrant, temporal, fi-fhir-ui, observability).

3. Persistence and contract governance are in place but not fully enforced.
- `initEventStoreFromEnv` enables Postgres event store when DB env vars exist and falls back to in-memory when absent.
- Contract checker script compares canonical (`pkg/events`), GraphQL enum, and OpenAPI enum.
- Make targets exist (`contract-check`, `contract-check-strict`, `contract-matrix`).
- CI `lint:contracts` still runs with `allow_failure: true`, so drift can surface without merge blocking.

4. Workflow action surface is broad enough for cross-service integration without architecture rewrite.
- Built-in engine actions include `webhook`, `fhir`, `database`, `queue`, `event_store`, plus `exec`/`email`/`file`/`log`.
- Validator enforces action-shape rules and warns on unknown patterns; transform schema already supports mapping and redaction primitives.

5. Sibling repos expose immediately usable integration contracts.
- `flexinfer`: OpenAI-compatible proxy endpoints (`/v1/models`, `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`) with SSE pass-through and documented cold-start/queue semantics.
- `loom-core`: Streamable MCP HTTP endpoint (`POST /mcp`) with auth/TLS modes and compatibility policy for tool/CLI surfaces; suited for control-plane automation and ops integration rather than patient-data runtime path.
- `mentatlab`: orchestrator REST + SSE under `/api/v1` (`/runs`, `/flows`, `/runs/{id}/events`) with replay/Last-Event-ID semantics; suitable for run orchestration and status fanout.

6. Planning constraint: codebase-memory index remains unavailable.
- `repo_id=fi-fhir` reports `total_chunks: 0`.
- Index job stayed `running` with `files_total=0/files_done=0` and was canceled.
- Local shell/file evidence remains the dependable planning baseline for now.

## Options

### Option A: Direct Point-to-Point Integrations

- Pros: fastest initial hookup.
- Cons: auth/timeouts/retries drift across services; hard to operate at scale.

### Option B: Event-Driven Integration via Existing Workflow Actions

- Pros: leverages existing `webhook`/`queue`/`event_store`; preserves profile-driven parsing and warning semantics.
- Cons: needs stronger contract/version governance and trace propagation.

### Option C: Control-Plane + Runtime Split (recommended)

- Runtime path: fi-fhir backend/UI + workflow actions for clinical/event processing.
- Cross-service runtime edges: `flexinfer` (LLM inference) and `mentatlab` (orchestration/SSE state).
- Control-plane/ops path: `loom-core` for tooling orchestration, diagnostics, and automation workflows.
- Pros: aligns with current shipped boundaries; minimizes rewrite risk.
- Cons: requires explicit policy docs to prevent accidental coupling across planes.

## Recommendation

Adopt Option C with a phased integration program:

1. Lock internal contract baseline (promote drift gate, add smoke tests for `/graphql`, `/graphql/ws`, `/health`, `/ready`).
2. Complete backend↔frontend contract testing (typed query/mutation/subscription parity and startup env profiles).
3. Integrate `flexinfer` via OpenAI-compatible client profile and codified cold-start timeout/retry envelope.
4. Integrate `mentatlab` for workflow run orchestration + SSE status ingestion using correlation IDs.
5. Integrate `loom-core` as control-plane automation surface (health checks, CI/runtime diagnostics, runbooks).

## Sources

- [S1] Command: `git log --since='2026-02-01' --date=short --pretty=format:'%h %ad %s' -n 80`
- [S2] Command: `git log --since='2026-02-01' --name-only --pretty=format: | awk -F/ 'NF{print $1}' | sort | uniq -c | sort -nr | head -20`
- [S3] Command: `git log --since='2026-02-01' --pretty=format:'%s' | sed 's/:.*//' | awk '{print tolower($1)}' | sed 's/[^a-z0-9_-].*//' | sort | uniq -c | sort -nr | head -20`
- [S4] Commands: `git show --stat --oneline ... 4a6048d`, `8b58964`, `6e1c5e7`, `96550d1`, `843ba26`
- [S5] `internal/api/graphql/server.go:126`
- [S6] `internal/api/graphql/server.go:130`
- [S7] `internal/api/graphql/server.go:141`
- [S8] `cmd/fi-fhir/main.go:4901`
- [S9] `cmd/fi-fhir/main.go:4930`
- [S10] `ui/src/lib/graphql/client.ts:38`
- [S11] `ui/src/lib/graphql/subscriptions.ts:29`
- [S12] `ui/nginx/default.conf.template:40`
- [S13] `ui/nginx/default.conf.template:54`
- [S14] `ui/nginx/default.conf.template:70`
- [S15] `ui/Dockerfile:34`
- [S16] `docker-compose.yaml:198`
- [S17] `cmd/fi-fhir/serve_event_store.go:16`
- [S18] `cmd/fi-fhir/main.go:4695`
- [S19] `scripts/check_event_contracts.go:40`
- [S20] `Makefile:203`
- [S21] `.gitlab-ci.yml:320`
- [S22] `.gitlab-ci.yml:325`
- [S23] `internal/workflow/engine.go:125`
- [S24] `internal/workflow/engine.go:134`
- [S25] `internal/workflow/validate.go:230`
- [S26] `internal/workflow/validate.go:289`
- [S27] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
- [S28] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:111`
- [S29] `/Users/cblevins/workspace/services/flexinfer/docs/user/proxy.md:45`
- [S30] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
- [S31] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:221`
- [S32] `/Users/cblevins/workspace/services/loom-core/docs/API_STABILITY.md:72`
- [S33] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
- [S34] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:12`
- [S35] `/Users/cblevins/workspace/services/mentatlab/docs/references/orchestrator-api.md:57`
- [S36] `/Users/cblevins/workspace/services/mentatlab/docs/references/orchestrator-api.md:60`
- [S37] Tool output: `mcp__loom__codebase_memory__codebase_stats(repo_id='fi-fhir')`
- [S38] Tool outputs: `codebase_index_start/poll/cancel` (`job_id=4f93c59a0acaa0a1`)

## 2026-07-12 Completion audit: integration engine and IDE

This section supersedes the earlier description of the frontend/backend stack as
"production-shaped" and the sibling-integration-first recommendation. Those
contracts remain useful, but completion now prioritizes the clinical runtime and
authoring lifecycle inside fi-fhir.

### Question and method

What separates the current repository from a complete, healthcare-grade
integration engine and IDE? The audit refreshed `origin/main`, reviewed the
runtime/GraphQL/IDE/deployment/CI paths in parallel, exercised Go and UI suites,
proved a live GraphQL WebSocket session run, inspected historical security jobs,
and compared the product boundary with official NextGen Connect, InterSystems,
FHIR, SMART, and Bulk Data documentation.

### Findings

1. **The capability kernel is real; the product spine is missing.** Parsers,
   Source Profiles, canonical events, workflow actions, FHIR mapping,
   event-sourcing primitives, terminology, GraphQL, and substantial SvelteKit
   authoring surfaces work. The interactive parse/persist/workflow composition
   lives in GraphQL, while headless ingress does not share that composition.

2. **There is no deployed integration lifecycle.** `serve` loads one workflow;
   generic webhook ingestion wraps JSON rather than parsing healthcare payloads;
   S3/SFTP discovery is not registered into the runtime and does not complete
   parse/route/checkpoint; no MLLP source exists.

3. **The Integration Session Engine is a useful prototype, not the IDE runtime.**
   It is in-memory and HL7-only, ignores selected profile/workflow drafts, forces
   raw retention in its UI path, is feature-flagged off by default, and does not
   wire its existing subscription helper. A manual WebSocket proof did establish
   that subscribe-before-run yields ordered stage/diagnostic/completion events.

4. **Several IDE surfaces are real but not durable or truthful.** Profile publish
   summary logic compares the selection to itself; terminology upload event names
   disagree; debug mutation/subscription paths can duplicate steps; workflow
   drafts and HL7 samples use browser storage; dashboard alerts/trends are partly
   hard-coded; document tabs remain placeholders.

5. **Deployment and observability claims drift from executables.** Docker and
   Compose default to `help`; the Kustomize probe runs `version`; port 9090 and
   `/ready`/`/metrics` are claimed without the corresponding mounted runtime.
   GraphQL currently permits wildcard CORS/WebSocket origins and has no complete
   auth/RBAC/tenant boundary.

6. **CI can be green without exercising the integration contract.** The binary
   producer rules do not match UI/smoke consumers, UI/smoke jobs may exit zero
   when the binary is absent, and `scripts/smoke-test.sh` exits after its first
   successful increment under `set -e`. npm install/test is current (571 pass,
   2 live tests skipped); the committed pnpm lock cannot satisfy a frozen install.

7. **The deployed baseline carried known security failures.** Pipeline 15878
   reported 14 reachable standard-library vulnerabilities under Go 1.25.7 and a
   HIGH/HIGH G701 event-store table path, while security jobs were advisory.
   Further review found unvalidated IDE-authored event-store/database identifiers.
   Go 1.26.5, a Go-1.26-built linter, pinned scanners, and strict identifier
   validation are therefore Gate 0A rather than later hardening.

8. **The product comparison points to lifecycle, not parser breadth.** NextGen
   Connect describes filter/transform/extract/route fundamentals; InterSystems
   makes adapters, persistent messages, visual trace, testing, resend, and
   monitoring a cohesive production. fi-fhir should differentiate with
   profile-driven semantics and governed healthcare diagnostics while meeting
   that lifecycle baseline.

### Recommendation

Execute in this order: secure baseline -> truthful CI -> pre-schema
tenant/identity/PHI/secret contracts -> one shared MessageProcessor -> durable
receipt/idempotency/outbox -> authenticated HTTP kill-test -> IntegrationDefinition
lifecycle/MLLP -> durable IDE -> operations/governance/scale -> pinned standards
conformance. Do not add connector breadth or sibling-service coupling until the
Golden Path 001 profile/duplicate/restart/IDE-parity kill-test passes.

### Completion-audit sources

- [C1] `cmd/fi-fhir/main.go` (`serve` workflow and HTTP mounts)
- [C2] `internal/ingest/http.go` and `internal/ingest/temporal.go`
- [C3] `internal/api/graphql/resolvers/schema.resolvers.go`
- [C4] `internal/integration/session/runner.go` and `store.go`
- [C5] `ui/src/lib/features/integration-session/api.ts`
- [C6] `Dockerfile`, `docker-compose.yaml`, and `deploy/kubernetes/base/deployment.yaml`
- [C7] `.gitlab-ci.yml` and `scripts/smoke-test.sh`
- [C8] GitLab pipelines `15878` and `16623`
- [C9] https://github.com/nextgenhealthcare/connect
- [C10] https://docs.intersystems.com/irislatest/csp/docbook/DocBook.UI.Page.cls?KEY=EGIN_intro
- [C11] https://go.dev/doc/devel/release
- [C12] https://hl7.org/fhir/us/core/STU9/
- [C13] https://hl7.org/fhir/smart-app-launch/
- [C14] https://hl7.org/fhir/uv/bulkdata/
