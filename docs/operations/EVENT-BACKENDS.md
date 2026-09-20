# Event backends and workflow consumers

`pkg/eventbus` provides Kafka, Redis Streams, and Google Cloud Pub/Sub publishers
and consumers. Workflow `queue` actions register all three drivers automatically.
`log` remains available for local diagnostics; additional drivers can still be
registered with `workflow.RegisterQueueDriver`.

The same `Handler` contract works across the three backends. `JSONHandler` checks
that a message contains one JSON object with string `id`, `type`, and `source`
fields. `Chain` executes handlers in order and stops on the first error.
`Engine.EventHandler()` connects canonical JSON events to existing workflow
filters, transforms, and actions. `eventsourcing.BrokerPublisher` connects an
`OutboxRelay` to a backend and preserves the aggregate key and outbox identity.

## Publish from a workflow

```yaml
workflow:
  name: admissions-to-kafka
  routes:
    - name: admissions
      filter:
        event_type: patient_admit
      actions:
        - type: queue
          driver: kafka
          brokers: localhost:9092
          topic: canonical-events
          key: id
          timeout: 10s
```

Use `driver: redis` with `url: redis://localhost:6379` to publish to a Redis
stream named by `topic`. Use `driver: pubsub` with `project_id: my-project` to
publish to a Google Cloud topic. Pub/Sub uses application default credentials;
topics and subscriptions must already exist. Keys become Pub/Sub ordering keys.
The subscription must enable message ordering when ordering is needed.

## Process events from a broker

```sh
fi-fhir workflow consume --config workflow.yaml --backend backend.yaml
```

Kafka backend configuration:

```yaml
driver: kafka
subscription: canonical-events
options:
  brokers: ${KAFKA_BROKERS}
  group: admission-processors
  tls: "true"
  username: ${KAFKA_USERNAME}
  password: ${KAFKA_PASSWORD}
```

Redis backend configuration:

```yaml
driver: redis
subscription: canonical-events
options:
  url: ${REDIS_URL}
  group: admission-processors
  handler_timeout: 30s
  reclaim_idle: 1m
```

Pub/Sub backend configuration:

```yaml
driver: pubsub
subscription: admission-processors
options:
  project_id: my-project
  timeout: 10s
```

`subscription` means a Kafka topic, Redis stream, or Pub/Sub subscription ID
(a fully qualified Pub/Sub subscription name also works). Environment expansion
applies to the consumer backend file's `options` values. An unset referenced
variable is an error. Protect files containing credentials; use `rediss://` for
Redis TLS. Kafka username/password authentication requires TLS and supports
an optional `ca_file`. Cloud Pub/Sub uses the official client's ADC support.
For tests, `PUBSUB_EMULATOR_HOST` selects an unauthenticated emulator.

| Option | Backend | Meaning |
| --- | --- | --- |
| `brokers` | Kafka | Required comma-separated broker addresses |
| `client_id` | Kafka | Optional broker client identifier |
| `tls`, `ca_file` | Kafka | TLS and optional PEM root certificates |
| `username`, `password` | Kafka | SASL PLAIN over TLS |
| `url` | Redis | Required `redis://` or `rediss://` URL, including database/auth |
| `group` | Kafka, Redis | Required for consumption; consumer group name |
| `project_id` | Pub/Sub | Required Google Cloud project |
| `timeout` | All | Positive publish/network timeout, default `10s` |
| `handler_timeout` | Redis | Handler context deadline, default `30s` |
| `reclaim_idle` | Redis | Abandoned-delivery age, default `1m`; at least twice the handler timeout |

## Acknowledgment and retry behavior

Publish returns after the broker confirms the write. Kafka requires all in-sync
replica acknowledgments. Redis uses `XADD`; persistence across a Redis server
failure depends on the server's AOF/replication configuration. Pub/Sub waits for
the publish result.

Consumers acknowledge only after successful processing:

- Kafka disables automatic offset commits and commits each successful record
  synchronously. Processing is serial and rebalances are held during a handler.
- Redis uses consumer groups, `XACK`, and `XAUTOCLAIM` (Redis 6.2 or newer).
  Restarting the same consumer resumes its pending entries first. New consumer
  instances reclaim abandoned entries after `reclaim_idle`. Recovery can change
  order across consumers. Handlers must honor their context deadline. The driver
  never trims the stream; configure retention without deleting pending entries.
- Pub/Sub calls `Ack` after success and `Nack` on failure. The official client
  extends acknowledgment deadlines while processing.

A handler failure stops consumption and is returned to the caller. The CLI
exits nonzero, leaving the message for retry after repair or deliberate
quarantine. It does not silently acknowledge an invalid event. Workflow
transform/action errors are failures even if the workflow also writes its own
DLQ record. An event that matches no workflow routes is successfully filtered
and acknowledged.

Delivery is **at least once**. A process can fail after a side effect but before
acknowledgment; handlers and workflow actions must tolerate repeated event IDs.
Successful earlier handlers in a `Chain` may run again if a later handler fails.
Cancel a consumer and wait for it to return before closing its backend. The CLI
handles SIGINT/SIGTERM and closes its input and output clients.

These interfaces process canonical events. The server's identity-bound durable
destination dispatcher continues to use its existing destination configuration
and authorization contracts. `FI_FHIR_QUEUE_DRIVER` configures that dispatcher;
it is not the selector for `workflow consume`.

## Verification

`go test -race ./pkg/eventbus ./internal/workflow ./pkg/eventsourcing` exercises
acknowledgment, redelivery, cancellation, canonical JSON validation, and shared
handlers using Kafka's protocol test broker, Redis's protocol test server, and
Google's Pub/Sub gRPC test server. The blocking `test:event-backends` job also
runs against real Kafka and Redis services:

```sh
EVENTBUS_KAFKA_BROKERS=localhost:9092 \
EVENTBUS_REDIS_URL=redis://localhost:6379 make event-backends
```

Source: [backend contract](../../pkg/eventbus/eventbus.go),
[workflow adapters](../../internal/workflow/queue_backends.go),
[consumer command](../../cmd/fi-fhir/workflow_consume.go),
[outbox adapter](../../pkg/eventsourcing/eventbus.go), and
[CI proof](../../ci/test-event-backends.yml).
