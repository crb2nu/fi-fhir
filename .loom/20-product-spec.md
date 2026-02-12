# Product Spec

## Summary

Enhance fi-fhir backend ETL and format parsing/transform so data ingestion is resilient, contracts are explicitly governed across API surfaces, and every transformation/action is auditable with end-to-end lineage.

## Goals

- Improve ETL reliability and operational visibility (run tracking, checkpoints, idempotency, replayability).
- Expand profile-driven parsing/transform capabilities without regressing tolerant healthcare parsing behavior.
- Enforce robust API contracts across canonical model, GraphQL, and OpenAPI artifacts.
- Provide complete auditability from raw payload intake through workflow actions and event-store persistence.

## Non-Goals

- Rewriting the platform into a separate ingestion service.
- Changing canonical business semantics (event type meaning) without compatibility process.
- Replacing GraphQL runtime with a different API stack in this phase.

## Users / Stakeholders

- Integration engineers (feed onboarding, parser tuning)
- Platform/backend engineers (API stability, ETL operations)
- Compliance and operations teams (audit trails, incident forensics)
- Downstream consumers (workflow, FHIR, queue/db sinks)

## Requirements

### Functional

- ETL orchestration
  - Persist ETL run records with lifecycle states, source/version, checkpoint position, and failure cause.
  - Support deterministic re-run and replay from checkpoint.
  - Add incremental sync hooks for eligible sources.
- Parsing and transform
  - Continue profile-driven tolerance behavior (`missing_segments`, delimiter quirks, identifier handling).
  - Add transform governance (explicit transform schema, versioning, validation).
  - Standardize warning taxonomy (`phase/code/severity/path`) and preserve across pipeline stages.
- API contracts
  - Define canonical contract source-of-truth and generated artifacts.
  - Add compatibility checks that fail CI on enum/schema drift across canonical events, GraphQL, and OpenAPI.
  - Decide and document REST `/api/v1/*` runtime support vs documentation-only status.
- Auditability
  - Introduce audit envelope for parse/transform/workflow stages with correlation ID, source profile, timestamp, actor/system, and immutable event references.
  - Persist raw payload hash + optional encrypted payload pointer.
  - Provide queryable audit trail for replay and compliance investigations.

### Non-Functional

- Reliability: no data loss for accepted messages; idempotent replay for audited workflows.
- Security/Compliance: PHI-safe storage and access controls for raw payload/audit records.
- Performance: maintain current parse/workflow throughput envelopes while adding audit metadata.
- Operability: metrics/logging/tracing emitted for each ETL and workflow stage.

## UX / Flows

- Feed onboarding flow
  1. Create/update Source Profile.
  2. Validate profile + sample corpus.
  3. Promote with contract compatibility checks.
- Runtime ingestion flow
  1. Receive raw payload.
  2. Parse with warnings.
  3. Apply transforms.
  4. Route actions.
  5. Append immutable audit + event records.
- Incident/replay flow
  1. Query audit trail by correlation/source/time/event type.
  2. Inspect warning/action history.
  3. Replay from chosen checkpoint with diff report.

## Data / APIs

- Canonical data model remains `pkg/events`-based and continues to carry `ParseWarnings`, `SourceProfileID`, and `RawPayload` metadata.
- GraphQL remains runtime API baseline; contract tests ensure enum/object parity with canonical model.
- OpenAPI artifact must be either:
  - generated from runtime handlers/contracts, or
  - clearly marked documentation-only with explicit compatibility policy.

## Rollout / Migration

- Phase 1: contract governance + drift detection in CI (no runtime breaking changes).
- Phase 2: ETL run persistence/checkpoints and audit envelope write path.
- Phase 3: parser/transform enhancements and expanded replay tooling.
- Phase 4: staged production rollout by source-profile cohort, with canary + rollback.

## Observability

- Logs: structured logs with `trace_id`/`span_id`, correlation ID, source profile ID, event ID.
- Metrics: ETL run counts/durations/errors, warning rates by code/phase, contract validation failures, replay stats.
- Traces: parse/transform/action spans with stage-level outcome tags.

## Risks

- Contract alignment work could surface latent client dependencies on inconsistent enums.
- Expanded audit storage may raise retention and PHI management costs.
- ETL checkpointing complexity may vary by source capabilities.

## Open Questions

- Confirm authoritative external API surface: GraphQL-only runtime, or GraphQL + REST `/api/v1/*`.
- Define required retention windows and encryption/key-management standards for audit payload artifacts.
- Define minimum backward-compatibility window for event schema changes.

## Sources

- [S1] `pkg/events/events.go:95`
- [S2] `pkg/events/events.go:124`
- [S3] `internal/parser/hl7v2/parser.go:1215`
- [S4] `pkg/profile/profile.go:59`
- [S5] `internal/workflow/transforms.go:56`
- [S6] `api/openapi.yaml:486`
- [S7] `internal/api/graphql/schema.graphql:12`
- [S8] `internal/api/graphql/server.go:126`
- [S9] `pkg/eventsourcing/store.go:2`
- [S10] `internal/workflow/replay.go:57`
- [S11] `docs/STATUS.md:39`
