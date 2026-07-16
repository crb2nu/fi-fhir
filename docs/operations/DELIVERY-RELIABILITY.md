# Durable Delivery Reliability

Slice 2.3 turns the production submission outbox into an optional, restart-safe
Kafka delivery worker. PostgreSQL remains authoritative for delivery state;
Kafka is the first real external queue transport.

## Guarantee boundary

- Ingress acceptance is durable once: one idempotency key still resolves to one
  receipt, canonical event, lineage unit, initial attempt, and outbox row.
- Kafka delivery is at-least-once. Every record uses the stable delivery attempt
  ID as its key and repeats that ID in `fi-fhir-attempt-id`.
- A process can publish to Kafka and fail before recording `published` in
  PostgreSQL. Lease expiry republishes that record. Consumers must suppress
  duplicates by attempt ID when duplicate effects are unsafe.
- Source bytes and credentials are never part of the delivery command. The
  command contains the sanitized canonical event and exact destination revision.
- `202` and positive MLLP ACKs still mean durable acceptance, not Kafka delivery.

There is deliberately no distributed transaction between PostgreSQL and Kafka,
and fi-fhir does not claim universal exactly-once external I/O.

## State machine

```text
pending -> leased -> published
   ^          |
   |          +-> pending (bounded retry or expired lease)
   |          +-> failed -> DLQ
   |                         |
   +-------------------------+ replay (same attempt)

failed -> resubmit -> pending child attempt
```

Workers claim due rows with `FOR UPDATE SKIP LOCKED`. A live lease belongs to one
worker ID. Expired leases are returned to `pending` and audited before another
worker may publish them. Exponential retry is capped by both delay and attempt
count. Repeated retryable failures open a destination-revision circuit; other
work for that exact revision waits until the circuit deadline.

Terminal failures create or reactivate one `integration_delivery_dlq` row.
Append-only `integration_delivery_audit` events record claims, lease recovery,
retry, DLQ, publication, replay, and resubmit. Operator operations are separately
idempotent in `integration_delivery_operations`.

## Runtime configuration

The worker is absent unless `FI_FHIR_DELIVERY_WORKER_ENABLED=true`. It requires
the same PostgreSQL submission database as production ingress plus:

```bash
FI_FHIR_QUEUE_DRIVER=kafka
FI_FHIR_QUEUE_BROKERS=kafka-1:9092,kafka-2:9092
FI_FHIR_QUEUE_CLIENT_ID=fi-fhir-delivery
```

Production credentials require TLS:

```bash
FI_FHIR_QUEUE_TLS=true
FI_FHIR_QUEUE_TLS_ROOT_CA_FILE=/var/run/secrets/fi-fhir-kafka/ca.crt
FI_FHIR_QUEUE_USERNAME=fi-fhir-producer
FI_FHIR_QUEUE_PASSWORD_FILE=/var/run/secrets/fi-fhir-kafka/password
```

SASL/PLAIN without TLS fails startup. All in-sync replicas must acknowledge a
record. The producer is idempotent within a live Kafka producer session, but the
database/Kafka crash boundary can still repeat a record.

Retry, lease, polling, publish timeout, and circuit bounds are listed in
`.env.example`. Invalid or partial configuration fails startup; disabling the
worker leaves durable pending/DLQ state intact.

## Inspect and recover

Inspect counts and safe failure metadata without selecting canonical event JSON:

```sql
SELECT status, count(*)
FROM integration_delivery_outbox
GROUP BY status
ORDER BY status;

SELECT d.tenant_id, d.attempt_id, d.failure_code, d.failure_detail,
       d.failed_at, d.replay_count
FROM integration_delivery_dlq d
WHERE d.active
ORDER BY d.failed_at;
```

Repair the broker/configuration before recovery. Use a PostgreSQL credential
that identifies the operator; `current_user` becomes the immutable audit
principal. Always supply a unique operation key and a specific reason:

```bash
fi-fhir delivery replay \
  --tenant tenant-a \
  --attempt ATTEMPT_ID \
  --idempotency-key incident-123-replay-1 \
  --reason "Kafka ACL repaired under incident 123"

fi-fhir delivery resubmit \
  --tenant tenant-a \
  --attempt ATTEMPT_ID \
  --idempotency-key incident-123-resubmit-1 \
  --reason "Destination revision repaired under incident 123"
```

Replay requeues the same attempt and preserves its downstream idempotency key.
Resubmit creates one deterministic child attempt linked by `parent_attempt_id`.
Repeating either command with the same operation key returns the original result;
reusing a key for another operation fails closed.

Do not mutate outbox, DLQ, circuit, or attempt tables manually. To roll back
publication, set `FI_FHIR_DELIVERY_WORKER_ENABLED=false` and restart; forward-only
migrations and durable work remain available for later recovery.

## Verification

`make delivery-reliability` runs the PostgreSQL 16/Kafka failure-and-replay
proof. It covers concurrent claims, circuit delay, bounded DLQ, expired-lease
restart, idempotent resubmit/replay, stable Kafka keys, a simulated
publish-before-database-ack crash, durable cardinality, and PHI/credential scans.
