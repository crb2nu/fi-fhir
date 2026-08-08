//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestDeliveryDispatch_ContactsNoDestination is the Slice 4.1c-a day-1 gate.
//
// The sprint scope asked "what identity does the engine present to
// destinations?". This test answers it from behavior rather than from the plan
// text: a live TLS endpoint stands at the address a webhook destination would be
// reached on, one complete production submission runs through the durable
// admission path, and the dispatcher publishes it. The endpoint records zero
// accepted TCP connections and zero served requests, while Kafka records exactly
// one produced command.
//
// Three properties are asserted together so the result cannot be an accident:
//
//  1. The destination endpoint is genuinely live — the test dials it itself at
//     the end and both counters move. Without that control, "nothing connected"
//     is indistinguishable from "the listener was broken".
//  2. No durable record and no broker payload carries a destination address at
//     all. The production contract has no field for one: a destination is
//     {artifact_id, revision_id, digest, class} and nothing else.
//  3. A URL cannot even be expressed as a destination. The published DSL
//     restricts destination names to ^[a-z][a-z0-9_.-]*$, so a workflow naming
//     an https:// endpoint fails planning before any durable row exists.
//
// Passing on unmodified main is the point: it proves correction 13 of
// .loom/31-sprint3-execution-specs.md, converts 4.1c from "scope the existing
// credential" into "build the missing contract", and marks the boundary that
// 4.1c-b's HTTPS consumer will cross.
func TestDeliveryDispatch_ContactsNoDestination(t *testing.T) {
	ctx := t.Context()
	endpoint := newDestinationListener(t)
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

	clock := fixedDestinationClock()
	fixture := newDurableSubmissionFixture(
		t, db, clock,
		destinationWorkflowYAML("webhook", "webhook-primary"),
		[]string{"webhook-primary"},
	)

	result, err := fixture.processor.Process(ctx, fixture.request)
	if err != nil {
		t.Fatalf("durable production submission: %v", err)
	}
	if result.Receipt == nil || len(result.Deliveries) != 1 {
		t.Fatalf("production result = %#v", result)
	}
	if result.Deliveries[0].Status != integration.DeliveryStatusQueued {
		t.Fatalf("delivery status = %q, want queued", result.Deliveries[0].Status)
	}
	attemptID := result.Deliveries[0].AttemptID
	if submissionCount(t, db, "integration_delivery_outbox") != 1 {
		t.Fatalf("outbox rows = %d, want 1", submissionCount(t, db, "integration_delivery_outbox"))
	}

	publisher, err := NewKafkaPublisher(KafkaConfig{
		Brokers:         brokers,
		ClientID:        "fi-fhir-destination-contact-gate",
		DialTimeout:     5 * time.Second,
		DeliveryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewKafkaPublisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	createDeliveryTopic(t, ctx, publisher.client, deliveryCommandSchema)

	store, err := NewPostgresStore(db, clock)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	counting := &countingPublisher{Publisher: publisher}
	dispatcherConfig := DefaultConfig()
	dispatcherConfig.PublishTimeout = 15 * time.Second
	dispatcher, err := NewDispatcher(store, counting, "worker-contact-gate", dispatcherConfig)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomePublished {
		t.Fatalf("RunOnce outcome=%q error=%v", outcome, err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomeIdle {
		t.Fatalf("second RunOnce outcome=%q error=%v, want idle", outcome, err)
	}
	if published := counting.count.Load(); published != 1 {
		t.Fatalf("dispatcher published %d messages, want exactly 1", published)
	}

	// Assertion 1: the engine contacted no destination.
	if accepted, requests := endpoint.Accepted(), endpoint.Requests(); accepted != 0 || requests != 0 {
		t.Fatalf("destination endpoint was contacted: accepted=%d requests=%d", accepted, requests)
	}

	// Assertion 2: exactly one Kafka command, on the one constant topic, and it
	// carries no destination address of any kind.
	records := consumeDeliveryRecords(t, ctx, brokers, deliveryCommandSchema, 1)
	if len(records) != 1 {
		t.Fatalf("Kafka records = %d, want 1", len(records))
	}
	if string(records[0].Key) != attemptID {
		t.Fatalf("Kafka key = %q, want attempt %q", records[0].Key, attemptID)
	}
	if records[0].Topic != "integration.delivery.v1" {
		t.Fatalf("Kafka topic = %q, want the constant delivery topic", records[0].Topic)
	}
	assertNoDestinationAddress(t, "kafka value", string(records[0].Value), endpoint)
	for _, header := range records[0].Headers {
		assertNoDestinationAddress(t, "kafka header "+header.Key, string(header.Value), endpoint)
	}
	assertDurableRecordsCarryNoDestinationAddress(t, db, endpoint)

	// Assertion 3: a destination URL is unrepresentable in the production
	// contract, so there is nothing for the engine to have dialed.
	assertDestinationURLIsUnrepresentable(t, db, clock, endpoint.URL())

	// Control: the endpoint really is live, so the zero above is a fact about
	// the engine and not about the listener.
	endpoint.proveReachable(t)
}

// assertDestinationURLIsUnrepresentable proves the published DSL refuses a URL
// as a destination name, which is why no address ever reaches the dispatcher.
// The revision itself accepts the URL as an artifact ID, so the refusal is
// proved where it actually happens: production planning, before any durable row.
func assertDestinationURLIsUnrepresentable(t *testing.T, db *sql.DB, clock func() time.Time, url string) {
	t.Helper()
	before := submissionCount(t, db, "integration_delivery_attempts")
	urlFixture := newDurableSubmissionFixture(
		t, db, clock,
		destinationWorkflowYAML("webhook", url),
		[]string{url},
	)
	_, err := urlFixture.processor.Process(t.Context(), urlFixture.request)
	if !errors.Is(err, processor.ErrWorkflowPlanningFailed) {
		t.Fatalf("production submission naming %q planned with error %v, want ErrWorkflowPlanningFailed", url, err)
	}
	if after := submissionCount(t, db, "integration_delivery_attempts"); after != before {
		t.Fatalf("rejected URL destination wrote durable attempts: %d -> %d", before, after)
	}
}

// assertDurableRecordsCarryNoDestinationAddress scans every durable record class
// written by the submission for a scheme, host, or port.
func assertDurableRecordsCarryNoDestinationAddress(t *testing.T, db *sql.DB, endpoint *destinationListener) {
	t.Helper()
	queries := map[string]string{
		"integration_receipts":          `SELECT result_json::text FROM integration_receipts`,
		"integration_canonical_events":  `SELECT payload_json::text FROM integration_canonical_events`,
		"integration_message_lineage":   `SELECT artifact_revisions_json::text || routes_json::text FROM integration_message_lineage`,
		"integration_delivery_attempts": `SELECT destination_revision_json::text FROM integration_delivery_attempts`,
		"integration_delivery_outbox":   `SELECT payload_json::text FROM integration_delivery_outbox`,
	}
	for table, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			assertNoDestinationAddress(t, table, value, endpoint)
			if strings.Contains(value, rawIngressSentinel) {
				_ = rows.Close()
				t.Fatalf("%s carries the raw source sentinel", table)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate %s: %v", table, err)
		}
		_ = rows.Close()
	}
}

func assertNoDestinationAddress(t *testing.T, where, value string, endpoint *destinationListener) {
	t.Helper()
	for _, forbidden := range []string{
		endpoint.URL(), endpoint.HostPort(), "https://", "http://", "127.0.0.1",
	} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("%s carries destination address fragment %q", where, forbidden)
		}
	}
}

// countingPublisher records how many delivery commands actually reached the
// broker, so "exactly one produce" is asserted at the publisher as well as at
// the topic.
type countingPublisher struct {
	Publisher
	count atomic.Int64
}

func (p *countingPublisher) Publish(ctx context.Context, message Message) error {
	if err := p.Publisher.Publish(ctx, message); err != nil {
		return err
	}
	p.count.Add(1)
	return nil
}
