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
// and nowhere else in the seeded database.
const (
	ledgerFHIREndpoint    = "https://fhir-ledger.example.test/r4"
	ledgerHTTPSEndpoint   = "https://https-ledger.example.test/ingest"
	ledgerCertSubject     = "CN=https-ledger.example.test"
	ledgerFHIRTypes       = "Patient,Encounter"
	ledgerTenantBEndpoint = "https://tenant-b.example.test/r4"
)

var ledgerDigest = "sha256:" + strings.Repeat("4", 64)

const deliverySelection = `deliveries {
	transport outcome failureCode httpStatusClass digestVerified
	destination { artifactId revisionId digest class }
	endpointAdvisory servedCertificateSubjectAdvisory completedAt
	fhirResourceTypes fhirEntryCount fhirOutcomeCodesAdvisory
}`

// TestOperatorControlPlane_TraceShowsTheFHIRDelivery is the Slice 4.2c gate.
//
// Its day-1 form (commit "test(operator): day-1 gate", PASS on main 1465aa516)
// proved the opposite: `deliveries` was not a field of OperatorDeliveryAttempt
// and no ledger value reached any response. Inverted, it proves that over the
// real GraphQL handler with a verified OIDC operator identity:
//
//   - the single-attempt read carries every ledger row for the attempt, newest
//     first, each column projected one to one — the fhir facts on the fhir
//     rows, empty fhir facts and populated status class, endpoint, and
//     certificate subject on the https row;
//   - the attempt list and the message trace carry the newest five;
//   - a replay's control result carries the ledger too;
//   - another tenant's row under the SAME attempt id never appears, although
//     it is the newest row in the table;
//   - the raw-PHI sentinel stored in the canonical event reaches no response.
func TestOperatorControlPlane_TraceShowsTheFHIRDelivery(t *testing.T) {
	ctx := t.Context()
	db := openOperatorDatabase(t, ctx)
	seededAt := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	client := newFHIRTraceClient(t, ctx, db, seededAt)

	readRoles := []string{graphqlapi.GraphQLOperatorRole, operator.ReadRole}
	completedAt := func(minutes int) string {
		return seededAt.Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
	}

	// ---------------------------------------------------------------------
	// Single attempt: all six of tenant-a's rows, newest first.
	// ---------------------------------------------------------------------
	attemptBody := client.query(readRoles, `
		query { operatorDeliveryAttempt(attemptId: "attempt-a") { attemptId `+deliverySelection+` } }`)
	requireGraphQLData(t, "operatorDeliveryAttempt", attemptBody)
	single := attemptBody.path("data", "operatorDeliveryAttempt", "deliveries").array()
	if len(single) != 6 {
		t.Fatalf("single-attempt deliveries = %d, want all 6 of tenant-a's rows: %s", len(single), attemptBody.raw)
	}
	requireNewestFirst(t, "operatorDeliveryAttempt", single)
	for index, minutes := range []int{6, 5, 4, 3, 2, 1} {
		if got := (jsonValue{single[index]}).path("completedAt").str(); got != completedAt(minutes) {
			t.Fatalf("delivery %d completedAt = %s, want %s", index, got, completedAt(minutes))
		}
	}

	delivered := jsonValue{single[0]}
	requireDelivery(t, "delivered fhir row", delivered, wantDelivery{
		transport: "fhir", outcome: "delivered", failureCode: "", statusClass: "2xx",
		artifactID: "destination-fhir", endpoint: ledgerFHIREndpoint, certSubject: "",
		resourceTypes: []string{"Patient", "Encounter"}, entryCount: 2, outcomeCodes: []string{},
	})
	refused := jsonValue{single[1]}
	requireDelivery(t, "refused fhir row", refused, wantDelivery{
		transport: "fhir", outcome: "refused", failureCode: destination.FailureRejected, statusClass: "4xx",
		artifactID: "destination-fhir", endpoint: ledgerFHIREndpoint, certSubject: "",
		resourceTypes: []string{"Patient", "Encounter"}, entryCount: 2, outcomeCodes: []string{"invalid", "not-found"},
	})
	https := jsonValue{single[2]}
	requireDelivery(t, "https row", https, wantDelivery{
		transport: "https", outcome: "refused", failureCode: destination.FailureRejected, statusClass: "4xx",
		artifactID: "destination-https", endpoint: ledgerHTTPSEndpoint, certSubject: ledgerCertSubject,
		resourceTypes: []string{}, entryCount: 0, outcomeCodes: []string{},
	})
	retried := jsonValue{single[5]}
	requireDelivery(t, "oldest retryable fhir row", retried, wantDelivery{
		transport: "fhir", outcome: "retryable", failureCode: destination.FailureUnavailable, statusClass: "5xx",
		artifactID: "destination-fhir", endpoint: ledgerFHIREndpoint, certSubject: "",
		resourceTypes: []string{"Patient", "Encounter"}, entryCount: 2, outcomeCodes: []string{},
	})
	if got := delivered.path("destination", "digest").str(); got != ledgerDigest {
		t.Fatalf("delivery destination.digest = %q, want the verified digest", got)
	}
	if got := delivered.path("digestVerified").str(); got != ledgerDigest {
		t.Fatalf("delivery digestVerified = %q, want %q", got, ledgerDigest)
	}
	if got := delivered.path("destination", "revisionId").str(); got != "destination-1" {
		t.Fatalf("delivery destination.revisionId = %q, want destination-1", got)
	}
	if got := delivered.path("destination", "class").str(); got != "production" {
		t.Fatalf("delivery destination.class = %q, want production", got)
	}

	// ---------------------------------------------------------------------
	// Attempt list and message trace: the newest five, same order.
	// ---------------------------------------------------------------------
	listBody := client.query(readRoles, `
		query { operatorDeliveryAttempts(page: {first: 50}) { nodes { attemptId `+deliverySelection+` } } }`)
	requireGraphQLData(t, "operatorDeliveryAttempts", listBody)
	nodes := listBody.path("data", "operatorDeliveryAttempts", "nodes").array()
	if len(nodes) != 1 || (jsonValue{nodes[0]}).path("attemptId").str() != "attempt-a" {
		t.Fatalf("attempt list = %s, want exactly tenant-a's attempt-a", listBody.raw)
	}
	listed := (jsonValue{nodes[0]}).path("deliveries").array()
	requireNewestFive(t, "operatorDeliveryAttempts", listed, completedAt)

	traceBody := client.query(readRoles, `
		query { operatorMessageTrace(receiptId: "receipt-a") { attempts { attemptId `+deliverySelection+` } } }`)
	requireGraphQLData(t, "operatorMessageTrace", traceBody)
	traceAttempts := traceBody.path("data", "operatorMessageTrace", "attempts").array()
	if len(traceAttempts) != 1 {
		t.Fatalf("trace attempts = %s, want 1", traceBody.raw)
	}
	traced := (jsonValue{traceAttempts[0]}).path("deliveries").array()
	requireNewestFive(t, "operatorMessageTrace", traced, completedAt)
	requireDelivery(t, "trace delivered fhir row", jsonValue{traced[0]}, wantDelivery{
		transport: "fhir", outcome: "delivered", failureCode: "", statusClass: "2xx",
		artifactID: "destination-fhir", endpoint: ledgerFHIREndpoint, certSubject: "",
		resourceTypes: []string{"Patient", "Encounter"}, entryCount: 2, outcomeCodes: []string{},
	})

	// ---------------------------------------------------------------------
	// A control result's attempt is the single-attempt read.
	// ---------------------------------------------------------------------
	replayBody := client.query(
		[]string{graphqlapi.GraphQLOperatorRole, operator.ReadRole, delivery.OperatorRole}, `
		mutation {
			replayDelivery(input: {
				attemptId: "attempt-a",
				reason: "FHIR destination fixed its validation profile",
				idempotencyKey: "trace-replay-1"
			}) { attempt { attemptId status `+deliverySelection+` } }
		}`)
	requireGraphQLData(t, "replayDelivery", replayBody)
	replayed := replayBody.path("data", "replayDelivery", "attempt", "deliveries").array()
	if len(replayed) != 6 {
		t.Fatalf("replay result deliveries = %d, want 6: %s", len(replayed), replayBody.raw)
	}

	// ---------------------------------------------------------------------
	// Tenant scope and PHI posture, across every response above.
	// ---------------------------------------------------------------------
	for _, body := range []graphQLResponse{attemptBody, listBody, traceBody, replayBody} {
		if strings.Contains(body.raw, ledgerTenantBEndpoint) || strings.Contains(body.raw, "destination-fhir-b") {
			t.Fatalf("another tenant's ledger row reached tenant-a: %s", body.raw)
		}
	}
	if leaked := client.bodies.leaks(rawPHISentinel); leaked != "" {
		t.Fatalf("a GraphQL response exposed the raw-PHI sentinel: %s", leaked)
	}
	if client.bodies.count() != 4 {
		t.Fatalf("sentinel scan covered %d responses, want 4", client.bodies.count())
	}
}

type wantDelivery struct {
	transport, outcome, failureCode, statusClass string
	artifactID, endpoint, certSubject            string
	resourceTypes                                []string
	entryCount                                   int
	outcomeCodes                                 []string
}

func requireDelivery(t *testing.T, name string, got jsonValue, want wantDelivery) {
	t.Helper()
	fields := map[string][2]string{
		"transport":                        {got.path("transport").str(), want.transport},
		"outcome":                          {got.path("outcome").str(), want.outcome},
		"failureCode":                      {got.path("failureCode").str(), want.failureCode},
		"httpStatusClass":                  {got.path("httpStatusClass").str(), want.statusClass},
		"destination.artifactId":           {got.path("destination", "artifactId").str(), want.artifactID},
		"endpointAdvisory":                 {got.path("endpointAdvisory").str(), want.endpoint},
		"servedCertificateSubjectAdvisory": {got.path("servedCertificateSubjectAdvisory").str(), want.certSubject},
		"fhirResourceTypes":                {strings.Join(jsonStrings(got.path("fhirResourceTypes")), ","), strings.Join(want.resourceTypes, ",")},
		"fhirOutcomeCodesAdvisory":         {strings.Join(jsonStrings(got.path("fhirOutcomeCodesAdvisory")), ","), strings.Join(want.outcomeCodes, ",")},
	}
	for field, pair := range fields {
		if pair[0] != pair[1] {
			t.Fatalf("%s %s = %q, want %q", name, field, pair[0], pair[1])
		}
	}
	// A non-null list, never null, even when empty.
	for _, field := range []string{"fhirResourceTypes", "fhirOutcomeCodesAdvisory"} {
		if got.path(field).value == nil {
			t.Fatalf("%s %s is null, want a list", name, field)
		}
	}
	if entries := int(got.path("fhirEntryCount").number()); entries != want.entryCount {
		t.Fatalf("%s fhirEntryCount = %d, want %d", name, entries, want.entryCount)
	}
}

func requireNewestFive(t *testing.T, name string, deliveries []any, completedAt func(int) string) {
	t.Helper()
	if len(deliveries) != operator.MaxListedAttemptDeliveries {
		t.Fatalf("%s deliveries = %d, want the newest %d", name, len(deliveries), operator.MaxListedAttemptDeliveries)
	}
	requireNewestFirst(t, name, deliveries)
	if got := (jsonValue{deliveries[0]}).path("completedAt").str(); got != completedAt(6) {
		t.Fatalf("%s newest delivery completedAt = %s, want %s", name, got, completedAt(6))
	}
	if got := (jsonValue{deliveries[4]}).path("completedAt").str(); got != completedAt(2) {
		t.Fatalf("%s fifth delivery completedAt = %s, want %s (the oldest row is past the bound)", name, got, completedAt(2))
	}
}

func requireNewestFirst(t *testing.T, name string, deliveries []any) {
	t.Helper()
	for index := 1; index < len(deliveries); index++ {
		previous := (jsonValue{deliveries[index-1]}).path("completedAt").str()
		current := (jsonValue{deliveries[index]}).path("completedAt").str()
		if current >= previous {
			t.Fatalf("%s deliveries are not newest first: %s then %s", name, previous, current)
		}
	}
}

func requireGraphQLData(t *testing.T, name string, body graphQLResponse) {
	t.Helper()
	if body.status != 200 || body.decoded["errors"] != nil || body.decoded["data"] == nil {
		t.Fatalf("%s failed: %s", name, body.raw)
	}
}

func jsonStrings(value jsonValue) []string {
	values := make([]string, 0)
	for _, item := range value.array() {
		text, _ := item.(string)
		values = append(values, text)
	}
	return values
}

// newFHIRTraceClient migrates the submission and destination schemas, seeds
// tenant-a's failed attempt-a (and tenant-b's attempt-b), records six
// provenance-ledger rows for attempt-a through the production writer plus one
// newer tenant-b row under the SAME attempt id, and returns a GraphQL client
// over the real handler with a verified OIDC operator identity.
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

	at := func(minutes int) time.Time { return seededAt.Add(time.Duration(minutes) * time.Minute) }
	fhirRow := func(minutes int, outcome, failureCode, statusClass, codes string) destination.DeliveryRecord {
		return destination.DeliveryRecord{
			TenantID: operatorTenant, AttemptID: "attempt-a", Transport: destination.TransportFHIR,
			DestinationArtifactID: "destination-fhir", DestinationRevisionID: "destination-1",
			DestinationClass: "production", DestinationDigestVerified: ledgerDigest,
			Outcome: outcome, FailureCode: failureCode, HTTPStatusClass: statusClass,
			EndpointAdvisory: ledgerFHIREndpoint, CompletedAt: at(minutes),
			FHIRResourceTypes: ledgerFHIRTypes, FHIREntryCount: 2, FHIROutcomeCodesAdvisory: codes,
		}
	}
	records := []destination.DeliveryRecord{
		// Oldest: the default retry budget against a sick FHIR server.
		fhirRow(1, "retryable", destination.FailureUnavailable, "5xx", ""),
		fhirRow(2, "retryable", destination.FailureUnavailable, "5xx", ""),
		fhirRow(3, "retryable", destination.FailureUnavailable, "5xx", ""),
		{ // an https exchange the destination refused
			TenantID: operatorTenant, AttemptID: "attempt-a", Transport: destination.TransportHTTPS,
			DestinationArtifactID: "destination-https", DestinationRevisionID: "destination-1",
			DestinationClass: "production", DestinationDigestVerified: ledgerDigest,
			Outcome: "refused", FailureCode: destination.FailureRejected, HTTPStatusClass: "4xx",
			EndpointAdvisory: ledgerHTTPSEndpoint, ServedCertificateSubjectAdvisory: ledgerCertSubject,
			CompletedAt: at(4),
		},
		// A transaction refused with two OperationOutcome issue codes.
		fhirRow(5, "refused", destination.FailureRejected, "4xx", "invalid,not-found"),
		// Newest of tenant-a's: the transaction delivered.
		fhirRow(6, "delivered", "", "2xx", ""),
		{ // another tenant's row under the same attempt id, and the newest row overall
			TenantID: "tenant-b", AttemptID: "attempt-a", Transport: destination.TransportFHIR,
			DestinationArtifactID: "destination-fhir-b", DestinationRevisionID: "destination-9",
			DestinationClass: "production", DestinationDigestVerified: ledgerDigest,
			Outcome: "delivered", HTTPStatusClass: "2xx",
			EndpointAdvisory: ledgerTenantBEndpoint, CompletedAt: at(7),
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
