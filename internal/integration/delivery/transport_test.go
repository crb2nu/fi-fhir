package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// stubTransportError is a minimal structural implementation of
// TransportFailure, standing in for the destination package's TransportError so
// this package's tests do not import it.
type stubTransportError struct {
	code      string
	detail    string
	retryable bool
}

func (s stubTransportError) Error() string                  { return s.detail }
func (s stubTransportError) DeliveryFailureCode() string    { return s.code }
func (s stubTransportError) DeliveryFailureDetail() string  { return s.detail }
func (s stubTransportError) DeliveryFailureRetryable() bool { return s.retryable }

func TestTransportFailureClassifiesBoundedOutcomes(t *testing.T) {
	tests := map[string]struct {
		err           error
		wantClassify  bool
		wantCode      string
		wantDetail    string
		wantRetryable bool
	}{
		"retryable transport failure": {
			err:           stubTransportError{code: "DEST_UNAVAILABLE", detail: "retry me", retryable: true},
			wantClassify:  true,
			wantCode:      "DEST_UNAVAILABLE",
			wantDetail:    "retry me",
			wantRetryable: true,
		},
		"terminal transport failure": {
			err:          stubTransportError{code: "DEST_REJECTED", detail: "no", retryable: false},
			wantClassify: true,
			wantCode:     "DEST_REJECTED",
			wantDetail:   "no",
		},
		"a refusal stays terminal": {
			err:          NewDenial(ForbiddenFailureCode, "not authorized"),
			wantClassify: true,
			wantCode:     ForbiddenFailureCode,
			wantDetail:   "not authorized",
		},
		"an oversized detail collapses rather than becoming unwritable": {
			err: stubTransportError{
				code: "DEST_REJECTED", detail: strings.Repeat("x", 513), retryable: true,
			},
			wantClassify:  true,
			wantCode:      "DEST_REJECTED",
			wantDetail:    defaultTransportDetail,
			wantRetryable: true,
		},
		"a code carrying destination content collapses to the catalog code": {
			err: stubTransportError{
				code: "server said: <html>\n", detail: "bad", retryable: false,
			},
			wantClassify: true,
			wantCode:     TransportFailureCode,
			wantDetail:   "bad",
		},
		"an empty detail collapses to the catalog detail": {
			err:          stubTransportError{code: "DEST_REJECTED", detail: "   "},
			wantClassify: true,
			wantCode:     "DEST_REJECTED",
			wantDetail:   defaultTransportDetail,
		},
		"an infrastructure error is never dead-lettered": {
			err:          errors.New("connection reset while recording provenance"),
			wantClassify: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			failure, classified := transportFailure(test.err)
			if classified != test.wantClassify {
				t.Fatalf("classified = %v, want %v", classified, test.wantClassify)
			}
			if !classified {
				return
			}
			if failure.Code != test.wantCode || failure.Detail != test.wantDetail ||
				failure.Retryable != test.wantRetryable {
				t.Fatalf("failure = %#v, want %s/%s retryable=%v",
					failure, test.wantCode, test.wantDetail, test.wantRetryable)
			}
		})
	}
}

// routerTransport records what it was asked about and answers a fixed script.
type routerTransport struct {
	owns      bool
	err       error
	calls     int
	payload   []byte
	attemptID string
}

func (r *routerTransport) DeliverDestination(
	_ context.Context, _ string, attemptID string,
	_ integration.DestinationRevisionRef, payload []byte,
) (bool, error) {
	r.calls++
	r.attemptID = attemptID
	r.payload = append([]byte(nil), payload...)
	return r.owns, r.err
}

func TestDispatcherRoutesBetweenTransportAndBroker(t *testing.T) {
	tests := map[string]struct {
		transport     *routerTransport
		wantOutcome   Outcome
		wantErr       bool
		wantPublishes int
		wantPublished int
		wantFailed    int
	}{
		"no transport configured publishes to the broker": {
			transport:     nil,
			wantOutcome:   OutcomePublished,
			wantPublishes: 1,
			wantPublished: 1,
		},
		"a destination the transport does not own publishes to the broker": {
			transport:     &routerTransport{owns: false},
			wantOutcome:   OutcomePublished,
			wantPublishes: 1,
			wantPublished: 1,
		},
		"an owned destination completes through MarkPublished and never publishes": {
			transport:     &routerTransport{owns: true},
			wantOutcome:   OutcomePublished,
			wantPublishes: 0,
			wantPublished: 1,
		},
		"a retryable transport failure schedules a retry and never publishes": {
			transport: &routerTransport{owns: true, err: stubTransportError{
				code: "DEST_UNAVAILABLE", detail: "retry me", retryable: true,
			}},
			wantOutcome: OutcomeRetry,
			wantFailed:  1,
		},
		"a terminal transport failure dead-letters and never publishes": {
			transport: &routerTransport{owns: true, err: stubTransportError{
				code: "DEST_REJECTED", detail: "no", retryable: false,
			}},
			wantOutcome: OutcomeDLQ,
			wantFailed:  1,
		},
		"an infrastructure error surfaces instead of dead-lettering": {
			transport:   &routerTransport{owns: true, err: errors.New("provenance outage")},
			wantErr:     true,
			wantFailed:  0,
			wantOutcome: "",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := &recordingDispatchStore{item: dispatchTestItem(), retry: true}
			publisher := &recordingDispatchPublisher{}
			var transport DestinationTransport
			if test.transport != nil {
				transport = test.transport
			}
			dispatcher, err := NewDispatcherWithDestination(
				store, publisher, "worker-router", DefaultConfig(), nil, transport,
			)
			if err != nil {
				t.Fatalf("NewDispatcherWithDestination: %v", err)
			}
			outcome, err := dispatcher.RunOnce(context.Background())
			if test.wantErr {
				if err == nil {
					t.Fatal("RunOnce returned no error for an infrastructure failure")
				}
			} else if err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			if outcome != test.wantOutcome {
				t.Fatalf("outcome = %q, want %q", outcome, test.wantOutcome)
			}
			if publisher.published != test.wantPublishes {
				t.Fatalf("broker publishes = %d, want %d", publisher.published, test.wantPublishes)
			}
			if store.publishedCount != test.wantPublished || store.failedCount != test.wantFailed {
				t.Fatalf("store MarkPublished=%d MarkFailed=%d, want %d/%d",
					store.publishedCount, store.failedCount, test.wantPublished, test.wantFailed)
			}
			if test.transport != nil && test.transport.calls != 1 {
				t.Fatalf("transport consulted %d times, want exactly 1", test.transport.calls)
			}
		})
	}
}

// TestDispatcherHandsTheTransportTheBrokerNeutralCommand proves the bytes the
// transport receives are the same delivery command the broker would have
// carried: the HTTPS transport substitutes for the broker rather than inventing
// a second wire contract.
func TestDispatcherHandsTheTransportTheBrokerNeutralCommand(t *testing.T) {
	item := dispatchTestItem()
	expected, err := messageForWorkItem(item)
	if err != nil {
		t.Fatalf("messageForWorkItem: %v", err)
	}
	transport := &routerTransport{owns: true}
	dispatcher, err := NewDispatcherWithDestination(
		&recordingDispatchStore{item: item}, &recordingDispatchPublisher{},
		"worker-payload", DefaultConfig(), nil, transport,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithDestination: %v", err)
	}
	if _, err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if string(transport.payload) != string(expected.Value) {
		t.Fatalf("transport payload = %s, want the delivery command %s",
			transport.payload, expected.Value)
	}
	if transport.attemptID != item.AttemptID {
		t.Fatalf("transport attempt = %q, want %q", transport.attemptID, item.AttemptID)
	}
}

type recordingDispatchStore struct {
	item           WorkItem
	claimed        bool
	retry          bool
	publishedCount int
	failedCount    int
}

func (s *recordingDispatchStore) Claim(context.Context, string, time.Duration) (*WorkItem, error) {
	if s.claimed {
		return nil, nil
	}
	s.claimed = true
	item := s.item
	return &item, nil
}

func (s *recordingDispatchStore) MarkPublished(context.Context, WorkItem) error {
	s.publishedCount++
	return nil
}

func (s *recordingDispatchStore) MarkFailed(
	_ context.Context, _ WorkItem, failure Failure, _ Config,
) (bool, error) {
	s.failedCount++
	return s.retry && failure.Retryable, nil
}

type recordingDispatchPublisher struct {
	published int
}

func (p *recordingDispatchPublisher) Publish(context.Context, Message) error {
	p.published++
	return nil
}

func (p *recordingDispatchPublisher) Close() error { return nil }

func dispatchTestItem() WorkItem {
	return WorkItem{
		TenantID:     "tenant-a",
		OutboxID:     "outbox-router",
		AttemptID:    "attempt-router",
		ReceiptID:    "receipt-router",
		EventID:      "event-router",
		TraceID:      "trace-router",
		Topic:        deliveryCommandSchema,
		Route:        "matched",
		Action:       "send",
		AttemptCount: 1,
		Destination: integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "dest-https-alpha", RevisionID: "destination-1",
				Digest: "sha256:" + strings.Repeat("a", 64),
			},
			Class: integration.DestinationClassProduction,
		},
		EventPayload:   []byte(`{"id":"event-router","type":"patient_admit"}`),
		LeaseOwner:     "worker-router",
		LeaseExpiresAt: time.Now().Add(time.Minute),
	}
}
