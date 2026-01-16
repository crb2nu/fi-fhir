//nolint:dogsled // Test file - runCLI returns are often ignored
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Workflow Loadtest Additional Coverage Tests
// =============================================================================

func TestWorkflowLoadtest_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "loadtest", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "loadtest")
}

func TestWorkflowLoadtest_MissingConfig(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest")
	assertError(t, err)
	assertErrorContains(t, err, "--config")
}

func TestWorkflowLoadtest_InvalidDuration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "loadtest", "-c", configPath, "-d", "invalid")
	assertError(t, err)
	assertErrorContains(t, err, "duration")
}

func TestWorkflowLoadtest_InvalidRPS(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "loadtest", "-c", configPath, "-r", "invalid")
	assertError(t, err)
	assertErrorContains(t, err, "rps")
}

func TestWorkflowLoadtest_InvalidWorkers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "loadtest", "-c", configPath, "-w", "invalid")
	assertError(t, err)
	assertErrorContains(t, err, "workers")
}

func TestWorkflowLoadtest_InvalidWarmup(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "loadtest", "-c", configPath, "--warmup", "invalid")
	assertError(t, err)
	assertErrorContains(t, err, "warmup")
}

func TestWorkflowLoadtest_ListScenarios(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "loadtest", "--list-scenarios")
	assertNoError(t, err)
	assertContains(t, stdout, "scenarios")
}

func TestWorkflowLoadtest_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

// =============================================================================
// Workflow Simulate Additional Coverage Tests
// =============================================================================

func TestWorkflowSimulate_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "simulate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "simulate")
}

func TestWorkflowSimulate_MissingConfig(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate")
	assertError(t, err)
	assertErrorContains(t, err, "--config")
}

func TestWorkflowSimulate_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestWorkflowSimulate_ValidRun(t *testing.T) {
	tmpDir := t.TempDir()
	workflowYAML := `routes:
  - name: test
    filter:
      event_type: patient_admit
    actions:
      - type: log
        config:
          level: info`
	workflowPath := createTempFile(t, tmpDir, "workflow*.yaml", workflowYAML)
	eventJSON := `{"type":"patient_admit","data":{"patient":{"id":"123"}}}`
	eventPath := createTempFile(t, tmpDir, "event*.json", eventJSON)

	stdout, _, err := runCLI(t, "workflow", "simulate", "-c", workflowPath, eventPath)
	assertNoError(t, err)
	assertContains(t, stdout, "Simulation")
}

// =============================================================================
// Config Command Additional Coverage Tests
// =============================================================================

func TestConfigEnv_SectionOption(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "-s", "server")
	assertNoError(t, err)
	assertContains(t, stdout, "FI_FHIR")
}

func TestConfigEnv_FormatList(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "-f", "list")
	assertNoError(t, err)
	assertContains(t, stdout, "FI_FHIR")
}

func TestConfigEnv_FormatExport(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "-f", "export")
	assertNoError(t, err)
	assertContains(t, stdout, "export")
}

func TestConfigEnv_FormatMarkdown(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "-f", "markdown")
	assertNoError(t, err)
	assertContains(t, stdout, "FI_FHIR")
}

func TestConfigEnv_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "config", "env", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestConfigValidate_MissingConfig(t *testing.T) {
	_, _, err := runCLI(t, "config", "validate")
	assertError(t, err)
}

func TestConfigValidate_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(configPath, []byte("not: valid: config"), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, _, err := runCLI(t, "config", "validate", configPath)
	assertError(t, err)
}

func TestConfigValidate_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configYAML := `server:
  host: localhost
  port: 8080`
	configPath := createTempFile(t, tmpDir, "config*.yaml", configYAML)

	stdout, _, err := runCLI(t, "config", "validate", configPath)
	assertNoError(t, err)
	assertContains(t, stdout, "valid")
}

func TestConfigShow_JSONOutput(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show", "--format", "json")
	assertNoError(t, err)
	assertContains(t, stdout, "{")
}

// =============================================================================
// Workflow Replay Additional Coverage Tests
// =============================================================================

func TestWorkflowReplay_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "replay", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "replay")
}

func TestWorkflowReplay_MissingConfig(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay")
	assertError(t, err)
	assertErrorContains(t, err, "--config")
}

func TestWorkflowReplay_MissingRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "replay", "-c", configPath)
	assertError(t, err)
	assertErrorContains(t, err, "recordings")
}

func TestWorkflowReplay_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

// =============================================================================
// Validate Profile Additional Coverage Tests
// =============================================================================

func TestValidateProfile_MissingProfile(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--profile", "/nonexistent/profile.yaml")
	assertError(t, err)
}

func TestValidateProfile_InvalidProfile(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(profilePath, []byte("not: valid: profile"), 0o600); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	// May succeed or fail depending on parsing, but should not crash
	_, _, _ = runCLI(t, "validate", "--profile", profilePath)
}

func TestValidateProfile_ValidProfile(t *testing.T) {
	tmpDir := t.TempDir()
	profileYAML := `source_profile:
  id: test
  name: Test Profile
  hl7v2:
    default_version: "2.5"`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", profileYAML)
	msgPath := testdataPath(t, "adt_a01_sample.hl7")

	if _, err := os.Stat(msgPath); err != nil {
		t.Skip("test data not found")
	}

	stdout, _, err := runCLI(t, "validate", "--profile", profilePath, "--format", "hl7v2", msgPath)
	assertNoError(t, err)
	assertContains(t, stdout, "valid")
}
