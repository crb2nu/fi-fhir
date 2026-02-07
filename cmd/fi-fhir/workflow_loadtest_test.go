package main

import "testing"

// =============================================================================
// runWorkflowLoadtest — flag-parsing branches NOT covered by
// workflow_coverage_test.go
// =============================================================================

func TestWorkflowLoadtest_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a file path")
}

func TestWorkflowLoadtest_MissingScenarioValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--scenario")
	assertError(t, err)
	assertErrorContains(t, err, "--scenario requires a name")
}

func TestWorkflowLoadtest_MissingDurationValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--duration")
	assertError(t, err)
	assertErrorContains(t, err, "--duration requires a value")
}

func TestWorkflowLoadtest_MissingRPSValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--rps")
	assertError(t, err)
	assertErrorContains(t, err, "--rps requires a value")
}

func TestWorkflowLoadtest_MissingWorkersValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--workers")
	assertError(t, err)
	assertErrorContains(t, err, "--workers requires a value")
}

func TestWorkflowLoadtest_MissingWarmupValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "--warmup")
	assertError(t, err)
	assertErrorContains(t, err, "--warmup requires a value")
}

func TestWorkflowLoadtest_ShortConfigFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-c")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a file path")
}

func TestWorkflowLoadtest_ShortScenarioFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-s")
	assertError(t, err)
	assertErrorContains(t, err, "--scenario requires a name")
}

func TestWorkflowLoadtest_ShortDurationFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-d")
	assertError(t, err)
	assertErrorContains(t, err, "--duration requires a value")
}

func TestWorkflowLoadtest_ShortRPSFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-r")
	assertError(t, err)
	assertErrorContains(t, err, "--rps requires a value")
}

func TestWorkflowLoadtest_ShortWorkersFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-w")
	assertError(t, err)
	assertErrorContains(t, err, "--workers requires a value")
}

func TestWorkflowLoadtest_ConfigNotFound(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "loadtest", "-c", "/nonexistent/workflow.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "failed to read workflow")
}

func TestWorkflowLoadtest_UnknownScenario(t *testing.T) {
	tmpDir := t.TempDir()
	workflowYAML := `routes:
  - name: test
    filter:
      event_type: patient_admit
    actions:
      - type: log
        config:
          level: info`
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", workflowYAML)

	_, _, err := runCLI(t, "workflow", "loadtest", "-c", configPath, "-s", "nonexistent-scenario")
	assertError(t, err)
	assertErrorContains(t, err, "unknown scenario")
}

// =============================================================================
// runWorkflowReplay — flag-parsing branches NOT covered by
// workflow_coverage_test.go
// =============================================================================

func TestWorkflowReplay_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestWorkflowReplay_MissingRecordingsValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--recordings")
	assertError(t, err)
	assertErrorContains(t, err, "--recordings requires a value")
}

func TestWorkflowReplay_MissingEventTypeValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--event-type")
	assertError(t, err)
	assertErrorContains(t, err, "--event-type requires a value")
}

func TestWorkflowReplay_MissingSourceValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--source")
	assertError(t, err)
	assertErrorContains(t, err, "--source requires a value")
}

func TestWorkflowReplay_MissingLimitValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--limit")
	assertError(t, err)
	assertErrorContains(t, err, "--limit requires a value")
}

func TestWorkflowReplay_InvalidLimit(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", `routes: []`)

	_, _, err := runCLI(t, "workflow", "replay", "-c", configPath, "--recordings", "/tmp/rec.json", "--limit", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "invalid limit")
}

func TestWorkflowReplay_MissingOutputValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--output")
	assertError(t, err)
	assertErrorContains(t, err, "--output requires a value")
}

func TestWorkflowReplay_ShortConfigFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "-c")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestWorkflowReplay_ShortRecordingsFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "-r")
	assertError(t, err)
	assertErrorContains(t, err, "--recordings requires a value")
}

func TestWorkflowReplay_ShortEventTypeFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "-t")
	assertError(t, err)
	assertErrorContains(t, err, "--event-type requires a value")
}

func TestWorkflowReplay_ShortSourceFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "-s")
	assertError(t, err)
	assertErrorContains(t, err, "--source requires a value")
}

func TestWorkflowReplay_ShortOutputFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "-o")
	assertError(t, err)
	assertErrorContains(t, err, "--output requires a value")
}

// =============================================================================
// runWorkflowSimulate — flag-parsing branches NOT covered by
// workflow_coverage_test.go
// =============================================================================

func TestWorkflowSimulate_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestWorkflowSimulate_MissingOutputValue(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "--output")
	assertError(t, err)
	assertErrorContains(t, err, "--output requires a value")
}

func TestWorkflowSimulate_ShortConfigFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "-c")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestWorkflowSimulate_ShortOutputFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "-o")
	assertError(t, err)
	assertErrorContains(t, err, "--output requires a value")
}
