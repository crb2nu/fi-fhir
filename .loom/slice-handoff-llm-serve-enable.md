# RALPH Slice Handoff: LLM Serve Enablement Gate

## Slice Summary

- Milestone: Wave 3 backend capability fills / LLM config namespace follow-up
- Slice: Honor `FI_FHIR_LLM_ENABLED` in `fi-fhir serve`
- Status: complete locally

## What Landed

- Key changes:
  - `fi-fhir serve` now treats explicit `FI_FHIR_LLM_ENABLED=false` as an off switch for GraphQL LLM resolver setup.
  - Unset, empty, or invalid `FI_FHIR_LLM_ENABLED` preserves the legacy auto-attempt path for existing deployments that rely on `LLM_*` or default endpoint/model values.
  - Serve LLM setup is isolated in `initServeLLMFeatures` with `serveLLMClientFactory` for focused tests.
  - LLM user docs and `.loom/23-functionality-gaps-plan.md` now describe the compatibility behavior.
- Key files:
  - `cmd/fi-fhir/main.go`
  - `cmd/fi-fhir/serve_coverage_test.go`
  - `docs/user-guide/llm-features.md`
  - `.loom/iteration-plan-llm-serve-enable.md`
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/50-worklog.md`
- Validation results:
  - `go test ./cmd/fi-fhir -run 'TestServeLLMEnabledFromEnv|TestInitServeLLMFeatures'` — pass
  - `go test ./cmd/fi-fhir` — pass
  - `go test ./...` — pass
  - `go vet ./...` — pass

## What Is Still Open

- Remaining acceptance criteria: none for this slice.
- Known issues:
  - CI was not verified from this local loop.
  - `FI_FHIR_LLM_*_ENABLED` per-feature toggles are still not wired into serve.
  - Embedding/semantic terminology config still uses `LLM_EMBEDDING_*` / `LLM_*`.
  - UI still needs a GraphQL capability field to distinguish LLM disabled from provider unreachable.
- Dependencies:
  - Any future UI badge should depend on an explicit backend capability query rather than inferring from operation errors.

## Next Actions

1. Add a GraphQL LLM capability field for honest UI state.
2. Wire per-feature LLM toggles into serve with compatibility tests.
3. Unify embedding/semantic terminology env namespace.

## Context Links

- Agent-context session: `a8cf7f7bf785d544`
- Task IDs: recorded in agent-context follow-up tasks for this session
- Relevant docs/specs:
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/iteration-plan-llm-serve-enable.md`
  - `docs/user-guide/llm-features.md`
