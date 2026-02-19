# Research Brief

## Problem

Define a practical roadmap to strengthen fi-fhir backend ETL and parsing/transform capabilities while making API contracts verifiable and audit trails complete.

## Questions

- Q1: Where are the highest-impact gaps in ETL + parsing/transform today?
- Q2: Where are contract risks between canonical model and external API surfaces?
- Q3: What auditability primitives already exist, and what is missing for full lineage?

## Constraints

- Preserve profile-driven architecture as the scaling unit (`SourceProfile`).
- Maintain warning-tolerant parsing semantics for messy healthcare feeds.
- Build incrementally on existing GraphQL/workflow/event-sourcing architecture.

## Method

- Initialized Loom context pack and generated workspace snapshot.
- Inspected planning/status docs, parser/profile code, ETL pipeline/CLI, API schemas, workflow transforms, replay/event-store, and logging primitives.
- Compared OpenAPI, GraphQL, and canonical event enums for contract drift.

## Findings

1. ETL foundation exists but is terminology-centric and low coverage.
- ETL provides pluggable `Source`/`Sink`, run metadata (`PipelineRun`), streaming transfer, retry knobs, and CLI orchestration (`sync/fetch/load/status/validate`) (`pkg/etl/pipeline.go:13`, `pkg/etl/pipeline.go:65`, `cmd/fi-fhir/etl.go:39`, `cmd/fi-fhir/etl.go:593`).
- Status matrix flags ETL/storage/terminology-db as lower maturity or coverage hotspots (ETL 41.7%, storage 31.6%, terminology-db 22.6%) (`docs/STATUS.md:39`, `docs/STATUS.md:40`, `docs/STATUS.md:42`).

2. Parser and transform subsystems already support profile-driven tolerance and extensible transforms.
- HL7 parser records non-fatal warnings and writes them into event metadata (`internal/parser/hl7v2/parser.go:118`, `internal/parser/hl7v2/parser.go:160`, `internal/parser/hl7v2/parser.go:190`).
- Missing segments can be tolerated by profile with warning instead of hard failure (`internal/parser/hl7v2/parser.go:1215`, `pkg/profile/profile.go:330`).
- Transform pipeline supports `set_field`, `map_terminology`, `redact`, and contextual `explain_warnings` (`internal/workflow/transforms.go:56`, `internal/workflow/transforms.go:72`, `internal/workflow/transforms.go:78`, `internal/workflow/transforms.go:96`).

3. API contract drift risk is real across OpenAPI vs GraphQL vs canonical events.
- Canonical event constants include `appointment_scheduled` and `claim_adjudicated` (`pkg/events/events.go:25`, `pkg/events/events.go:39`).
- GraphQL enum aligns with those semantics (`internal/api/graphql/schema.graphql:19`, `internal/api/graphql/schema.graphql:23`).
- OpenAPI event enum includes different values such as `appointment_booked` and `claim_response` (`api/openapi.yaml:547`, `api/openapi.yaml:550`).
- Runtime server path is GraphQL-focused (`/graphql`, `/graphql/ws`, `/health`) (`internal/api/graphql/server.go:126`, `internal/api/graphql/server.go:130`, `internal/api/graphql/server.go:140`), and CLI `serve` help documents GraphQL endpoints, not `/api/v1/*` (`cmd/fi-fhir/main.go:4876`, `cmd/fi-fhir/main.go:4905`).
- Command check for runtime `/api/v1/parse|/api/v1/workflow` handlers returned no matches: `rg -n "/api/v1/parse|/api/v1/workflow" cmd internal`.

4. Strong auditability building blocks exist, but lineage is not unified end-to-end.
- Canonical events include `ParseWarnings`, `SourceProfileID`, and widespread `RawPayload` fields (`pkg/events/events.go:115`, `pkg/events/events.go:124`, `pkg/events/events.go:680`).
- Event sourcing is append-only with immutable `StoredEvent` metadata/timestamp and explicitly positioned as audit trail infrastructure (`pkg/eventsourcing/store.go:2`, `pkg/eventsourcing/store.go:34`, `pkg/eventsourcing/store.go:98`, `pkg/eventsourcing/store.go:101`).
- Workflow supports recording/replay and event_store append action (`internal/workflow/replay.go:15`, `internal/workflow/replay.go:57`, `internal/workflow/event_store.go:169`).
- Structured logs already support trace/span correlation (`internal/workflow/logging.go:15`, `internal/workflow/logging.go:91`, `internal/workflow/logging.go:165`).

## Options

### Option A: Incremental hardening on current architecture (recommended)

- Pros:
  - Leverages shipped parser/workflow/event-store primitives.
  - Fastest path to contract governance and auditable lineage.
  - Limits migration risk for existing feeds and workflows.
- Cons:
  - Requires disciplined compatibility checks and schema governance work.
  - ETL runtime concerns (scheduling/checkpoints) need explicit new components.
- Risks:
  - Partial implementation could leave contract drift unresolved.

### Option B: New dedicated ingestion platform/service rewrite

- Pros:
  - Clean-slate design for ETL, contracts, and audit in one model.
  - Could standardize ingestion patterns earlier.
- Cons:
  - Duplicates mature capabilities already in repo.
  - High migration and delivery risk; slower customer impact.
- Risks:
  - Divergent behavior between legacy and rewritten paths during migration.

## Recommendation

Pursue Option A with a contract-first program:
1. Establish canonical contract governance and compatibility tests immediately.
2. Add ETL run persistence/checkpointing + lineage envelope.
3. Expand parser/transform capabilities with profile-driven rules and warning taxonomy.
4. Normalize audit trail across parse → transform → route → store with correlation IDs and immutable records.

## Sources

- [S1] `docs/planning/README.md:220`
- [S2] `docs/STATUS.md:21`
- [S3] `docs/STATUS.md:39`
- [S4] `pkg/etl/pipeline.go:13`
- [S5] `pkg/etl/pipeline.go:65`
- [S6] `cmd/fi-fhir/etl.go:39`
- [S7] `internal/parser/hl7v2/parser.go:118`
- [S8] `internal/parser/hl7v2/parser.go:1215`
- [S9] `pkg/profile/profile.go:330`
- [S10] `internal/workflow/transforms.go:56`
- [S11] `pkg/events/events.go:25`
- [S12] `internal/api/graphql/schema.graphql:19`
- [S13] `api/openapi.yaml:547`
- [S14] `internal/api/graphql/server.go:126`
- [S15] `cmd/fi-fhir/main.go:4876`
- [S16] Command output: `rg -n "/api/v1/parse|/api/v1/workflow" cmd internal` (no matches)
- [S17] `pkg/events/events.go:124`
- [S18] `pkg/eventsourcing/store.go:2`
- [S19] `internal/workflow/event_store.go:169`
- [S20] `internal/workflow/logging.go:15`
