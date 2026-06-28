# RALPH Slice Handoff: LLM Capability Query

## Slice Summary

- Milestone: Wave 3 backend capability fills / LLM honest-state follow-up
- Slice: GraphQL LLM capability query
- Status: complete locally

## What Landed

- Key changes:
  - Added `llmCapabilities` to the GraphQL schema.
  - Added resolver-level capability state with feature rows for explain warnings, extraction, quality, workflow generation/explanation, and terminology suggestions.
  - `fi-fhir serve` now reports `disabled` for explicit `FI_FHIR_LLM_ENABLED=false`, `unavailable` for LLM client construction failures, and `partial` when the LLM client is available but terminology DB-backed suggestions are not configured.
  - Existing LLM operation graceful-degrade behavior is unchanged.
- Key files:
  - `internal/api/graphql/schema.graphql`
  - `internal/api/graphql/generated.go`
  - `internal/api/graphql/model/models.go`
  - `internal/api/graphql/resolvers/resolver.go`
  - `internal/api/graphql/resolvers/schema.resolvers.go`
  - `cmd/fi-fhir/main.go`
  - `cmd/fi-fhir/serve_coverage_test.go`
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/iteration-plan-llm-capabilities.md`
- Validation results:
  - `go test ./internal/api/graphql/resolvers -run 'TestQueryResolver_LlmCapabilities|TestQueryResolver_Health'` — pass
  - `go test ./cmd/fi-fhir -run 'TestServeLLMEnabledFromEnv|TestInitServeLLMFeatures'` — pass
  - `go test ./internal/api/graphql/... ./cmd/fi-fhir` — pass
  - `go vet ./internal/api/graphql/... ./cmd/fi-fhir` — pass
  - gqlgen regeneration idempotence check — pass
  - `git diff --check` — pass
  - `go test ./...` — pass

## What Is Still Open

- Remaining acceptance criteria:
  - None for this backend slice.
- Known issues:
  - CI was not verified from this local loop.
  - Copilot UI still needs to query `llmCapabilities` and render disabled/unavailable/partial state.
  - Per-feature `FI_FHIR_LLM_*_ENABLED` toggles are still not wired into serve.
  - Embedding/semantic terminology config still uses `LLM_EMBEDDING_*` / `LLM_*`.
- Dependencies:
  - Future UI badge work should use `llmCapabilities` rather than inferring state from failed LLM operations.

## Next Actions

1. Run broader local verification and update this handoff with results.
2. Wire Copilot UI to query `llmCapabilities`.
3. Add per-feature serve toggles if a deployment needs partial enablement.

## Context Links

- Agent-context session: `340ad73c02b1ce22`
- Task IDs: pending follow-up task should be created for Copilot UI wiring
- Relevant docs/specs:
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/iteration-plan-llm-capabilities.md`
  - `docs/user-guide/llm-features.md`
