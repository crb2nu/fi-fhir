# Changelog

All notable changes to fi-fhir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Format Adapters
- CDA/CCDA clinical document parser with namespace-aware XML handling (`internal/parser/cda/`)
- CDA section handlers for structured data extraction (`internal/parser/cda/sections/`)
- CDA-to-canonical event mapper (`internal/parser/cda/mapper.go`)
- FHIR R4 resource parser for inbound FHIR ingestion (`internal/parser/fhir/`)
- HL7v2 MDM messages (T01–T11) — Medical Document Management with TXA/OBX support
- HL7v2 DFT messages (P03, P11) — Detail Financial Transactions with FT1/DG1/PR1/IN1
- HL7v2 VXU immunization messages
- HL7v2 RDE pharmacy messages
- EDI X12 270/271 eligibility inquiry and response transactions
- EDI X12 276/277 claim status inquiry and response transactions
- EDI companion guide framework with built-in payer guides (`internal/parser/edi/companion/`)
- Built-in companion guides: Medicare, BlueCross, United Healthcare (`companion/builtin/`)
- Companion guide validator and path-based field resolution

#### Event Sourcing / CQRS
- Event store interface with append-only semantics (`pkg/eventsourcing/store.go`)
- In-memory event store for testing (`pkg/eventsourcing/memory_store.go`)
- PostgreSQL event store for production (`pkg/eventsourcing/postgres_store.go`)
- Projection framework with checkpointing (`pkg/eventsourcing/projection.go`)
- Snapshot store interface with memory and PostgreSQL implementations
- Healthcare projections: patient timeline, event statistics, active encounters (`pkg/eventsourcing/projections/`)
- Event replay tooling with ProjectionRebuilder (progress, dry-run, snapshot-aware)
- Time range queries for point-in-time recovery (`pkg/eventsourcing/time_range.go`)
- Event archival and HIPAA-aware retention policies (`pkg/eventsourcing/archive.go`)
- Event stream compaction with aggregate snapshots (`pkg/eventsourcing/compaction.go`)
- Saga orchestration for multi-step transactions with compensation (`pkg/eventsourcing/saga.go`)
- Outbox pattern for reliable event publishing (`pkg/eventsourcing/outbox.go`)
- CLI commands: `eventstore init|stats|streams|read|append`, `projection list|status|run|rebuild`

#### FHIR Resources
- US Core Patient, Encounter, Observation, DiagnosticReport (enhanced)
- US Core Condition, Procedure, Immunization, MedicationRequest
- US Core AllergyIntolerance, CarePlan, Goal, CareTeam, ServiceRequest
- US Core DocumentReference, DiagnosticReport (clinical notes), Provenance
- US Core Location, Organization, Practitioner, PractitionerRole, RelatedPerson
- Observation (Vital Signs) with 8 specific US Core profiles
- Da Vinci PAS Claim resource (837P → FHIR)
- PDex ExplanationOfBenefit resource (835 → FHIR)
- CoverageEligibilityResponse resource (271 → FHIR)
- FHIR Coverage resource (US Core)
- FHIR resource validation with configurable failure policy (warn vs error per profile)
- FHIR validation golden fixtures for high-volume resources

#### GraphQL API
- GraphQL schema with queries, mutations, and subscriptions (`internal/api/graphql/schema.graphql`)
- GraphQL HTTP server with WebSocket support for real-time notifications
- Resolver implementations: event queries, workflow triggers, FHIR subscription CRUD
- Batch event submission endpoint (`submitBatch` mutation with parallel/sequential modes)
- DataLoaders for N+1 query prevention (`internal/api/graphql/dataloaders/`)
- Projection resolvers wired to event sourcing service layer
- GraphQL codegen with gqlgen and CI validation (`lint:gqlgen`)

#### Terminology System
- LOINC file loader with panel expansion (`pkg/terminology/loinc.go`)
- ICD-10-CM loader with ETL pipeline integration (`pkg/terminology/db/icd10.go`)
- RxNorm loader and cross-walk queries (`pkg/terminology/db/rxnorm.go`)
- UMLS API integration with rate limiting, caching, ticket auth (`pkg/terminology/umls.go`)
- Cross-walk queries: ICD-10 ↔ SNOMED, RxNorm ↔ NDC
- Fuzzy terminology matching with confidence scoring (`pkg/terminology/fuzzy.go`)
- Terminology version pinning and registry/index (`fi-fhir terminology status|use`)
- Version-aware validation modes: pass / warn / error
- Semantic search engine (`pkg/terminology/semantic/`)
- Suggestion engine with feedback loop (`pkg/terminology/suggest/`)
- Full-text terminology indexing (`pkg/terminology/index/`)
- Mapping file upload pipeline (`pkg/terminology/upload/`)
- Automatic terminology routing engine (`internal/terminology/autoroute/`)
- Temporal workflow activities for terminology operations (`internal/terminology/workflow/`)

#### LLM Features
- Multi-provider LLM client with retry and rate limiting (`pkg/llm/`)
- Embedding generation for semantic search (`pkg/llm/embeddings.go`)
- Natural language explanation generation (`internal/llm/explain/`)
- Structured data extraction from documents (`internal/llm/extract/`)
- Data quality analysis with scoring (`internal/llm/quality/`)
- CEL-based copilot actions (`pkg/llm/copilot/`)
- LLM-integrated workflow actions (`internal/workflow/actions_llm.go`)

#### Patient Matching
- Deterministic matching rules: SSN, MBI, MRN exact match (`pkg/matching/deterministic.go`)
- Probabilistic scoring: Jaro-Winkler, Soundex, Levenshtein (`pkg/matching/similarity.go`)
- Combined matcher with configurable thresholds (`pkg/matching/matcher.go`)
- Master Patient Index (MPI) interface with in-memory implementation (`pkg/matching/mpi.go`)
- Batch matching with blocking keys for performance

#### UI / Mapping Studio
- SvelteKit 5 frontend with feature-based architecture (`ui/src/`)
- HL7 Inspector with segment/field viewer (`ui/src/lib/features/hl7/`)
- Profile Selector and Profile Draft Panel
- Sample Inbox for test message management
- Terminology Editor with mapping browser and uploader
- Autoroute Resolver and Pending Review List
- Workflow Builder with visual route/action/transform editors
- Workflow Monitor and Dry Run Panel
- Event Stream Panel for real-time event viewing
- System Status Panel
- LLM Extraction Panel
- Generate-from-description (natural language → workflow)
- Reusable UI component library: Badge, Button, Toast, Tooltip, Tabs, etc.
- GraphQL client with subscription support
- OpenAPI-generated type-safe API client

#### Source Profiles
- `fi-fhir profile infer` — generate profile skeleton from sample messages
- `fi-fhir profile lint` — schema validation + opinionated warnings
- Vendor profile templates: Epic, Cerner, Meditech, Allscripts
- Template selection guide and feed-specific fork workflow
- Inference fixtures and golden outputs

#### Workflow Engine
- Email action (SMTP/SES; templated subject/body; retries + circuit breaker)
- File action (templated paths; atomic writes; rotation/retention)
- Exec action (allowlist + timeouts for custom scripts)
- LLM action for AI-powered workflow steps
- Event replay and simulation tooling (`internal/workflow/replay.go`, `simulation.go`)
- Performance benchmarking (`internal/workflow/benchmark_test.go`)
- Load testing utilities (`internal/workflow/loadtest.go`)

#### ETL Pipeline
- Source/sink framework with provider abstraction (`pkg/etl/source/`, `pkg/etl/sink/`)
- CLI commands: `etl fetch`, `etl load`, `etl validate`
- Storage provider abstraction (file, S3, MinIO) (`pkg/storage/`)

#### Observability
- Prometheus metrics adapter (`internal/workflow/metrics_prometheus.go`)
- OpenTelemetry distributed tracing adapter (`internal/workflow/tracing_otel.go`)
- Structured JSON logging with trace correlation (trace_id, span_id)
- Grafana dashboard templates (`dashboards/grafana/`)
- Prometheus alerting rules: standalone + Kubernetes PrometheusRule CRD (`dashboards/alerting/`)
- Health check endpoints: /health, /ready (`internal/workflow/health.go`)
- Log correlation with trace IDs (`internal/workflow/logging.go`)

#### Reliability
- Retry with exponential backoff for HTTP actions (`internal/workflow/retry.go`)
- Circuit breaker pattern for failing external services (`internal/workflow/circuit_breaker.go`)
- Dead letter queue for failed events (`internal/workflow/dlq.go`)
- Rate limiting (token bucket) for high-volume streams (`internal/workflow/ratelimit.go`)
- Configuration validation (`internal/workflow/validate.go`)

#### Testing
- End-to-end test framework with Docker Compose integration (`test/e2e/`)
- PostgreSQL integration tests with testcontainers
- CLI offline stubs + live tests (`-tags=live`)
- Performance benchmarks for workflow engine
- Load testing runner with event generators
- FHIR validation golden fixtures

#### Deployment
- Multi-stage Dockerfile with distroless base (enhanced)
- Kubernetes manifests with Kustomize overlays
- Helm chart with full templating: HPA, PDB, ServiceMonitor (`deploy/helm/fi-fhir/`)
- GitLab CI/CD pipeline: 7 lint + 5 test + 7 security + 4 build + 6 release jobs
- Harbor container registry integration with automated pushes
- UI Docker image with Nginx serving
- Cross-platform release binaries (linux/darwin/windows × amd64/arm64)
- Helm OCI + npm registry publishing on tags

#### SDK
- TypeScript SDK with CLI wrapper (`sdk/typescript/`)
- Type definitions for events, workflow, and profiles
- Platform-specific optional dependency packaging (darwin/linux/windows)
- npm publish pipeline in CI

#### Documentation
- OpenAPI 3.1 specification for REST API (`api/openapi.yaml`)
- Production hardening guide for HIPAA compliance (`docs/operations/PRODUCTION-HARDENING.md`)
- Operations runbook with troubleshooting procedures (`docs/operations/RUNBOOK.md`)
- User guide: getting started, core concepts, CLI reference, playground tutorial
- Developer guide: architecture, setup, testing, adding parsers
- Mermaid architecture diagrams (overview, parsing phases, CLI flow, UI mapping)
- Planning documents for all major features (14 design docs)
- Component status matrix (`docs/STATUS.md`)
- Documentation conventions (`docs/DOCUMENTATION-CONVENTIONS.md`)

### Fixed

- Runtime verification CI now requires the fi-fhir binary for UI, TypeScript
  SDK, and smoke consumers; waits for the configured server port; runs the
  complete SvelteKit/Vitest suite; aggregates every smoke assertion safely
  under strict shell mode; and proves GraphQL WebSocket subscription delivery
  through the production handler. npm 10.9.3 is the canonical UI package
  manager and the stale pnpm lock has been removed.

- Workflow benchmarks now replace terminal log actions with a benchmark-only
  no-op handler, parse `events/sec`, and fail when a thresholded result is
  missing; benchmark test failures now propagate through the artifact-capture
  step, and shared-x86 latency ceilings are calibrated from default-branch
  evidence so the gate measures engine performance instead of console I/O or
  silently skipped records. The calibrated benchmark job is now blocking.

### Security
- Security evidence is now enforced: govulncheck, high-confidence/high-severity
  gosec, Trivy filesystem critical/secret checks, UI and TypeScript SDK npm
  audits, pinned go-licenses policy checks, and both runtime image scans are
  required merge-request jobs with their reports retained as artifacts.
- Refreshed the UI dependency lock within declared ranges, pinned the patched
  same-major Lodash resolution required by the current GraphQL Codegen
  toolchain, moved the UI to the compatible Vite 7/Svelte plugin 6 pair, and
  upgraded the TypeScript SDK to Vitest 4.1.10; both frozen npm 10.9.3 trees now
  contain no HIGH or CRITICAL audit findings.
- Replaced the mutable full nginx UI runtime base with a digest-pinned nginx
  Alpine slim image that removes the four vulnerable unused packages; backend
  and UI images are now built and scanned before merge and reject every
  CRITICAL plus every fixed HIGH finding. Main deploys wait for those scans,
  and tagged releases retag the exact scanned artifacts instead of rebuilding
  mutable inputs. The backend Docker context now excludes UI dependencies and
  local build/tool scratch data.
- Upgraded the Go build/runtime baseline to 1.26.5 and the Go-1.26-compatible
  golangci-lint baseline to 2.12.2; govulncheck and gosec versions are now pinned.
- Event-store and database workflow actions now reject configuration-controlled
  SQL identifiers outside lowercase PostgreSQL identifiers (`[a-z_][a-z0-9_]*`,
  maximum 63 characters) and quote identifiers at direct query boundaries.
- Public PostgreSQL event, checkpoint, projection-snapshot, and stream-snapshot
  stores now quote raw unqualified table and derived index names internally;
  embedded NUL bytes and names over PostgreSQL's 63-byte limit receive a
  deterministic hash suffix.
- Non-root container execution
- Read-only root filesystem
- Secret provider interface (env, file, Vault, AWS SSM, K8s secrets)
- TLS 1.3 support
- Pod security standards (restricted)
- Network policy templates
- govulncheck + gosec in CI pipeline
- Trivy filesystem and image scanning
- Required npm audits for UI and TypeScript SDK dependencies
- Required pinned license compliance checking (go-licenses)

## [0.1.0] - 2024-01-15

### Added

#### Core Functionality
- HL7v2 message parsing (ADT A01-A04, A08, ORU R01, SIU S12-S15, S26)
- CSV/flatfile parsing with schema inference
- EDI X12 parsing (837P claims, 835 remittance, 270/271 eligibility, 276/277 status)
- Canonical semantic event model (`pkg/events/`)
- Source Profile system for per-interface configuration

#### Workflow Engine
- YAML-based workflow DSL for event routing
- CEL (Common Expression Language) filter conditions
- Transform pipeline (set_field, map_terminology, redact)
- Action types: log, webhook, fhir, database, queue
- Dry-run mode for testing workflows

#### FHIR Integration
- FHIR R4 resource generation (Patient, Encounter, Observation, DiagnosticReport)
- US Core profile mapper
- OAuth2 client credentials flow with token caching
- Automatic 401 retry with token refresh

#### Reliability Features
- Retry with exponential backoff for HTTP actions
- Circuit breaker pattern for failing external services
- Dead letter queue (DLQ) for failed events
- Rate limiting for high-volume event streams
- Event replay from DLQ or recordings

#### Observability
- Prometheus metrics (`workflow_events_processed_total`, etc.)
- OpenTelemetry distributed tracing
- Structured JSON logging with trace correlation
- Grafana dashboard templates
- Prometheus alerting rules

#### CLI
- `parse` - Parse messages (HL7v2, CSV, EDI)
- `workflow run` - Process events through workflow
- `workflow validate` - Validate workflow configuration
- `config show/validate/env/init` - Configuration management
- `version` - Version information

#### Deployment
- Multi-stage Dockerfile with distroless base
- Docker Compose for local development
- Kubernetes manifests with Kustomize overlays
- Helm chart with full templating
- GitLab CI/CD pipeline (lint, test, security, build, release)

#### SDK
- TypeScript SDK with CLI wrapper
- Type definitions for events and workflow

#### Validation
- NPI (National Provider Identifier) validation with Luhn check
- MBI (Medicare Beneficiary Identifier) validation
- SSN format validation
- DEA number validation

### Security
- Non-root container execution
- Read-only root filesystem
- Secret provider interface (env, file, Vault, AWS SSM, K8s secrets)
- TLS 1.3 support
- Pod security standards (restricted)
- Network policy templates

## Types of Changes

- `Added` for new features
- `Changed` for changes in existing functionality
- `Deprecated` for soon-to-be removed features
- `Removed` for now removed features
- `Fixed` for any bug fixes
- `Security` for vulnerability fixes

[Unreleased]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/compare/v0.1.0...main
[0.1.0]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/releases/v0.1.0
