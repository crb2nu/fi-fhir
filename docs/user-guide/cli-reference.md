# CLI Reference

Complete reference for the fi-fhir command-line interface.

## Global Options

```bash
fi-fhir [global-options] <command> [command-options]

Global Options:
  --config FILE       Configuration file path
  --verbose, -v       Enable verbose output
  --quiet, -q         Suppress non-error output
  --json              Output in JSON format
  --help, -h          Show help
  --version           Show version
```

## Commands Overview

| Command | Description |
|---------|-------------|
| `parse` | Parse healthcare messages to semantic events |
| `workflow` | Run events through workflows |
| `profile` | Manage source profiles |
| `validate` | Validate profiles and messages |
| `fhir` | FHIR resource operations |
| `config` | Configuration management |
| `serve` | Start GraphQL API server |
| `subscription` | Manage FHIR subscriptions |
| `eventstore` | Event sourcing operations |
| `projection` | Projection management |
| `terminology` | Terminology database operations |
| `etl` | ETL pipeline for terminology |
| `storage` | Object storage operations |
| `companion` | EDI companion guide operations |

---

## parse

Parse healthcare messages into semantic events.

### Usage

```bash
fi-fhir parse [options] [FILE...]
```

### Options

| Option | Description |
|--------|-------------|
| `--format FORMAT` | Input format: `hl7v2`, `csv`, `edi`, `edi837p`, `edi835`, `cda`, `fhir` |
| `--profile FILE` | Source profile to use |
| `--pretty` | Pretty-print JSON output |
| `--source NAME` | Source system name |
| `--output FILE` | Output file (default: stdout) |
| `--explain-warnings` | Add LLM-powered explanations to warnings |
| `--extract-clinical` | Extract clinical entities from documents |
| `--edi-companion` | EDI companion guide: `auto`, or path to guide YAML |
| `--edi-companion-dir` | Directory containing companion guides |

### Examples

```bash
# Parse HL7v2 message
fi-fhir parse --format hl7v2 --pretty message.hl7

# Parse with custom profile
fi-fhir parse --format hl7v2 --profile epic_adt.yaml message.hl7

# Parse EDI claim
fi-fhir parse --format edi --pretty claim.x12

# Parse EDI 837P with companion guide (auto-detect payer)
fi-fhir parse -f edi837p claim.x12 --edi-companion auto

# Parse EDI with specific companion guide
fi-fhir parse -f edi837p claim.x12 --edi-companion /path/to/bcbs-guide.yaml

# Parse EDI with companion guide directory
fi-fhir parse -f edi837p claim.x12 --edi-companion-dir /guides/

# Parse CSV file
fi-fhir parse --format csv --pretty patients.csv

# Parse CDA document
fi-fhir parse --format cda --pretty clinical_doc.xml

# Parse multiple files
fi-fhir parse --format hl7v2 messages/*.hl7 > all_events.json

# Parse with LLM warning explanations
fi-fhir parse --format hl7v2 --explain-warnings --pretty message.hl7

# Extract clinical entities from MDM document
fi-fhir parse --format hl7v2 --extract-clinical mdm_message.hl7
```

---

## workflow

Run events through workflow routes.

### Usage

```bash
fi-fhir workflow <subcommand> [options]
```

### Subcommands

#### workflow run

Execute a workflow on events.

```bash
fi-fhir workflow run [options] [EVENT_FILE]
```

| Option | Description |
|--------|-------------|
| `--config FILE` | Workflow configuration file |
| `--dry-run` | Execute without side effects |
| `--route NAME` | Run specific route only |

```bash
# Run workflow on events from stdin
cat events.json | fi-fhir workflow run --config workflow.yaml

# Dry-run mode
fi-fhir workflow run --dry-run --config workflow.yaml events.json

# Run specific route
fi-fhir workflow run --route critical_alerts --config workflow.yaml events.json
```

#### workflow validate

Validate workflow configuration.

```bash
fi-fhir workflow validate FILE
```

```bash
fi-fhir workflow validate workflow.yaml
```

#### workflow simulate

Test workflow with simulated actions.

```bash
fi-fhir workflow simulate --config FILE --events FILE
```

#### workflow generate

Generate workflow YAML from natural language description using LLM.

```bash
fi-fhir workflow generate "DESCRIPTION"
fi-fhir workflow generate --interactive "DESCRIPTION"
```

| Option | Description |
|--------|-------------|
| `--interactive` | Interactive mode for complex workflows |
| `--model MODEL` | LLM model to use |
| `--output FILE` | Output file (default: stdout) |

```bash
# Generate workflow from description
fi-fhir workflow generate "Route critical lab results to the pager system"

# Interactive mode
fi-fhir workflow generate --interactive "Create a workflow for ADT events"

# Save to file
fi-fhir workflow generate "Alert on ICU admissions" --output icu_alerts.yaml
```

#### workflow explain

Generate human-readable explanation of a workflow.

```bash
fi-fhir workflow explain FILE
```

| Option | Description |
|--------|-------------|
| `--format FORMAT` | Output format: `markdown`, `text`, `json` |

```bash
# Explain workflow in markdown
fi-fhir workflow explain workflow.yaml

# JSON output for programmatic use
fi-fhir workflow explain --format json workflow.yaml
```

#### workflow cel

Generate CEL filter expression from natural language.

```bash
fi-fhir workflow cel "DESCRIPTION"
```

| Option | Description |
|--------|-------------|
| `--test` | Test expression against sample event |
| `--event FILE` | Sample event for testing |

```bash
# Generate CEL expression
fi-fhir workflow cel "patient over 65 with abnormal lab results"

# Test against sample event
fi-fhir workflow cel --test --event sample.json "patient admitted to ICU"
```

#### workflow dry-run

Execute workflow without side effects. Actions are simulated and logged.

```bash
fi-fhir workflow dry-run -c FILE [EVENT_FILE]
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-v, --verbose` | Show detailed route matching |

```bash
# Dry-run from file
fi-fhir workflow dry-run -c workflow.yaml events.json

# Dry-run from stdin
cat events.json | fi-fhir workflow dry-run -c workflow.yaml -
```

#### workflow record

Capture events and results for regression testing.

```bash
fi-fhir workflow record -c FILE -o FILE [EVENT_FILE]
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-o, --output` | Output file for recordings (required) |

```bash
fi-fhir workflow record -c workflow.yaml -o recordings.json events.json
```

#### workflow replay

Replay recorded events and compare against baseline.

```bash
fi-fhir workflow replay -c FILE [options] RECORDINGS_FILE
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-t, --event-type` | Filter by event type |
| `-s, --source` | Filter by source system |
| `-l, --limit` | Maximum events to replay |
| `-d, --diffs` | Show diffs for mismatches |
| `-o, --output` | Save comparison results |

```bash
# Replay with diff output
fi-fhir workflow replay -c workflow.yaml -d recordings.json

# Filter by event type
fi-fhir workflow replay -c workflow.yaml -t patient_admit recordings.json
```

#### workflow loadtest

Performance test workflows under load.

```bash
fi-fhir workflow loadtest -c FILE [options]
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-s, --scenario` | Predefined scenario: `smoke`, `standard`, `stress`, `burst`, `soak` |
| `-d, --duration` | Test duration (e.g., 30s, 5m) |
| `-r, --rps` | Target requests per second |
| `-w, --workers` | Concurrent workers |
| `--warmup` | Warmup duration |
| `-v, --verbose` | Real-time metrics |
| `--json` | JSON output |

```bash
# Quick smoke test
fi-fhir workflow loadtest -c workflow.yaml -s smoke -v

# Custom load test
fi-fhir workflow loadtest -c workflow.yaml -d 60s -r 2000 -w 8
```

See [Workflow Configuration](workflows.md#testing--validation) for detailed testing documentation.

---

## profile

Manage source profiles.

### Usage

```bash
fi-fhir profile <subcommand> [options]
```

### Subcommands

#### profile infer

Generate a profile from sample messages.

```bash
fi-fhir profile infer [options] FILES...
```

| Option | Description |
|--------|-------------|
| `--output FILE` | Output profile file |
| `--name NAME` | Profile name |

```bash
fi-fhir profile infer --output inferred.yaml samples/*.hl7
```

#### profile lint

Check profile for best practices.

```bash
fi-fhir profile lint FILE
```

```bash
fi-fhir profile lint my_profile.yaml
```

---

## validate

Validate profiles and messages.

### Usage

```bash
fi-fhir validate <subcommand> [options] FILE
```

### Subcommands

#### validate profile

Validate a source profile.

```bash
fi-fhir validate profile FILE
```

#### validate message

Validate a healthcare message.

```bash
fi-fhir validate message --format FORMAT [--profile FILE] FILE
```

---

## fhir

FHIR resource operations.

### Usage

```bash
fi-fhir fhir <subcommand> [options]
```

### Subcommands

#### fhir validate

Validate FHIR resources.

```bash
fi-fhir fhir validate [options] FILE
```

| Option | Description |
|--------|-------------|
| `--profile NAME` | Profile to validate against (e.g., `us-core`) |
| `--strict` | Enable strict validation |

```bash
fi-fhir fhir validate patient.json
fi-fhir fhir validate --profile us-core bundle.json
```

#### fhir generate

Generate FHIR from events.

```bash
cat events.json | fi-fhir fhir generate --profile us-core
```

---

## config

Configuration management.

### Usage

```bash
fi-fhir config <subcommand>
```

### Subcommands

#### config show

Display current configuration.

```bash
fi-fhir config show
```

#### config validate

Validate configuration file.

```bash
fi-fhir config validate [FILE]
```

#### config env

Show environment variable mappings.

```bash
fi-fhir config env
```

---

## serve

Start the GraphQL API server.

### Usage

```bash
fi-fhir serve [options]
```

### Options

| Option | Description |
|--------|-------------|
| `--host HOST` | Bind address (default: `0.0.0.0`) |
| `--port PORT` | Port number (default: `8081`) |
| `--path PATH` | GraphQL path (default: `/graphql`) |
| `--no-playground` | Disable the development Playground |
| `--no-introspection` | Disable GraphQL introspection |
| `--max-depth N` | Maximum query depth |
| `--max-complexity N` | Maximum query complexity |
| `--timeout DURATION` | HTTP read/write timeout |
| `--dry-run` | Print the effective non-secret server configuration |

Required in both authentication modes:

| Variable | Purpose |
| --- | --- |
| `FI_FHIR_DEPLOYMENT_TENANT_ID` | Tenant bound to the immutable registry |
| `FI_FHIR_GRAPHQL_AUTH_MODE` | `static` (default) or `oidc` |
| `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS` | Exact comma-separated browser origins |
| `FI_FHIR_INTEGRATION_REGISTRY_PATH` | Startup registry JSON path |

Static compatibility mode additionally requires:

| Variable | Purpose |
| --- | --- |
| `FI_FHIR_GRAPHQL_PRINCIPAL_ID` | Server-owned preview principal |
| `FI_FHIR_GRAPHQL_ROLES` | Must include `integration:preview` |
| `FI_FHIR_GRAPHQL_BEARER_TOKEN` or `_FILE` | Exactly one bearer source |

OIDC mode instead requires `FI_FHIR_GRAPHQL_OIDC_ISSUER_URL` and
`FI_FHIR_GRAPHQL_OIDC_AUDIENCE`. Optional claim/algorithm controls are
`FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM` (default `tenant_id`),
`FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM` (default `roles`), and
`FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS` (default `RS256`). OIDC mode rejects every
static bearer, principal, role, and trusted-CIDR setting. Bearers must be signed
JWT access tokens with the protected header `typ=at+jwt`; generic `typ=JWT` and
typeless tokens are rejected.

Optional production HL7v2 HTTP ingress is mounted at `/v1/hl7v2` when
`FI_FHIR_HTTP_INGRESS_AUTH_MODE` is set. Every mode also requires
`FI_FHIR_HTTP_INGRESS_INTEGRATION_ID`; leaving the mode unset keeps the endpoint
unmounted.

| Mode | Additional configuration |
| --- | --- |
| `bearer` or `hmac-sha256` | `FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID` and exactly one of `FI_FHIR_HTTP_INGRESS_SECRET` or `FI_FHIR_HTTP_INGRESS_SECRET_FILE` |
| `oauth2` | `FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL`, `_AUDIENCE`, and `_ALLOWED_CLIENT_IDS` |

OAuth2 ingress additionally accepts claim/algorithm controls
`FI_FHIR_HTTP_INGRESS_OAUTH_TENANT_CLAIM` (default `tenant_id`),
`FI_FHIR_HTTP_INGRESS_OAUTH_ROLES_CLAIM` (default `roles`),
`FI_FHIR_HTTP_INGRESS_OAUTH_CLIENT_ID_CLAIM` (default `client_id`), and
`FI_FHIR_HTTP_INGRESS_OAUTH_SIGNING_ALGS` (default `RS256`). It accepts only
allowlisted clients whose signed JWT access token has protected `typ=at+jwt`,
`sub == client_id`, the deployment tenant, the exact audience, and
`integration:submit`. Static and OAuth2 settings are mutually exclusive. This
server validates externally obtained tokens; it does not issue tokens or
introspect opaque access tokens.

```bash
# After exporting the required preview environment
fi-fhir serve --port 8080 --no-playground --no-introspection
```

---

## subscription

Manage FHIR subscriptions.

### Usage

```bash
fi-fhir subscription <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `list` | List active subscriptions |
| `status ID` | Get subscription status |
| `create` | Create new subscription |
| `delete ID` | Delete subscription |
| `pause ID` | Pause subscription |
| `resume ID` | Resume subscription |
| `serve` | Start subscription receiver |
| `validate` | Validate subscription config |
| `test` | Test subscription connectivity |

---

## eventstore

Event sourcing operations.

### Usage

```bash
fi-fhir eventstore <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `init` | Initialize event store |
| `stats` | Show event store statistics |
| `streams` | List event streams |
| `read STREAM` | Read events from stream |
| `append STREAM` | Append events to stream |

```bash
# Initialize PostgreSQL event store
fi-fhir eventstore init --driver postgres --dsn ${DATABASE_URL}

# Show statistics
fi-fhir eventstore stats

# Read from a stream
fi-fhir eventstore read patient-MRN12345 --limit 10
```

---

## projection

Projection management.

### Usage

```bash
fi-fhir projection <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `list` | List projections |
| `status NAME` | Get projection status |
| `run NAME` | Run projection |
| `rebuild NAME` | Rebuild projection from events |

```bash
# List all projections
fi-fhir projection list

# Rebuild patient timeline projection
fi-fhir projection rebuild patient-timeline --from 2024-01-01
```

---

## terminology

Terminology database operations for managing healthcare vocabularies, custom mappings, and semantic search.

### Usage

```bash
fi-fhir terminology <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `init` | Initialize terminology database |
| `status` | Show loaded terminologies and versions |
| `load VOCAB` | Load vocabulary (rxnorm, loinc, umls, icd10cm) |
| `use VOCAB VERSION` | Set active vocabulary version |
| `drop` | Drop all terminology data |
| `crosswalk` | Cross-reference codes between systems |
| `search` | Semantic search for terminology codes |
| `index` | Manage terminology embedding index |
| `mapping` | Custom code mapping operations |

### Database Management

```bash
# Initialize terminology database
fi-fhir terminology init --db "$DATABASE_URL"

# Check status
fi-fhir terminology status --db "$DATABASE_URL"

# Set active vocabulary version
fi-fhir terminology use rxnorm 2024-01 --db "$DATABASE_URL"

# Drop all data (destructive)
fi-fhir terminology drop --force --db "$DATABASE_URL"
```

### Loading Vocabularies

```bash
# Load RxNorm from RRF directory
fi-fhir terminology load rxnorm /path/to/rrf/ --version 2024-01

# Load LOINC from CSV
fi-fhir terminology load loinc /data/loinc/LoincTable.csv --version 2.77

# Load UMLS Metathesaurus
fi-fhir terminology load umls /data/umls/META/ --version 2024AB

# Load ICD-10-CM
fi-fhir terminology load icd10cm /data/icd10cm/codes.csv --version FY2024
```

### Custom Mappings

```bash
# Upload mappings from CSV
fi-fhir terminology mapping upload mappings.csv \
  --source-system epic_labs --target-system http://loinc.org

# List mapping sets
fi-fhir terminology mapping list --source-system epic_labs

# Get mapping details
fi-fhir terminology mapping get <mapping-id>

# Delete mapping set
fi-fhir terminology mapping delete <mapping-id> --force

# Resolve a code (with optional LLM autoroute)
fi-fhir terminology mapping resolve GLU001 \
  --source-system epic_labs --target-system http://loinc.org
```

### Semantic Search & Crosswalk

```bash
# Semantic search (finds codes by meaning)
fi-fhir terminology search --query "blood sugar" --vocabulary loinc --limit 10
fi-fhir terminology search --query "chest pain" --vocabulary snomed

# Cross-walk between vocabularies
fi-fhir terminology crosswalk --from icd10cm --to snomed E11.9

# Build embedding index for semantic search
fi-fhir terminology index build --vocabulary loinc --source ./data/LoincTable.csv
fi-fhir terminology index status
```

See [Terminology Management](terminology.md) for comprehensive documentation.

---

## etl

ETL pipeline for terminology data.

### Usage

```bash
fi-fhir etl <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `sync` | Synchronize terminology sources |
| `fetch` | Fetch terminology files |
| `status` | Show ETL pipeline status |

---

## storage

Object storage operations.

### Usage

```bash
fi-fhir storage <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `test` | Test storage connectivity |
| `ls PATH` | List objects |
| `get PATH` | Download object |
| `put PATH` | Upload object |
| `rm PATH` | Delete object |

```bash
# Test S3 connectivity
fi-fhir storage test

# List objects
fi-fhir storage ls events/2024/

# Download file
fi-fhir storage get events/2024/01/event.json
```

---

## companion

EDI companion guide operations.

### Usage

```bash
fi-fhir companion <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `list` | List available companion guides |
| `show NAME` | Show companion guide details |
| `validate` | Validate EDI against companion guide |

```bash
# List built-in guides
fi-fhir companion list

# Show guide details
fi-fhir companion show bcbs-837p

# Validate EDI
fi-fhir companion validate --guide bcbs-837p claim.x12
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `FI_FHIR_CONFIG` | Configuration file path |
| `FI_FHIR_LOG_LEVEL` | Log level (debug, info, warn, error) |
| `FI_FHIR_DATABASE_URL` | Database connection string |
| `FI_FHIR_FHIR_ENDPOINT` | Default FHIR server endpoint |
| `FI_FHIR_TRACING_ENABLED` | Enable distributed tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry collector endpoint |
| `LLM_BASE_URL` | LLM API endpoint (OpenAI-compatible) |
| `LLM_API_KEY` | LLM API authentication key |
| `LLM_DEFAULT_MODEL` | Default model for LLM operations |
| `LLM_QUALITY_MODEL` | Model for complex LLM tasks |

---

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | General error |
| 2 | Configuration error |
| 3 | Validation error |
| 4 | Network/connectivity error |
| 5 | Authentication error |

---

## See Also

- [Getting Started](getting-started.md) - Quick start tutorial
- [Workflow Configuration](workflows.md) - Workflow DSL and testing
- [Terminology Management](terminology.md) - Vocabulary and mapping operations
- [Source Profiles](source-profiles.md) - Profile configuration
- [LLM-Powered Features](llm-features.md) - AI-assisted features
