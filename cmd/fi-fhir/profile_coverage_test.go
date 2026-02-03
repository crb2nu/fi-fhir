package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Profile subcommand dispatch tests
// ============================================================================

func TestProfile_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "profile", "badcmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown profile subcommand")
}

// ============================================================================
// Profile Infer flag and argument tests
// ============================================================================

func TestProfileInfer_NoSamples_ReturnsError(t *testing.T) {
	_, _, err := runCLI(t, "profile", "infer")
	assertError(t, err)
	assertErrorContains(t, err, "no samples provided")
}

func TestProfileInfer_UnsupportedFormat_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	if err := os.WriteFile(samplePath, []byte("MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	_, _, err := runCLI(t, "profile", "infer", "--format", "xml", samplePath)
	assertError(t, err)
	assertErrorContains(t, err, "unsupported format")
}

func TestProfileInfer_UnknownFlag_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	if err := os.WriteFile(samplePath, []byte("MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	_, _, err := runCLI(t, "profile", "infer", "--notaflag", samplePath)
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestProfileInfer_FlagMissingValue(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{"id", "--id"},
		{"name", "--name"},
		{"version", "--version"},
		{"timezone", "--timezone"},
		{"format", "--format"},
		{"out", "--out"},
		{"max-files", "--max-files"},
	}

	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	if err := os.WriteFile(samplePath, []byte("MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Flag at end of args without value
			_, _, err := runCLI(t, "profile", "infer", samplePath, tt.flag)
			assertError(t, err)
			assertErrorContains(t, err, "requires a value")
		})
	}
}

func TestProfileInfer_MaxFilesInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	if err := os.WriteFile(samplePath, []byte("MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	tests := []struct {
		name  string
		value string
	}{
		{"negative", "-5"},
		{"zero", "0"},
		{"not_a_number", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(t, "profile", "infer", "--max-files", tt.value, samplePath)
			assertError(t, err)
			assertErrorContains(t, err, "must be a positive integer")
		})
	}
}

func TestProfileInfer_OutputToFile(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	outPath := filepath.Join(tmpDir, "profile_out.yaml")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "infer", "--id", "test_out", "--out", outPath, samplePath)
	assertNoError(t, err)

	// stdout should be empty when writing to file
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected empty stdout when using --out, got: %s", stdout)
	}

	// Output file should exist and contain valid YAML
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(data), "source_profile:") {
		t.Errorf("output file missing source_profile header")
	}
	if !strings.Contains(string(data), "id: test_out") {
		t.Errorf("output file missing expected id")
	}
}

func TestProfileInfer_Verbose(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	_, stderr, err := runCLI(t, "profile", "infer", "--verbose", "--id", "test_verbose", samplePath)
	assertNoError(t, err)

	// Verbose mode should print inference summary to stderr
	if !strings.Contains(stderr, "Inferred from") {
		t.Errorf("expected verbose output on stderr, got: %s", stderr)
	}
}

func TestProfileInfer_WithTimezone(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "infer", "--id", "tz_test", "--timezone", "America/New_York", samplePath)
	assertNoError(t, err)
	assertContains(t, stdout, "America/New_York")
}

func TestProfileInfer_FormatHL7Alias(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	// "hl7" is an alias for "hl7v2"
	stdout, _, err := runCLI(t, "profile", "infer", "--format", "hl7", "--id", "alias_test", samplePath)
	assertNoError(t, err)
	assertContains(t, stdout, "source_profile:")
}

func TestProfileInfer_MultipleSamples(t *testing.T) {
	tmpDir := t.TempDir()
	sample1 := filepath.Join(tmpDir, "sample1.hl7")
	sample2 := filepath.Join(tmpDir, "sample2.hl7")

	msg1 := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	msg2 := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180931||ADT^A02|MSG0002|P|2.5.1\r" +
		"PID|1||67890^^^MRN||Smith^Jane\r"

	if err := os.WriteFile(sample1, []byte(msg1), 0o600); err != nil {
		t.Fatalf("write sample1: %v", err)
	}
	if err := os.WriteFile(sample2, []byte(msg2), 0o600); err != nil {
		t.Fatalf("write sample2: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "infer", "--id", "multi_test", "--verbose", sample1, sample2)
	assertNoError(t, err)
	assertContains(t, stdout, "source_profile:")
}

func TestProfileInfer_DirectoryInput(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "samples")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(filepath.Join(subDir, "test.hl7"), []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "infer", "--id", "dir_test", subDir)
	assertNoError(t, err)
	assertContains(t, stdout, "source_profile:")
}

func TestProfileInfer_MaxFilesLimit(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	// Valid max-files value should work
	stdout, _, err := runCLI(t, "profile", "infer", "--max-files", "10", "--id", "maxfiles_test", samplePath)
	assertNoError(t, err)
	assertContains(t, stdout, "source_profile:")
}

func TestProfileInfer_ShortFlags(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	outPath := filepath.Join(tmpDir, "out.yaml")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	// Test short flags: -f, -o, -v
	_, stderr, err := runCLI(t, "profile", "infer", "-f", "hl7v2", "-o", outPath, "-v", "--id", "short_test", samplePath)
	assertNoError(t, err)
	assertContains(t, stderr, "Inferred from")

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	assertContains(t, string(data), "source_profile:")
}

// ============================================================================
// Profile Lint flag and argument tests
// ============================================================================

func TestProfileLint_NoProfile_ReturnsError(t *testing.T) {
	_, _, err := runCLI(t, "profile", "lint")
	assertError(t, err)
	assertErrorContains(t, err, "no profile specified")
}

func TestProfileLint_ProfileNotFound_ReturnsError(t *testing.T) {
	_, _, err := runCLI(t, "profile", "lint", "--profile", "/nonexistent/profile.yaml")
	assertError(t, err)
	// Error message will mention the file not existing
}

func TestProfileLint_UnknownFlag_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
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

	_, _, err := runCLI(t, "profile", "lint", "--badoption", profilePath)
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestProfileLint_FlagMissingValue(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{"profile", "--profile"},
		{"samples", "--samples"},
		{"format", "--format"},
		{"max-files", "--max-files"},
	}

	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(t, "profile", "lint", profilePath, tt.flag)
			assertError(t, err)
			assertErrorContains(t, err, "requires a value")
		})
	}
}

func TestProfileLint_MaxFilesInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
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

	tests := []struct {
		name  string
		value string
	}{
		{"negative", "-5"},
		{"zero", "0"},
		{"not_a_number", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(t, "profile", "lint", "--max-files", tt.value, profilePath)
			assertError(t, err)
			assertErrorContains(t, err, "must be a positive integer")
		})
	}
}

func TestProfileLint_Verbose(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
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
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	_, stderr, err := runCLI(t, "profile", "lint", "--profile", profilePath, "--samples", samplePath, "--verbose", "--allow-warnings")
	assertNoError(t, err)
	// Verbose mode should print sample stats to stderr
	assertContains(t, stderr, "Sample stats")
}

func TestProfileLint_PositionalProfile(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Profile as positional arg (not via --profile flag)
	stdout, _, err := runCLI(t, "profile", "lint", profilePath)
	assertNoError(t, err)
	assertContains(t, stdout, "passed lint")
}

func TestProfileLint_UnexpectedArg_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")

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

	// Two positional args should fail
	_, _, err := runCLI(t, "profile", "lint", profilePath, "extra_arg")
	assertError(t, err)
	assertErrorContains(t, err, "unexpected argument")
}

func TestProfileLint_StrictModeDefault(t *testing.T) {
	// Test that warnings cause failure in strict mode (the default)
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	// A sample that will generate warnings due to missing expected segments
	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	// Profile that expects more segments
	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
    message_types:
      ADT_A01:
        segments:
          - segment: MSH
            min: 1
            max: 1
          - segment: PID
            min: 1
            max: 1
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Should fail due to warnings in strict mode
	_, _, err := runCLI(t, "profile", "lint", profilePath, "--samples", samplePath)
	assertError(t, err)
}

func TestProfileLint_ShortFlags(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
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
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Test short flags: -p, -f, -v
	_, stderr, err := runCLI(t, "profile", "lint", "-p", profilePath, "-f", "hl7v2", "-v", "--samples", samplePath, "--allow-warnings")
	assertNoError(t, err)
	assertContains(t, stderr, "Sample stats")
}

func TestProfileLint_JSONOutput_WithWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	// Message with PV1 for valid parse (note: may still generate warnings)
	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
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
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "lint", profilePath, "--samples", samplePath, "--json", "--allow-warnings")
	assertNoError(t, err)

	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, stdout)
	}
	// Should have profile field
	if _, ok := got["profile"]; !ok {
		t.Errorf("expected profile field in JSON output")
	}
}

func TestProfileLint_JSONOutput_StructuredFields(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	profileYAML := `source_profile:
  id: json_test
  name: JSON Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "lint", profilePath, "--samples", samplePath, "--json", "--allow-warnings")
	assertNoError(t, err)

	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, stdout)
	}

	// Verify expected JSON structure
	if got["profile"] != profilePath {
		t.Errorf("expected profile=%s, got %v", profilePath, got["profile"])
	}
	if got["format"] != "hl7v2" {
		t.Errorf("expected format=hl7v2, got %v", got["format"])
	}
	if _, ok := got["ok"].(bool); !ok {
		t.Errorf("expected ok to be boolean")
	}
}

func TestProfileLint_NoSamples_PassesLint(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")

	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Lint without samples should pass (just validates profile structure)
	stdout, _, err := runCLI(t, "profile", "lint", profilePath)
	assertNoError(t, err)
	assertContains(t, stdout, "passed lint")
}

func TestProfileLint_DirectorySamples(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplesDir := filepath.Join(tmpDir, "samples")

	if err := os.MkdirAll(samplesDir, 0o755); err != nil {
		t.Fatalf("create samples dir: %v", err)
	}

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
	if err := os.WriteFile(filepath.Join(samplesDir, "test.hl7"), []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	profileYAML := `source_profile:
  id: test
  name: Test
  version: "0.1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "UTC"
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	stdout, _, err := runCLI(t, "profile", "lint", profilePath, "--samples", samplesDir, "--allow-warnings")
	assertNoError(t, err)
	assertContains(t, stdout, "passed lint")
}

func TestProfileLint_MaxFilesLimit(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"PV1|1|I|ER^1^A\r"
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
    encoding:
      charset_default: UTF-8
      charset_detection: true
      line_ending_mode: tolerant
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Valid max-files should work
	stdout, _, err := runCLI(t, "profile", "lint", profilePath, "--samples", samplePath, "--max-files", "5", "--allow-warnings")
	assertNoError(t, err)
	assertContains(t, stdout, "passed lint")
}

// ============================================================================
// Profile lint error path tests
// ============================================================================

func TestProfileLint_InvalidYAML_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "bad.yaml")

	// Invalid YAML (bad indentation)
	badYAML := `source_profile:
id: test
  name: Test
`
	if err := os.WriteFile(profilePath, []byte(badYAML), 0o600); err != nil {
		t.Fatalf("write bad profile: %v", err)
	}

	_, _, err := runCLI(t, "profile", "lint", profilePath)
	assertError(t, err)
}

func TestProfileLint_MissingRequiredFields_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "incomplete.yaml")

	// Missing required fields
	incompleteYAML := `source_profile:
  id: test
`
	if err := os.WriteFile(profilePath, []byte(incompleteYAML), 0o600); err != nil {
		t.Fatalf("write incomplete profile: %v", err)
	}

	_, _, err := runCLI(t, "profile", "lint", profilePath)
	// Should error due to missing fields
	assertError(t, err)
}
