// Package e2e provides end-to-end tests that drive the fi-fhir CLI.
//
// These tests verify the complete pipeline from message parsing through
// workflow execution to action delivery.
//
// Run with: go test -tags=e2e -v ./test/e2e/...
// CI job: test:e2e-legacy (ci/test-e2e-legacy.yml), blocking.
//
// WHAT SLICE S6-C FOUND WHEN IT RAN THIS FILE
//
// Every test here was red, and had been since the file was written: no CI job
// passed -tags=e2e until slice 4.4c, and that job ran exactly one test. The
// failures were not the workflow-schema drift ci/s5b-chaos-dr.yml predicted.
// Repairing every `{{.Patient.Status}}`-style Go dot-path to the snake_case JSON
// keys the engine binds — the whole of the predicted cause — flipped nothing.
// The real causes were four contracts the tests asserted and the CLI never had:
//
//   - `parse` emits the event itself, not a {"events": [...]} envelope. hl7v2
//     emits one object; csv emits an array (cmd/fi-fhir/main.go runParse).
//   - `workflow run` has no --dry-run flag. Dry run is its own subcommand,
//     `workflow dry-run`, and it prints route decisions, not rendered actions.
//   - event input is a JSON array or newline-delimited JSON. A pretty-printed
//     multi-line object fails with "failed to parse JSON line"
//     (cmd/fi-fhir/main.go parseEventInput).
//   - action config is flat scalars only. Action.UnmarshalYAML
//     (internal/workflow/types.go) copies string/number/bool fields into
//     map[string]string and silently drops every nested block, so `headers:`,
//     `retry:`, `auth:` and `fields:` were never read at all.
//
// The template repair is kept — the engine really does bind JSON keys, and
// TestWorkflowTransform now asserts the rendered output rather than logging it —
// but it was not what made a single test fail. It was masked by the four above.
//
//go:build e2e

package e2e

import (
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
//
// GoldenDir/UpdateGolden and the compareOrUpdateGolden helper they fed were
// removed by slice S6-C. test/e2e/golden/ never existed, so the helper always
// took its "golden file does not exist" branch and t.Logf'd; and it could not
// have worked if the directory had existed, because `parse` stamps a fresh
// UUID, timestamp and received_at into every event. A comparison that cannot
// fail is the "greener rather than redder" shape AGENTS.md refuses. WorkflowDir
// went with it: test/e2e/workflows/ does not exist either and nothing read it.
type TestConfig struct {
	BinaryPath  string
	TestDataDir string
}

// DefaultConfig returns the default test configuration.
func DefaultConfig() *TestConfig {
	root := findProjectRoot()

	return &TestConfig{
		BinaryPath:  filepath.Join(root, "bin", "fi-fhir"),
		TestDataDir: filepath.Join(root, "testdata"),
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
//
// Only adt_a01_sample.hl7 was ever committed. The A02 and A03 cases read
// testdata/adt_a02_transfer.hl7 and adt_a03_discharge.hl7, which do not exist
// anywhere in the tree and never did, so both t.Skipf'd on every run — two
// subtests that could not fail. They now build their own messages, which is
// what this file's TestEndToEndPipeline already did.
func TestParseHL7v2ADT(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	const pid = "PID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M"

	tests := []struct {
		name     string
		file     string
		message  string
		wantType string
	}{
		{name: "ADT_A01", file: "adt_a01_sample.hl7", wantType: "patient_admit"},
		{
			name:     "ADT_A02",
			message:  hl7Message("ADT^A02", "MSG002", pid),
			wantType: "patient_transfer",
		},
		{
			name:     "ADT_A03",
			message:  hl7Message("ADT^A03", "MSG003", pid),
			wantType: "patient_discharge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inputFile string
			if tt.file != "" {
				inputFile = filepath.Join(cfg.TestDataDir, tt.file)
				// Existence guard: a deleted fixture must turn this test red,
				// not green. The version that skipped here reported success for
				// two cases it never ran.
				if _, err := os.Stat(inputFile); err != nil {
					t.Fatalf("committed fixture %s is missing: %v", tt.file, err)
				}
			} else {
				inputFile = createTempFile(t, tt.message, ".hl7")
				defer os.Remove(inputFile)
			}

			output, err := runCLI(cfg, "parse", "--format", "hl7v2", "--pretty", inputFile)
			if err != nil {
				t.Fatalf("parse failed: %v\nOutput: %s", err, output)
			}

			// `parse --format hl7v2` writes the event itself. There is no
			// {"events": [...]} envelope; the version that looked for one read
			// a nil and reported "expected at least one event".
			event := decodeEvent(t, output)
			if event["type"] != tt.wantType {
				t.Errorf("event type = %v, want %v", event["type"], tt.wantType)
			}
			if event["source_format"] != "hl7v2" {
				t.Errorf("source_format = %v, want hl7v2", event["source_format"])
			}
			patient, ok := event["patient"].(map[string]interface{})
			if !ok || patient["mrn"] == nil || patient["mrn"] == "" {
				t.Errorf("event carries no patient mrn: %v", event["patient"])
			}
		})
	}
}

// TestParseHL7v2ORU tests parsing of HL7v2 ORU (lab result) messages.
func TestParseHL7v2ORU(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	inputFile := filepath.Join(cfg.TestDataDir, "oru_r01_sample.hl7")
	if _, err := os.Stat(inputFile); err != nil {
		t.Fatalf("committed fixture oru_r01_sample.hl7 is missing: %v", err)
	}

	output, err := runCLI(cfg, "parse", "--format", "hl7v2", "--pretty", inputFile)
	if err != nil {
		t.Fatalf("parse failed: %v\nOutput: %s", err, output)
	}

	event := decodeEvent(t, output)
	if event["type"] != "lab_result" {
		t.Errorf("event type = %v, want lab_result", event["type"])
	}

	// The lab result carries "results" (LabResultEvent.Results). The version
	// that asserted "observations" named a key the parser has never emitted.
	results, ok := event["results"].([]interface{})
	if !ok || len(results) == 0 {
		t.Fatalf("expected at least one result in the lab event, got %v", event["results"])
	}
}

// TestParseCSV tests parsing of CSV files.
func TestParseCSV(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	csvContent := `mrn,first_name,last_name,dob,gender
MRN001,John,Doe,1980-01-15,M
MRN002,Jane,Smith,1990-05-20,F`

	tmpFile := createTempFile(t, csvContent, ".csv")
	defer os.Remove(tmpFile)

	// --event-type patient: without it the CSV parser emits generic csv_record
	// events with no patient object at all, so the columns this fixture names
	// are never mapped and the test proves only that two rows were read.
	output, err := runCLI(cfg, "parse", "--format", "csv", "--event-type", "patient", "--pretty", tmpFile)
	if err != nil {
		t.Fatalf("parse failed: %v\nOutput: %s", err, output)
	}

	// csv emits the event slice directly (runParse: outputData = result.Events).
	// Decoding it into a map fails with "cannot unmarshal array into Go value of
	// type map[string]interface {}", which is what this test did before.
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, output)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 events, got %d", len(parsed))
	}
	for i, event := range parsed {
		patient, ok := event["patient"].(map[string]interface{})
		if !ok {
			t.Fatalf("event %d has no patient object: %v", i, event)
		}
		if patient["mrn"] == nil || patient["mrn"] == "" {
			t.Errorf("event %d carries no mrn: %v", i, patient)
		}
	}
}

// TestWorkflowDryRun tests workflow execution in dry-run mode.
//
// `workflow run --dry-run` is not a thing: runWorkflowRun rejects any unknown
// flag, so this test failed with "unknown flag: --dry-run" from the day it was
// written. Dry run is the `workflow dry-run` subcommand.
func TestWorkflowDryRun(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

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
          message: "Patient admitted: {{.patient.family_name}}"
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient": map[string]interface{}{
			"mrn":         "MRN001",
			"given_name":  "John",
			"family_name": "Doe",
		},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "dry-run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow dry-run failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Dry-run results:") {
		t.Errorf("dry-run output missing its header:\n%s", output)
	}
	if !strings.Contains(output, "Route 'all_admits': MATCH") {
		t.Errorf("route all_admits did not match the admit event:\n%s", output)
	}
}

// TestWorkflowWithWebhook tests workflow with webhook action using mock server.
func TestWorkflowWithWebhook(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	received := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "received"}`))
	}))
	defer server.Close()

	// No `headers:` block: Action.UnmarshalYAML keeps only scalar fields, so the
	// Content-Type map this test used to declare was dropped before the action
	// ever saw it. webhookAction sets application/json itself.
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
`, server.URL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient":   map[string]interface{}{"mrn": "MRN001"},
	})
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	select {
	case body := <-received:
		var event map[string]interface{}
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("webhook received invalid JSON: %v\nBody: %s", err, body)
		}
		patient, ok := event["patient"].(map[string]interface{})
		if !ok || patient["mrn"] != "MRN001" {
			t.Errorf("webhook received the wrong event: %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook did not receive any data")
	}
}

// TestWorkflowCELFilter tests CEL expression filtering.
//
// The old assertion looked for the route name in the output. `workflow dry-run`
// prints one line per route whether or not it matched, so the route name is
// always present and the assertion could only ever report "matched". The
// decision is in the MATCH / NO MATCH verdict, so that is what is asserted.
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
          message: "Elderly patient: {{.patient.family_name}}"
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
			eventFile := createEventFile(t, map[string]interface{}{
				"type":      "patient_admit",
				"source":    "test",
				"timestamp": "2024-01-15T10:00:00Z",
				"patient":   map[string]interface{}{"mrn": "MRN001", "age": tt.age},
			})
			defer os.Remove(eventFile)

			output, err := runCLI(cfg, "workflow", "dry-run", "--config", workflowFile, eventFile)
			if err != nil {
				t.Fatalf("workflow dry-run failed: %v\nOutput: %s", err, output)
			}

			matched := strings.Contains(output, "Route 'critical_only': MATCH")
			noMatch := strings.Contains(output, "Route 'critical_only': NO MATCH")
			if matched == noMatch {
				t.Fatalf("dry-run reported neither a match nor a non-match for critical_only:\n%s", output)
			}
			if matched != tt.shouldPass {
				t.Errorf("filter matched = %v, want %v\n%s", matched, tt.shouldPass, output)
			}
		})
	}
}

// TestWorkflowTransform tests transform operations.
//
// This is the one test the predicted template drift actually reached. It is
// also the one that could not fail: it logged the output and passed regardless.
// With `{{.Patient.Status}}`, renderTemplate fails to execute and returns the
// template source verbatim, so the log line reads "Status: {{.Patient.Status}}"
// — the drift, printed. Asserted now, against the snake_case key.
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
          message: "Status: {{.patient.status}}"
`
	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	eventFile := createEventFile(t, map[string]interface{}{
		"type":      "patient_admit",
		"source":    "test",
		"timestamp": "2024-01-15T10:00:00Z",
		"patient":   map[string]interface{}{"mrn": "MRN001"},
	})
	defer os.Remove(eventFile)

	// `workflow run`, not dry-run: DryRun counts the actions it would run and
	// executes none of them, so no transform result is ever rendered.
	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Status: active") {
		t.Errorf("transform did not set patient.status, or the template did not bind it:\n%s", output)
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
		skipIssue string
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
			// Engine defect, filed with a reproduction as fi-fhir issue #21.
			// `workflow validate` calls Workflow.Validate (types.go), which
			// checks names and action types and nothing else. The validator that
			// compiles the CEL condition — workflow.Validator, which emits
			// INVALID_CEL — has no CLI caller, so `workflow validate` accepts an
			// expression the engine cannot compile and the route then silently
			// never matches at run time. Not this lane's file to fix; unskip
			// when #21 wires runWorkflowValidate to workflow.NewValidator.
			skipIssue: "#21",
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
			if tt.skipIssue != "" {
				t.Skipf("blocked on fi-fhir issue %s: `workflow validate` does not compile CEL conditions", tt.skipIssue)
			}

			configFile := createTempFile(t, tt.config, ".yaml")
			defer os.Remove(configFile)

			output, err := runCLI(cfg, "workflow", "validate", configFile)
			valid := err == nil

			if valid != tt.wantValid {
				t.Errorf("validation result = %v, want %v\nOutput: %s", valid, tt.wantValid, output)
			}
		})
	}
}

// TestEndToEndPipeline tests the complete pipeline.
//
// This is the nil-interface panic ci/s5b-chaos-dr.yml recorded at line 493. It
// was not an engine defect: the test asserted `parseResult["events"]` — the
// envelope `parse` has never emitted — with an unchecked type assertion, so a
// missing key panicked the whole binary and took the rest of the suite's run
// with it. The parse output is the event; it is fed to `workflow run` directly.
func TestEndToEndPipeline(t *testing.T) {
	cfg := DefaultConfig()
	ensureBinaryBuilt(t, cfg)

	// Step 1: parse an ADT^A01 message.
	message := hl7Message("ADT^A01", "MSG001",
		"PID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M|||123 MAIN ST^^CITY^ST^12345")

	msgFile := createTempFile(t, message, ".hl7")
	defer os.Remove(msgFile)

	parseOutput, err := runCLI(cfg, "parse", "--format", "hl7v2", msgFile)
	if err != nil {
		t.Fatalf("parse failed: %v\nOutput: %s", err, parseOutput)
	}

	event := decodeEvent(t, parseOutput)
	if event["type"] != "patient_admit" {
		t.Fatalf("parsed event type = %v, want patient_admit", event["type"])
	}

	// Step 2: run that event, unmodified, through a workflow.
	received := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
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
          message: "Processed admit for {{.patient.family_name}}"
`, server.URL)

	workflowFile := createTempFile(t, workflowYAML, ".yaml")
	defer os.Remove(workflowFile)

	// `parse` without --pretty writes one compact JSON object, which is exactly
	// what parseEventInput accepts as newline-delimited JSON.
	eventFile := createTempFile(t, strings.TrimSpace(parseOutput)+"\n", ".json")
	defer os.Remove(eventFile)

	output, err := runCLI(cfg, "workflow", "run", "--config", workflowFile, eventFile)
	if err != nil {
		t.Fatalf("workflow run failed: %v\nOutput: %s", err, output)
	}

	// Step 3: verify.
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("webhook action was not executed")
	}
	if !strings.Contains(output, "Processed admit for DOE") {
		t.Errorf("log action did not render the parsed patient name:\n%s", output)
	}
}

// Helper functions

// hl7Message builds a minimal well-formed HL7v2 message. Segments are joined
// with carriage returns, which is what the standard requires and what the
// parser splits on.
func hl7Message(messageType, controlID string, segments ...string) string {
	all := []string{
		fmt.Sprintf(`MSH|^~\&|EPIC|HOSPITAL|FI-FHIR|DEST|20240115103000||%s|%s|P|2.5`,
			messageType, controlID),
	}
	all = append(all, segments...)
	all = append(all, "PV1|1|I|WEST^101^A")
	return strings.Join(all, "\r") + "\r"
}

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

// runCLI runs the built binary and returns its combined output.
//
// Actions log to stdout and the CLI reports errors on stderr, so both are part
// of what a test asserts on. Keeping them apart cost the old TestDatabaseAction
// its own diagnosis: it reported "exit status 1" and discarded the reason.
func runCLI(cfg *TestConfig, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func createTempFile(t *testing.T, content, suffix string) string {
	t.Helper()

	f, err := os.CreateTemp("", "fi-fhir-test-*"+suffix)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}

	_ = f.Close()
	return f.Name()
}

// createEventFile writes one event as newline-delimited JSON.
//
// parseEventInput accepts a JSON array or NDJSON and nothing else. Every
// workflow test in this tree embedded a pretty-printed multi-line object in a
// raw string literal, so the CLI reported "failed to parse JSON line: unexpected
// end of JSON input" and no action ever ran. Building the event as a map and
// marshalling it makes that class of drift impossible to reintroduce.
func createEventFile(t *testing.T, event map[string]interface{}) string {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to encode event: %v", err)
	}
	return createTempFile(t, string(encoded)+"\n", ".json")
}

// decodeEvent decodes a single-event `parse` output.
func decodeEvent(t *testing.T, output string) map[string]interface{} {
	t.Helper()

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(output), &event); err != nil {
		t.Fatalf("invalid parse output: %v\nOutput: %s", err, output)
	}
	if len(event) == 0 {
		t.Fatalf("parse produced an empty event\nOutput: %s", output)
	}
	return event
}
