//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestChaosRecovery_DestinationOutageOpensTheCircuitAndResumesOnRepair is slice
// 4.4c's budget 4 proof: "destination recovery under an injected fault"
// (.loom/20-product-spec-integration-engine-ide-completion.md, budget 4). Of
// the seven release budgets it is the one fully provable in CI today, and every
// primitive it needs already shipped.
//
// The fault is injected with a TCP proxy in front of the destination's TLS
// endpoint, not by stopping a container. That is not a convenience: a GitLab
// job receives its dependencies as service containers and has no Docker socket,
// so "stop the destination" is not available, and severing the socket is the
// stronger fault anyway — it exercises the dispatcher's own reconnect and
// classification rather than a container's restart timing. The pattern is
// S3-A's, at internal/observability/replicas_integration_test.go:872-969; the
// copy below is destination-facing and unexported for the same reason S3-A's
// is.
//
// The shape is one https destination behind the proxy and three queued attempts
// against it, because the property under test is about the destination, not
// about one message:
//
//  0. Healthy control. One attempt delivers through the proxy. Without this,
//     every zero below could be a broken fixture rather than an outage.
//  1. Break(). The dispatcher runs. Each claimed attempt fails to reach the
//     destination, is classified DELIVERY_DESTINATION_UNREACHABLE and retried,
//     and after CircuitFailureThreshold consecutive failures the circuit opens
//     and the remaining attempts stop being claimed at all. That is the bound:
//     the number of contacts wasted on a dead destination is capped by the
//     threshold and does not grow with the depth of the queue.
//  2. Repair(), clock unmoved. Nothing is delivered. This is the control on
//     step 3: it proves resumption is attributable to the circuit closing and
//     the backoff elapsing, and not merely to the network coming back.
//  3. The clock advances past the open duration and the backoff. Every queued
//     attempt is delivered EXACTLY ONCE with no operator intervention — no
//     replay, no resubmit, nothing in integration_delivery_operations — and the
//     circuit closes.
//  4. A further poll delivers nothing. Recovery must not manufacture a
//     duplicate delivery for an idempotency key the destination already
//     accepted.
//
// No Kafka. Every destination here is https-class, so the broker publisher is
// never reached; a stub Publisher satisfies the dispatcher's requirement and
// the proof needs one PostgreSQL service container and nothing else.
func TestChaosRecovery_DestinationOutageOpensTheCircuitAndResumesOnRepair(t *testing.T) {
	ctx := t.Context()

	endpoint := newRecordingDestination(t, 200)
	proxy := startDestinationProxy(t, endpoint.URL())

	secretRoot := t.TempDir()
	writeIdentitySecret(t, secretRoot, "chaos/token", "chaos-token-material")
	writeIdentitySecret(t, secretRoot, "chaos/ca.pem", endpoint.CertificatePEM())

	// The destination's declared URL is the proxy's address. Its declared CA is
	// still the endpoint's own certificate: the proxy forwards bytes and
	// terminates nothing, so TLS is negotiated end to end with the endpoint and
	// the certificate's 127.0.0.1 SAN covers the proxy's loopback address too.
	revision := mustTLSDestinationRevision(
		t, "dest-https-chaos", "chaos-client", "chaos-token", "chaos-ca", proxy.URL(),
	)

	dsn := deliveryPostgresDSN(t, ctx)
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
		destinationWorkflowYAMLFor("webhook", revision.ArtifactID),
		[]integration.DestinationRevisionRef{revision.Reference()},
	)

	registry := mustRegistry(t, transportRegistryJSON(t,
		map[string]string{"chaos-token": "chaos/token", "chaos-ca": "chaos/ca.pem"},
		revision,
	), destination.ModeStrict)
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
	transport, err := destination.NewTransport(destination.TransportConfig{
		Registry: registry,
		Resolver: newFileSecretResolver(secretRoot),
		Recorder: provenance,
		Clock:    clock.Now,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	store, err := NewPostgresStore(db, clock.Now)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	config := DefaultConfig()
	// Small, explicit, and all derived from each other so the assertions can
	// name the mechanism. PublishTimeout must stay below LeaseDuration or
	// Config.validate refuses the dispatcher.
	config.CircuitFailureThreshold = 2
	config.CircuitOpenDuration = 5 * time.Second
	config.MaxAttempts = 5
	config.RetryBaseDelay = time.Second
	config.RetryMaxDelay = 2 * time.Second
	config.PublishTimeout = 3 * time.Second
	config.LeaseDuration = 30 * time.Second

	dispatcher, err := NewDispatcherWithDestination(
		store, &fakePublisher{}, "worker-chaos", config, authorizer, transport,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithDestination: %v", err)
	}

	// ---- Step 0: healthy control -------------------------------------------
	admitted := submitOne(ctx, t, fixture, "chaos-healthy")
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomePublished {
		t.Fatalf("healthy control: RunOnce outcome=%q error=%v, want published.\n"+
			"  Nothing below means anything if the destination cannot be reached through the "+
			"proxy while the proxy is intact.", outcome, err)
	}
	if served := len(endpoint.Requests()); served != 1 {
		t.Fatalf("healthy control: destination served %d requests, want exactly 1", served)
	}

	// ---- Step 1: sever the destination -------------------------------------
	//
	// The outage attempts hang off the receipt and canonical event the durable
	// submission above already admitted, which is the real fan-out shape: one
	// admitted event, several delivery attempts. They are seeded rather than
	// re-submitted because a canonical event id is derived from the message and
	// a second submission of the same envelope collides on its primary key —
	// re-admitting would be testing idempotency, not recovery.
	const queued = 3
	attempts := make([]string, 0, queued)
	for index := range queued {
		attemptID := fmt.Sprintf("attempt-chaos-%d", index)
		seedChaosAttempt(t, db, clock.Now(), revision.Reference(),
			admitted.receiptID, admitted.eventID, attemptID, fmt.Sprintf("outbox-chaos-%d", index))
		attempts = append(attempts, attemptID)
	}

	proxy.Break()
	outageOutcomes := drainDispatcher(t, ctx, dispatcher, queued)

	if retries := outageOutcomes[OutcomeRetry]; retries != config.CircuitFailureThreshold {
		t.Fatalf("the outage produced %d retry-scheduled outcomes, want exactly %d "+
			"(CircuitFailureThreshold).\n"+
			"  %d attempts were queued against a dead destination. The circuit exists so the "+
			"number of contacts wasted on it is capped by the threshold rather than growing "+
			"with the depth of the queue. Outcomes: %v",
			retries, config.CircuitFailureThreshold, queued, outageOutcomes)
	}
	if dlq := outageOutcomes[OutcomeDLQ]; dlq != 0 {
		t.Fatalf("the outage dead-lettered %d attempts. A destination that is unreachable is "+
			"retryable (DELIVERY_DESTINATION_UNREACHABLE); dead-lettering it would discard "+
			"deliverable work on a transient fault.", dlq)
	}
	assertChaosCircuit(t, db, revision.ArtifactID, "open", config.CircuitFailureThreshold)

	// The severed proxy refuses the connection before any TLS handshake, so the
	// endpoint must not have seen anything beyond the healthy control.
	if served := len(endpoint.Requests()); served != 1 {
		t.Fatalf("destination served %d requests during the outage, want the healthy control's "+
			"1 and nothing more", served)
	}
	// Nothing was lost and nothing was abandoned: every attempt is still queued
	// and none has burned through MaxAttempts.
	for _, attemptID := range attempts {
		status, count := chaosAttemptState(t, db, attemptID)
		if status != "queued" {
			t.Fatalf("attempt %s is %q after the outage, want queued", attemptID, status)
		}
		if count < 1 || count > config.MaxAttempts {
			t.Fatalf("attempt %s has attempt_count %d, outside [1, %d]",
				attemptID, count, config.MaxAttempts)
		}
	}

	// ---- Step 2: repair the network, do not move the clock ------------------
	//
	// The control on step 3. If work resumed here, resumption would be
	// attributable to the socket rather than to the circuit and the backoff,
	// and step 3's success would prove nothing about either.
	proxy.Repair()
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomeIdle {
		t.Fatalf("CONTROL BROKEN: with the destination reachable again but the circuit still "+
			"open and the backoff not yet elapsed, RunOnce returned outcome=%q error=%v, want "+
			"idle.\n"+
			"  Work resuming here would mean the circuit is not gating the claim, so step 3's "+
			"recovery would not be evidence that the circuit closes correctly.", outcome, err)
	}
	if served := len(endpoint.Requests()); served != 1 {
		t.Fatalf("destination served %d requests while the circuit was open, want 1", served)
	}

	// ---- Step 3: the circuit closes and the queue drains itself -------------
	clock.Advance(config.CircuitOpenDuration + config.RetryMaxDelay)
	recovered := drainDispatcher(t, ctx, dispatcher, queued)
	if published := recovered[OutcomePublished]; published != queued {
		t.Fatalf("recovery published %d of %d queued attempts with no manual intervention. "+
			"Outcomes: %v", published, queued, recovered)
	}
	if served := len(endpoint.Requests()); served != 1+queued {
		t.Fatalf("destination served %d requests in total, want %d — the healthy control plus "+
			"exactly one per queued attempt. More than that is a duplicate delivery for an "+
			"idempotency key the destination already accepted.", served, 1+queued)
	}
	for _, attemptID := range attempts {
		status, count := chaosAttemptState(t, db, attemptID)
		if status != "succeeded" {
			t.Fatalf("attempt %s is %q after recovery, want succeeded", attemptID, status)
		}
		if count > config.MaxAttempts {
			t.Fatalf("attempt %s ended at attempt_count %d, above MaxAttempts %d",
				attemptID, count, config.MaxAttempts)
		}
	}
	assertChaosCircuit(t, db, revision.ArtifactID, "closed", 0)

	// Recovery is automatic by contract, so the operator control plane must be
	// untouched: no replay, no resubmit, no manual repair of any kind.
	var operations int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM integration_delivery_operations`,
	).Scan(&operations); err != nil {
		t.Fatalf("count delivery operations: %v", err)
	}
	if operations != 0 {
		t.Fatalf("%d operator operations were recorded. Budget 4 is recovery WITHOUT manual "+
			"repair; an operation here means the queue did not drain itself.", operations)
	}

	// ---- Step 4: a drained queue redelivers nothing -------------------------
	if outcome, err := dispatcher.RunOnce(ctx); err != nil || outcome != OutcomeIdle {
		t.Fatalf("post-recovery RunOnce outcome=%q error=%v, want idle", outcome, err)
	}
	if served := len(endpoint.Requests()); served != 1+queued {
		t.Fatalf("destination served %d requests after the queue drained, want %d",
			served, 1+queued)
	}

	// Control on the counters: the endpoint is genuinely live, so every count
	// above is a fact about the engine rather than about a dead listener.
	endpoint.proveReachable(t)
}

// admittedSubmission is the durable identity one production submission created.
type admittedSubmission struct {
	receiptID string
	eventID   string
	attemptID string
}

// submitOne runs one production submission through durable admission.
func submitOne(
	ctx context.Context,
	t *testing.T,
	fixture durableSubmissionFixture,
	label string,
) admittedSubmission {
	t.Helper()
	request := fixture.request
	request.IdempotencyKey = fmt.Sprintf("%s-%d", label, time.Now().UnixNano())
	result, err := fixture.processor.Process(ctx, request)
	if err != nil {
		t.Fatalf("durable production submission %s: %v", label, err)
	}
	if result.Receipt == nil || len(result.Deliveries) != 1 ||
		result.Deliveries[0].Status != integration.DeliveryStatusQueued {
		t.Fatalf("submission %s produced %#v, want exactly one queued delivery", label, result.Deliveries)
	}
	return admittedSubmission{
		receiptID: result.Receipt.ID,
		eventID:   result.Deliveries[0].EventID,
		attemptID: result.Deliveries[0].AttemptID,
	}
}

// seedChaosAttempt queues one more delivery attempt against an already-admitted
// receipt and canonical event, carrying the real deployed destination revision
// reference.
//
// The reference is the real one on purpose: the strict registry resolves an
// attempt by matching its reference byte for byte, so a fabricated digest would
// be refused by the identity decision before the transport ever ran and the
// outage would never be reached.
func seedChaosAttempt(
	t *testing.T,
	db *sql.DB,
	now time.Time,
	ref integration.DestinationRevisionRef,
	receiptID, eventID, attemptID, outboxID string,
) {
	t.Helper()
	destinationJSON, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal destination reference: %v", err)
	}
	payload := fmt.Sprintf(
		`{"schema":%q,"tenant_id":"tenant-a","receipt_id":%q,"event_id":%q,`+
			`"trace_id":"trace-chaos","attempt_id":%q,"destination":%s,`+
			`"route":"matched","action":"webhook","attempt_count":1}`,
		deliveryCommandSchema, receiptID, eventID, attemptID, destinationJSON)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed chaos attempt: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id,
			destination_revision_json, route_name, action_id, status,
			attempt_count, recorded_at, scheduled_at
		) VALUES ('tenant-a', $2, $3, $4, 'trace-chaos', $5,
			'matched', 'webhook', 'queued', 1, $1, $1)
	`, now, attemptID, receiptID, eventID, destinationJSON); err != nil {
		t.Fatalf("seed chaos delivery attempt: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_delivery_outbox (
			tenant_id, outbox_id, attempt_id, topic, status, payload_json,
			created_at, scheduled_at, updated_at
		) VALUES ('tenant-a', $2, $3, $4, 'pending', $5, $1, $1, $1)
	`, now, outboxID, attemptID, deliveryCommandSchema, payload); err != nil {
		t.Fatalf("seed chaos delivery outbox: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed chaos attempt: %v", err)
	}
}

func chaosAttemptState(t *testing.T, db *sql.DB, attemptID string) (string, int) {
	t.Helper()
	var status string
	var count int
	if err := db.QueryRow(`
		SELECT status, attempt_count FROM integration_delivery_attempts
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
	`, attemptID).Scan(&status, &count); err != nil {
		t.Fatalf("read attempt %s: %v", attemptID, err)
	}
	return status, count
}

// assertChaosCircuit reads the per-destination circuit. It is separate from
// assertCircuitState because that one is pinned to the `queue-primary`
// destination the broker proofs seed.
func assertChaosCircuit(t *testing.T, db *sql.DB, artifactID, wantState string, wantFailures int) {
	t.Helper()
	var state string
	var failures int
	if err := db.QueryRow(`
		SELECT state, consecutive_failures FROM integration_delivery_circuits
		WHERE tenant_id = 'tenant-a' AND destination_artifact_id = $1
	`, artifactID).Scan(&state, &failures); err != nil {
		t.Fatalf("read circuit for %s: %v", artifactID, err)
	}
	if state != wantState || failures != wantFailures {
		t.Fatalf("circuit for %s = %s/%d failures, want %s/%d",
			artifactID, state, failures, wantState, wantFailures)
	}
}

// ---------------------------------------------------------------------------
// Destination proxy
// ---------------------------------------------------------------------------

// destinationProxy interposes on the dispatcher's connections to a destination
// so a test can make that destination unreachable from inside a CI job.
//
// It is the destination-facing twin of the PostgreSQL proxy at
// internal/observability/replicas_integration_test.go:882-969, and it exists
// for the reason recorded there: a GitLab job receives its dependencies as
// service containers and has no Docker socket, so "stop the container" is not
// an available fault. Severing the socket is portable and stronger — it
// exercises the transport's own classification and the dispatcher's circuit
// rather than a container's restart timing.
//
// It forwards bytes and terminates no TLS. The destination's certificate is
// therefore still the endpoint's own, and the dispatcher validates it against
// the CA bundle the deployed revision declares, exactly as in production.
type destinationProxy struct {
	listener net.Listener
	target   string

	mu     sync.Mutex
	broken bool
	conns  []net.Conn
	closed bool
}

// startDestinationProxy stands a proxy in front of an https destination and
// returns it. targetURL is the endpoint's own https:// URL.
func startDestinationProxy(t *testing.T, targetURL string) *destinationProxy {
	t.Helper()
	target := targetURL
	for _, prefix := range []string{"https://", "http://"} {
		if len(target) > len(prefix) && target[:len(prefix)] == prefix {
			target = target[len(prefix):]
			break
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start destination proxy: %v", err)
	}
	proxy := &destinationProxy{listener: listener, target: target}
	t.Cleanup(proxy.Close)
	go proxy.accept()
	return proxy
}

// URL is the address a destination revision declares to be reached through the
// proxy.
func (p *destinationProxy) URL() string { return "https://" + p.listener.Addr().String() }

func (p *destinationProxy) accept() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		broken := p.broken
		if !broken {
			p.conns = append(p.conns, client)
		}
		p.mu.Unlock()
		if broken {
			_ = client.Close()
			continue
		}
		go p.pipe(client)
	}
}

func (p *destinationProxy) pipe(client net.Conn) {
	upstream, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	p.conns = append(p.conns, upstream)
	p.mu.Unlock()

	go func() {
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
}

// Break refuses new connections and severs live ones.
func (p *destinationProxy) Break() {
	p.mu.Lock()
	p.broken = true
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// Repair restores forwarding. The listener never stopped accepting, so nothing
// on the dispatcher side has to be reconfigured.
func (p *destinationProxy) Repair() {
	p.mu.Lock()
	p.broken = false
	p.mu.Unlock()
}

func (p *destinationProxy) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	_ = p.listener.Close()
}
