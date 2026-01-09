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

### Transform Operations

Transforms modify events before action execution:

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

#### FHIR Action
Send to FHIR server:

```yaml
- type: fhir
  endpoint: https://fhir.example.com/r4
  resource: Patient          # or Observation, Encounter, etc.
  operation: create          # create, update, upsert
  profile: us-core           # Apply US Core profile mapping
  auth:
    type: oauth2
    token_url: https://auth.example.com/token
    client_id: ${FHIR_CLIENT_ID}
    client_secret: ${FHIR_CLIENT_SECRET}
  retry:
    attempts: 3
    backoff: exponential
```

#### Webhook Action
HTTP callback:

```yaml
- type: webhook
  url: https://api.example.com/events
  method: POST
  headers:
    Content-Type: application/json
    X-Source: fi-fhir
  body: json               # json, xml, form
  auth:
    type: bearer
    token: ${WEBHOOK_TOKEN}
  timeout: 30s
  retry:
    attempts: 3
```

#### Database Action
Store to database:

```yaml
- type: database
  connection: ${DATABASE_URL}
  table: events
  operation: insert         # insert, upsert
  mapping:
    event_id: id
    event_type: type
    patient_mrn: patient.mrn
    payload: __raw__        # Full event JSON
```

#### Queue Action
Publish to message queue:

```yaml
- type: queue
  driver: kafka             # kafka, rabbitmq, sqs, nats
  topic: healthcare.events.{{.Type}}
  config:
    brokers: ["kafka:9092"]
  message:
    key: patient.mrn
    headers:
      source: fi-fhir
```

#### Log Action
Log for debugging/audit:

```yaml
- type: log
  level: info               # debug, info, warn, error
  message: "Processed {{.Type}} for {{.Patient.MRN}}"
  fields:
    event_id: id
    source: source
```

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
    workflow *Workflow
    actions  map[string]ActionHandler
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

### ADT to FHIR

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
          resource: Patient
          operation: upsert
          match_by: identifier
        - type: fhir
          endpoint: https://fhir.hospital.org/r4
          resource: Encounter
          operation: create
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
          resource: Observation
          profile: us-core

    - name: critical_labs
      filter:
        event_type: lab_result
        condition: result.interpretation in ["critical", "panic"]
      actions:
        - type: webhook
          url: https://alerts.hospital.org/critical
          method: POST
        - type: log
          level: warn
          message: "CRITICAL LAB: {{.Test.Code}} = {{.Result.Value}}"
```

### Multi-Source Aggregation

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

    - name: cerner_patients
      filter:
        source: cerner_adt
      transform:
        - set_field: patient.source_system = "CERNER"
      actions:
        - type: queue
          driver: kafka
          topic: patients.unified
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

### Phase 1: Core Engine
- [x] Define workflow YAML schema
- [ ] Implement Workflow, Route, Filter types
- [ ] Basic filter matching (event_type, source)
- [ ] Log action
- [ ] Webhook action
- [ ] Engine orchestration

### Phase 2: Transforms & FHIR
- [ ] CEL expression evaluation for conditions
- [ ] Transform pipeline
- [ ] FHIR action with US Core mapping
- [ ] OAuth2 authentication

### Phase 3: Advanced Actions
- [ ] Database action
- [ ] Queue action (Kafka, RabbitMQ)
- [ ] Retry/error handling

### Phase 4: CLI & Tooling
- [ ] `workflow run` command
- [ ] `workflow validate` command
- [ ] `workflow dry-run` command
