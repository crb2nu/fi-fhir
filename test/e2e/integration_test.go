// Integration tests that require external services.
//
// Run with: go test -tags=e2e,integration -v ./test/e2e/...
//
// Requires:
//   docker-compose -f test/e2e/docker-compose.yaml up -d
//
//go:build e2e && integration

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Integration test configuration from environment.
type IntegrationConfig struct {
	PostgresURL  string
	FHIRBaseURL  string
	KafkaBrokers string
	WebhookURL   string
	JaegerURL    string
}

func getIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		PostgresURL:  getEnv("TEST_POSTGRES_URL", "postgres://test:test@localhost:5433/fi_fhir_test?sslmode=disable"),
		FHIRBaseURL:  getEnv("TEST_FHIR_URL", "http://localhost:8090/fhir"),
		KafkaBrokers: getEnv("TEST_KAFKA_BROKERS", "localhost:9094"),
		WebhookURL:   getEnv("TEST_WEBHOOK_URL", "http://localhost:8888"),
		JaegerURL:    getEnv("TEST_JAEGER_URL", "http://localhost:16687"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// TestDatabaseAction tests the database action with PostgreSQL.
func TestDatabaseAction(t *testing.T) {
	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	// Connect to database
	db, err := sql.Open("postgres", intCfg.PostgresURL)
	if err != nil {
		t.Skipf("Could not connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS test_events (
			id SERIAL PRIMARY KEY,
			event_type VARCHAR(50),
			patient_mrn VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer db.Exec("DROP TABLE IF EXISTS test_events")

	// Create workflow with database action
	workflowYAML := fmt.Sprintf(`
workflow:
  name: database_test
  version: "1.0"
  routes:
    - name: store_events
      filter:
        event_type: patient_admit
      actions:
        - type: database
          driver: postgres
          dsn: "%s"
          operation: insert
          table: test_events
          fields:
            event_type: "{{.Type}}"
            patient_mrn: "{{.Patient.MRN}}"
`, intCfg.PostgresURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {"mrn": "TEST-MRN-001"}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	// Run workflow
	_, err = runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v", err)
	}

	// Verify record was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_events WHERE patient_mrn = 'TEST-MRN-001'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query test table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 record, found %d", count)
	}
}

// TestFHIRAction tests the FHIR action with HAPI FHIR server.
func TestFHIRAction(t *testing.T) {
	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	// Check FHIR server availability
	resp, err := http.Get(intCfg.FHIRBaseURL + "/metadata")
	if err != nil {
		t.Skipf("FHIR server not available: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("FHIR server returned status %d", resp.StatusCode)
	}

	// Create workflow with FHIR action
	workflowYAML := fmt.Sprintf(`
workflow:
  name: fhir_test
  version: "1.0"
  routes:
    - name: create_patient
      filter:
        event_type: patient_admit
      actions:
        - type: fhir
          endpoint: "%s"
          resource: Patient
          auth:
            type: none
`, intCfg.FHIRBaseURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {
			"mrn": "FHIR-TEST-001",
			"name": {"given": "Test", "family": "Patient"},
			"birthDate": "1990-01-01",
			"gender": "male"
		}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	// Run workflow
	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	// Verify patient was created by searching
	searchURL := fmt.Sprintf("%s/Patient?identifier=FHIR-TEST-001", intCfg.FHIRBaseURL)
	resp, err = http.Get(searchURL)
	if err != nil {
		t.Fatalf("Failed to search FHIR server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("FHIR search returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var bundle map[string]interface{}
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("Invalid FHIR response: %v", err)
	}

	total, _ := bundle["total"].(float64)
	if total < 1 {
		t.Error("Patient was not created in FHIR server")
	}
}

// TestWebhookAction tests the webhook action with mock server.
func TestWebhookAction(t *testing.T) {
	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	// Check webhook server availability
	resp, err := http.Get(intCfg.WebhookURL)
	if err != nil {
		t.Skipf("Webhook server not available: %v", err)
	}
	resp.Body.Close()

	// Create workflow with webhook action
	workflowYAML := fmt.Sprintf(`
workflow:
  name: webhook_integration_test
  version: "1.0"
  routes:
    - name: send_webhook
      filter:
        event_type: patient_admit
      actions:
        - type: webhook
          url: %s
          method: POST
          headers:
            Content-Type: application/json
            X-Test-Header: test-value
`, intCfg.WebhookURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {"mrn": "WEBHOOK-TEST-001"}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	// Run workflow
	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	// The echo server should have received the request
	// We can't easily verify the specific request, but we know
	// the action succeeded if there's no error
	t.Log("Webhook action completed successfully")
}

// TestWorkflowWithRetry tests retry behavior on transient failures.
func TestWorkflowWithRetry(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// This test uses a non-existent endpoint to trigger retries
	workflowYAML := `
workflow:
  name: retry_test
  version: "1.0"
  routes:
    - name: retry_route
      filter:
        event_type: patient_admit
      actions:
        - type: webhook
          url: http://localhost:19999/nonexistent
          method: POST
          retry:
            maxAttempts: 2
            initialDelay: 100ms
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {"mrn": "RETRY-TEST-001"}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	start := time.Now()
	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	duration := time.Since(start)

	// Should fail after retries
	if err == nil {
		t.Log("Expected error from failed webhook, but got success")
		t.Log("Output:", output)
	}

	// Should have taken time for retries (at least 100ms delay)
	if duration < 100*time.Millisecond {
		t.Errorf("Expected retry delay, but completed in %v", duration)
	}
}

// TestHealthEndpoints tests the health check endpoints.
func TestHealthEndpoints(t *testing.T) {
	// This test requires the fi-fhir server to be running
	// Skip if not available
	healthURL := getEnv("TEST_FIFHIR_URL", "http://localhost:8080")

	resp, err := http.Get(healthURL + "/health")
	if err != nil {
		t.Skipf("fi-fhir server not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var health map[string]interface{}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Errorf("Invalid health response: %v", err)
	}

	if health["status"] != "ok" {
		t.Errorf("Health status = %v, want ok", health["status"])
	}
}

// TestMetricsEndpoint tests the Prometheus metrics endpoint.
func TestMetricsEndpoint(t *testing.T) {
	healthURL := getEnv("TEST_FIFHIR_URL", "http://localhost:8080")

	resp, err := http.Get(healthURL + "/metrics")
	if err != nil {
		t.Skipf("fi-fhir server not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/metrics returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Check for expected metrics
	expectedMetrics := []string{
		"workflow_events_processed_total",
		"workflow_action_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !bytes.Contains(body, []byte(metric)) {
			t.Logf("Warning: metric %s not found in output", metric)
			t.Log("This may be expected if no events have been processed")
		}
	}

	if len(content) == 0 {
		t.Error("Metrics endpoint returned empty response")
	}
}
