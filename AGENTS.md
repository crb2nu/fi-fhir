# AGENTS.md - fi-fhir

Guidance for AI coding assistants working on this healthcare integration library.

## Project Overview

fi-fhir is a **format-agnostic healthcare integration library** that transforms legacy formats (HL7v2, flatfiles, EDI) into semantic events. The core innovation is **Source Profile-driven normalization** - each interface/feed gets its own profile that controls parsing behavior. Users work with business concepts (`patient_admit`, `lab_result`) rather than format-specific structures (`ADT^A01`, `OBX`).

---

## Core Architecture Principles

### 1. Profile-Driven Design
- **The unit of scalability is the Source Profile, not "HL7v2 support"**
- All parsing decisions flow through the profile configuration
- Tolerance, validation, and mapping rules are profile-configurable
- See `docs/planning/SOURCE-PROFILES.md` for full specification

### 2. Three-Phase Parsing Pipeline
```
Byte Normalization → Syntactic Parse → Semantic Extraction
      (Phase 1)         (Phase 2)          (Phase 3)
```
- Phase 1: Character encoding, line endings, preserve raw
- Phase 2: Delimiters, repetitions, escape sequences
- Phase 3: Profile-driven extraction, event classification

### 3. Warnings Over Errors
- Healthcare data is messy; don't fail on recoverable issues
- Use `ParseWarning` to record anomalies while continuing
- Check profile tolerance before returning errors:
```go
if segment == nil {
    if p.profile.IsMissingSegmentTolerated(segmentID) {
        p.addWarning("semantic", "MISSING_"+segmentID, msg, path)
        return DefaultValue{}, nil  // Don't fail
    }
    return DefaultValue{}, fmt.Errorf("%s not found", segmentID)
}
```

### 4. Identifier-First Design
- `IdentifierSet` is a first-class type for handling PID-3 repetitions
- Always validate identifiers (NPI, MBI, SSN) and record warnings
- Map assigning authorities using profile configuration
- Preserve original values before normalization

---

## Architecture

```
Input Formats          Semantic Layer           Output/Actions
─────────────         ────────────────         ──────────────
HL7v2    ──┐          ┌─────────────┐          ┌─> FHIR API
Flatfile ──┼──────────┤ Canonical   ├──────────┼─> REST Webhook
EDI X12  ──┤          │ Event Model │          ├─> Database
CDA/CCDA ──┤          └─────────────┘          └─> Message Queue
FHIR     ──┘
```

### Key Directories

| Path | Purpose |
|------|---------|
| `cmd/fi-fhir/` | CLI entry point |
| `internal/parser/hl7v2/` | HL7v2 message parsing |
| `internal/parser/csv/` | CSV/flatfile parsing |
| `internal/parser/edi/` | EDI X12 (837/835) parsing |
| `internal/parser/cda/` | CDA/CCDA clinical document parsing |
| `internal/fhir/subscription/` | FHIR R4 Subscriptions (bidirectional) |
| `internal/workflow/` | Workflow engine with CEL conditions |
| `pkg/events/` | **Public** semantic event types - the canonical model |
| `pkg/eventsourcing/` | Event store, projections, snapshots for CQRS |
| `pkg/config/` | Configuration types |
| `testdata/` | Sample messages for testing |

### Critical Files

- `pkg/events/events.go` - **THE** canonical event model. All format adapters map TO these types.
- `internal/parser/hl7v2/parser.go` - HL7v2 parsing logic, reference implementation
- `internal/parser/cda/parser.go` - CDA/CCDA XML parsing with namespace handling
- `internal/parser/cda/mapper.go` - CDA to canonical event mapping

## Build & Test

```bash
# Build CLI
go build -o bin/fi-fhir ./cmd/fi-fhir

# Run all tests
go test ./...

# Run with verbose output
go test -v ./internal/parser/hl7v2/...

# Test CLI manually
./bin/fi-fhir parse --pretty testdata/adt_a01_sample.hl7
```

## Code Conventions

### Go Style
- Follow standard Go idioms
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Table-driven tests
- Use `internal/` for private implementation, `pkg/` for public API

### HL7v2 Specifics
- MSH field numbers match array indices (MSH-9 → `fields[9]`)
- Always handle empty fields gracefully
- Preserve raw payload for audit purposes
- Test with real-world message variants (Z-segments, missing fields)

### Semantic Events
- Events are **immutable** - create new instances, don't modify
- Always populate `EventMeta` with source, format, timestamp
- Use `json.RawMessage` for `RawPayload` to preserve original data
- Event types should be self-documenting with JSON tags

## Adding a New Format Adapter

1. Create package in `internal/parser/<format>/`
2. Implement a `Parser` struct with `Parse(raw string) (interface{}, error)`
3. Map source data to appropriate `pkg/events/*Event` types
4. Add CLI support in `cmd/fi-fhir/main.go` switch statement
5. Add tests with sample files in `testdata/`

## Adding a New Event Type

1. Add constant to `EventType` in `pkg/events/events.go`
2. Create struct for the event (embed `EventMeta`)
3. Update relevant parsers to emit the new event type
4. Add tests for round-trip parsing

## Common Patterns

### Parsing HL7v2 Segments
```go
// Get segment by ID
seg := p.getSegment(msg, "PID")

// Get field safely (returns "" if missing)
field := p.getField(seg, 3)

// Get component from field (^ separated)
component := p.getComponent(field, 0)
```

### Creating Events
```go
meta := events.NewEventMeta(
    events.EventPatientAdmit,
    p.source,
    events.FormatHL7v2,
)
meta.SourceMessageID = msg.ControlID

event := &events.PatientAdmitEvent{
    EventMeta: meta,
    Patient:   patient,
    Encounter: encounter,
}
```

## Healthcare Domain Notes

### HL7v2 Message Types to Know
- **ADT** (Admit/Discharge/Transfer): Patient movement events
- **ORU** (Observation Result Unsolicited): Lab results
- **ORM** (Order Message): Lab/procedure orders
- **SIU** (Scheduling): Appointments
- **MDM** (Medical Document Management): Clinical documents

### Common Fields
- **PID**: Patient identification (name, DOB, MRN, address)
- **PV1**: Patient visit (encounter class, location, providers)
- **OBX**: Observation (lab result values)
- **OBR**: Observation request (test orders)
- **SCH**: Scheduling (appointments)

### Z-Segments
Custom segments (e.g., `ZPD`) vary by vendor. The parser extracts them but mapping is configurable.

## Roadmap Context

**Current State**: Multi-format parsing (HL7v2, CSV, EDI X12) with workflow routing operational. TypeScript SDK available. FHIR R4 output with US Core mapper implemented. FHIR action in workflow engine complete.

**Completed**:
- HL7v2 ADT messages (A01, A02, A03, A04, A08)
- HL7v2 ORU^R01 with multiple OBX support
- HL7v2 SIU messages (S12, S13, S14, S15, S26)
- Source Profile system with YAML configuration
- Terminology mapping (LOCAL to LOINC/SNOMED)
- FHIR R4 resource types and US Core mapper
- Identifier validation (SSN, NPI, MBI)
- Z-segment extraction
- HL7v2 escape sequence handling
- CLI with parse, validate, workflow, and config commands
- CSV adapter with schema inference (patient, lab result parsing)
- Workflow DSL engine (routing, log/webhook/fhir actions, dry-run)
- CEL condition evaluation in workflow filters
- Transform pipeline (set_field, map_terminology, redact)
- UUID v4 generation for event IDs
- TypeScript SDK with CLI wrapper (parse, workflow, type definitions)
- EDI X12 parser (837P claims, 835 remittance, 270/271 eligibility, envelope/loop parsing)
- FHIR action in workflow (POST Patient, Encounter, Observation, DiagnosticReport to FHIR servers)
- OAuth2 client credentials flow for FHIR action (token caching, automatic refresh)
- Database action in workflow (PostgreSQL, MySQL, SQLite; insert/upsert; field mapping)
- Queue action in workflow (driver registry pattern; topic templates; message keys)

**Completed**:
- EDI X12 276/277 claim status transactions (see `Map276ToEvents()`, `Map277ToEvents()`)
- Retry/error handling with exponential backoff (`internal/workflow/retry.go`)
- OAuth2 token refresh with 401 handling (`internal/workflow/oauth.go`)
- Circuit breaker pattern for failing external services (`internal/workflow/circuit_breaker.go`)
- Dead letter queue for failed events (`internal/workflow/dlq.go`)
- Rate limiting for high-volume event streams (`internal/workflow/ratelimit.go`)
- Metrics/observability instrumentation (`internal/workflow/metrics.go`)
- Prometheus metrics adapter (`internal/workflow/metrics_prometheus.go`)
- Distributed tracing with OpenTelemetry (`internal/workflow/tracing.go`, `tracing_otel.go`)
- Grafana dashboard templates (`dashboards/grafana/`)
- Log correlation with trace IDs (`internal/workflow/logging.go`, `logging_test.go`)
- Alerting rule templates (`dashboards/alerting/workflow-alerts.yaml`, `workflow-alerts-k8s.yaml`)
- Health check endpoints (`internal/workflow/health.go`)
- Configuration validation (`internal/workflow/validate.go`)
- Event replay/simulation tooling (`internal/workflow/replay.go`, `simulation.go`)
- Performance benchmarking (`internal/workflow/benchmark_test.go`, `benchmark_util.go`)
- Load testing utilities (`internal/workflow/loadtest.go`, `loadtest_test.go`)

**Phase 6 (Testing Infrastructure) Complete!**:
- ~~Workflow testing framework (CLI integration)~~ ✓
- ~~Performance benchmarking~~ ✓
- ~~Load testing utilities~~ ✓

**Phase 7 (Production Readiness) - Complete!**:
- ~~Configuration management~~ ✓ (`pkg/config/`, CLI `config` command)
- ~~Deployment examples~~ ✓ (Dockerfile, docker-compose, Kubernetes manifests)
- ~~Helm chart~~ ✓ (`deploy/helm/fi-fhir/` with full templating)
- ~~CI/CD pipelines~~ ✓ (GitLab CI primary, GitHub Actions mirror)
- ~~Production documentation~~ ✓ (`docs/operations/` - hardening guide, runbook)
- Production monitoring dashboards (Grafana dashboards exist in `dashboards/grafana/`)

**All Phases Complete!** 🎉

**Post-v1.0 Additions**:
- ~~API documentation~~ ✓ (`api/openapi.yaml` - OpenAPI 3.1 spec)
- ~~End-to-end tests~~ ✓ (`test/e2e/` - CLI tests, integration tests with Docker)
- ~~Example workflows~~ ✓ (`examples/workflows/` - ADT, labs, claims, appointments)
- ~~CHANGELOG~~ ✓ (`CHANGELOG.md` - release history)

**Bidirectional FHIR Subscriptions - Complete!** 🎉
- FHIR R4 Subscription client for managing subscriptions on FHIR servers
- Notification receiver (webhook server) for incoming notifications
- FHIR-to-canonical event mapper (Patient, Encounter, Observation, Appointment)
- Workflow integration for routing received events
- CLI commands: `subscription list|status|create|delete|pause|resume|serve|validate|test`
- Design document: `docs/planning/FHIR-SUBSCRIPTIONS.md`

**Event Sourcing / CQRS - Complete!** 🎉
- Event store interface with append-only semantics (`pkg/eventsourcing/store.go`)
- In-memory store for testing (`pkg/eventsourcing/memory_store.go`)
- PostgreSQL store for production (`pkg/eventsourcing/postgres_store.go`)
- Projection framework with checkpointing (`pkg/eventsourcing/projection.go`)
- Snapshot support for large projections (`pkg/eventsourcing/snapshot.go`)
- Healthcare projections (patient timeline, event statistics, active encounters)
- CLI commands: `eventstore init|stats|streams|read|append` and `projection list|status|run|rebuild`
- Design document: `docs/planning/EVENT-SOURCING.md`

**Future Enhancements**:
  1. ~~Bidirectional FHIR subscriptions~~ ✓
  2. ~~Additional format adapters (CDA, CCDA)~~ ✓
  3. ~~GraphQL API layer~~ ✓
  4. ~~Event sourcing / CQRS patterns~~ ✓
  5. ~~Workflow action for event store integration~~ ✓
  6. ~~GraphQL queries for projections~~ ✓
  7. Wire projection resolvers to actual projection stores
  8. PostgreSQL-backed snapshot store
  9. Event replay tooling for projection rebuilds

## Testing Strategy

- Unit tests for each parser function
- Integration tests with real message samples
- Edge cases: empty fields, missing segments, Z-segments
- Use `testdata/` for sample messages (not fixtures in code)

## Dependencies

Minimal external dependencies by design:
- Standard library only for core functionality
- `gopkg.in/yaml.v3` for profile configuration
- `github.com/google/cel-go` for CEL expression evaluation in workflow filters
- UUID v4 generation uses `crypto/rand` (no external dependency)

---

## AI Antipatterns to Avoid

### 1. Over-Abstraction
```go
// BAD: Unnecessary interface for single implementation
type IdentifierExtractor interface {
    Extract(field string) IdentifierSet
}

// GOOD: Direct implementation until abstraction is needed
func (p *Parser) extractIdentifiers(field, path string) events.IdentifierSet
```

### 2. God Objects
```go
// BAD: Everything in one struct
type MegaParser struct { /* 50 fields, 100 methods */ }

// GOOD: Focused structs with single responsibility
type Parser struct { ... }         // Parses messages
type ProfileRegistry struct { ... } // Manages profiles
type NPIValidator struct { ... }    // Validates NPIs
```

### 3. Premature Optimization
```go
// BAD: Complex caching before profiling
var identifierCache = sync.Map{}

// GOOD: Simple, correct code first
func (p *Parser) extractIdentifiers(field, path string) events.IdentifierSet {
    // Direct implementation - optimize only if benchmarks show need
}
```

### 4. Stringly-Typed Code
```go
// BAD: Magic strings everywhere
if eventType == "patient_admit" { ... }

// GOOD: Type-safe constants
const EventPatientAdmit EventType = "patient_admit"
if event.Type == events.EventPatientAdmit { ... }
```

### 5. Deep Nesting
```go
// BAD: Deep nesting
func process(msg *Message) error {
    if msg != nil {
        if msg.Type != "" {
            if strings.HasPrefix(msg.Type, "ADT") {
                // More nesting...
            }
        }
    }
}

// GOOD: Early returns (guard clauses)
func process(msg *Message) error {
    if msg == nil {
        return errors.New("nil message")
    }
    if msg.Type == "" {
        return errors.New("empty message type")
    }
    if !strings.HasPrefix(msg.Type, "ADT") {
        return nil
    }
    // Handle ADT...
    return nil
}
```

### 6. Excessive Comments
```go
// BAD: Obvious comments
// increment i by 1
i++

// GOOD: Comments explain "why", not "what"
// Use 80840 prefix per CMS NPI specification for Luhn check
prefixed := "80840" + npi
```

### 7. Hardcoding Business Logic
```go
// BAD: Hardcoded rules
if patientClass == "I" { return "inpatient" }

// GOOD: Profile-driven rules
classifiedType := p.profile.GetEventClassification(msgType, patientClass)
```

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `pkg/events/events.go` | Canonical event types (Patient, Encounter, IdentifierSet) |
| `pkg/profile/profile.go` | Source Profile types, registry, tolerance config |
| `pkg/validate/identifiers.go` | NPI, MBI, SSN, DEA validators with Luhn/checksum |
| `internal/parser/hl7v2/parser.go` | HL7v2 parser with profile integration |
| `internal/workflow/cel.go` | CEL evaluator with expression caching |
| `internal/workflow/engine.go` | Workflow engine with filters, transforms, actions |
| `internal/workflow/transforms.go` | Transform pipeline (set_field, map_terminology, redact) |
| `internal/workflow/oauth.go` | OAuth2 client credentials token manager with caching |
| `internal/workflow/database.go` | Database action with connection pooling and field mapping |
| `internal/workflow/queue.go` | Queue action with driver registry and topic templates |
| `internal/workflow/retry.go` | Retry with exponential backoff for HTTP actions |
| `internal/workflow/circuit_breaker.go` | Circuit breaker pattern for failing services |
| `internal/workflow/dlq.go` | Dead letter queue for failed events |
| `internal/workflow/ratelimit.go` | Token bucket rate limiter for high-volume streams |
| `internal/workflow/metrics.go` | Pluggable metrics interface with InMemory implementation |
| `internal/workflow/metrics_prometheus.go` | Prometheus adapter for metrics export |
| `internal/workflow/tracing.go` | Pluggable tracer interface with NoOp implementation |
| `internal/workflow/tracing_otel.go` | OpenTelemetry adapter for distributed tracing |
| `internal/workflow/logging.go` | Structured logger with trace correlation (trace_id, span_id) |
| `dashboards/grafana/workflow-overview.json` | Grafana dashboard for workflow monitoring |
| `dashboards/alerting/workflow-alerts.yaml` | Prometheus alerting rules (standalone) |
| `dashboards/alerting/workflow-alerts-k8s.yaml` | PrometheusRule CRD for Kubernetes |
| `internal/workflow/health.go` | Health check endpoints (/health, /ready) for Kubernetes |
| `internal/workflow/validate.go` | Workflow configuration validator with CEL/action validation |
| `internal/workflow/replay.go` | Event recording and replay for testing workflow changes |
| `internal/workflow/simulation.go` | SimulationEngine with mock actions for isolated testing |
| `internal/workflow/benchmark_test.go` | Performance benchmarks for workflow engine |
| `internal/workflow/benchmark_util.go` | Benchmark comparison and threshold validation utilities |
| `internal/workflow/loadtest.go` | Load testing runner with event generators |
| `internal/workflow/loadtest_test.go` | Load testing tests |
| `pkg/config/config.go` | Application configuration with layered loading (defaults → file → env) |
| `pkg/config/secrets.go` | Secret provider interface (env, file, vault, aws-ssm, k8s) |
| `Dockerfile` | Multi-stage Docker build with distroless base |
| `docker-compose.yaml` | Local development environment with Postgres, Kafka, FHIR server |
| `deploy/kubernetes/base/` | Kubernetes manifests (Kustomize base) |
| `deploy/kubernetes/overlays/production/` | Production Kustomize overlay |
| `deploy/helm/fi-fhir/Chart.yaml` | Helm chart metadata |
| `deploy/helm/fi-fhir/values.yaml` | Helm chart default values (config, secrets, resources) |
| `deploy/helm/fi-fhir/templates/` | Helm templates (deployment, service, ingress, hpa, pdb, servicemonitor) |
| `.gitlab-ci.yml` | GitLab CI/CD pipeline (lint, test, security, build, release) |
| `.github/workflows/ci.yaml` | GitHub Actions CI workflow (lint, test, security, docker) |
| `.github/workflows/release.yaml` | GitHub Actions release workflow (binaries, docker, helm) |
| `.golangci.yml` | golangci-lint configuration |
| `docs/operations/PRODUCTION-HARDENING.md` | Security hardening guide (HIPAA, network policies, secrets) |
| `docs/operations/RUNBOOK.md` | Operations runbook (troubleshooting, incident response) |
| `api/openapi.yaml` | OpenAPI 3.1 specification for REST API |
| `test/e2e/e2e_test.go` | E2E tests for parsing and workflow (no external deps) |
| `test/e2e/integration_test.go` | Integration tests with database, FHIR, Kafka |
| `test/e2e/docker-compose.yaml` | Docker services for integration testing |
| `examples/workflows/adt-to-fhir.yaml` | Example: ADT events to FHIR server |
| `examples/workflows/lab-results-routing.yaml` | Example: Multi-destination lab routing with alerts |
| `examples/workflows/claims-processing.yaml` | Example: EDI 837/835 claims pipeline |
| `examples/workflows/appointment-sync.yaml` | Example: Scheduling sync across systems |
| `CHANGELOG.md` | Release history and version notes |
| `docs/planning/SOURCE-PROFILES.md` | Source Profile specification |
| `docs/planning/IDENTIFIERS.md` | Patient/provider ID systems reference |
| `docs/planning/HL7V2-QUIRKS.md` | Version differences and vendor variations |
| `docs/planning/FHIR-SUBSCRIPTIONS.md` | FHIR R4 Subscriptions design document |
| `internal/fhir/subscription/client.go` | FHIR Subscription CRUD client |
| `internal/fhir/subscription/receiver.go` | Webhook notification receiver |
| `internal/fhir/subscription/mapper.go` | FHIR resource to canonical event mapper |
| `internal/fhir/subscription/router.go` | Event routing to workflow engine |
| `internal/fhir/subscription/config.go` | Subscription configuration types |
| `internal/api/graphql/schema.graphql` | GraphQL schema with queries, mutations, subscriptions |
| `internal/api/graphql/server.go` | GraphQL HTTP server with WebSocket support |
| `internal/api/graphql/resolvers/` | Query, mutation, subscription resolvers |
| `internal/api/graphql/store/store.go` | EventStore interface with MemoryStore implementation |
| `internal/api/graphql/model/` | GraphQL model types (Event interface, concrete types) |
| `docs/planning/GRAPHQL-API.md` | GraphQL API design document |
| `docs/planning/EVENT-SOURCING.md` | Event sourcing / CQRS design document |
| `pkg/eventsourcing/store.go` | EventStore interface with append-only semantics |
| `pkg/eventsourcing/memory_store.go` | In-memory event store for testing |
| `pkg/eventsourcing/projection.go` | Projection framework with checkpointing |
| `pkg/eventsourcing/projections/` | Healthcare projections (timeline, stats, encounters) |

---

## Decision Log

| Decision | Rationale |
|----------|-----------|
| Go for core | Performance, single binary, strong typing |
| Profile-driven parsing | Each feed is different; config over code |
| Warnings over errors | Healthcare data is messy; don't block flow |
| Canonical events | Decouple workflows from format specifics |
| IdentifierSet first-class | PID-3 repetition is the norm, not exception |
| Validators in pkg/validate | Reusable across parsers; clear API boundary |
