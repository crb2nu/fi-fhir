# FHIR R4 Subscriptions

This document describes bidirectional FHIR integration via the R4 Subscription mechanism.

## Overview

FHIR R4 Subscriptions enable fi-fhir to **receive** events from FHIR servers, completing the bidirectional integration:

```
                    ┌─────────────────────────────────────────┐
                    │              fi-fhir                     │
                    │                                          │
 HL7v2/CSV/EDI ────▶│  ┌──────────┐    ┌─────────────────┐    │
                    │  │  Parser  │───▶│ Workflow Engine │    │
                    │  └──────────┘    └────────┬────────┘    │
                    │                           │              │
                    │  ┌──────────────┐         ▼              │
 FHIR Server ◀─────────│ FHIR Action  │◀── Actions ──────────▶│──▶ Webhook/DB/Queue
                    │  └──────────────┘                        │
                    │                                          │
                    │  ┌──────────────────┐                    │
 FHIR Server ──────────│ Subscription     │───▶ Events ───────▶│──▶ Workflow Engine
 (notifications)    │  │ Receiver         │                    │
                    │  └──────────────────┘                    │
                    └─────────────────────────────────────────┘
```

## FHIR R4 Subscription Model

### Subscription Resource

```json
{
  "resourceType": "Subscription",
  "status": "requested",
  "reason": "Monitor patient admissions",
  "criteria": "Encounter?status=in-progress&class=inpatient",
  "channel": {
    "type": "rest-hook",
    "endpoint": "https://fi-fhir.example.com/fhir/notify",
    "payload": "application/fhir+json",
    "header": [
      "Authorization: Bearer ${NOTIFY_TOKEN}"
    ]
  }
}
```

### Channel Types

| Type | Description | Use Case |
|------|-------------|----------|
| `rest-hook` | HTTP POST to endpoint | Most common, webhook-style |
| `websocket` | Persistent WebSocket connection | Real-time, low-latency |
| `email` | Email notification | Alerts, not for integration |
| `message` | FHIR messaging | Formal message exchange |

fi-fhir supports **rest-hook** (primary) and **websocket** (planned).

### Notification Payload

When a subscription triggers, the FHIR server POSTs a Bundle:

```json
{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "resource": {
        "resourceType": "Encounter",
        "id": "enc-123",
        "status": "in-progress",
        "class": { "code": "IMP" },
        "subject": { "reference": "Patient/pat-456" }
      },
      "request": {
        "method": "PUT",
        "url": "Encounter/enc-123"
      }
    }
  ]
}
```

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Subscription Manager                      │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Subscription│  │ Notification│  │ FHIR-to-Canonical   │  │
│  │ Client      │  │ Receiver    │  │ Mapper              │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                     │             │
│         ▼                ▼                     ▼             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                    Event Router                          ││
│  │  (routes canonical events to workflow engine)            ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 1. Subscription Client (`internal/fhir/subscription/client.go`)

Manages subscription lifecycle on FHIR servers:

```go
type SubscriptionClient struct {
    fhirEndpoint string
    httpClient   *http.Client
    tokenManager *oauth.TokenManager  // Reuse existing OAuth
}

// Create registers a new subscription
func (c *SubscriptionClient) Create(ctx context.Context, sub *Subscription) (*Subscription, error)

// List retrieves all subscriptions
func (c *SubscriptionClient) List(ctx context.Context) ([]*Subscription, error)

// Get retrieves a specific subscription
func (c *SubscriptionClient) Get(ctx context.Context, id string) (*Subscription, error)

// Delete removes a subscription
func (c *SubscriptionClient) Delete(ctx context.Context, id string) error

// UpdateStatus changes subscription status (off, requested, active, error)
func (c *SubscriptionClient) UpdateStatus(ctx context.Context, id, status string) error
```

### 2. Notification Receiver (`internal/fhir/subscription/receiver.go`)

HTTP server that receives webhook notifications:

```go
type NotificationReceiver struct {
    subscriptions map[string]*SubscriptionConfig
    mapper        *FHIRMapper
    router        EventRouter
    metrics       Metrics
    tracer        Tracer
}

// ServeHTTP handles incoming notifications
func (r *NotificationReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request)

// RegisterSubscription adds a subscription to handle
func (r *NotificationReceiver) RegisterSubscription(id string, config *SubscriptionConfig)

// UnregisterSubscription removes a subscription
func (r *NotificationReceiver) UnregisterSubscription(id string)
```

### 3. FHIR-to-Canonical Mapper (`internal/fhir/subscription/mapper.go`)

Converts FHIR resources to canonical events:

```go
type FHIRMapper struct {
    resourceMappers map[string]ResourceMapper
}

// MapBundle converts a notification Bundle to canonical events
func (m *FHIRMapper) MapBundle(bundle *fhir.Bundle) ([]interface{}, error)

// ResourceMapper interface for resource-specific mapping
type ResourceMapper interface {
    Map(resource map[string]interface{}, action string) (interface{}, error)
}

// Built-in mappers:
// - PatientMapper: Patient -> patient_created, patient_updated, patient_deleted
// - EncounterMapper: Encounter -> patient_admit, patient_discharge, patient_transfer
// - ObservationMapper: Observation -> lab_result, vital_sign
// - AppointmentMapper: Appointment -> appointment_booked, appointment_cancelled
```

### 4. Event Router (`internal/fhir/subscription/router.go`)

Routes canonical events to the workflow engine:

```go
type EventRouter interface {
    Route(ctx context.Context, event interface{}) error
}

// WorkflowRouter routes events through workflow engine
type WorkflowRouter struct {
    engine *workflow.Engine
}

func (r *WorkflowRouter) Route(ctx context.Context, event interface{}) error {
    return r.engine.ProcessWithContext(ctx, event)
}
```

## Configuration

### Subscription Definition (YAML)

```yaml
# subscriptions.yaml
subscriptions:
  - name: patient_changes
    description: Monitor patient demographics changes
    server: https://fhir.hospital.org/r4
    auth:
      type: oauth2
      token_url: https://auth.hospital.org/token
      client_id: ${FHIR_CLIENT_ID}
      client_secret: ${FHIR_CLIENT_SECRET}
    criteria: Patient?_lastUpdated=gt${LAST_SYNC}
    channel:
      endpoint: https://fi-fhir.example.com/fhir/notify/patient_changes
      payload: application/fhir+json
    event_mapping:
      create: patient_created
      update: patient_updated
      delete: patient_deleted

  - name: inpatient_encounters
    description: Monitor inpatient admissions and discharges
    server: https://fhir.hospital.org/r4
    auth:
      type: bearer
      token: ${FHIR_TOKEN}
    criteria: Encounter?class=IMP&status=in-progress,finished
    channel:
      endpoint: https://fi-fhir.example.com/fhir/notify/encounters
      payload: application/fhir+json
    event_mapping:
      # Map based on resource state
      rules:
        - condition: resource.status == "in-progress"
          event_type: patient_admit
        - condition: resource.status == "finished"
          event_type: patient_discharge

  - name: critical_labs
    description: Monitor critical lab results
    server: https://fhir.hospital.org/r4
    criteria: Observation?category=laboratory&interpretation=critical
    channel:
      endpoint: https://fi-fhir.example.com/fhir/notify/labs
    event_mapping:
      create: lab_result
      update: lab_result
```

### Application Configuration

```yaml
# config.yaml
subscription_receiver:
  enabled: true
  host: "0.0.0.0"
  port: 8081
  path_prefix: /fhir/notify

  # Security
  verify_signature: true
  allowed_sources:
    - https://fhir.hospital.org
    - https://fhir.clinic.org

  # TLS (required for production)
  tls:
    enabled: true
    cert_file: /etc/fi-fhir/tls/cert.pem
    key_file: /etc/fi-fhir/tls/key.pem

  # Processing
  max_bundle_size: 100
  timeout: 30s

  # Retry failed routing
  retry:
    enabled: true
    max_attempts: 3
    initial_delay: 1s
```

## Event Mapping

### FHIR Resource to Canonical Event

| FHIR Resource | Action | Canonical Event |
|--------------|--------|-----------------|
| Patient | create | `patient_created` |
| Patient | update | `patient_updated` |
| Patient | delete | `patient_deleted` |
| Encounter (status=in-progress, class=IMP) | create/update | `patient_admit` |
| Encounter (status=finished) | update | `patient_discharge` |
| Encounter (location changed) | update | `patient_transfer` |
| Observation (category=laboratory) | create/update | `lab_result` |
| Observation (category=vital-signs) | create/update | `vital_sign` |
| Appointment (status=booked) | create/update | `appointment_booked` |
| Appointment (status=cancelled) | update | `appointment_cancelled` |
| DiagnosticReport | create/update | `diagnostic_report` |

### Custom Mapping Rules

For complex mapping logic, CEL expressions can determine event type:

```yaml
event_mapping:
  rules:
    - condition: >
        resource.resourceType == "Encounter" &&
        resource.status == "in-progress" &&
        resource.class.code == "IMP"
      event_type: patient_admit

    - condition: >
        resource.resourceType == "Observation" &&
        resource.interpretation.exists(i, i.coding.exists(c, c.code in ["H", "HH", "L", "LL"]))
      event_type: abnormal_lab_result
```

## CLI Commands

### Subscription Management

```bash
# List configured subscriptions
fi-fhir subscription list

# Show subscription status
fi-fhir subscription status patient_changes

# Create/register subscription on FHIR server
fi-fhir subscription create --config subscriptions.yaml --name patient_changes

# Delete subscription from FHIR server
fi-fhir subscription delete --name patient_changes

# Pause subscription (set status to off)
fi-fhir subscription pause --name patient_changes

# Resume subscription (set status to requested)
fi-fhir subscription resume --name patient_changes

# Test subscription endpoint (simulate notification)
fi-fhir subscription test --name patient_changes --resource testdata/patient.json
```

### Receiver Management

```bash
# Start subscription receiver
fi-fhir subscription serve --config config.yaml

# Validate subscription configuration
fi-fhir subscription validate subscriptions.yaml
```

## Workflow Integration

Canonical events from subscriptions are routed through the same workflow engine:

```yaml
# workflow.yaml
workflow:
  name: fhir_subscription_processing
  version: "1.0"

  routes:
    # Handle patient updates from FHIR subscription
    - name: patient_updates_to_ehr
      filter:
        event_type: patient_updated
        source: fhir_subscription  # Identifies events from subscriptions
      actions:
        - type: webhook
          url: ${EHR_WEBHOOK_URL}
          method: POST
        - type: database
          connection: ${DATABASE_URL}
          table: patient_sync_log
          operation: insert
          mapping_patient_id: patient.id
          mapping_fhir_id: meta.source_message_id
          mapping_timestamp: timestamp

    # Handle critical labs from FHIR subscription
    - name: critical_lab_alerts
      filter:
        event_type: lab_result
        condition: event.result.interpretation in ["critical", "HH", "LL"]
      actions:
        - type: webhook
          url: ${ALERT_WEBHOOK_URL}
        - type: log
          level: warn
          message: "Critical lab from FHIR: {{.Test.Code}} = {{.Result.Value}}"
```

## Security Considerations

### 1. TLS Required

All subscription endpoints MUST use HTTPS in production:

```yaml
subscription_receiver:
  tls:
    enabled: true
    cert_file: /etc/fi-fhir/tls/cert.pem
    key_file: /etc/fi-fhir/tls/key.pem
```

### 2. Source Validation

Verify notifications come from expected FHIR servers:

```yaml
allowed_sources:
  - https://fhir.hospital.org
  - https://fhir.clinic.org
```

### 3. Signature Verification

Some FHIR servers sign notifications. Verify when available:

```yaml
verify_signature: true
signature_secret: ${SUBSCRIPTION_SECRET}
```

### 4. Authentication Token in Channel

Subscriptions can include auth headers for callbacks:

```json
{
  "channel": {
    "header": [
      "Authorization: Bearer ${NOTIFY_TOKEN}"
    ]
  }
}
```

### 5. Rate Limiting

Protect against notification floods:

```yaml
subscription_receiver:
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst: 200
```

## Observability

### Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `fhir_notifications_received_total` | Counter | subscription, resource_type | Notifications received |
| `fhir_notifications_processed_total` | Counter | subscription, success | Successfully processed |
| `fhir_notifications_duration_seconds` | Histogram | subscription | Processing duration |
| `fhir_subscriptions_active` | Gauge | server | Active subscriptions per server |
| `fhir_subscription_errors_total` | Counter | subscription, error_type | Processing errors |

### Tracing

Notifications create spans that link to workflow processing:

```
fhir.notification.receive (root span)
├── fhir.notification.validate
├── fhir.bundle.parse
├── fhir.resource.map (one per resource)
└── workflow.process (linked to workflow trace)
```

### Logging

```json
{
  "level": "info",
  "msg": "notification received",
  "subscription": "patient_changes",
  "bundle_size": 3,
  "trace_id": "abc123"
}
```

## Implementation Plan

### Phase 1: Core Infrastructure ✅
- [x] Subscription client for CRUD operations - see `internal/fhir/subscription/client.go`
- [x] Notification receiver HTTP server - see `internal/fhir/subscription/receiver.go`
- [x] Basic FHIR-to-canonical mappers (Patient, Encounter, Observation, Appointment) - see `mapper.go`

### Phase 2: Event Routing ✅
- [x] Integration with workflow engine - see `internal/fhir/subscription/router.go`
- [x] Source identification (`source: fhir_subscription`) - see `mapper.go`
- [x] Subscription configuration YAML - see `internal/fhir/subscription/config.go`
- [x] OAuth2 support for outgoing requests - see `router.go` (OAuth2Auth provider)

### Phase 3: CLI & Management ✅
- [x] `subscription` CLI commands - see `cmd/fi-fhir/` (list, status, create, delete, pause, resume, serve, validate, test)
- [x] Status monitoring and health checks - see `receiver.go`
- [x] Validation tooling - see `validate` command

### Phase 4: Advanced Features ⚠️
- [ ] WebSocket channel support (planned) — #3
- [x] Custom CEL-based event mapping - see `mapper.go` (uses workflow.CELEvaluator)
- [ ] Subscription backfill (initial sync) — #3

## Example: Full Integration

```bash
# 1. Configure subscription
cat > subscriptions.yaml << 'EOF'
subscriptions:
  - name: all_patients
    server: https://fhir.hospital.org/r4
    auth:
      type: oauth2
      token_url: https://auth.hospital.org/token
      client_id: ${FHIR_CLIENT_ID}
      client_secret: ${FHIR_CLIENT_SECRET}
    criteria: Patient
    channel:
      endpoint: https://fi-fhir.example.com/fhir/notify/patients
EOF

# 2. Configure workflow to handle events
cat > workflow.yaml << 'EOF'
workflow:
  name: fhir_sync
  routes:
    - name: sync_patients
      filter:
        event_type: [patient_created, patient_updated]
      actions:
        - type: database
          connection: ${DATABASE_URL}
          table: patients
          operation: upsert
          conflict_on: fhir_id
          mapping_fhir_id: meta.source_message_id
          mapping_mrn: patient.mrn
          mapping_name: patient.name.full
EOF

# 3. Register subscription on FHIR server
fi-fhir subscription create --config subscriptions.yaml --name all_patients

# 4. Start receiver with workflow
fi-fhir subscription serve \
  --subscriptions subscriptions.yaml \
  --workflow workflow.yaml

# 5. Monitor
curl http://localhost:9090/metrics | grep fhir_notifications
```

## See Also

- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Workflow engine that processes subscription events
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - US Core profiles for FHIR resources
- [PRODUCTION-HARDENING.md](../operations/PRODUCTION-HARDENING.md) - Security for production deployments
