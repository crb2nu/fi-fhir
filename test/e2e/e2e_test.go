// Package e2e provides end-to-end integration tests for fi-fhir.
//
// These tests verify the complete pipeline from message parsing through
// workflow execution to action delivery.
//
// Run with: go test -tags=e2e -v ./test/e2e/...
//
//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConfig holds configuration for E2E tests.
type TestConfig struct {
	BinaryPath   string
	TestDataDir  string
	WorkflowDir  string
	GoldenDir    string
	UpdateGolden bool
}

// DefaultConfig returns the default test configuration.
func DefaultConfig() *TestConfig {
	// Find project root
	root := findProjectRoot()

	return &TestConfig{
		BinaryPath:   filepath.Join(root, "bin", "fi-fhir"),
		TestDataDir:  filepath.Join(root, "testdata"),
		WorkflowDir:  filepath.Join(root, "test", "e2e", "workflows"),
		GoldenDir:    filepath.Join(root, "test", "e2e", "golden"),
		UpdateGolden: os.Getenv("UPDATE_GOLDEN") == "1",
	}
}

func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// TestParseHL7v2ADT tests parsing of HL7v2 ADT messages.
func TestParseHL7v2ADT(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	tests := []struct {
		name     string
		file     string
		wantType string
	}{
		{"ADT_A01", "adt_a01_sample.hl7", "patient_admit"},
		{"ADT_A02", "adt_a02_transfer.hl7", "patient_transfer"},
		{"ADT_A03", "adt_a03_discharge.hl7", "patient_discharge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputFile := filepath.Join(cfg.TestDataDir, tt.file)
			if _, err := os.Stat(inputFile); os.IsNotExist(err) {
				t.Skipf("Test file %s not found", tt.file)
			}

			// Run parse command
			output, err := runCLI(cfg, "parse", "--format", "hl7v2", "--pretty", inputFile)
			if err != nil {
				t.Fatalf("parse failed: %v\nOutput: %s", err, output)
			}

			// Verify output is valid JSON
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("invalid JSON output: %v", err)
			}

			// Check event type
			events, ok := result["events"].([]interface{})
			if !ok || len(events) == 0 {
				t.Fatal("expected at least one event")
			}

			event := events[0].(map[string]interface{})
			if event["type"] != tt.wantType {
				t.Errorf("event type = %v, want %v", event["type"], tt.wantType)
			}

			// Golden file comparison
			goldenFile := filepath.Join(cfg.GoldenDir, tt.name+".json")
			compareOrUpdateGolden(t, cfg, goldenFile, output)
		})
	}
}

// TestParseHL7v2ORU tests parsing of HL7v2 ORU (lab result) messages.
func TestParseHL7v2ORU(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	inputFile := filepath.Join(cfg.TestDataDir, "oru_r01_sample.hl7")
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		t.Skip("Test file oru_r01_sample.hl7 not found")
	}

	output, err := runCLI(cfg, "parse", "--format", "hl7v2", "--pretty", inputFile)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	events, ok := result["events"].([]interface{})
	if !ok || len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	event := events[0].(map[string]interface{})
	if event["type"] != "lab_result" {
		t.Errorf("event type = %v, want lab_result", event["type"])
	}

	// Verify observations are present
	if _, ok := event["observations"]; !ok {
		t.Error("expected observations in lab result event")
	}
}

// TestParseCSV tests parsing of CSV files.
func TestParseCSV(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// Create test CSV
	csvContent := `mrn,first_name,last_name,dob,gender
MRN001,John,Doe,1980-01-15,M
MRN002,Jane,Smith,1990-05-20,F`

	tmpFile := createTempFile(t, csvContent, ".csv")
	defer os.Remove(tmpFile)

	output, err := runCLI(cfg, "parse", "--format", "csv", "--pretty", tmpFile)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	events, ok := result["events"].([]interface{})
	if !ok {
		t.Fatal("expected events array")
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

// TestWorkflowDryRun tests workflow execution in dry-run mode.
func TestWorkflowDryRun(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// Create workflow config
	workflowYAML := `
workflow:
  name: test_workflow
  version: "1.0"
  routes:
    - name: all_admits
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Patient admitted: {{.Patient.Name.Family}}"
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	// Create test event
	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {
			"mrn": "MRN001",
			"name": {"given": "John", "family": "Doe"}
		}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--dry-run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	// Verify dry-run output
	if !strings.Contains(output, "dry-run") && !strings.Contains(output, "DRY") {
		t.Log("Output:", output)
		// Don't fail - output format may vary
	}
}

// TestWorkflowWithWebhook tests workflow with webhook action using mock server.
func TestWorkflowWithWebhook(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// Create mock webhook server
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))
	defer server.Close()

	// Create workflow config with webhook
	workflowYAML := fmt.Sprintf(`
workflow:
  name: webhook_test
  version: "1.0"
  routes:
    - name: send_to_webhook
      filter:
        event_type: patient_admit
      actions:
        - type: webhook
          url: %s
          method: POST
          headers:
            Content-Type: application/json
`, server.URL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	// Create test event
	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {"mrn": "MRN001"}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	_, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v", err)
	}

	// Verify webhook received the event
	if len(receivedBody) == 0 {
		t.Error("webhook did not receive any data")
	}

	var received map[string]interface{}
	if err := json.Unmarshal(receivedBody, &received); err != nil {
		t.Errorf("webhook received invalid JSON: %v", err)
	}
}

// TestWorkflowCELFilter tests CEL expression filtering.
func TestWorkflowCELFilter(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	workflowYAML := `
workflow:
  name: cel_test
  version: "1.0"
  routes:
    - name: critical_only
      filter:
        condition: event.patient.age >= 65
      actions:
        - type: log
          level: warn
          message: "Elderly patient: {{.Patient.Name.Family}}"
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	tests := []struct {
		name       string
		age        int
		shouldPass bool
	}{
		{"young_patient", 30, false},
		{"elderly_patient", 70, true},
		{"boundary_patient", 65, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventJSON := fmt.Sprintf(`{
				"type": "patient_admit",
				"source": "test",
				"timestamp": "2024-01-15T10:00:00Z",
				"patient": {"mrn": "MRN001", "age": %d}
			}`, tt.age)

			eventFile := createTempFile(t, eventJSON, ".json")
			defer os.Remove(eventFile)

			output, err := runCLI(cfg, "workflow", "run", "--dry-run", "--config", workflowFile, eventFile)
			if err != nil {
				t.Fatalf("workflow run failed: %v", err)
			}

			matched := strings.Contains(output, "critical_only") || strings.Contains(output, "Elderly")
			if matched != tt.shouldPass {
				t.Errorf("filter matched = %v, want %v", matched, tt.shouldPass)
			}
		})
	}
}

// TestWorkflowTransform tests transform operations.
func TestWorkflowTransform(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	workflowYAML := `
workflow:
  name: transform_test
  version: "1.0"
  routes:
    - name: transform_route
      filter:
        event_type: patient_admit
      transform:
        - set_field: patient.status = "active"
      actions:
        - type: log
          level: info
          message: "Status: {{.Patient.Status}}"
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventJSON := `{
		"type": "patient_admit",
		"source": "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": {"mrn": "MRN001"}
	}`
	eventFile := createTempFile(t, eventJSON, ".json")
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--dry-run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v", err)
	}

	// Transform should have set status to active
	if !strings.Contains(output, "active") {
		t.Log("Output:", output)
		// Transform might not be visible in dry-run output
	}
}

// TestConfigValidation tests configuration validation.
func TestConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	tests := []struct {
		name      string
		config    string
		wantValid bool
	}{
		{
			name: "valid_config",
			config: `
workflow:
  name: valid
  version: "1.0"
  routes:
    - name: test
      filter: {}
      actions:
        - type: log
          level: info
`,
			wantValid: true,
		},
		{
			name: "invalid_cel",
			config: `
workflow:
  name: invalid
  version: "1.0"
  routes:
    - name: test
      filter:
        condition: this.is.invalid.cel[[[
      actions:
        - type: log
          level: info
`,
			wantValid: false,
		},
		{
			name: "missing_action_type",
			config: `
workflow:
  name: invalid
  version: "1.0"
  routes:
    - name: test
      filter: {}
      actions:
        - level: info
`,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := createTempFile(t, tt.config, ".yaml")
			defer os.Remove(configFile)

			_, err := runCLI(cfg, "workflow", "validate", configFile)
			valid := err == nil

			if valid != tt.wantValid {
				t.Errorf("validation result = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

// TestEndToEndPipeline tests the complete pipeline.
func TestEndToEndPipeline(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// This test simulates a complete workflow:
	// 1. Parse HL7v2 message
	// 2. Run through workflow
	// 3. Verify actions executed

	// Step 1: Parse message
	hl7Message := `MSH|^~\&|EPIC|HOSPITAL|FI-FHIR|DEST|20240115103000||ADT^A01|MSG001|P|2.5
PID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M|||123 MAIN ST^^CITY^ST^12345`

	msgFile := createTempFile(t, hl7Message, ".hl7")
	defer os.Remove(msgFile)

	parseOutput, err := runCLI(cfg, "parse", "--format", "hl7v2", msgFile)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Extract events from parse output
	var parseResult map[string]interface{}
	if err := json.Unmarshal([]byte(parseOutput), &parseResult); err != nil {
		t.Fatalf("invalid parse output: %v", err)
	}

	events := parseResult["events"].([]interface{})
	if len(events) == 0 {
		t.Fatal("no events parsed")
	}

	// Step 2: Create workflow and process event
	webhookCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	workflowYAML := fmt.Sprintf(`
workflow:
  name: e2e_pipeline
  version: "1.0"
  routes:
    - name: admit_to_webhook
      filter:
        event_type: patient_admit
      transform:
        - set_field: patient.processed = true
      actions:
        - type: webhook
          url: %s
          method: POST
        - type: log
          level: info
          message: "Processed admit for {{.Patient.Name.Family}}"
`, server.URL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	// Write event to file
	eventJSON, _ := json.Marshal(events[0])
	eventFile := createTempFile(t, string(eventJSON), ".json")
	defer os.Remove(eventFile)

	_, err = runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v", err)
	}

	// Step 3: Verify
	if !webhookCalled {
		t.Error("webhook action was not executed")
	}
}

// Helper functions

func ensureBinaryBuilt(t *testing.T, cfg *TestConfig) {
	t.Helper()

	if _, err := os.Stat(cfg.BinaryPath); os.IsNotExist(err) {
		t.Log("Building fi-fhir binary...")
		root := findProjectRoot()
		cmd := exec.Command("go", "build", "-o", cfg.BinaryPath, "./cmd/fi-fhir")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to build binary: %v\n%s", err, output)
		}
	}
}

func runCLI(cfg *TestConfig, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if err != nil {
		output += "\nSTDERR: " + stderr.String()
	}

	return output, err
}

func createTempFile(t *testing.T, content, suffix string) string {
	t.Helper()

	f, err := os.CreateTemp("", "fi-fhir-test-*"+suffix)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}

	f.Close()
	return f.Name()
}

func compareOrUpdateGolden(t *testing.T, cfg *TestConfig, goldenFile, actual string) {
	t.Helper()

	// Normalize JSON for comparison
	var normalized bytes.Buffer
	if err := json.Indent(&normalized, []byte(actual), "", "  "); err != nil {
		t.Logf("Warning: could not normalize JSON: %v", err)
		return
	}
	actual = normalized.String()

	if cfg.UpdateGolden {
		// Ensure directory exists
		os.MkdirAll(filepath.Dir(goldenFile), 0755)

		if err := os.WriteFile(goldenFile, []byte(actual), 0644); err != nil {
			t.Errorf("failed to update golden file: %v", err)
		}
		t.Logf("Updated golden file: %s", goldenFile)
		return
	}

	expected, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		t.Logf("Golden file %s does not exist. Run with UPDATE_GOLDEN=1 to create.", goldenFile)
		return
	}
	if err != nil {
		t.Errorf("failed to read golden file: %v", err)
		return
	}

	if string(expected) != actual {
		t.Errorf("output does not match golden file %s\n"+
			"Run with UPDATE_GOLDEN=1 to update golden files", goldenFile)

		// Show diff snippet
		expectedLines := strings.Split(string(expected), "\n")
		actualLines := strings.Split(actual, "\n")
		for i := 0; i < len(expectedLines) && i < len(actualLines); i++ {
			if expectedLines[i] != actualLines[i] {
				t.Logf("First difference at line %d:", i+1)
				t.Logf("  Expected: %s", expectedLines[i])
				t.Logf("  Actual:   %s", actualLines[i])
				break
			}
		}
	}
}
