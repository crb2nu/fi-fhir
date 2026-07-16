//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	kafkacontainer "github.com/testcontainers/testcontainers-go/modules/kafka"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/twmb/franz-go/pkg/kgo"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestDeliveryReliability_PostgresKafkaFailureReplay(t *testing.T) {
	ctx := t.Context()
	dsn := deliveryPostgresDSN(t, ctx)
	brokers := deliveryKafkaBrokers(t, ctx)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submissionStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	clock := &deliveryTestClock{now: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)}
	seedDelivery(t, db, clock.Now(), "attempt-original", "outbox-original")
	storeA, _ := NewPostgresStore(db, clock.Now)
	storeB, _ := NewPostgresStore(db, clock.Now)

	claimed := make(chan *WorkItem, 2)
	errorsByWorker := make(chan error, 2)
	start := make(chan struct{})
	for index, store := range []*PostgresStore{storeA, storeB} {
		go func(index int, store *PostgresStore) {
			<-start
			item, err := store.Claim(ctx, fmt.Sprintf("worker-%d", index), 5*time.Second)
			claimed <- item
			errorsByWorker <- err
		}(index, store)
	}
	close(start)
	var first *WorkItem
	for range 2 {
		if err := <-errorsByWorker; err != nil {
			t.Fatalf("concurrent Claim: %v", err)
		}
		if item := <-claimed; item != nil {
			if first != nil {
				t.Fatal("two workers leased one outbox row")
			}
			first = item
		}
	}
	if first == nil || first.AttemptID != "attempt-original" {
		t.Fatalf("first claim = %#v", first)
	}

	retryConfig := DefaultConfig()
	retryConfig.MaxAttempts = 2
	retryConfig.RetryBaseDelay = time.Second
	retryConfig.RetryMaxDelay = time.Second
	retryConfig.CircuitFailureThreshold = 1
	retryConfig.CircuitOpenDuration = 2 * time.Second
	if retry, err := storeA.MarkFailed(ctx, *first, testKafkaFailure(), retryConfig); err != nil || !retry {
		t.Fatalf("MarkFailed(retry) retry=%v error=%v", retry, err)
	}
	assertDeliveryState(t, db, "attempt-original", "queued", 2, "pending", "absent")
	assertCircuitState(t, db, "open", 1)

	clock.Advance(time.Second)
	if item, err := storeA.Claim(ctx, "worker-before-circuit", 5*time.Second); err != nil || item != nil {
		t.Fatalf("claim while circuit open item=%#v error=%v", item, err)
	}
	clock.Advance(time.Second)
	leaked, err := storeA.Claim(ctx, "worker-before-restart", 5*time.Second)
	if err != nil || leaked == nil {
		t.Fatalf("claim before restart item=%#v error=%v", leaked, err)
	}
	clock.Advance(5 * time.Second)
	reclaimed, err := storeB.Claim(ctx, "worker-after-restart", 5*time.Second)
	if err != nil || reclaimed == nil || reclaimed.AttemptID != leaked.AttemptID {
		t.Fatalf("reclaim expired lease item=%#v error=%v", reclaimed, err)
	}
	if retry, err := storeB.MarkFailed(ctx, *reclaimed, testKafkaFailure(), retryConfig); err != nil || retry {
		t.Fatalf("MarkFailed(DLQ) retry=%v error=%v", retry, err)
	}
	assertDeliveryState(t, db, "attempt-original", "failed", 2, "failed", "active")

	operator := Operation{
		IdempotencyKey: "resubmit-original-1",
		Principal: integration.Principal{
			ID: "operator-a", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{OperatorRole},
		},
		Reason: "Kafka route repaired",
	}
	childAttemptID, err := storeA.Resubmit(ctx, "tenant-a", "attempt-original", operator)
	if err != nil {
		t.Fatalf("Resubmit: %v", err)
	}
	duplicateChildID, err := storeB.Resubmit(ctx, "tenant-a", "attempt-original", operator)
	if err != nil || duplicateChildID != childAttemptID {
		t.Fatalf("duplicate Resubmit id=%q error=%v", duplicateChildID, err)
	}
	assertCounts(t, db, 1, 1, 2, 2, 1)

	child, err := storeA.Claim(ctx, "worker-child", 5*time.Second)
	if err != nil || child == nil || child.AttemptID != childAttemptID {
		t.Fatalf("claim child item=%#v error=%v", child, err)
	}
	terminalConfig := retryConfig
	terminalConfig.MaxAttempts = 1
	if retry, err := storeA.MarkFailed(ctx, *child, testKafkaFailure(), terminalConfig); err != nil || retry {
		t.Fatalf("dead-letter child retry=%v error=%v", retry, err)
	}
	replay := operator
	replay.IdempotencyKey = "replay-child-1"
	replay.Reason = "Broker acknowledgement restored"
	replayedID, err := storeA.Replay(ctx, "tenant-a", childAttemptID, replay)
	if err != nil || replayedID != childAttemptID {
		t.Fatalf("Replay id=%q error=%v", replayedID, err)
	}
	duplicateReplayID, err := storeB.Replay(ctx, "tenant-a", childAttemptID, replay)
	if err != nil || duplicateReplayID != childAttemptID {
		t.Fatalf("duplicate Replay id=%q error=%v", duplicateReplayID, err)
	}
	assertCounts(t, db, 1, 1, 2, 2, 2)

	publisher, err := NewKafkaPublisher(KafkaConfig{
		Brokers:         brokers,
		ClientID:        "fi-fhir-delivery-kill-test",
		DialTimeout:     5 * time.Second,
		DeliveryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewKafkaPublisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	dispatcherConfig := DefaultConfig()
	dispatcherConfig.PublishTimeout = 15 * time.Second
	dispatcher, err := NewDispatcher(storeA, publisher, "worker-kafka", dispatcherConfig)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomePublished {
		t.Fatalf("RunOnce replay outcome=%q error=%v", outcome, err)
	}
	assertDeliveryState(t, db, childAttemptID, "succeeded", 1, "published", "inactive")

	seedDelivery(t, db, clock.Now(), "attempt-crash", "outbox-crash")
	crashItem, err := storeA.Claim(ctx, "worker-crash", 2*time.Second)
	if err != nil || crashItem == nil || crashItem.AttemptID != "attempt-crash" {
		t.Fatalf("claim crash item=%#v error=%v", crashItem, err)
	}
	crashMessage, err := messageForWorkItem(*crashItem)
	if err != nil {
		t.Fatalf("messageForWorkItem: %v", err)
	}
	if err := publisher.Publish(ctx, crashMessage); err != nil {
		t.Fatalf("publish before simulated DB crash: %v", err)
	}
	clock.Advance(2 * time.Second)
	if err := storeA.MarkPublished(ctx, *crashItem); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired worker completed stale lease: %v", err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomePublished {
		t.Fatalf("RunOnce reclaimed crash outcome=%q error=%v", outcome, err)
	}

	records := consumeDeliveryRecords(t, ctx, brokers, deliveryCommandSchema, 3)
	keyCounts := make(map[string]int)
	for _, record := range records {
		keyCounts[string(record.Key)]++
		for _, forbidden := range []string{"RAW-INGRESS-SENTINEL", "raw_payload", "secret"} {
			if strings.Contains(string(record.Value), forbidden) {
				t.Fatalf("Kafka value contains %q", forbidden)
			}
		}
	}
	if keyCounts[childAttemptID] != 1 || keyCounts["attempt-crash"] != 2 {
		t.Fatalf("Kafka key counts = %#v", keyCounts)
	}
	assertCounts(t, db, 1, 1, 3, 3, 2)
	assertDatabaseRawFree(t, db)
}

func deliveryPostgresDSN(t *testing.T, ctx context.Context) string {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		container, err := postgrescontainer.Run(ctx, "postgres:16-alpine",
			postgrescontainer.WithDatabase("fi_fhir_delivery"),
			postgrescontainer.WithUsername("testuser"),
			postgrescontainer.WithPassword("testpass"),
			postgrescontainer.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatalf("start PostgreSQL container: %v", err)
		}
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
		base, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("PostgreSQL connection string: %v", err)
		}
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open PostgreSQL admin: %v", err)
	}
	schema := fmt.Sprintf("delivery_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		_ = admin.Close()
		t.Fatalf("create delivery schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		_ = admin.Close()
	})
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func deliveryKafkaBrokers(t *testing.T, ctx context.Context) []string {
	t.Helper()
	if configured := os.Getenv("KAFKA_TEST_BROKERS"); configured != "" {
		return strings.Split(configured, ",")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("KAFKA_TEST_BROKERS is required in CI")
	}
	container, err := kafkacontainer.Run(ctx,
		"confluentinc/confluent-local:7.5.0",
		kafkacontainer.WithClusterID("fi-fhir-delivery-test"),
	)
	if err != nil {
		t.Fatalf("start Kafka container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	brokers, err := container.Brokers(ctx)
	if err != nil {
		t.Fatalf("Kafka brokers: %v", err)
	}
	return brokers
}

func seedDelivery(t *testing.T, db *sql.DB, now time.Time, attemptID, outboxID string) {
	t.Helper()
	destination := integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{
			ArtifactID: "queue-primary", RevisionID: "1",
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Class: integration.DestinationClassProduction,
	}
	destinationJSON, _ := json.Marshal(destination)
	payload := fmt.Sprintf(`{"schema":"%s","tenant_id":"tenant-a","receipt_id":"receipt-a","event_id":"event-a","trace_id":"trace-a","attempt_id":%q,"destination":%s,"route":"admit","action":"send-kafka","attempt_count":1}`,
		deliveryCommandSchema, attemptID, destinationJSON)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed delivery: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, result_json
		) VALUES ('tenant-a', 'receipt-a', 'key-a', 'fingerprint-a', '{}',
			'accepted', $1, 'correlation-a', 'ephemeral', '{}', '{}')
		ON CONFLICT DO NOTHING
	`, now); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', 'event-a', 'receipt-a', 'patient_admit',
			'message-a', 'correlation-a', 'phi',
			'{"id":"event-a","type":"patient_admit","patient":{"mrn":"123"}}', $1)
		ON CONFLICT DO NOTHING
	`, now); err != nil {
		t.Fatalf("seed canonical event: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id,
			destination_revision_json, route_name, action_id, status,
			attempt_count, recorded_at, scheduled_at
		) VALUES ('tenant-a', $2, 'receipt-a', 'event-a', 'trace-a', $3,
			'admit', 'send-kafka', 'queued', 1, $1, $1)
	`, now, attemptID, destinationJSON); err != nil {
		t.Fatalf("seed delivery attempt: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_delivery_outbox (
			tenant_id, outbox_id, attempt_id, topic, status, payload_json,
			created_at, scheduled_at, updated_at
		) VALUES ('tenant-a', $2, $3, $4, 'pending', $5, $1, $1, $1)
	`, now, outboxID, attemptID, deliveryCommandSchema, payload); err != nil {
		t.Fatalf("seed delivery outbox: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed delivery: %v", err)
	}
}

func assertDeliveryState(
	t *testing.T,
	db *sql.DB,
	attemptID string,
	wantAttemptStatus string,
	wantAttemptCount int,
	wantOutboxStatus string,
	wantDLQState string,
) {
	t.Helper()
	var attemptStatus, outboxStatus string
	var attemptCount int
	var active sql.NullBool
	if err := db.QueryRow(`
		SELECT a.status, a.attempt_count, o.status, d.active
		FROM integration_delivery_attempts a
		JOIN integration_delivery_outbox o
		  ON o.tenant_id = a.tenant_id AND o.attempt_id = a.attempt_id
		LEFT JOIN integration_delivery_dlq d
		  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
		WHERE a.tenant_id = 'tenant-a' AND a.attempt_id = $1
	`, attemptID).Scan(&attemptStatus, &attemptCount, &outboxStatus, &active); err != nil {
		t.Fatalf("read delivery state: %v", err)
	}
	if attemptStatus != wantAttemptStatus || attemptCount != wantAttemptCount || outboxStatus != wantOutboxStatus {
		t.Fatalf("delivery state = %s/%d/%s, want %s/%d/%s",
			attemptStatus, attemptCount, outboxStatus,
			wantAttemptStatus, wantAttemptCount, wantOutboxStatus)
	}
	switch wantDLQState {
	case "absent":
		if active.Valid {
			t.Fatalf("DLQ row is present with active=%v, want absent", active.Bool)
		}
	case "active":
		if !active.Valid || !active.Bool {
			t.Fatalf("DLQ active = %#v, want true", active)
		}
	case "inactive":
		if !active.Valid || active.Bool {
			t.Fatalf("DLQ active = %#v, want false", active)
		}
	default:
		t.Fatalf("invalid expected DLQ state %q", wantDLQState)
	}
}

func assertCircuitState(t *testing.T, db *sql.DB, wantState string, wantFailures int) {
	t.Helper()
	var state string
	var failures int
	if err := db.QueryRow(`
		SELECT state, consecutive_failures
		FROM integration_delivery_circuits
		WHERE tenant_id = 'tenant-a'
		  AND destination_artifact_id = 'queue-primary'
		  AND destination_revision_id = '1'
	`).Scan(&state, &failures); err != nil {
		t.Fatalf("read circuit state: %v", err)
	}
	if state != wantState || failures != wantFailures {
		t.Fatalf("circuit state = %s/%d, want %s/%d", state, failures, wantState, wantFailures)
	}
}

func assertCounts(t *testing.T, db *sql.DB, receipts, events, attempts, outbox, operations int) {
	t.Helper()
	wants := map[string]int{
		"integration_receipts":            receipts,
		"integration_canonical_events":    events,
		"integration_delivery_attempts":   attempts,
		"integration_delivery_outbox":     outbox,
		"integration_delivery_operations": operations,
	}
	for table, want := range wants {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
}

func consumeDeliveryRecords(t *testing.T, ctx context.Context, brokers []string, topic string, count int) []*kgo.Record {
	t.Helper()
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("create Kafka consumer: %v", err)
	}
	defer consumer.Close()
	pollCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	records := make([]*kgo.Record, 0, count)
	for len(records) < count {
		fetches := consumer.PollFetches(pollCtx)
		if err := fetches.Err(); err != nil {
			t.Fatalf("consume Kafka records: %v", err)
		}
		fetches.EachRecord(func(record *kgo.Record) {
			if len(records) < count {
				records = append(records, record)
			}
		})
	}
	return records
}

func assertDatabaseRawFree(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`
		SELECT payload_json::text FROM integration_delivery_outbox
		UNION ALL SELECT detail_json::text FROM integration_delivery_audit
		UNION ALL SELECT principal_json::text FROM integration_delivery_operations
	`)
	if err != nil {
		t.Fatalf("read persisted delivery JSON: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan persisted delivery JSON: %v", err)
		}
		for _, forbidden := range []string{"RAW-INGRESS-SENTINEL", "raw_payload", "secret"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("persisted delivery JSON contains %q", forbidden)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate persisted delivery JSON: %v", err)
	}
}

func testKafkaFailure() Failure {
	return Failure{
		Code:      "KAFKA_PUBLISH_FAILED",
		Detail:    "Kafka did not acknowledge the delivery command",
		Retryable: true,
	}
}

type deliveryTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *deliveryTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *deliveryTestClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}
