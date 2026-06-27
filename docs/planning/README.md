# fi-fhir Planning Documents

This directory contains detailed planning and specification documents for the fi-fhir healthcare integration library.

## Document Overview

| Document                                         | Purpose                                                              | Status                                                                                                         |
| ------------------------------------------------ | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| [SOURCE-PROFILES.md](SOURCE-PROFILES.md)         | Source Profile configuration system - the unit of scalability        | ✅ Core + inference/lint shipped                                                                               |
| [WORKFLOW-DSL.md](WORKFLOW-DSL.md)               | Workflow routing, transforms, and actions                            | ✅ Core + action pack shipped                                                                                  |
| [FHIR-PROFILES.md](FHIR-PROFILES.md)             | FHIR R4 output with US Core mapping                                  | ✅ 17+ resources + validation shipped                                                                          |
| [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md)               | HL7 v2.x version differences and parsing edge cases                  | ✅ Core + vendor templates shipped                                                                             |
| [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md)       | X12 EDI parsing (837P, 835, 270/271, 276/277)                        | ✅ Parsing + companion guide framework shipped                                                                 |
| [IDENTIFIERS.md](IDENTIFIERS.md)                 | Patient/provider identifier systems and validation                   | ✅ Complete (validators + matching engine)                                                                     |
| [TERMINOLOGY.md](TERMINOLOGY.md)                 | Healthcare code systems and mapping (LOINC, SNOMED, UMLS, ICD-10-CM) | ✅ Core + version tracking shipped                                                                             |
| [TERMINOLOGY-MAPPING.md](TERMINOLOGY-MAPPING.md) | Mapping upload, LLM autorouting, and decision telemetry              | 🟡 Partial — upload/autoroute/review workflow shipped (GraphQL/UI + CLI); telemetry spans/analytics polish planned |
| [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md)           | TypeScript/JavaScript SDK                                            | ✅ SDK + distribution shipped                                                                                  |
| [CDA-CCDA.md](CDA-CCDA.md)                       | CDA/CCDA clinical document parsing                                   | ✅ Complete                                                                                                    |
| [FHIR-SUBSCRIPTIONS.md](FHIR-SUBSCRIPTIONS.md)   | FHIR R4 Subscriptions (bidirectional)                                | ✅ Complete                                                                                                    |
| [GRAPHQL-API.md](GRAPHQL-API.md)                 | GraphQL API layer for events                                         | ✅ Complete                                                                                                    |
| [EVENT-SOURCING.md](EVENT-SOURCING.md)           | Event sourcing / CQRS patterns                                       | ✅ Complete                                                                                                    |
| [API-CONTRACT-MATRIX.md](API-CONTRACT-MATRIX.md) | Canonical vs GraphQL vs OpenAPI event contract drift matrix          | 🟡 Generated baseline (M0)                                                                                     |

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

| Build  | Outcome                                  | Status     | Primary Docs                             |
| ------ | ---------------------------------------- | ---------- | ---------------------------------------- |
| FB-001 | Source Profile inference + linting       | ✅ Shipped | [SOURCE-PROFILES.md](SOURCE-PROFILES.md) |
| FB-002 | Workflow action pack (email/file/custom) | ✅ Shipped | [WORKFLOW-DSL.md](WORKFLOW-DSL.md)       |
| FB-003 | FHIR validation + conformance checks     | ✅ Shipped | [FHIR-PROFILES.md](FHIR-PROFILES.md)     |
| FB-004 | Terminology version tracking             | ✅ Shipped | [TERMINOLOGY.md](TERMINOLOGY.md)         |
| FB-005 | TypeScript SDK distribution              | ✅ Shipped | [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md)   |
| FB-006 | HL7 vendor templates + fixtures          | ✅ Shipped | [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md)       |

#### FB-001: Source Profile Inference + Linting

- [x] Add `fi-fhir profile infer` to generate a profile skeleton from sample messages (delimiter/version/segment/Z-segment heuristics).
- [x] Add `fi-fhir profile lint` with schema validation plus opinionated warnings (unknown segments, missing mappings, unsafe tolerances).
- [x] Add fixtures + golden outputs for inference and linting (profiles + representative `testdata/` corpus).

#### FB-002: Workflow Action Pack (Email/File/Custom)

- [x] Implement `email` action (SMTP/SES; templated subject/body; retries + circuit breaker parity with `webhook`).
- [x] Implement `file` action (write events to disk with templated paths; atomic writes; rotation/retention knobs).
- [x] Define a safe “custom action” extension point (e.g. `exec` action with allowlist + timeouts) and document it in `WORKFLOW-DSL.md`.

#### FB-003: FHIR Validation + Conformance

- [x] Add a validation path for generated FHIR resources/bundles (US Core-focused), used by CLI and workflow (`fi-fhir fhir validate`, `fhir` action `validate_fhir`).
- [x] Add golden fixtures to validate the highest-volume resources (Patient/Encounter/Observation/DiagnosticReport + bundle).
- [x] Decide and document validator strategy (pure-Go structural checks vs external validator) and failure policy (warn vs error per profile).

#### FB-004: Terminology Version Tracking

- [x] Implement version pinning in config (per code system) and plumb through mapper/loader APIs.
- [x] Add a lightweight “terminology registry/index” to track installed versions and effective dates (`terminology.releases`, `fi-fhir terminology status`, `fi-fhir terminology use`).
- [x] Add version-aware validation modes: `pass` / `warn` / `error` (and surface them as `ParseWarning`s when tolerated).

#### FB-005: TypeScript SDK Distribution

- [x] Decide packaging for the Go CLI dependency (platform-specific optional deps; `FI_FHIR_PATH` override).
- [x] Add publish-ready artifacts + CI for `sdk/typescript` (build + test jobs; npm publish on tags).
- [x] Add usage docs and integration examples (Node service, serverless function, simple ETL).

#### FB-006: HL7 Vendor Templates + Fixtures

- [x] Add vendor profile templates (Epic/Cerner/Meditech/Allscripts) with documented deviations and recommended tolerances.
- [x] Add real-world-ish fixtures (Z-segments, optionality drift, encoding/line ending variations) and map them to templates.
- [x] Add a “template selection” guide (how to fork a template into a feed-specific Source Profile).

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

✅ All current Feature Builds (FB-001..FB-006) are shipped.

Next focus areas:

- P2 test coverage gaps (especially CLI + terminology db) — tracked in [libs/fi-fhir#7](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/7)
- Targeted P3 enhancements driven by real feed drift — tracked in [libs/fi-fhir#8](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/8)
- Platform integration M1 (backend↔frontend runtime parity) — tracked in [libs/fi-fhir#9](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/9)
- Platform integration M2 (`flexinfer` inference path) — tracked in [libs/fi-fhir#10](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/10)
- Platform integration M3 (`mentatlab` orchestration + SSE) — tracked in [libs/fi-fhir#11](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/11)

### P2 - Test Coverage Gaps

Tracking issue: [libs/fi-fhir#7](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/7)

> **Last refreshed**: 2026-02-06. Run `make docs-status` for latest numbers. See `docs/STATUS.md` for full component matrix.

| Area                 | Current Coverage | Target | Notes                                                                                                                                            |
| -------------------- | ---------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| CLI (`cmd/fi-fhir/`) | 70.2%            | 80%+   | Improved with offline stubs + CI live tests (k3s MinIO/PostgreSQL via GitLab CI variables; live CLI tests run under `-tags=live` in `test:unit`) |
| Terminology (db)     | 22.6%            | 80%+   | Requires PostgreSQL testcontainers                                                                                                               |
| ✅ CDA Parser        | 90.8%            | 80%+   | ✅ Above target                                                                                                                                  |
| ✅ GraphQL Resolvers | 80.8%            | 80%+   |                                                                                                                                                  |
| ✅ FHIR Subscription | 83.8%            | 80%+   |                                                                                                                                                  |
| Terminology (pkg)    | 84.2%            | 80%+   | ✅ Core pkg above target; db/index/semantic/suggest still low                                                                                    |
| ✅ FHIR Parser       | 95.2%            | 80%+   |                                                                                                                                                  |
| Workflow Engine      | 78.9%            | 80%+   | Close to target                                                                                                                                  |

#### Terminology DB Integration Tests

`pkg/terminology/db` integration tests now run in CI through the existing
`test:integration` PostgreSQL service:

```bash
POSTGRES_TEST_URL=postgres://testuser:testpass@postgres:5432/fi_fhir_test?sslmode=disable \
  go test -tags=integration -p 1 ./pkg/terminology/db/
```

Keep this package serialized with other integration packages that reset the
`terminology` schema. The CI job runs the CLI integration tests first and then
the terminology DB package against the same service database, avoiding
testcontainers/Docker-in-Docker and avoiding unsafe concurrent schema drops.

#### P2 Next Steps (CLI Coverage)

1. Add offline tests for low-coverage CLI commands: `companion`, `serve`, `subscription *`, `config show/env`.
2. Add `-tags=live` CLI tests for untested terminology loaders: `terminology load snomed`, `terminology load icd10pcs` (minimal fixtures).
3. Add remaining ETL CLI coverage (`etl fetch/load/validate`) and projection rebuild coverage, preferring stubs first and `-tags=live` where Postgres/MinIO are required.
   - 2026-06-27: Added offline projection status formatting coverage; projection rebuild and ETL load branches remain the next highest-value CLI coverage targets.

### P3 - Future Enhancements

Tracking issue: [libs/fi-fhir#8](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/8)

- ✅ Additional HL7v2 message types (MDM, DFT) — Implemented 2026-01-16
- CDA/CCDA section expansion (Medications, Allergies, Social History) — tracked in [libs/fi-fhir#13](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/13)
- Test data organization and edge case fixtures — tracked in [libs/fi-fhir#15](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/15)
- UI enhancements: dark mode, keyboard shortcuts, bulk operations, accessibility audit — tracked in [libs/fi-fhir#16](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/16)
- ✅ ETL expansion: additional source/sink providers, scheduling, incremental sync — Implemented 2026-03-07
- ✅ LLM feature expansion: multi-model routing, prompt versioning, evaluation framework
- Terminology approval workflow (human-in-the-loop review for autoroute suggestions) — tracked in [libs/fi-fhir#17](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/17)
- Additional FHIR Implementation Guides (USCDI v3, Bulk Data, SMART App Launch) — tracked in [libs/fi-fhir#12](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/12)
- ✅ Terminology index test coverage (91.3%) — 2026-03-05
- ✅ Terminology semantic search test coverage (90.9%) — 2026-03-05
- ✅ LLM extract test coverage (94.2%, up from 76.6%) — 2026-03-05
- ✅ LLM copilot test coverage (98.0%) — 2026-03-05
- Terminology suggest test coverage (79.2%, mostly DB-dependent) — tracked in [libs/fi-fhir#7](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/7)
- Storage provider test expansion (S3/MinIO integration tests) — tracked in [libs/fi-fhir#18](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/18)

---

## Runtime Config (Proxy / Backend↔Frontend)

The UI frontend is served via nginx, which reverse-proxies API requests to the fi-fhir backend. The following assumptions are validated by `make check-runtime-config` and `test:smoke` CI.

### Proxy Routes

| Route         | Backend         | Purpose                              |
| ------------- | --------------- | ------------------------------------ |
| `/health`     | `fi-fhir serve` | Liveness/readiness probe             |
| `/graphql`    | `fi-fhir serve` | GraphQL queries+mutations            |
| `/graphql/ws` | `fi-fhir serve` | GraphQL subscriptions (WebSocket)    |
| `/ready`      | `fi-fhir serve` | Readiness passthrough for k8s probes |

### WebSocket Upgrade

The nginx config must include `Upgrade` and `Connection` header passthrough for `/graphql/ws`:

```
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

See `ui/nginx/default.conf.template` for the canonical config.

### Key Environment Variables

| Variable                     | Default   | Purpose                                      |
| ---------------------------- | --------- | -------------------------------------------- |
| `FI_FHIR_ADDR`               | `:8080`   | Server listen address                        |
| `FI_FHIR_UI_API_ORIGIN`      | —         | Backend origin for UI proxy                  |
| `FI_FHIR_DATABASE_URL`       | —         | PostgreSQL DSN (if absent, uses MemoryStore) |
| `FI_FHIR_DATABASE_HOST`      | —         | Alternative: PostgreSQL host                 |
| `FI_FHIR_DATABASE_SSL_MODE`  | `disable` | SSL mode (defaults to disable in-cluster)    |
| `FI_FHIR_TERMINOLOGY_DB_URL` | —         | Terminology database connection              |
| `FI_FHIR_CORS_ORIGINS`       | —         | Allowed CORS origins                         |
| `FI_FHIR_LLM_ENABLED`        | `false`   | Enable LLM features (autoroute, copilot)     |
| `FI_FHIR_LLM_BASE_URL`       | in-cluster LiteLLM | Canonical LLM provider base URL       |
| `FI_FHIR_LLM_API_KEY`        | —         | Canonical LLM provider API key               |
| `FI_FHIR_LLM_DEFAULT_MODEL`  | `qwen3-8b-fast` | Default LLM model                       |
| `FI_FHIR_LLM_QUALITY_MODEL`  | `qwen3-14b-quality` | Higher-quality LLM model              |

For LLM runtime configuration, `FI_FHIR_LLM_*` names are canonical. Legacy
`LLM_BASE_URL`, `LLM_API_KEY`, `LLM_DEFAULT_MODEL`, and `LLM_QUALITY_MODEL`
remain supported as fallbacks, with `OPENAI_API_KEY` as the final API-key
fallback. When both namespaces are set, `FI_FHIR_LLM_*` wins. GraphQL callers
can inspect the safe `llmCapability` query for `enabled`, `configured`,
provider host, model names, `status`, and warnings without exposing API keys or
full provider URLs.

Full env var enumeration: `make check-runtime-config`.

---

## Contributing

When adding new planning documents:

1. Follow the existing format (Overview → Details → Implementation Plan → See Also → References)
2. Include a "See Also" section linking related docs
3. Mark implementation status with checkboxes and file path references
4. Update this README with the new document
