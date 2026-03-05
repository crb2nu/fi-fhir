# Product Spec

## Summary

Deliver an integration-complete fi-fhir platform slice that is operationally coherent from backend to frontend, and explicitly interoperable with sibling services:

- `flexinfer` for inference workloads (OpenAI-compatible APIs)
- `mentatlab` for run orchestration and SSE lifecycle visibility
- `loom-core` for control-plane automation and operational tooling

## Goals

- Complete and harden fi-fhir internal backend→frontend contracts (GraphQL HTTP + WS + health + env profiles).
- Enforce canonical contract governance across canonical events, GraphQL schema, and OpenAPI artifacts.
- Integrate inference path with `flexinfer` using predictable retry/timeout behavior around cold starts.
- Integrate orchestration path with `mentatlab` for run initiation and live status/event monitoring.
- Integrate ops/control path with `loom-core` for runbook automation, diagnostics, and environment health.

## Non-Goals

- Rewriting fi-fhir into a separate ingestion/orchestration platform.
- Replacing GraphQL runtime with a new primary API surface in this phase.
- Deeply coupling clinical runtime processing to loom-core internals.

## Users / Stakeholders

- Integration platform engineers (backend + workflow + contracts)
- Frontend engineers (Svelte routes, GraphQL clients, live event UX)
- MLOps / AI platform teams (`flexinfer` integration and SLOs)
- Orchestration teams (`mentatlab` flows/runs)
- SRE / platform ops (`loom-core` automation and diagnostics)

## Requirements

### Functional

1. Backend↔Frontend Integration Baseline
- GraphQL HTTP and WS endpoints remain stable and versioned.
- UI routes (`/`, `/events`, `/terminology`) are validated against backend schema and availability.
- Startup profiles (`.env.example`, `configs/full-stack.env`, compose) are aligned and tested.

2. Contract Governance
- Canonical event model remains the source of truth.
- Drift checks (`canonical ↔ GraphQL ↔ OpenAPI`) run in CI and become blocking after rollout gate.
- Contract matrix is regenerated with every schema/event change.

3. FlexInfer Integration
- fi-fhir LLM/inference clients support OpenAI-compatible calls to flexinfer proxy endpoints.
- Timeouts/retries account for queue/cold-start behavior.
- Error contracts are mapped into fi-fhir warning/error taxonomy for workflow handling.

4. Mentatlab Integration
- fi-fhir can start or signal orchestration runs via `mentatlab` `/api/v1` endpoints.
- fi-fhir can consume run lifecycle via SSE (`/runs/{id}/events`) with replay/resume.
- Correlation IDs are shared across fi-fhir event metadata and orchestrator run context.

5. Loom-Core Integration (Control Plane)
- Runbook operations and diagnostics can be executed via loom-core tooling (`/mcp` remote transport where applicable).
- Integration playbooks are explicit about control-plane boundaries and avoid PHI runtime dependencies.

### Non-Functional

- Reliability: integration failures degrade gracefully (warnings + retries), no hard parser regressions.
- Security: explicit auth/TLS posture per service edge (`flexinfer`, `mentatlab`, `loom-core`).
- Observability: trace/log correlation across backend, UI request edges, and sibling-service calls.
- Operability: reproducible local full-stack dev setup and smoke test script.

## End-to-End Flows

1. Internal Processing Flow
- Ingest payload → parse with warnings → map canonical event → workflow actions → EventStore → GraphQL query/subscription → UI views.

2. Inference-Assisted Flow (`flexinfer`)
- fi-fhir workflow or LLM action calls flexinfer OpenAI-compatible endpoint.
- If model is cold, request waits through activation queue bounded by configured timeout.
- Result returns to fi-fhir and is persisted/annotated for auditability.

3. Orchestration Flow (`mentatlab`)
- fi-fhir initiates run request for complex external orchestration.
- fi-fhir subscribes to run SSE stream and updates event/action status.
- Replay support and correlation IDs enable deterministic incident reconstruction.

4. Operations Flow (`loom-core`)
- Operators invoke loom tools for health checks, deployment diagnostics, and scripted mitigations.
- Output is stored in worklog/runbook artifacts; runtime data path remains in fi-fhir.

## API / Data Contracts

- fi-fhir runtime API: GraphQL (`/graphql`, `/graphql/ws`) + health endpoints.
- OpenAPI remains documentation/contract artifact and must stay schema-aligned.
- `flexinfer`: `/v1/models`, `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings` (+ streaming).
- `mentatlab`: `/api/v1/runs`, `/api/v1/runs/{id}/events` and related run/flow APIs.
- `loom-core`: Streamable MCP HTTP (`POST /mcp`) for control-plane tool routing.

## Rollout

- Phase 0: baseline contract + smoke gate.
- Phase 1: internal backend/frontend parity and automated checks.
- Phase 2: flexinfer integration behind feature flag.
- Phase 3: mentatlab integration behind feature flag.
- Phase 4: loom-core control-plane playbooks and cross-service observability dashboarding.

## Acceptance Criteria

- Contract CI fails on drift after rollout toggle date.
- UI GraphQL operations used in dashboard/events/terminology pass against live server in CI.
- flexinfer path passes timeout/cold-start integration tests with documented retry envelope.
- mentatlab run lifecycle can be created and observed with SSE replay in integration tests.
- Ops playbook commands execute through loom-core with reproducible outputs and no manual hidden steps.

## Risks

- Contract soft-fail period can mask regressions.
- Cold-start latency variance from flexinfer may exceed current workflow defaults.
- SSE reconnect/resume edge cases can create duplicate state transitions without idempotency keys.
- Cross-service auth divergence can stall rollout unless standardized early.

## Open Questions

- Timeline to flip `lint:contracts` to blocking for all merge requests.
- Preferred mentatlab integration mode: direct per-event run creation vs batched/coordinated orchestration.
- Which loom-core tools should be mandatory vs optional for platform operations.

## Sources

- [S1] `internal/api/graphql/server.go:126`
- [S2] `cmd/fi-fhir/main.go:4901`
- [S3] `ui/src/lib/graphql/client.ts:38`
- [S4] `ui/src/lib/graphql/subscriptions.ts:29`
- [S5] `ui/nginx/default.conf.template:40`
- [S6] `ui/nginx/default.conf.template:54`
- [S7] `docker-compose.yaml:198`
- [S8] `scripts/check_event_contracts.go:40`
- [S9] `Makefile:203`
- [S10] `.gitlab-ci.yml:320`
- [S11] `.gitlab-ci.yml:325`
- [S12] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
- [S13] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:149`
- [S14] `/Users/cblevins/workspace/services/flexinfer/docs/user/proxy.md:45`
- [S15] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
- [S16] `/Users/cblevins/workspace/services/mentatlab/docs/references/orchestrator-api.md:57`
- [S17] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
- [S18] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:221`
- [S19] `internal/workflow/engine.go:125`
- [S20] `internal/workflow/engine.go:134`
