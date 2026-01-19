package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileInferAndLintCLI(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A^FAC\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "infer", "--id", "test_profile", "--name", "Test Profile", samplePath)
	if err != nil {
		t.Fatalf("profile infer: %v", err)
	}
	if !strings.Contains(stdout, "source_profile:") {
		t.Fatalf("expected YAML output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "id: test_profile") {
		t.Fatalf("expected id in YAML output, got: %s", stdout)
	}

	profilePath := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte(stdout), 0o600); err != nil {
		t.Fatalf("write inferred profile: %v", err)
	}

	stdout, _, err = runCLI(t, "profile", "lint", "--profile", profilePath, "--samples", samplePath, "--allow-warnings")
	if err != nil {
		t.Fatalf("profile lint: %v", err)
	}
	if !strings.Contains(stdout, "passed lint") {
		t.Fatalf("expected lint success output, got: %s", stdout)
	}
}

func TestProfileLintFailsOnInvalidSample(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "bad.hl7")
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	// Not an HL7 message (missing MSH).
	if err := os.WriteFile(samplePath, []byte("NOTHL7\n"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	// Minimal valid profile.
	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	_, _, err := runCLI(t, "profile", "lint", "--profile", profilePath, "--samples", samplePath, "--strict")
	if err == nil {
		t.Fatalf("expected lint error")
	}
}

func TestProfileLintJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "lint", "--profile", profilePath, "--samples", samplePath, "--json", "--allow-warnings")
	if err != nil {
		t.Fatalf("profile lint --json: %v", err)
	}

	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\n%s", jsonErr, stdout)
	}
	if ok, _ := got["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got: %v", got["ok"])
	}
}
