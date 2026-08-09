# Loom Context Pack

## Quick Links

- Workspace snapshot: `00-workspace-snapshot.md`
- MCP inventory: `00-mcp-inventory.md`
- Research: `10-research.md`
- Product spec: `20-product-spec.md`
- UX professional redesign spec + plan: `21-ux-professional-redesign.md`
- Toast budget policy: `22-toast-budget-policy.md`
- **IDE + Backend functionality gap-fill program: `23-functionality-gaps-plan.md`** (active)
- **Parallel execution specs: `24-parallel-execution-specs.md`** (active handoff map)
- Product speclet - CDA/CCDA section expansion: `25-spec-cda-section-expansion.md`
- Product speclet - Storage/provider integration tests: `26-spec-storage-provider-tests.md`
- Product speclet - Terminology approval workflow hardening: `27-spec-terminology-governance.md`
- Product speclet - FHIR IG/Bulk/SMART scoping: `28-spec-fhir-ig-bulk-smart.md`
- Product speclet - Dynamic Source Profile management: `29-spec-profile-management-observability.md`
- Implementation plan: `30-implementation-plan.md`
- Decisions: `40-decisions.md`
- Worklog: `worklog/` (one file per entry; `make worklog` renders it, `make worklog-new TITLE="..."` starts one). `50-worklog.md` is a pointer kept for existing links.

## Current Goal — IDE + Backend functionality gap-fill (see `23-functionality-gaps-plan.md`)

- [x] Audit IDE + backend for functional gaps (UI mock-masquerade + unwired capability; backend reach gaps).
- [x] Produce sequenced 3-wave program with riskiest-assumption kill-test.
- [x] Convert latest brainstorm/spec docs into parallel execution lanes (`24-parallel-execution-specs.md`).
- [x] **Lane A**: verify/harden Workflow AI generate/explain surfaces now that code is wired.
- [x] **Lane B**: unify LLM config namespace and add truthful capability surface.
- [x] **Lane D**: recover terminology DB integration tests and CI path before more store automation.
- [x] **Lane F**: split the product backlog into speclets `25`-`29`.
- [x] **Lane C1**: serve-time pending-autoroute expiry sweep (2026-08-03).
- [ ] **Lane C2**: pending-autoroute notifications (webhook, thresholds, non-blocking dispatch).
- [x] **Lane E**: integration/contract CI hardening (2026-08-08). `test:integration` and `lint:docs` are now blocking; the CI `minio` service was dead on arrival, silently skipping 30 integration tests.

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
- [ ] Should LLM runtime config make `FI_FHIR_LLM_*` canonical with `LLM_*` fallback, or keep both namespaces fully peer-level?

## Risks

- [x] Contract policy drift: closed — `lint:contracts` was promoted to blocking on 2026-03-04.
- [ ] STATUS.md coverage drift can still reach main while `test:docs-status` is advisory (see `.loom/40-decisions.md`, 2026-08-08).
- [ ] Codebase-memory indexing is currently unavailable (`total_chunks: 0`), reducing semantic-search-assisted planning confidence.
- [ ] Cross-service auth/timeouts differ (GraphQL, OpenAI-compatible proxy, REST+SSE, Streamable MCP HTTP), raising integration fragility without a unified policy.
- [ ] Cold-start behavior in `flexinfer` can violate fi-fhir workflow latency assumptions unless retry/timeout envelopes are codified.
