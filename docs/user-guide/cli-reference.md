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
| `--format FORMAT` | Input format: `hl7v2`, `csv`, `edi`, `cda`, `fhir` |
| `--profile FILE` | Source profile to use |
| `--pretty` | Pretty-print JSON output |
| `--source NAME` | Source system name |
| `--output FILE` | Output file (default: stdout) |

### Examples

```bash
# Parse HL7v2 message
fi-fhir parse --format hl7v2 --pretty message.hl7

# Parse with custom profile
fi-fhir parse --format hl7v2 --profile epic_adt.yaml message.hl7

# Parse EDI claim
fi-fhir parse --format edi --pretty claim.x12

# Parse CSV file
fi-fhir parse --format csv --pretty patients.csv

# Parse CDA document
fi-fhir parse --format cda --pretty clinical_doc.xml

# Parse multiple files
fi-fhir parse --format hl7v2 messages/*.hl7 > all_events.json
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
| `--host HOST` | Bind address (default: 0.0.0.0) |
| `--port PORT` | Port number (default: 8080) |
| `--tls-cert FILE` | TLS certificate file |
| `--tls-key FILE` | TLS key file |

```bash
# Start server
fi-fhir serve --port 8080

# With TLS
fi-fhir serve --port 8443 --tls-cert cert.pem --tls-key key.pem
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

Terminology database operations.

### Usage

```bash
fi-fhir terminology <subcommand> [options]
```

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `init` | Initialize terminology database |
| `load SYSTEM` | Load terminology (loinc, snomed, icd10) |
| `status` | Show loaded terminologies |
| `crosswalk` | Cross-reference codes between systems |

```bash
# Initialize database
fi-fhir terminology init --driver postgres --dsn ${DATABASE_URL}

# Load LOINC
fi-fhir terminology load loinc --file loinc.csv

# Check status
fi-fhir terminology status

# Cross-walk ICD-10 to SNOMED
fi-fhir terminology crosswalk --from icd10 --to snomed E11.9
```

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
- [Workflow Configuration](workflows.md) - Workflow DSL
- [Source Profiles](source-profiles.md) - Profile configuration
