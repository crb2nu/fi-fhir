package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubscriptionServe_DryRun_UsesReceiverConfig(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")
	appCfgPath := filepath.Join(tmpDir, "app.yaml")

	subs := `subscriptions:
  - name: patient_changes
    server: https://example.invalid/fhir
    criteria: Patient?status=active
    channel:
      endpoint: https://receiver.example.invalid/fhir/notify/patient_changes
`
	if err := os.WriteFile(subsPath, []byte(subs), 0o600); err != nil {
		t.Fatalf("write subscriptions: %v", err)
	}

	appCfg := `subscription_receiver:
  host: 127.0.0.1
  port: 9099
  path_prefix: /notify
  max_bundle_size: 42
  verify_source: true
  allowed_sources:
    - https://example.invalid/fhir
`
	if err := os.WriteFile(appCfgPath, []byte(appCfg), 0o600); err != nil {
		t.Fatalf("write app config: %v", err)
	}

	stdout, _, err := runCLI(t,
		"subscription", "serve",
		"--subscriptions", subsPath,
		"--config", appCfgPath,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("subscription serve --dry-run: %v", err)
	}
	assertContains(t, stdout, "Dry-run")
	assertContains(t, stdout, "Bind: 127.0.0.1:9099")
	assertContains(t, stdout, "Path prefix: /notify")
	assertContains(t, stdout, "Max bundle size: 42")
	assertContains(t, stdout, "Verify source: true")
}

func TestSubscriptionServe_DryRun_FlagsOverrideHostPort(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")
	appCfgPath := filepath.Join(tmpDir, "app.yaml")

	subs := `subscriptions:
  - name: patient_changes
    server: https://example.invalid/fhir
    criteria: Patient?status=active
    channel:
      endpoint: https://receiver.example.invalid/fhir/notify/patient_changes
`
	if err := os.WriteFile(subsPath, []byte(subs), 0o600); err != nil {
		t.Fatalf("write subscriptions: %v", err)
	}

	appCfg := `subscription_receiver:
  host: 127.0.0.1
  port: 9099
  path_prefix: /notify
`
	if err := os.WriteFile(appCfgPath, []byte(appCfg), 0o600); err != nil {
		t.Fatalf("write app config: %v", err)
	}

	stdout, _, err := runCLI(t,
		"subscription", "serve",
		"--subscriptions", subsPath,
		"--config", appCfgPath,
		"--host", "0.0.0.0",
		"--port", "8081",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("subscription serve --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "Bind: 0.0.0.0:8081") {
		t.Fatalf("expected host/port override, got: %s", stdout)
	}
}
