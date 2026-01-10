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
| `event_store` | ✅ Implemented | Append to event sourcing store |

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

**OAuth2 Token Refresh:**
- Tokens are cached and reused until 60 seconds before expiry
- If a 401 Unauthorized is received, the cached token is invalidated and a fresh token is fetched
- Token fetch requests use retry with exponential backoff (3 attempts)
- Supports proactive refresh (fetches new token before expiry)

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

#### Event Store Action
Append to event sourcing store:

```yaml
- type: event_store
  connection: ${DATABASE_URL}          # PostgreSQL connection string
  table: events                        # Events table name (default: events)
  stream: patient:{{.patient.mrn}}     # Stream ID template (required)
  event_type: {{.type}}                # Event type override (uses event.type by default)
  metadata_source: adt_interface       # Additional metadata (metadata_<key>)
  metadata_environment: production
```

**Config Options:**
| Option | Required | Description |
|--------|----------|-------------|
| `connection` | Yes | PostgreSQL DSN (also accepts `dsn` or `db`) |
| `table` | No | Events table name (default: `events`) |
| `stream` | Yes | Stream ID template (e.g., `patient:{{.patient.mrn}}`) |
| `event_type` | No | Event type override (default: uses event's type field) |
| `metadata_<key>` | No | Additional metadata to store with event |

**Key Features:**
- Append-only storage with automatic versioning
- Optimistic concurrency (uses `VersionAny` for conflict-free appends)
- Full event JSON payload preserved
- Template support for stream IDs and event types
- Metadata enrichment from config and event

**Use Cases:**
- Capture all events for audit trail
- Build CQRS read models via projections
- Enable event replay for debugging/recovery
- Feed downstream analytics pipelines

**Implementation:** `internal/workflow/event_store.go`

### Retry Configuration

All HTTP-based actions (webhook, fhir) support retry with exponential backoff. Retry is enabled by default with sensible defaults:

```yaml
- type: webhook
  url: https://api.example.com/events
  # Retry configuration (all optional)
  retry_max: 3                    # Max retry attempts (0 to disable)
  retry_delay: 1s                 # Initial delay before first retry
  retry_max_delay: 30s            # Maximum delay cap
  retry_multiplier: 2.0           # Exponential backoff multiplier
  retry_jitter: 0.1               # Jitter factor (0.0-1.0) to prevent thundering herd
  retry_on_status: "429,500,502,503,504"  # Status codes that trigger retry
```

**Retry Config Options:**
| Option | Default | Description |
|--------|---------|-------------|
| `retry_max` | 3 | Maximum retry attempts (0 disables retries) |
| `retry_delay` | 1s | Initial delay before first retry |
| `retry_max_delay` | 30s | Maximum delay cap for exponential backoff |
| `retry_multiplier` | 2.0 | Multiplier for exponential backoff |
| `retry_jitter` | 0.1 | Jitter factor (0.0-1.0) for delay randomization |
| `retry_on_status` | 429,500,502,503,504 | Comma-separated HTTP status codes to retry |
| `retry` | true | Set to "false" to disable retries entirely |

**Behavior:**
- Network errors (connection refused, timeout) are always retried
- HTTP status codes in `retry_on_status` trigger retry
- Non-retryable status codes (400, 401, 404, etc.) fail immediately
- Delay grows exponentially: `initial * multiplier^(attempt-1)`
- Jitter randomizes delay: `delay * (1 ± jitter)` to prevent synchronized retries
- Context cancellation stops retry loop immediately

**Example with aggressive retries:**
```yaml
- type: fhir
  endpoint: https://fhir.hospital.org/r4
  token: ${FHIR_TOKEN}
  retry_max: 5
  retry_delay: 500ms
  retry_max_delay: 10s
  retry_multiplier: 1.5
  retry_on_status: "408,429,500,502,503,504"  # Include 408 Request Timeout
```

**Example with retries disabled:**
```yaml
- type: webhook
  url: https://fast-api.example.com/events
  retry_max: 0  # or retry: "false"
```

**Implementation:** `internal/workflow/retry.go`

### Circuit Breaker

HTTP-based actions (webhook, fhir) support the circuit breaker pattern to prevent cascading failures when external services are unavailable. The circuit breaker is disabled by default:

```yaml
- type: fhir
  endpoint: https://fhir.hospital.org/r4
  token: ${FHIR_TOKEN}
  # Circuit breaker configuration (all optional)
  circuit_breaker: "true"           # Enable circuit breaker (default: disabled)
  circuit_failure_threshold: 5      # Failures before opening (default: 5)
  circuit_success_threshold: 2      # Successes in half-open before closing (default: 2)
  circuit_timeout: 30s              # Time in open state before half-open (default: 30s)
```

**Circuit Breaker Config Options:**
| Option | Default | Description |
|--------|---------|-------------|
| `circuit_breaker` | disabled | Set to "true" to enable circuit breaker |
| `circuit_failure_threshold` | 5 | Consecutive failures before circuit opens |
| `circuit_success_threshold` | 2 | Consecutive successes in half-open before closing |
| `circuit_timeout` | 30s | Time circuit stays open before transitioning to half-open |

**State Machine:**
```
       failure >= threshold
           ┌─────┐
           │     ▼
┌──────────┴───┐   timeout   ┌────────────┐   success >= threshold
│   CLOSED     │────────────▶│  HALF-OPEN │───────────────────────┐
│ (normal)     │◀────────────│  (probing) │                       │
└──────────────┘   failure   └────────────┘                       │
       ▲                                                          │
       └──────────────────────────────────────────────────────────┘
```

**Behavior:**
- **Closed (normal)**: Requests pass through. Failures increment counter; successes reset it
- **Open (fast-fail)**: Requests immediately fail with `circuit breaker is open` error
- **Half-Open (probing)**: Limited requests allowed to test if service recovered

**Key Features:**
- Per-endpoint circuit breakers via global registry
- HTTP status codes 500, 502, 503, 504 count as failures
- Network errors (connection refused, timeout) count as failures
- Circuit breaker wraps retry logic (outermost layer) to prevent retry storms
- Thread-safe with concurrent access support

**Example with aggressive circuit breaker:**
```yaml
- type: webhook
  url: https://fragile-api.example.com/events
  circuit_breaker: "true"
  circuit_failure_threshold: 3      # Open after just 3 failures
  circuit_success_threshold: 1      # Close after 1 success
  circuit_timeout: 10s              # Try again after 10s
  retry_max: 2                      # Fewer retries to fail faster
```

**Example monitoring circuit state:**
The circuit breaker tracks statistics including total failures, successes, and rejected requests. These can be accessed programmatically via `GetCircuitBreaker(endpoint).Stats()`.

**Implementation:** `internal/workflow/circuit_breaker.go`

### Rate Limiting

HTTP-based actions (webhook, fhir) support rate limiting to prevent overwhelming external services during high-volume event streams. Rate limiting is disabled by default:

```yaml
- type: webhook
  url: https://api.example.com/events
  # Rate limiting configuration (all optional)
  rate_limit: "true"             # Enable rate limiting (default: disabled)
  rate_limit_rate: 10            # Requests per second (default: 10)
  rate_limit_burst: 20           # Maximum burst size (default: 20)
  rate_limit_wait: "true"        # Wait when limited vs fail fast (default: true)
```

**Rate Limit Config Options:**
| Option | Default | Description |
|--------|---------|-------------|
| `rate_limit` | disabled | Set to "true" to enable rate limiting |
| `rate_limit_rate` | 10 | Requests per second |
| `rate_limit_burst` | 20 | Maximum burst capacity |
| `rate_limit_wait` | true | "true" to wait for token, "false" to fail immediately |

**Token Bucket Algorithm:**
Rate limiting uses the token bucket algorithm:
- Tokens are added at `rate_limit_rate` tokens per second
- Bucket can hold up to `rate_limit_burst` tokens (also initial capacity)
- Each request consumes one token
- If no tokens available: wait for refill (`rate_limit_wait: true`) or fail with `ErrRateLimited`

**Behavior:**
- Per-endpoint limiters: Each unique URL/endpoint gets its own rate limiter
- Limiters are reused across requests to the same endpoint
- Rate limiting is applied before circuit breaker and retry logic
- With `rate_limit_wait: true`, requests queue up and wait their turn
- With `rate_limit_wait: false`, requests fail immediately when rate limited

**Example with conservative rate limiting:**
```yaml
- type: fhir
  endpoint: https://fhir.external-payer.com/r4
  token: ${PAYER_TOKEN}
  rate_limit: "true"
  rate_limit_rate: 5              # Only 5 requests per second
  rate_limit_burst: 10            # Allow small bursts
  rate_limit_wait: "true"         # Wait rather than fail
```

**Example with fail-fast rate limiting:**
```yaml
- type: webhook
  url: https://rate-limited-api.example.com/events
  rate_limit: "true"
  rate_limit_rate: 100
  rate_limit_burst: 50
  rate_limit_wait: "false"        # Return error immediately if rate limited
```

**Programmatic Access:**
```go
// Get rate limiter for an endpoint
limiter := workflow.GetRateLimiter("https://api.example.com")

// Check current token count
tokens := limiter.Tokens()

// Wait for a token (with context for cancellation)
err := limiter.Wait(ctx)
```

**Implementation:** `internal/workflow/ratelimit.go`

### Dead Letter Queue

Failed events can be stored in a dead letter queue (DLQ) for later inspection and reprocessing. The DLQ captures failed events with metadata about why they failed.

**Setup:**
```go
// Create workflow engine
engine, _ := workflow.NewEngine(workflowConfig)

// Configure DLQ with in-memory storage (for development/testing)
dlq := workflow.NewMemoryDLQ()
engine.SetDLQ(dlq)

// Or use file-based storage (for persistence)
dlq, _ := workflow.NewFileDLQ("/var/lib/fi-fhir/dlq")
engine.SetDLQ(dlq)

// Optional: Configure with callback for alerting
config := workflow.DLQConfig{
    MaxAttempts: 5,  // Stop retrying after 5 attempts
    OnDeadLetter: func(event *workflow.FailedEvent) {
        log.Printf("Event %s failed: %s", event.ID, event.Error)
        // Send alert to monitoring system
    },
}
engine.SetDLQ(dlq, config)
```

**Failed Event Structure:**
```go
type FailedEvent struct {
    ID           string                 // Unique identifier
    Event        interface{}            // Original event data
    RouteName    string                 // Route that failed
    ActionType   string                 // Action type (fhir, webhook, etc.)
    Error        string                 // Error message
    ErrorType    string                 // Classified error type
    Attempts     int                    // Number of attempts
    FirstFailure time.Time              // When first failed
    LastFailure  time.Time              // Most recent failure
    Metadata     map[string]string      // Additional context
}
```

**Error Types:**
| ErrorType | Description |
|-----------|-------------|
| `circuit_open` | Circuit breaker is open |
| `timeout` | Request timed out |
| `connection_error` | Network connection failed |
| `auth_error` | Authentication/authorization failed |
| `server_error` | HTTP 5xx response |
| `client_error` | HTTP 4xx response |
| `unknown` | Unclassified error |

**Reprocessing:**
```go
// Reprocess all events in DLQ (up to limit)
result := engine.ReprocessDLQ(100)
fmt.Printf("Processed: %d, Failed: %d, Skipped: %d\n",
    result.Processed, result.Failed, result.Skipped)

// Reprocess a specific event
result, err := engine.ReprocessDLQEvent("event-id-123")
if err != nil {
    log.Printf("Reprocess failed: %v", err)
}

// List failed events for inspection
events, _ := dlq.List(50)
for _, e := range events {
    fmt.Printf("Event %s failed %d times: %s\n", e.ID, e.Attempts, e.Error)
}
```

**DLQ Backends:**
| Backend | Use Case | Persistence |
|---------|----------|-------------|
| `MemoryDLQ` | Development, testing | None (lost on restart) |
| `FileDLQ` | Simple persistence | JSON files in directory |
| Custom | Production | Implement `DeadLetterQueue` interface |

**Custom DLQ Implementation:**
```go
type DeadLetterQueue interface {
    Push(event *FailedEvent) error
    Pop() (*FailedEvent, error)
    Peek() (*FailedEvent, error)
    List(limit int) ([]*FailedEvent, error)
    Get(id string) (*FailedEvent, error)
    Remove(id string) error
    Len() int
    Clear() error
}
```

**Implementation:** `internal/workflow/dlq.go`, `internal/workflow/engine.go`

### Metrics / Observability

The workflow engine provides pluggable metrics collection for monitoring and observability. Metrics are disabled by default (no-op) and can be enabled by setting a metrics backend.

**Setup:**
```go
// Create workflow engine
engine, _ := workflow.NewEngine(workflowConfig)

// Use in-memory metrics for testing
metrics := workflow.NewInMemoryMetrics()
engine.SetMetrics(metrics)

// Or use global metrics for action-level instrumentation
workflow.SetGlobalMetrics(metrics)
```

**Available Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `EventProcessed` | Counter + Histogram | eventType, source, success | Events processed with duration |
| `EventRouted` | Counter | eventType, routeName | Events matched to routes |
| `ActionExecuted` | Counter + Histogram | actionType, routeName, success | Actions executed with duration |
| `ActionRetried` | Counter | actionType, routeName, attempt | Retry attempts per action |
| `CircuitBreakerStateChanged` | Counter | endpoint, fromState, toState | Circuit state transitions |
| `CircuitBreakerRejected` | Counter | endpoint | Requests rejected by open circuit |
| `RateLimitWaited` | Counter + Histogram | endpoint | Requests that waited for rate limit |
| `RateLimitRejected` | Counter | endpoint | Requests rejected by rate limiter |
| `DLQPushed` | Counter | routeName, actionType, errorType | Events pushed to DLQ |
| `DLQPopped` | Counter | routeName, success | Events reprocessed from DLQ |
| `HTTPRequestCompleted` | Counter + Histogram | endpoint, method, statusCode | HTTP requests with duration |

**Metrics Interface:**
```go
type Metrics interface {
    // Event processing
    EventProcessed(eventType, source string, success bool, duration time.Duration)
    EventRouted(eventType, routeName string)

    // Action execution
    ActionExecuted(actionType, routeName string, success bool, duration time.Duration)
    ActionRetried(actionType, routeName string, attempt int)

    // Circuit breaker
    CircuitBreakerStateChanged(endpoint string, fromState, toState CircuitState)
    CircuitBreakerRejected(endpoint string)

    // Rate limiting
    RateLimitWaited(endpoint string, waitDuration time.Duration)
    RateLimitRejected(endpoint string)

    // Dead letter queue
    DLQPushed(routeName, actionType, errorType string)
    DLQPopped(routeName string, success bool)
    DLQDepth() int64

    // HTTP requests (for webhook, FHIR actions)
    HTTPRequestCompleted(endpoint, method string, statusCode int, duration time.Duration)
}
```

**Built-in Implementations:**

| Implementation | Use Case |
|---------------|----------|
| `NoOpMetrics` | Default, discards all metrics |
| `InMemoryMetrics` | Testing, inspectable counters |
| Custom | Production (Prometheus, StatsD, etc.) |

**Prometheus Adapter (Reference Implementation):**

A ready-to-use Prometheus adapter is included:

```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Create Prometheus metrics with defaults
promMetrics, err := workflow.NewPrometheusMetrics(nil)
if err != nil {
    log.Fatal(err)
}

// Configure workflow engine
engine.SetMetrics(promMetrics)
workflow.SetGlobalMetrics(promMetrics) // For action-level metrics

// Expose /metrics endpoint
http.Handle("/metrics", promMetrics.Handler())
http.ListenAndServe(":9090", nil)
```

**Custom Configuration:**
```go
config := &workflow.PrometheusConfig{
    Namespace:   "myapp",           // Metric prefix (default: fi_fhir)
    Subsystem:   "events",          // Subsystem name (default: workflow)
    ConstLabels: prometheus.Labels{ // Labels added to all metrics
        "service": "healthcare-integration",
        "env":     "production",
    },
    DurationBuckets: []float64{     // Custom histogram buckets
        0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5,
    },
    Registry: customRegistry,        // Use custom registry (optional)
}

promMetrics, err := workflow.NewPrometheusMetrics(config)
```

**Exported Metrics:**
| Metric | Type | Labels |
|--------|------|--------|
| `fi_fhir_workflow_events_processed_total` | Counter | event_type, source, success |
| `fi_fhir_workflow_events_processed_duration_seconds` | Histogram | event_type, source |
| `fi_fhir_workflow_events_routed_total` | Counter | event_type, route_name |
| `fi_fhir_workflow_actions_executed_total` | Counter | action_type, route_name, success |
| `fi_fhir_workflow_actions_executed_duration_seconds` | Histogram | action_type, route_name |
| `fi_fhir_workflow_action_retries_total` | Counter | action_type, route_name |
| `fi_fhir_workflow_circuit_breaker_state_changes_total` | Counter | endpoint, from_state, to_state |
| `fi_fhir_workflow_circuit_breaker_rejections_total` | Counter | endpoint |
| `fi_fhir_workflow_rate_limit_waits_total` | Counter | endpoint |
| `fi_fhir_workflow_rate_limit_wait_duration_seconds` | Histogram | endpoint |
| `fi_fhir_workflow_rate_limit_rejections_total` | Counter | endpoint |
| `fi_fhir_workflow_dlq_pushed_total` | Counter | route_name, action_type, error_type |
| `fi_fhir_workflow_dlq_popped_total` | Counter | route_name, success |
| `fi_fhir_workflow_dlq_depth` | Gauge | - |
| `fi_fhir_workflow_http_requests_total` | Counter | endpoint, method, status_code |
| `fi_fhir_workflow_http_requests_duration_seconds` | Histogram | endpoint, method |

**Implementation:** `internal/workflow/metrics_prometheus.go`

**Inspecting Metrics (Testing):**
```go
metrics := workflow.NewInMemoryMetrics()
engine.SetMetrics(metrics)

// Process events...
engine.Process(event)

// Check metrics
snapshot := metrics.Snapshot()
fmt.Printf("Events processed: %d\n", snapshot.EventsProcessed)
fmt.Printf("Successful: %d, Failed: %d\n", snapshot.EventsSucceeded, snapshot.EventsFailed)
fmt.Printf("HTTP requests: %d\n", snapshot.HTTPRequests)
```

**Implementation:** `internal/workflow/metrics.go`

### Distributed Tracing

The workflow engine supports distributed tracing via a pluggable tracer interface. Tracing is disabled by default (no-op) and can be enabled with an OpenTelemetry adapter or custom implementation.

**Setup:**
```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Create workflow engine
engine, _ := workflow.NewEngine(workflowConfig)

// Create OpenTelemetry tracer
tracer, err := workflow.NewOTelTracer(&workflow.OTelConfig{
    ServiceName:    "my-healthcare-service",
    ServiceVersion: "1.0.0",
    Environment:    "production",
})
if err != nil {
    log.Fatal(err)
}
defer tracer.Shutdown(context.Background())

// Configure workflow engine
engine.SetTracer(tracer)

// Or set global tracer for action-level spans
workflow.SetGlobalTracer(tracer)
```

**Span Hierarchy:**
```
workflow.process (root span)
├── workflow.route (one per matched route)
│   ├── workflow.transform (one per transform)
│   ├── workflow.action (one per action)
│   │   └── http.request (for webhook/FHIR actions)
```

**Span Names and Attributes:**

| Span Name | Attributes | Description |
|-----------|------------|-------------|
| `workflow.process` | event.type, event.source | Root span for event processing |
| `workflow.route` | route.name, route.matched | Route evaluation and execution |
| `workflow.transform` | transform.type, transform.index | Transform application |
| `workflow.action` | action.type, route.name, action.success | Action execution |
| `http.request` | http.method, http.url, http.status_code | HTTP client calls |

**Tracer Interface:**
```go
type Tracer interface {
    StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
}

type Span interface {
    End()
    SetAttribute(key string, value interface{})
    SetStatus(code SpanStatus, message string)
    RecordError(err error)
    AddEvent(name string, attrs ...SpanAttribute)
}
```

**Built-in Implementations:**

| Implementation | Use Case |
|---------------|----------|
| `NoOpTracer` | Default, creates no-op spans |
| `OTelTracer` | OpenTelemetry integration (OTLP, Jaeger, Zipkin, etc.) |
| Custom | Integrate with any tracing backend |

**OpenTelemetry Configuration:**
```go
import (
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Create OTLP exporter for Jaeger/Tempo/etc
exporter, err := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("localhost:4317"),
    otlptracegrpc.WithInsecure(),
)

config := &workflow.OTelConfig{
    ServiceName:    "healthcare-integration",
    ServiceVersion: "2.0.0",
    Environment:    "staging",
    Exporter:       exporter,
    Sampler:        sdktrace.AlwaysSample(), // or sdktrace.TraceIDRatioBased(0.1)
}

tracer, _ := workflow.NewOTelTracer(config)
```

**Context Propagation:**
```go
// Process with existing trace context
ctx := extractTraceContext(incomingRequest) // e.g., from HTTP headers
result := engine.ProcessWithContext(ctx, event)

// Extract span from context (for adding attributes later)
span := workflow.SpanFromContext(ctx)
span.SetAttribute("custom.key", "value")
```

**Context-Aware Actions:**
```go
// Custom action with tracing support
type MyAction struct{}

func (a *MyAction) ExecuteWithContext(ctx context.Context, event interface{}, config map[string]string) error {
    // Child spans will be linked to parent action span
    _, span := workflow.GetGlobalTracer().StartSpan(ctx, "my.operation")
    defer span.End()

    // ... operation logic
    return nil
}

// Register context-aware action
engine.RegisterAction("my_action", &MyAction{})
```

**Implementation Files:**
- `internal/workflow/tracing.go` - Interface and no-op implementation
- `internal/workflow/tracing_otel.go` - OpenTelemetry adapter

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

### Phase 1 (MVP) ✅
- `log` - Structured logging - see `internal/workflow/actions.go`
- `webhook` - HTTP callbacks - see `internal/workflow/actions.go`
- `fhir` - FHIR R4 server integration - see `internal/workflow/fhir.go`

### Phase 2 ✅
- `database` - SQL/NoSQL storage - see `internal/workflow/database.go`
- `queue` - Message queue publishing - see `internal/workflow/queue.go`

### Phase 3 🔲
- `email` - Email notifications (planned)
- `file` - File output (planned)
- `custom` - User-defined action plugins (planned)

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

### Eligibility Verification Processing

```yaml
workflow:
  name: eligibility_processing
  version: "1.0"

  routes:
    # Track all eligibility inquiries
    - name: log_inquiries
      filter:
        event_type: eligibility_inquiry
      actions:
        - type: database
          connection: ${DATABASE_URL}
          table: eligibility_inquiries
          operation: insert
          mapping_id: id
          mapping_trace_number: trace_number
          mapping_subscriber_id: subscriber.identifiers.member_id
          mapping_payer_id: information_source.identifiers.payer_id
          mapping_service_types: inquiry.service_types
          mapping_timestamp: timestamp
          mapping_payload: __raw__

    # Handle successful eligibility responses
    - name: active_coverage
      filter:
        event_type: eligibility_response
        condition: event.status.eligible == true
      actions:
        - type: database
          connection: ${DATABASE_URL}
          table: eligibility_cache
          operation: upsert
          conflict_on: subscriber_id,payer_id
          mapping_subscriber_id: subscriber.identifiers.member_id
          mapping_payer_id: information_source.identifiers.payer_id
          mapping_status: status.status
          mapping_plan_name: status.plan_name
          mapping_plan_begin: plan_begin_date
          mapping_plan_end: plan_end_date
          mapping_benefits: __raw__
        - type: log
          level: info
          message: "Eligibility confirmed: {{.subscriber.name.family}}, {{.subscriber.name.given}} - {{.status.plan_name}}"

    # Alert on eligibility rejections
    - name: eligibility_errors
      filter:
        event_type: eligibility_response
        condition: size(event.errors) > 0
      actions:
        - type: webhook
          url: https://notifications.hospital.org/eligibility-error
          method: POST
          token: ${NOTIFICATION_TOKEN}
        - type: log
          level: warn
          message: "Eligibility rejected: {{.errors}}"
```

### Claim Status Tracking (276/277)

```yaml
name: claim_status_processing
version: "1.0"

routes:
  # Track all claim status requests
  - name: log_status_requests
    filter:
      event_type: claim_status_request
    actions:
      - type: database
        connection: ${DATABASE_URL}
        table: claim_status_requests
        operation: insert
        mapping_id: id
        mapping_trace_number: trace_number
        mapping_claim_id: inquiry.claim_submitter_id
        mapping_payer_claim_id: inquiry.payer_claim_id
        mapping_subscriber_id: subscriber.identifiers.member_id
        mapping_payer_id: payer.identifiers.payer_id
        mapping_timestamp: timestamp

  # Handle finalized claims (A2 status)
  - name: finalized_claims
    filter:
      event_type: claim_status_response
      condition: event.statuses.exists(s, s.status_category_code == "A2")
    actions:
      - type: database
        connection: ${DATABASE_URL}
        table: claim_status_cache
        operation: upsert
        conflict_on: claim_submitter_id,payer_id
        mapping_claim_id: claim_submitter_id
        mapping_payer_claim_id: payer_claim_id
        mapping_status: statuses[0].status_category_code
        mapping_status_description: statuses[0].status_category_description
        mapping_updated_at: timestamp
      - type: log
        level: info
        message: "Claim finalized: {{.claim_submitter_id}} - {{.statuses[0].status_category_description}}"

  # Alert on denied/rejected claims (A5, A8 status)
  - name: denied_claims
    filter:
      event_type: claim_status_response
      condition: event.statuses.exists(s, s.status_category_code in ["A5", "A8"])
    actions:
      - type: webhook
        url: https://notifications.hospital.org/claim-denied
        method: POST
        token: ${NOTIFICATION_TOKEN}
      - type: log
        level: warn
        message: "Claim denied/rejected: {{.claim_submitter_id}} - {{.statuses[0].status_code_description}}"

  # Track pending claims for follow-up (A1 status)
  - name: pending_claims
    filter:
      event_type: claim_status_response
      condition: event.statuses.exists(s, s.status_category_code == "A1")
    actions:
      - type: database
        connection: ${DATABASE_URL}
        table: pending_claims_followup
        operation: upsert
        conflict_on: claim_submitter_id
        mapping_claim_id: claim_submitter_id
        mapping_status: statuses[0].status_code
        mapping_next_check: __now_plus_24h__
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

# Record events for replay testing
fi-fhir workflow record -c workflow.yaml -o recordings.json events.json

# Replay recordings against modified workflow
fi-fhir workflow replay -c workflow_v2.yaml -d recordings.json

# Simulate without side effects
fi-fhir workflow simulate -c workflow.yaml -v events.json
```

### CLI Testing Commands

| Command | Description |
|---------|-------------|
| `workflow record` | Process events and save results for later replay |
| `workflow replay` | Replay recordings and compare with original results |
| `workflow simulate` | Process with mock actions (no HTTP, DB, queue calls) |

**Record → Replay Workflow:**
```bash
# 1. Record production-like events with current workflow
fi-fhir workflow record -c workflow_v1.yaml -o baseline.json events.json

# 2. After making changes, replay to detect differences
fi-fhir workflow replay -c workflow_v2.yaml -d baseline.json

# 3. Use --output for CI integration
fi-fhir workflow replay -c workflow_v2.yaml -o results.json baseline.json
if [ $? -ne 0 ]; then
  echo "Workflow regression detected!"
  cat results.json
  exit 1
fi
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
- [x] Retry/error handling with exponential backoff (`internal/workflow/retry.go`)
  - [x] Configurable max retries, delays, multiplier, jitter
  - [x] Customizable retryable status codes
  - [x] Context-aware cancellation support
  - [x] Applied to webhook and FHIR actions
- [x] Circuit breaker pattern (`internal/workflow/circuit_breaker.go`)
  - [x] Three-state machine: Closed → Open → Half-Open
  - [x] Per-endpoint breakers via global registry
  - [x] Configurable failure/success thresholds and timeout
  - [x] Applied to webhook and FHIR actions
- [x] Dead letter queue for failed events (`internal/workflow/dlq.go`)
  - [x] MemoryDLQ and FileDLQ backends
  - [x] Error classification by type
  - [x] Reprocessing with attempt tracking
  - [x] Callback notifications for alerting
- [x] Rate limiting for high-volume event streams (`internal/workflow/ratelimit.go`)
  - [x] Token bucket algorithm implementation
  - [x] Per-endpoint rate limiters via registry
  - [x] Configurable rate, burst, and wait behavior
  - [x] Applied to webhook and FHIR actions

### Phase 4: CLI & Tooling ✅
- [x] `workflow run` command
- [x] `workflow validate` command
- [x] `workflow dry-run` command

### Phase 5: Observability ✅
- [x] Metrics interface (`internal/workflow/metrics.go`)
  - [x] Pluggable backend design (NoOp, InMemory, Custom)
  - [x] Event processing metrics (processed, routed, success/failure)
  - [x] Action execution metrics with timing
  - [x] HTTP request metrics (endpoint, method, status code, duration)
  - [x] Circuit breaker state change metrics
  - [x] Rate limit wait/rejection metrics
  - [x] DLQ push/pop metrics
- [x] Engine instrumentation
  - [x] Automatic metrics collection in Process()
  - [x] Action timing and success tracking
  - [x] DLQ reprocessing metrics
- [x] Action instrumentation
  - [x] HTTP metrics for webhook action
  - [x] HTTP metrics for FHIR actions (single resource and bundle)
- [x] InMemoryMetrics for testing with snapshot support
- [x] Prometheus adapter (`internal/workflow/metrics_prometheus.go`)
  - [x] Full Metrics interface implementation
  - [x] Configurable namespace, subsystem, and const labels
  - [x] Custom histogram buckets for duration metrics
  - [x] HTTP handler for /metrics endpoint
  - [x] Endpoint URL sanitization to prevent cardinality explosion
- [x] Distributed tracing (`internal/workflow/tracing.go`, `tracing_otel.go`)
  - [x] Pluggable Tracer interface (NoOp, OTel, Custom)
  - [x] OpenTelemetry adapter with configurable exporter and sampler
  - [x] Span hierarchy: process → route → transform/action → http
  - [x] Context propagation via ProcessWithContext()
  - [x] ContextActionHandler interface for action-level spans
  - [x] HTTP spans for webhook and FHIR actions
- [x] Grafana dashboard templates (`dashboards/grafana/`)
  - [x] Workflow overview dashboard with all metric categories
  - [x] Import instructions and provisioning examples
  - [x] Sample alerting rules
- [x] Log correlation with trace IDs (`internal/workflow/logging.go`)
  - [x] Structured logger interface with trace context extraction
  - [x] JSON and text output formats
  - [x] Automatic trace_id and span_id inclusion in logs
  - [x] Engine integration with debug/info/error logging
  - [x] Global and per-engine logger configuration
  - [x] Helper functions for manual trace context extraction
- [x] Alerting rule templates (`dashboards/alerting/`)
  - [x] Prometheus alerting rules (standalone and Kubernetes CRD)
  - [x] 18 alerts across 5 categories (availability, latency, resilience, DLQ, HTTP)
  - [x] Alertmanager routing examples
  - [x] Customization guidance for different traffic patterns
- [x] Health check endpoints (`internal/workflow/health.go`)
  - [x] Liveness probe (/health) for Kubernetes
  - [x] Readiness probe (/ready) for Kubernetes
  - [x] Pluggable HealthChecker interface
  - [x] Built-in checkers: Engine, DLQ, HTTP, CircuitBreaker
  - [x] Concurrent checks with timeout and caching
- [x] Configuration validation (`internal/workflow/validate.go`)
  - [x] Workflow structure validation (name, routes, actions)
  - [x] CEL expression syntax validation
  - [x] Action-specific validation (webhook, FHIR, database, queue)
  - [x] Transform validation (set_field, map_terminology, redact)
  - [x] Error/warning/info severity levels with codes

### Log Correlation

The workflow engine supports structured logging with automatic trace correlation. When configured with a logger and tracer, all log entries include trace_id and span_id for correlation with distributed traces.

**Setup:**
```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Create workflow engine
engine, _ := workflow.NewEngine(workflowConfig)

// Create structured logger with trace correlation
logger := workflow.NewStructuredLogger(&workflow.LoggerConfig{
    Level:          workflow.LevelInfo,
    Format:         workflow.FormatJSON,
    Output:         os.Stdout,
    IncludeTraceID: true,
    IncludeSpanID:  true,
    ServiceName:    "my-healthcare-service",
})

// Configure workflow engine
engine.SetLogger(logger)

// Or set global logger for all engines
workflow.SetGlobalLogger(logger)
```

**Logger Configuration:**
| Option | Default | Description |
|--------|---------|-------------|
| `Level` | LevelInfo | Minimum log level (Debug, Info, Warn, Error) |
| `Format` | FormatText | Output format (FormatText, FormatJSON) |
| `Output` | os.Stdout | io.Writer for log output |
| `IncludeTraceID` | true | Include trace_id from span context |
| `IncludeSpanID` | true | Include span_id from span context |
| `ServiceName` | "" | Optional service name added to all entries |

**Log Output Formats:**

JSON format (for log aggregators like Loki, ELK):
```json
{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","service":"healthcare","trace_id":"abc123...","span_id":"def456...","msg":"event processed","event_type":"patient_admit","duration_ms":45}
```

Text format (human-readable):
```
[2024-01-15T10:30:00Z] INFO: event processed [trace_id=abc123... span_id=def456...] event_type=patient_admit duration_ms=45
```

**Logger Interface:**
```go
type Logger interface {
    Debug(ctx context.Context, msg string, fields ...Field)
    Info(ctx context.Context, msg string, fields ...Field)
    Warn(ctx context.Context, msg string, fields ...Field)
    Error(ctx context.Context, msg string, fields ...Field)
    WithFields(fields ...Field) Logger
}
```

**Creating Child Loggers:**
```go
// Create logger with base fields (added to all log entries)
routeLogger := logger.WithFields(
    workflow.F("route", "patient_admits"),
    workflow.F("source", "epic_adt"),
)

// All subsequent log calls include these fields
routeLogger.Info(ctx, "processing event", workflow.F("mrn", "12345"))
```

**Manual Trace Context Extraction:**
```go
// Extract trace/span IDs for external use
traceID := workflow.TraceIDFromContext(ctx)
spanID := workflow.SpanIDFromContext(ctx)

// Get fields for adding to existing loggers
fields := workflow.TraceContextFields(ctx)
// Returns: []Field{{"trace_id", "abc..."}, {"span_id", "def..."}}
```

**Built-in Implementations:**
| Implementation | Use Case |
|---------------|----------|
| `StructuredLogger` | Production logging with trace correlation |
| `NoOpLogger` | Default, discards all logs |
| Custom | Integrate with existing logging infrastructure |

**Engine Logging:**
When a logger is configured, the engine automatically logs:
- `DEBUG`: Event processing start with type and source
- `INFO`: Event processing completion with duration and routes matched
- `ERROR`: Action failures with error details and duration

**Correlating Logs with Traces:**
```go
// Set up both tracer and logger
tracer, _ := workflow.NewOTelTracer(&workflow.OTelConfig{
    ServiceName: "healthcare-integration",
})
engine.SetTracer(tracer)

logger := workflow.NewStructuredLogger(&workflow.LoggerConfig{
    Level:          workflow.LevelDebug,
    Format:         workflow.FormatJSON,
    IncludeTraceID: true,
    IncludeSpanID:  true,
})
engine.SetLogger(logger)

// Process event - logs will include trace context
result := engine.ProcessWithContext(ctx, event)

// Sample log output:
// {"timestamp":"...","level":"INFO","trace_id":"abc123","span_id":"def456",
//  "msg":"event processed","event_type":"patient_admit","duration_ms":45}
```

**Querying Correlated Logs (Grafana/Loki Example):**
```promql
{service="healthcare-integration"} |= `trace_id=abc123`
```

**Implementation:** `internal/workflow/logging.go`, `internal/workflow/logging_test.go`

### Alerting Rules

Pre-built Prometheus alerting rules are provided for monitoring workflow health:

**Installation:**
```bash
# Standalone Prometheus
cp dashboards/alerting/workflow-alerts.yaml /etc/prometheus/rules/

# Kubernetes with Prometheus Operator
kubectl apply -f dashboards/alerting/workflow-alerts-k8s.yaml -n monitoring
```

**Alert Categories:**

| Category | Alerts | Description |
|----------|--------|-------------|
| Availability | 3 | Error rates, action failures, no-event detection |
| Latency | 3 | p99 processing time for events, actions, HTTP |
| Resilience | 5 | Circuit breaker, retry rate, rate limiting |
| DLQ | 4 | Queue depth, push rate, reprocessing failures |
| HTTP | 3 | 5xx/4xx error rates, authentication failures |

**Key Alerts:**

| Alert | Severity | Threshold |
|-------|----------|-----------|
| `WorkflowHighErrorRate` | Critical | > 5% error rate for 5 minutes |
| `WorkflowCircuitBreakerOpen` | Critical | Circuit opens for any endpoint |
| `WorkflowDLQCritical` | Critical | > 1000 events in DLQ |
| `WorkflowHighEventLatency` | Warning | p99 > 5 seconds for 10 minutes |
| `WorkflowHighRetryRate` | Warning | > 30% actions retried |

**Alertmanager Routing Example:**
```yaml
route:
  receiver: 'default'
  routes:
    - match:
        service: fi-fhir
        severity: critical
      receiver: 'pagerduty'
    - match:
        service: fi-fhir
        severity: warning
      receiver: 'slack'
```

**Customization:**

Adjust thresholds for your environment:
```yaml
# High-traffic: tighter thresholds, faster alerts
- alert: WorkflowHighErrorRate
  expr: |
    sum(rate(fi_fhir_workflow_events_processed_total{success="false"}[1m]))
    / sum(rate(fi_fhir_workflow_events_processed_total[1m]))
    > 0.01  # 1% instead of 5%
  for: 2m   # 2 minutes instead of 5

# Low-traffic: longer windows to avoid false positives
- alert: WorkflowNoEventsProcessed
  expr: sum(rate(fi_fhir_workflow_events_processed_total[1h])) == 0
  for: 2h  # 2 hours instead of 30 minutes
```

**Implementation:** `dashboards/alerting/workflow-alerts.yaml`, `dashboards/alerting/workflow-alerts-k8s.yaml`

### Health Check Endpoints

The workflow package provides Kubernetes-compatible health check endpoints for liveness and readiness probes.

**Setup:**
```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Create health service
health := workflow.NewHealthService(&workflow.HealthConfig{
    Version:        "1.0.0",
    LivenessPath:   "/health",
    ReadinessPath:  "/ready",
    Timeout:        5 * time.Second,
    IncludeDetails: true,
})

// Register liveness checks (lightweight - is the process alive?)
health.RegisterLivenessCheck("app", workflow.AlwaysHealthy())

// Register readiness checks (can the service handle traffic?)
health.RegisterReadinessCheck("engine", workflow.EngineHealthChecker(engine))
health.RegisterReadinessCheck("dlq", workflow.DLQHealthChecker(dlq, 100, 1000))
health.RegisterReadinessCheck("fhir", workflow.HTTPHealthChecker(fhirURL, 2*time.Second))
health.RegisterReadinessCheck("circuits", workflow.CircuitBreakerHealthChecker(
    "https://fhir.hospital.org",
    "https://api.payer.com",
))

// Mount on HTTP server
http.Handle("/", health.Handler())
http.ListenAndServe(":8080", nil)
```

**Response Format:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "components": [
    {"name": "engine", "status": "healthy", "message": "Engine operational", "checked_at": "..."},
    {"name": "dlq", "status": "healthy", "message": "DLQ depth: 5 events", "checked_at": "..."},
    {"name": "circuits", "status": "degraded", "message": "1 circuit breaker(s) open", "checked_at": "..."}
  ],
  "checked_at": "2024-01-15T10:30:00Z"
}
```

**Health Status Levels:**
| Status | HTTP Code | Description |
|--------|-----------|-------------|
| `healthy` | 200 | All checks passing |
| `degraded` | 200 | Some issues but can handle traffic |
| `unhealthy` | 503 | Cannot handle traffic |

**Built-in Health Checkers:**

| Checker | Description |
|---------|-------------|
| `AlwaysHealthy()` | Always returns healthy (basic liveness) |
| `EngineHealthChecker(engine)` | Checks engine has workflow loaded |
| `DLQHealthChecker(dlq, warn, crit)` | Checks DLQ depth against thresholds |
| `HTTPHealthChecker(url, timeout)` | Checks HTTP endpoint reachability |
| `CircuitBreakerHealthChecker(endpoints...)` | Checks circuit breaker states |
| `CustomHealthChecker(name, fn)` | Custom check from simple function |

**Kubernetes Integration:**
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

**Features:**
- Concurrent check execution with timeout
- Readiness response caching (1 second) to avoid hammering dependencies
- Degraded status allows traffic while indicating issues
- Component-level details for debugging

**Implementation:** `internal/workflow/health.go`, `internal/workflow/health_test.go`

### Configuration Validation

The workflow package provides comprehensive validation of workflow configurations before execution.

**Programmatic Usage:**
```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Load workflow from YAML
wf, err := workflow.LoadWorkflowFromFile("workflow.yaml")
if err != nil {
    log.Fatal(err)
}

// Validate workflow
result, err := workflow.ValidateWorkflow(wf)
if err != nil {
    log.Fatal(err)
}

if !result.Valid {
    for _, e := range result.Errors {
        fmt.Printf("ERROR [%s]: %s - %s\n", e.Code, e.Path, e.Message)
    }
    os.Exit(1)
}

for _, w := range result.Warnings {
    fmt.Printf("WARN [%s]: %s - %s\n", w.Code, w.Path, w.Message)
}

fmt.Println(result.Summary())
```

**Validation Categories:**

| Category | Checks |
|----------|--------|
| Structure | Workflow name, version, route names, duplicate detection |
| Filters | CEL expression syntax, filter criteria presence |
| Transforms | set_field format, map_terminology fields, redact fields |
| Actions | Type-specific required fields, URL/template validation |

**Severity Levels:**

| Level | Behavior | Example |
|-------|----------|---------|
| `error` | Prevents workflow execution | Missing required field |
| `warning` | Workflow runs but may have issues | No authentication configured |
| `info` | Informational only | Using default values |

**Error Codes:**

| Code | Description |
|------|-------------|
| `MISSING_NAME` | Workflow name not specified |
| `DUPLICATE_ROUTE_NAME` | Two routes have the same name |
| `INVALID_CEL` | CEL expression cannot be compiled |
| `MISSING_WEBHOOK_URL` | Webhook action missing URL |
| `MISSING_FHIR_ENDPOINT` | FHIR action missing endpoint |
| `MISSING_OAUTH_CLIENT_ID` | OAuth configured without client_id |
| `MISSING_DB_CONNECTION` | Database action missing connection |
| `MISSING_QUEUE_DRIVER` | Queue action missing driver |
| `INVALID_SET_FIELD` | set_field not in "path = value" format |
| `NO_ROUTES` | Workflow has no routes defined |
| `NO_FILTER` | Route has no filter criteria |
| `NO_ACTIONS` | Route has no actions |

**Validation Result:**
```go
type ValidationResult struct {
    Valid    bool              // false if any errors
    Errors   []ValidationError // Fatal issues
    Warnings []ValidationError // Non-fatal issues
    Info     []ValidationError // Informational
}

type ValidationError struct {
    Path     string            // e.g., "routes[0].filter.condition"
    Message  string            // Human-readable message
    Severity ValidationSeverity
    Code     string            // Machine-readable code
}
```

**CLI Integration Example:**
```bash
# Validate workflow before running
fi-fhir workflow validate workflow.yaml

# Output:
# ERROR [MISSING_FHIR_ENDPOINT]: routes[0].actions[0].endpoint - FHIR action requires endpoint
# WARN [NO_FHIR_AUTH]: routes[0].actions[0] - No authentication configured for FHIR action
# Workflow configuration invalid: 1 error(s), 1 warning(s)
```

**Implementation:** `internal/workflow/validate.go`, `internal/workflow/validate_test.go`

### Event Replay / Simulation

The workflow package provides event recording, replay, and simulation capabilities for testing and debugging workflows.

#### Event Recording

Record events as they're processed for later replay:

```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Option 1: Wrap engine with automatic recording
recorder := workflow.NewMemoryRecorder()
engine, _ := workflow.NewRecordingEngine(workflowConfig, recorder)

// Process events - they're automatically recorded
engine.Process(event1)
engine.Process(event2)

// Export to file for later analysis
recorder.Export("/path/to/events.json")

// Option 2: Manual recording after processing
engine, _ := workflow.NewEngine(workflowConfig)
recorder := workflow.NewMemoryRecorder()

result := engine.Process(event)
recorder.Record(event, result) // Record with processing result
```

**Recorder Implementations:**

| Implementation | Use Case | Persistence |
|---------------|----------|-------------|
| `MemoryRecorder` | Testing, short-term analysis | None (in-memory) |
| `FileRecorder` | Long-term storage, production debugging | JSON files |

**Recorder Options:**
```go
// Limit recorder size (oldest events dropped)
recorder := workflow.NewMemoryRecorder(workflow.WithMaxSize(1000))

// Custom ID prefix for event identifiers
recorder := workflow.NewMemoryRecorder(workflow.WithIDPrefix("prod"))
```

#### Event Replay

Replay recorded events to test workflow changes:

```go
// Load events from file
events, _ := workflow.LoadRecordedEvents("/path/to/events.json")

// Create replayer with new/modified workflow
replayer := workflow.NewEventReplayer(engine, recorder)

// Replay all events and compare results
summary, err := replayer.Replay(ctx, &workflow.ReplayConfig{
    EventTypes: []string{"patient_admit"}, // Filter by type (optional)
    Sources:    []string{"epic_adt"},      // Filter by source (optional)
    Limit:      100,                        // Max events to replay
    Callback: func(recorded *workflow.RecordedEvent, result *workflow.Result, diff *workflow.ReplayDiff) {
        if !diff.RoutingMatch {
            fmt.Printf("Event %s: routing changed\n", recorded.ID)
            fmt.Printf("  Added routes: %v\n", diff.AddedRoutes)
            fmt.Printf("  Removed routes: %v\n", diff.RemovedRoutes)
        }
    },
})

// Check summary
fmt.Printf("Replayed: %d events\n", summary.TotalEvents)
fmt.Printf("Routing changes: %d\n", summary.DifferentRouting)
fmt.Printf("Error state changes: %d\n", summary.DifferentErrors)

// Replay single event for debugging
result, diff, _ := replayer.ReplayOne(ctx, "evt-123")
```

**Replay Summary:**
```go
type ReplaySummary struct {
    TotalEvents      int           // Events replayed
    MatchedRouting   int           // Same routing behavior
    DifferentRouting int           // Different routing
    MatchedErrors    int           // Same error state
    DifferentErrors  int           // Different error state
    TotalDuration    time.Duration // Time taken
    Diffs            []*ReplayDiff // Detailed differences
}
```

**Use Cases:**
- **Regression Testing**: Verify workflow changes don't break existing routing
- **Debug Production Issues**: Replay events that caused problems
- **Load Testing**: Replay production traffic against staging
- **Workflow Migration**: Compare old vs new workflow versions

#### Simulation Engine

Test workflows without side effects using mock actions:

```go
// Create simulation engine (all actions mocked)
sim, _ := workflow.NewSimulationEngine(workflowConfig)

// Process events
sim.Process(event1)
sim.Process(event2)

// Verify behavior with assertions
err := sim.Assert().ActionCalledTimes("webhook", 2)
err = sim.Assert().ActionNotCalled("fhir")
err = sim.Assert().NoErrors()

// Inspect invocations
for _, inv := range sim.InvocationsOf("webhook") {
    fmt.Printf("Webhook called with config: %v\n", inv.Config)
}

// Generate report
report := sim.Report()
fmt.Printf("Total actions: %d\n", report.TotalActions)
fmt.Printf("Errors: %v\n", report.Errors)
```

**Configuring Mock Behavior:**
```go
// Configure specific action to fail
sim.SetMock("webhook", workflow.NewMockAction().WithError(errors.New("network error")))

// Fail after N successful calls
sim.SetMock("fhir", workflow.NewMockAction().FailAfter(3, errors.New("rate limited")))

// Add simulated latency
sim.SetMock("database", workflow.NewMockAction().WithDelay(100 * time.Millisecond))

// Custom response data
sim.SetMock("queue", workflow.NewMockAction().WithResponse(map[string]string{"id": "msg-123"}))
```

**Assertion Methods:**

| Method | Description |
|--------|-------------|
| `ActionCalled(type)` | Assert action was called at least once |
| `ActionNotCalled(type)` | Assert action was never called |
| `ActionCalledTimes(type, n)` | Assert action called exactly N times |
| `TotalActionCalls(n)` | Assert total action invocations |
| `ActionCalledWithConfig(type, config)` | Assert action called with specific config |
| `NoErrors()` | Assert no action errors occurred |

#### Scenario Testing

Run test scenarios against the simulation engine:

```go
runner := workflow.NewScenarioRunner(sim)

scenarios := []*workflow.Scenario{
    {
        Name: "patient admits route to webhook",
        Setup: func(sim *workflow.SimulationEngine) {
            // Optional: configure mocks
        },
        Events: []interface{}{
            map[string]interface{}{"type": "patient_admit", "source": "epic"},
            map[string]interface{}{"type": "patient_admit", "source": "cerner"},
        },
        Assertions: func(a *workflow.Assertions) error {
            if err := a.ActionCalledTimes("webhook", 2); err != nil {
                return err
            }
            return a.ActionNotCalled("fhir")
        },
    },
    {
        Name: "lab results route to FHIR",
        Events: []interface{}{
            map[string]interface{}{"type": "lab_result"},
        },
        Assertions: func(a *workflow.Assertions) error {
            return a.ActionCalled("fhir")
        },
    },
}

// Run all scenarios
results := runner.RunAll(scenarios)
for name, err := range results {
    if err != nil {
        fmt.Printf("FAIL: %s - %v\n", name, err)
    } else {
        fmt.Printf("PASS: %s\n", name)
    }
}
```

**Implementation:** `internal/workflow/replay.go`, `internal/workflow/simulation.go`

---

## Performance Benchmarking

The workflow engine includes comprehensive benchmarks for measuring and tracking performance.

### Running Benchmarks

```bash
# Run all workflow benchmarks
go test -bench=. -benchmem ./internal/workflow/...

# Run specific benchmark categories
go test -bench='BenchmarkEngine' -benchmem ./internal/workflow/...
go test -bench='BenchmarkCEL' -benchmem ./internal/workflow/...
go test -bench='BenchmarkTransform' -benchmem ./internal/workflow/...

# Run with longer duration for more stable results
go test -bench=. -benchmem -benchtime=1s ./internal/workflow/...

# Save baseline for comparison
go test -bench=. -benchmem ./internal/workflow/... > baseline.txt

# Compare against baseline (requires benchstat)
go install golang.org/x/perf/cmd/benchstat@latest
go test -bench=. -benchmem ./internal/workflow/... > current.txt
benchstat baseline.txt current.txt
```

### Benchmark Categories

| Category | Description | Key Metrics |
|----------|-------------|-------------|
| `BenchmarkEngineProcess*` | Event processing throughput | ns/op, allocs/op |
| `BenchmarkCELEvaluate*` | CEL condition evaluation | Cache hit vs miss |
| `BenchmarkFilterMatch*` | Filter matching performance | EventType, Source, CEL |
| `BenchmarkTransform*` | Transform pipeline | SetField, Redact |
| `BenchmarkThroughput*` | Events per second | events/sec |
| `BenchmarkParallel*` | Concurrent processing | Scalability |

### Performance Baselines

Typical performance on modern hardware (Apple M-series, AMD Ryzen):

| Benchmark | Expected | Threshold |
|-----------|----------|-----------|
| Engine.Process (simple) | ~1-2 µs | < 5 µs |
| CEL Evaluate (cached) | ~150-250 ns | < 500 ns |
| CEL Evaluate (compile) | ~40-50 µs | N/A |
| Filter Match (combined) | ~1-1.5 µs | < 3 µs |
| Transform SetField | ~180-200 ns | < 500 ns |

### Using Benchmark Utilities

```go
import "github.com/cblevins/fi-fhir/internal/workflow"

// Create benchmark suite for tracking
suite := workflow.NewBenchmarkSuite("v1.0.0")
suite.AddResult(workflow.BenchmarkResult{
    Name:         "BenchmarkEngineProcess",
    NsPerOp:      1500,
    AllocsPerOp:  30,
    EventsPerSec: 666666,
})

// Compare two suites
baseline := workflow.NewBenchmarkSuite("v1.0.0")
current := workflow.NewBenchmarkSuite("v1.1.0")
// ... add results ...

comparison := workflow.Compare(baseline, current)
fmt.Println(comparison.Summary())

if comparison.HasRegressions() {
    fmt.Println("Performance regression detected!")
}
```

### Performance Thresholds

Define acceptable performance bounds for CI validation:

```go
thresholds := workflow.DefaultWorkflowThresholds()
// Or customize:
thresholds := &workflow.PerformanceThresholds{
    MaxNsPerOp: map[string]float64{
        "BenchmarkEngineProcess": 5000,  // 5µs max
    },
    MinThroughput: map[string]float64{
        "BenchmarkThroughput_Simple": 100000,  // 100k events/sec
    },
}

violations := thresholds.Validate(suite)
if len(violations) > 0 {
    for _, v := range violations {
        fmt.Printf("Threshold violation: %s\n", v)
    }
}
```

### Quick Runtime Benchmark

For runtime performance monitoring:

```go
engine, _ := workflow.NewEngine(myWorkflow)
event := map[string]interface{}{"type": "patient_admit"}

// Run 100ms benchmark
opsPerSec := workflow.QuickBenchmark(engine, event, 100*time.Millisecond)
fmt.Printf("Current throughput: %.0f events/sec\n", opsPerSec)
```

### Performance Optimization Tips

1. **CEL Expression Caching**: CEL expressions are automatically cached after first compilation. Avoid dynamically generating expressions.

2. **Filter Short-Circuit**: Place fast filters (EventType, Source) before CEL conditions in route order.

3. **Transform Pipeline**: Transforms create deep copies of events. Minimize transforms for high-throughput routes.

4. **Parallel Processing**: The engine is safe for concurrent use. Use goroutine pools for parallel event processing.

5. **Metrics Overhead**: In-memory metrics add ~10-15% overhead. Use NoOpMetrics for maximum throughput.

**Implementation:** `internal/workflow/benchmark_test.go`, `internal/workflow/benchmark_util.go`

---

## Load Testing

The workflow engine includes load testing utilities for validating performance under sustained load.

### CLI Load Testing

```bash
# Quick smoke test (10s, 100 RPS)
fi-fhir workflow loadtest -c workflow.yaml -s smoke -v

# Standard load test (60s, 1000 RPS)
fi-fhir workflow loadtest -c workflow.yaml -s standard -v

# Custom load test
fi-fhir workflow loadtest -c workflow.yaml -d 60s -r 2000 -w 8 -v

# Stress test with JSON output
fi-fhir workflow loadtest -c workflow.yaml -s stress --json > results.json

# Burst test (maximum throughput)
fi-fhir workflow loadtest -c workflow.yaml -r 0 -d 10s -v

# List available scenarios
fi-fhir workflow loadtest --list-scenarios
```

### Predefined Scenarios

| Scenario | Duration | Target RPS | Workers | Use Case |
|----------|----------|------------|---------|----------|
| smoke | 10s | 100 | 2 | Quick validation |
| standard | 60s | 1000 | 4 | Normal load testing |
| stress | 120s | 5000 | 8 | High load conditions |
| burst | 30s | unlimited | 16 | Maximum throughput |
| soak | 5min | 500 | 4 | Long-running stability |

### Programmatic Load Testing

```go
import "github.com/cblevins/fi-fhir/internal/workflow"

engine, _ := workflow.NewEngine(myWorkflow)
tester := workflow.NewLoadTester(engine)

config := &workflow.LoadTestConfig{
    Duration:         60 * time.Second,
    TargetRPS:        1000,
    Workers:          4,
    WarmupDuration:   5 * time.Second,
    EventGenerator:   workflow.NewHealthcareEventGenerator(),
    ProgressInterval: 5 * time.Second,
    OnProgress: func(stats workflow.LoadTestProgress) {
        fmt.Printf("[%v] Rate: %.0f/s, P99: %v\n",
            stats.Elapsed, stats.EventsPerSec, stats.P99Latency)
    },
}

result, err := tester.Run(context.Background(), config)
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Summary())

// Validate against thresholds
if !result.Passed(0.90, 0.01, 10*time.Millisecond) {
    log.Fatal("Load test failed performance targets")
}
```

### Event Generators

Different generators for varied load patterns:

```go
// Static event (single event type)
gen := &workflow.StaticEventGenerator{
    Event: map[string]interface{}{"type": "patient_admit"},
}

// Random selection from pool
gen := workflow.NewRandomEventGenerator([]interface{}{
    map[string]interface{}{"type": "patient_admit"},
    map[string]interface{}{"type": "lab_result"},
})

// Weighted distribution (realistic traffic)
gen := workflow.NewWeightedEventGenerator([]workflow.WeightedEvent{
    {Event: map[string]interface{}{"type": "lab_result"}, Weight: 60},
    {Event: map[string]interface{}{"type": "patient_admit"}, Weight: 30},
    {Event: map[string]interface{}{"type": "claim_submitted"}, Weight: 10},
})

// Sequence (round-robin)
gen := &workflow.SequenceEventGenerator{
    Events: []interface{}{event1, event2, event3},
}

// Healthcare mix (realistic distribution)
gen := workflow.NewHealthcareEventGenerator()
```

### Load Test Results

Results include comprehensive metrics:

- **Throughput**: Total events, events/sec, achieved ratio vs target
- **Latency**: Min, mean, P50, P90, P95, P99, P99.9, max, std dev
- **Errors**: Count, rate, individual error details
- **Timing**: Duration, warmup events

### CI/CD Integration

```bash
# Run load test and fail if thresholds not met
fi-fhir workflow loadtest -c workflow.yaml -s standard --json > results.json

# Parse results with jq
if [ $(jq '.ErrorRate > 0.01' results.json) = "true" ]; then
    echo "Error rate exceeded threshold"
    exit 1
fi

if [ $(jq '.LatencyP99 > 10000000' results.json) = "true" ]; then
    echo "P99 latency exceeded 10ms"
    exit 1
fi
```

**Implementation:** `internal/workflow/loadtest.go`, `internal/workflow/loadtest_test.go`

---

## Application Configuration

fi-fhir supports layered configuration following 12-Factor App principles:

```
Priority (highest to lowest):
1. CLI flags (not yet implemented for all options)
2. Environment variables (FI_FHIR_*)
3. Configuration file (YAML)
4. Default values
```

### CLI Commands

```bash
# Show effective configuration (defaults + env overrides)
fi-fhir config show

# Show configuration from file with env overrides
fi-fhir config show -c config.yaml

# Validate configuration file
fi-fhir config validate config.yaml

# List all environment variables
fi-fhir config env

# Generate .env template
fi-fhir config env -f export > .env.template

# Generate sample config file
fi-fhir config init -o config.yaml
fi-fhir config init -m -o minimal.yaml  # minimal version
```

### Configuration File

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  shutdown_timeout: 10s

workflow:
  config_path: workflow.yaml
  max_concurrency: 10
  retry_max_attempts: 3
  dlq_enabled: true

fhir:
  base_url: https://fhir.example.com/r4
  auth_type: oauth2  # none, basic, bearer, oauth2
  oauth2:
    token_url: https://auth.example.com/token
    client_id: my-app
    client_secret: ${secret:OAUTH_CLIENT_SECRET}  # Secret reference

database:
  driver: postgres
  host: localhost
  password: ${secret:DATABASE_PASSWORD}  # Secret reference

observability:
  metrics_enabled: true
  log_level: info
  log_format: json
```

### Environment Variables

All configuration can be overridden via environment variables with the `FI_FHIR_` prefix:

```bash
# Server
FI_FHIR_SERVER_PORT=9000
FI_FHIR_SERVER_READ_TIMEOUT=60s

# Workflow
FI_FHIR_WORKFLOW_CONFIG_PATH=/etc/workflow.yaml
FI_FHIR_WORKFLOW_DRY_RUN=true

# Database
FI_FHIR_DATABASE_HOST=db.example.com
FI_FHIR_DATABASE_PASSWORD=secret123

# Observability
FI_FHIR_LOG_LEVEL=debug
FI_FHIR_TRACING_ENABLED=true
```

See full list: `fi-fhir config env`

### Secrets Management

Sensitive values can be stored separately using the `${secret:KEY}` syntax:

```yaml
# Values in config file
fhir:
  password: ${secret:FHIR_PASSWORD}
database:
  password: ${secret:DB_PASSWORD}

# Secret provider configuration
secrets:
  provider: env  # env, file, vault, aws-ssm, k8s
  options:
    prefix: APP_SECRET_  # For env provider
    path: /run/secrets   # For file provider
```

**Secret Providers:**

| Provider | Description | Use Case |
|----------|-------------|----------|
| `env` | Environment variables | Local development, container environments |
| `file` | Files in directory (filename=key) | Docker secrets, Kubernetes secrets mounted as volumes |
| `vault` | HashiCorp Vault (planned) | Enterprise deployments |
| `aws-ssm` | AWS Systems Manager Parameter Store (planned) | AWS deployments |
| `k8s` | Kubernetes secrets (uses file provider) | Kubernetes deployments |

**Implementation:** `pkg/config/config.go`, `pkg/config/secrets.go`

---

## Deployment

### Docker

Build and run with Docker:

```bash
# Build image
docker build -t fi-fhir:latest .

# Run with defaults
docker run -p 8080:8080 fi-fhir:latest

# Run with config file
docker run -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config/config.yaml \
  -e FI_FHIR_WORKFLOW_CONFIG_PATH=/app/config/workflow.yaml \
  fi-fhir:latest
```

### Docker Compose

For local development with full stack:

```bash
# Start all services (fi-fhir, Postgres, Kafka, FHIR server, Jaeger, Prometheus, Grafana)
docker-compose up -d

# View logs
docker-compose logs -f fi-fhir

# Access services:
# - fi-fhir API: http://localhost:8080
# - fi-fhir metrics: http://localhost:9090/metrics
# - HAPI FHIR: http://localhost:8090
# - Jaeger UI: http://localhost:16686
# - Prometheus: http://localhost:9091
# - Grafana: http://localhost:3000 (admin/admin)

# Stop all services
docker-compose down
```

### Kubernetes

Deploy using Kustomize:

```bash
# Deploy base configuration
kubectl apply -k deploy/kubernetes/base/

# Deploy production overlay
kubectl apply -k deploy/kubernetes/overlays/production/

# Check status
kubectl -n fi-fhir get pods
kubectl -n fi-fhir get services
kubectl -n fi-fhir logs -f deployment/fi-fhir
```

Key manifests in `deploy/kubernetes/base/`:
- `namespace.yaml` - Namespace definition
- `deployment.yaml` - Deployment with health checks, resource limits
- `service.yaml` - ClusterIP service for internal access
- `ingress.yaml` - Ingress with TLS (nginx + cert-manager)
- `configmap.yaml` - Application and workflow configuration
- `secret.yaml` - Sensitive configuration (replace in production)
- `pdb.yaml` - Pod disruption budget for HA

### Helm Chart

For more flexible deployments with parameterized values:

```bash
# Install with defaults
helm install fi-fhir deploy/helm/fi-fhir/

# Install with custom values
helm install fi-fhir deploy/helm/fi-fhir/ \
  --set replicaCount=3 \
  --set config.server.port=8080 \
  --set config.database.enabled=true \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=fi-fhir.example.com

# Install with values file
helm install fi-fhir deploy/helm/fi-fhir/ -f custom-values.yaml

# Upgrade deployment
helm upgrade fi-fhir deploy/helm/fi-fhir/ --reuse-values \
  --set image.tag=v1.2.0

# View generated manifests without installing
helm template fi-fhir deploy/helm/fi-fhir/ --debug

# Uninstall
helm uninstall fi-fhir
```

Key configurable values:
- `replicaCount` - Number of pod replicas (default: 2)
- `config.server.*` - Server settings (port, timeouts)
- `config.workflow.*` - Workflow engine settings (concurrency, retries, DLQ)
- `config.database.enabled` - Enable database action (requires secrets)
- `config.queue.enabled` - Enable queue action (requires secrets)
- `autoscaling.enabled` - Enable HPA for auto-scaling
- `ingress.enabled` - Enable Ingress resource with TLS
- `serviceMonitor.enabled` - Enable Prometheus ServiceMonitor
- `podDisruptionBudget.enabled` - Enable PDB for HA

**Implementation:** `deploy/helm/fi-fhir/`

### CI/CD Pipelines

**GitLab CI** (primary) - `.gitlab-ci.yml`:
```yaml
# Pipeline stages
stages:
  - lint        # golangci-lint, go fmt, go vet, helm lint
  - test        # unit tests (Go 1.21, 1.22), integration, benchmarks
  - security    # govulncheck, gosec, trivy
  - build       # binary, docker image
  - release     # multi-arch binaries, docker push, helm package

# Triggered on:
# - Merge requests to main
# - Pushes to main branch
# - Tag pushes (v*)
```

**GitHub Actions** (mirror) - `.github/workflows/`:
- `ci.yaml` - Lint, test, security scan, Docker build on PRs/main
- `release.yaml` - Multi-arch binaries, Docker push to GHCR, Helm chart on tags

**Security scanning** (HIPAA compliance):
- `govulncheck` - Go vulnerability database
- `gosec` - Static security analysis
- `trivy` - Container vulnerability scanning
- `golangci-lint` - Comprehensive Go linting

**Release process**:
```bash
# Create and push a tag to trigger release
git tag v1.0.0
git push origin v1.0.0

# CI will automatically:
# 1. Run full test suite
# 2. Build multi-arch binaries (linux, darwin, windows × amd64, arm64)
# 3. Push Docker image to registry with version tags
# 4. Package and publish Helm chart
# 5. Create GitLab/GitHub release with artifacts
```

**Implementation:** `.gitlab-ci.yml`, `.github/workflows/`, `.golangci.yml`

---

## See Also

- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Profile configuration feeds into workflow event parsing
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - FHIR action uses US Core mapper for resource generation
- [TYPESCRIPT-SDK.md](TYPESCRIPT-SDK.md) - TypeScript Workflow class wraps CLI commands
- [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) - EDI events can be routed through workflows
- [PRODUCTION-HARDENING.md](../operations/PRODUCTION-HARDENING.md) - Security hardening for HIPAA compliance
- [RUNBOOK.md](../operations/RUNBOOK.md) - Operations runbook for troubleshooting and incident response
- [OpenAPI Specification](../../api/openapi.yaml) - REST API documentation (view with Swagger UI or Redoc)
