# Implementation Plan

## Scope

Implement a phased integration program spanning:

- fi-fhir backend↔frontend hardening
- contract governance enforcement
- sibling integrations with `flexinfer`, `mentatlab`, and `loom-core`

## Milestones

1. M0 - Baseline and Contract Hardening (1 sprint)
2. M1 - Backend↔Frontend Integration Completion (1 sprint)
3. M2 - FlexInfer Runtime Integration (1 sprint)
4. M3 - Mentatlab Orchestration Integration (1 sprint)
5. M4 - Loom-Core Control-Plane Integration (0.5-1 sprint)
6. M5 - Cross-Service Hardening and Rollout (1 sprint)

## Tracking Issues

- M1: [#9](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/9) - Complete backend↔frontend integration parity.
- M2: [#10](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/10) - Integrate flexinfer inference path.
- M3: [#11](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/11) - Integrate mentatlab orchestration and SSE lifecycle events.

## Plan

1. M0 Baseline and Contract Hardening
- Keep `scripts/check_event_contracts.go` as canonical diff checker and ensure matrix artifact generation.
- Change CI `lint:contracts` from soft-fail to blocking after a defined clean-run gate.
- Add a single smoke script covering `/health`, `/graphql`, `/graphql/ws` reachability for local full-stack.
- Deliverable: enforced contract gate + smoke baseline in CI.

2. M1 Backend↔Frontend Integration Completion
- Add CI-backed GraphQL operation checks for UI query docs (`events.graphql`, `terminology.graphql`).
- Validate proxy config assumptions (`FI_FHIR_UI_API_ORIGIN`, WS upgrades, `/ready` passthrough).
- Add regression tests for Event Dashboard and Terminology route data paths against live backend.
- Deliverable: reproducible local and CI flow from UI route to backend resolver response.

3. M2 FlexInfer Runtime Integration
- Define fi-fhir integration profile for OpenAI-compatible flexinfer endpoints.
- Add timeout/retry policy aligned to cold-start/queue semantics.
- Map flexinfer errors (`503/504`, queue/timeout) into structured workflow warnings/errors.
- Add integration tests with mocked and live-proxy scenarios.
- Deliverable: feature-flagged flexinfer-backed inference path with SLO-aware behavior.

4. M3 Mentatlab Orchestration Integration
- Add adapter for run creation/management via `/api/v1/runs` endpoints.
- Add SSE consumer for `/api/v1/runs/{id}/events` with replay and idempotent processing.
- Propagate correlation IDs between fi-fhir events and mentatlab runs.
- Deliverable: run orchestration and status ingestion with replay-safe state updates.

5. M4 Loom-Core Control-Plane Integration
- Define runbooks that call loom-core tool surfaces for diagnostics and maintenance.
- Optionally wire remote `/mcp` usage for shared daemon operations where org policy allows.
- Capture control-plane outputs in consistent operational artifacts (logs/worklog/checklists).
- Deliverable: documented, executable ops workflows with clear separation from runtime PHI path.

6. M5 Cross-Service Hardening and Rollout
- Introduce unified integration config profile (auth, timeouts, retry budgets, tracing headers).
- Add cross-service observability dashboard and alert rules (error rate, latency, reconnect churn).
- Roll out by environment and source-profile cohort with rollback toggles.
- Deliverable: production rollout checklist with verification evidence.

## Test Plan

- Unit Tests
  - Contract drift checker and enum mapping tests.
  - Integration adapters (flexinfer error mapping, mentatlab run/SSE parsing).
  - Correlation ID propagation helpers.

- Integration Tests
  - End-to-end UI → GraphQL → EventStore flow.
  - Flexinfer cold-start timeout/retry behavior.
  - Mentatlab SSE replay/resume and dedup behavior.

- CI Gates
  - `contract-check-strict` as blocking.
  - GraphQL client documents compile/typecheck against current schema.
  - Smoke probes for core runtime endpoints.

- Performance/Resilience
  - Latency budget checks across external service edges.
  - Fault injection for 503/504, SSE disconnects, and delayed health readiness.

## Rollout / Backout

- Rollout
  - Enable each sibling integration behind feature flags.
  - Start in non-prod with synthetic traffic, then canary by source-profile cohort.
  - Promote contract gate and observability alerts before broad rollout.

- Backout
  - Disable sibling integration flags independently (`flexinfer`, `mentatlab`, `loom-core` paths).
  - Preserve core fi-fhir parse/workflow/EventStore behavior without external dependencies.
  - Revert CI gate strictness only under explicit incident exception.

## Acceptance Criteria

- M0: Contract drift blocks merges after gate promotion.
- M1: UI dashboard/events/terminology flows pass against live backend in CI.
- M2: flexinfer integration passes timeout/retry/error mapping tests.
- M3: mentatlab run create + SSE consume + replay works idempotently.
- M4: loom-core runbooks executed and validated in at least one shared environment.
- M5: rollout shows stable error budgets and no regression in parse/workflow throughput.

## Dependencies

- fi-fhir CI maintainers for gate promotion timing.
- flexinfer environment availability and model warm/cold behavior profiling.
- mentatlab environment with SSE endpoint availability and auth policy.
- loom-core shared daemon availability for control-plane workflows.

## Risks / Mitigations

- Risk: contract gate churn blocks development.
  - Mitigation: short burn-in window with daily drift review before hard enforcement.
- Risk: external cold starts cause cascading timeouts.
  - Mitigation: explicit budgets, circuit breakers, queue-aware retries.
- Risk: SSE duplicate processing.
  - Mitigation: Last-Event-ID + idempotency key handling in consumer state.

## Sources

- [S1] `scripts/check_event_contracts.go:40`
- [S2] `Makefile:203`
- [S3] `.gitlab-ci.yml:320`
- [S4] `.gitlab-ci.yml:325`
- [S5] `internal/api/graphql/server.go:126`
- [S6] `internal/api/graphql/server.go:130`
- [S7] `ui/src/lib/graphql/client.ts:38`
- [S8] `ui/src/lib/graphql/subscriptions.ts:29`
- [S9] `ui/nginx/default.conf.template:40`
- [S10] `ui/src/lib/graphql/events.graphql:3`
- [S11] `ui/src/lib/graphql/terminology.graphql:40`
- [S12] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
- [S13] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:149`
- [S14] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
- [S15] `/Users/cblevins/workspace/services/mentatlab/docs/references/orchestrator-api.md:57`
- [S16] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
- [S17] `/Users/cblevins/workspace/services/loom-core/docs/API_STABILITY.md:72`
