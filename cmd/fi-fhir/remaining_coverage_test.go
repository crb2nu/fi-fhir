package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion"
)

// =============================================================================
// companionIssuesToWarnings — pure function coverage (82.4% → higher)
// =============================================================================

func TestCompanionIssuesToWarnings_Nil(t *testing.T) {
	result := companionIssuesToWarnings(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestCompanionIssuesToWarnings_EmptyResult(t *testing.T) {
	result := companionIssuesToWarnings(&companion.ValidationResult{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty result, got %d", len(result))
	}
}

func TestCompanionIssuesToWarnings_WithGuideID(t *testing.T) {
	result := companionIssuesToWarnings(&companion.ValidationResult{
		GuideID: "837P-Professional",
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result))
	}
	if result[0].Code != "COMPANION_GUIDE" {
		t.Errorf("expected code COMPANION_GUIDE, got %q", result[0].Code)
	}
	assertContains(t, result[0].Message, "837P-Professional")
}

func TestCompanionIssuesToWarnings_WithIssues(t *testing.T) {
	result := companionIssuesToWarnings(&companion.ValidationResult{
		GuideID: "test-guide",
		Info: []companion.ValidationIssue{
			{Code: "INFO_01", Message: "Informational", Path: "CLM/01", Severity: "info"},
		},
		Warnings: []companion.ValidationIssue{
			{Code: "WARN_01", Message: "Missing recommended field", Path: "NM1/09", Severity: "warning"},
		},
		Errors: []companion.ValidationIssue{
			{Code: "ERR_01", Message: "Required field missing", Path: "SBR/01", Severity: "error"},
		},
	})
	// 1 guide + 1 info + 1 warning + 1 error = 4
	if len(result) != 4 {
		t.Errorf("expected 4 warnings, got %d", len(result))
	}
}

func TestCompanionIssuesToWarnings_WithValue(t *testing.T) {
	result := companionIssuesToWarnings(&companion.ValidationResult{
		Errors: []companion.ValidationIssue{
			{Code: "ERR_01", Message: "Invalid value", Value: "ABC", Path: "CLM/05", Severity: "error"},
		},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result))
	}
	// When Value is set, message should include it
	assertContains(t, result[0].Message, "ABC")
}

func TestCompanionIssuesToWarnings_WithoutValue(t *testing.T) {
	result := companionIssuesToWarnings(&companion.ValidationResult{
		Warnings: []companion.ValidationIssue{
			{Code: "W01", Message: "Missing field", Path: "NM1/09", Severity: "warning"},
		},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result))
	}
	if result[0].Message != "Missing field" {
		t.Errorf("expected exact message without value suffix, got %q", result[0].Message)
	}
}

// =============================================================================
// Subscription commands — with valid config file (exercises past LoadConfig)
// =============================================================================

const subscriptionConfigYAML = `subscriptions:
  - name: test-sub
    server: http://localhost:99999/fhir
    criteria: Patient
    channel:
      type: rest-hook
      endpoint: http://localhost:99998/webhook
`

func writeSubscriptionConfig(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "subscriptions.yaml")
	if err := os.WriteFile(p, []byte(subscriptionConfigYAML), 0o600); err != nil {
		t.Fatalf("write subscription config: %v", err)
	}
	return p
}

func TestRunSubscriptionStatus_ValidConfig_SubNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionStatus([]string{"--config", cfgPath, "--name", "nonexistent-sub"})
	assertError(t, err)
	assertErrorContains(t, err, "not found")
}

func TestRunSubscriptionStatus_ValidConfig_ConnError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionStatus([]string{"--config", cfgPath, "--name", "test-sub"})
	assertError(t, err)
	// Will fail at FHIR client List() with connection refused
}

func TestRunSubscriptionDelete_ValidConfig_SubNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionDelete([]string{"--config", cfgPath, "--name", "nonexistent-sub", "--id", "sub-123"})
	assertError(t, err)
	assertErrorContains(t, err, "not found")
}

func TestRunSubscriptionDelete_ValidConfig_ConnError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionDelete([]string{"--config", cfgPath, "--name", "test-sub", "--id", "sub-123"})
	assertError(t, err)
	// Will fail at FHIR client Delete() with connection refused
}

func TestRunSubscriptionPause_ValidConfig_SubNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionPauseResume([]string{"--config", cfgPath, "--name", "nonexistent-sub", "--id", "sub-123"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "not found")
}

func TestRunSubscriptionPause_ValidConfig_ConnError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionPauseResume([]string{"--config", cfgPath, "--name", "test-sub", "--id", "sub-123"}, true)
	assertError(t, err)
	// Connection refused
}

func TestRunSubscriptionCreate_ValidConfig_SubNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionCreate([]string{"--config", cfgPath, "--name", "nonexistent-sub"})
	assertError(t, err)
	assertErrorContains(t, err, "not found")
}

func TestRunSubscriptionCreate_ValidConfig_ConnError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	err := runSubscriptionCreate([]string{"--config", cfgPath, "--name", "test-sub"})
	assertError(t, err)
	// Connection refused
}

func TestRunSubscriptionList_ValidConfig_PrintsConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeSubscriptionConfig(t, dir)

	stdout, _ := captureOutput(t, func() {
		err := runSubscriptionList([]string{"--config", cfgPath})
		if err != nil {
			t.Errorf("expected nil error for list, got: %v", err)
		}
	})
	assertContains(t, stdout, "test-sub")
}

// =============================================================================
// initProfileStoreFromEnv — offline tests (73.1% → higher)
// =============================================================================

func TestInitProfileStoreFromEnv_EmptyConfig(t *testing.T) {
	// Clear all DB-related env vars
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")
	t.Setenv("FI_FHIR_DATABASE_PORT", "")
	t.Setenv("FI_FHIR_DATABASE_PASSWORD", "")

	store, err := initProfileStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for empty config, got: %v", err)
	}
	if store != nil {
		t.Errorf("expected nil store for empty config")
	}
}

func TestInitProfileStoreFromEnv_PartialConfig(t *testing.T) {
	// Host set but not database or username → nil,nil
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	store, err := initProfileStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for partial config, got: %v", err)
	}
	if store != nil {
		t.Errorf("expected nil store for partial config")
	}
}

func TestInitProfileStoreFromEnv_FallbackUser(t *testing.T) {
	// Set host, db, and FI_FHIR_DATABASE_USER (not USERNAME) → tries to connect
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "testdb")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "testuser")
	t.Setenv("FI_FHIR_DATABASE_PORT", "5432")
	t.Setenv("FI_FHIR_DATABASE_PASSWORD", "testpass")
	t.Setenv("FI_FHIR_DATABASE_SSL_MODE", "")

	_, err := initProfileStoreFromEnv(context.Background())
	// Will error on ping since no real DB
	assertError(t, err)
}

// =============================================================================
// validateMessage — unsupported format path
// =============================================================================

func TestValidateMessage_BadFormat(t *testing.T) {
	errs := validateMessage("/tmp/fake-profile.yaml", "/tmp/fake-msg.txt", "xml", false)
	if len(errs) == 0 {
		t.Error("expected errors for unsupported format")
	}
	found := false
	for _, e := range errs {
		if contains(e, "unsupported format") || contains(e, "failed to load profile") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'unsupported format' or 'failed to load profile', got: %v", errs)
	}
}

// =============================================================================
// runWorkflow — additional dispatcher paths
// =============================================================================

func TestRunWorkflow_DryRunDispatch(t *testing.T) {
	err := runWorkflow([]string{"dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestRunWorkflow_RunDispatch(t *testing.T) {
	err := runWorkflow([]string{"run"})
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestRunWorkflow_ValidateDispatch(t *testing.T) {
	err := runWorkflow([]string{"validate"})
	assertError(t, err)
	assertErrorContains(t, err, "workflow file path required")
}
