package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintProfileFile_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	// Missing id
	yaml := `source_profile:
  name: No ID
  hl7v2:
    default_version: "2.5.1"
`
	if err := os.WriteFile(profilePath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	report, err := LintProfileFile(profilePath, LintOptions{})
	if err != nil {
		t.Fatalf("LintProfileFile: %v", err)
	}
	if len(report.Errors) == 0 {
		t.Fatalf("expected errors, got none")
	}
}

func TestLintProfileFile_WithSamples_WarnsMissingZMappings(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	yaml := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
`
	if err := os.WriteFile(profilePath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"ZPV|foo|bar\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	report, err := LintProfileFile(profilePath, LintOptions{
		Format:      "hl7v2",
		SamplesPath: samplePath,
	})
	if err != nil {
		t.Fatalf("LintProfileFile: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("expected no errors, got: %v", report.Errors)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected warnings, got none")
	}
}
