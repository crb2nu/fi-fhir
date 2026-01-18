package main

import (
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
		"ZPV|foo|bar\r"
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

	stdout, _, err = runCLI(t, "profile", "lint", "--profile", profilePath, "--samples", samplePath)
	if err != nil {
		t.Fatalf("profile lint: %v", err)
	}
	if !strings.Contains(stdout, "passed lint") {
		t.Fatalf("expected lint success output, got: %s", stdout)
	}
}
