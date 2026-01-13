package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/etl"
	"github.com/crb2nu/fi-fhir/pkg/storage"
)

// Stub implementations to exercise ETL sync logic without external services.
type stubSource struct {
	name     string
	versions []etl.VersionInfo
	payload  []byte
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) AvailableVersions(ctx context.Context) ([]etl.VersionInfo, error) {
	return s.versions, nil
}

func (s *stubSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	n, err := w.Write(s.payload)
	return int64(n), err
}

func (s *stubSource) Validate(ctx context.Context) error { return nil }

type stubSink struct {
	exists bool
	writes int
	data   []byte
}

func (s *stubSink) Name() string { return "stub" }

func (s *stubSink) Write(ctx context.Context, path string, r io.Reader, _ int64) error {
	s.writes++
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	s.data = buf.Bytes()
	return err
}

func (s *stubSink) Exists(ctx context.Context, path string) (bool, error) { return s.exists, nil }

func (s *stubSink) Validate(ctx context.Context) error { return nil }

func (s *stubSink) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	return []storage.FileInfo{
		{Path: prefix + "v1", LastModified: time.Unix(0, 0)},
	}, nil
}

func TestSyncSource_DryRunUsesLatestVersion(t *testing.T) {
	src := &stubSource{
		name: "stub",
		versions: []etl.VersionInfo{
			{Version: "v1", IsLatest: true},
		},
		payload: []byte("data"),
	}
	snk := &stubSink{}

	err := syncSource(src, snk, "", true, false)
	if err != nil {
		t.Fatalf("syncSource returned error: %v", err)
	}
	if snk.writes != 0 {
		t.Fatalf("expected no writes on dry run, got %d", snk.writes)
	}
}

func TestSyncSource_DownloadsToSink(t *testing.T) {
	src := &stubSource{
		name: "stub",
		versions: []etl.VersionInfo{
			{Version: "v1", IsLatest: true},
		},
		payload: []byte("hello"),
	}
	snk := &stubSink{exists: false}

	err := syncSource(src, snk, "v1", false, false)
	if err != nil {
		t.Fatalf("syncSource returned error: %v", err)
	}
	if snk.writes != 1 {
		t.Fatalf("expected 1 write, got %d", snk.writes)
	}
	if string(snk.data) != "hello" {
		t.Fatalf("unexpected sink data: %q", string(snk.data))
	}
}

func TestCreateMinIOSink_MissingCredentials(t *testing.T) {
	// Clear env to force credential error path.
	oldAccess := os.Getenv("MINIO_ACCESS_KEY")
	oldSecret := os.Getenv("MINIO_SECRET_KEY")
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	defer func() {
		os.Setenv("MINIO_ACCESS_KEY", oldAccess)
		os.Setenv("MINIO_SECRET_KEY", oldSecret)
	}()

	_, err := createMinIOSink()
	if err == nil || !contains(err.Error(), "MINIO_ACCESS_KEY") {
		t.Fatalf("expected credential error, got %v", err)
	}
}

func TestRunETLSync_WithStubSink(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	src := &stubSource{
		name: "stubsrc",
		versions: []etl.VersionInfo{
			{Version: "v1", IsLatest: true},
		},
		payload: []byte("payload"),
	}
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{"stubsrc": src} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	if err := runETL([]string{"sync", "--source", "stubsrc", "--dry-run"}); err != nil {
		t.Fatalf("runETL sync with stub sink failed: %v", err)
	}
}

func TestRunETLStatus_WithStubSink(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"stubsrc": &stubSource{name: "stubsrc", versions: []etl.VersionInfo{{Version: "v1", IsLatest: true}}},
		}
	}
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	if err := runETL([]string{"status"}); err != nil {
		t.Fatalf("runETL status with stub sink failed: %v", err)
	}
}

func TestRunETLLoad_MissingVersion(t *testing.T) {
	err := runETLLoad([]string{"loinc", "/data/path"})
	if err == nil || !contains(err.Error(), "--version is required") {
		t.Fatalf("expected version required error, got %v", err)
	}
}

func TestRunETLLoad_MissingDB(t *testing.T) {
	// Ensure DB env vars are cleared to hit the expected validation branch.
	oldDB := os.Getenv("FI_FHIR_DATABASE_URL")
	oldDB2 := os.Getenv("DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("DATABASE_URL")
	defer func() {
		if oldDB != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldDB)
		}
		if oldDB2 != "" {
			os.Setenv("DATABASE_URL", oldDB2)
		}
	}()

	err := runETLLoad([]string{"loinc", "/data/path", "--version", "2.77"})
	if err == nil || (!contains(err.Error(), "database URL") && !contains(err.Error(), "environment variable is required")) {
		t.Fatalf("expected database URL error, got %v", err)
	}
}

func TestRunTerminologyDrop_RequiresForce(t *testing.T) {
	// Set dummy DB URL so function passes initial check and exercises force guard.
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	defer os.Unsetenv("FI_FHIR_DATABASE_URL")

	err := runTerminologyDrop([]string{"--db", "postgres://example/test"})
	if err != nil {
		t.Fatalf("expected drop to short-circuit without --force, got %v", err)
	}
}

func TestRunTerminologyLoad_MissingVersion(t *testing.T) {
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	defer os.Unsetenv("FI_FHIR_DATABASE_URL")

	err := runTerminologyLoad([]string{"loinc", "/data/path"})
	if err == nil || !contains(err.Error(), "--version is required") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestRunETLFetchTest_UnknownSource(t *testing.T) {
	err := runETLFetchTest([]string{"nonexistent"})
	if err == nil || !contains(err.Error(), "unknown source") {
		t.Fatalf("expected unknown source error, got %v", err)
	}
}

func TestRunETLStatus_MissingCredentials(t *testing.T) {
	// Clear env to force early error from createMinIOSink.
	oldAccess := os.Getenv("MINIO_ACCESS_KEY")
	oldSecret := os.Getenv("MINIO_SECRET_KEY")
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	defer func() {
		os.Setenv("MINIO_ACCESS_KEY", oldAccess)
		os.Setenv("MINIO_SECRET_KEY", oldSecret)
	}()

	err := runETLStatus(nil)
	if err == nil || !contains(err.Error(), "MINIO_ACCESS_KEY") {
		t.Fatalf("expected credential error, got %v", err)
	}
}

func TestValidateMessage_HL7(t *testing.T) {
	hl7Path := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(hl7Path); err != nil {
		t.Skip("HL7 sample missing")
	}

	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	profileYAML := `id: test_profile
name: Test Profile
hl7v2:
  default_version: "2.5"`
	profileYAML = "source_profile:\n  " + strings.ReplaceAll(profileYAML, "\n", "\n  ")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	errs := validateMessage(profilePath, hl7Path, "hl7v2", false)
	if len(errs) > 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

func TestValidateMessage_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	_ = os.WriteFile(profilePath, []byte("id: test\nname: Test"), 0o644)
	msgPath := filepath.Join(tmpDir, "msg.txt")
	_ = os.WriteFile(msgPath, []byte("dummy"), 0o644)

	errs := validateMessage(profilePath, msgPath, "unsupported", false)
	if len(errs) == 0 {
		t.Fatal("expected unsupported format error")
	}
}

func TestRunTerminologyCrosswalk_RequiresVocab(t *testing.T) {
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	defer os.Unsetenv("FI_FHIR_DATABASE_URL")

	err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM"})
	if err == nil || !contains(err.Error(), "--from and --to") {
		t.Fatalf("expected missing vocab error, got %v", err)
	}
}
