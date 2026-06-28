# RALPH Iteration Plan: LLM Capability Query

## Review

- Roadmap milestone: Wave 3 backend capability fills / LLM honest-state follow-up
- Spec section(s): `.loom/23-functionality-gaps-plan.md` Slice 3e follow-ups, `docs/user-guide/llm-features.md`
- Prior decisions to preserve:
  - `FI_FHIR_LLM_*` is the canonical namespace for new deployments.
  - Legacy unset/empty `FI_FHIR_LLM_ENABLED` preserves auto-attempt behavior for existing deployments.
  - Existing LLM operations keep graceful-degrade responses when resolver modules are absent.

## Align

- Slice name: GraphQL LLM capability query
- Scope in:
  - Add a read-only `llmCapabilities` GraphQL field.
  - Report disabled, unavailable, unconfigured, available, and partial states without probing an LLM operation.
  - Populate explicit serve-time capability state from the existing LLM initialization path.
  - Add focused resolver and serve helper tests.
  - Update roadmap/user docs for the new backend contract.
- Scope out:
  - Copilot UI badge/rendering changes.
  - Per-feature `FI_FHIR_LLM_*_ENABLED` toggles.
  - Embedding/semantic terminology namespace changes.
  - Live provider validation.
- Acceptance criteria:
  1. `llmCapabilities` returns `disabled` when `FI_FHIR_LLM_ENABLED=false`.
  2. `llmCapabilities` returns `unavailable` when LLM client construction fails.
  3. Successful serve LLM setup returns enabled capability rows and `partial` when terminology DB-backed suggestions are not configured.
  4. Resolver defaults remain honest (`unconfigured`) when no LLM resolver options are registered.
  5. Existing LLM operation behavior is unchanged.
- Dependencies/blockers:
  - Existing serve LLM setup helper from Slice 3e.
  - gqlgen generation path must remain clean.

## Land

- Planned file areas:
  - `internal/api/graphql/schema.graphql`
  - `internal/api/graphql/resolvers/`
  - `internal/api/graphql/model/`
  - `cmd/fi-fhir/main.go`
  - `cmd/fi-fhir/serve_coverage_test.go`
  - `docs/user-guide/llm-features.md`
  - `.loom/23-functionality-gaps-plan.md`
- Implementation steps:
  1. Add GraphQL capability schema/model/resolver.
  2. Attach serve-time LLM capability status during initialization.
  3. Regenerate gqlgen and add focused tests.

## Prove

- Tests to run:
  - `go test ./internal/api/graphql/resolvers -run 'TestQueryResolver_LlmCapabilities|TestQueryResolver_Health'`
  - `go test ./cmd/fi-fhir -run 'TestServeLLMEnabledFromEnv|TestInitServeLLMFeatures'`
  - `go test ./internal/api/graphql/... ./cmd/fi-fhir`
- Lint/static checks:
  - `gofmt`
  - `git diff --check`
  - `go vet ./internal/api/graphql/... ./cmd/fi-fhir`
- CI checks:
  - Not verified in this local slice.

## Handoff/Harvest

- Docs to update:
  - `docs/user-guide/llm-features.md`
  - `.loom/23-functionality-gaps-plan.md`
- Agent-context entries to add:
  - Decision: expose explicit GraphQL LLM capability state instead of requiring UI probes.
  - Finding: serve can report disabled/unavailable/partial states from existing initialization outcomes.
- Next-slice candidates:
  - Wire Copilot UI to query `llmCapabilities` and render honest badges/disabled actions.
  - Wire per-feature LLM toggles into serve.
  - Extend canonical namespace support to embedding/semantic terminology config.
