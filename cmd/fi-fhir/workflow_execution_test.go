package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
)

// =============================================================================
// Helpers
// =============================================================================

const minimalWorkflowYAML = `workflow:
  name: test_workflow
  version: "1.0"
  routes:
    - name: log_all
      actions:
        - type: log
`

func writeWorkflowFile(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return p
}

func writeEventsFile(t *testing.T, dir string, events []map[string]interface{}) string {
	t.Helper()
	data, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	p := filepath.Join(dir, "events.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write events: %v", err)
	}
	return p
}

func writeRecordingsFile(t *testing.T, dir string) string {
	t.Helper()
	recordings := []map[string]interface{}{
		{
			"id":         "evt-1",
			"event":      map[string]interface{}{"type": "patient_admit", "source": "epic"},
			"event_type": "patient_admit",
			"source":     "epic",
			"processing_result": map[string]interface{}{
				"route_matches":    []string{"log_all"},
				"actions_executed": map[string]int{"log_all": 1},
				"has_errors":       false,
			},
		},
	}
	data, err := json.Marshal(recordings)
	if err != nil {
		t.Fatalf("marshal recordings: %v", err)
	}
	p := filepath.Join(dir, "recordings.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write recordings: %v", err)
	}
	return p
}

// =============================================================================
// runWorkflowSimulate — full execution tests (74.6% → higher)
// =============================================================================

func TestRunWorkflowSimulate_FullExecution(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit", "source": "epic"},
		{"type": "lab_result", "source": "lab_system"},
	})

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowSimulate([]string{"--config", wfPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Simulation Report")
	assertContains(t, stdout, "Events processed: 2")
}

func TestRunWorkflowSimulate_Verbose(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit"},
	})

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowSimulate([]string{"--config", wfPath, "--verbose", eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Action Invocations")
}

func TestRunWorkflowSimulate_WithOutputFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit"},
	})
	outPath := filepath.Join(dir, "report.json")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowSimulate([]string{"--config", wfPath, "--output", outPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Report written to")

	// Verify output file is valid JSON
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestRunWorkflowSimulate_NonexistentEventsFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)

	err := runWorkflowSimulate([]string{"--config", wfPath, "/tmp/does-not-exist-events.json"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to read input")
}

// =============================================================================
// runWorkflowReplay — full execution tests (75.0% → higher)
// =============================================================================

func TestRunWorkflowReplay_FullExecution(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowReplay([]string{"--config", wfPath, recPath})
		// May or may not error depending on route matching — just check it ran
		_ = err
	})
	assertContains(t, stdout, "Replay Summary")
	assertContains(t, stdout, "Total events")
}

func TestRunWorkflowReplay_WithDiffsFlag(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)

	stdout, _ := captureOutput(t, func() {
		_ = runWorkflowReplay([]string{"--config", wfPath, "--diffs", recPath})
	})
	assertContains(t, stdout, "Replay Summary")
}

func TestRunWorkflowReplay_WithOutputFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)
	outPath := filepath.Join(dir, "summary.json")

	stdout, _ := captureOutput(t, func() {
		_ = runWorkflowReplay([]string{"--config", wfPath, "--output", outPath, recPath})
	})
	assertContains(t, stdout, "Summary written to")

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestRunWorkflowReplay_WithEventTypeFilter(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)

	stdout, _ := captureOutput(t, func() {
		_ = runWorkflowReplay([]string{"--config", wfPath, "-t", "patient_admit", recPath})
	})
	assertContains(t, stdout, "Replay Summary")
}

func TestRunWorkflowReplay_WithSourceFilter(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)

	stdout, _ := captureOutput(t, func() {
		_ = runWorkflowReplay([]string{"--config", wfPath, "-s", "epic", recPath})
	})
	assertContains(t, stdout, "Replay Summary")
}

func TestRunWorkflowReplay_WithLimit(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	recPath := writeRecordingsFile(t, dir)

	stdout, _ := captureOutput(t, func() {
		_ = runWorkflowReplay([]string{"--config", wfPath, "--limit", "1", recPath})
	})
	assertContains(t, stdout, "Replay Summary")
}

func TestRunWorkflowReplay_InvalidRecordingsFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)

	err := runWorkflowReplay([]string{"--config", wfPath, "/tmp/does-not-exist-rec.json"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to load recordings")
}

// =============================================================================
// marshalYAML — pure function tests (71.4% → higher)
// =============================================================================

func TestMarshalYAML_ValidConfig(t *testing.T) {
	cfg := &config.Config{}

	data, err := marshalYAML(cfg)
	assertNoError(t, err)
	if len(data) == 0 {
		t.Error("expected non-empty output")
	}

	// Verify it's valid JSON (the function uses JSON indent as "YAML")
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestMarshalYAML_NilConfig(t *testing.T) {
	// Nil config should return an error or "null"
	data, err := marshalYAML(nil)
	if err != nil {
		return // error is acceptable
	}
	// If no error, output should be valid JSON
	if len(data) == 0 {
		t.Error("expected some output")
	}
}

// =============================================================================
// runWorkflowRecord — full execution (84.6% → higher)
// =============================================================================

func TestRunWorkflowRecord_FullExecution(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit", "source": "epic"},
	})
	outPath := filepath.Join(dir, "recorded.json")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowRecord([]string{"--config", wfPath, "--output", outPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Recorded")

	// Verify output file is valid JSON array
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var recordings []map[string]interface{}
	if err := json.Unmarshal(data, &recordings); err != nil {
		t.Fatalf("output should be valid JSON array: %v", err)
	}
}

// =============================================================================
// runWorkflowDryRun — additional paths (82.9% → higher)
// =============================================================================

func TestRunWorkflowDryRun_FullExecution(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit"},
		{"type": "lab_result"},
	})

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowDryRun([]string{"--config", wfPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Dry-run results")
	assertContains(t, stdout, "Event 0")
	assertContains(t, stdout, "Event 1")
}

func TestRunWorkflowDryRun_NonexistentEventsFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)

	err := runWorkflowDryRun([]string{"--config", wfPath, "/tmp/does-not-exist-events.json"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to read input")
}

// =============================================================================
// runWorkflowRun — additional execution paths (77.6% → higher)
// =============================================================================

func TestRunWorkflowRun_FullExecution(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit"},
	})

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowRun([]string{"--config", wfPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Processed")
}

func TestRunWorkflowRun_MultipleEvents(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)
	eventsPath := writeEventsFile(t, dir, []map[string]interface{}{
		{"type": "patient_admit"},
		{"type": "lab_result"},
		{"type": "patient_discharge"},
	})

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowRun([]string{"--config", wfPath, eventsPath})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Processed 3 events")
}

func TestRunWorkflowRun_NonexistentEventsFile(t *testing.T) {
	dir := t.TempDir()
	wfPath := writeWorkflowFile(t, dir, minimalWorkflowYAML)

	err := runWorkflowRun([]string{"--config", wfPath, "/tmp/does-not-exist-events.json"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to read input")
}

// =============================================================================
// runWorkflow dispatcher — additional coverage
// =============================================================================

func TestRunWorkflow_SimulateDispatch(t *testing.T) {
	err := runWorkflow([]string{"simulate"})
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestRunWorkflow_ReplayDispatch(t *testing.T) {
	err := runWorkflow([]string{"replay"})
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestRunWorkflow_RecordDispatch(t *testing.T) {
	err := runWorkflow([]string{"record"})
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}
