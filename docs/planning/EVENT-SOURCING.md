# Event Sourcing / CQRS Patterns

Design document for implementing event sourcing and CQRS (Command Query Responsibility Segregation) patterns in fi-fhir.

## Overview

Event sourcing stores the full history of changes as a sequence of events rather than just current state. Combined with CQRS, this enables:
- **Complete audit trail** - Every change is recorded (critical for healthcare compliance)
- **Temporal queries** - Query state at any point in time ("What was the patient's diagnosis list on 2024-01-15?")
- **Event replay** - Rebuild projections from events, fix bugs in projection logic
- **Decoupled read models** - Multiple optimized views from the same event stream

## Architecture

```
                    Commands                      Queries
                        │                            │
                        ▼                            ▼
              ┌─────────────────┐          ┌─────────────────┐
              │  Command Handler │          │   Query Handler  │
              └────────┬────────┘          └────────┬────────┘
                       │                            │
                       ▼                            │
              ┌─────────────────┐                   │
              │   Event Store   │                   │
              │  (append-only)  │                   │
              └────────┬────────┘                   │
                       │                            │
                       ▼                            │
              ┌─────────────────┐          ┌───────▼─────────┐
              │   Projector     │─────────▶│    Read Model   │
              │  (event handler)│          │   (optimized)   │
              └─────────────────┘          └─────────────────┘
```

## Event Store Interface

```go
// EventStore is the core event sourcing interface for append-only event storage.
type EventStore interface {
    // Append adds events to a stream. Returns the new stream version.
    // Uses optimistic concurrency: fails if expectedVersion doesn't match.
    Append(ctx context.Context, streamID string, expectedVersion int64, events []StoredEvent) (int64, error)

    // ReadStream reads events from a stream starting at fromVersion.
    ReadStream(ctx context.Context, streamID string, fromVersion int64, maxCount int) ([]StoredEvent, error)

    // ReadAll reads events across all streams for projections.
    ReadAll(ctx context.Context, fromPosition int64, maxCount int) ([]StoredEvent, error)

    // Subscribe returns a channel of new events for real-time projections.
    Subscribe(ctx context.Context, fromPosition int64) (<-chan StoredEvent, error)

    // GetStreamVersion returns the current version of a stream (-1 if not exists).
    GetStreamVersion(ctx context.Context, streamID string) (int64, error)
}

// StoredEvent represents an event as stored in the event store.
type StoredEvent struct {
    // Position is the global ordering position across all streams
    Position int64

    // StreamID identifies the aggregate/entity this event belongs to
    StreamID string

    // StreamVersion is the version within the stream (starts at 0)
    StreamVersion int64

    // EventType is the event type (e.g., "patient_admit", "lab_result")
    EventType string

    // Data is the serialized event payload
    Data []byte

    // Metadata contains correlation IDs, user info, etc.
    Metadata map[string]string

    // Timestamp when the event was stored
    Timestamp time.Time
}
```

## Stream Naming Conventions

Healthcare events naturally map to streams:

| Stream Pattern | Description | Example |
|---------------|-------------|---------|
| `patient:{mrn}` | Patient-centric events | `patient:MRN001` |
| `encounter:{id}` | Encounter-specific events | `encounter:ENC-2024-001` |
| `claim:{id}` | Claim lifecycle events | `claim:CLM-837-001` |
| `source:{id}` | Events from a specific source | `source:epic_adt` |

## Projection Framework

```go
// Projection builds a read model from events.
type Projection interface {
    // Name returns the projection name (used for checkpointing).
    Name() string

    // Handle processes an event and updates the read model.
    Handle(ctx context.Context, event StoredEvent) error

    // GetCheckpoint returns the last processed position.
    GetCheckpoint(ctx context.Context) (int64, error)

    // SetCheckpoint saves the last processed position.
    SetCheckpoint(ctx context.Context, position int64) error
}

// ProjectionRunner manages projection lifecycle.
type ProjectionRunner struct {
    store       EventStore
    projections []Projection
    batchSize   int
    pollInterval time.Duration
}

// Run starts the projection runner (blocking).
func (r *ProjectionRunner) Run(ctx context.Context) error
```

## Example Projections

### 1. Patient Timeline Projection

Builds a chronological view of all events for a patient:

```go
type PatientTimeline struct {
    MRN    string
    Events []TimelineEvent
}

type TimelineEvent struct {
    Timestamp   time.Time
    EventType   string
    Summary     string
    SourceEvent StoredEvent
}

type PatientTimelineProjection struct {
    db *sql.DB
}

func (p *PatientTimelineProjection) Handle(ctx context.Context, event StoredEvent) error {
    // Extract patient MRN from event
    mrn := extractMRN(event)
    if mrn == "" {
        return nil // Skip non-patient events
    }

    // Build timeline entry
    summary := buildSummary(event)

    // Upsert into read model
    _, err := p.db.ExecContext(ctx, `
        INSERT INTO patient_timelines (mrn, position, timestamp, event_type, summary, event_data)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (mrn, position) DO NOTHING
    `, mrn, event.Position, event.Timestamp, event.EventType, summary, event.Data)

    return err
}
```

### 2. Event Statistics Projection

Aggregates event counts by type, source, and time window:

```go
type EventStatisticsProjection struct {
    counts map[string]map[string]int64 // [eventType][source] -> count
    mu     sync.RWMutex
}

func (p *EventStatisticsProjection) Handle(ctx context.Context, event StoredEvent) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    source := event.Metadata["source"]
    if p.counts[event.EventType] == nil {
        p.counts[event.EventType] = make(map[string]int64)
    }
    p.counts[event.EventType][source]++

    return nil
}

func (p *EventStatisticsProjection) GetStats() map[string]map[string]int64 {
    p.mu.RLock()
    defer p.mu.RUnlock()

    // Return deep copy
    result := make(map[string]map[string]int64)
    for eventType, sources := range p.counts {
        result[eventType] = make(map[string]int64)
        for source, count := range sources {
            result[eventType][source] = count
        }
    }
    return result
}
```

### 3. Active Encounters Projection

Maintains a list of currently active encounters:

```go
type ActiveEncountersProjection struct {
    encounters map[string]*ActiveEncounter // encounterID -> encounter
    mu         sync.RWMutex
}

type ActiveEncounter struct {
    ID        string
    PatientMRN string
    Class     string
    Location  string
    AdmitTime time.Time
    Provider  string
}

func (p *ActiveEncountersProjection) Handle(ctx context.Context, event StoredEvent) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    switch event.EventType {
    case "patient_admit":
        enc := parseAdmitEvent(event)
        p.encounters[enc.ID] = enc

    case "patient_discharge":
        encID := parseDischargeEvent(event)
        delete(p.encounters, encID)

    case "patient_transfer":
        encID, newLocation := parseTransferEvent(event)
        if enc, ok := p.encounters[encID]; ok {
            enc.Location = newLocation
        }
    }

    return nil
}
```

## Snapshot Support

For projections with large state, snapshots avoid replaying all events:

```go
// SnapshotStore manages projection snapshots.
type SnapshotStore interface {
    // Save stores a snapshot of projection state.
    Save(ctx context.Context, projectionName string, position int64, state []byte) error

    // Load retrieves the latest snapshot.
    Load(ctx context.Context, projectionName string) (position int64, state []byte, err error)
}

// SnapshotProjection extends Projection with snapshot support.
type SnapshotProjection interface {
    Projection

    // Serialize returns the current state as bytes.
    Serialize() ([]byte, error)

    // Deserialize restores state from bytes.
    Deserialize(data []byte) error

    // ShouldSnapshot returns true if a snapshot should be taken.
    ShouldSnapshot(eventsProcessed int64) bool
}
```

## PostgreSQL Event Store Implementation

```sql
-- Event store schema
CREATE TABLE events (
    position BIGSERIAL PRIMARY KEY,
    stream_id TEXT NOT NULL,
    stream_version BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    data JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (stream_id, stream_version)
);

CREATE INDEX idx_events_stream ON events (stream_id, stream_version);
CREATE INDEX idx_events_type ON events (event_type);
CREATE INDEX idx_events_timestamp ON events (timestamp);

-- Projection checkpoints
CREATE TABLE projection_checkpoints (
    projection_name TEXT PRIMARY KEY,
    position BIGINT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Snapshots
CREATE TABLE projection_snapshots (
    projection_name TEXT PRIMARY KEY,
    position BIGINT NOT NULL,
    state BYTEA NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Integration with Existing fi-fhir

### Workflow Integration

Events from the workflow engine can be stored in the event store:

```yaml
# workflow.yaml
routes:
  - name: store_all_events
    filter:
      types: ["*"]
    actions:
      - type: event_store
        config:
          stream_template: "patient:{{.Patient.MRN}}"
```

### GraphQL Integration

The GraphQL API can query projections:

```graphql
type Query {
    # Existing queries...

    # Event sourcing queries
    patientTimeline(mrn: String!, from: DateTime, to: DateTime): [TimelineEvent!]!
    eventStatistics(eventTypes: [EventType!], sources: [String!]): EventStats!
    activeEncounters(location: String): [ActiveEncounter!]!

    # Temporal queries
    patientStateAt(mrn: String!, timestamp: DateTime!): PatientState
}

type TimelineEvent {
    timestamp: DateTime!
    eventType: EventType!
    summary: String!
    rawEvent: Event
}

type EventStats {
    byType: [TypeCount!]!
    bySource: [SourceCount!]!
    total: Int!
}
```

## CLI Commands

```bash
# Event store management
fi-fhir eventstore init          # Initialize database schema
fi-fhir eventstore stats         # Show event store statistics
fi-fhir eventstore streams       # List streams
fi-fhir eventstore read <stream> # Read events from a stream

# Projection management
fi-fhir projection list          # List registered projections
fi-fhir projection status        # Show projection positions
fi-fhir projection rebuild <name> # Rebuild projection from scratch
fi-fhir projection run           # Run projection runner
```

## Implementation Plan

### Phase 1: Core Event Store ✅
- [x] `EventStore` interface in `pkg/eventsourcing/store.go`
- [x] `StoredEvent` type with serialization
- [x] In-memory implementation for testing (`pkg/eventsourcing/memory_store.go`)
- [x] PostgreSQL implementation (`pkg/eventsourcing/postgres_store.go`)

### Phase 2: Projection Framework ✅
- [x] `Projection` interface (`pkg/eventsourcing/projection.go`)
- [x] `ProjectionRunner` with checkpointing
- [x] Snapshot support (`pkg/eventsourcing/snapshot.go`)
- [x] Concurrent projection execution

### Phase 3: Built-in Projections ✅
- [x] Patient timeline projection (`pkg/eventsourcing/projections/patient_timeline.go`)
- [x] Event statistics projection (`pkg/eventsourcing/projections/event_statistics.go`)
- [x] Active encounters projection (`pkg/eventsourcing/projections/active_encounters.go`)

### Phase 4: Integration ✅
- [x] Workflow action for event store (`internal/workflow/event_store.go`)
- [x] GraphQL queries for projections (`internal/api/graphql/schema.graphql`)
- [x] CLI commands (`cmd/fi-fhir/eventstore.go`)

## Healthcare Compliance Considerations

- **Immutability**: Events are never modified or deleted (append-only)
- **Audit Trail**: Complete history of all data changes
- **Temporal Queries**: Query state at any point in time for audits
- **Data Retention**: Configurable retention policies per stream type
- **Access Logging**: All reads can be logged for HIPAA compliance

## See Also

- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Workflow routing integration
- [GRAPHQL-API.md](GRAPHQL-API.md) - GraphQL query integration
- [FHIR-SUBSCRIPTIONS.md](FHIR-SUBSCRIPTIONS.md) - Subscription-based event ingestion

## References

- [Event Sourcing (Martin Fowler)](https://martinfowler.com/eaaDev/EventSourcing.html)
- [CQRS (Martin Fowler)](https://martinfowler.com/bliki/CQRS.html)
- [EventStoreDB Documentation](https://www.eventstore.com/docs)
- [Marten Event Store (.NET)](https://martendb.io/events/)
