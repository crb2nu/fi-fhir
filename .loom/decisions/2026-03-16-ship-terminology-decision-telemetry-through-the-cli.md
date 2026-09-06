### 2026-03-16: Ship Terminology Decision Telemetry Through the CLI Before Analytics/UI Work

- Decision:
  - Land a narrow CLI telemetry slice now: record `terminology mapping resolve` decisions into `mapping_decisions`, and expose read-only decision list/detail/stats commands before tackling OTel polish or UI analytics.
- Rationale:
  - The persistence layer and workflow telemetry path already exist, so CLI parity is a small, high-leverage gap that improves auditability without expanding the workflow surface.
  - This keeps M2 moving with a backward-compatible increment and gives operators a concrete inspection path for clinical mapping decisions.
- Alternatives considered:
  - Jump directly to UI analytics or OpenTelemetry enrichment (rejected; broader scope and less immediately useful for CLI/operator workflows).
- Consequences:
  - Decision telemetry becomes easier to validate and troubleshoot from the terminal.
  - OTel spans, partitioning, and analytics dashboards remain explicit follow-up work.
- Sources:
  - [S1] `docs/planning/README.md:16`
  - [S2] `docs/planning/TERMINOLOGY-MAPPING.md:14`
  - [S3] `pkg/terminology/db/mappings.go:1059`
  - [S4] `cmd/fi-fhir/terminology.go:1490`
