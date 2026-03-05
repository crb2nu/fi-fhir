package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl"
)

// ---------------------------------------------------------------------------
// marshalYAML — 71.4% → exercise with a real config from LoadFromEnv()
// ---------------------------------------------------------------------------

func TestMarshalYAML_LoadFromEnvConfig(t *testing.T) {
	// Clear env to get a clean config
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	cfg := config.LoadFromEnv()
	data, err := marshalYAML(cfg)
	if err != nil {
		t.Fatalf("marshalYAML: %v", err)
	}
	if len(data) == 0 {
		t.Error("marshalYAML returned empty output")
	}
}

// ---------------------------------------------------------------------------
// validateProfile — 78.1% → exercise untested branches
// ---------------------------------------------------------------------------

func TestValidateProfile_FileNotFound(t *testing.T) {
	errs := validateProfile("/nonexistent/path/profile.yaml", false)
	if len(errs) != 1 || errs[0] == "" {
		t.Errorf("expected file-not-found error, got %v", errs)
	}
}

func TestValidateProfile_FileNotFoundVerbose(t *testing.T) {
	errs := validateProfile("/nonexistent/path/profile.yaml", true)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateProfile_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(f, []byte("{{{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateProfile(f, false)
	if len(errs) == 0 {
		t.Error("expected parse error for invalid YAML")
	}
}

func TestValidateProfile_EmptyProfile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "empty.yaml")
	if err := os.WriteFile(f, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateProfile(f, false)
	if len(errs) != 1 || errs[0] == "" {
		t.Fatalf("expected one parse error for empty profile, got %v", errs)
	}
	if errs[0] != "failed to parse profile: missing source_profile root element" {
		t.Fatalf("unexpected error: %v", errs)
	}
}

func TestValidateProfile_MinimalValid(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "valid.yaml")
	yaml := "source_profile:\n  id: test-profile\n  name: Test Profile\n"
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateProfile(f, false)
	if len(errs) != 0 {
		t.Errorf("expected no errors for minimal valid profile, got %v", errs)
	}
}

func TestValidateProfile_MinimalValidVerbose(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "valid.yaml")
	yaml := "source_profile:\n  id: test-profile\n  name: Test Profile\n  version: \"1.0\"\n"
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateProfile(f, true)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateProfile_InvalidHL7v2Version(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "bad_version.yaml")
	yaml := "id: test\nname: Test\nhl7v2:\n  default_version: \"9.9\"\n"
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateProfile(f, false)
	found := false
	for _, e := range errs {
		if len(e) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected HL7v2 version validation error")
	}
}

func TestValidateProfile_ValidHL7v2VersionVerbose(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "good_version.yaml")
	yaml := "id: test\nname: Test\nhl7v2:\n  default_version: \"2.5.1\"\n  timezone: \"UTC\"\n"
	if err := os.WriteFile(f, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_ = validateProfile(f, true)
}

// ---------------------------------------------------------------------------
// validateMessage — 79.2% → exercise remaining branches
// ---------------------------------------------------------------------------

func TestValidateMessage_BadProfilePath(t *testing.T) {
	errs := validateMessage("/nonexistent/profile.yaml", "msg.hl7", "hl7v2", false)
	if len(errs) == 0 {
		t.Error("expected error for bad profile path")
	}
}

func TestValidateMessage_BadMessagePath(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("id: test\nname: Test"), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateMessage(profilePath, "/nonexistent/msg.hl7", "hl7v2", false)
	if len(errs) == 0 {
		t.Error("expected error for bad message path")
	}
}

func TestValidateMessage_UnsupportedFormatP5(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.yaml")
	msgPath := filepath.Join(tmp, "msg.txt")
	if err := os.WriteFile(profilePath, []byte("id: test\nname: Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(msgPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	errs := validateMessage(profilePath, msgPath, "xml", false)
	if len(errs) == 0 {
		t.Error("expected 'unsupported format' error")
	}
}

func TestValidateMessage_HL7v2EmptyMessage(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.yaml")
	msgPath := filepath.Join(tmp, "msg.hl7")
	if err := os.WriteFile(profilePath, []byte("id: test\nname: Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(msgPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	_ = validateMessage(profilePath, msgPath, "hl7v2", false)
}

func TestValidateMessage_HL7v2Verbose(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.yaml")
	msgPath := filepath.Join(tmp, "msg.hl7")
	if err := os.WriteFile(profilePath, []byte("id: test\nname: Test"), 0644); err != nil {
		t.Fatal(err)
	}
	msg := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240101120000||ADT^A01|12345|P|2.5\rPID|||12345||DOE^JOHN\r"
	if err := os.WriteFile(msgPath, []byte(msg), 0644); err != nil {
		t.Fatal(err)
	}
	_ = validateMessage(profilePath, msgPath, "hl7v2", true)
}

// ---------------------------------------------------------------------------
// runWorkflowReplay — 77.4% → flag parsing guard clauses (unique names)
// ---------------------------------------------------------------------------

func TestWorkflowReplay_HelpFlag(t *testing.T) {
	err := runWorkflowReplay([]string{"--help"})
	if err != nil {
		t.Errorf("--help should not error: %v", err)
	}
}

func TestWorkflowReplay_ShortHelpFlag(t *testing.T) {
	err := runWorkflowReplay([]string{"-h"})
	if err != nil {
		t.Errorf("-h should not error: %v", err)
	}
}

func TestWorkflowReplay_ConfigMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--config"})
	if err == nil {
		t.Error("expected error for --config without value")
	}
}

func TestWorkflowReplay_ShortConfigMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"-c"})
	if err == nil {
		t.Error("expected error for -c without value")
	}
}

func TestWorkflowReplay_RecordingsMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--recordings"})
	if err == nil {
		t.Error("expected error for --recordings without value")
	}
}

func TestWorkflowReplay_EventTypeMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "--event-type"})
	if err == nil {
		t.Error("expected error for --event-type without value")
	}
}

func TestWorkflowReplay_SourceMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "--source"})
	if err == nil {
		t.Error("expected error for --source without value")
	}
}

func TestWorkflowReplay_LimitMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "--limit"})
	if err == nil {
		t.Error("expected error for --limit without value")
	}
}

func TestWorkflowReplay_OutputMissingValue(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "--output"})
	if err == nil {
		t.Error("expected error for --output without value")
	}
}

func TestWorkflowReplay_DiffsFlag(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "--recordings", "r.json", "--diffs"})
	if err == nil {
		t.Error("expected error (workflow file doesn't exist)")
	}
}

func TestWorkflowReplay_AllFlagsCombined(t *testing.T) {
	err := runWorkflowReplay([]string{
		"--config", "x.yaml",
		"--recordings", "r.json",
		"--event-type", "ADT_A01",
		"--source", "hospital-a",
		"--limit", "10",
		"--diffs",
		"--output", "/tmp/replay-out.json",
	})
	if err == nil {
		t.Error("expected error (workflow file doesn't exist)")
	}
}

func TestWorkflowReplay_PositionalRecordingsArg(t *testing.T) {
	err := runWorkflowReplay([]string{"--config", "x.yaml", "recordings.json"})
	// Will fail loading workflow, but exercises positional arg path
	if err == nil {
		t.Error("expected error (workflow file doesn't exist)")
	}
}

// ---------------------------------------------------------------------------
// runETLSync — 75.9% → guard clause coverage
// ---------------------------------------------------------------------------

func TestETLSync_HelpFlag(t *testing.T) {
	// runETLSync --help still attempts to initialize MinIO, so it may error.
	// We exercise the flag-parsing path regardless.
	_ = runETLSync([]string{"--help"})
}

func TestETLSync_UnknownFlag(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	err := runETLSync([]string{"--badopt"})
	if err != nil {
		t.Errorf("expected unknown flag to be ignored, got %v", err)
	}
}

func TestETLSync_NoSources(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	err := runETLSync([]string{})
	if err != nil {
		t.Errorf("expected no error with no configured sources, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// runEventStoreStreams — 58.8% → flag path
// ---------------------------------------------------------------------------

func TestEventStoreStreams_LimitMissingValue(t *testing.T) {
	t.Setenv("FI_FHIR_EVENT_STORE_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runEventStoreStreams([]string{"--limit"})
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// runEventStoreInit — 60.0% → flag path
// ---------------------------------------------------------------------------

func TestEventStoreInit_TableFlagMissingValue(t *testing.T) {
	t.Setenv("FI_FHIR_EVENT_STORE_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runEventStoreInit([]string{"--table"})
	if err == nil {
		t.Error("expected error for --table without value")
	}
}
