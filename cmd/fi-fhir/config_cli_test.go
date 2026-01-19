package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShow_DefaultYAML(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("expected output")
	}
}

func TestConfigShow_JSONFormat(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show", "--format", "json")
	if err != nil {
		t.Fatalf("config show --format json: %v", err)
	}
	if !strings.Contains(stdout, "{") {
		t.Fatalf("expected JSON-ish output, got: %s", stdout)
	}
}

func TestConfigValidate_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := `server:
  port: 8080
workflow:
  config_path: workflow.yaml
observability:
  log_level: info
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, _, err := runCLI(t, "config", "validate", cfgPath)
	if err != nil {
		t.Fatalf("config validate: %v", err)
	}
	if !strings.Contains(stdout, "is valid") {
		t.Fatalf("expected success output, got: %s", stdout)
	}
}

func TestConfigEnv_ExportFormat(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "--format", "export")
	if err != nil {
		t.Fatalf("config env --format export: %v", err)
	}
	if !strings.Contains(stdout, "export FI_FHIR_") {
		t.Fatalf("expected export output, got: %s", stdout)
	}
}

func TestConfigEnv_SectionFilter(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "--section", "server")
	if err != nil {
		t.Fatalf("config env --section server: %v", err)
	}
	if !strings.Contains(stdout, "FI_FHIR_SERVER_") {
		t.Fatalf("expected server vars, got: %s", stdout)
	}
	if strings.Contains(stdout, "FI_FHIR_WORKFLOW_") {
		t.Fatalf("did not expect workflow vars in server section output")
	}
}

func TestConfigInit_WritesMinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "config.yaml")

	stdout, _, err := runCLI(t, "config", "init", "--minimal", "--output", outPath)
	if err != nil {
		t.Fatalf("config init --minimal: %v", err)
	}
	if !strings.Contains(stdout, "Created") {
		t.Fatalf("expected create output, got: %s", stdout)
	}

	b, err := os.ReadFile(outPath) //nolint:gosec // G304: test fixture
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	if !strings.Contains(string(b), "fi-fhir minimal configuration") {
		t.Fatalf("expected minimal config header")
	}
}
