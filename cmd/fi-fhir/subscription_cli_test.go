package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubscriptionListAndValidateCLI(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subscriptions.yaml")

	cfg := `subscriptions:
  - name: patient_changes
    description: Patient updates
    server: https://example.invalid/fhir
    criteria: Patient?status=active
    channel:
      endpoint: https://receiver.example.invalid/fhir/notify/patient_changes
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write subscriptions config: %v", err)
	}

	stdout, _, err := runCLI(t, "subscription", "list", "--config", cfgPath)
	if err != nil {
		t.Fatalf("subscription list: %v", err)
	}
	if !strings.Contains(stdout, "Configured Subscriptions") {
		t.Fatalf("expected header output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "patient_changes") {
		t.Fatalf("expected subscription name output, got: %s", stdout)
	}

	stdout, _, err = runCLI(t, "subscription", "validate", cfgPath)
	if err != nil {
		t.Fatalf("subscription validate: %v", err)
	}
	if !strings.Contains(stdout, "Configuration valid") {
		t.Fatalf("expected validation output, got: %s", stdout)
	}
}
