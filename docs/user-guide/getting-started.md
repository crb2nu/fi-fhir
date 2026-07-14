# Getting Started with fi-fhir

This guide walks you through parsing your first healthcare message, understanding the output, and running a basic workflow.

## Installation

### Option 1: Build from Source

```bash
# Clone the repository
git clone https://gitlab.flexinfer.ai/libs/fi-fhir.git
cd fi-fhir

# Build the CLI
make build

# Verify installation
./bin/fi-fhir --version
```

### Option 2: Go Install

```bash
go install gitlab.flexinfer.ai/libs/fi-fhir/cmd/fi-fhir@latest
```

### Option 3: Docker

```bash
docker pull registry.gitlab.flexinfer.ai/libs/fi-fhir:latest
docker run registry.gitlab.flexinfer.ai/libs/fi-fhir:latest --version
```

### Option 4: TypeScript SDK

```bash
npm install @fi-fhir/sdk
```

## Parse Your First Message

Let's parse a sample HL7v2 ADT (Admit/Discharge/Transfer) message.

### 1. Create a Sample Message

Save this as `sample.hl7`:

```
MSH|^~\&|EPIC|HOSPITAL|FHIRSERVER|DEST|20240115120000||ADT^A01|12345|P|2.5
EVN|A01|20240115120000
PID|1||MRN12345^^^EPIC^MR||Smith^John^A||19800115|M|||123 Main St^^Springfield^IL^62701||555-123-4567|||||SSN123456789
PV1|1|I|ICU^101^A^HOSPITAL||||1234567890^Jones^Sarah^M^^^MD|||MED||||||||VN98765|||||||||||||||||||||||||20240115100000
```

### 2. Parse the Message

```bash
fi-fhir parse --format hl7v2 --pretty sample.hl7
```

### 3. Understand the Output

You'll see a JSON event like this:

```json
{
  "meta": {
    "id": "evt_abc123",
    "type": "patient_admit",
    "source": "default",
    "format": "HL7v2",
    "timestamp": "2024-01-15T12:00:00Z",
    "source_message_id": "12345"
  },
  "patient": {
    "mrn": "MRN12345",
    "name": {
      "family": "Smith",
      "given": ["John", "A"]
    },
    "birthDate": "1980-01-15",
    "gender": "M",
    "address": {
      "line": ["123 Main St"],
      "city": "Springfield",
      "state": "IL",
      "postalCode": "62701"
    }
  },
  "encounter": {
    "identifier": "VN98765",
    "class": "inpatient",
    "location": "ICU-101-A"
  }
}
```

**Key observations:**
- The message type `ADT^A01` became the semantic event `patient_admit`
- HL7 field paths like `PID.5` are now structured JSON (`patient.name`)
- Healthcare-specific encoding (like `^` delimiters) is handled automatically

## The Three-Phase Pipeline

fi-fhir processes messages through three distinct phases:

### Phase 1: Byte Normalization
- Character encoding detection (UTF-8, ISO-8859-1, etc.)
- Line ending normalization (CR, LF, CRLF)
- BOM marker handling

### Phase 2: Syntactic Parsing
- Delimiter extraction from MSH segment
- Segment and field splitting
- Escape sequence handling

### Phase 3: Semantic Extraction
- Event type classification (A01 → patient_admit)
- Identifier extraction and validation
- Field mapping to canonical model

This pipeline is visualized interactively in the [Playground](playground-tutorial.md).

## Run a Basic Workflow

Workflows route events to destinations based on filters.

### 1. Create a Workflow Configuration

Save this as `workflow.yaml`:

```yaml
workflow:
  name: basic_routing
  version: "1.0"

  routes:
    - name: log_admits
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Patient admitted: {{.Patient.Name.Family}}, {{.Patient.Name.Given}}"
```

### 2. Run the Workflow

```bash
# Parse and pipe to workflow
fi-fhir parse --format hl7v2 sample.hl7 | fi-fhir workflow run --config workflow.yaml

# Or use dry-run to see what would happen without side effects
fi-fhir parse --format hl7v2 sample.hl7 | fi-fhir workflow run --dry-run --config workflow.yaml
```

### 3. Expected Output

```
INFO  Patient admitted: Smith, [John A]
```

## Parse Different Formats

### CSV Patient Data

```bash
# Create sample CSV
cat > patients.csv << 'EOF'
mrn,first_name,last_name,dob,gender
12345,John,Smith,1980-01-15,M
67890,Jane,Doe,1990-06-20,F
EOF

# Parse
fi-fhir parse --format csv --pretty patients.csv
```

### EDI 837P Claim

```bash
# Parse an EDI claim file
fi-fhir parse --format edi --pretty claim.edi
```

### CDA/CCDA Document

```bash
# Parse a clinical document
fi-fhir parse --format cda --pretty document.xml
```

## Validate Configuration

Before running in production, validate your configurations:

```bash
# Validate workflow configuration
fi-fhir workflow validate workflow.yaml

# Validate source profile
fi-fhir validate profile epic_adt.yaml

# Show current configuration
fi-fhir config show
```

## Quick Start with Persistence

`fi-fhir serve` always exposes authenticated, stateless preview. It can also
mount the PostgreSQL-only HL7v2 production ingress when explicitly enabled.

### 1. Start PostgreSQL

```bash
# Using docker-compose (includes PostgreSQL, Qdrant, Temporal)
docker-compose up -d

# Or use an existing PostgreSQL instance
```

### 2. Configure Environment

```bash
export FI_FHIR_DATABASE_DRIVER=postgres
export FI_FHIR_DATABASE_HOST=localhost
export FI_FHIR_DATABASE_PORT=5432
export FI_FHIR_DATABASE_NAME=fi_fhir
export FI_FHIR_DATABASE_USERNAME=fi_fhir
export FI_FHIR_DATABASE_PASSWORD=fi_fhir_dev
export FI_FHIR_DATABASE_SSL_MODE=disable
export FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
export FI_FHIR_GRAPHQL_PRINCIPAL_ID=local-operator
export FI_FHIR_GRAPHQL_ROLES=integration:preview
export FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=http://localhost:5173
export FI_FHIR_INTEGRATION_REGISTRY_PATH="$PWD/testdata/golden/integration/adt-http/preview-registry.json"
```

To enable the durable endpoint, bind one credential to one integration in that
registry. Use a managed secret file outside local development.

```bash
export FI_FHIR_HTTP_INGRESS_AUTH_MODE=bearer
export FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID=local-adt-service
export FI_FHIR_HTTP_INGRESS_INTEGRATION_ID=adt-east
export FI_FHIR_HTTP_INGRESS_SECRET="$(openssl rand -hex 32)"
```

### 3. Start the Server

```bash
fi-fhir serve --workflow configs/adt-workflow.yaml --no-playground --no-introspection
```

You should see:
```
Profile store: PostgreSQL
Event store: PostgreSQL
Workflow lifecycle store: PostgreSQL
```

Or use the `make dev` shortcut which handles all of the above:

```bash
make dev
```

### 4. Open the Mapping Studio

Start the UI, paste the deployment bearer into its credential gate, and use the
HL7 workspace. Preview calls the one `previewIntegrationMessage` mutation and
does not persist the raw sample, run, event, or a delivery receipt.

The production endpoint accepts exact `POST /v1/hl7v2` with media type
`application/hl7-v2+er7`, `X-Fi-Fhir-Integration-ID`, bearer or HMAC
credentials, and optional `Idempotency-Key` and `X-Correlation-ID`. A `202`
means the receipt, canonical event, lineage, initial attempt, and outbox row
committed atomically. It does not mean the external destination completed.

### Graceful Degradation

When database env vars are absent, non-preview catalogs fall back to in-memory
stores with a warning. Durable ingress never falls back: enabling it without
PostgreSQL, a valid credential, or an exact registry binding stops startup.

## Next Steps

| To learn about... | Read... |
|-------------------|---------|
| Source Profile configuration | [Source Profiles](source-profiles.md) |
| Workflow filters and actions | [Workflows](workflows.md) |
| FHIR resource generation | [FHIR Output](fhir-output.md) |
| Interactive learning | [Playground Tutorial](playground-tutorial.md) |
| Full development setup | [Development Setup](../developer-guide/development-setup.md) |

## Common Issues

### "Unknown format"
Ensure you specify `--format` correctly: `hl7v2`, `csv`, `edi`, `cda`, or `fhir`.

### "Failed to parse message"
Check that your message is well-formed. Use `--verbose` flag for detailed error messages:
```bash
fi-fhir parse --format hl7v2 --verbose sample.hl7
```

### "Warning: Missing segment"
This is often expected behavior. Healthcare data is messy, and fi-fhir records warnings while continuing to parse. See [Source Profiles](source-profiles.md) for tolerance configuration.
