package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestDispatcherPublishesStableRawFreeCommand(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcher(store, publisher, "worker-a", testConfig())
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outcome != OutcomePublished || store.published != 1 || store.failed != 0 {
		t.Fatalf("outcome = %q, published = %d, failed = %d", outcome, store.published, store.failed)
	}
	if got := string(publisher.message.Key); got != item.AttemptID {
		t.Fatalf("message key = %q, want %q", got, item.AttemptID)
	}
	if got := publisher.message.Headers["fi-fhir-attempt-id"]; got != item.AttemptID {
		t.Fatalf("attempt header = %q", got)
	}
	var command map[string]any
	if err := json.Unmarshal(publisher.message.Value, &command); err != nil {
		t.Fatalf("decode command: %v", err)
	}
	if command["schema"] != deliveryCommandSchema || command["attempt_id"] != item.AttemptID {
		t.Fatalf("unexpected command: %#v", command)
	}
	for _, forbidden := range []string{"raw_payload", "RAW-INGRESS-SENTINEL", "secret"} {
		if strings.Contains(string(publisher.message.Value), forbidden) {
			t.Fatalf("delivery command contains %q", forbidden)
		}
	}
}

func TestDispatcherBoundsPublishFailureWithoutPersistingError(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item, retry: true}
	publisher := &fakePublisher{err: errors.New("broker error RAW-INGRESS-SENTINEL secret")}
	dispatcher, err := NewDispatcher(store, publisher, "worker-a", testConfig())
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outcome != OutcomeRetry || store.failed != 1 || store.failure.Code != "KAFKA_PUBLISH_FAILED" {
		t.Fatalf("outcome = %q, failure = %#v", outcome, store.failure)
	}
	for _, forbidden := range []string{"RAW-INGRESS-SENTINEL", "secret", "broker error"} {
		if strings.Contains(store.failure.Detail, forbidden) {
			t.Fatalf("persisted failure contains %q", forbidden)
		}
	}
}

// TestDispatcherExitsWhenTheTransportReturnsAnUnclassifiedError is the second
// half of found defect D2, and the written-down answer to MR 0b's task 3.
//
// The destination package's TestTransportRecordsProvenanceWhenTheDestinationIsSlow
// proves the transport used to return a raw context.DeadlineExceeded when a slow
// destination starved the provenance write. This proves what that costs here:
// transportFailure() cannot classify an error that is neither a TransportFailure
// nor a Refusal, so RunOnce returns it with the attempt neither published nor
// failed, Run propagates it, and the delivery worker component stops. The
// attempt stays leased until the lease expires and is then redelivered to a
// destination that already accepted it.
//
// This behaviour is deliberate and is kept: a governance ledger that cannot be
// written to should stop the worker rather than let deliveries proceed
// unrecorded. It was simply never written down, and it was being triggered by a
// self-inflicted timeout rather than by a genuine provenance outage. MR 0b
// removes the self-inflicted trigger and leaves the escalation intact, so this
// test is green before and after — a characterization, not a regression gate.
func TestDispatcherExitsWhenTheTransportReturnsAnUnclassifiedError(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item}
	provenanceOutage := errors.New("record destination delivery: ledger unavailable")
	dispatcher, err := NewDispatcherWithDestination(
		store, &fakePublisher{}, "worker-a", testConfig(), nil,
		&stubDestinationTransport{owned: true, err: provenanceOutage},
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithDestination: %v", err)
	}

	outcome, runErr := dispatcher.RunOnce(context.Background())

	if !errors.Is(runErr, provenanceOutage) {
		t.Fatalf("RunOnce error = %v, want the unclassified transport error surfaced raw", runErr)
	}
	if outcome != "" {
		t.Fatalf("outcome = %q, want empty: nothing was decided about this attempt", outcome)
	}
	if store.published != 0 || store.failed != 0 {
		t.Fatalf("published = %d, failed = %d, want 0 and 0: the attempt stays leased, "+
			"which is what makes the lease expiry a redelivery", store.published, store.failed)
	}
	if _, classified := transportFailure(provenanceOutage); classified {
		t.Fatal("a provenance outage was classified as a delivery failure and would dead-letter " +
			"an attempt whose outcome was never written down")
	}
}

func TestKafkaConfigRequiresTLSForCredentials(t *testing.T) {
	_, err := NewKafkaPublisher(KafkaConfig{
		Brokers:  []string{"localhost:9092"},
		Username: "user",
		Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "require TLS") {
		t.Fatalf("NewKafkaPublisher error = %v", err)
	}
}

func TestConfigRequiresPublishTimeoutInsideLease(t *testing.T) {
	config := DefaultConfig()
	config.PublishTimeout = config.LeaseDuration
	if err := config.validate(); err == nil {
		t.Fatal("publish timeout equal to lease accepted")
	}
}

func TestOperationValidationRequiresRoleReasonAndIdempotency(t *testing.T) {
	valid := Operation{
		IdempotencyKey: "recover-1",
		Principal: integration.Principal{
			ID: "operator-a", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{OperatorRole},
		},
		Reason: "Destination repaired",
	}
	if !validOperation(valid) {
		t.Fatal("valid operation rejected")
	}
	withoutRole := valid
	withoutRole.Principal.Roles = nil
	if validOperation(withoutRole) {
		t.Fatal("operation without role accepted")
	}
	withoutReason := valid
	withoutReason.Reason = ""
	if validOperation(withoutReason) {
		t.Fatal("operation without reason accepted")
	}
}

type fakeStore struct {
	item      *WorkItem
	published int
	failed    int
	failure   Failure
	retry     bool
}

func (s *fakeStore) Claim(context.Context, string, time.Duration) (*WorkItem, error) {
	item := s.item
	s.item = nil
	return item, nil
}

func (s *fakeStore) MarkPublished(context.Context, WorkItem) error {
	s.published++
	return nil
}

func (s *fakeStore) MarkFailed(_ context.Context, _ WorkItem, failure Failure, _ Config) (bool, error) {
	s.failed++
	s.failure = failure
	return s.retry, nil
}

type fakePublisher struct {
	message Message
	err     error
}

func (p *fakePublisher) Publish(_ context.Context, message Message) error {
	p.message = message
	return p.err
}

func (p *fakePublisher) Close() error { return nil }

// stubDestinationTransport reports a fixed (owned, error) pair, so a test can
// hand the dispatcher an error shape without standing up a TLS destination.
type stubDestinationTransport struct {
	owned bool
	err   error
}

func (s *stubDestinationTransport) DeliverDestination(
	context.Context, string, string, integration.DestinationRevisionRef, []byte,
) (bool, error) {
	return s.owned, s.err
}

func testWorkItem() WorkItem {
	return WorkItem{
		TenantID:     "tenant-a",
		OutboxID:     "outbox-a",
		AttemptID:    "attempt-a",
		ReceiptID:    "receipt-a",
		EventID:      "event-a",
		TraceID:      "trace-a",
		Topic:        "integration.delivery.v1",
		Route:        "admit",
		Action:       "send-kafka",
		AttemptCount: 1,
		Destination: integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "queue-primary", RevisionID: "1",
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			Class: integration.DestinationClassProduction,
		},
		EventPayload: json.RawMessage(`{"id":"event-a","type":"patient_admit"}`),
		LeaseOwner:   "worker-a",
	}
}

func testConfig() Config {
	config := DefaultConfig()
	config.PublishTimeout = time.Second
	return config
}
