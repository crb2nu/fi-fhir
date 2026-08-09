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
      condition: event.result.interpretation in ["HH", "LL"]
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

Complex conditions using Common Expression Language (CEL). The `event`
variable is the event's JSON form, so field names are the lowercase
snake_case JSON keys (see `pkg/events`). A condition that references a
missing field evaluates as an error and the route does not match.

```yaml
filter:
  condition: event.patient.gender == "F"

filter:
  condition: event.encounter.class == "I"

filter:
  condition: event.result.interpretation in ["HH", "LL", "AA"]

filter:
  condition: |
    event.is_critical &&
    event.encounter.class == "I"
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
| `event.encounter.class` | Encounter class code (e.g. `"I"`, `"O"`) |
| `event.result.value` | Primary lab result value |
| `event.results.exists(o, o.result.interpretation == "HH")` | Any observation matches |
| `has(event.patient.email)` | Check field exists |
| `size(event.results)` | Collection size |
| `timestamp(event.appointment.start_time)` | Parse RFC 3339 string to timestamp |

## Transforms

Transforms modify events before sending to actions.

### set_field

Set or update a field. The value is a literal (quoted string, number, or
boolean) — function calls are not supported:

```yaml
transform:
  - set_field: patient.active = true
  - set_field: claim.status = "received"
```

### map_terminology

Map local codes to standard terminology. Requires a configured terminology
mapper; if the field is missing or no mapping is found, the event passes
through unchanged:

```yaml
transform:
  - map_terminology:
      field: test.code
      from: LOCAL
      to: LOINC
```

### redact

Remove sensitive fields from the event (the fields are deleted, not masked):

```yaml
transform:
  - redact:
      fields: [patient.phone, patient.address]
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

Action configuration is a flat map of string values. Nested YAML blocks
under an action are ignored — always use the flat keys shown below.

### FHIR Action

Send to a FHIR R4 server:

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    resource: Patient                    # Resource type (auto-detected if omitted)
    operation: create                    # create, update, upsert

    # Authentication (optional, pick one)
    token: my-static-bearer-token        # Static bearer token
    # authorization: "Basic ..."         # Custom Authorization header
    # OAuth2 client credentials:
    # token_url: https://auth.example.com/oauth2/token
    # client_id: my-client-id
    # client_secret: my-client-secret
    # scopes: system/Patient.write

    # Options
    validate_fhir: "true"                # Validate before sending
    bundle: "true"                       # Send as transaction bundle
```

### Webhook Action

HTTP request to any endpoint. The event is sent as the JSON body with
`Content-Type: application/json`; the URL supports templates:

```yaml
actions:
  - type: webhook
    url: https://api.example.com/events/{{.type}}
    method: POST                         # default POST
    token: my-api-token                  # Sets "Authorization: Bearer <token>"
    # authorization: "Basic ..."         # Custom Authorization header

    # Reliability
    retry_max: 3                         # Max retry attempts (0 disables)
    retry_delay: 1s                      # Initial retry delay
    retry_multiplier: "2.0"              # Backoff multiplier
    retry_max_delay: 30s                 # Delay cap
```

### Database Action

Write to a relational database. The driver is detected from the DSN prefix
(`postgres://`, `mysql://`, `sqlite://`). Column values are **event field
paths** (`mapping_<column>: <path>`), not templates:

```yaml
actions:
  - type: database
    connection: postgres://user:pass@db.example.com:5432/events
    operation: upsert                    # insert or upsert
    table: healthcare_events

    # Field mapping: column -> event field path
    mapping_id: id
    mapping_event_type: type
    mapping_patient_mrn: patient.mrn
    mapping_created_at: timestamp

    # Upsert conflict handling (comma-separated column list)
    conflict_on: id
```

### Queue Action

Publish to a message queue:

```yaml
actions:
  - type: queue
    driver: log                          # Built-in driver; register others
                                         # via workflow.RegisterQueueDriver
    topic: healthcare-events             # Supports templates

    # Message key: an event field path (for partitioning)
    key: patient.mrn

    # Static headers (header_<name>: value)
    header_pipeline: fi-fhir
```

The shipped binary includes only the `log` driver, which prints messages
to stdout. External brokers (Kafka, RabbitMQ, NATS, SQS) require
registering a driver factory in Go via `workflow.RegisterQueueDriver`.

### Email Action

Send email notifications:

```yaml
actions:
  - type: email
    smtp_host: smtp.example.com
    smtp_port: 587
    starttls: "true"
    username: alerts@hospital.com
    password: replace-with-password

    from: alerts@hospital.com
    to: oncall@hospital.com              # Comma-separated list

    subject: "Critical Lab Result: {{.patient.family_name}}"
    body: |
      Patient: {{.patient.given_name}} {{.patient.family_name}}
      MRN: {{.patient.mrn}}
      Test: {{.test.description}}
      Value: {{.result.value}} {{.result.unit}}
```

### File Action

Write to disk:

```yaml
actions:
  - type: file
    path: /data/events/{{.type}}/{{.id}}.json
    format: json                         # json, pretty, ndjson

    # Restrict writes to a base directory (path resolved under it)
    base_dir: /data/events

    # Permissions (octal, default 0600)
    perm: "0644"
```

### Log Action

Write to logs:

```yaml
actions:
  - type: log
    level: info                          # debug, info, warn, error
    message: "Processed: {{.type}} for {{.patient.mrn}}"
```

At `level: debug` the full event JSON is appended to the log line.

### Event Store Action

Write to event sourcing store:

```yaml
actions:
  - type: event_store
    connection: postgres://user:pass@db.example.com:5432/events
    stream: "patient-{{.patient.mrn}}"   # Supports templates

    # Metadata to include (metadata_<key>: value, supports templates)
    metadata_source: "{{.source}}"
    metadata_correlation_id: "{{.id}}"
```

### Exec Action

Run external command (with allowlist):

```yaml
actions:
  - type: exec
    command: /usr/local/bin/notify-script
    # Args as a JSON array or whitespace-separated string (supports templates)
    args: '["{{.type}}", "{{.patient.mrn}}"]'

    timeout: 30s

    # Comma-separated absolute paths allowed to run (required)
    allowlist: /usr/local/bin/notify-script,/usr/local/bin/audit-script
```

### LLM Extract Action

Extract clinical entities from document text using LLM:

```yaml
actions:
  - type: llm_extract
    model: qwen3-14b-quality             # Model to use
    document_type: progress_note         # Hint: progress_note, discharge_summary, consult_note
    min_confidence: "0.7"                # Minimum confidence threshold
    text_field: document.content         # Field containing clinical text
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
    model: qwen3-8b-fast
    fail_below: "0.5"                    # Fail route if score below threshold
```

Quality dimensions: completeness, accuracy, consistency, conformance, timeliness.

Results are added to the event under `quality_score`.

## Templates

Config values marked as template-capable (log `message`, webhook `url`,
file `path`, email `from`/`to`/`subject`/`body`, queue `topic`,
event_store `stream` and `metadata_*`, exec `args`/`stdin_template`) are
rendered with standard Go [text/template](https://pkg.go.dev/text/template).

The template data is the event's **JSON form**, so field paths use the
lowercase snake_case JSON key names defined in `pkg/events` — the same
names you see in `fi-fhir parse` output:

```yaml
message: "Patient admitted: {{.patient.family_name}} (MRN: {{.patient.mrn}})"
message: "Result: {{.result.value}} {{.result.unit}} for {{.test.description}}"
message: "First observation: {{(index .results 0).result.value}}"
message: "{{len .results}} observation(s) received from {{.source}}"
```

Only the built-in text/template functions are available (`printf`, `len`,
`index`, `slice`, comparison operators, and so on). There are **no** custom
functions such as `now`, `json`, `upper`, or `default`.

Rendering behavior:

- A path that does not exist in the event renders as `<no value>`.
- If a template fails to parse or execute (for example, it references an
  unknown function), the raw template string is used unchanged and a
  warning is logged.

| Useful built-in | Example |
|-----------------|---------|
| `index` | `{{(index .results 0).result.value}}` |
| `len` | `{{len .results}}` |
| `printf` | `{{printf "%s-%s" .type .id}}` |
| `if`/`else` | `{{if .is_critical}}CRITICAL{{else}}routine{{end}}` |

## Reliability Features

### Retry Configuration

Retry, circuit breaking, and rate limiting are configured per action with
flat keys (webhook and fhir actions):

```yaml
actions:
  - type: webhook
    url: https://api.example.com
    retry_max: 5                         # Max attempts (0 disables)
    retry_delay: 1s                      # Initial delay
    retry_max_delay: 30s                 # Delay cap
    retry_multiplier: "2.0"              # Backoff multiplier
```

### Circuit Breaker

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com
    circuit_breaker: "true"
    circuit_failure_threshold: "5"       # Failures before opening
    circuit_timeout: 60s                 # Time in open state before half-open
```

### Rate Limiting

```yaml
actions:
  - type: webhook
    url: https://api.example.com
    rate_limit: "true"
    rate_limit_rate: "100"               # Requests per second
    rate_limit_burst: "50"               # Maximum burst
```

Dead-letter queueing is available when embedding the workflow engine in Go
(`Engine.SetDLQ`); it is not configurable from workflow YAML.

## Environment Variables

Workflow YAML values are **literal** — `${VAR}` references are *not*
expanded by `fi-fhir`. If you need environment-specific values, render the
file before loading it, for example:

```bash
envsubst < workflow.yaml.tmpl > workflow.yaml
fi-fhir workflow validate workflow.yaml
```

## Complete Example

```yaml
workflow:
  name: hospital_integration
  version: "2.0"

  routes:
    # Critical lab results - immediate alert
    - name: critical_labs
      filter:
        event_type: lab_result
        condition: >
          event.is_critical ||
          event.results.exists(o,
            o.result.interpretation in ["HH", "LL", "AA"]
          )
      transform:
        - set_field: priority = "CRITICAL"
      actions:
        - type: webhook
          url: https://alerts.example.com/hooks/critical
          retry_max: 5
        - type: email
          smtp_host: smtp.example.com
          smtp_port: 587
          from: alerts@hospital.com
          to: oncall@hospital.com
          subject: "CRITICAL: Lab Result for {{.patient.family_name}}"
          body: "{{.test.description}}: {{.result.value}} {{.result.unit}} ({{.result.interpretation}})"

    # All patient events to FHIR
    - name: patients_to_fhir
      filter:
        event_type: [patient_admit, patient_discharge, patient_update]
      transform:
        - redact:
            fields: [patient.phone, patient.address]
      actions:
        - type: fhir
          endpoint: https://fhir.example.com/r4
          token_url: https://auth.example.com/oauth2/token
          client_id: my-client-id
          client_secret: my-client-secret
          circuit_breaker: "true"
          circuit_failure_threshold: "5"
          circuit_timeout: 60s

    # All events to data warehouse
    - name: data_warehouse
      filter: {}  # Match all
      transform:
        - redact:
            fields: [patient.phone, patient.address]
      actions:
        - type: database
          connection: postgres://etl:password@warehouse.example.com:5432/analytics
          operation: insert
          table: raw_events
          mapping_id: id
          mapping_type: type
          mapping_patient_mrn: patient.mrn
          mapping_created_at: timestamp
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
