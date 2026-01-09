# Workflow DSL Planning

This document describes the Workflow DSL for event routing, transformation, and action execution.

## Overview

The Workflow DSL provides a declarative way to define event processing pipelines:

```
Input Formats          Semantic Layer           Workflow Engine         Output/Actions
─────────────         ────────────────         ───────────────         ──────────────
HL7v2    ──┐          ┌─────────────┐          ┌─────────────┐          ┌─> FHIR API
Flatfile ──┼──────────┤ Canonical   ├──────────┤  Workflow   ├──────────┼─> REST Webhook
CSV      ──┤          │ Event Model │          │  Execution  │          ├─> Database
EDI X12  ──┘          └─────────────┘          └─────────────┘          └─> Message Queue
```

## YAML Configuration

### Basic Structure

```yaml
workflow:
  name: patient_adt_routing
  version: "1.0"

  routes:
    - name: patient_admits_to_fhir
      filter:
        event_type: patient_admit
        source: ["epic_adt", "cerner_adt"]
      transform:
        - set_field: patient.status = "active"
        - map_terminology: patient.race
      actions:
        - type: fhir
          endpoint: https://fhir.example.com/r4
          resource: Patient

    - name: critical_labs_alert
      filter:
        event_type: lab_result
        condition: result.interpretation == "critical"
      actions:
        - type: webhook
          url: https://alerts.example.com/critical
          method: POST
        - type: log
          level: warn
          message: "Critical lab result: {{.Test.Code}} = {{.Result.Value}}"
```

### Filter Expressions

Filters determine which events a route handles:

| Filter Type | Status | Description |
|------------|--------|-------------|
| `event_type` | ✅ Implemented | Match by event type(s) |
| `source` | ✅ Implemented | Match by source system(s) |
| `condition` | ✅ Implemented | CEL expressions for complex conditions |

```yaml
filter:
  # Match by event type(s)
  event_type: patient_admit
  event_type: [patient_admit, patient_update, patient_discharge]

  # Match by source system
  source: epic_adt
  source: [epic_adt, cerner_adt]

  # CEL expressions for complex conditions
  condition: event.patient.age >= 65
  condition: event.result.interpretation in ["H", "HH", "critical"]
  condition: has(event.patient.identifiers.mrn) && event.source == "lab_system"
```

### CEL Quick Reference

CEL (Common Expression Language) provides safe, sandboxed expression evaluation. The `event` variable contains the full event data.

| Expression Type | Example | Description |
|----------------|---------|-------------|
| **Equality** | `event.type == "patient_admit"` | Exact string match |
| **String methods** | `event.patient.mrn.startsWith("TEST")` | String prefix check |
| **Comparison** | `event.encounter.length > 3` | Numeric comparison |
| **Membership** | `event.source in ["epic", "cerner"]` | Check if value in list |
| **Logical AND** | `event.type == "lab_result" && event.source == "lab_system"` | Both conditions must match |
| **Logical OR** | `event.source == "epic" \|\| event.source == "cerner"` | Either condition matches |
| **Nested access** | `event.patient.identifiers.mrn` | Dot notation for nested fields |
| **Has check** | `has(event.patient.ssn)` | Check if field exists |
| **Negation** | `!(event.source == "test")` | Negate a condition |

**Implementation**: `internal/workflow/cel.go` - compiled expressions are cached for performance.

### Transform Operations

Transforms modify events before action execution. The original event is not modified; a copy is returned.

| Transform Type | Status | Description |
|---------------|--------|-------------|
| `set_field` | ✅ Implemented | Set field values with path notation |
| `map_terminology` | ✅ Implemented | Map codes between terminology systems |
| `redact` | ✅ Implemented | Remove sensitive fields |

Transforms are applied sequentially, and each receives the output of the previous transform:

```yaml
transform:
  # Set a field value
  - set_field: patient.status = "active"

  # Map using terminology service
  - map_terminology:
      field: test.code
      from: LOCAL
      to: LOINC

  # Enrich with external lookup
  - enrich:
      field: patient.insurance
      source: eligibility_api
      key: patient.mrn

  # Remove sensitive data
  - redact:
      fields: [patient.ssn, patient.dob]

  # Format transformation
  - format:
      field: patient.phone
      pattern: "+1 (###) ###-####"
```

### Action Types

| Action Type | Status | Description |
|------------|--------|-------------|
| `log` | ✅ Implemented | Log events with Go template messages |
| `webhook` | ✅ Implemented | POST to REST endpoints with auth |
| `fhir` | ✅ Implemented | POST to FHIR R4 servers with US Core mapping |
| `database` | ✅ Implemented | Insert/upsert to PostgreSQL/MySQL/SQLite |
| `queue` | ✅ Implemented | Publish to Kafka/RabbitMQ/NATS/SQS |

#### FHIR Action
Send to FHIR server:

```yaml
- type: fhir
  endpoint: https://fhir.example.com/r4
  operation: create               # create, update, upsert
  bundle: "true"                  # Send as transaction bundle
  timeout: 30s
  # Static token auth
  token: ${FHIR_TOKEN}
  # OR OAuth2 client credentials (takes priority over token)
  token_url: https://auth.example.com/token
  client_id: ${FHIR_CLIENT_ID}
  client_secret: ${FHIR_CLIENT_SECRET}
  scopes: patient/*.read system/*.write
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `endpoint` | Yes | FHIR server base URL |
| `operation` | No | `create` (default), `update`, or `upsert` |
| `bundle` | No | `"true"` to send as transaction bundle |
| `timeout` | No | Request timeout (default: 30s) |
| `token` | No | Static Bearer token |
| `token_url` | No | OAuth2 token endpoint (enables OAuth) |
| `client_id` | For OAuth | OAuth2 client ID |
| `client_secret` | For OAuth | OAuth2 client secret |
| `scopes` | No | OAuth2 scopes (space or comma separated) |
| `authorization` | No | Custom Authorization header (highest priority) |

**Implementation:** `internal/workflow/actions.go` (fhirAction), `internal/workflow/oauth.go` (OAuth2)

#### Webhook Action
HTTP callback:

```yaml
- type: webhook
  url: https://api.example.com/events/{{.type}}  # Supports templates
  method: POST
  timeout: 30s
  user_agent: fi-fhir/1.0
  token: ${WEBHOOK_TOKEN}             # Bearer token
  # OR custom authorization header
  authorization: Basic ${BASIC_AUTH}
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `url` | Yes | Webhook URL (supports Go templates) |
| `method` | No | HTTP method (default: POST) |
| `timeout` | No | Request timeout (default: 30s) |
| `token` | No | Bearer token |
| `authorization` | No | Custom Authorization header |
| `user_agent` | No | Custom User-Agent header |

**Implementation:** `internal/workflow/actions.go` (webhookAction)

#### Database Action
Store to database:

```yaml
- type: database
  connection: ${DATABASE_URL}
  table: events
  operation: insert                # insert, upsert
  conflict_on: event_id            # For upsert: conflict columns
  mapping_event_id: id             # Column mappings use mapping_<column> prefix
  mapping_event_type: type
  mapping_patient_mrn: patient.mrn # Dot notation for nested fields
  mapping_payload: __raw__         # Special value for full JSON
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `connection` | Yes | Database DSN (postgres://, mysql://, sqlite://) |
| `table` | Yes | Target table name |
| `operation` | No | `insert` (default) or `upsert` |
| `conflict_on` | For upsert | Comma-separated conflict columns |
| `mapping_<col>` | Yes (1+) | Map column to event field path |

**Implementation:** `internal/workflow/database.go` - Uses Go's `database/sql` interface. Users must register their own drivers.

#### Queue Action
Publish to message queue:

```yaml
- type: queue
  driver: kafka                    # kafka, rabbitmq, nats, sqs, log
  topic: healthcare.events.{{.type}}  # Supports Go templates
  key: patient.mrn                 # Message key for partitioning
  header_source: fi-fhir           # Headers use header_<name> prefix
  header_env: production
  brokers: localhost:9092          # Driver-specific config passed through
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `driver` | Yes | Queue driver name (kafka, rabbitmq, nats, sqs, log) |
| `topic` | Yes | Topic/queue name (supports Go templates) |
| `key` | No | Event field path for message key |
| `header_<name>` | No | Static headers to add to messages |
| Other | No | Driver-specific options passed to factory |

**Implementation:** `internal/workflow/queue.go` - Uses driver registry pattern. Users register drivers via `RegisterQueueDriver()`. Built-in `log` driver for testing.

#### Log Action
Log for debugging/audit:

```yaml
- type: log
  level: info                                           # debug, info, warn, error
  message: "Processed {{.type}} for {{.patient.mrn}}"   # Go template
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `level` | No | Log level: debug, info (default), warn, error |
| `message` | No | Message template (default: "Event processed") |

Note: `debug` level includes full event JSON in output. `warn` and `error` write to stderr.

**Implementation:** `internal/workflow/actions.go` (logAction)

## Go Implementation

### Core Types

```go
// Workflow represents a complete workflow configuration
type Workflow struct {
    Name    string   `yaml:"name"`
    Version string   `yaml:"version"`
    Routes  []Route  `yaml:"routes"`
}

// Route defines a single event processing route
type Route struct {
    Name       string      `yaml:"name"`
    Filter     Filter      `yaml:"filter"`
    Transforms []Transform `yaml:"transform"`
    Actions    []Action    `yaml:"actions"`
}

// Filter matches events for routing
type Filter struct {
    EventType  StringOrSlice `yaml:"event_type"`
    Source     StringOrSlice `yaml:"source"`
    Condition  string        `yaml:"condition"`  // CEL expression
}

// Transform modifies events
type Transform struct {
    SetField        string            `yaml:"set_field,omitempty"`
    MapTerminology  *TerminologyMap   `yaml:"map_terminology,omitempty"`
    Enrich          *EnrichConfig     `yaml:"enrich,omitempty"`
    Redact          *RedactConfig     `yaml:"redact,omitempty"`
}

// Action executes on matched events
type Action struct {
    Type     string                 `yaml:"type"`
    Config   map[string]interface{} `yaml:"-,inline"`
}
```

### Engine Interface

```go
// Engine processes events through workflow routes
type Engine struct {
    workflow     *Workflow
    actions      map[string]ActionHandler
    celEvaluator *CELEvaluator  // For condition expression evaluation
}

// CELEvaluator evaluates CEL expressions against events (internal/workflow/cel.go)
type CELEvaluator struct {
    env   *cel.Env
    cache map[string]cel.Program  // Compiled expressions cached for performance
    mu    sync.RWMutex
}

// ActionHandler executes a specific action type
type ActionHandler interface {
    Execute(event interface{}, config map[string]interface{}) error
}

// Process routes an event through the workflow
func (e *Engine) Process(event interface{}) []error {
    var errors []error

    for _, route := range e.workflow.Routes {
        if !e.matches(event, route.Filter) {
            continue
        }

        // Apply transforms
        transformed := event
        for _, t := range route.Transforms {
            transformed = e.transform(transformed, t)
        }

        // Execute actions
        for _, action := range route.Actions {
            handler := e.actions[action.Type]
            if err := handler.Execute(transformed, action.Config); err != nil {
                errors = append(errors, err)
            }
        }
    }

    return errors
}
```

## Built-in Actions

### Phase 1 (MVP)
- `log` - Structured logging
- `webhook` - HTTP callbacks
- `fhir` - FHIR R4 server integration

### Phase 2
- `database` - SQL/NoSQL storage
- `queue` - Message queue publishing

### Phase 3
- `email` - Email notifications
- `file` - File output
- `custom` - User-defined action plugins

## Example Workflows

### ADT to FHIR with OAuth

```yaml
workflow:
  name: adt_to_fhir
  version: "1.0"

  routes:
    - name: admits
      filter:
        event_type: patient_admit
      transform:
        - map_terminology:
            field: patient.race
            from: LOCAL
            to: CDC
      actions:
        - type: fhir
          endpoint: https://fhir.hospital.org/r4
          operation: upsert
          token_url: https://auth.hospital.org/token
          client_id: ${FHIR_CLIENT_ID}
          client_secret: ${FHIR_CLIENT_SECRET}
          scopes: patient/*.write
```

### Lab Results with Alerts

```yaml
workflow:
  name: lab_processing
  version: "1.0"

  routes:
    - name: all_labs
      filter:
        event_type: lab_result
      actions:
        - type: fhir
          endpoint: https://fhir.hospital.org/r4
          token: ${FHIR_TOKEN}

    - name: critical_labs
      filter:
        event_type: lab_result
        condition: event.result.interpretation in ["critical", "panic"]
      actions:
        - type: webhook
          url: https://alerts.hospital.org/critical
          method: POST
          token: ${ALERT_TOKEN}
        - type: log
          level: warn
          message: "CRITICAL LAB: {{.test.code}} = {{.result.value}}"
```

### Multi-Source Aggregation to Kafka

```yaml
workflow:
  name: patient_aggregation
  version: "1.0"

  routes:
    - name: epic_patients
      filter:
        source: epic_adt
      transform:
        - set_field: patient.source_system = "EPIC"
      actions:
        - type: queue
          driver: kafka
          topic: patients.unified
          key: patient.mrn
          brokers: kafka.hospital.org:9092

    - name: cerner_patients
      filter:
        source: cerner_adt
      transform:
        - set_field: patient.source_system = "CERNER"
      actions:
        - type: queue
          driver: kafka
          topic: patients.unified
          key: patient.mrn
          brokers: kafka.hospital.org:9092
```

### Event Audit Log to Database

```yaml
workflow:
  name: event_audit
  version: "1.0"

  routes:
    - name: audit_all_events
      filter:
        event_type: [patient_admit, patient_discharge, lab_result]
      actions:
        - type: database
          connection: ${DATABASE_URL}
          table: audit_log
          operation: insert
          mapping_event_id: id
          mapping_event_type: type
          mapping_source: source
          mapping_patient_mrn: patient.mrn
          mapping_timestamp: timestamp
          mapping_payload: __raw__
```

## CLI Integration

```bash
# Run workflow on parsed events
fi-fhir workflow run --config workflow.yaml events.json

# Validate workflow configuration
fi-fhir workflow validate workflow.yaml

# Dry-run to see what would happen
fi-fhir workflow dry-run --config workflow.yaml message.hl7

# Combined parse + workflow
fi-fhir parse -f hl7v2 message.hl7 | fi-fhir workflow run --config workflow.yaml -
```

## Implementation Plan

### Phase 1: Core Engine ✅
- [x] Define workflow YAML schema
- [x] Implement Workflow, Route, Filter types
- [x] Basic filter matching (event_type, source)
- [x] Log action (with template rendering)
- [x] Webhook action (with auth, timeout, templates)
- [x] Engine orchestration
- [x] DryRun mode for testing routes without execution

### Phase 2: Transforms & FHIR ✅
- [x] CEL expression evaluation for conditions (`internal/workflow/cel.go`)
- [x] Transform pipeline (`internal/workflow/transforms.go`)
  - [x] `set_field` - Set field values with path notation
  - [x] `map_terminology` - Map codes between terminology systems
  - [x] `redact` - Remove sensitive fields
- [x] FHIR action with US Core mapping (Patient, Encounter, Observation, DiagnosticReport)
- [x] FHIR transaction bundle support for multi-resource events
- [x] Bearer token authentication for FHIR
- [x] OAuth2 client credentials flow (`internal/workflow/oauth.go`)

### Phase 3: Advanced Actions ✅
- [x] Database action (`internal/workflow/database.go`)
  - [x] PostgreSQL, MySQL, SQLite support via database/sql
  - [x] Insert and upsert operations
  - [x] Field mapping with dot notation
  - [x] `__raw__` special value for full JSON payload
  - [x] Connection pooling with caching
- [x] Queue action (`internal/workflow/queue.go`)
  - [x] Driver registry pattern (bring your own Kafka/RabbitMQ/NATS/SQS)
  - [x] Topic templates with Go text/template support
  - [x] Message key for partitioning
  - [x] Custom headers
  - [x] Built-in log driver for testing/debugging
- [ ] Retry/error handling with exponential backoff

### Phase 4: CLI & Tooling ✅
- [x] `workflow run` command
- [x] `workflow validate` command
- [x] `workflow dry-run` command

## See Also

- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Profile configuration feeds into workflow event parsing
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - FHIR action uses US Core mapper for resource generation
- [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) - TypeScript Workflow class wraps CLI commands
- [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) - EDI events can be routed through workflows
