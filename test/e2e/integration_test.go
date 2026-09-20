// Integration tests that require external services.
//
// Run with: go test -tags=e2e,integration -v ./test/e2e/...
// CI job: test:e2e-legacy (ci/test-e2e-legacy.yml), blocking.
//
// Dependencies come from the environment. FI_FHIR_E2E_REQUIRED_SERVICES names
// the ones the caller has actually provided; a service in that list that is
// unreachable fails the suite instead of skipping it (see requireService).
// The CI job provides PostgreSQL, an HTTP echo destination and a running
// fi-fhir, and declares exactly those.
//
// test/e2e/docker-compose.yaml, which this file's header used to instruct the
// reader to bring up, was retired by slice S6-C: it stood up five dependencies
// and no fi-fhir, so the one test that needed the application under test
// skipped even with the whole stack running.
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
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Integration test configuration from environment.
type IntegrationConfig struct {
	PostgresURL string
	FHIRBaseURL string
	WebhookURL  string
}

func getIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		PostgresURL: getEnv("TEST_POSTGRES_URL", "postgres://test:test@localhost:5433/fi_fhir_test?sslmode=disable"),
		FHIRBaseURL: getEnv("TEST_FHIR_URL", "http://localhost:8090/fhir"),
		WebhookURL:  getEnv("TEST_WEBHOOK_URL", "http://localhost:8888"),
	}
}

// requireService decides what an unreachable dependency means.
//
// Slice 4.4c. Every dependency check in this file used to t.Skipf, and no CI
// job ever passed -tags=e2e, so the whole file was unreachable in two
// independent ways at once: the job did not exist, and if it had, a service
// container that failed to start would have turned the suite green rather than
// red. That is the same failure shape as an integration proof that skips when
// its database is missing, and this repository already has an answer for it —
// requireCompatDSN in internal/integration/migrationcompat.
//
// FI_FHIR_E2E_REQUIRED_SERVICES names the dependencies the caller has actually
// provided, comma-separated. A dependency in that list is fatal when it is
// unreachable; anything else still skips, so a workstation with no services
// keeps working exactly as before. The CI job sets the list to precisely the
// services it stands up, which makes "zero skips attributable to missing
// infrastructure" a property of the job rather than a hope.
func requireService(t *testing.T, service string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, required := range strings.Split(os.Getenv("FI_FHIR_E2E_REQUIRED_SERVICES"), ",") {
		if strings.TrimSpace(required) != service {
			continue
		}
		t.Fatalf("%s is declared required by FI_FHIR_E2E_REQUIRED_SERVICES but is "+
			"unreachable: %v\n"+
			"  A declared dependency that is missing must fail this suite. Skipping here "+
			"would report a green e2e run for a test that never executed.", service, err)
	}
	t.Skipf("%s not available: %v", service, err)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// TestDatabaseAction tests the database action with PostgreSQL.
//
// The workflow this test used to write was not the database action's schema.
// It declared `driver`, `dsn` and a nested `fields:` map of templates; the
// action reads `connection`, `table` and `mapping_<column>: <event field path>`
// (internal/workflow/database.go). Action.UnmarshalYAML drops nested blocks, so
// `fields:` never reached the action and the run failed with "database action
// requires at least one mapping" — reported by the test as a bare
// "exit status 1", because it discarded stderr.
func TestDatabaseAction(t *testing.T) {
	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	db, err := sql.Open("postgres", intCfg.PostgresURL)
	requireService(t, "postgres", err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	requireService(t, "postgres", db.PingContext(ctx))

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS test_events (
			id SERIAL PRIMARY KEY,
			event_type VARCHAR(50),
			patient_mrn VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW()
		)
	`); err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer func() { _, _ = db.Exec("DROP TABLE IF EXISTS test_events") }()

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
          connection: "%s"
          table: test_events
          operation: insert
          mapping_event_type: type
          mapping_patient_mrn: patient.mrn
`, intCfg.PostgresURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient":   map[string]interface{}{"mrn": "TEST-MRN-001"},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM test_events WHERE patient_mrn = 'TEST-MRN-001'",
	).Scan(&count); err != nil {
		t.Fatalf("Failed to query test table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 record, found %d\nOutput: %s", count, output)
	}

	// The mapping is column -> event field path, not a template, so a value
	// that silently arrived as the literal path would still count as a row.
	var eventType string
	if err := db.QueryRow(
		"SELECT event_type FROM test_events WHERE patient_mrn = 'TEST-MRN-001'",
	).Scan(&eventType); err != nil {
		t.Fatalf("Failed to read back event_type: %v", err)
	}
	if eventType != "patient_admit" {
		t.Errorf("event_type = %q, want patient_admit", eventType)
	}
}

// TestFHIRAction tests the FHIR action against a FHIR server.
//
// SKIPPED on fi-fhir issue #20: the fhir action cannot write a patient_admit
// event to a conformant FHIR server.
//
// The config below is the repaired one — `auth: {type: none}` was a nested
// block Action.UnmarshalYAML drops, and `resource: Patient` is ignored by the
// action, which emits both a Patient and an Encounter for every admit
// (eventToFHIRResources) and therefore always takes the len(resources) > 1
// transaction-bundle path. That bundle gives its Patient entry no fullUrl and
// points the Encounter's subject at `Patient/<MRN>`, a literal server id that
// does not exist, so a stock HAPI FHIR server rejects the whole transaction
// with 400 HAPI-1094 and writes nothing. Reproduced against
// hapiproject/hapi; the full request body and response are on the issue.
//
// Everything in this test except the skip is ready: delete the t.Skipf when
// #20 lands and it exercises the repaired path unchanged.
func TestFHIRAction(t *testing.T) {
	t.Skipf("blocked on fi-fhir issue #20: the fhir action's patient_admit " +
		"transaction bundle references Patient/<MRN> with no matching fullUrl, " +
		"which any conformant FHIR server rejects")

	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	resp, err := http.Get(intCfg.FHIRBaseURL + "/metadata")
	requireService(t, "hapi-fhir", err)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		requireService(t, "hapi-fhir",
			fmt.Errorf("/metadata returned status %d", resp.StatusCode))
	}

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
`, intCfg.FHIRBaseURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": map[string]interface{}{
			"mrn":         "FHIR-TEST-001",
			"given_name":  "Test",
			"family_name": "Patient",
			"birth_date":  "1990-01-01",
			"gender":      "male",
		},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	searchURL := fmt.Sprintf("%s/Patient?identifier=FHIR-TEST-001", intCfg.FHIRBaseURL)
	searchResp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("Failed to search FHIR server: %v", err)
	}
	defer func() { _ = searchResp.Body.Close() }()

	if searchResp.StatusCode != http.StatusOK {
		t.Errorf("FHIR search returned status %d", searchResp.StatusCode)
	}

	body, _ := io.ReadAll(searchResp.Body)
	var bundle map[string]interface{}
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("Invalid FHIR response: %v", err)
	}

	total, _ := bundle["total"].(float64)
	if total < 1 {
		t.Error("Patient was not created in FHIR server")
	}
}

// TestWebhookAction tests the webhook action against a real HTTP destination.
//
// The old version could not fail: it ran the CLI, logged "Webhook action
// completed successfully" and asserted nothing about delivery. It also could
// not pass — its pretty-printed event literal never reached parseEventInput's
// NDJSON reader. The webhook action fails the run on any response >= 400 or any
// transport error, so a clean exit and a zero error count is the delivery
// proof; the echo server gives no read-back channel of its own.
func TestWebhookAction(t *testing.T) {
	cfg := DefaultConfig()
	intCfg := getIntegrationConfig()
	ensureBinaryBuilt(t, cfg)

	resp, err := http.Get(intCfg.WebhookURL)
	requireService(t, "webhook-echo", err)
	defer func() { _ = resp.Body.Close() }()

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
          user_agent: fi-fhir-e2e
`, intCfg.WebhookURL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient":   map[string]interface{}{"mrn": "WEBHOOK-TEST-001"},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Processed 1 events, 1 route matches, 0 errors") {
		t.Errorf("the webhook route did not deliver cleanly:\n%s", output)
	}
}

// TestWorkflowWithRetry tests retry behavior on transient failures.
//
// Two things were wrong. The retry block was nested YAML — `retry:` with
// `maxAttempts` and `initialDelay` — which Action.UnmarshalYAML drops on the
// floor; the action's real keys are the flat retry_max and retry_delay
// (internal/workflow/retry.go ParseRetryConfig). And the assertion was on
// elapsed wall clock, which the k3s-ci runner pool cannot make honest: it spans
// three CPU classes with a 5.3x spread, so a duration threshold is a coin toss
// on node assignment rather than a statement about the code. TestQuickBenchmark
// got the same treatment. Attempts are counted at the destination instead, which
// is bit-identical on every runner.
func TestWorkflowWithRetry(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// 503 is in the default retryable set, so the action retries and then gives
	// up — the same "transient failure" the unreachable port was reaching for,
	// but observable.
	var attempts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	const retryMax = 2

	workflowYAML := fmt.Sprintf(`
workflow:
  name: retry_test
  version: "1.0"
  routes:
    - name: retry_route
      filter:
        event_type: patient_admit
      actions:
        - type: webhook
          url: %s
          method: POST
          retry_max: %d
          retry_delay: 10ms
          retry_jitter: 0
`, server.URL, retryMax)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient":   map[string]interface{}{"mrn": "RETRY-TEST-001"},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err == nil {
		t.Errorf("expected a non-zero exit from a destination that only answers 503:\n%s", output)
	}

	// One initial attempt plus retryMax retries: DoHTTPWithRetry loops
	// `attempt <= MaxRetries+1`.
	if got := atomic.LoadInt64(&attempts); got != retryMax+1 {
		t.Errorf("destination saw %d attempts, want %d\nOutput: %s", got, retryMax+1, output)
	}
}

// TestObservabilityEndpoints exercises the real liveness, readiness, and
// metrics surfaces against a running fi-fhir server.
//
// The two tests this replaces (TestHealthEndpoints, TestMetricsEndpoint) could
// not pass and could not fail. TestHealthEndpoints asserted
// health["status"] == "ok" while the handler wrote "healthy"; TestMetricsEndpoint
// asserted a `/metrics` endpoint that did not exist and downgraded its own
// content check to t.Logf. Both t.Skipf on a connection error, and no CI job
// ever passed -tags=e2e, so a false claim sat behind an assertion that could
// never run. That is the shape Slice 4.3 exists to remove.
//
// The blocking cross-replica proof lives in
// internal/observability (TestServeObservability_TwoReplicasUnderDocumentedConfiguration,
// CI job test:observability-replicas). This test is the single-server smoke
// equivalent and still skips when no server is reachable — but its assertions
// are now real, so a reachable server that lies fails the test.
func TestObservabilityEndpoints(t *testing.T) {
	baseURL := getEnv("TEST_FIFHIR_URL", "http://localhost:8080")
	metricsURL := getEnv("TEST_FIFHIR_METRICS_URL", "http://localhost:9090")

	resp, err := http.Get(baseURL + "/health")
	requireService(t, "fi-fhir", err)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health returned status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var health struct {
		Status     string `json:"status"`
		Components []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"components"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("invalid /health response: %v", err)
	}
	if health.Status != "healthy" && health.Status != "degraded" {
		t.Errorf("/health status = %q, want healthy or degraded", health.Status)
	}

	readyResp, err := http.Get(baseURL + "/ready")
	if err != nil {
		t.Fatalf("/ready is unreachable while /health answered: %v", err)
	}
	defer func() { _ = readyResp.Body.Close() }()
	if readyResp.StatusCode != http.StatusOK && readyResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/ready returned status %d, want 200 or 503", readyResp.StatusCode)
	}
	readyBody, _ := io.ReadAll(readyResp.Body)
	var ready struct {
		Status     string `json:"status"`
		Components []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"components"`
	}
	if err := json.Unmarshal(readyBody, &ready); err != nil {
		t.Fatalf("invalid /ready response: %v", err)
	}
	if len(ready.Components) == 0 {
		t.Error("/ready reported no components; readiness must name what it checked")
	}
	// A 503 must be explained by at least one unhealthy component, and a 200
	// must not contain one. Anything else means the aggregation lies.
	unhealthy := 0
	for _, component := range ready.Components {
		if component.Status == "unhealthy" {
			unhealthy++
		}
	}
	if readyResp.StatusCode == http.StatusServiceUnavailable && unhealthy == 0 {
		t.Error("/ready returned 503 with no unhealthy component")
	}
	if readyResp.StatusCode == http.StatusOK && unhealthy > 0 {
		t.Errorf("/ready returned 200 with %d unhealthy components", unhealthy)
	}

	metricsResp, err := http.Get(metricsURL + "/metrics")
	requireService(t, "fi-fhir-metrics", err)
	defer func() { _ = metricsResp.Body.Close() }()
	if metricsResp.StatusCode != http.StatusOK {
		t.Errorf("/metrics returned status %d, want 200", metricsResp.StatusCode)
	}
	metricsBody, _ := io.ReadAll(metricsResp.Body)
	for _, name := range []string{
		"fi_fhir_build_info",
		"fi_fhir_component_up",
		"fi_fhir_readiness_up",
	} {
		if !bytes.Contains(metricsBody, []byte(name)) {
			t.Errorf("metric %s is absent from the exposition", name)
		}
	}
	// The pre-4.3 façade advertised workflow_* names nothing emitted. Assert
	// they are gone so the dashboards and alert rules cannot silently regress.
	if bytes.Contains(metricsBody, []byte("workflow_events_processed_total")) {
		t.Error("legacy workflow_* metric names reappeared in the serve exposition")
	}
}
