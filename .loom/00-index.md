# Loom Context Pack

## Quick Links

- Workspace snapshot: `00-workspace-snapshot.md`
- MCP inventory: `00-mcp-inventory.md`
- Research: `10-research.md`
- Product spec: `20-product-spec.md`
- UX professional redesign spec + plan: `21-ux-professional-redesign.md`
- Implementation plan: `30-implementation-plan.md`
- Decisions: `40-decisions.md`
- Worklog: `50-worklog.md`

## Current Goal

- [x] Review recent commit stream (feature focus, quality gates, and integration-relevant deltas).
- [x] Update planning for full backend-to-frontend platform integration in fi-fhir.
- [x] Define concrete integration points to sibling repos in `~/workspace/services` (`flexinfer`, `loom-core`, `mentatlab`).
- [ ] Execute M0 integration alignment tasks (contract CI promotion, gateway baseline, and smoke tests).

## Success Criteria

- [x] Commit-backed current-state summary captured, including key shipped feature slices.
- [x] End-to-end fi-fhir runtime path documented from UI proxy to GraphQL/EventStore backend.
- [x] Cross-service integration map defined with explicit API/protocol contracts and phased delivery.
- [x] Planning artifacts updated with actionable milestones, test strategy, and rollout gates.
- [x] GitLab tracking issues created for M1-M3: [#9](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/9), [#10](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/10), [#11](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/11).

## Open Questions

- [ ] Should `lint:contracts` be promoted to hard-fail immediately, or after a fixed burn-in window (for example one clean week)?
- [ ] For `mentatlab` integration, do we want fi-fhir to create runs directly (`POST /api/v1/runs`) or publish integration events that a gateway consumes?
- [ ] Should `loom-core` integration stay control-plane only (ops/automation) or expose selected runtime actions through fi-fhir admin APIs?

## Risks

- [ ] Contract policy drift remains possible while `lint:contracts` is still `allow_failure: true`.
- [ ] Codebase-memory indexing is currently unavailable (`total_chunks: 0`), reducing semantic-search-assisted planning confidence.
- [ ] Cross-service auth/timeouts differ (GraphQL, OpenAI-compatible proxy, REST+SSE, Streamable MCP HTTP), raising integration fragility without a unified policy.
- [ ] Cold-start behavior in `flexinfer` can violate fi-fhir workflow latency assumptions unless retry/timeout envelopes are codified.
