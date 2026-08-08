//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/twmb/franz-go/pkg/kgo"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// destinationSecretSentinel is planted in identity A's secret file. It must
// appear in no durable record, no Kafka field, and no process output.
const destinationSecretSentinel = "DESTINATION-SECRET-SENTINEL-9f3c"

// TestDeliveryIdentity_PostgresKafkaScopedDispatch is the Slice 4.1c-a kill-test.
//
// One tenant, one integration revision, three destinations: dest-alpha bound to
// identity A, dest-beta bound to identity B, and dest-orphan planned by the
// workflow but absent from the deployed destination set. It proves the
// integration.deliver decision runs on the real dispatch path with real durable
// consequences rather than being a contract nobody consults.
//
//  1. Alpha and beta both publish, and the recorded provenance names A for alpha
//     and B for beta; neither names the other.
//  2. An attempt for dest-beta carrying dest-alpha's destination digest fails the
//     digest check and is dead-lettered, not published.
//  3. dest-orphan produces a DELIVERY_FORBIDDEN dead letter with attempt_count
//     unchanged and zero Kafka records for its attempt ID.
//  4. The sentinel planted in identity A's secret appears in none of the durable
//     record classes, the Kafka key/value/headers, or the decision provenance.
//  5. compatibility mode authorizes the unbound class; strict mode carrying
//     compatibility-only configuration fails startup.
//
// Negative control: stubbing the decision to return nil unconditionally fails
// assertions 2, 3, and 5.
func TestDeliveryIdentity_PostgresKafkaScopedDispatch(t *testing.T) {
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
		t.Fatalf("Migrate submission store: %v", err)
	}

	clock := fixedDestinationClock()
	// Unique per run so per-attempt Kafka assertions stay exact on a broker that
	// retains records from earlier runs.
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	attemptAlpha := "attempt-alpha-" + runID
	attemptBeta := "attempt-beta-" + runID
	attemptCrossed := "attempt-crossed-" + runID
	attemptOrphan := "attempt-orphan-" + runID
	secretRoot := t.TempDir()
	writeIdentitySecret(t, secretRoot, "alpha/token", destinationSecretSentinel)
	writeIdentitySecret(t, secretRoot, "beta/token", "beta-token-material")

	alpha := mustDestinationRevision(t, "dest-alpha", "alpha", "alpha-client", "alpha-token",
		"https://alpha.example/fhir")
	beta := mustDestinationRevision(t, "dest-beta", "beta", "beta-client", "beta-token",
		"https://beta.example/fhir")

	// The deployed registry holds alpha and beta only. dest-orphan is named by the
	// workflow and carried on a durable attempt, but the deployment never
	// published a destination revision for it.
	registry := mustRegistry(t, strictRegistryJSON(t, alpha, beta), destination.ModeStrict)
	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("Migrate provenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("Migrate provenance (idempotent): %v", err)
	}
	authorizer, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance, Clock: clock,
	})
	if err != nil {
		t.Fatalf("NewAuthorizer: %v", err)
	}

	// Assertion 5, first half: a strict deployment carrying compatibility-only
	// configuration must not start.
	if _, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance, CompatibilitySubject: "fallback-client",
	}); err == nil {
		t.Fatal("strict mode accepted a compatibility subject")
	}
	unboundRegistry := strictRegistryJSON(t, mustUnboundDestinationRevision(t))
	if _, err := destination.LoadRegistry(strings.NewReader(unboundRegistry), destination.ModeStrict); err == nil {
		t.Fatal("strict mode loaded a destination set containing an unbound destination")
	}

	// Seed one durable attempt per destination, exactly as the durable submission
	// path writes them.
	seedIdentityDelivery(t, db, clock(), attemptAlpha, "outbox-alpha", alpha.Reference())
	seedIdentityDelivery(t, db, clock(), attemptBeta, "outbox-beta", beta.Reference())
	crossed := beta.Reference()
	crossed.Digest = alpha.Digest
	seedIdentityDelivery(t, db, clock(), attemptCrossed, "outbox-crossed", crossed)
	orphan := alpha.Reference()
	orphan.ArtifactID = "dest-orphan"
	seedIdentityDelivery(t, db, clock(), attemptOrphan, "outbox-orphan", orphan)

	publisher, err := NewKafkaPublisher(KafkaConfig{
		Brokers:         brokers,
		ClientID:        "fi-fhir-delivery-identity-kill-test",
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
		store, counting, "worker-identity", dispatcherConfig, authorizer,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithIdentity: %v", err)
	}

	outcomes := drainDispatcher(t, ctx, dispatcher, 4)
	if outcomes[OutcomePublished] != 2 || outcomes[OutcomeForbidden] != 2 {
		t.Fatalf("dispatch outcomes = %#v, want 2 published and 2 forbidden", outcomes)
	}
	if published := counting.count.Load(); published != 2 {
		t.Fatalf("publisher saw %d messages, want exactly the 2 authorized dispatches", published)
	}

	// Assertion 1: each authorized dispatch is recorded under its own identity.
	alphaDecision := readIdentityDecision(t, db, attemptAlpha)
	betaDecision := readIdentityDecision(t, db, attemptBeta)
	if !alphaDecision.authorized || alphaDecision.subject != "alpha-client" ||
		alphaDecision.digest != alpha.Digest ||
		alphaDecision.grant != authorization.DestinationClientGrant {
		t.Fatalf("alpha decision = %#v", alphaDecision)
	}
	if !betaDecision.authorized || betaDecision.subject != "beta-client" ||
		betaDecision.digest != beta.Digest ||
		betaDecision.grant != authorization.DestinationClientGrant {
		t.Fatalf("beta decision = %#v", betaDecision)
	}
	if strings.Contains(alphaDecision.subject, "beta") || strings.Contains(betaDecision.subject, "alpha") {
		t.Fatal("a dispatch was authorized under another destination's identity")
	}
	if alphaDecision.mode != string(destination.ModeStrict) || alphaDecision.decidedAt.IsZero() {
		t.Fatalf("alpha provenance is not server-owned: %#v", alphaDecision)
	}
	if alphaDecision.endpointAdvisory != "https://alpha.example/fhir" {
		t.Fatalf("alpha advisory endpoint = %q", alphaDecision.endpointAdvisory)
	}

	// Assertion 2: a crossed digest is dead-lettered, never published.
	assertDeliveryState(t, db, attemptCrossed, "failed", 1, "failed", "active")
	crossedDecision := readIdentityDecision(t, db, attemptCrossed)
	if crossedDecision.authorized || crossedDecision.denialCode != UnverifiedDestinationFailureCode ||
		crossedDecision.grant != "" || crossedDecision.subject != "" {
		t.Fatalf("crossed decision = %#v", crossedDecision)
	}
	assertDLQFailureCode(t, db, attemptCrossed, UnverifiedDestinationFailureCode)

	// Assertion 3: dest-orphan is DELIVERY_FORBIDDEN with attempt_count unchanged.
	assertDeliveryState(t, db, attemptOrphan, "failed", 1, "failed", "active")
	orphanDecision := readIdentityDecision(t, db, attemptOrphan)
	if orphanDecision.authorized || orphanDecision.denialCode != ForbiddenFailureCode ||
		orphanDecision.digest != "" {
		t.Fatalf("orphan decision = %#v", orphanDecision)
	}
	assertDLQFailureCode(t, db, attemptOrphan, ForbiddenFailureCode)

	drained := drainDeliveryRecords(t, ctx, brokers, deliveryCommandSchema)
	byKey := recordsByKey(drained)
	if len(byKey[attemptAlpha]) != 1 || len(byKey[attemptBeta]) != 1 {
		t.Fatalf("Kafka records: alpha=%d beta=%d, want exactly one each",
			len(byKey[attemptAlpha]), len(byKey[attemptBeta]))
	}
	for _, denied := range []string{attemptOrphan, attemptCrossed} {
		if len(byKey[denied]) != 0 {
			t.Fatalf("denied attempt %s produced %d Kafka records", denied, len(byKey[denied]))
		}
	}
	records := append(append([]*kgo.Record(nil), byKey[attemptAlpha]...), byKey[attemptBeta]...)

	// Assertion 4: the sentinel planted in identity A's secret escapes nowhere.
	assertSentinelAbsent(t, db, records, secretRoot)

	// Assertion 5, second half: compatibility mode authorizes the unbound class
	// that strict refuses.
	assertCompatibilityAuthorizesUnbound(t, ctx, db, clock)
}

func assertCompatibilityAuthorizesUnbound(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	clock func() time.Time,
) {
	t.Helper()
	unbound := mustUnboundDestinationRevision(t)
	registry, err := destination.LoadRegistry(
		strings.NewReader(strictRegistryJSON(t, unbound)), destination.ModeCompatibility,
	)
	if err != nil {
		t.Fatalf("compatibility LoadRegistry: %v", err)
	}
	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	authorizer, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance,
		CompatibilitySubject: "fallback-client", Clock: clock,
	})
	if err != nil {
		t.Fatalf("compatibility NewAuthorizer: %v", err)
	}
	attemptID := "attempt-compat-" + fmt.Sprintf("%d", time.Now().UnixNano())
	seedIdentityDelivery(t, db, clock(), attemptID, "outbox-compat", unbound.Reference())
	store, err := NewPostgresStore(db, clock)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	item, err := store.Claim(ctx, "worker-compat", 30*time.Second)
	if err != nil || item == nil || item.AttemptID != attemptID {
		t.Fatalf("claim compatibility attempt item=%#v error=%v", item, err)
	}
	if err := authorizer.Decide(ctx, item.TenantID, item.AttemptID, item.Destination); err != nil {
		t.Fatalf("compatibility Decide: %v", err)
	}
	decision := readIdentityDecision(t, db, attemptID)
	if !decision.authorized || decision.mode != string(destination.ModeCompatibility) ||
		decision.subject != "fallback-client" ||
		decision.grant != authorization.DestinationCompatibilityGrant {
		t.Fatalf("compatibility decision = %#v", decision)
	}
	if _, err := destination.NewAuthorizer(destination.AuthorizerConfig{
		Registry: registry, Recorder: provenance, Clock: clock,
	}); err == nil {
		t.Fatal("compatibility mode started without a compatibility subject")
	}
}

func assertSentinelAbsent(t *testing.T, db *sql.DB, records []*kgo.Record, secretRoot string) {
	t.Helper()
	planted, err := os.ReadFile(filepath.Join(secretRoot, "alpha", "token"))
	if err != nil || string(planted) != destinationSecretSentinel {
		t.Fatalf("sentinel was never planted, so its absence proves nothing: %q %v", planted, err)
	}
	queries := []string{
		`SELECT result_json::text FROM integration_receipts`,
		`SELECT payload_json::text FROM integration_canonical_events`,
		`SELECT destination_revision_json::text FROM integration_delivery_attempts`,
		`SELECT payload_json::text FROM integration_delivery_outbox`,
		`SELECT detail_json::text || principal_json::text FROM integration_delivery_audit`,
		`SELECT failure_code || failure_detail FROM integration_delivery_dlq`,
		`SELECT principal_subject || granted_role || destination_endpoint_advisory || destination_digest_verified
		   FROM integration_delivery_identity_decisions`,
	}
	for _, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("read durable record class: %v", err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan durable record class: %v", err)
			}
			if strings.Contains(value, destinationSecretSentinel) {
				_ = rows.Close()
				t.Fatalf("durable record carries the secret sentinel: %s", value)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate durable record class: %v", err)
		}
		_ = rows.Close()
	}
	for _, record := range records {
		if strings.Contains(string(record.Key), destinationSecretSentinel) ||
			strings.Contains(string(record.Value), destinationSecretSentinel) {
			t.Fatal("a Kafka record carries the secret sentinel")
		}
		for _, header := range record.Headers {
			if strings.Contains(string(header.Value), destinationSecretSentinel) {
				t.Fatalf("Kafka header %s carries the secret sentinel", header.Key)
			}
		}
	}
}

func drainDispatcher(
	t *testing.T,
	ctx context.Context,
	dispatcher *Dispatcher,
	expected int,
) map[Outcome]int {
	t.Helper()
	outcomes := make(map[Outcome]int, expected)
	for step := 0; step < expected+1; step++ {
		outcome, err := dispatcher.RunOnce(ctx)
		if err != nil {
			t.Fatalf("RunOnce step %d: %v", step, err)
		}
		if outcome == OutcomeIdle {
			break
		}
		outcomes[outcome]++
	}
	return outcomes
}

type identityDecisionRow struct {
	authorized       bool
	mode             string
	subject          string
	authMethod       string
	grant            string
	digest           string
	denialCode       string
	endpointAdvisory string
	decidedAt        time.Time
}

func readIdentityDecision(t *testing.T, db *sql.DB, attemptID string) identityDecisionRow {
	t.Helper()
	var row identityDecisionRow
	var label string
	if err := db.QueryRow(`
		SELECT decision, identity_mode, principal_subject, principal_auth_method,
			granted_role, destination_digest_verified, denial_code,
			destination_endpoint_advisory, decided_at
		FROM integration_delivery_identity_decisions
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
		ORDER BY decision_id DESC
		LIMIT 1
	`, attemptID).Scan(
		&label, &row.mode, &row.subject, &row.authMethod, &row.grant,
		&row.digest, &row.denialCode, &row.endpointAdvisory, &row.decidedAt,
	); err != nil {
		t.Fatalf("read identity decision for %s: %v", attemptID, err)
	}
	row.authorized = label == "authorized"
	return row
}

func assertDLQFailureCode(t *testing.T, db *sql.DB, attemptID, wantCode string) {
	t.Helper()
	var code string
	if err := db.QueryRow(`
		SELECT failure_code FROM integration_delivery_dlq
		WHERE tenant_id = 'tenant-a' AND attempt_id = $1
	`, attemptID).Scan(&code); err != nil {
		t.Fatalf("read DLQ entry for %s: %v", attemptID, err)
	}
	if code != wantCode {
		t.Fatalf("DLQ failure code for %s = %q, want %q", attemptID, code, wantCode)
	}
}

func mustDestinationRevision(t *testing.T, artifactID, destinationID, subject, tokenBinding, url string) destination.Revision {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID:    artifactID,
		RevisionID:    "destination-1",
		DestinationID: destinationID,
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportHTTPS,
		HTTPS: &destination.HTTPSPolicy{
			URL: url, Method: "POST", TokenBinding: tokenBinding,
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

func mustUnboundDestinationRevision(t *testing.T) destination.Revision {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "queue-primary", RevisionID: "destination-1", DestinationID: "queue-primary",
		Class: integration.DestinationClassProduction, Transport: destination.TransportKafka,
		Kafka: &destination.KafkaPolicy{Topic: deliveryCommandSchema},
	})
	if err != nil {
		t.Fatalf("NewRevision(unbound): %v", err)
	}
	return revision
}

func mustRegistry(t *testing.T, document string, mode destination.Mode) *destination.Registry {
	t.Helper()
	registry, err := destination.LoadRegistry(strings.NewReader(document), mode)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return registry
}

func strictRegistryJSON(t *testing.T, revisions ...destination.Revision) string {
	t.Helper()
	bindings := []map[string]any{
		{"name": "alpha-token", "reference": map[string]string{"provider": "file", "key": "alpha/token"}},
		{"name": "beta-token", "reference": map[string]string{"provider": "file", "key": "beta/token"}},
	}
	document := map[string]any{
		"schema":    "fi-fhir/destination-registry/v1",
		"tenant_id": "tenant-a",
		"integration_revision": map[string]string{
			"artifact_id": "integration-adt", "revision_id": "revision-1",
			"digest": "sha256:" + strings.Repeat("b", 64),
		},
		"secret_bindings": bindings,
		"destinations":    revisions,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	return string(encoded)
}

func writeIdentitySecret(t *testing.T, root, key, value string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create secret directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
}

// seedIdentityDelivery writes one durable attempt and its outbox row for an
// exact destination reference, matching the shape the durable submission path
// commits.
func seedIdentityDelivery(
	t *testing.T,
	db *sql.DB,
	now time.Time,
	attemptID, outboxID string,
	reference integration.DestinationRevisionRef,
) {
	t.Helper()
	destinationJSON, err := json.Marshal(reference)
	if err != nil {
		t.Fatalf("marshal destination reference: %v", err)
	}
	payload := fmt.Sprintf(
		`{"schema":%q,"tenant_id":"tenant-a","receipt_id":"receipt-identity","event_id":"event-identity",`+
			`"trace_id":"trace-identity","attempt_id":%q,"destination":%s,"route":"matched",`+
			`"action":"send","attempt_count":1}`,
		deliveryCommandSchema, attemptID, destinationJSON)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, result_json
		) VALUES ('tenant-a', 'receipt-identity', 'key-identity', 'fingerprint-identity', '{}',
			'accepted', $1, 'correlation-identity', 'ephemeral', '{}', '{}')
		ON CONFLICT DO NOTHING
	`, now); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', 'event-identity', 'receipt-identity', 'patient_admit',
			'message-identity', 'correlation-identity', 'phi',
			'{"id":"event-identity","type":"patient_admit"}', $1)
		ON CONFLICT DO NOTHING
	`, now); err != nil {
		t.Fatalf("seed canonical event: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id,
			destination_revision_json, route_name, action_id, status,
			attempt_count, recorded_at, scheduled_at
		) VALUES ('tenant-a', $2, 'receipt-identity', 'event-identity', 'trace-identity', $3,
			'matched', 'send', 'queued', 1, $1, $1)
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
		t.Fatalf("commit seed: %v", err)
	}
}
