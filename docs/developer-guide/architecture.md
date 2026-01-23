# Architecture Overview

This document describes fi-fhir's internal architecture for developers contributing to the project.

## System Architecture

```
┌────────────────────────────────────────────────────────────────────────────┐
│                              fi-fhir                                       │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌────────────┐ │
│  │   Parsers   │───>│  Semantic   │───>│  Workflow   │───>│  Actions   │ │
│  │  (internal/)│    │   Events    │    │   Engine    │    │ (internal/)│ │
│  └─────────────┘    │   (pkg/)    │    │ (internal/) │    └────────────┘ │
│        │            └─────────────┘    └─────────────┘          │        │
│        │                   │                  │                  │        │
│        v                   v                  v                  v        │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌────────────┐ │
│  │   Source    │    │    Event    │    │   GraphQL   │    │    FHIR    │ │
│  │  Profiles   │    │    Store    │    │     API     │    │   Mapper   │ │
│  │   (pkg/)    │    │   (pkg/)    │    │ (internal/) │    │   (pkg/)   │ │
│  └─────────────┘    └─────────────┘    └─────────────┘    └────────────┘ │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Parsers (internal/parser/)

Each format has a dedicated parser package:

```
internal/parser/
├── hl7v2/          # HL7 v2.x messages
│   ├── parser.go   # Main parser
│   ├── segments.go # Segment handlers
│   └── escape.go   # Escape sequence handling
├── csv/            # CSV/flatfiles
├── edi/            # EDI X12
├── cda/            # CDA/CCDA
└── fhir/           # FHIR resources
```

**Parser Contract:**

```go
type Parser interface {
    Parse(raw []byte, profile *profile.Profile) ([]events.Event, error)
}
```

### 2. Canonical Events (pkg/events/)

The heart of the system - format-agnostic event types:

```go
// Base metadata for all events
type EventMeta struct {
    ID              string        `json:"id"`
    Type            EventType     `json:"type"`
    Source          string        `json:"source"`
    Format          SourceFormat  `json:"format"`
    Timestamp       time.Time     `json:"timestamp"`
    SourceMessageID string        `json:"source_message_id,omitempty"`
    ParseWarnings   []ParseWarning `json:"parse_warnings,omitempty"`
    RawPayload      json.RawMessage `json:"raw_payload,omitempty"`
}

// Example event type
type PatientAdmitEvent struct {
    EventMeta
    Patient   Patient   `json:"patient"`
    Encounter Encounter `json:"encounter"`
}
```

### 3. Source Profiles (pkg/profile/)

Configuration-driven parsing behavior:

```go
type Profile struct {
    ID          string           `yaml:"id"`
    Name        string           `yaml:"name"`
    Version     string           `yaml:"version"`
    Encoding    EncodingConfig   `yaml:"encoding"`
    HL7v2       HL7v2Config      `yaml:"hl7v2,omitempty"`
    Identifiers IdentifierConfig `yaml:"identifiers,omitempty"`
    // ...
}

// Usage in parser
func (p *Parser) Parse(msg []byte, profile *Profile) ([]Event, error) {
    if profile.IsMissingSegmentTolerated("NK1") {
        // Don't fail on missing NK1
    }
}
```

### 4. Workflow Engine (internal/workflow/)

Event routing and transformation:

```
internal/workflow/
├── engine.go       # Main engine
├── types.go        # Workflow configuration types
├── filter.go       # Event filtering
├── transform.go    # Event transformation
├── cel.go          # CEL expression evaluation
├── actions/        # Action implementations
│   ├── fhir.go
│   ├── webhook.go
│   ├── database.go
│   └── queue.go
├── retry.go        # Retry with backoff
├── circuit_breaker.go
├── dlq.go          # Dead letter queue
└── metrics.go      # Observability
```

**Engine Contract:**

```go
type Engine struct {
    config  *WorkflowConfig
    actions map[string]Action
    metrics Metrics
    tracer  Tracer
}

func (e *Engine) Process(ctx context.Context, event Event) error {
    for _, route := range e.config.Routes {
        if route.Filter.Matches(event) {
            event = route.Transform.Apply(event)
            for _, action := range route.Actions {
                if err := action.Execute(ctx, event); err != nil {
                    e.handleError(ctx, event, err)
                }
            }
        }
    }
    return nil
}
```

### 5. FHIR Mapper (pkg/fhir/)

Transforms canonical events to FHIR R4 resources:

```go
type Mapper struct {
    targetVersion string
    validator     *Validator
}

func (m *Mapper) MapPatient(event *events.PatientEvent) (*fhir.Patient, error) {
    patient := &fhir.Patient{
        Meta: &fhir.Meta{
            Profile: []string{"http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"},
        },
        Identifier: mapIdentifiers(event.Patient.Identifiers),
        Name:       mapName(event.Patient.Name),
        // ...
    }

    if m.validator != nil {
        if err := m.validator.Validate(patient); err != nil {
            return nil, fmt.Errorf("validation failed: %w", err)
        }
    }

    return patient, nil
}
```

### 6. Event Store (pkg/eventsourcing/)

CQRS/Event Sourcing infrastructure:

```go
// Event store interface
type EventStore interface {
    Append(ctx context.Context, stream string, events []Event) error
    Read(ctx context.Context, stream string, from int64) ([]Event, error)
    Subscribe(ctx context.Context, stream string) (<-chan Event, error)
}

// Projections
type Projection interface {
    Name() string
    Handle(event Event) error
    State() interface{}
}

// Implementations
type MemoryStore struct { ... }     // For testing
type PostgresStore struct { ... }   // For production
```

### 7. GraphQL API (internal/api/graphql/)

Query and mutation interface:

```graphql
type Query {
    events(filter: EventFilter): [Event!]!
    projection(name: String!): Projection
}

type Mutation {
    triggerWorkflow(events: [EventInput!]!): WorkflowResult!
    createFHIRSubscription(input: SubscriptionInput!): Subscription!
}

type Subscription {
    eventStream(filter: EventFilter): Event!
}
```

## Data Flow

### 1. Parse Flow

```
Raw Message → Parser → Source Profile → Canonical Event → JSON Output
                          ↓
                   Profile Registry
                          ↓
                   Tolerance Rules
                   Identifier Config
                   Event Classification
```

### 2. Workflow Flow

```
Event → Filter Match? → Transform → Actions → Retry/DLQ
              ↓              ↓           ↓
         CEL Eval      Transforms    Circuit Breaker
         Type Match    set_field     Rate Limiter
         Source Match  redact        Metrics
                       map_terminology
```

### 3. FHIR Flow

```
Event → FHIR Mapper → Validation → Bundle → FHIR Server
              ↓            ↓           ↓
        US Core      JSON Schema   OAuth2 Auth
        Profile      Required      Token Cache
        Mapping      Elements      Retry
```

## Key Interfaces

### Parser Interface

```go
type Parser interface {
    // Parse raw message bytes into events
    Parse(raw []byte, profile *profile.Profile) ([]events.Event, error)

    // Validate a message without full parsing
    Validate(raw []byte, profile *profile.Profile) []ValidationError
}
```

### Action Interface

```go
type Action interface {
    // Type returns the action type name
    Type() string

    // Execute performs the action
    Execute(ctx context.Context, event events.Event) error

    // Validate checks action configuration
    Validate() error
}
```

### Event Interface

```go
type Event interface {
    // Meta returns event metadata
    Meta() EventMeta

    // Type returns the event type
    Type() EventType

    // JSON marshaling
    json.Marshaler
    json.Unmarshaler
}
```

## Concurrency Model

### Thread Safety

- Parsers are stateless and safe for concurrent use
- Profiles are immutable after loading
- Event store uses optimistic concurrency
- Workflow engine processes events independently

### Context Propagation

All operations accept `context.Context` for:
- Cancellation
- Timeouts
- Trace propagation

```go
func (e *Engine) Process(ctx context.Context, event Event) error {
    ctx, span := tracer.Start(ctx, "workflow.process")
    defer span.End()

    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Continue processing
    }
}
```

## Extension Points

### 1. Custom Parsers

Add new format support by implementing the Parser interface.

### 2. Custom Actions

Add new workflow actions by implementing the Action interface.

### 3. Custom Projections

Add new event projections by implementing the Projection interface.

### 4. Custom Validators

Add identifier validators to `pkg/validate/`.

## Configuration

### Layered Configuration

```
Built-in Defaults
      ↓
Config File (fi-fhir.yaml)
      ↓
Environment Variables (FI_FHIR_*)
      ↓
Command-line Flags
```

### Secret Providers

```go
type SecretProvider interface {
    Get(ctx context.Context, key string) (string, error)
}

// Implementations
type EnvSecretProvider struct { ... }
type FileSecretProvider struct { ... }
type VaultSecretProvider struct { ... }
type AWSSSMSecretProvider struct { ... }
type K8sSecretProvider struct { ... }
```

## Observability

### Metrics (Prometheus)

```go
var (
    eventsProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflow_events_processed_total",
        },
        []string{"type", "route", "status"},
    )

    actionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "workflow_action_duration_seconds",
        },
        []string{"type", "action"},
    )
)
```

### Tracing (OpenTelemetry)

```go
func (e *Engine) Process(ctx context.Context, event Event) error {
    ctx, span := tracer.Start(ctx, "workflow.process",
        trace.WithAttributes(
            attribute.String("event.type", string(event.Type())),
            attribute.String("event.id", event.Meta().ID),
        ),
    )
    defer span.End()
    // ...
}
```

### Logging (Structured)

```go
logger.Info("event processed",
    "event_id", event.Meta().ID,
    "event_type", event.Type(),
    "trace_id", span.SpanContext().TraceID(),
)
```

## See Also

- [Development Setup](development-setup.md) - Environment setup
- [Adding a Parser](adding-parser.md) - Parser development
- [Adding Actions](adding-actions.md) - Action development
- [AGENTS.md](../../AGENTS.md) - Comprehensive file reference
