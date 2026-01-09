# fi-fhir Planning Documents

This directory contains detailed planning and specification documents for the fi-fhir healthcare integration library.

## Document Overview

| Document | Purpose | Status |
|----------|---------|--------|
| [SOURCE-PROFILES.md](SOURCE-PROFILES.md) | Source Profile configuration system - the unit of scalability | Core complete |
| [WORKFLOW-DSL.md](WORKFLOW-DSL.md) | Workflow routing, transforms, and actions | Core + FHIR action complete |
| [FHIR-PROFILES.md](FHIR-PROFILES.md) | FHIR R4 output with US Core mapping | US Core mapper complete |
| [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) | HL7 v2.x version differences and parsing edge cases | Core parsing complete |
| [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) | X12 EDI parsing (837P claims, 835 remittance) | 837P/835 complete |
| [IDENTIFIERS.md](IDENTIFIERS.md) | Patient/provider identifier systems and validation | Validators complete |
| [TERMINOLOGY.md](TERMINOLOGY.md) | Healthcare code systems and mapping (LOINC, SNOMED) | Mapper engine complete |
| [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) | TypeScript/JavaScript SDK | SDK complete |

## Architecture Overview

```
Input Formats          Source Profiles         Semantic Layer          Workflow Engine
─────────────         ───────────────         ──────────────          ───────────────
                      ┌───────────────┐
HL7v2    ────────────▶│ epic_adt.yaml │──┐
                      └───────────────┘  │    ┌─────────────┐     ┌─────────────┐
                      ┌───────────────┐  ├───▶│  Canonical  │────▶│  Workflow   │──▶ FHIR/Webhook/DB
CSV      ────────────▶│ csv_import    │──┤    │   Events    │     │   Routes    │
                      └───────────────┘  │    └─────────────┘     └─────────────┘
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
- Filters (event type, source, conditions)
- Transforms (field mapping, terminology)
- Actions (FHIR POST, webhook, logging)

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

### In Progress
- EDI 270/271 eligibility

## Contributing

When adding new planning documents:
1. Follow the existing format (Overview → Details → Implementation Plan → See Also → References)
2. Include a "See Also" section linking related docs
3. Mark implementation status with checkboxes and file path references
4. Update this README with the new document
