# fi-fhir Examples

This directory contains example configurations and workflows for common healthcare integration scenarios.

## Workflow Examples

| File | Description |
|------|-------------|
| [adt-to-fhir.yaml](workflows/adt-to-fhir.yaml) | Route ADT events to FHIR server |
| [lab-results-routing.yaml](workflows/lab-results-routing.yaml) | Multi-destination lab result routing with critical alerts |
| [claims-processing.yaml](workflows/claims-processing.yaml) | EDI 837/835 claims processing pipeline |
| [appointment-sync.yaml](workflows/appointment-sync.yaml) | Scheduling sync across EHR, portal, and reminder systems |

## Running Examples

### 1. ADT to FHIR

Routes HL7v2 ADT messages (admits, discharges, transfers) to a FHIR server:

```bash
# Set environment variables
export FHIR_SERVER_URL=http://localhost:8090/fhir
export FHIR_TOKEN_URL=http://auth.example.com/oauth/token
export FHIR_CLIENT_ID=fi-fhir
export FHIR_CLIENT_SECRET=secret

# Parse and process an ADT message
fi-fhir parse --format hl7v2 testdata/adt_a01_sample.hl7 | \
  fi-fhir workflow run --config examples/workflows/adt-to-fhir.yaml

# Or in dry-run mode
fi-fhir workflow run --dry-run --config examples/workflows/adt-to-fhir.yaml event.json
```

### 2. Lab Results Routing

Routes lab results to FHIR, sends critical alerts, and stores in data warehouse:

```bash
# Set environment variables
export FHIR_SERVER_URL=http://localhost:8090/fhir
export FHIR_BEARER_TOKEN=your-token
export ALERT_WEBHOOK_URL=https://alerts.example.com/critical
export PAGER_WEBHOOK_URL=https://pager.example.com/notify
export WAREHOUSE_DSN=postgres://user:pass@localhost:5432/warehouse
export KAFKA_BROKERS=localhost:9092

# Process lab result
fi-fhir parse --format hl7v2 testdata/oru_r01_sample.hl7 | \
  fi-fhir workflow run --config examples/workflows/lab-results-routing.yaml
```

### 3. Claims Processing

Processes EDI 837 claims and 835 remittance advice:

```bash
# Set environment variables
export CLAIMS_DSN=postgres://user:pass@localhost:5432/claims
export CLEARINGHOUSE_URL=https://clearinghouse.example.com/api
export SENDER_ID=YOUR_SENDER_ID
export KAFKA_BROKERS=localhost:9092
export BILLING_WEBHOOK_URL=https://billing.example.com/webhook
export ALERT_WEBHOOK_URL=https://alerts.example.com/claims

# Process 837P claim
fi-fhir parse --format edi testdata/837p_sample.edi | \
  fi-fhir workflow run --config examples/workflows/claims-processing.yaml

# Process 835 remittance
fi-fhir parse --format edi testdata/835_sample.edi | \
  fi-fhir workflow run --config examples/workflows/claims-processing.yaml
```

### 4. Appointment Sync

Synchronizes scheduling events across systems:

```bash
# Set environment variables
export FHIR_SERVER_URL=http://localhost:8090/fhir
export FHIR_BEARER_TOKEN=your-token
export SCHEDULING_DSN=postgres://user:pass@localhost:5432/scheduling
export PATIENT_PORTAL_URL=https://portal.example.com/api
export PORTAL_API_KEY=your-api-key
export KAFKA_BROKERS=localhost:9092
export FRONT_DESK_WEBHOOK=https://internal.example.com/frontdesk

# Process SIU message
fi-fhir parse --format hl7v2 testdata/siu_s12_sample.hl7 | \
  fi-fhir workflow run --config examples/workflows/appointment-sync.yaml
```

## Environment Variables

All examples use environment variable substitution (`${VAR_NAME}`) for sensitive configuration. Set these before running:

| Variable | Description |
|----------|-------------|
| `FHIR_SERVER_URL` | FHIR R4 server base URL |
| `FHIR_BEARER_TOKEN` | Bearer token for FHIR auth |
| `FHIR_TOKEN_URL` | OAuth2 token endpoint |
| `FHIR_CLIENT_ID` | OAuth2 client ID |
| `FHIR_CLIENT_SECRET` | OAuth2 client secret |
| `KAFKA_BROKERS` | Kafka broker addresses |
| `*_DSN` | Database connection strings |
| `*_WEBHOOK_URL` | Webhook endpoints |

## Testing with Docker Compose

Use the local development stack for testing:

```bash
# Start all dependencies
docker-compose up -d

# Access services:
# - HAPI FHIR: http://localhost:8090/fhir
# - PostgreSQL: localhost:5432
# - Kafka: localhost:9092
# - Jaeger UI: http://localhost:16686

# Set environment for local testing
export FHIR_SERVER_URL=http://localhost:8090/fhir
export CLAIMS_DSN=postgres://postgres:postgres@localhost:5432/fi_fhir
export KAFKA_BROKERS=localhost:9092

# Run example
fi-fhir workflow run --dry-run --config examples/workflows/adt-to-fhir.yaml event.json
```

## Customizing Examples

These examples are starting points. Common customizations:

### Add Custom Transformations

```yaml
transform:
  # Map local codes to standard terminologies
  - map_terminology: patient.race
  - map_terminology: observation.code

  # Set computed fields
  - set_field: patient.fullName = patient.name.given + " " + patient.name.family

  # Redact sensitive data
  - redact: patient.ssn
```

### Add Retry Logic

```yaml
actions:
  - type: webhook
    url: ${WEBHOOK_URL}
    retry:
      maxAttempts: 3
      initialDelay: 1s
      maxDelay: 30s
      backoffMultiplier: 2
```

### Add Conditional Actions

```yaml
actions:
  - type: webhook
    url: ${ALERT_URL}
    condition: event.observation.interpretation == "critical"
```

### Filter with CEL Expressions

```yaml
filter:
  condition: >
    event.patient.age >= 65 &&
    event.encounter.class == "inpatient" &&
    event.source in ["epic", "cerner"]
```

## Creating New Workflows

1. Copy an example that's closest to your use case
2. Modify filters to match your event types
3. Add/remove actions for your destinations
4. Set environment variables for credentials
5. Test with `--dry-run` before production use

```bash
# Validate your workflow configuration
fi-fhir workflow validate my-workflow.yaml

# Test with dry-run
fi-fhir workflow run --dry-run --config my-workflow.yaml test-event.json

# Run in production
fi-fhir workflow run --config my-workflow.yaml
```
