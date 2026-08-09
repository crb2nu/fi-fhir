### 2026-03-16

- What changed:
  - Added CLI telemetry coverage for terminology mapping decisions:
    - `fi-fhir terminology mapping decisions`
    - `fi-fhir terminology mapping decision`
    - `fi-fhir terminology mapping decision-stats`
  - Updated `terminology mapping resolve` to record CLI decisions into `terminology.mapping_decisions` for persistent hits, autoroute results, and no-match outcomes.
  - Added focused CLI tests plus an integration test scaffold for the new telemetry commands.
  - Updated terminology planning docs and added a dedicated iteration plan for this slice.
- Why:
  - The roadmap/spec still showed terminology CLI/telemetry gaps even though the persistence layer already existed. This closes a high-value auditability gap with a small, shippable increment.
- What’s next:
  - Re-run focused Go tests after freeing disk space on the workstation.
  - Continue Phase 3/5 telemetry follow-ups: OpenTelemetry attributes, analytics query/UI, and retention/partitioning work.
- Sources:
  - [S1] `cmd/fi-fhir/terminology.go`
  - [S2] `cmd/fi-fhir/terminology_telemetry_cli_test.go`
  - [S3] `cmd/fi-fhir/terminology_telemetry_integration_test.go`
  - [S4] `.loom/iteration-plan-m2-terminology-telemetry.md`
