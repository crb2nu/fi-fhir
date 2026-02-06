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

### explain_warnings

Add LLM-powered explanations to parse warnings:

```yaml
transform:
  - explain_warnings:
      model: qwen3-8b-fast      # Optional: model override
      include_fix: true          # Include fix suggestions
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

### LLM Extract Action

Extract clinical entities from document text using LLM:

```yaml
actions:
  - type: llm_extract
    config:
      model: qwen3-14b-quality           # Model to use
      document_type: progress_note       # Hint: progress_note, discharge_summary, consult_note
      min_confidence: 0.7                # Minimum confidence threshold
      text_field: document.content       # Field containing clinical text
```

Extracted entities are added to the event under `extracted_entities`:
- Conditions (SNOMED CT, ICD-10)
- Medications (RxNorm)
- Vital Signs (LOINC)
- Allergies, Procedures

### LLM Quality Check Action

Analyze data quality and optionally fail the route:

```yaml
actions:
  - type: llm_quality_check
    config:
      model: qwen3-8b-fast
      fail_below: 0.5                    # Fail route if score below threshold
```

Quality dimensions: completeness, accuracy, consistency, conformance, timeliness.

Results are added to the event under `quality_score`.

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

---

## Testing & Validation

fi-fhir provides dedicated commands for testing workflows without affecting production systems, recording events for regression testing, and load testing workflow performance.

### Dry-Run Mode

Execute workflows without triggering actual side effects. Actions are simulated and their would-be outputs are logged.

```bash
# Dry-run from file
fi-fhir workflow dry-run -c workflow.yaml events.json

# Dry-run from stdin
cat events.json | fi-fhir workflow dry-run -c workflow.yaml -

# Verbose output showing route matching
fi-fhir workflow dry-run -c workflow.yaml -v events.json
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-v, --verbose` | Show detailed route matching information |

Dry-run output shows which routes matched, transforms applied, and actions that would execute:

```json
{
  "event_id": "evt_001",
  "matched_routes": ["critical_labs", "all_events"],
  "transforms_applied": 2,
  "actions_simulated": [
    {"route": "critical_labs", "action": "webhook", "url": "https://alerts.example.com"},
    {"route": "all_events", "action": "database", "table": "events"}
  ]
}
```

### Recording Events

Capture events and their workflow results for regression testing. Recordings create a baseline to compare against future workflow changes.

```bash
# Record events to JSON file
fi-fhir workflow record -c workflow.yaml -o recordings.json events.json

# Record from stdin
cat events.json | fi-fhir workflow record -c workflow.yaml -o baseline.json -
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-o, --output` | Output file for recordings (required) |

Recording format captures the event, matched routes, and action outputs:

```json
{
  "recorded_at": "2024-01-15T10:30:00Z",
  "workflow_version": "2.0",
  "events": [
    {
      "event": { "type": "lab_result", "..." },
      "routes_matched": ["critical_labs"],
      "action_results": [
        {
          "action": "webhook",
          "status": 200,
          "response_hash": "abc123..."
        }
      ]
    }
  ]
}
```

### Replay and Compare

Replay recorded events through a workflow and compare results against the baseline. Essential for validating workflow changes don't break existing behavior.

```bash
# Basic replay with diff output
fi-fhir workflow replay -c workflow.yaml -d recordings.json

# Filter by event type
fi-fhir workflow replay -c workflow.yaml -t patient_admit recordings.json

# Filter by source system
fi-fhir workflow replay -c workflow.yaml -s epic_adt recordings.json

# Limit number of events
fi-fhir workflow replay -c workflow.yaml -l 100 recordings.json

# Save comparison results
fi-fhir workflow replay -c workflow.yaml -o results.json recordings.json
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-r, --recordings` | Recordings file to replay |
| `-t, --event-type` | Filter by event type |
| `-s, --source` | Filter by source system |
| `-l, --limit` | Maximum events to replay |
| `-d, --diffs` | Show diffs for mismatches |
| `-o, --output` | Save comparison results to file |

Replay output shows pass/fail status and differences:

```
Replaying 150 events...
  ✓ 147 passed
  ✗ 3 failed

Failed events:
  evt_042: Route mismatch
    - Expected: [critical_labs, all_events]
    + Actual:   [all_events]

  evt_089: Action output changed
    - webhook response: {"status": "sent"}
    + webhook response: {"status": "queued"}
```

### Load Testing

Performance test workflows under various load conditions. Identifies bottlenecks and validates throughput requirements.

```bash
# Quick smoke test
fi-fhir workflow loadtest -c workflow.yaml -s smoke -v

# Standard load test
fi-fhir workflow loadtest -c workflow.yaml -s standard

# Custom parameters
fi-fhir workflow loadtest -c workflow.yaml -d 60s -r 2000 -w 8 -v

# Stress test with JSON output
fi-fhir workflow loadtest -c workflow.yaml -s stress --json
```

| Option | Description |
|--------|-------------|
| `-c, --config` | Workflow configuration file (required) |
| `-s, --scenario` | Predefined scenario (see below) |
| `-d, --duration` | Test duration (e.g., 30s, 5m) |
| `-r, --rps` | Target requests per second |
| `-w, --workers` | Number of concurrent workers |
| `--warmup` | Warmup duration before measuring |
| `-v, --verbose` | Show real-time metrics |
| `--json` | Output results as JSON |

#### Predefined Scenarios

| Scenario | Duration | RPS | Workers | Purpose |
|----------|----------|-----|---------|---------|
| `smoke` | 10s | 100 | 2 | Quick validation after changes |
| `standard` | 60s | 1000 | 4 | Normal production load simulation |
| `stress` | 120s | 5000 | 8 | High load boundary testing |
| `burst` | 30s | unlimited | 16 | Maximum throughput discovery |
| `soak` | 5min | 500 | 4 | Memory leak and stability testing |

#### Load Test Output

```
Load Test: workflow.yaml
Scenario: standard (60s @ 1000 RPS)

Running... ████████████████████████████████ 60s

Results:
  Total Requests:     59,847
  Successful:         59,812 (99.94%)
  Failed:             35 (0.06%)

  Throughput:         997.5 req/s
  Avg Latency:        12.3ms
  P50 Latency:        8.2ms
  P95 Latency:        34.1ms
  P99 Latency:        89.7ms

  Route Performance:
    critical_labs:    2.1ms avg (1,203 matches)
    patients_to_fhir: 15.4ms avg (18,402 matches)
    data_warehouse:   8.7ms avg (59,847 matches)
```

### Testing Best Practices

1. **Start with smoke tests**: Run `smoke` scenario after every workflow change
2. **Build regression baselines**: Record production event samples for replay testing
3. **Test in isolation**: Use dry-run mode before connecting to real systems
4. **Version your recordings**: Store recordings alongside workflow configs in version control
5. **Automate in CI/CD**: Include workflow validation and replay tests in pipelines

```bash
# Example CI/CD workflow
fi-fhir workflow validate workflow.yaml
fi-fhir workflow dry-run -c workflow.yaml test_events.json
fi-fhir workflow replay -c workflow.yaml recordings/baseline.json
fi-fhir workflow loadtest -c workflow.yaml -s smoke
```

## See Also

- [Planning: WORKFLOW-DSL.md](../planning/WORKFLOW-DSL.md) - Complete DSL specification
- [FHIR Output](fhir-output.md) - FHIR action details
- [Core Concepts](core-concepts.md) - Architecture overview
- [LLM-Powered Features](llm-features.md) - AI-assisted features and LLM configuration
