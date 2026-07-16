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
