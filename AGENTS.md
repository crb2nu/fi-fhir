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
- **MDM** (Medical Document Management): Clinical documents (T01-T11 variants)
- **DFT** (Detail Financial Transaction): Billing/charge postings (P03, P11)

### Common Fields
- **PID**: Patient identification (name, DOB, MRN, address)
- **PV1**: Patient visit (encounter class, location, providers)
- **OBX**: Observation (lab result values, document content in MDM)
- **OBR**: Observation request (test orders)
- **SCH**: Scheduling (appointments)
- **TXA**: Transcription document header (MDM document metadata)
- **FT1**: Financial transaction (charges, credits)
- **DG1**: Diagnosis information
- **PR1**: Procedure information
- **IN1**: Insurance information

### Z-Segments
Custom segments (e.g., `ZPD`) vary by vendor. The parser extracts them but mapping is configurable.

## Roadmap Context

**Current State**: fi-fhir is a production-capable healthcare integration platform with 35+ components spanning multi-format parsing, FHIR R4 mapping, event sourcing, workflow orchestration, terminology management, LLM-powered features, and a full UI. All core phases (1–7) and Feature Builds (FB-001–FB-006) are shipped.

**Major Capability Areas**:
- **Format Adapters** — HL7v2 (ADT/ORU/SIU/MDM/DFT), CSV, EDI X12 (837/835/270/271/276/277 + companion guides), CDA/CCDA, FHIR R4
- **FHIR Output** — 24+ US Core resource mappers, validation, Da Vinci PAS, PDex EOB
- **Event Sourcing** — Append-only store (Postgres), projections, snapshots, sagas, outbox, archival
- **GraphQL API** — Schema with queries/mutations/subscriptions, WebSocket, dataloaders, batch submission
- **Workflow Engine** — CEL filters, transform pipeline, 7 action types (webhook/FHIR/DB/queue/email/file/exec), DLQ, circuit breaker, replay/simulation
- **Terminology** — LOINC/ICD-10/RxNorm/UMLS, fuzzy matching, semantic search, autoroute engine, version tracking
- **LLM Integration** — Multi-provider client, explain/extract/quality analyzers, copilot, LLM workflow actions
- **Patient Matching** — Deterministic + probabilistic scoring, MPI interface, batch matching
- **UI / Mapping Studio** — SvelteKit 5 with HL7 inspector, workflow builder, terminology editor, event streaming
- **Observability** — Prometheus, OpenTelemetry, Grafana dashboards, alerting rules, health checks

**Active Work** — See `docs/planning/README.md` for prioritized backlog (P2 coverage gaps + P3 enhancements).

**Component Details** — See `docs/STATUS.md` for per-component maturity, test coverage, and freshness.

## Migration authoring

Six forward-only migration ledgers exist — submission, session, lifecycle,
batch, destination (`internal/integration/*/migrations/`), and terminology
(`pkg/terminology/db/schema.go`). None has a down path. Three rules, all of
which cost something real when broken.

### 1. A new `NOT NULL` column on an existing table carries a `DEFAULT`

Otherwise the migration breaks one-version rollback.

During a rolling upgrade both binaries run against the migrated schema at the
same time, and after a rollback the older binary runs against it indefinitely.
That binary's `INSERT` does not name the new column, so without a server-side
`DEFAULT` it dies on `SQLSTATE 23502`. This is not hypothetical: slice 4.1d C1's
`0004_export_attribution.sql` made three columns `NOT NULL` with no `DEFAULT`,
and every export from an N-1 replica failed until slice 4.4a's
`0006_export_attribution_defaults.sql` repaired it.

Choose a default that makes the older binary's row **visibly incomplete rather
than impossible**. Do not invent a plausible value — that is retroactive
vouching. `0006` reuses the same `unattributed_legacy_export` sentinel `0004`
already backfills historical rows with, so one predicate finds both classes.

`TestMigrationRule_NotNullOnExistingColumnCarriesADefault`
(`internal/integration/migrationcompat`, no database required, runs in
`test:unit`) enforces this mechanically. A column that genuinely cannot carry a
default goes in that test's `knownRollbackUnsafeColumns` with a dated reason and
a decision in `.loom/40-decisions.md`. Do not delete the test.

### 2. Take the advisory transaction lock, and re-read the version inside it

Every migrator begins a transaction, takes `pg_advisory_xact_lock` on its own
key, and only then reads its ledger version. Reading the version outside the
lock leaves the race intact — two replicas both observe "not applied" before
either acquires the lock. `pkg/terminology/db` did exactly that until slice
4.4a, and two replicas starting together against a fresh database raced to
`duplicate key value violates unique constraint "pg_namespace_nspname_index"`.
`IF NOT EXISTS` is not atomic and does not substitute for the lock.

Lock keys share one global namespace, so a new ledger picks a value distinct
from every existing `*MigrationLockKey`.

### 3. The migration number is settled by the ledger at rebase, not by a claim

Re-verify against `origin/main`'s `migrations/` directories on **every** rebase.
Two lanes claiming the same number in a planning document is normal; two lanes
merging it is a corruption.

When you add a migration, bump the owning package's exported `SchemaVersion`.
`fi-fhir version` and the `fi_fhir_schema_ledger_version` metric report it, and
it is the boundary an operator uses to decide whether a rollback is safe. A
migrationcompat proof asserts each declared version equals the version actually
applied, so the two cannot drift.

## Testing Strategy

- Unit tests for each parser function
- Integration tests with real message samples
- Edge cases: empty fields, missing segments, Z-segments
- Use `testdata/` for sample messages (not fixtures in code)

### Integration tests (blocking in CI since 2026-08-08)

The CI `test:integration` job (`.gitlab-ci.yml`) is a **blocking merge gate**
(`allow_failure: false`). It runs, in order against shared `postgres` and `minio`
service containers:

```bash
go test -tags=integration ./cmd/fi-fhir/...
go test -tags=integration -p 1 ./pkg/terminology/db/
```

Both paths `DROP SCHEMA terminology CASCADE`, so they run **against separate
databases** in the same PostgreSQL service: `fi_fhir_test` for the cmd suite and
`fi_fhir_terms_test` (created by the job script) for `pkg/terminology/db`. Do not
collapse them back onto one database — sharing it makes the terminology package's
schema teardown/rebuild run against rows left by the cmd suite, which blew the
go-test timeout in CI. `-p 1` does not help here: it limits parallel packages
within a single `go test` invocation, and these are two separate commands.

Reproduce the job locally — no Docker-in-Docker needed, and no Docker Desktop on
this machine, so use the remote context:

```bash
# Use the workspace's remote Docker context (see the workspace AGENTS.md);
# there is no local Docker Desktop. Substitute your context's host for <docker-host>.
docker --context 7900xtx run --rm -d --name pg -e POSTGRES_USER=testuser \
  -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=fi_fhir_test -p 15503:5432 postgres:16
# `server /data` is required — the image's default CMD prints usage and exits.
docker --context 7900xtx run --rm -d --name mio -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin -p 15504:9000 minio/minio:latest server /data

# NOTE: assign each var on its own line. In `export A=1 B="$A"` bash expands
# $A before the assignment, so B ends up empty and the tests silently skip.
PGHOST_PORT="<docker-host>:15503"
export FI_FHIR_DATABASE_URL="postgres://testuser:testpass@${PGHOST_PORT}/fi_fhir_test?sslmode=disable"
export FI_FHIR_TERMINOLOGY_DB_URL="postgres://testuser:testpass@${PGHOST_PORT}/fi_fhir_test?sslmode=disable"
# Separate database, mirroring CI:
psql "$FI_FHIR_DATABASE_URL" -c 'CREATE DATABASE fi_fhir_terms_test OWNER testuser'
export POSTGRES_TEST_URL="postgres://testuser:testpass@${PGHOST_PORT}/fi_fhir_terms_test?sslmode=disable"
export FI_FHIR_MINIO_ENDPOINT="<docker-host>:15504"
export FI_FHIR_MINIO_ACCESS_KEY=minioadmin FI_FHIR_MINIO_SECRET_KEY=minioadmin
go test -tags=integration ./cmd/fi-fhir/...
go test -tags=integration -p 1 ./pkg/terminology/db/
```

**Watch the skip count, not just the exit code.** `setupTestInfra()` calls
`t.Skipf` — not `t.Fatalf` — when Postgres or MinIO is unreachable, so a broken
service makes the job *greener*, not redder. A dead `minio` service container
hid 30 skipped tests behind a passing job until 2026-08-08. Sanity check:
`./cmd/fi-fhir/...` should report ~75.9% coverage with both services live; ~73.2%
means MinIO is down and 30 tests skipped. See `.loom/40-decisions.md`
(2026-08-08) for the full analysis.

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
| `pkg/eventsourcing/postgres_store.go` | PostgreSQL event store for production |
| `pkg/eventsourcing/projection.go` | Projection framework with checkpointing |
| `pkg/eventsourcing/projections/` | Healthcare projections (timeline, stats, encounters) |
| `pkg/eventsourcing/snapshot.go` | Snapshot store interface and memory implementation |
| `pkg/eventsourcing/postgres_snapshot.go` | PostgreSQL-backed snapshot store |
| `pkg/eventsourcing/rebuild.go` | ProjectionRebuilder with progress, dry-run, snapshots |
| `pkg/eventsourcing/time_range.go` | TimeRangeEventStore for temporal queries |
| `pkg/eventsourcing/archive.go` | Event archival and HIPAA-aware retention policies |
| `pkg/eventsourcing/compaction.go` | Stream compaction with aggregate snapshots |
| `pkg/eventsourcing/saga.go` | Saga orchestration for multi-step transactions |
| `pkg/eventsourcing/outbox.go` | Outbox pattern for reliable event publishing |

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

---

## Documentation

Docs are rendered on flexinfer.ai/docs/fi-fhir.

To update the site after doc changes:
1. Push changes to this repo
2. In flexinfer-site: `pnpm sync:fi-fhir-docs && pnpm build`

Navigation structure is defined in `flexinfer-site/content/fi-fhir-docs/nav.yaml`.

<!-- BEGIN LOOM:AGENT-SAFETY -->
## Loom Agent Safety Policy (Generated)

- Pre-existing uncommitted/untracked files are baseline context, not an automatic blocker.
- Continue on the current branch/worktree by default.
- Stage and commit only files intentionally changed for the active task.
- Escalate only when new unexpected changes appear in files you are editing, or when a branch/worktree switch is explicitly requested.
- Dirty-worktree mode: `continue_scoped_commits`.

Canonical nudge for CLI hooks:
> Dirty worktree detected. Treat pre-existing changes as baseline context, continue work, and stage/commit only files for the active task. Escalate only if new unexpected changes appear in files you are editing.

<!-- END LOOM:AGENT-SAFETY -->
