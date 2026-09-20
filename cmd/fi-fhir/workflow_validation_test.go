package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowCommandsRejectInvalidConfigurationBeforeInput(t *testing.T) {
	tests := []struct {
		fixture string
		issues  []string
	}{
		{"invalid-cel", []string{"error [INVALID_CEL] routes[0].filter.condition"}},
		{"undeclared-cel", []string{"error [INVALID_CEL] routes[0].filter.condition", "undeclared reference"}},
		{"invalid-action-transform", []string{"error [MISSING_WEBHOOK_URL] routes[0].actions[0].url", "error [INVALID_SET_FIELD] routes[0].transform[0].set_field"}},
		{"missing-routes", []string{"workflow must have at least one route"}},
		{"missing-actions", []string{"must have at least one action"}},
	}
	for _, command := range []string{"validate", "run", "dry-run", "consume"} {
		for _, tt := range tests {
			t.Run(command+"/"+tt.fixture, func(t *testing.T) {
				config := filepath.Join("..", "..", "testdata", "workflows", "validation", tt.fixture+".yaml")
				args := []string{"workflow", command, config}
				if command != "validate" {
					// An unreadable input must not mask the configuration error.
					args = []string{"workflow", command, "--config", config, filepath.Join(t.TempDir(), "missing.json")}
				}
				if command == "consume" {
					backend := filepath.Join(t.TempDir(), "backend.yaml")
					// Opening this backend fails; workflow diagnostics must come first.
					if err := os.WriteFile(backend, []byte("driver: unsupported\nsubscription: events\n"), 0600); err != nil {
						t.Fatal(err)
					}
					args = []string{"workflow", command, "--config", config, "--backend", backend}
				}

				stdout, stderr, err := runCLI(t, args...)
				if err == nil || !strings.Contains(err.Error(), "workflow validation failed") {
					t.Fatalf("expected validation failure before input read, got %v", err)
				}
				if stdout != "" {
					t.Errorf("invalid workflow produced success output: %s", stdout)
				}
				for _, issue := range tt.issues {
					if !strings.Contains(stderr, issue) {
						t.Errorf("stderr missing %q: %s", issue, stderr)
					}
				}
			})
		}
	}
}

func TestWorkflowCommandsAcceptValidConfigurationWithWarnings(t *testing.T) {
	for _, command := range []string{"validate", "run", "dry-run"} {
		for _, fixture := range []string{"valid", "warnings"} {
			t.Run(command+"/"+fixture, func(t *testing.T) {
				config := filepath.Join("..", "..", "testdata", "workflows", "validation", fixture+".yaml")
				args := []string{"workflow", command, config}
				if command != "validate" {
					events := writeEventsFile(t, t.TempDir(), []map[string]interface{}{{"type": "patient_admit"}})
					args = []string{"workflow", command, "--config", config, events}
				}
				stdout, stderr, err := runCLI(t, args...)
				if err != nil {
					t.Fatalf("valid workflow rejected: %v\n%s", err, stderr)
				}
				want := map[string]string{"validate": "is valid", "run": "1 route matches, 0 errors", "dry-run": "MATCH - would run 1 action(s)"}[command]
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout missing %q: %s", want, stdout)
				}
				if fixture == "warnings" {
					for _, issue := range []string{"warning [MISSING_VERSION] version", "warning [NO_FILTER] routes[0].filter"} {
						if !strings.Contains(stderr, issue) {
							t.Errorf("stderr missing %q: %s", issue, stderr)
						}
					}
				} else if strings.Contains(stderr, "Validation diagnostics:") {
					t.Errorf("unexpected diagnostics: %s", stderr)
				}
			})
		}
	}
}
