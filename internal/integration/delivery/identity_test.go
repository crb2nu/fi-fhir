package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestDispatcherDeniesBeforeAnyBrokerContact is the unit-level statement of the
// enforcement point: the decision runs after Claim and before the delivery
// command is built or published, so a denied attempt produces a non-retryable
// dead letter and zero publishes.
func TestDispatcherDeniesBeforeAnyBrokerContact(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item}
	publisher := &fakePublisher{}
	decider := &fakeDecider{err: NewDenial(ForbiddenFailureCode, "destination is not in the deployed integration revision")}
	dispatcher, err := NewDispatcherWithIdentity(store, publisher, "worker-a", testConfig(), decider)
	if err != nil {
		t.Fatalf("NewDispatcherWithIdentity: %v", err)
	}
	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outcome != OutcomeForbidden {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeForbidden)
	}
	if publisher.message.Topic != "" || len(publisher.message.Value) != 0 {
		t.Fatalf("a denied attempt reached the publisher: %#v", publisher.message)
	}
	if store.published != 0 || store.failed != 1 {
		t.Fatalf("published = %d failed = %d", store.published, store.failed)
	}
	if store.failure.Code != ForbiddenFailureCode || store.failure.Retryable {
		t.Fatalf("recorded failure = %#v", store.failure)
	}
	if decider.attemptID != item.AttemptID || decider.tenantID != item.TenantID ||
		decider.destination != item.Destination {
		t.Fatalf("decider saw tenant=%q attempt=%q destination=%#v",
			decider.tenantID, decider.attemptID, decider.destination)
	}
}

func TestDispatcherDeadLettersAnUnverifiedDestination(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item, retry: true}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcherWithIdentity(store, publisher, "worker-a", testConfig(),
		&fakeDecider{err: NewDenial(UnverifiedDestinationFailureCode,
			"destination reference does not match the deployed destination revision")})
	if err != nil {
		t.Fatalf("NewDispatcherWithIdentity: %v", err)
	}
	// The store would schedule a retry for a retryable failure, but a denial is
	// never retryable, so the outcome stays terminal regardless.
	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outcome != OutcomeForbidden || store.failure.Code != UnverifiedDestinationFailureCode ||
		store.failure.Retryable {
		t.Fatalf("outcome = %q failure = %#v", outcome, store.failure)
	}
	if publisher.message.Topic != "" {
		t.Fatal("an unverified destination reached the publisher")
	}
}

// TestDispatcherSurfacesDeciderInfrastructureFailures proves an
// infrastructure error is not silently converted into a dead letter. A database
// outage in the decision path must retry the item, not discard it.
func TestDispatcherSurfacesDeciderInfrastructureFailures(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcherWithIdentity(store, publisher, "worker-a", testConfig(),
		&fakeDecider{err: errors.New("provenance database is unreachable")})
	if err != nil {
		t.Fatalf("NewDispatcherWithIdentity: %v", err)
	}
	if _, err := dispatcher.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce swallowed a decider infrastructure failure")
	}
	if store.failed != 0 || store.published != 0 {
		t.Fatalf("infrastructure failure mutated durable state: failed=%d published=%d",
			store.failed, store.published)
	}
	if publisher.message.Topic != "" {
		t.Fatal("an unauthorized attempt reached the publisher")
	}
}

// TestDispatcherWithoutADeciderIsUnchanged pins the compatibility property that
// every pre-4.1c-a deployment and the Slice 2.3 proof depend on.
func TestDispatcherWithoutADeciderIsUnchanged(t *testing.T) {
	item := testWorkItem()
	store := &fakeStore{item: &item}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcher(store, publisher, "worker-a", testConfig())
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil || outcome != OutcomePublished || store.published != 1 || store.failed != 0 {
		t.Fatalf("outcome = %q error = %v published = %d failed = %d",
			outcome, err, store.published, store.failed)
	}
}

// TestRefusalFailureIsBoundedAndCatalogSafe proves an out-of-range refusal from
// any decider still produces a writable, non-retryable failure. A refusal whose
// code or detail the store would reject must not wedge the worker.
func TestRefusalFailureIsBoundedAndCatalogSafe(t *testing.T) {
	t.Parallel()

	refusals := map[string]Refusal{
		"oversized": NewDenial(strings.Repeat("C", 200), strings.Repeat("d", 600)),
		"blank":     NewDenial("", "   "),
		"control":   NewDenial("BAD\nCODE", "detail\x00"),
	}
	for name, refusal := range refusals {
		failure := refusalFailure(refusal)
		if failure.Retryable || !validFailure(failure) {
			t.Fatalf("%s refusal failure = %#v", name, failure)
		}
	}
	var nilDenial *DenialError
	if nilDenial.Error() != ErrDeliveryForbidden.Error() {
		t.Fatalf("nil denial error = %q", nilDenial.Error())
	}
	if failure := refusalFailure(nilDenial); failure.Retryable || !validFailure(failure) {
		t.Fatalf("nil denial failure = %#v", failure)
	}
	if failure := refusalFailure(nil); failure.Retryable || failure.Code != ForbiddenFailureCode {
		t.Fatalf("nil refusal failure = %#v", failure)
	}
	if !errors.Is(NewDenial(ForbiddenFailureCode, "denied"), ErrDeliveryForbidden) {
		t.Fatal("denials do not unwrap to the catalog-safe kind")
	}
}

type fakeDecider struct {
	err         error
	tenantID    string
	attemptID   string
	destination integration.DestinationRevisionRef
}

func (d *fakeDecider) Decide(
	_ context.Context,
	tenantID string,
	attemptID string,
	destination integration.DestinationRevisionRef,
) error {
	d.tenantID = tenantID
	d.attemptID = attemptID
	d.destination = destination
	return d.err
}
