# RALPH Iteration Plan: LLM Serve Enablement Gate

## Review

- Roadmap milestone: Wave 3 backend capability fills / LLM config namespace follow-up
- Spec section(s): `.loom/23-functionality-gaps-plan.md` Slice 3e, `docs/user-guide/llm-features.md`
- Prior decisions to preserve:
  - `FI_FHIR_LLM_*` is the canonical namespace for new deployments.
  - Legacy `LLM_*` deployments must keep working during the namespace transition.

## Align

- Slice name: Honor `FI_FHIR_LLM_ENABLED` in `fi-fhir serve`
- Scope in:
  - Add an explicit serve-time LLM enablement gate for `FI_FHIR_LLM_ENABLED=false`.
  - Preserve legacy serve behavior when the enablement variable is unset.
  - Add focused tests around the gate and serve LLM resolver initialization.
  - Update docs/spec notes for the actual compatibility behavior.
- Scope out:
  - GraphQL capability fields for "LLM enabled" UI badges.
  - Per-feature toggles such as extraction/copilot/data-quality.
  - Embedding and semantic terminology namespace changes.
  - UI changes.
- Acceptance criteria:
  1. `FI_FHIR_LLM_ENABLED=false` prevents serve from constructing an LLM client or registering LLM resolver options.
  2. `FI_FHIR_LLM_ENABLED=true` still initializes LLM resolver options when the client is available.
  3. Unset or empty `FI_FHIR_LLM_ENABLED` keeps legacy auto-attempt behavior for existing deployments.
  4. Client construction failures continue to degrade gracefully with a warning.
- Dependencies/blockers:
  - Existing `pkg/llm.Config.WithEnv()` namespace precedence from Slice 3e.
  - No live provider required because tests use a mock LLM client.

## Land

- Planned file areas:
  - `cmd/fi-fhir/main.go`
  - `cmd/fi-fhir/serve_coverage_test.go`
  - `docs/user-guide/llm-features.md`
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/50-worklog.md`
- Implementation steps:
  1. Extract serve LLM setup into testable helpers.
  2. Add focused tests for enabled/disabled/unset/error paths.
  3. Update RALPH planning and handoff notes.

## Prove

- Tests to run:
  - `go test ./cmd/fi-fhir -run 'TestServeLLMEnabledFromEnv|TestInitServeLLMFeatures'`
  - `go test ./cmd/fi-fhir`
- Lint/static checks:
  - `gofmt` on touched Go files.
- CI checks:
  - Not verified in this local slice.

## Handoff/Harvest

- Docs to update:
  - `docs/user-guide/llm-features.md`
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/50-worklog.md`
- Agent-context entries to add:
  - Decision: explicit `FI_FHIR_LLM_ENABLED=false` disables serve LLM resolvers while unset preserves legacy behavior.
  - Finding: serve LLM setup is now isolated behind a testable helper and mockable client factory.
- Next-slice candidates:
  - Add a GraphQL capability field so the UI can distinguish LLM off from unreachable.
  - Wire per-feature `FI_FHIR_LLM_*_ENABLED` toggles into serve.
  - Extend canonical namespace support to embedding/semantic terminology config.
