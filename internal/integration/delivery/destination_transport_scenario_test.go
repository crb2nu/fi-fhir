//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Sentinels planted in the two identity-bound credentials. Each must reach its
// own destination's Authorization header, the other's never, and neither may
// appear in any durable record, any broker field, or captured process output.
const (
	httpsAlphaSecretSentinel = "DESTINATION-A-SECRET-SENTINEL-7b12"
	httpsBetaSecretSentinel  = "DESTINATION-B-SECRET-SENTINEL-3c94"
)

// The named assertions of the primary proof. Naming them is what lets the
// negative control assert that a specific set FAILS rather than that "the test"
// fails, which would be satisfied by a compile error.
const (
	assertScopedIdentity = "alpha and beta each contacted exactly once under their own identity"
	assertNoRedelivery   = "a drained queue redelivers nothing"
	assertFlakyCircuit   = "a 503 retries under the existing backoff and circuit, then recovers"
	assertTerminalStatus = "403 and 302 dead-letter non-retryably and no redirect is followed"
	assertKafkaClass     = "a kafka-class destination still publishes and contacts nothing"
	assertNoLeak         = "no secret and no destination address escapes into any durable class"
)

// TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity is the
// Slice 4.1c-b kill-test.
//
// One tenant, one integration revision, one strict registry, six destinations:
//
//   - dest-https-alpha  — identity A, its own TLS server, 200
//   - dest-https-beta   — identity B, a second TLS server, 200
//   - dest-https-flaky  — 503 then 200
//   - dest-https-denied — 403
//   - dest-https-moved  — 302 pointing at a listener that must stay untouched
//   - dest-kafka-legacy — `kafka` transport
//
// One production submission through durable admission seeds one attempt per
// destination, and the dispatcher runs them all. Six named assertions then hold.
//
// Negative control: the same scenario runs a second time against a router that
// unconditionally reports "not mine". Assertions 1-4 must FAIL there — nothing
// is contacted, no circuit opens, nothing dead-letters — while the kafka-class
// assertion still PASSES. A pipeline where the control passes means the router
// is not on the dispatch path, and this test turns red rather than green.
func TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity(t *testing.T) {
	ctx := t.Context()

	results := runDestinationTransportScenario(t, ctx, false)
	for _, assertion := range []string{
		assertScopedIdentity, assertNoRedelivery, assertFlakyCircuit,
		assertTerminalStatus, assertKafkaClass, assertNoLeak,
	} {
		if reason, present := results[assertion]; !present {
			t.Fatalf("assertion %q was never evaluated", assertion)
		} else if reason != nil {
			t.Errorf("assertion %q failed: %v", assertion, reason)
		}
	}
	if t.Failed() {
		return
	}

	control := runDestinationTransportScenario(t, ctx, true)
	for _, assertion := range []string{
		assertScopedIdentity, assertNoRedelivery, assertFlakyCircuit, assertTerminalStatus,
	} {
		reason, present := control[assertion]
		if !present {
			t.Fatalf("negative control: assertion %q was never evaluated", assertion)
		}
		if reason == nil {
			t.Errorf("negative control: assertion %q PASSED under a router that owns nothing; "+
				"the transport is not on the dispatch path and the proof above is vacuous", assertion)
			continue
		}
		t.Logf("negative control: %s failed as required — %v", assertion, reason)
	}
	if reason := control[assertKafkaClass]; reason != nil {
		t.Errorf("negative control: assertion %q must still pass, got %v", assertKafkaClass, reason)
	}
}

// runDestinationTransportScenario runs the whole proof once and reports each
// named assertion's outcome. A nil value means the assertion held.
//
// ownsNothing forces the router to report "not mine" for every destination,
// leaving the identity decision, the durable store, the broker, and all six TLS
// endpoints exactly as they are. That is the negative control: the only thing
// that changes is the one answer the router gives.
func runDestinationTransportScenario(
	t *testing.T,
	ctx context.Context,
	ownsNothing bool,
) map[string]error {
	t.Helper()
	results := make(map[string]error, 6)

	alpha := newRecordingDestination(t, http.StatusOK)
	beta := newRecordingDestination(t, http.StatusOK)
	flaky := newRecordingDestination(t, http.StatusServiceUnavailable, http.StatusOK)
	denied := newRecordingDestination(t, http.StatusForbidden)
	redirectTarget := newDestinationListener(t)
	moved := newRecordingDestination(t, http.StatusFound)
	moved.redirectTo = redirectTarget.URL() + "/moved"

	secretRoot := t.TempDir()
	writeIdentitySecret(t, secretRoot, "alpha/token", httpsAlphaSecretSentinel)
	writeIdentitySecret(t, secretRoot, "beta/token", httpsBetaSecretSentinel)
	writeIdentitySecret(t, secretRoot, "flaky/token", "flaky-token-material")
	writeIdentitySecret(t, secretRoot, "denied/token", "denied-token-material")
	writeIdentitySecret(t, secretRoot, "moved/token", "moved-token-material")
	bindings := map[string]string{
		"alpha-token": "alpha/token", "beta-token": "beta/token",
		"flaky-token": "flaky/token", "denied-token": "denied/token",
		"moved-token": "moved/token",
	}
	for name, endpoint := range map[string]*recordingDestination{
		"alpha": alpha, "beta": beta, "flaky": flaky, "denied": denied, "moved": moved,
	} {
		writeIdentitySecret(t, secretRoot, name+"/ca.pem", endpoint.CertificatePEM())
		bindings[name+"-ca"] = name + "/ca.pem"
	}

	revisions := []destination.Revision{
		mustTLSDestinationRevision(t, "dest-https-alpha", "alpha-client", "alpha-token", "alpha-ca", alpha.URL()),
		mustTLSDestinationRevision(t, "dest-https-beta", "beta-client", "beta-token", "beta-ca", beta.URL()),
		mustTLSDestinationRevision(t, "dest-https-flaky", "flaky-client", "flaky-token", "flaky-ca", flaky.URL()),
		mustTLSDestinationRevision(t, "dest-https-denied", "denied-client", "denied-token", "denied-ca", denied.URL()),
		mustTLSDestinationRevision(t, "dest-https-moved", "moved-client", "moved-token", "moved-ca", moved.URL()),
		mustKafkaDestinationRevision(t, "dest-kafka-legacy", "legacy-client"),
	}
	references := make([]integration.DestinationRevisionRef, 0, len(revisions))
	artifactIDs := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		references = append(references, revision.Reference())
		artifactIDs = append(artifactIDs, revision.ArtifactID)
	}

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

	clock := &deliveryTestClock{now: time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)}
	fixture := newDurableSubmissionFixtureWithDestinations(
		t, db, clock.Now,
		destinationWorkflowYAMLFor("webhook", artifactIDs...),
		references,
	)
	result, err := fixture.processor.Process(ctx, fixture.request)
	if err != nil {
		t.Fatalf("durable production submission: %v", err)
	}
	if len(result.Deliveries) != len(revisions) {
		t.Fatalf("production planned %d deliveries, want %d", len(result.Deliveries), len(revisions))
	}
	attemptByArtifact := make(map[string]string, len(result.Deliveries))
	for _, delivery := range result.Deliveries {
		attemptByArtifact[delivery.Destination.ArtifactID] = delivery.AttemptID
	}

	registry := mustRegistry(t, transportRegistryJSON(t, bindings, revisions...), destination.ModeStrict)
	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("Migrate provenance: %v", err)
	}
	authorizer, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance, Clock: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewAuthorizer: %v", err)
	}
	built, err := destination.NewTransport(destination.TransportConfig{
		Registry: registry,
		Resolver: newFileSecretResolver(secretRoot),
		Recorder: provenance,
		Clock:    clock.Now,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	var transport DestinationTransport = built
	if ownsNothing {
		transport = ownsNothingTransport{}
	}

	publisher, err := NewKafkaPublisher(KafkaConfig{
		Brokers:         brokers,
		ClientID:        "fi-fhir-destination-transport-kill-test",
		DialTimeout:     5 * time.Second,
		DeliveryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewKafkaPublisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	createDeliveryTopic(t, ctx, publisher.client, deliveryCommandSchema)

	store, err := NewPostgresStore(db, clock.Now)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	counting := &countingPublisher{Publisher: publisher}
	dispatcherConfig := DefaultConfig()
	dispatcherConfig.PublishTimeout = 15 * time.Second
	dispatcherConfig.LeaseDuration = 60 * time.Second
	dispatcher, err := NewDispatcherWithDestination(
		store, counting, "worker-transport", dispatcherConfig, authorizer, transport,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithDestination: %v", err)
	}

	// The whole dispatch phase runs with process output captured, so assertion 6
	// can prove no credential reached stdout or stderr.
	output := captureProcessOutput(t, func() {
		drainDispatcher(t, ctx, dispatcher, len(revisions))
		// The flaky destination's retry is scheduled one backoff into the future,
		// so it is not due until the clock moves.
		clock.Advance(2 * dispatcherConfig.RetryBaseDelay)
		drainDispatcher(t, ctx, dispatcher, 1)
	})

	results[assertScopedIdentity] = assertTransportScopedIdentity(alpha, beta, flaky)
	results[assertNoRedelivery] = assertTransportNoRedelivery(ctx, dispatcher, alpha, beta, flaky)
	results[assertFlakyCircuit] = assertTransportFlakyCircuit(
		db, attemptByArtifact["dest-https-flaky"], flaky,
	)
	results[assertTerminalStatus] = assertTransportTerminalStatus(
		db, attemptByArtifact, denied, moved, redirectTarget,
	)
	results[assertKafkaClass] = assertTransportKafkaClass(
		t, ctx, brokers, attemptByArtifact["dest-kafka-legacy"],
		[]*recordingDestination{alpha, beta, flaky, denied, moved},
		counting, ownsNothing,
	)
	results[assertNoLeak] = assertTransportNoLeak(db, output, alpha, redirectTarget)

	// Controls on the controls: every endpoint is genuinely reachable, so a zero
	// anywhere above is a fact about the engine and not about a broken listener.
	redirectTarget.proveReachable(t)
	for _, endpoint := range []*recordingDestination{alpha, beta, flaky, denied, moved} {
		endpoint.proveReachable(t)
	}
	return results
}

func assertTransportScopedIdentity(alpha, beta, flaky *recordingDestination) error {
	alphaRequests := alpha.Requests()
	betaRequests := beta.Requests()
	if len(alphaRequests) != 1 {
		return fmt.Errorf("alpha served %d requests, want exactly 1", len(alphaRequests))
	}
	if len(betaRequests) != 1 {
		return fmt.Errorf("beta served %d requests, want exactly 1", len(betaRequests))
	}
	if !strings.Contains(alphaRequests[0].authorization, httpsAlphaSecretSentinel) {
		return fmt.Errorf("alpha's Authorization does not carry identity A's material")
	}
	if strings.Contains(alphaRequests[0].authorization, httpsBetaSecretSentinel) {
		return fmt.Errorf("alpha's Authorization carries identity B's material")
	}
	if !strings.Contains(betaRequests[0].authorization, httpsBetaSecretSentinel) {
		return fmt.Errorf("beta's Authorization does not carry identity B's material")
	}
	if strings.Contains(betaRequests[0].authorization, httpsAlphaSecretSentinel) {
		return fmt.Errorf("beta's Authorization carries identity A's material")
	}
	for name, requests := range map[string][]recordedRequest{
		"alpha": alphaRequests, "beta": betaRequests, "flaky": flaky.Requests(),
	} {
		for _, request := range requests {
			if request.method != "POST" {
				return fmt.Errorf("%s was contacted with %s, want the declared POST", name, request.method)
			}
			if request.idempotencyKey == "" {
				return fmt.Errorf("%s received no server-owned Idempotency-Key", name)
			}
		}
	}
	return nil
}

func assertTransportNoRedelivery(
	ctx context.Context,
	dispatcher *Dispatcher,
	alpha, beta, flaky *recordingDestination,
) error {
	before := []int{len(alpha.Requests()), len(beta.Requests()), len(flaky.Requests())}
	if before[0] != 1 || before[1] != 1 || before[2] != 2 {
		return fmt.Errorf("request counts before the idle poll = %v, want [1 1 2]", before)
	}
	outcome, err := dispatcher.RunOnce(ctx)
	if err != nil {
		return fmt.Errorf("RunOnce on a drained queue: %w", err)
	}
	if outcome != OutcomeIdle {
		return fmt.Errorf("RunOnce on a drained queue = %q, want idle", outcome)
	}
	after := []int{len(alpha.Requests()), len(beta.Requests()), len(flaky.Requests())}
	if after[0] != before[0] || after[1] != before[1] || after[2] != before[2] {
		return fmt.Errorf("request counts moved on an idle poll: %v -> %v", before, after)
	}
	return nil
}

func assertTransportFlakyCircuit(db *sql.DB, attemptID string, flaky *recordingDestination) error {
	if requests := flaky.Requests(); len(requests) != 2 {
		return fmt.Errorf("flaky served %d requests, want 2 (503 then 200)", len(requests))
	}
	var attemptStatus string
	var attemptCount int
	if err := db.QueryRow(`
		SELECT status, attempt_count FROM integration_delivery_attempts
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
	`, attemptID).Scan(&attemptStatus, &attemptCount); err != nil {
		return fmt.Errorf("read flaky attempt: %w", err)
	}
	if attemptStatus != "succeeded" || attemptCount != 2 {
		return fmt.Errorf("flaky attempt = %s/%d, want succeeded/2", attemptStatus, attemptCount)
	}
	var state string
	var failures int
	if err := db.QueryRow(`
		SELECT state, consecutive_failures FROM integration_delivery_circuits
		WHERE tenant_id = 'tenant-a' AND destination_artifact_id = 'dest-https-flaky'
	`).Scan(&state, &failures); err != nil {
		return fmt.Errorf("read flaky circuit: %w", err)
	}
	if state != "closed" || failures != 0 {
		return fmt.Errorf("flaky circuit = %s/%d after recovery, want closed/0", state, failures)
	}
	// The retry ran through the existing MarkFailed, so the durable audit trail
	// carries the scheduled retry the circuit counter was incremented by.
	var retries int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM integration_delivery_audit
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1 AND event_kind = 'retry_scheduled'
	`, attemptID).Scan(&retries); err != nil {
		return fmt.Errorf("read flaky audit: %w", err)
	}
	if retries != 1 {
		return fmt.Errorf("flaky recorded %d retry_scheduled audit rows, want 1", retries)
	}
	if err := assertDeliveryProvenance(db, attemptID, []string{"retryable", "delivered"}); err != nil {
		return err
	}
	return nil
}

func assertTransportTerminalStatus(
	db *sql.DB,
	attemptByArtifact map[string]string,
	denied, moved *recordingDestination,
	redirectTarget *destinationListener,
) error {
	deniedAttempt := attemptByArtifact["dest-https-denied"]
	movedAttempt := attemptByArtifact["dest-https-moved"]
	for artifact, attemptID := range map[string]string{
		"dest-https-denied": deniedAttempt, "dest-https-moved": movedAttempt,
	} {
		var status, dlqCode string
		var attemptCount int
		var active bool
		if err := db.QueryRow(`
			SELECT a.status, a.attempt_count, d.failure_code, d.active
			FROM integration_delivery_attempts a
			JOIN integration_delivery_dlq d
			  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
			WHERE a.tenant_id = 'tenant-a' AND a.attempt_id = $1
		`, attemptID).Scan(&status, &attemptCount, &dlqCode, &active); err != nil {
			return fmt.Errorf("read %s dead letter: %w", artifact, err)
		}
		if status != "failed" || attemptCount != 1 || !active {
			return fmt.Errorf("%s = %s/%d active=%v, want failed/1 active", artifact, status, attemptCount, active)
		}
		if dlqCode == "" {
			return fmt.Errorf("%s dead letter carries no failure code", artifact)
		}
	}
	if requests := denied.Requests(); len(requests) != 1 {
		return fmt.Errorf("denied served %d requests, want exactly 1", len(requests))
	}
	if requests := moved.Requests(); len(requests) != 1 {
		return fmt.Errorf("moved served %d requests, want exactly 1", len(requests))
	}
	if accepted := redirectTarget.Accepted(); accepted != 0 {
		return fmt.Errorf("the redirect target accepted %d connections, want 0", accepted)
	}
	if err := assertDeliveryProvenance(db, deniedAttempt, []string{"refused"}); err != nil {
		return err
	}
	if err := assertDeliveryProvenance(db, movedAttempt, []string{"refused"}); err != nil {
		return err
	}
	var class, code string
	if err := db.QueryRow(`
		SELECT http_status_class, failure_code FROM integration_destination_deliveries
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
	`, movedAttempt).Scan(&class, &code); err != nil {
		return fmt.Errorf("read moved provenance: %w", err)
	}
	if class != "3xx" || code != destination.FailureRedirect {
		return fmt.Errorf("moved provenance = %s/%s, want 3xx/%s", class, code, destination.FailureRedirect)
	}
	return nil
}

func assertTransportKafkaClass(
	t *testing.T,
	ctx context.Context,
	brokers []string,
	attemptID string,
	httpsEndpoints []*recordingDestination,
	counting *countingPublisher,
	ownsNothing bool,
) error {
	t.Helper()
	records := recordsByKey(drainDeliveryRecords(t, ctx, brokers, deliveryCommandSchema))[attemptID]
	if len(records) != 1 {
		return fmt.Errorf("kafka-class attempt produced %d broker records, want exactly 1", len(records))
	}
	if records[0].Topic != "integration.delivery.v1" {
		return fmt.Errorf("kafka-class record landed on %q, want the constant delivery topic", records[0].Topic)
	}
	if ownsNothing {
		// Under the control every destination publishes, so only the kafka-class
		// claim above is meaningful; the per-endpoint zero below would be a
		// statement about the control rather than about the engine.
		return nil
	}
	if published := counting.count.Load(); published != 1 {
		return fmt.Errorf("the broker saw %d publishes, want exactly the one kafka-class destination", published)
	}
	total := 0
	for _, endpoint := range httpsEndpoints {
		total += len(endpoint.Requests())
	}
	if total != 6 {
		return fmt.Errorf("https endpoints served %d requests in total, want 6", total)
	}
	return nil
}

func assertTransportNoLeak(
	db *sql.DB,
	output string,
	alpha *recordingDestination,
	redirectTarget *destinationListener,
) error {
	queries := []string{
		`SELECT result_json::text FROM integration_receipts`,
		`SELECT payload_json::text FROM integration_canonical_events`,
		`SELECT artifact_revisions_json::text || routes_json::text FROM integration_message_lineage`,
		`SELECT destination_revision_json::text FROM integration_delivery_attempts`,
		`SELECT payload_json::text FROM integration_delivery_outbox`,
		`SELECT detail_json::text || principal_json::text FROM integration_delivery_audit`,
		`SELECT failure_code || failure_detail FROM integration_delivery_dlq`,
		`SELECT principal_subject || granted_role || destination_endpoint_advisory ||
			destination_digest_verified FROM integration_delivery_identity_decisions`,
		`SELECT outcome || failure_code || http_status_class || destination_endpoint_advisory ||
			served_certificate_subject_advisory FROM integration_destination_deliveries`,
	}
	for _, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("read durable record class: %w", err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan durable record class: %w", err)
			}
			if strings.Contains(value, httpsAlphaSecretSentinel) ||
				strings.Contains(value, httpsBetaSecretSentinel) {
				_ = rows.Close()
				return fmt.Errorf("a durable record carries a destination credential")
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate durable record class: %w", err)
		}
		_ = rows.Close()
	}
	// The five classes of correction 9 may carry no destination address at all.
	// The advisory columns of the two provenance ledgers are the only place one
	// lives, and they are deliberately outside this scan.
	addressed := map[string]string{
		"integration_receipts":          `SELECT result_json::text FROM integration_receipts`,
		"integration_canonical_events":  `SELECT payload_json::text FROM integration_canonical_events`,
		"integration_message_lineage":   `SELECT artifact_revisions_json::text || routes_json::text FROM integration_message_lineage`,
		"integration_delivery_attempts": `SELECT destination_revision_json::text FROM integration_delivery_attempts`,
		"integration_delivery_outbox":   `SELECT payload_json::text FROM integration_delivery_outbox`,
	}
	for table, query := range addressed {
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("read %s: %w", table, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan %s: %w", table, err)
			}
			for _, forbidden := range []string{
				alpha.URL(), redirectTarget.URL(), "https://", "http://", "127.0.0.1",
			} {
				if strings.Contains(value, forbidden) {
					_ = rows.Close()
					return fmt.Errorf("%s carries destination address fragment %q", table, forbidden)
				}
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate %s: %w", table, err)
		}
		_ = rows.Close()
	}
	if strings.Contains(output, httpsAlphaSecretSentinel) ||
		strings.Contains(output, httpsBetaSecretSentinel) {
		return fmt.Errorf("process output carries a destination credential")
	}
	// The bytes actually delivered are the raw-free delivery command, not the
	// source message and not anything carrying an address.
	for _, request := range alpha.Requests() {
		if strings.Contains(request.body, rawIngressSentinel) {
			return fmt.Errorf("the delivered payload carries the raw source sentinel")
		}
		if strings.Contains(request.body, "https://") || strings.Contains(request.body, "127.0.0.1") {
			return fmt.Errorf("the delivered payload carries a destination address")
		}
	}
	return nil
}

// assertDeliveryProvenance proves the destination delivery ledger recorded
// exactly the expected outcomes, in order, for one attempt.
func assertDeliveryProvenance(db *sql.DB, attemptID string, want []string) error {
	rows, err := db.Query(`
		SELECT outcome FROM integration_destination_deliveries
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
		ORDER BY delivery_id
	`, attemptID)
	if err != nil {
		return fmt.Errorf("read delivery provenance for %s: %w", attemptID, err)
	}
	defer func() { _ = rows.Close() }()
	got := make([]string, 0, len(want))
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			return fmt.Errorf("scan delivery provenance: %w", err)
		}
		got = append(got, outcome)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate delivery provenance: %w", err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		return fmt.Errorf("delivery provenance for %s = %v, want %v", attemptID, got, want)
	}
	return nil
}

// ownsNothingTransport is the negative control's router: it reports that no
// destination is its own, which is exactly the pre-slice behaviour.
type ownsNothingTransport struct{}

func (ownsNothingTransport) DeliverDestination(
	context.Context, string, string, integration.DestinationRevisionRef, []byte,
) (bool, error) {
	return false, nil
}

// recordedRequest is one served request, reduced to what the proof asserts on.
type recordedRequest struct {
	method         string
	authorization  string
	idempotencyKey string
	body           string
}

// recordingDestination is a live TLS endpoint that records every request it
// serves and answers with a scripted status sequence.
type recordingDestination struct {
	server     *httptest.Server
	listener   *countingListener
	redirectTo string

	mu       sync.Mutex
	requests []recordedRequest
	statuses []int
	served   atomic.Int64
}

// newRecordingDestination starts a real TLS server on loopback. statuses are
// returned one per request; the last repeats for every request after it.
func newRecordingDestination(t *testing.T, statuses ...int) *recordingDestination {
	t.Helper()
	if len(statuses) == 0 {
		t.Fatal("a recording destination needs at least one scripted status")
	}
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for recording destination: %v", err)
	}
	endpoint := &recordingDestination{
		listener: &countingListener{Listener: base},
		statuses: append([]int(nil), statuses...),
	}
	endpoint.server = httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			endpoint.mu.Lock()
			index := len(endpoint.requests)
			endpoint.requests = append(endpoint.requests, recordedRequest{
				method:         r.Method,
				authorization:  r.Header.Get("Authorization"),
				idempotencyKey: r.Header.Get("Idempotency-Key"),
				body:           string(body),
			})
			if index >= len(endpoint.statuses) {
				index = len(endpoint.statuses) - 1
			}
			status := endpoint.statuses[index]
			endpoint.mu.Unlock()
			endpoint.served.Add(1)
			if endpoint.redirectTo != "" {
				w.Header().Set("Location", endpoint.redirectTo)
			}
			w.WriteHeader(status)
		}))
	_ = endpoint.server.Listener.Close()
	endpoint.server.Listener = endpoint.listener
	endpoint.server.StartTLS()
	t.Cleanup(endpoint.server.Close)
	return endpoint
}

func (d *recordingDestination) URL() string { return d.server.URL }

// CertificatePEM is the endpoint's self-signed certificate, written to disk as
// the destination's declared CA bundle. It is how the production transport's
// trust roots come from the deployment rather than from the destination.
func (d *recordingDestination) CertificatePEM() string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: d.server.Certificate().Raw,
	}))
}

func (d *recordingDestination) Requests() []recordedRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]recordedRequest(nil), d.requests...)
}

// proveReachable dials the endpoint from the test itself, so a zero or a small
// count above is a fact about the engine and not about a broken listener. The
// probe's own request is recorded and then removed, leaving the counts the
// assertions read untouched.
func (d *recordingDestination) proveReachable(t *testing.T) {
	t.Helper()
	before := d.served.Load()
	response, err := d.server.Client().Get(d.server.URL + "/probe")
	if err != nil {
		t.Fatalf("destination endpoint is not reachable, so its counts prove nothing: %v", err)
	}
	_ = response.Body.Close()
	if d.served.Load() <= before {
		t.Fatalf("destination endpoint counters did not register a real contact: %d -> %d",
			before, d.served.Load())
	}
}

// fileSecretResolver is the proof's stand-in for the file-backed resolver cmd/
// wires in production. It is deliberately a separate, minimal implementation:
// the production one lives in package main and cannot be imported.
type fileSecretResolver struct {
	root string
}

func newFileSecretResolver(root string) *fileSecretResolver {
	return &fileSecretResolver{root: root}
}

func (r *fileSecretResolver) Resolve(
	ctx context.Context,
	reference integration.SecretReference,
) ([]byte, error) {
	if ctx == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if integration.ValidateSecretReference(reference) != nil ||
		reference.Provider != integration.SecretProviderFile || reference.Version != "" {
		return nil, integration.ErrSecretUnresolvable
	}
	raw, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(reference.Key)))
	if err != nil || len(raw) == 0 || len(raw) > integration.MaxSecretBytes {
		return nil, integration.ErrSecretUnresolvable
	}
	return raw, nil
}

// captureProcessOutput redirects os.Stdout and os.Stderr for the duration of one
// call and returns everything written to them.
func captureProcessOutput(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create output pipe: %v", err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer
	captured := make(chan string, 1)
	go func() {
		var buffer strings.Builder
		_, _ = io.Copy(&buffer, reader)
		captured <- buffer.String()
	}()
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		_ = writer.Close()
		_ = reader.Close()
	}()
	run()
	os.Stdout, os.Stderr = stdout, stderr
	_ = writer.Close()
	output := <-captured
	_ = reader.Close()
	return output
}

func mustTLSDestinationRevision(
	t *testing.T,
	artifactID, subject, tokenBinding, caBinding, url string,
) destination.Revision {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID:    artifactID,
		RevisionID:    "destination-1",
		DestinationID: artifactID,
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportHTTPS,
		HTTPS: &destination.HTTPSPolicy{
			URL: url, Method: "POST",
			TokenBinding: tokenBinding, CABundleBinding: caBinding,
		},
		Identity: &destination.ClientIdentity{
			Subject: subject, Grants: []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(%s): %v", artifactID, err)
	}
	return revision
}

func mustKafkaDestinationRevision(t *testing.T, artifactID, subject string) destination.Revision {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID:    artifactID,
		RevisionID:    "destination-1",
		DestinationID: artifactID,
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportKafka,
		Kafka:         &destination.KafkaPolicy{Topic: deliveryCommandSchema},
		Identity: &destination.ClientIdentity{
			Subject: subject, Grants: []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(%s): %v", artifactID, err)
	}
	return revision
}
