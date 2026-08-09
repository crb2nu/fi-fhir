//go:build integration

package delivery

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// httpsGateTokenMaterial is the credential the gate's destination binds. It is
// planted so the gate can prove the deployment is complete — a "nothing was
// contacted" result means nothing if the destination was unusable.
const httpsGateTokenMaterial = "HTTPS-GATE-TOKEN-MATERIAL-4f21"

// TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday is the Slice
// 4.1c-b day-1 gate. It must pass on unmodified `main`.
//
// It stands a live TLS endpoint at the exact address an `https`-transport
// destination declares, deploys that destination in a strict registry with a
// resolvable credential binding, runs one complete production submission through
// durable admission, and dispatches it once. Then:
//
//  1. The TLS endpoint records zero accepted TCP connections and zero served
//     requests. The `Transport` field on a destination revision routes nothing.
//  2. Kafka records exactly one command for the attempt, on the one constant
//     delivery topic, exactly as a `kafka`-class destination would.
//  3. `integration_delivery_identity_decisions` records `authorized` for the
//     attempt, under the destination's own declared subject, against the digest
//     this process verified — and carries the destination's URL in
//     `destination_endpoint_advisory` and nowhere else.
//
// Together those kill the assumption that "4.1c-a resolves the destination
// revision on the dispatch path, so 4.1c-b is wiring a transport onto an
// existing resolution". The resolution happens, is fully authorized, is
// provenance-recorded with the address — and is then discarded in favour of the
// broker. There is no transport to wire onto; 4.1c-b has to build the routing
// seam, the dispatch-time credential lifetime, and the client.
//
// Two controls keep the zeros honest: the endpoint is dialed by the test itself
// at the end and both counters move, and the planted credential is read back
// from disk so a resolvable binding is proven rather than assumed.
//
// After 4.1c-b lands this test becomes the negative control's companion: it is
// the recorded behaviour the slice deliberately changes for `https`-class
// destinations, and the behaviour that must stay true for `kafka`-class ones
// (TestDeliveryDispatch_ContactsNoDestination).
func TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday(t *testing.T) {
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
	secretRoot := t.TempDir()
	writeIdentitySecret(t, secretRoot, "gate/token", httpsGateTokenMaterial)

	// The destination declares transport: https and points at the live endpoint.
	revision := mustDestinationRevision(
		t, "dest-https-gate", "https-gate", "gate-client", "gate-token", endpoint.URL(),
	)
	if revision.Transport != destination.TransportHTTPS || revision.HTTPS == nil ||
		revision.HTTPS.URL != endpoint.URL() {
		t.Fatalf("gate destination is not an https destination at the live endpoint: %#v", revision)
	}

	fixture := newDurableSubmissionFixtureWithDestinations(
		t, db, clock,
		destinationWorkflowYAMLFor("webhook", "dest-https-gate"),
		[]integration.DestinationRevisionRef{revision.Reference()},
	)
	result, err := fixture.processor.Process(ctx, fixture.request)
	if err != nil {
		t.Fatalf("durable production submission: %v", err)
	}
	if result.Receipt == nil || len(result.Deliveries) != 1 ||
		result.Deliveries[0].Status != integration.DeliveryStatusQueued {
		t.Fatalf("production result = %#v", result)
	}
	attemptID := result.Deliveries[0].AttemptID

	registry := mustRegistry(t, transportRegistryJSON(
		t, map[string]string{"gate-token": "gate/token"}, revision,
	), destination.ModeStrict)
	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("Migrate provenance: %v", err)
	}
	authorizer, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance, Clock: clock,
	})
	if err != nil {
		t.Fatalf("NewAuthorizer: %v", err)
	}

	publisher, err := NewKafkaPublisher(KafkaConfig{
		Brokers:         brokers,
		ClientID:        "fi-fhir-destination-transport-gate",
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
	dispatcher, err := NewDispatcherWithIdentity(
		store, counting, "worker-transport-gate", dispatcherConfig, authorizer,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithIdentity: %v", err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomePublished {
		t.Fatalf("RunOnce outcome=%q error=%v, want published", outcome, err)
	}
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomeIdle {
		t.Fatalf("second RunOnce outcome=%q error=%v, want idle", outcome, err)
	}

	// Assertion 1: the declared https transport executed nothing.
	if accepted, requests := endpoint.Accepted(), endpoint.Requests(); accepted != 0 || requests != 0 {
		t.Fatalf("https destination was contacted: accepted=%d requests=%d", accepted, requests)
	}

	// Assertion 2: the broker carried it instead, exactly once.
	if published := counting.count.Load(); published != 1 {
		t.Fatalf("dispatcher published %d messages, want exactly 1", published)
	}
	records := recordsByKey(drainDeliveryRecords(t, ctx, brokers, deliveryCommandSchema))[attemptID]
	if len(records) != 1 {
		t.Fatalf("Kafka records for attempt %s = %d, want exactly 1", attemptID, len(records))
	}
	if records[0].Topic != "integration.delivery.v1" {
		t.Fatalf("Kafka topic = %q, want the constant delivery topic", records[0].Topic)
	}

	// Assertion 3: the destination was fully resolved, verified, authorized, and
	// provenance-recorded with its address — and then not used.
	decision := readIdentityDecision(t, db, attemptID)
	if !decision.authorized || decision.subject != "gate-client" ||
		decision.digest != revision.Digest || decision.denialCode != "" {
		t.Fatalf("gate decision = %#v", decision)
	}
	if decision.endpointAdvisory != endpoint.URL() {
		t.Fatalf("advisory endpoint = %q, want %q", decision.endpointAdvisory, endpoint.URL())
	}

	// Assertion 4: the advisory column is the only place the address lives.
	assertNoDestinationAddress(t, "kafka value", string(records[0].Value), endpoint)
	for _, header := range records[0].Headers {
		assertNoDestinationAddress(t, "kafka header "+header.Key, string(header.Value), endpoint)
	}
	assertDurableRecordsCarryNoDestinationAddress(t, db, endpoint)

	// Control A: the credential the destination binds really does resolve, so the
	// zero above is not "the deployment was broken".
	planted, err := os.ReadFile(filepath.Join(secretRoot, "gate", "token"))
	if err != nil || string(planted) != httpsGateTokenMaterial {
		t.Fatalf("gate credential was never planted, so zero-contact proves nothing: %q %v", planted, err)
	}
	// Control B: the endpoint really is live.
	endpoint.proveReachable(t)
}

// transportRegistryJSON builds a destination registry document over an explicit
// binding set, so a proof can declare exactly the credentials its destinations
// name instead of inheriting 4.1c-a's fixed alpha/beta pair.
func transportRegistryJSON(
	t *testing.T,
	bindings map[string]string,
	revisions ...destination.Revision,
) string {
	t.Helper()
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	declared := make([]map[string]any, 0, len(names))
	for _, name := range names {
		declared = append(declared, map[string]any{
			"name":      name,
			"reference": map[string]string{"provider": "file", "key": bindings[name]},
		})
	}
	document := map[string]any{
		"schema":    "fi-fhir/destination-registry/v1",
		"tenant_id": "tenant-a",
		"integration_revision": map[string]string{
			"artifact_id": "integration-adt", "revision_id": "revision-1",
			"digest": "sha256:" + strings.Repeat("b", 64),
		},
		"secret_bindings": declared,
		"destinations":    revisions,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	return string(encoded)
}
