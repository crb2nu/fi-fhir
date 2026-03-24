# Iteration Plan: M2 Terminology CLI Telemetry

## Review

- Roadmap milestone: M2 Terminology Approval Workflow
- Spec section(s): `.loom/20-product-spec.md` terminology governance; `.loom/30-implementation-plan.md` M2; `docs/planning/TERMINOLOGY-MAPPING.md`
- Prior decisions to preserve:
  - Keep terminology governance additive and backward-compatible.
  - Prefer small operational slices over broad workflow rewrites.

## Align

- Slice name: CLI telemetry polish for terminology mapping decisions
- Scope in:
  - Record `fi-fhir terminology mapping resolve` outcomes into `terminology.mapping_decisions`
  - Add read-only CLI commands to inspect decision telemetry and aggregate stats
  - Update roadmap/spec docs to reflect the shipped CLI telemetry surface
- Scope out:
  - OpenTelemetry span attributes
  - Partitioned telemetry tables
  - UI analytics dashboard / notifications
- Acceptance criteria:
  1. `mapping resolve` records `PERSISTENT_HIT`, `AUTOROUTE_*`, and `NO_MATCH` decisions with `request_source=cli`
  2. CLI users can list decisions, fetch one decision trace, and view summary stats
  3. Terminology planning docs reflect the new CLI coverage accurately
- Dependencies/blockers:
  - Existing `pkg/terminology/db.MappingStore` decision APIs
  - Local disk space may constrain package-level Go test execution

## Land

- Planned file areas:
  - `cmd/fi-fhir/terminology.go`
  - `cmd/fi-fhir/terminology_telemetry_cli_test.go`
  - `cmd/fi-fhir/terminology_telemetry_integration_test.go`
  - `.loom/*.md`
  - `docs/planning/*.md`
- Implementation steps:
  1. Extend terminology mapping CLI dispatch/help for telemetry commands.
  2. Persist CLI resolve outcomes into `mapping_decisions`.
  3. Add focused tests and update planning/spec notes.

## Prove

- Tests to run:
  - Focused CLI tests for telemetry parsing/help/output helpers
  - Integration test for telemetry commands when Docker-backed infra is available
- Lint/static checks:
  - `gofmt` on touched Go files
- CI checks:
  - Re-run targeted CLI tests once disk pressure is resolved

## Handoff/Harvest

- Docs to update:
  - `docs/planning/README.md`
  - `docs/planning/TERMINOLOGY-MAPPING.md`
  - `.loom/40-decisions.md`
  - `.loom/50-worklog.md`
- Agent-context entries to add:
  - Decision: expose terminology decision telemetry via CLI before deeper analytics/UI work
  - Finding: package-level Go tests are currently constrained by local disk pressure
- Next-slice candidates:
  - Add OpenTelemetry span attributes for mapping decision traces
  - Build GraphQL/UI mapping analytics dashboard
