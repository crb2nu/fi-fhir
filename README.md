![Banner](assets/banner.png)

# fi-fhir

A format-agnostic healthcare integration platform that transforms legacy formats (HL7v2, CSV, EDI X12, CDA) into semantic events and routes them through configurable workflows.

CI (build, tests, coverage gates) runs on self-hosted GitLab; github.com/crb2nu/fi-fhir mirrors `main`.

## Overview

fi-fhir addresses a core problem in healthcare integration: **users think in workflow terms, but tools require format-specific knowledge**.

Instead of writing code that references `PID.3.1` or `OBX.5`, you work with semantic events like `patient_admit` and `lab_result`. The library handles format parsing, field mapping, validation, and routing automatically.

fi-fhir runs as two planes. The CLI plane (`fi-fhir parse`, `fi-fhir workflow`)
parses any supported format and executes actions directly, which suits scripting
and local iteration. The integration engine (`fi-fhir serve`) accepts HL7v2 over
MLLP, an authenticated HTTP endpoint, or S3/SFTP batch, resolves an immutable
content-addressed revision, records a durable receipt, and delivers through an
outbox with retry and circuit breaking.

![Overview Dataflow](docs/mermaid/overview-flow.svg)

![CLI Dataflow](docs/mermaid/cli-flow.svg)

## 60-second demo

Parse a sample ADT admit that ships in this repo into a semantic event:

```bash
git clone https://github.com/crb2nu/fi-fhir.git && cd fi-fhir
make build
./bin/fi-fhir parse --format hl7v2 --pretty testdata/adt_a01_sample.hl7
```

```json
{
  "type": "patient_admit",
  "source_format": "hl7v2",
  "source_message_id": "MSG00001",
  "patient": {
    "mrn": "123456789",
    "family_name": "DOE",
    "given_name": "JOHN",
    "date_of_birth": "1980-03-15T00:00:00Z",
    "address": { "line1": "123 MAIN ST", "city": "ANYTOWN", "state": "VA" }
  },
  "encounter": {
    "class": "I",
    "classified_event_type": "inpatient_admit",
    "location": { "facility": "HOSPITAL", "unit": "ICU", "room": "101", "bed": "A" },
    "attending_provider": { "family_name": "SMITH", "given_name": "JANE" }
  }
}
```

Output trimmed; the full event also carries typed identifiers with assigners,
demographics, and provenance fields. No `PID.3.1` in sight. Pipe the same
output into `fi-fhir workflow run` to route it (see Quick Start below).

## Mapping Studio (UI)

The `ui/` app is a SvelteKit 5 "Mapping Studio": a VS Code-style shell with an
activity bar, editor tabs, and a bottom panel for Output, Problems, Debug,
Trace, and Copilot. Work is organized as a five-stage journey — source intake,
normalization, translation, delivery, and verification — around a mission
control dashboard. Preview runs stream stage events, diagnostics, and field
lineage over GraphQL SSE, and a workflow draft can be simulated and then
published, approved, and deployed.

![Mapping Studio Loop](docs/mermaid/ui-mapping-flow.svg)

See `ui/README.md` for the current UI roadmap and dev commands.

## Documentation

### Getting Started

- **[User Guide](docs/user-guide/README.md)** - Tutorials, concepts, and CLI reference
- **[Playground](https://flexinfer.ai/playground/fi-fhir)** - Interactive browser-based learning environment

### Developer Resources

- **[Developer Guide](docs/developer-guide/README.md)** - Architecture, contributing, and extension development
- **[AGENTS.md](AGENTS.md)** - AI assistant guidance and comprehensive architecture reference

### Reference

- **[Planning Documents](docs/planning/README.md)** - Technical specifications and design docs
- **[Architecture Diagrams](docs/diagrams/README.md)** - Generated package and call-graph diagrams

## Features

- **Multi-format parsing**: HL7v2, CSV/flatfiles, EDI X12, CDA/CCDA
- **Workflow DSL**: YAML-based routing with CEL expression filters
- **FHIR R4 output**: US Core R4 mapper producing 26 resource types, including Patient, Encounter, Observation, Condition, Coverage, Claim, ExplanationOfBenefit, MedicationRequest, AllergyIntolerance, Procedure, Immunization, DocumentReference, Provenance, Practitioner, and Organization; see [`pkg/fhir/mapper.go`](pkg/fhir/mapper.go) and [`docs/STATUS.md`](docs/STATUS.md) for the full list
- **Multiple actions**: log, webhook, FHIR, email, exec, file, database (PostgreSQL/MySQL/SQLite), message queue (Kafka), event store
- **Production ingestion**: MLLP listener with mTLS and ACK semantics, authenticated HTTP endpoint, S3/SFTP batch worker
- **Deployment lifecycle**: immutable content-addressed revisions, draft → validated → approved → published → deployed
- **Reliability**: Retry with backoff, circuit breaker, dead letter queue, rate limiting
- **Observability**: Prometheus metrics and structured logging; OpenTelemetry tracing is scaffolded at the workflow layer but not wired into `serve` (see "Tracing (OpenTelemetry) — NOT IMPLEMENTED" below)
- **Production-ready**: Helm chart, CI/CD pipelines, security hardening guide

### Companion tool

[edilint](https://github.com/crb2nu/edilint) is a single-binary pre-send linter for interchange files from the same author.
It began as fi-fhir's lint pass and now runs as the gate in front of the pipeline fi-fhir provides, catching malformed
files before they are transmitted.

## Installation

### CLI

```bash
# Build from the GitHub mirror
git clone https://github.com/crb2nu/fi-fhir.git
cd fi-fhir
make build

# Or, with access to the canonical GitLab host:
go install gitlab.flexinfer.ai/libs/fi-fhir/cmd/fi-fhir@latest
```

### Docker

Images are published to the canonical GitLab registry (requires access to
that host); public users should build from source above.

```bash
docker pull registry.gitlab.flexinfer.ai/libs/fi-fhir:latest

# The entrypoint is the CLI, so pass a subcommand
docker run --rm registry.gitlab.flexinfer.ai/libs/fi-fhir:latest version
```

`fi-fhir serve` fails closed unless the authenticated preview runtime is
configured. Use `docker-compose up -d` below for a runnable local stack.

### Helm

```bash
helm install fi-fhir deploy/helm/fi-fhir/ \
  --set secrets.fhir.baseUrl=https://fhir.example.com
```

## Quick Start

### 1. Parse a Message

```bash
# Parse HL7v2 ADT message
fi-fhir parse --format hl7v2 --pretty message.hl7

# Parse CSV patient file
fi-fhir parse --format csv --pretty patients.csv

# Parse EDI 837P claim
fi-fhir parse --format edi --pretty claim.edi
```

### 2. Run a Workflow

```bash
# Create workflow configuration
cat > workflow.yaml << 'EOF'
workflow:
  name: adt_routing
  version: "1.0"
  routes:
    - name: admits_to_fhir
      filter:
        event_type: patient_admit
      actions:
        - type: fhir
          endpoint: http://localhost:8090/fhir
          resource: Patient
        - type: log
          level: info
          message: "Patient admitted: {{.patient.family_name}}"
EOF

# Process events through workflow
fi-fhir parse --format hl7v2 message.hl7 | \
  fi-fhir workflow run --config workflow.yaml

# Dry-run mode (no side effects)
fi-fhir workflow dry-run --config workflow.yaml event.json
```

Action templates are Go templates evaluated against the event JSON, so field
paths use the JSON key names (`{{.patient.family_name}}`), not Go struct field
names.

### 3. Validate Configuration

```bash
fi-fhir workflow validate workflow.yaml
fi-fhir config validate
fi-fhir config show
```

## Supported Formats

### HL7 v2.x

| Message Type | Description | Semantic Event |
|--------------|-------------|----------------|
| ADT^A01 | Admit | `patient_admit` |
| ADT^A02 | Transfer | `patient_transfer` |
| ADT^A03 | Discharge | `patient_discharge` |
| ADT^A04 | Register (outpatient) | `patient_admit` |
| ADT^A08 | Update patient info | `patient_update` |
| ORU^R01 | Lab result | `lab_result` |
| RDE^O11 | Pharmacy order | `medication_request` |
| VXU^V04 | Immunization update | `immunization` |
| SIU^S12-S15, S26 | Scheduling | `appointment_scheduled`, `appointment_rescheduled`, `appointment_modified`, `appointment_cancelled`, `appointment_noshow` |
| MDM^T01-T11 | Clinical documents | `document_original`, `document_status_change`, `document_addendum`, `document_edit`, `document_replacement` |
| DFT^P03, P11 | Financial transaction | `financial_transaction` |

### EDI X12

| Transaction | Description | Semantic Event |
|-------------|-------------|----------------|
| 837P | Professional claim | `claim_submitted` |
| 837I | Institutional claim | `claim_submitted` |
| 835 | Remittance advice | `claim_adjudicated` |
| 270 | Eligibility inquiry | `eligibility_inquiry` |
| 271 | Eligibility response | `eligibility_response` |
| 276 | Claim status request | `claim_status_request` |
| 277 | Claim status response | `claim_status_response` |

`fi-fhir parse --format edi` emits semantic events for every transaction set above.
The parser also recognizes 278 and 834, but no event mappers exist for them yet;
those transaction sets parse to a generic `unknown_transaction` record. Payer
companion guide validation is available via `--edi-companion`; see
`fi-fhir companion list`.

### CDA/CCDA

- Section parsers for medications, allergies, and social history
- Narrative extraction

### CSV/Flatfiles

- Automatic schema inference
- Patient demographics
- Lab results
- Custom record types

## Workflow DSL

### Filters

```yaml
filter:
  # Match by event type
  event_type: patient_admit
  event_type: [patient_admit, patient_transfer]

  # Match by source system
  source: epic_adt
  source: [epic_adt, cerner_adt]

  # CEL expressions for complex conditions
  condition: event.patient.age >= 65
  condition: event.observation.interpretation in ["critical", "HH"]
```

### Transforms

```yaml
transform:
  - set_field: patient.status = "active"
  - map_terminology: patient.race
  - redact: patient.ssn
```

### Actions

```yaml
actions:
  # FHIR server (OAuth2 client credentials)
  - type: fhir
    endpoint: https://fhir.example.com/r4
    resource: Patient
    token_url: https://auth.example.com/oauth2/token
    client_id: my-client-id
    client_secret: my-client-secret

  # Webhook (event is POSTed as JSON)
  - type: webhook
    url: https://api.example.com/events
    method: POST
    token: my-api-token

  # Database (column values are event field paths)
  - type: database
    connection: postgres://user:pass@db.example.com:5432/events
    operation: upsert
    table: events
    conflict_on: patient_mrn
    mapping_patient_mrn: patient.mrn
    mapping_event_type: type

  # Message queue (built-in driver is "log"; key is an event field path)
  - type: queue
    driver: log
    topic: healthcare-events
    key: patient.mrn

  # Logging
  - type: log
    level: info
    message: "Processed: {{.type}} for {{.patient.mrn}}"
```

## TypeScript SDK

```bash
npm install @fi-fhir/sdk
```

```typescript
import { parseHL7, Workflow } from '@fi-fhir/sdk';

const event = await parseHL7(hl7Message, { source: 'epic_adt' });

const workflow = new Workflow('./workflow.yaml');
await workflow.validate();
const output = await workflow.run([event]);
```

## All Documentation

| Document | Description |
|----------|-------------|
| **User Guide** | |
| [Getting Started](docs/user-guide/getting-started.md) | First-time setup and tutorials |
| [Core Concepts](docs/user-guide/core-concepts.md) | Architecture and design philosophy |
| [CLI Reference](docs/user-guide/cli-reference.md) | Complete command reference |
| [Source Profiles](docs/user-guide/source-profiles.md) | Profile configuration guide |
| [Workflows](docs/user-guide/workflows.md) | Workflow DSL reference |
| [FHIR Output](docs/user-guide/fhir-output.md) | FHIR R4 mapping details |
| [Playground Tutorial](docs/user-guide/playground-tutorial.md) | Interactive learning guide |
| **Developer Guide** | |
| [Architecture](docs/developer-guide/architecture.md) | System architecture overview |
| [Development Setup](docs/developer-guide/development-setup.md) | Environment setup |
| [Adding Parsers](docs/developer-guide/adding-parser.md) | Format parser development |
| [Testing](docs/developer-guide/testing.md) | Testing guidelines |
| **Operations** | |
| [Production Hardening](docs/operations/PRODUCTION-HARDENING.md) | Security hardening guide |
| [Operations Runbook](docs/operations/RUNBOOK.md) | Troubleshooting and operations |
| **Reference** | |
| [AGENTS.md](AGENTS.md) | AI assistant guidance and architecture |
| [CHANGELOG.md](CHANGELOG.md) | Release history |
| [API Reference](api/openapi.yaml) | OpenAPI 3.1 specification |
| [Example Workflows](examples/README.md) | Ready-to-use workflow templates |

## Development

```bash
# Build
make build

# Test
make test              # Unit tests
make test-e2e          # E2E tests
make test-integration  # Integration tests (requires Docker)

# Lint
make lint

# Run benchmarks
make bench

# Docker
make docker-build
```

### Local Development Stack

```bash
# Start dependencies (PostgreSQL, Kafka, FHIR server, Jaeger)
docker-compose up -d

# Run with local config
./bin/fi-fhir workflow run --config examples/workflows/adt-to-fhir.yaml
```

## Deployment

### Docker Compose

```bash
docker-compose up -d
# API: http://localhost:8080
# Metrics: http://localhost:9090/metrics
```

### Kubernetes

```bash
kubectl apply -k deploy/kubernetes/base/
# Or with production overlay
kubectl apply -k deploy/kubernetes/overlays/production/
```

### Helm

```bash
helm install fi-fhir deploy/helm/fi-fhir/ \
  --set replicaCount=3 \
  --set config.database.enabled=true \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=fi-fhir.example.com
```

## Observability

### Metrics (Prometheus)

Metrics use the `fi_fhir` namespace and `workflow` subsystem:

```
fi_fhir_workflow_events_processed_total
fi_fhir_workflow_events_processed_duration_seconds
fi_fhir_workflow_actions_executed_total
fi_fhir_workflow_actions_executed_duration_seconds
fi_fhir_workflow_action_retries_total
fi_fhir_workflow_circuit_breaker_state_changes_total
fi_fhir_workflow_dlq_depth
```

### Tracing (OpenTelemetry) — NOT IMPLEMENTED

`FI_FHIR_TRACING_ENABLED`, `FI_FHIR_TRACING_ENDPOINT`, and
`FI_FHIR_TRACING_SAMPLER` are parsed and validated by `pkg/config`, but nothing
consumes them: there is no OpenTelemetry exporter in the `serve` path, and
setting them changes no runtime behaviour. The exporter is slice 4.4d, which
depends on structured logging landing first.

Until then, correlation across a message's lifecycle comes from the correlation
and trace identifiers already carried on every durable record — receipts,
canonical events, lineage rows, and delivery attempts — not from spans. See
[docs/operations/README.md](docs/operations/README.md) "Tracing — not
implemented".

### Health Checks

- `/health` - Liveness probe
- `/ready` - Readiness probe (checks dependencies)
- `/metrics` - Prometheus metrics

## Project Structure

```
fi-fhir/
├── cmd/fi-fhir/           # CLI entry point
├── internal/
│   ├── parser/            # Format parsers (hl7v2, csv, edi, cda, fhir)
│   ├── integration/       # Integration engine (ingress, mllp, batch,
│   │                      #   processor, lifecycle, delivery, session)
│   ├── workflow/          # Workflow engine and actions
│   ├── api/               # GraphQL server and resolvers
│   ├── terminology/       # Terminology services
│   ├── fhir/              # FHIR client and subscriptions
│   └── llm/               # LLM-backed operations
├── pkg/
│   ├── events/            # Public semantic event types
│   ├── integration/       # Immutable revision and policy types
│   ├── config/            # Configuration management
│   ├── profile/           # Source profiles
│   ├── eventsourcing/     # Event store and projections
│   ├── storage/           # Object storage
│   └── validate/          # Identifier validators (NPI, MBI, SSN)
├── ui/                    # SvelteKit Mapping Studio
├── api/                   # OpenAPI specification
├── deploy/
│   ├── helm/              # Helm chart
│   └── kubernetes/        # Kustomize manifests
├── dashboards/            # Grafana dashboards & alerting rules
├── examples/              # Example workflows
├── sdk/typescript/        # TypeScript SDK
└── test/e2e/              # End-to-end tests
```

## Contributing

See [AGENTS.md](AGENTS.md) for architecture guidance and coding conventions.

## License

Apache License 2.0 - see [LICENSE](LICENSE)
