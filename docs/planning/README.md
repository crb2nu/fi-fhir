# fi-fhir Planning Documents

This directory contains detailed planning and specification documents for the fi-fhir healthcare integration library.

## Document Overview

| Document | Purpose | Status |
|----------|---------|--------|
| [SOURCE-PROFILES.md](SOURCE-PROFILES.md) | Source Profile configuration system - the unit of scalability | ✅ Core + inference/lint shipped |
| [WORKFLOW-DSL.md](WORKFLOW-DSL.md) | Workflow routing, transforms, and actions | ⚠️ Core complete, email/file/custom actions pending |
| [FHIR-PROFILES.md](FHIR-PROFILES.md) | FHIR R4 output with US Core mapping | ✅ 17+ resources + validation shipped |
| [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) | HL7 v2.x version differences and parsing edge cases | ⚠️ Core complete, vendor templates pending |
| [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) | X12 EDI parsing (837P, 835, 270/271, 276/277) | ✅ Parsing + companion guide framework shipped |
| [IDENTIFIERS.md](IDENTIFIERS.md) | Patient/provider identifier systems and validation | ✅ Complete (validators + matching engine) |
| [TERMINOLOGY.md](TERMINOLOGY.md) | Healthcare code systems and mapping (LOINC, SNOMED, UMLS, ICD-10-CM) | ⚠️ Core complete, version tracking pending |
| [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) | TypeScript/JavaScript SDK | ⚠️ SDK complete, distribution pending |
| [CDA-CCDA.md](CDA-CCDA.md) | CDA/CCDA clinical document parsing | ✅ Complete |
| [FHIR-SUBSCRIPTIONS.md](FHIR-SUBSCRIPTIONS.md) | FHIR R4 Subscriptions (bidirectional) | ✅ Complete |
| [GRAPHQL-API.md](GRAPHQL-API.md) | GraphQL API layer for events | ✅ Complete |
| [EVENT-SOURCING.md](EVENT-SOURCING.md) | Event sourcing / CQRS patterns | ✅ Complete |

## Architecture Overview

![Architecture Overview](../mermaid/overview-flow.svg)

![Parsing Phases](../mermaid/parsing-phases.svg)

See also [Architecture Diagrams](../diagrams/README.md) for generated package and call-graph diagrams.

## Reading Order

For new contributors, recommended reading order:

1. **[SOURCE-PROFILES.md](SOURCE-PROFILES.md)** - Understand the core abstraction (profile-per-feed)
2. **[HL7V2-QUIRKS.md](HL7V2-QUIRKS.md)** - Learn why profiles are necessary (real-world variations)
3. **[IDENTIFIERS.md](IDENTIFIERS.md)** - Understand healthcare identifier complexity
4. **[TERMINOLOGY.md](TERMINOLOGY.md)** - Learn about code system mapping
5. **[WORKFLOW-DSL.md](WORKFLOW-DSL.md)** - See how events get routed to actions
6. **[FHIR-PROFILES.md](FHIR-PROFILES.md)** - Understand FHIR output requirements
7. **[EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md)** - Deep dive on claims processing
8. **[TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md)** - SDK for JavaScript/TypeScript usage

## Key Concepts

### Source Profiles
The unit of scalability is a **Source Profile** (per interface/feed), not "HL7v2 support" in general. Each profile controls:
- Parsing tolerance (missing segments, extra components)
- Z-segment extraction and mapping
- Identifier normalization and validation
- Terminology mapping rules
- Event classification (A01 → inpatient vs outpatient)

### Canonical Events
All input formats map to semantic events (`patient_admit`, `lab_result`, `claim_submitted`). This decouples:
- Input parsing from business logic
- Workflow routing from format specifics
- FHIR generation from source system

### Workflow Engine
Events flow through configurable routes with:
- Filters (event type, source, CEL conditions)
- Transforms (set_field, map_terminology, redact)
- Actions (FHIR, webhook, database, queue, log)

## Implementation Status

The backlog of remaining work is documented in the [Backlog section](#backlog-prioritized) below.
For AI assistant guidance, see [AGENTS.md](../../AGENTS.md).

### Feature Builds (Roadmap)

These are the remaining “big rocks” referenced by the Document Overview statuses above.

| Build | Outcome | Status | Primary Docs |
|-------|---------|--------|--------------|
| FB-001 | Source Profile inference + linting | ✅ Shipped | [SOURCE-PROFILES.md](SOURCE-PROFILES.md) |
| FB-002 | Workflow action pack (email/file/custom) | 🚧 In progress | [WORKFLOW-DSL.md](WORKFLOW-DSL.md) |
| FB-003 | FHIR validation + conformance checks | ✅ Shipped | [FHIR-PROFILES.md](FHIR-PROFILES.md) |
| FB-004 | Terminology version tracking | ⏳ Planned | [TERMINOLOGY.md](TERMINOLOGY.md) |
| FB-005 | TypeScript SDK distribution | ⏳ Planned | [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) |
| FB-006 | HL7 vendor templates + fixtures | ⏳ Planned | [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) |

#### FB-001: Source Profile Inference + Linting
- [x] Add `fi-fhir profile infer` to generate a profile skeleton from sample messages (delimiter/version/segment/Z-segment heuristics).
- [x] Add `fi-fhir profile lint` with schema validation plus opinionated warnings (unknown segments, missing mappings, unsafe tolerances).
- [x] Add fixtures + golden outputs for inference and linting (profiles + representative `testdata/` corpus).

#### FB-002: Workflow Action Pack (Email/File/Custom)
- [ ] Implement `email` action (SMTP/SES; templated subject/body; retries + circuit breaker parity with `webhook`).
- [x] Implement `file` action (write events to disk with templated paths; atomic writes; rotation/retention knobs).
- [ ] Define a safe “custom action” extension point (e.g. `exec` action with allowlist + timeouts) and document it in `WORKFLOW-DSL.md`.

#### FB-003: FHIR Validation + Conformance
- [x] Add a validation path for generated FHIR resources/bundles (US Core-focused), used by CLI and workflow (`fi-fhir fhir validate`, `fhir` action `validate_fhir`).
- [x] Add golden fixtures to validate the highest-volume resources (Patient/Encounter/Observation/DiagnosticReport + bundle).
- [x] Decide and document validator strategy (pure-Go structural checks vs external validator) and failure policy (warn vs error per profile).

#### FB-004: Terminology Version Tracking
- [ ] Implement version pinning in config (per code system) and plumb through mapper/loader APIs.
- [ ] Add a lightweight “terminology registry/index” to track installed versions and effective dates.
- [ ] Add version-aware validation modes: `pass` / `warn` / `error` (and surface them as `ParseWarning`s when tolerated).

#### FB-005: TypeScript SDK Distribution
- [ ] Decide packaging for the Go CLI dependency (download prebuilt binaries, user-provided path, or container-based runner).
- [ ] Add publish-ready artifacts + CI for `sdk/typescript` (build, test, provenance, changelog/versioning).
- [ ] Add usage docs and integration examples (Node service, serverless function, simple ETL).

#### FB-006: HL7 Vendor Templates + Fixtures
- [ ] Add vendor profile templates (Epic/Cerner/Meditech/Allscripts) with documented deviations and recommended tolerances.
- [ ] Add real-world-ish fixtures (Z-segments, optionality drift, encoding/line ending variations) and map them to templates.
- [ ] Add a “template selection” guide (how to fork a template into a feed-specific Source Profile).

### Completed (Highlights)

- Multi-format parsing (HL7v2, CSV, EDI X12, CDA/CCDA) into canonical events
- Workflow engine (filters/transforms/actions) with observability, retry, and DLQ support
- FHIR R4 mapping (US Core-focused), event sourcing, and GraphQL API layer

<details>
<summary>Full shipped feature list</summary>

- HL7v2 parsing (ADT, ORU, SIU, MDM, DFT messages)
- CSV parsing with schema inference
- EDI X12 parsing (837P, 835)
- Source Profile system
- Identifier validators (NPI, MBI, SSN, DEA)
- Terminology mapper
- Workflow engine with FHIR action
- CEL condition evaluation for workflow filters
- Transform pipeline (set_field, map_terminology, redact)
- TypeScript SDK
- OAuth2 client credentials for FHIR action
- Database action (PostgreSQL, MySQL, SQLite)
- Queue action (Kafka, RabbitMQ, NATS, SQS)
- EDI 270/271 eligibility transactions
- EDI 276/277 claim status transactions
- Retry/error handling with exponential backoff
- OAuth2 token refresh with 401 handling
- Circuit breaker pattern for failing external services
- Dead letter queue for failed events
- Rate limiting for high-volume event streams
- Metrics/observability instrumentation
- Prometheus metrics adapter (reference implementation)
- Distributed tracing with OpenTelemetry
- Grafana dashboard templates (`dashboards/grafana/`)
- Event sourcing / CQRS patterns (store, projections, CLI)
- Workflow action for event store integration
- GraphQL queries for projections
- Projection resolver wiring (GraphQL → projection service layer)
- PostgreSQL-backed snapshot store for projection recovery
- Event replay tooling (ProjectionRebuilder with progress, dry-run, snapshot-aware)
- Batch event submission endpoint (submitBatch mutation with parallel/sequential modes)
- PostgreSQL integration tests (testcontainers for EventStore, CheckpointStore, SnapshotStore)
- Projection rebuild from time range (TimeRangeEventStore interface, point-in-time recovery)
- Event archival and retention policies (HIPAA-aware retention, file archive, deletion)
- Event stream compaction (aggregate snapshots, incremental compaction, bulk prefix compaction)
- Saga orchestration (multi-step transactions with compensation, retry, timeout)
- Outbox pattern for reliable event publishing (OutboxStore, OutboxRelay, OutboxEventStore)
- LOINC file loader with panel expansion (`pkg/terminology/loinc.go`)
- ICD-10-CM loader with ETL pipeline integration (`pkg/terminology/db/icd10.go`)
- Fuzzy terminology matching with confidence scoring (`pkg/terminology/fuzzy.go`)
- FHIR Condition resource (US Core profile) - `pkg/fhir/mapper.go:MapCondition()`
- FHIR Coverage resource (US Core profile) - `pkg/fhir/mapper.go:MapCoverage()`
- Da Vinci PAS Claim resource (for 837P → FHIR) - `pkg/fhir/mapper.go:MapClaim()`
- PDex ExplanationOfBenefit resource (for 835 → FHIR) - `pkg/fhir/mapper.go:MapExplanationOfBenefit()`
- CoverageEligibilityResponse resource (for 271 → FHIR) - `pkg/fhir/mapper.go:MapCoverageEligibilityResponse()`
- FHIR Procedure resource (US Core profile) - `pkg/fhir/mapper.go:MapProcedure()`
- FHIR Immunization resource (US Core profile) - `pkg/fhir/mapper.go:MapImmunization()`
- FHIR Observation (Vital Signs) - `pkg/fhir/mapper.go:MapVitalSign()` (8 specific US Core profiles)
- FHIR MedicationRequest resource (US Core profile) - `pkg/fhir/mapper.go:MapMedicationRequest()`
- FHIR AllergyIntolerance resource (US Core profile) - `pkg/fhir/mapper.go:MapAllergyIntolerance()`
- FHIR CarePlan resource (US Core profile) - `pkg/fhir/mapper.go:MapCarePlan()`
- FHIR Goal resource (US Core profile) - `pkg/fhir/mapper.go:MapGoal()`
- FHIR CareTeam resource (US Core profile) - `pkg/fhir/mapper.go:MapCareTeam()`
- FHIR ServiceRequest resource (US Core profile) - `pkg/fhir/mapper.go:MapServiceRequest()`
- FHIR DocumentReference resource (US Core profile) - `pkg/fhir/mapper.go:MapDocumentReference()`
- FHIR DiagnosticReport (clinical notes) resource (US Core profile) - `pkg/fhir/mapper.go:MapDiagnosticReportNote()`
- FHIR Provenance resource (US Core profile) - `pkg/fhir/mapper.go:MapProvenance()`
- FHIR Location resource (US Core profile) - `pkg/fhir/mapper.go:MapLocation()`
- FHIR Organization resource (US Core profile) - `pkg/fhir/mapper.go:MapOrganization()`
- FHIR Practitioner resource (US Core profile) - `pkg/fhir/mapper.go:MapPractitioner()`
- FHIR PractitionerRole resource (US Core profile) - `pkg/fhir/mapper.go:MapPractitionerRole()`
- FHIR RelatedPerson resource (US Core profile) - `pkg/fhir/mapper.go:MapRelatedPerson()`
- UMLS API integration - `pkg/terminology/umls.go:UMLSClient`
  - Cross-walk queries (ICD-10 ↔ SNOMED, RxNorm ↔ NDC)
  - Concept normalization and search
  - Rate limiting and caching
  - Ticket-based authentication
- GraphQL triggerWorkflow mutation - `internal/api/graphql/resolvers/schema.resolvers.go`
- GraphQL FHIR subscription CRUD mutations - `internal/api/graphql/resolvers/schema.resolvers.go`
- GraphQL workflow event notifications (pub/sub) - `internal/api/graphql/resolvers/resolver.go`
- CEL expression evaluation in FHIR subscription mapper - `internal/fhir/subscription/mapper.go`
- OAuth2 client credentials for FHIR subscriptions - `internal/fhir/subscription/router.go`
- Patient Matching Engine - `pkg/matching/`
  - Deterministic matching rules (SSN, MBI, MRN exact match)
  - Probabilistic scoring (Jaro-Winkler, Soundex, Levenshtein)
  - Combined matcher with configurable thresholds
  - MPI interface abstraction with in-memory implementation
  - Batch matching with blocking keys

</details>

---

## Backlog (Prioritized)

The following items remain for full production readiness:

### P0 - Release Blockers

✅ No P0 items currently tracked.

### P1 - Feature Builds

Work these in order unless a customer/production need pulls something forward:

1. FB-003: FHIR validation + conformance checks
2. FB-002: Workflow action pack (email/file/custom)
3. FB-004: Terminology version tracking
4. FB-006: HL7 vendor templates + fixtures
5. FB-005: TypeScript SDK distribution

### P2 - Test Coverage Gaps

| Area | Current Coverage | Target | Notes |
|------|------------------|--------|-------|
| CLI (`cmd/fi-fhir/`) | 68.8% | 80%+ | Improved with offline stubs + CI live tests (k3s MinIO/PostgreSQL via GitLab CI variables; live CLI tests run under `-tags=live` in `test:unit`) |
| Terminology (db) | 13.5% | 80%+ | Requires PostgreSQL testcontainers |
| CDA Parser | 87.1% | 80%+ | ✅ Above target |
| ✅ GraphQL Resolvers | 80.8% | 80%+ | |
| ✅ FHIR Subscription | 84.7% | 80%+ | |
| Terminology (pkg) | 66.9% | 80%+ | Core pkg good, db layer needs work |
| ✅ FHIR Parser | 92.5% | 80%+ | |
| Workflow Engine | 79.5% | 80%+ | |

#### P2 Next Steps (CLI Coverage)

1. Add offline tests for low-coverage CLI commands: `companion`, `serve`, `subscription *`, `config show/env`.
2. Add `-tags=live` CLI tests for untested terminology loaders: `terminology load snomed`, `terminology load icd10pcs` (minimal fixtures).
3. Add remaining ETL CLI coverage (`etl fetch/load/validate`) and projection rebuild coverage, preferring stubs first and `-tags=live` where Postgres/MinIO are required.

### P3 - Future Enhancements

- ✅ Additional HL7v2 message types (MDM, DFT) - Implemented 2026-01-16
- CDA/CCDA section expansion (Medications, Allergies, Social History)
- Test data organization and edge case fixtures

---

## Contributing

When adding new planning documents:
1. Follow the existing format (Overview → Details → Implementation Plan → See Also → References)
2. Include a "See Also" section linking related docs
3. Mark implementation status with checkboxes and file path references
4. Update this README with the new document
