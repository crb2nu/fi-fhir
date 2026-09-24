//go:build integration

package graphql_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

// Ledger-only sentinels. Each value exists in integration_destination_deliveries
// and nowhere else in the seeded database, so its presence in a response body
// can only mean the control plane projected the ledger.
const (
	ledgerFHIREndpoint  = "https://fhir-ledger.example.test/r4"
	ledgerHTTPSEndpoint = "https://https-ledger.example.test/ingest"
	ledgerCertSubject   = "CN=https-ledger.example.test"
	ledgerFHIRTypes     = "Patient,Encounter"
)

// TestOperatorControlPlane_TraceShowsTheFHIRDelivery is the Slice 4.2c gate.
//
// Day-1 form (unmodified main): with a fhir row and an https row recorded in
// the destination provenance ledger for attempt-a, the operator control plane's
// attempt, attempt-list, and trace projections carry no field derived from
// that ledger — `deliveries` is not a field of OperatorDeliveryAttempt, and no
// ledger-only value appears in any response body.
func TestOperatorControlPlane_TraceShowsTheFHIRDelivery(t *testing.T) {
	ctx := t.Context()
	db := openOperatorDatabase(t, ctx)
	seededAt := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	client := newFHIRTraceClient(t, ctx, db, seededAt)

	roles := []string{graphqlapi.GraphQLOperatorRole, operator.ReadRole}

	// The field does not exist on main: the server masks validation messages,
	// so the proof is that adding `deliveries` to an otherwise valid document
	// is exactly what makes it invalid.
	controlBody := client.query(roles, `
		query { operatorDeliveryAttempt(attemptId: "attempt-a") { attemptId } }`)
	if controlBody.status != 200 || controlBody.decoded["errors"] != nil {
		t.Fatalf("control document failed: %s", controlBody.raw)
	}
	fieldBody := client.query(roles, `
		query { operatorDeliveryAttempt(attemptId: "attempt-a") { attemptId deliveries { transport } } }`)
	if !fieldBody.hasErrorCode("GRAPHQL_VALIDATION_FAILED") {
		t.Fatalf("deliveries is already a field of OperatorDeliveryAttempt: %s", fieldBody.raw)
	}

	// Every existing projection of the attempt, and no ledger value in any.
	attemptSelection := `
		attemptId parentAttemptId receiptId eventId traceId
		destination { artifactId revisionId digest class }
		route action status attemptCount recordedAt scheduledAt completedAt
		lastErrorCode lastErrorDetail outboxStatus topic leaseOwner leaseExpiresAt
		deadLetter { attemptId active failureCode failureDetail replayCount resolution }`
	documents := map[string]string{
		"operatorDeliveryAttempt": `query { operatorDeliveryAttempt(attemptId: "attempt-a") {` + attemptSelection + `} }`,
		"operatorDeliveryAttempts": `query { operatorDeliveryAttempts(page: {first: 50}) { nodes {` +
			attemptSelection + `} } }`,
		"operatorMessageTrace": `query { operatorMessageTrace(receiptId: "receipt-a") { attempts {` +
			attemptSelection + `} } }`,
	}
	for name, document := range documents {
		body := client.query(roles, document)
		if body.status != 200 || body.decoded["errors"] != nil {
			t.Fatalf("%s failed: %s", name, body.raw)
		}
		if !strings.Contains(body.raw, `"attempt-a"`) {
			t.Fatalf("%s did not return attempt-a: %s", name, body.raw)
		}
		for _, sentinel := range []string{ledgerFHIREndpoint, ledgerHTTPSEndpoint, ledgerCertSubject, "Encounter", "not-found"} {
			if strings.Contains(body.raw, sentinel) {
				t.Fatalf("%s exposed ledger value %q on main: %s", name, sentinel, body.raw)
			}
		}
	}
}

// newFHIRTraceClient migrates the submission and destination schemas, seeds
// tenant-a's failed attempt-a (and tenant-b's attempt-b), records three
// provenance-ledger rows for attempt-a plus one tenant-b row under the SAME
// attempt id, and returns a GraphQL client over the real handler with a
// verified OIDC operator identity.
func newFHIRTraceClient(t *testing.T, ctx context.Context, db *sql.DB, seededAt time.Time) *operatorClient {
	t.Helper()
	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submissionStore.Migrate(ctx); err != nil {
		t.Fatalf("migrate submission schema: %v", err)
	}
	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("migrate destination ledger: %v", err)
	}
	seedFailedDelivery(t, db, operatorTenant, "receipt-a", "attempt-a", "outbox-a", seededAt)
	seedFailedDelivery(t, db, "tenant-b", "receipt-b", "attempt-b", "outbox-b", seededAt)

	digest := "sha256:" + strings.Repeat("4", 64)
	records := []destination.DeliveryRecord{
		{ // oldest: an https exchange the destination refused
			TenantID: operatorTenant, AttemptID: "attempt-a", Transport: destination.TransportHTTPS,
			DestinationArtifactID: "destination-https", DestinationRevisionID: "destination-1",
			DestinationClass: "production", DestinationDigestVerified: digest,
			Outcome: "refused", FailureCode: destination.FailureRejected, HTTPStatusClass: "4xx",
			EndpointAdvisory: ledgerHTTPSEndpoint, ServedCertificateSubjectAdvisory: ledgerCertSubject,
			CompletedAt: seededAt.Add(1 * time.Minute),
		},
		{ // a fhir transaction the destination refused with two issue codes
			TenantID: operatorTenant, AttemptID: "attempt-a", Transport: destination.TransportFHIR,
			DestinationArtifactID: "destination-fhir", DestinationRevisionID: "destination-1",
			DestinationClass: "production", DestinationDigestVerified: digest,
			Outcome: "refused", FailureCode: destination.FailureRejected, HTTPStatusClass: "4xx",
			EndpointAdvisory: ledgerFHIREndpoint, CompletedAt: seededAt.Add(2 * time.Minute),
			FHIRResourceTypes: ledgerFHIRTypes, FHIREntryCount: 2, FHIROutcomeCodesAdvisory: "invalid,not-found",
		},
		{ // newest: the fhir transaction delivered
			TenantID: operatorTenant, AttemptID: "attempt-a", Transport: destination.TransportFHIR,
			DestinationArtifactID: "destination-fhir", DestinationRevisionID: "destination-1",
			DestinationClass: "production", DestinationDigestVerified: digest,
			Outcome: "delivered", HTTPStatusClass: "2xx",
			EndpointAdvisory: ledgerFHIREndpoint, CompletedAt: seededAt.Add(3 * time.Minute),
			FHIRResourceTypes: ledgerFHIRTypes, FHIREntryCount: 2,
		},
		{ // another tenant's row under the same attempt id: never tenant-a's
			TenantID: "tenant-b", AttemptID: "attempt-a", Transport: destination.TransportFHIR,
			DestinationArtifactID: "destination-fhir-b", DestinationRevisionID: "destination-9",
			DestinationClass: "production", DestinationDigestVerified: digest,
			Outcome: "delivered", HTTPStatusClass: "2xx",
			EndpointAdvisory: "https://tenant-b.example.test/r4", CompletedAt: seededAt.Add(4 * time.Minute),
			FHIRResourceTypes: "Patient", FHIREntryCount: 1,
		},
	}
	for index, record := range records {
		if err := provenance.RecordDelivery(ctx, record); err != nil {
			t.Fatalf("RecordDelivery(%d): %v", index, err)
		}
	}

	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{})
	if err != nil {
		t.Fatalf("NewPostgresCatalog: %v", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatalf("migrate lifecycle catalog: %v", err)
	}
	reads, err := operator.NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}
	recovery, err := delivery.NewPostgresStore(db, nil)
	if err != nil {
		t.Fatalf("delivery.NewPostgresStore: %v", err)
	}
	controlPlane, err := operator.NewService(reads, provenance, recovery, catalog, operatorTenant)
	if err != nil {
		t.Fatalf("operator.NewService: %v", err)
	}

	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  operatorTenant,
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	config := graphqlapi.DefaultServerConfig()
	config.PlaygroundEnabled = false
	config.AllowedOrigins = []string{"https://ide.example.test"}
	config.MaxRequestBodyBytes = 16 * 1024
	config.Authenticator = authenticator
	server, err := graphqlapi.NewServer(
		resolvers.NewResolver(resolvers.WithOperatorControlPlane(controlPlane)),
		config,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return &operatorClient{
		t:       t,
		handler: server.Handler(),
		path:    config.Path,
		issuer:  issuer,
		bodies:  &responseRecorder{},
	}
}
