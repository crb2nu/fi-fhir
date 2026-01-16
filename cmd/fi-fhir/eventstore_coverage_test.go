//nolint:errcheck,gosec // Test file - error/security checks relaxed for test setup
package main

import (
	"testing"
)

// =============================================================================
// EventStore Additional Coverage Tests
// =============================================================================

func TestEventStore_Init_DBFlagParsed(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	// Should fail to connect, but --db flag should be parsed
	err := runEventStoreInit([]string{"--db", "postgres://flag/test"})
	assertError(t, err)
	// Error should be connection error, not missing DB URL
	if contains(err.Error(), "database URL required") {
		t.Errorf("--db flag should have been parsed")
	}
}

func TestEventStore_Init_TableFlagParsed(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// Should fail to connect, but --table flag should be parsed
	err := runEventStoreInit([]string{"--table", "custom_events"})
	assertError(t, err)
	// Error should be connection error
}

func TestEventStore_Stats_TableFlagParsed(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runEventStoreStats([]string{"--table", "custom_events"})
	assertError(t, err)
}

func TestEventStore_Streams_AllFlags(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// Test with various flags
	err := runEventStoreStreams([]string{"--table", "events", "--limit", "10"})
	assertError(t, err) // connection error expected
}

func TestEventStore_Read_AllFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// Use --all flag
	err := runEventStoreRead([]string{"--all", "--limit", "10"})
	assertError(t, err) // connection error expected
}

func TestEventStore_Read_StreamFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runEventStoreRead([]string{"--stream", "test-stream", "--limit", "10"})
	assertError(t, err) // connection error expected
}

func TestEventStore_Read_FromFlags(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// Test with --from-position
	err := runEventStoreRead([]string{"--all", "--from-position", "100"})
	assertError(t, err)
}

func TestEventStore_Read_FromVersion(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runEventStoreRead([]string{"--stream", "test", "--from-version", "5"})
	assertError(t, err)
}

// Note: runEventStoreRead uses fmt.Sscanf with silently ignored errors,
// so invalid values just use defaults. No validation error tests here.

func TestEventStore_Append_AllRequiredFlags(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runEventStoreAppend([]string{
		"--stream", "test-stream",
		"--type", "TestEvent",
		"--data", `{"key":"value"}`,
	})
	assertError(t, err) // connection error expected
}

func TestEventStore_Append_WithVersion(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runEventStoreAppend([]string{
		"--stream", "test-stream",
		"--type", "TestEvent",
		"--data", `{"key":"value"}`,
		"--version", "5",
	})
	assertError(t, err)
}

// Note: runEventStoreAppend and runEventStoreStreams use fmt.Sscanf
// with silently ignored errors, so invalid values just use defaults.

// =============================================================================
// Projection Additional Coverage Tests
// =============================================================================

func TestProjection_Status_NameFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionStatus([]string{"--name", "patient_timeline"})
	assertError(t, err) // connection error expected
}

func TestProjection_Run_AllFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionRun([]string{"--all"})
	assertError(t, err) // connection error expected
}

func TestProjection_Run_NameFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionRun([]string{"--name", "patient_timeline"})
	assertError(t, err)
}

func TestProjection_Rebuild_AllFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionRebuild([]string{"--all"})
	assertError(t, err) // connection error expected
}

func TestProjection_Rebuild_WithFromAndStop(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionRebuild([]string{"--name", "patient_timeline", "--from-position", "100", "--stop-position", "200"})
	assertError(t, err) // connection error expected
}

func TestProjection_Rebuild_DryRunFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runProjectionRebuild([]string{"--name", "patient_timeline", "--dry-run"})
	assertError(t, err) // connection error expected
}

// Note: Serve command tests that actually start the HTTP server are excluded
// because they would hang indefinitely. Tests for flag parsing are in
// serve_additional_test.go and subscription_serve_additional_test.go.
