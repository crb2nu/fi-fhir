![Banner](assets/banner.png)

# fi-fhir

A healthcare integration platform for turning HL7v2, CSV, EDI X12, and CDA messages into semantic events, building feed-specific Source Profiles, and operating durable delivery workflows.

Development and CI run on [GitLab](https://gitlab.flexinfer.ai/libs/fi-fhir).
[GitHub](https://github.com/crb2nu/fi-fhir) mirrors `main` and release tags.

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

## 60-second demo

With Go 1.26.9 or newer, parse a sample ADT admit from this repo into a semantic event:

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
demographics, and provenance fields. Omit `--pretty` when piping events into
`fi-fhir workflow run`: the workflow reader expects one JSON event per line
(see Quick Start below).

## Mapping Studio (UI)

Mapping Studio (`ui/`, SvelteKit) takes a feed through five stages: source
intake, normalization, translation, delivery, and verification. Its Build and
Operate explorer, saved record tabs, command palette, and responsive drawers
keep sessions and connections within reach. Keyboard navigation and unsaved-edit
prompts protect work while moving between editors.

- **Build from samples.** Paste or import messages, load synthetic examples, or
  capture samples from a source connection. Integration Sessions retain drafts,
  runs, diagnostics, and lineage; browse saved or archived sessions and reopen
  them by URL.
- **Shape and test a workflow.** Edit Source Profiles, map terminology, validate
  workflow drafts, and simulate changes. The bottom panel brings together
  Output, Problems, Debug, Trace, and Copilot. Copilot uses the deployment's LLM
  for explanations, suggestions, workflow generation, and review.
- **Author a release.** Connections manages sources, destinations, and integration
  definitions. Build a definition from compiled connections and a resolvable
  registry profile/workflow pair, then check, validate, approve, and publish it.
  Operator deploys the release and records its lifecycle history. Mounted
  adapter configuration still changes through deployment configuration.
- **Verify delivery.** Browse durable admissions, statistics, and retention;
  follow receipt traces and inspect delivery attempts. Engine properties show
  the answering replica's configured services and schema versions.

Features depend on the deployment's capabilities and your identity's roles.
Unavailable capabilities are explained in the UI. The hosted demo provides
stateless HL7 preview; durable sessions and operator views need a configured
backend.

![Mapping Studio workspace and integration journey](docs/mermaid/ui-mapping-flow.svg)

See the [Mapping Studio guide](docs/user-guide/ide.md),
[Connections and definitions](docs/user-guide/connections.md#definitions), and
[Verification guide](docs/user-guide/verification.md). Local UI commands are in
[ui/README.md](ui/README.md).

## Documentation

### Getting Started

- **[User Guide](docs/user-guide/README.md)** - Tutorials, concepts, and CLI reference
- **[Browser Playground](https://flexinfer.ai/playground/fi-fhir)** - The engine compiled to WebAssembly, running in your tab; nothing you paste leaves the page ([what it runs](docs/user-guide/playground.md))
- **[Mapping Studio guide](docs/user-guide/ide.md)** - The IDE, route by route
- **[Hosted demo](https://fi-fhir-demo.flexinfer.ai)** - The real IDE shell with a preview-only identity and no database: HL7 Preview works, every other surface shows its honest "not available on this deployment" state, and nothing is stored

### Developer Resources

- **[Developer Guide](docs/developer-guide/README.md)** - Architecture, contributing, and extension development
- **[AGENTS.md](AGENTS.md)** - AI assistant guidance and comprehensive architecture reference

### Reference

- **[Planning Documents](docs/planning/README.md)** - Technical specifications and design docs
- **[Architecture Diagrams](docs/diagrams/README.md)** - Generated package dependencies and CLI command dispatch

## Features

- **Multi-format parsing**: HL7v2, CSV/flatfiles, EDI X12, CDA/CCDA
- **Workflow DSL**: YAML-based routing with CEL expression filters
- **FHIR R4 output**: US Core R4 mapper with 26 exported `Map*` methods, including Patient, Encounter, Observation, Condition, Coverage, Claim, ExplanationOfBenefit, MedicationRequest, AllergyIntolerance, Procedure, Immunization, DocumentReference, Provenance, Practitioner, and Organization; see [`pkg/fhir/mapper.go`](pkg/fhir/mapper.go) and [`docs/STATUS.md`](docs/STATUS.md) for the full list
- **Multiple actions**: log, webhook, FHIR, email, exec, file, database (PostgreSQL/MySQL/SQLite), message queue (Kafka, Redis Streams, Google Cloud Pub/Sub), event store
- **Production ingestion**: MLLP listener with mTLS and ACK semantics, authenticated HTTP endpoint, S3/SFTP batch worker
- **Deployment lifecycle**: immutable content-addressed revisions, draft → validated → approved → published → deployed; authored in Connections and operated through Operator
- **Connections**: named source (MLLP, HTTP, S3, SFTP) and destination (HTTPS, FHIR, Kafka) declarations, validated and compiled into the exact documents `serve` mounts; the catalog authors and compiles, deployment configuration activates adapters ([guide](docs/user-guide/connections.md), [operations](docs/operations/CONNECTION-CATALOG.md))
- **Engine properties**: the IDE's Connections → Engine tab shows what the answering replica composed at startup: identity and access, the control plane, registry integrations, one panel per adapter with the environment key behind each property, and the schema ledgers; secrets show only `set` or `unset` (the `engineRuntime` allowlist)
- **Sample intake**: pull real messages from a source connection into an Integration Session (stream capture or batch peek) to build a profile against; every message is redacted before it is stored and every capture is an audited row
- **Browser playground**: the Source Profile compiler, HL7v2 parser and FHIR projection compiled to WebAssembly (`cmd/fi-fhir-wasm`), with a parity test against the IDE's session preview ([playground](docs/user-guide/playground.md))
- **Reliability**: Retry with backoff, circuit breaker, dead letter queue, rate limiting
- **Observability**: Prometheus metrics and structured JSON logging; OpenTelemetry tracing is scaffolded at the workflow layer but not wired into `serve` (see [Observability](#observability) below)
- **Production-ready**: Helm chart, CI/CD pipelines, security hardening guide

### Companion tool

[edilint](https://github.com/crb2nu/edilint) is a single-binary pre-send linter for interchange files from the same author.
It began as fi-fhir's lint pass and now runs as the gate in front of the pipeline fi-fhir provides, catching malformed
files before they are transmitted.

## Installation

### CLI

Requires Go 1.26.9 or newer (see [go.mod](go.mod)).

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
configured. Use the [local development stack](#local-development-stack) below to configure
a preview identity and start the API and UI.

### Helm

```bash
helm install fi-fhir deploy/helm/fi-fhir/ \
  --set secrets.fhir.baseUrl=https://fhir.example.com
```

## Quick Start

![CLI parsing and workflow commands](docs/mermaid/cli-flow.svg)

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

# Dry-run mode (no side effects; keep event JSON on one line)
fi-fhir parse --format hl7v2 message.hl7 > event.json
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
  event_type: [patient_admit, patient_transfer]

  # Match by source system
  source: [epic_adt, cerner_adt]

  # CEL expressions for complex conditions
  condition: event.encounter.class == "I"
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

  # Message queue (kafka, redis, pubsub, or log; key is an event field path)
  - type: queue
    driver: log
    topic: healthcare-events
    key: patient.mrn

  # Logging
  - type: log
    level: info
    message: "Processed: {{.type}} for {{.patient.mrn}}"
```

See [event backends and consumers](docs/operations/EVENT-BACKENDS.md) for broker
configuration, `workflow consume`, acknowledgment behavior, and outbox adapters.

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
| [Browser Playground](docs/user-guide/playground.md) | What runs in the browser, the kernel contract |
| [Mapping Studio (IDE)](docs/user-guide/ide.md) | The IDE, route by route |
| [Connections](docs/user-guide/connections.md) | Sources, destinations, definitions, sample intake, engine properties |
| [Verification](docs/user-guide/verification.md) | Durable admissions, delivery statistics, retention |
| **Developer Guide** | |
| [Architecture](docs/developer-guide/architecture.md) | System architecture overview |
| [Development Setup](docs/developer-guide/development-setup.md) | Environment setup |
| [Adding Parsers](docs/developer-guide/adding-parser.md) | Format parser development |
| [Testing](docs/developer-guide/testing.md) | Testing guidelines |
| **Operations** | |
| [Production Hardening](docs/operations/PRODUCTION-HARDENING.md) | Security hardening guide |
| [Operations Runbook](docs/operations/RUNBOOK.md) | Troubleshooting and operations |
| [Connection Catalog](docs/operations/CONNECTION-CATALOG.md) | The seventh ledger, roles, audit, allowlists |
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
make test-integration  # Live-service E2E; see test/e2e/README.md

# Lint
make lint

# Run benchmarks
make bench

# Docker
make docker-build
```

### Local Development Stack

Set a local bearer token before starting Compose; the API requires it. Keep it
in your shell and enter it when the UI asks for access.

```bash
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
make dev-ui
# UI:      http://localhost:3001
# API:     http://localhost:8080
# Metrics: http://localhost:9090/metrics

# Stop the stack
make dev-ui-down
```

Compose must be available as `docker-compose` for these Makefile targets.
See the [development setup](docs/developer-guide/development-setup.md) for
configuration and [live-service test setup](test/e2e/README.md) for test
prerequisites. On a remote Docker context, use the Docker host's address for
published ports and configure the UI/API origins for that host.

### Regenerate the diagrams

All seven documentation SVGs are generated with the workspace's
`py-diagram-gen`, `py-sprite-kit`, and `py-visual-kit` libraries:

```bash
make docs-diagrams
# Libraries outside ~/workspace/libs:
make docs-diagrams DIAGRAM_LIBS=/path/to/workspace/libs
```

Conceptual figures use a shared vector design and curated
[layout source](docs/diagrams/narratives.yaml); package imports and CLI dispatch
are extracted from Go source. See [tooling and prerequisites](docs/diagrams/README.md).
Commit the sources, SVGs, and generated Mermaid fallbacks together.

## Deployment

The [production hardening guide](docs/operations/PRODUCTION-HARDENING.md) covers
authentication, secrets, network policy, and deployment configuration. These
manifests require configuration for your environment.

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

The `serve` runtime exposes component health and durable processing metrics,
including:

```text
fi_fhir_build_info
fi_fhir_component_up
fi_fhir_readiness_up
fi_fhir_http_ingress_submissions_total
fi_fhir_mllp_messages_total
fi_fhir_delivery_attempts_total
fi_fhir_batch_objects_total
fi_fhir_session_stream_events_total
```

The standalone workflow metrics adapter also exposes `fi_fhir_workflow_*`
counters and histograms. See the [operations guide](docs/operations/README.md)
and [runtime metrics](internal/observability/metrics.go) for labels and scope.

### Structured logging

`serve` emits structured operational logs. Configure `FI_FHIR_LOG_LEVEL` and
`FI_FHIR_LOG_FORMAT` (`json` or `text`).

### Tracing (OpenTelemetry) — NOT IMPLEMENTED

`FI_FHIR_TRACING_ENABLED`, `FI_FHIR_TRACING_ENDPOINT`, and
`FI_FHIR_TRACING_SAMPLER` are parsed and validated by `pkg/config`, but nothing
consumes them: there is no OpenTelemetry exporter in the `serve` path, and
setting them changes no runtime behaviour.

Correlation across a message's lifecycle comes from the correlation
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
