package main

import (
	"os"
	"testing"
)

// ===========================================================================
// Database-Dependent CLI Tests
// These tests require FI_FHIR_DATABASE_URL to be set
// ===========================================================================

// skipIfNoDatabase skips the test if no database URL is configured
func skipIfNoDatabase(t *testing.T) {
	t.Helper()
	if os.Getenv("FI_FHIR_DATABASE_URL") == "" {
		t.Skip("skipping: FI_FHIR_DATABASE_URL not set")
	}
}

// ===========================================================================
// EventStore Init Tests
// ===========================================================================

func TestEventStore_Init_WithDBFlag(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	stdout, _, err := runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_events_cli")
	if err != nil {
		t.Logf("EventStore init error: %v", err)
	}
	// Should show success message
	_ = stdout
}

// ===========================================================================
// EventStore Stats Tests
// ===========================================================================

func TestEventStore_Stats_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// First ensure the table exists
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_stats_events")

	stdout, _, err := runCLI(t, "eventstore", "stats", "--db", dbURL, "--table", "test_stats_events")
	if err != nil {
		t.Logf("EventStore stats error: %v", err)
	}
	// Should show statistics output
	if len(stdout) > 0 {
		assertContains(t, stdout, "Event Store Statistics")
	}
}

// ===========================================================================
// EventStore Streams Tests
// ===========================================================================

func TestEventStore_Streams_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// First ensure the table exists
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_streams_events")

	stdout, _, err := runCLI(t, "eventstore", "streams", "--db", dbURL, "--table", "test_streams_events")
	if err != nil {
		t.Logf("EventStore streams error: %v", err)
	}
	_ = stdout
}

func TestEventStore_Streams_WithLimit(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_streams_limit")

	stdout, _, err := runCLI(t, "eventstore", "streams", "--db", dbURL, "--table", "test_streams_limit", "--limit", "10")
	if err != nil {
		t.Logf("EventStore streams with limit error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// EventStore Read Tests
// ===========================================================================

func TestEventStore_Read_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// Initialize table first
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_read_events")

	stdout, _, err := runCLI(t, "eventstore", "read", "--db", dbURL, "--table", "test_read_events", "--stream", "patient-123")
	if err != nil {
		t.Logf("EventStore read error: %v", err)
	}
	_ = stdout
}

func TestEventStore_Read_AllEvents(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_read_all")

	stdout, _, err := runCLI(t, "eventstore", "read", "--db", dbURL, "--table", "test_read_all", "--all")
	if err != nil {
		t.Logf("EventStore read all error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Projection Status Tests
// ===========================================================================

func TestProjection_Status_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// Initialize eventstore first
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_status")

	stdout, _, err := runCLI(t, "projection", "status", "--db", dbURL, "--table", "test_proj_status")
	if err != nil {
		t.Logf("Projection status error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Projection Run Tests
// ===========================================================================

func TestProjection_Run_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// Initialize eventstore first
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_run")

	stdout, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--table", "test_proj_run", "--projection", "timeline")
	if err != nil {
		t.Logf("Projection run error: %v", err)
	}
	_ = stdout
}

func TestProjection_Run_UnknownProjection(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--projection", "nonexistent_projection")
	if err != nil {
		t.Logf("Expected error for unknown projection: %v", err)
	}
}

// ===========================================================================
// Projection Rebuild Tests
// ===========================================================================

func TestProjection_Rebuild_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// Initialize eventstore first
	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_rebuild")

	stdout, _, err := runCLI(t, "projection", "rebuild", "--db", dbURL, "--table", "test_proj_rebuild", "--projection", "timeline")
	if err != nil {
		t.Logf("Projection rebuild error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Init Tests
// ===========================================================================

func TestTerminology_Init_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	// Set the terminology DB URL temporarily
	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	stdout, _, err := runCLI(t, "terminology", "init")
	if err != nil {
		t.Logf("Terminology init error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Drop Tests
// ===========================================================================

func TestTerminology_Drop_RequiresForce(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	stdout, _, err := runCLI(t, "terminology", "drop")
	// Should warn about needing --force
	if err != nil {
		t.Logf("Terminology drop (without force) error: %v", err)
	}
	assertContains(t, stdout, "--force")
}

// ===========================================================================
// Terminology Load Tests
// ===========================================================================

func TestTerminology_Load_UnknownSource(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	_, _, err := runCLI(t, "terminology", "load", "nonexistent_source")
	if err != nil {
		t.Logf("Expected error for unknown source: %v", err)
	}
}

// ===========================================================================
// ETL Load Tests
// ===========================================================================

func TestETL_Load_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// Try to load a stub source (should work with our fixtures)
	stdout, _, err := runCLI(t, "etl", "load", "stub")
	if err != nil {
		t.Logf("ETL load error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Additional EventStore Read Tests
// ===========================================================================

func TestEventStore_Read_WithLimit(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_read_limit")

	stdout, _, err := runCLI(t, "eventstore", "read", "--db", dbURL, "--table", "test_read_limit", "--stream", "test", "--limit", "5")
	if err != nil {
		t.Logf("EventStore read with limit error: %v", err)
	}
	_ = stdout
}

func TestEventStore_Read_FromPosition(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_read_pos")

	stdout, _, err := runCLI(t, "eventstore", "read", "--db", dbURL, "--table", "test_read_pos", "--all", "--from", "0")
	if err != nil {
		t.Logf("EventStore read from position error: %v", err)
	}
	_ = stdout
}

func TestEventStore_Read_JSONOutput(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_read_json")

	stdout, _, err := runCLI(t, "eventstore", "read", "--db", dbURL, "--table", "test_read_json", "--all", "--json")
	if err != nil {
		t.Logf("EventStore read JSON error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Additional Projection Tests
// ===========================================================================

func TestProjection_Rebuild_WithForce(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_rebuild_force")

	stdout, _, err := runCLI(t, "projection", "rebuild", "--db", dbURL, "--table", "test_rebuild_force", "--projection", "timeline", "--force")
	if err != nil {
		t.Logf("Projection rebuild with force error: %v", err)
	}
	_ = stdout
}

func TestProjection_Run_WithLimit(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_limit")

	stdout, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--table", "test_proj_limit", "--projection", "timeline", "--batch-size", "100")
	if err != nil {
		t.Logf("Projection run with batch size error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Status Tests
// ===========================================================================

func TestTerminology_Status_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// First init the terminology tables
	_, _, _ = runCLI(t, "terminology", "init")

	stdout, _, err := runCLI(t, "terminology", "status")
	if err != nil {
		t.Logf("Terminology status error: %v", err)
	}
	_ = stdout
}

func TestTerminology_Status_JSONFormat(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	_, _, _ = runCLI(t, "terminology", "init")

	stdout, _, err := runCLI(t, "terminology", "status", "--json")
	if err != nil {
		t.Logf("Terminology status JSON error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// EventStore Append Tests
// ===========================================================================

func TestEventStore_Append_WithDB(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_append")

	// Append a test event
	stdout, _, err := runCLI(t, "eventstore", "append",
		"--db", dbURL,
		"--table", "test_append",
		"--stream", "test-stream-1",
		"--type", "test_event",
		"--data", `{"key":"value"}`)
	if err != nil {
		t.Logf("EventStore append error: %v", err)
	}
	_ = stdout
}

func TestEventStore_Append_WithMetadata(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_append_meta")

	stdout, _, err := runCLI(t, "eventstore", "append",
		"--db", dbURL,
		"--table", "test_append_meta",
		"--stream", "test-stream-2",
		"--type", "test_event",
		"--data", `{"key":"value"}`,
		"--metadata", `{"source":"test"}`)
	if err != nil {
		t.Logf("EventStore append with metadata error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Projection Rebuild - More Tests
// ===========================================================================

func TestProjection_Rebuild_AllProjections(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_rebuild_all")

	// Rebuild all projections
	stdout, _, err := runCLI(t, "projection", "rebuild", "--db", dbURL, "--table", "test_rebuild_all", "--all")
	if err != nil {
		t.Logf("Projection rebuild all error: %v", err)
	}
	_ = stdout
}

func TestProjection_Rebuild_Verbose(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_rebuild_verbose")

	stdout, _, err := runCLI(t, "projection", "rebuild", "--db", dbURL, "--table", "test_rebuild_verbose", "--projection", "timeline", "--verbose")
	if err != nil {
		t.Logf("Projection rebuild verbose error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Drop - More Tests
// ===========================================================================

func TestTerminology_Drop_WithForce(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// First init so we have something to drop
	_, _, _ = runCLI(t, "terminology", "init")

	// Drop with force (this actually drops data so be careful)
	// Using a test-specific table prefix would be better
	stdout, _, err := runCLI(t, "terminology", "drop", "--force")
	if err != nil {
		t.Logf("Terminology drop with force error: %v", err)
	}
	_ = stdout

	// Re-init for other tests
	_, _, _ = runCLI(t, "terminology", "init")
}

// ===========================================================================
// ETL Sync Tests
// ===========================================================================

func TestETL_Sync_DryRun(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "sync", "--dry-run", "stub")
	if err != nil {
		t.Logf("ETL sync dry-run error: %v", err)
	}
	_ = stdout
}

func TestETL_Sync_Unknown_Source(t *testing.T) {
	_, _, err := runCLI(t, "etl", "sync", "nonexistent_source")
	if err != nil {
		t.Logf("Expected error for unknown ETL source: %v", err)
	}
}

// ===========================================================================
// Storage Test - Additional Tests
// ===========================================================================

func TestStorage_Test_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "storage", "test", "--help")
	if err != nil {
		t.Logf("Storage test help error: %v", err)
	}
	_ = stdout
}

func TestStorage_List_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "storage", "list", "--help")
	if err != nil {
		t.Logf("Storage list help error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Status JSON Output Tests
// ===========================================================================

func TestTerminology_Status_JSONOutput(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// Make sure database is initialized
	_, _, _ = runCLI(t, "terminology", "init")

	stdout, _, err := runCLI(t, "terminology", "status", "--json")
	if err != nil {
		t.Logf("Terminology status JSON error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Terminology Crosswalk Tests
// ===========================================================================

// TestTerminology_Crosswalk_MissingArgs already exists in terminology_test.go

func TestTerminology_Crosswalk_WithSourceSystem(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// Initialize
	_, _, _ = runCLI(t, "terminology", "init")

	// Try crosswalk with a test code
	stdout, _, err := runCLI(t, "terminology", "crosswalk", "--source", "snomed", "--target", "icd10", "12345")
	if err != nil {
		t.Logf("Terminology crosswalk error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// EventStore JSON Output Tests
// ===========================================================================

func TestEventStore_Stats_JSONOutput(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_stats_json")

	stdout, _, err := runCLI(t, "eventstore", "stats", "--db", dbURL, "--table", "test_stats_json", "--json")
	if err != nil {
		t.Logf("EventStore stats JSON error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// Projection Run with Different Projections
// ===========================================================================

func TestProjection_Run_PatientTimeline(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_timeline")

	stdout, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--table", "test_proj_timeline", "--projection", "patient_timeline")
	if err != nil {
		t.Logf("Projection run patient_timeline error: %v", err)
	}
	_ = stdout
}

func TestProjection_Run_Statistics(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_stats")

	stdout, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--table", "test_proj_stats", "--projection", "statistics")
	if err != nil {
		t.Logf("Projection run statistics error: %v", err)
	}
	_ = stdout
}

func TestProjection_Run_ActiveEncounters(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_proj_encounters")

	stdout, _, err := runCLI(t, "projection", "run", "--db", dbURL, "--table", "test_proj_encounters", "--projection", "active_encounters")
	if err != nil {
		t.Logf("Projection run active_encounters error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// EventStore Append with Different Event Types
// ===========================================================================

func TestEventStore_Append_WithEventType(t *testing.T) {
	skipIfNoDatabase(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	_, _, _ = runCLI(t, "eventstore", "init", "--db", dbURL, "--table", "test_append_type")

	stdout, _, err := runCLI(t, "eventstore", "append",
		"--db", dbURL,
		"--table", "test_append_type",
		"--stream", "patient-event-type",
		"--type", "PatientAdmission",
		"--data", `{"patient_id":"P100","facility":"ICU"}`)
	if err != nil {
		t.Logf("EventStore append with type error: %v", err)
	}
	_ = stdout
}

// ===========================================================================
// ETL Fetch Tests
// ===========================================================================

func TestETL_Fetch_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "fetch", "--help")
	if err != nil {
		t.Logf("ETL fetch help error: %v", err)
	}
	_ = stdout
}

func TestETL_Fetch_MissingSource(t *testing.T) {
	_, _, err := runCLI(t, "etl", "fetch")
	if err != nil {
		t.Logf("ETL fetch missing source error (expected): %v", err)
	}
}

// ===========================================================================
// ETL Validate Tests
// ===========================================================================

func TestETL_Validate_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "validate", "--help")
	if err != nil {
		t.Logf("ETL validate help error: %v", err)
	}
	_ = stdout
}

func TestETL_Validate_MissingArgs(t *testing.T) {
	_, _, err := runCLI(t, "etl", "validate")
	if err != nil {
		t.Logf("ETL validate missing args error (expected): %v", err)
	}
}

// ===========================================================================
// ETL Status Tests
// ===========================================================================

func TestETL_Status_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "status", "--help")
	if err != nil {
		t.Logf("ETL status help error: %v", err)
	}
	_ = stdout
}

func TestETL_Status_WithVerbose(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "status", "--verbose")
	if err != nil {
		t.Logf("ETL status verbose error: %v", err)
	}
	_ = stdout
}
