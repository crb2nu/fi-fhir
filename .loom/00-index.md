# Loom Context Pack

## Quick Links

- Workspace snapshot: `00-workspace-snapshot.md`
- MCP inventory: `00-mcp-inventory.md`
- Research: `10-research.md`
- Product spec: `20-product-spec.md`
- UX professional redesign spec + plan: `21-ux-professional-redesign.md`
- Toast budget policy: `22-toast-budget-policy.md`
- **IDE + Backend functionality gap-fill program: `23-functionality-gaps-plan.md`** (active)
- Implementation plan: `30-implementation-plan.md`
- Decisions: `40-decisions.md`
- Worklog: `50-worklog.md`

## Current Goal — IDE + Backend functionality gap-fill (see `23-functionality-gaps-plan.md`)

- [x] Audit IDE + backend for functional gaps (UI mock-masquerade + unwired capability; backend reach gaps).
- [x] Produce sequenced 3-wave program with riskiest-assumption kill-test.
- [ ] **Wave 1 — Honesty pass**: debug no-fake-session, observability simulated-badge, collaboration label-as-preview.
- [ ] **Wave 2 — Copilot real**: run LLM-reachability kill-test (Slice 0), then wire UI Copilot → existing LLM GraphQL.
- [ ] **Wave 3 — Backend fills**: MappingStats + dashboard, pending-autoroute auto-expiry + notifications.

### Prior goal (platform integration M0–M3) — parked
- [x] Backend-to-frontend integration planning; sibling-repo integration points (`flexinfer`, `loom-core`, `mentatlab`).
- [ ] Execute M0 integration alignment (contract CI promotion, gateway baseline, smoke tests).

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
