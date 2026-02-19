# Implementation Plan

## Scope

This plan covers backend ETL orchestration improvements, parser/transform capability expansion, API contract governance, and unified auditability. It does not include a platform rewrite.

## Milestones

1. M0 - Contract Baseline (1 sprint)
2. M1 - ETL Run Persistence + Checkpoints (1-2 sprints)
3. M2 - Parsing/Transform Expansion (1-2 sprints)
4. M3 - Unified Audit Trail + Replay UX (1 sprint)
5. M4 - Hardening + Rollout (1 sprint)

## Plan

1. M0 Contract Baseline
- Create compatibility matrix across canonical events (`pkg/events`), GraphQL schema, and OpenAPI enums.
- Add CI checks to detect enum/schema drift and fail on incompatible changes.
- Resolve current known drift items (e.g., appointment/claim enum naming differences).
- Deliverable: contract conformance report + enforced CI gate.

2. M1 ETL Persistence + Checkpoints
- Add durable ETL run store (status, timestamps, bytes, error, source/version, checkpoint).
- Introduce checkpoint model per source pipeline for resumable/incremental ingestion.
- Add idempotency keys for repeated fetch/load operations.
- Deliverable: ETL run APIs/queries + checkpoint-aware re-run command path.

3. M2 Parsing/Transform Expansion
- Formalize transform schema versioning and validator.
- Expand profile-driven transform features with strict validation + non-fatal warning capture.
- Standardize warning taxonomy and propagate through event metadata and workflow results.
- Deliverable: upgraded transform engine + migration notes for existing workflows.

4. M3 Unified Audit Trail
- Define and implement audit envelope persisted per stage (parse/transform/action/store).
- Store hash/pointer strategy for raw payload artifacts; preserve correlation/source profile identifiers.
- Wire audit records to replay/diff tooling for forensic inspection.
- Deliverable: queryable audit history + replay trace view.

5. M4 Hardening + Rollout
- Expand low-coverage ETL/storage/terminology-db tests.
- Add load and fault-injection tests for checkpoint/replay and warning-heavy payloads.
- Roll out by source-profile cohorts with canary gating and rollback switches.
- Deliverable: production rollout checklist + post-rollout verification report.

## Test Plan

- Unit tests
  - ETL run repository/checkpoint logic
  - Transform schema validation and warning classification
  - Contract diff validator rules
- Integration tests
  - End-to-end parse → transform → workflow → event_store with audit assertions
  - ETL resume from checkpoint + idempotent replay
- Contract tests
  - Snapshot/compare canonical enum set vs GraphQL/OpenAPI generated artifacts
- E2E tests
  - Real sample feeds across HL7v2/EDI/CDA/CSV with tolerated anomalies and replay verification
- Performance tests
  - Baseline parse/workflow throughput before and after audit envelope additions

## Rollout / Backout

- Rollout
  - Enable contract CI gate in warning mode for 1 cycle, then enforce.
  - Deploy ETL persistence/audit writes behind feature flags.
  - Enable per-source-profile and monitor warning/error/audit-write rates.
- Backout
  - Disable new audit writes and checkpoint resume path via config flags.
  - Revert to existing stateless ETL run behavior while preserving append-only event store integrity.

## Acceptance Criteria

- Contract governance
  - CI blocks incompatible enum/schema drift across canonical, GraphQL, and OpenAPI artifacts.
- ETL
  - 100% of ETL runs produce durable run records with checkpoint + result metadata.
  - Failed runs are resumable from checkpoint when source supports incremental fetch.
- Parsing/transform
  - Warning taxonomy (`phase/code/severity/path`) consistently preserved through parser and workflow output.
- Auditability
  - For each processed event, audit lookup by correlation ID returns parse/transform/action lineage with timestamps.
- Quality
  - Coverage increases in ETL-related components (target direction aligned with status matrix gaps).

## Risks / Dependencies

- Need decision on REST runtime surface vs docs-only OpenAPI to avoid ambiguous contract ownership.
- Checkpoint semantics depend on source capabilities and loader behavior.
- Audit storage/PHI policy must be ratified before broad rollout.

## Sources

- [S1] `docs/planning/README.md:221`
- [S2] `docs/STATUS.md:39`
- [S3] `pkg/etl/pipeline.go:65`
- [S4] `cmd/fi-fhir/etl.go:593`
- [S5] `internal/parser/hl7v2/parser.go:160`
- [S6] `internal/workflow/transforms.go:56`
- [S7] `pkg/events/events.go:124`
- [S8] `api/openapi.yaml:541`
- [S9] `internal/api/graphql/schema.graphql:12`
- [S10] `pkg/eventsourcing/store.go:34`
- [S11] `internal/workflow/replay.go:57`
