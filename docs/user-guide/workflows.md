# Workflow Configuration

Workflows route semantic events to destinations based on filters, transforms, and actions. This guide covers the complete workflow DSL.

## Basic Structure

```yaml
workflow:
  name: my_workflow
  version: "1.0"

  routes:
    - name: route_name
      filter:
        # Which events to match
      transform:
        # How to modify events
      actions:
        # Where to send events
```

## Routes

Routes are processed in order. An event can match multiple routes.

```yaml
routes:
  - name: critical_alerts
    filter:
      event_type: lab_result
      condition: event.interpretation == "critical"
    actions:
      - type: webhook
        url: https://alerts.example.com/critical

  - name: all_labs_to_fhir
    filter:
      event_type: lab_result
    actions:
      - type: fhir
        endpoint: https://fhir.example.com/r4
```

## Filters

### Event Type Filter

Match by semantic event type:

```yaml
filter:
  event_type: patient_admit              # Single type

filter:
  event_type: [patient_admit, patient_discharge]  # Multiple types
```

### Source Filter

Match by source system:

```yaml
filter:
  source: epic_adt                       # Single source

filter:
  source: [epic_adt, cerner_adt]         # Multiple sources
```

### CEL Expressions

Complex conditions using Common Expression Language (CEL):

```yaml
filter:
  condition: event.patient.age >= 65

filter:
  condition: event.encounter.class == "inpatient"

filter:
  condition: event.observation.interpretation in ["critical", "HH", "LL"]

filter:
  condition: |
    event.patient.age >= 65 &&
    event.encounter.class == "inpatient"
```

### Combined Filters

All filter conditions must match (AND logic):

```yaml
filter:
  event_type: patient_admit
  source: epic_adt
  condition: event.encounter.class == "inpatient"
```

### CEL Expression Reference

| Expression | Description |
|------------|-------------|
| `event.type` | Event type string |
| `event.patient.mrn` | Patient MRN |
| `event.patient.age` | Calculated age |
| `event.encounter.class` | Encounter classification |
| `event.observation.value` | Observation value |
| `has(event.patient.ssn)` | Check field exists |
| `size(event.patient.identifiers)` | Collection size |

## Transforms

Transforms modify events before sending to actions.

### set_field

Set or update a field:

```yaml
transform:
  - set_field: patient.status = "active"
  - set_field: processed_at = now()
  - set_field: meta.custom_field = "value"
```

### map_terminology

Map local codes to standard terminology:

```yaml
transform:
  - map_terminology: patient.race        # Use configured mapping
  - map_terminology:
      field: observation.code
      source: LOCAL
      target: LOINC
```

### redact

Remove sensitive data:

```yaml
transform:
  - redact: patient.ssn
  - redact: patient.address
  - redact:
      fields: [patient.ssn, patient.phone]
      replacement: "[REDACTED]"
```

### copy_field

Copy value to another field:

```yaml
transform:
  - copy_field:
      source: patient.mrn
      target: meta.patient_id
```

### delete_field

Remove a field:

```yaml
transform:
  - delete_field: patient.internal_id
```

## Actions

Actions send events to destinations.

### FHIR Action

Send to a FHIR R4 server:

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    resource: Patient                    # Resource type to create

    # Authentication (optional)
    auth:
      type: oauth2
      tokenUrl: https://auth.example.com/token
      clientId: ${CLIENT_ID}
      clientSecret: ${CLIENT_SECRET}
      scopes: [system/Patient.write]

    # Options
    validate_fhir: true                  # Validate before sending
    batch: true                          # Use batch endpoint
```

### Webhook Action

HTTP POST to any endpoint:

```yaml
actions:
  - type: webhook
    url: https://api.example.com/events
    method: POST                         # POST, PUT, PATCH

    headers:
      Authorization: Bearer ${API_KEY}
      Content-Type: application/json

    # Reliability
    retry:
      maxAttempts: 3
      backoffMultiplier: 2
      initialDelay: 1s
```

### Database Action

Write to a relational database:

```yaml
actions:
  - type: database
    driver: postgres                     # postgres, mysql, sqlite
    dsn: ${DATABASE_URL}

    operation: upsert                    # insert, upsert, update
    table: healthcare_events

    # Field mapping
    fields:
      id: "{{.Meta.ID}}"
      event_type: "{{.Meta.Type}}"
      patient_mrn: "{{.Patient.MRN}}"
      encounter_id: "{{.Encounter.Identifier}}"
      payload: "{{. | json}}"
      created_at: "{{now}}"

    # Upsert conflict handling
    conflictColumns: [id]
```

### Queue Action

Publish to a message queue:

```yaml
actions:
  - type: queue
    driver: kafka                        # kafka, rabbitmq, nats, sqs
    brokers: ${KAFKA_BROKERS}
    topic: healthcare-events

    # Message key (for partitioning)
    key: "{{.Patient.MRN}}"

    # Headers
    headers:
      event_type: "{{.Meta.Type}}"
      source: "{{.Meta.Source}}"
```

### Email Action

Send email notifications:

```yaml
actions:
  - type: email
    smtp:
      host: smtp.example.com
      port: 587
      username: ${SMTP_USER}
      password: ${SMTP_PASS}

    from: alerts@hospital.com
    to: [oncall@hospital.com]

    subject: "Critical Lab Result: {{.Patient.Name.Family}}"
    body: |
      Patient: {{.Patient.Name.Given}} {{.Patient.Name.Family}}
      MRN: {{.Patient.MRN}}
      Test: {{.Observation.Code.Display}}
      Value: {{.Observation.Value}}
```

### File Action

Write to disk:

```yaml
actions:
  - type: file
    path: /data/events/{{.Meta.Type}}/{{.Meta.ID}}.json
    format: json                         # json, yaml, csv

    # Atomic writes (write to temp, then rename)
    atomic: true

    # Permissions
    mode: 0644
```

### Log Action

Write to logs:

```yaml
actions:
  - type: log
    level: info                          # debug, info, warn, error
    message: "Processed: {{.Meta.Type}} for {{.Patient.MRN}}"

    # Include full event
    include_event: false
```

### Event Store Action

Write to event sourcing store:

```yaml
actions:
  - type: eventstore
    stream: "patient-{{.Patient.MRN}}"

    # Metadata to include
    metadata:
      source: "{{.Meta.Source}}"
      correlation_id: "{{.Meta.ID}}"
```

### Exec Action

Run external command (with allowlist):

```yaml
actions:
  - type: exec
    command: /usr/local/bin/notify-script
    args:
      - "{{.Meta.Type}}"
      - "{{.Patient.MRN}}"

    timeout: 30s

    # Must be in allowlist
    allowlist:
      - /usr/local/bin/notify-script
      - /usr/local/bin/audit-script
```

## Template Functions

Templates use Go text/template with additional functions:

| Function | Description | Example |
|----------|-------------|---------|
| `now` | Current timestamp | `{{now}}` |
| `json` | JSON encode | `{{. \| json}}` |
| `upper` | Uppercase | `{{.Patient.MRN \| upper}}` |
| `lower` | Lowercase | `{{.Meta.Type \| lower}}` |
| `replace` | String replace | `{{.Value \| replace "old" "new"}}` |
| `default` | Default value | `{{.Field \| default "N/A"}}` |

## Reliability Features

### Retry Configuration

```yaml
actions:
  - type: webhook
    url: https://api.example.com
    retry:
      maxAttempts: 5
      initialDelay: 1s
      maxDelay: 30s
      backoffMultiplier: 2
```

### Circuit Breaker

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com
    circuit_breaker:
      threshold: 5                       # Failures before opening
      timeout: 60s                       # Time before retry
```

### Dead Letter Queue

```yaml
workflow:
  name: with_dlq
  dlq:
    enabled: true
    type: file
    path: /data/dlq/

    # Or send to queue
    # type: queue
    # driver: kafka
    # topic: dlq-events
```

### Rate Limiting

```yaml
workflow:
  name: rate_limited
  rate_limit:
    requests_per_second: 100
    burst: 50
```

## Environment Variables

Reference environment variables with `${VAR}`:

```yaml
actions:
  - type: fhir
    endpoint: ${FHIR_ENDPOINT}
    auth:
      clientId: ${FHIR_CLIENT_ID}
      clientSecret: ${FHIR_CLIENT_SECRET}
```

## Complete Example

```yaml
workflow:
  name: hospital_integration
  version: "2.0"

  rate_limit:
    requests_per_second: 100

  dlq:
    enabled: true
    type: file
    path: /data/dlq/

  routes:
    # Critical lab results - immediate alert
    - name: critical_labs
      filter:
        event_type: lab_result
        condition: event.observation.interpretation in ["critical", "HH", "LL"]
      transform:
        - set_field: priority = "CRITICAL"
      actions:
        - type: webhook
          url: ${ALERT_WEBHOOK_URL}
          retry:
            maxAttempts: 5
        - type: email
          from: alerts@hospital.com
          to: [oncall@hospital.com]
          subject: "CRITICAL: Lab Result for {{.Patient.Name.Family}}"

    # All patient events to FHIR
    - name: patients_to_fhir
      filter:
        event_type: [patient_admit, patient_discharge, patient_update]
      transform:
        - redact: patient.ssn
      actions:
        - type: fhir
          endpoint: ${FHIR_ENDPOINT}
          auth:
            type: oauth2
            tokenUrl: ${FHIR_TOKEN_URL}
            clientId: ${FHIR_CLIENT_ID}
            clientSecret: ${FHIR_CLIENT_SECRET}
          circuit_breaker:
            threshold: 5
            timeout: 60s

    # All events to data warehouse
    - name: data_warehouse
      filter: {}  # Match all
      transform:
        - redact: [patient.ssn, patient.address]
      actions:
        - type: database
          driver: postgres
          dsn: ${DW_DATABASE_URL}
          operation: insert
          table: raw_events
          fields:
            id: "{{.Meta.ID}}"
            type: "{{.Meta.Type}}"
            payload: "{{. | json}}"
            created_at: "{{now}}"
```

## CLI Commands

### Validate Workflow

```bash
fi-fhir workflow validate workflow.yaml
```

### Run Workflow

```bash
# From stdin
cat events.json | fi-fhir workflow run --config workflow.yaml

# From file
fi-fhir workflow run --config workflow.yaml events.json

# Dry-run (no side effects)
fi-fhir workflow run --dry-run --config workflow.yaml events.json
```

### Test with Simulation

```bash
fi-fhir workflow simulate --config workflow.yaml --events test_events.json
```

## See Also

- [Planning: WORKFLOW-DSL.md](../planning/WORKFLOW-DSL.md) - Complete DSL specification
- [FHIR Output](fhir-output.md) - FHIR action details
- [Core Concepts](core-concepts.md) - Architecture overview
