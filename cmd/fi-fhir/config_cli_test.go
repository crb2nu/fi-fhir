package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
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

func TestConfigShow_ConfigFlagIgnoresExtraPositionalConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	primaryPath := filepath.Join(tmpDir, "primary.yaml")
	secondaryPath := filepath.Join(tmpDir, "secondary.yaml")

	primaryConfig := `server:
  host: primary.example
  port: 8011
`
	secondaryConfig := `server:
  host: secondary.example
  port: 9022
`

	if err := os.WriteFile(primaryPath, []byte(primaryConfig), 0o600); err != nil {
		t.Fatalf("write primary config: %v", err)
	}
	if err := os.WriteFile(secondaryPath, []byte(secondaryConfig), 0o600); err != nil {
		t.Fatalf("write secondary config: %v", err)
	}

	stdout, _, err := runCLI(t, "config", "show", "--config", primaryPath, secondaryPath)
	if err != nil {
		t.Fatalf("config show with extra positional config path: %v", err)
	}
	if !strings.Contains(stdout, "primary.example") {
		t.Fatalf("expected output from primary config, got: %s", stdout)
	}
	if strings.Contains(stdout, "secondary.example") {
		t.Fatalf("did not expect output from extra positional config path")
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

func TestConfigValidate_InvalidFileContent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid-config.yaml")

	cfg := `server:
  port: 0
  read_timeout: 0s
workflow:
  max_concurrency: 0
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, stderr, err := runCLI(t, "config", "validate", cfgPath)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("expected validation failure, got: %v", err)
	}
	if !strings.Contains(stderr, "Configuration validation failed") {
		t.Fatalf("expected validation details in stderr, got: %s", stderr)
	}
}

func TestConfigShow_InvalidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid-config.yaml")

	if err := os.WriteFile(cfgPath, []byte("server: ["), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	_, _, err := runCLI(t, "config", "show", "--config", cfgPath)
	if err == nil {
		t.Fatalf("expected load error for invalid config")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestConfigShow_YAMLMarshalFailure(t *testing.T) {
	original := marshalConfigYAML
	t.Cleanup(func() {
		marshalConfigYAML = original
	})

	marshalConfigYAML = func(*config.Config) ([]byte, error) {
		return nil, errors.New("forced yaml marshal error")
	}

	_, _, err := runCLI(t, "config", "show")
	if err == nil {
		t.Fatalf("expected marshal error")
	}
	if !strings.Contains(err.Error(), "failed to marshal config") {
		t.Fatalf("expected marshal failure, got: %v", err)
	}
}

func TestConfigShow_JSONMarshalFailure(t *testing.T) {
	original := marshalConfigJSON
	t.Cleanup(func() {
		marshalConfigJSON = original
	})

	marshalConfigJSON = func(interface{}) ([]byte, error) {
		return nil, errors.New("forced json marshal error")
	}

	_, _, err := runCLI(t, "config", "show", "--format", "json")
	if err == nil {
		t.Fatalf("expected marshal error")
	}
	if !strings.Contains(err.Error(), "failed to marshal config") {
		t.Fatalf("expected marshal failure, got: %v", err)
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

func TestConfigEnv_MissingFormatValue(t *testing.T) {
	_, _, err := runCLI(t, "config", "env", "--format")
	if err == nil {
		t.Fatalf("expected missing format value error")
	}
	if !strings.Contains(err.Error(), "--format requires a value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigEnv_MissingSectionValue(t *testing.T) {
	_, _, err := runCLI(t, "config", "env", "--section")
	if err == nil {
		t.Fatalf("expected missing section value error")
	}
	if !strings.Contains(err.Error(), "--section requires a value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigEnv_UnknownSection(t *testing.T) {
	_, _, err := runCLI(t, "config", "env", "--section", "does-not-exist")
	if err == nil {
		t.Fatalf("expected unknown section error")
	}
	if !strings.Contains(err.Error(), "unknown section") {
		t.Fatalf("unexpected error: %v", err)
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

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	if !strings.Contains(string(b), "fi-fhir minimal configuration") {
		t.Fatalf("expected minimal config header")
	}
}
