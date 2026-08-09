### 2026-06-19 - Lane B LLM config namespace + capability surface

- What changed:
  - Made `pkg/llm.Config.WithEnv()` prefer canonical `FI_FHIR_LLM_*` variables before legacy `LLM_*`, preserving `OPENAI_API_KEY` as the final API-key fallback.
  - Mirrored the same base URL/API key/default model/quality model precedence in `pkg/config.ApplyEnv()`.
  - Added a safe GraphQL `llmCapability` query with `enabled`, `configured`, provider host, model names, `status`, and warnings only.
  - Changed serve-time LLM wiring to honor `FI_FHIR_LLM_ENABLED` and report disabled/unavailable/degraded/available from actual initialization.
  - Updated LLM docs with canonical and legacy variable names.
- Why:
  - Lane B needed runtime LLM configuration to be unambiguous while keeping existing `LLM_*` deployments working.
- Verification:
  - `go test ./pkg/config ./pkg/llm ./cmd/fi-fhir ./internal/api/graphql/...` → passed.
  - `go run github.com/99designs/gqlgen generate --config gqlgen.yml` → passed.
- Sources:
  - [S1] `pkg/llm/config.go`
  - [S2] `pkg/config/config.go`
  - [S3] `cmd/fi-fhir/main.go`
  - [S4] `internal/api/graphql/schema.graphql`
  - [S5] `docs/user-guide/llm-features.md`
