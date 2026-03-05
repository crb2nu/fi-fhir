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
