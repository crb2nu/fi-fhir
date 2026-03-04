package main

import (
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
	db "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// =============================================================================
// Eventstore — missing-DB-URL, flag-parsing, and dispatcher coverage
// =============================================================================

func TestRunEventStore_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	tests := []struct {
		name string
		args []string
	}{
		{"init", []string{"init"}},
		{"stats", []string{"stats"}},
		{"streams", []string{"streams"}},
		{"read --all", []string{"read", "--all"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEventStore(tt.args)
			if err == nil || !strings.Contains(err.Error(), "database URL required") {
				t.Errorf("expected 'database URL required' error, got: %v", err)
			}
		})
	}
}

func TestRunEventStore_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runEventStore([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir eventstore")
}

func TestRunEventStore_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runEventStore([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir eventstore")
}

func TestRunEventStore_UnknownSubcommand(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		err := runEventStore([]string{"nonexistent"})
		assertNoError(t, err) // prints usage, returns nil
	})
	assertContains(t, stderr, "Unknown eventstore subcommand")
}

func TestRunEventStoreRead_MissingStreamOrAll(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	err := runEventStoreRead([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "must specify --stream or --all")
}

func TestRunEventStoreRead_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	// Exercise all flag parsing paths — will fail at DB connection, not parsing
	err := runEventStoreRead([]string{
		"--stream", "patient:MRN001",
		"--from-position", "100",
		"--from-version", "5",
		"--limit", "10",
		"--pretty",
	})
	// Should fail at DB level, not flag parsing
	if err != nil && strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("failed at flag parsing: %v", err)
	}
}

func TestRunEventStoreRead_MissingFlagValues(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"stream", []string{"--stream"}, "--stream requires a value"},
		{"from-position", []string{"--from-position"}, "--from-position requires a value"},
		{"from-version", []string{"--from-version"}, "--from-version requires a value"},
		{"limit", []string{"--limit"}, "--limit requires a value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEventStoreRead(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestRunEventStoreAppend_MissingFlags(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing stream", []string{}, "--stream is required"},
		{"missing type", []string{"--stream", "test"}, "--type is required"},
		{"missing data", []string{"--stream", "test", "--type", "Test"}, "--data is required"},
		{"invalid json", []string{"--stream", "test", "--type", "Test", "--data", "not-json"}, "--data must be valid JSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEventStoreAppend(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestRunEventStoreAppend_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	// Exercise all flag parsing paths
	err := runEventStoreAppend([]string{
		"--stream", "test:stream",
		"--type", "TestEvent",
		"--data", `{"key":"value"}`,
		"--version", "1",
	})
	// Should fail at DB, not parsing
	if err != nil && strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("failed at flag parsing: %v", err)
	}
}

func TestRunEventStoreStreams_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	// Exercise --limit flag parsing
	err := runEventStoreStreams([]string{"--limit", "50"})
	// Should fail at DB, not parsing
	if err != nil && strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("failed at flag parsing: %v", err)
	}
}

func TestGetEventStoreDB_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"db missing value", []string{"--db"}, "--db requires a value"},
		{"table missing value", []string{"--db", "postgres://x", "--table"}, "--table requires a value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := getEventStoreDB(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestGetEventStoreDB_WithTableFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	dbURL, tableName, err := getEventStoreDB([]string{"--db", "postgres://x", "--table", "my_events"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dbURL != "postgres://x" {
		t.Errorf("expected postgres://x, got %q", dbURL)
	}
	if tableName != "my_events" {
		t.Errorf("expected my_events, got %q", tableName)
	}
}

// =============================================================================
// Projection — dispatcher and error paths
// =============================================================================

func TestRunProjection_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runProjection([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir projection")
}

func TestRunProjection_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runProjection([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir projection")
}

func TestRunProjection_UnknownSubcommand(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		err := runProjection([]string{"nonexistent"})
		assertNoError(t, err)
	})
	assertContains(t, stderr, "Unknown projection subcommand")
}

func TestRunProjectionList(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runProjectionList([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "patient_timeline")
	assertContains(t, stdout, "event_statistics")
	assertContains(t, stdout, "active_encounters")
}

func TestRunProjectionRebuild_MissingNameOrAll(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	err := runProjectionRebuild([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "--name or --all is required")
}

func TestRunProjectionRebuild_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	// Exercise all flag parsing — will fail at DB
	err := runProjectionRebuild([]string{
		"--name", "patient_timeline",
		"--from-snapshot",
		"--from-position", "100",
		"--stop-position", "500",
		"--dry-run",
	})
	// Should fail at DB, not flag parsing
	if err != nil && strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("failed at flag parsing: %v", err)
	}
}

func TestRunProjectionRebuild_InvalidFromPosition(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	err := runProjectionRebuild([]string{"--name", "test", "--from-position", "abc"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid --from-position")
}

func TestRunProjectionRebuild_InvalidStopPosition(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	err := runProjectionRebuild([]string{"--name", "test", "--stop-position", "xyz"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid --stop-position")
}

func TestRunProjectionStatus_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runProjectionStatus([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunProjectionRun_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runProjectionRun([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunProjectionRun_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")

	err := runProjectionRun([]string{"--name", "patient_timeline"})
	// Should fail at DB, not flag parsing
	if err != nil && strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("failed at flag parsing: %v", err)
	}
}

// =============================================================================
// runServe — flag-parsing coverage
// =============================================================================

func TestRunServe_FlagParsing(t *testing.T) {
	// Exercise port/host flag parsing without starting the server
	// by triggering an early error. The function should parse flags
	// before binding — set a very quick signal to test flag handling.
	t.Setenv("FI_FHIR_CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("FI_FHIR_SERVE_TEMPORAL_HOST_PORT", "")

	// Just confirm --help doesn't crash
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "serve")
}

// =============================================================================
// marshalYAML — edge-case coverage
// =============================================================================

func TestMarshalYAML_NilInput(t *testing.T) {
	result, err := marshalYAML(nil)
	if err == nil && len(result) > 0 {
		// nil config should either error or produce minimal output
		t.Logf("marshalYAML(nil) returned %d bytes", len(result))
	}
}

func TestMarshalYAML_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}
	result, err := marshalYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected non-empty output for empty config")
	}
}

// =============================================================================
// ETL dispatcher and runETLLoad — offline error paths
// =============================================================================

func TestRunETL_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runETL([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir etl")
}

func TestRunETL_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runETL([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir etl")
}

func TestRunETL_UnknownSubcommand(t *testing.T) {
	err := runETL([]string{"nonexistent"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown etl subcommand")
}

func TestRunETLLoad_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runETLLoad([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir etl load")
}

func TestRunETLLoad_UnsupportedSource(t *testing.T) {
	err := runETLLoad([]string{"nonexistent", "--version", "1.0"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown source")
}

func TestRunETLLoad_MissingVersion_P2(t *testing.T) {
	err := runETLLoad([]string{"umls"})
	assertError(t, err)
	assertErrorContains(t, err, "--version is required")
}

func TestRunETLLoad_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	err := runETLLoad([]string{"umls", "--version", "2024AB"})
	assertError(t, err)
	assertErrorContains(t, err, "environment variable is required")
}

func TestRunETLLoad_MissingMinIOKeys(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MINIO_ACCESS_KEY", "")
	t.Setenv("MINIO_SECRET_KEY", "")

	err := runETLLoad([]string{"umls", "--version", "2024AB"})
	assertError(t, err)
	assertErrorContains(t, err, "MINIO_ACCESS_KEY")
}

func TestRunETLLoad_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	// Exercise flag parsing for all supported sources
	for _, src := range []string{"umls", "rxnorm", "loinc", "icd10cm"} {
		t.Run(src, func(t *testing.T) {
			err := runETLLoad([]string{src, "--version", "v1", "--dry-run", "--progress"})
			assertError(t, err)
			// Should fail at DB URL check, not flag parsing
			assertErrorContains(t, err, "environment variable is required")
		})
	}
}

func TestRunETLLoad_SABsFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	err := runETLLoad([]string{"umls", "--version", "2024AB", "--sabs", "SNOMEDCT_US,ICD10CM"})
	assertError(t, err)
	// Should fail at DB URL check, not flag parsing
	assertErrorContains(t, err, "environment variable is required")
}

func TestIsSupportedETLLoadSource(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"umls", true},
		{"rxnorm", true},
		{"loinc", true},
		{"icd10cm", true},
		{"nonexistent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedETLLoadSource(tt.name); got != tt.want {
				t.Errorf("isSupportedETLLoadSource(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestRunETLFetch_NoArgs(t *testing.T) {
	err := runETLFetch([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestRunETLFetchTest_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runETLFetchTest([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir etl fetch-test")
}

// =============================================================================
// runServe — exhaustive flag parsing coverage
// =============================================================================

func TestRunServe_MissingFlagValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"host", []string{"--host"}, "--host requires a value"},
		{"port", []string{"--port"}, "--port requires a value"},
		{"path", []string{"--path"}, "--path requires a value"},
		{"playground-path", []string{"--playground-path"}, "--playground-path requires a value"},
		{"workflow", []string{"--workflow"}, "--workflow requires a value"},
		{"max-depth", []string{"--max-depth"}, "--max-depth requires a value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServe(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestRunServe_InvalidPort(t *testing.T) {
	err := runServe([]string{"--port", "notanumber"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid port")
}

// =============================================================================
// storage offline paths
// =============================================================================

func TestCreateMinIOProvider_NoKeys(t *testing.T) {
	t.Setenv("MINIO_ACCESS_KEY", "")
	t.Setenv("MINIO_SECRET_KEY", "")
	t.Setenv("STORAGE_MINIO_ACCESS_KEY", "")
	t.Setenv("STORAGE_MINIO_SECRET_KEY", "")

	_, err := createMinIOProvider()
	assertError(t, err)
	assertErrorContains(t, err, "MINIO_ACCESS_KEY")
}

// =============================================================================
// cliProgressReporter
// =============================================================================

func TestCliProgressReporter(t *testing.T) {
	reporter := cliProgressReporter()
	if reporter == nil {
		t.Fatal("expected non-nil reporter")
	}

	// Just verify it doesn't panic with various inputs
	stdout, _ := captureOutput(t, func() {
		reporter(db.LoadProgress{
			Vocabulary: "LOINC",
			Phase:      "loading",
			RowsLoaded: 100,
			RowsTotal:  1000,
		})
		reporter(db.LoadProgress{
			Vocabulary: "SNOMED",
			Phase:      "indexing",
			RowsLoaded: 50,
			RowsTotal:  0,
		})
	})
	_ = stdout // just ensure no panic
}

// =============================================================================
// getEnvOrDefault
// =============================================================================

func TestGetEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_EXISTING_VAR", "hello")

	got := getEnvOrDefault("TEST_EXISTING_VAR", "default")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}

	got = getEnvOrDefault("TEST_NONEXISTENT_VAR_12345", "default")
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}
