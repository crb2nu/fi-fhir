//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	operatorplane "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

// This uses a fresh database so existing runtime tables cannot conceal a missing
// startup migration. CI provides FI_FHIR_DATABASE_URL; a local disposable server
// can instead be selected with FI_FHIR_OPERATOR_TEST_DATABASE_URL.
func TestIntegrationOperatorRuntimeWithoutIngress(t *testing.T) {
	dsn := os.Getenv("FI_FHIR_OPERATOR_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("FI_FHIR_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set FI_FHIR_OPERATOR_TEST_DATABASE_URL or FI_FHIR_DATABASE_URL to a test PostgreSQL server")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil {
		t.Fatal("operator test database configuration must be a PostgreSQL URL with host and user")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("open test PostgreSQL connection")
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.PingContext(ctx); err != nil {
		t.Fatal("configured test PostgreSQL server is unreachable")
	}
	databaseName := fmt.Sprintf("operator_runtime_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(databaseName)); err != nil {
		t.Fatalf("create isolated operator test database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+pq.QuoteIdentifier(databaseName)); err != nil {
			t.Errorf("drop isolated operator test database: %v", err)
		}
	})

	configureOperatorRuntimeTest(t)
	const serviceToken = "operator-runtime-service-test-token-only"
	serviceTokenPath := filepath.Join(t.TempDir(), "service-bearer")
	if err := os.WriteFile(serviceTokenPath, []byte(serviceToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FI_FHIR_GRAPHQL_SERVICE_BEARER_TOKEN_FILE", serviceTokenPath)
	t.Setenv("FI_FHIR_GRAPHQL_SERVICE_PRINCIPAL_ID", "mentatlab-test")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview,graphql:operator,clinical:read")
	t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", "192.168.50.0/24")
	t.Setenv("FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("FI_FHIR_DATABASE_DRIVER", "postgres")
	t.Setenv("FI_FHIR_DATABASE_HOST", u.Hostname())
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	t.Setenv("FI_FHIR_DATABASE_PORT", port)
	t.Setenv("FI_FHIR_DATABASE_NAME", databaseName)
	t.Setenv("FI_FHIR_DATABASE_USERNAME", u.User.Username())
	password, _ := u.User.Password()
	t.Setenv("FI_FHIR_DATABASE_PASSWORD", password)
	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "require"
	}
	t.Setenv("FI_FHIR_DATABASE_SSL_MODE", sslMode)

	runtime, err := loadServeIntegrationRuntimeFromEnv(ctx)
	if err != nil {
		t.Fatalf("load operator-only runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if runtime.submissionDB == nil {
		t.Fatal("enabled operator runtime has no database")
	}
	assertOperatorRuntimeHasNoWorkers(t, runtime)
	var version int
	if err := runtime.submissionDB.QueryRowContext(ctx, "SELECT max(version) FROM integration_submission_schema_migrations").Scan(&version); err != nil || version != processor.SchemaVersion {
		t.Fatalf("submission migration version = %d, error = %v", version, err)
	}
	var sessionTable sql.NullString
	if err := runtime.submissionDB.QueryRowContext(ctx, "SELECT to_regclass('integration_sessions')::text").Scan(&sessionTable); err != nil || sessionTable.Valid {
		t.Fatalf("unexpected session schema: present=%t, error=%v", sessionTable.Valid, err)
	}
	reads, err := operatorplane.NewPostgresReadStore(runtime.submissionDB)
	if err != nil {
		t.Fatal(err)
	}
	page := operatorplane.PageRequest{First: 5}
	if _, err := reads.ListReceipts(ctx, runtime.tenantID, operatorplane.ReceiptFilter{}, page); err != nil {
		t.Fatalf("operator receipts on newly migrated database: %v", err)
	}
	if _, err := reads.ListAttempts(ctx, runtime.tenantID, operatorplane.AttemptFilter{}, page); err != nil {
		t.Fatalf("operator attempts on newly migrated database: %v", err)
	}
	if _, err := reads.ListDeadLetters(ctx, runtime.tenantID, true, page); err != nil {
		t.Fatalf("operator dead letters on newly migrated database: %v", err)
	}
	if _, err := reads.ListCircuits(ctx, runtime.tenantID); err != nil {
		t.Fatalf("operator circuits on newly migrated database: %v", err)
	}
	if _, err := reads.ListAttemptAudit(ctx, runtime.tenantID, "synthetic-attempt", page); err != nil {
		t.Fatalf("operator audit on newly migrated database: %v", err)
	}

	// serve's existing control-plane wiring migrates this second ledger after
	// loading the runtime; neither catalog construction nor migration starts work.
	catalog, err := lifecycle.NewPostgresCatalog(runtime.submissionDB, lifecycle.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatalf("operator lifecycle migrations: %v", err)
	}
	if snapshots, err := catalog.ListSnapshots(ctx, runtime.tenantID, 5); err != nil || len(snapshots) != 0 {
		t.Fatalf("empty operator lifecycle inventory: count=%d, error=%v", len(snapshots), err)
	}
	if ids := serveDurableDefinitionIDs(runtime); len(ids) != 0 {
		t.Fatalf("operator runtime unexpectedly enabled lifecycle health writes for %v", ids)
	}

	// Match serve's complete operator wiring, including the destination ledger.
	ledger, err := destination.NewPostgresProvenance(runtime.submissionDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	recovery, err := delivery.NewPostgresStore(runtime.submissionDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	controlPlane, err := operatorplane.NewService(reads, ledger, recovery, catalog, runtime.tenantID)
	if err != nil {
		t.Fatal(err)
	}
	config := graphqlapi.DefaultServerConfig()
	config.PlaygroundEnabled = false
	config.Introspection = false
	config.AllowedOrigins = runtime.allowedOrigins
	config.Authenticator = runtime.authenticator
	config.TrustedNetworkAuthenticator = runtime.trustedNetwork
	server, err := graphqlapi.NewServer(resolvers.NewResolver(
		resolvers.WithPreviewService(runtime.previewService),
		resolvers.WithOperatorControlPlane(controlPlane),
	), config)
	if err != nil {
		t.Fatal(err)
	}

	// A real record in another tenant must remain absent from the aggregate view.
	if _, err := runtime.submissionDB.ExecContext(ctx, `
		INSERT INTO integration_delivery_circuits
		(tenant_id, destination_artifact_id, destination_revision_id, destination_digest,
		 state, consecutive_failures, updated_at)
		VALUES ('tenant-b', 'synthetic-destination', '1', $1, 'closed', 7, now())
	`, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	post := func(token, query string, variables map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Origin", "http://localhost:5173")
		request.Header.Set("X-Real-IP", "192.168.50.24")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if strings.Contains(response.Body.String(), serviceToken) || strings.Contains(response.Body.String(), "correct-horse-battery-staple") {
			t.Fatal("GraphQL response exposed a bearer credential")
		}
		return response
	}
	readData := func(response *httptest.ResponseRecorder) map[string]json.RawMessage {
		t.Helper()
		var result struct {
			Data   map[string]json.RawMessage `json:"data"`
			Errors []json.RawMessage          `json:"errors"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || len(result.Errors) != 0 {
			t.Fatalf("GraphQL status=%d body=%s decode=%v", response.Code, response.Body.String(), err)
		}
		return result.Data
	}
	// Keep the selection in sync with MentatLab clinical/queries.go: these are
	// the four aggregate surfaces the gateway actually requests.
	const overviewQuery = `query MentatLabClinicalOverview {
	  operatorDeployments { state health validationPassed }
	  operatorCircuits { state consecutiveFailures }
	  operatorDeliveryAttempts(page: {first: 100}) { nodes { status } pageInfo { hasNextPage } }
	  operatorDeadLetters(activeOnly: true, page: {first: 100}) { nodes { active } pageInfo { hasNextPage } }
	}`
	data := readData(post(serviceToken, overviewQuery, nil))
	if len(data) != 4 || string(data["operatorDeployments"]) != "[]" || string(data["operatorCircuits"]) != "[]" {
		t.Fatalf("operator overview leaked another tenant or omitted a surface: %v", data)
	}
	for _, field := range []string{"operatorDeliveryAttempts", "operatorDeadLetters"} {
		var page struct {
			Nodes    []json.RawMessage `json:"nodes"`
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		}
		if err := json.Unmarshal(data[field], &page); err != nil || len(page.Nodes) != 0 || page.PageInfo.HasNextPage {
			t.Fatalf("%s did not return an empty valid page", field)
		}
	}
	const previewQuery = `mutation MentatLabSyntheticPreview($input: PreviewIntegrationMessageInput!) {
	  previewIntegrationMessage(input: $input) {
	    mode events { type } diagnostics { severity } routes { matched skipped } deliveries { status }
	  }
	}`
	const syntheticHL7 = "MSH|^~\\&|MENTATLAB_SYNTHETIC|TEST_FACILITY|FI_FHIR|TEST_FACILITY|20260101090000||ADT^A01|SYNTHETIC-MENTATLAB-001|T|2.5.1\r" +
		"EVN|A01|20260101090000\r" +
		"PID|1||SYNTHETIC-0001^^^MENTATLAB^MR||SYNTHETIC^PATIENT||20000101|U\r" +
		"PV1|1|I|TEST_WARD^TEST_ROOM^TEST_BED\r"
	previewData := readData(post(serviceToken, previewQuery, map[string]any{"input": map[string]any{
		"integrationId": "adt-east", "data": syntheticHL7, "correlationId": "operator-preview-test",
		"reason": "MentatLab synthetic integration verification; no patient data or delivery.",
	}}))
	var previewResult struct {
		Mode   string `json:"mode"`
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
	}
	if err := json.Unmarshal(previewData["previewIntegrationMessage"], &previewResult); err != nil ||
		previewResult.Mode != "preview" || len(previewResult.Events) != 1 || previewResult.Events[0].Type != "patient_admit" {
		t.Fatalf("synthetic preview did not produce one preview-only admission: %s", previewData["previewIntegrationMessage"])
	}

	const clinicalQuery = `query { events(first: 1) { edges { node { id } } } }`
	for name, query := range map[string]string{
		"clinical reads": clinicalQuery,
		"replay":         `mutation { replayDelivery(input: {attemptId:"synthetic", reason:"forbidden test", idempotencyKey:"test"}) { kind } }`,
		"deploy":         `mutation { deployIntegrationRelease(input: {definitionId:"synthetic", revisionId:"1", expectedVersion:1, reason:"forbidden test"}) { state } }`,
	} {
		response := post(serviceToken, query, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":"FORBIDDEN"`) {
			t.Fatalf("service %s status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
	readData(post("correct-horse-battery-staple", clinicalQuery, nil))
	legacyOverview := post("correct-horse-battery-staple", overviewQuery, nil)
	if legacyOverview.Code != http.StatusOK || !strings.Contains(legacyOverview.Body.String(), `"message":"operator control-plane action forbidden"`) {
		t.Fatalf("legacy identity inherited the service's operator role: status=%d body=%s", legacyOverview.Code, legacyOverview.Body.String())
	}
	if unknown := post("unknown-bearer", overviewQuery, nil); unknown.Code != http.StatusUnauthorized {
		t.Fatalf("unknown bearer status=%d, want401", unknown.Code)
	}
	for _, table := range []string{"integration_receipts", "integration_delivery_outbox", "integration_delivery_operations", "integration_lifecycle_events"} {
		var count int
		if err := runtime.submissionDB.QueryRowContext(ctx, "SELECT count(*) FROM "+pq.QuoteIdentifier(table)).Scan(&count); err != nil || count != 0 {
			t.Fatalf("preview or forbidden operation persisted data in %s: count=%d error=%v", table, count, err)
		}
	}
}
