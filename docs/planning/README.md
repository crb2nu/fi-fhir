# fi-fhir Planning Documents

This directory contains detailed planning and specification documents for the fi-fhir healthcare integration library.

## Document Overview

| Document | Purpose | Status |
|----------|---------|--------|
| [SOURCE-PROFILES.md](SOURCE-PROFILES.md) | Source Profile configuration system - the unit of scalability | Core complete |
| [WORKFLOW-DSL.md](WORKFLOW-DSL.md) | Workflow routing, transforms, and actions | Complete with metrics |
| [FHIR-PROFILES.md](FHIR-PROFILES.md) | FHIR R4 output with US Core mapping | US Core mapper complete |
| [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) | HL7 v2.x version differences and parsing edge cases | Core parsing complete |
| [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) | X12 EDI parsing (837P, 835, 270/271, 276/277) | All transactions complete |
| [IDENTIFIERS.md](IDENTIFIERS.md) | Patient/provider identifier systems and validation | Validators complete |
| [TERMINOLOGY.md](TERMINOLOGY.md) | Healthcare code systems and mapping (LOINC, SNOMED) | LOINC loader complete |
| [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) | TypeScript/JavaScript SDK | SDK complete |
| [CDA-CCDA.md](CDA-CCDA.md) | CDA/CCDA clinical document parsing | Parser complete |
| [FHIR-SUBSCRIPTIONS.md](FHIR-SUBSCRIPTIONS.md) | FHIR R4 Subscriptions (bidirectional) | Complete |
| [GRAPHQL-API.md](GRAPHQL-API.md) | GraphQL API layer for events | Complete |
| [EVENT-SOURCING.md](EVENT-SOURCING.md) | Event sourcing / CQRS patterns | Core complete |

## Architecture Overview

```
Input Formats          Source Profiles         Semantic Layer          Workflow Engine
─────────────         ───────────────         ──────────────          ───────────────
                      ┌───────────────┐
HL7v2    ────────────▶│ epic_adt.yaml │──┐                                            ┌─▶ FHIR API
                      └───────────────┘  │    ┌─────────────┐     ┌─────────────┐     ├─▶ Webhook
                      ┌───────────────┐  ├───▶│  Canonical  │────▶│  Workflow   │─────┼─▶ Database
CSV      ────────────▶│ csv_import    │──┤    │   Events    │     │   Routes    │     ├─▶ Queue
                      └───────────────┘  │    └─────────────┘     └─────────────┘     └─▶ Log
                      ┌───────────────┐  │
EDI X12  ────────────▶│ edi_claims    │──┘
                      └───────────────┘
```

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

See [AGENTS.md](../../AGENTS.md) for the canonical "what's done" list and current roadmap.

### Completed
- HL7v2 parsing (ADT, ORU, SIU messages)
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
- Fuzzy terminology matching with confidence scoring (`pkg/terminology/fuzzy.go`)
- FHIR Condition resource (US Core profile) - `pkg/fhir/mapper.go:MapCondition()`
- FHIR Coverage resource (US Core profile) - `pkg/fhir/mapper.go:MapCoverage()`
- Da Vinci PAS Claim resource (for 837P → FHIR) - `pkg/fhir/mapper.go:MapClaim()`
- PDex ExplanationOfBenefit resource (for 835 → FHIR) - `pkg/fhir/mapper.go:MapExplanationOfBenefit()`
- CoverageEligibilityResponse resource (for 271 → FHIR) - `pkg/fhir/mapper.go:MapCoverageEligibilityResponse()`

### Next Up
- UMLS API integration (optional)
- Additional US Core profiles (Procedure, MedicationRequest, etc.)

## Contributing

When adding new planning documents:
1. Follow the existing format (Overview → Details → Implementation Plan → See Also → References)
2. Include a "See Also" section linking related docs
3. Mark implementation status with checkboxes and file path references
4. Update this README with the new document
