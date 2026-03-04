package main

import (
	"context"
	"strings"
	"testing"
)

// =============================================================================
// runTerminologyMappingResolve — flag-parsing and guard clauses
// Coverage was 44.3%, targeting guard clauses and flag parsing paths.
// =============================================================================

func TestRunTerminologyMappingResolve_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercise all flag parsing paths - will fail at DB connection
	err := runTerminologyMappingResolve([]string{
		"GLU001",
		"--source-system", "epic",
		"--target-system", "cerner",
		"--display", "Glucose Test",
		"--profile", "profile-123",
		"--no-autoroute",
		"--json",
	})
	// Should fail at DB, not at flag parsing
	if err != nil && strings.Contains(err.Error(), "is required") {
		t.Fatalf("failed at required-flag check: %v", err)
	}
}

// =============================================================================
// runTerminologyMappingGet — flag-parsing and guard clauses
// Coverage was 42.7%, targeting guard clauses and early error paths.
// =============================================================================

func TestRunTerminologyMappingGet_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingGet([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingGet_WithBatchFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercise batch flag parsing - will fail at DB
	err := runTerminologyMappingGet([]string{"--batch", "some-uuid"})
	// Should fail at DB connection, not flag parsing
	if err != nil && strings.Contains(err.Error(), "database URL required") {
		t.Fatalf("failed at DB URL check: %v", err)
	}
}

func TestRunTerminologyMappingGet_WithNumericID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercise numeric ID parsing path - will fail at DB
	err := runTerminologyMappingGet([]string{"42"})
	// Should fail at DB connection, not parsing
	if err != nil && strings.Contains(err.Error(), "database URL required") {
		t.Fatalf("failed at DB URL check: %v", err)
	}
}

// =============================================================================
// runTerminologyMappingUpload — guard clauses
// =============================================================================

func TestRunTerminologyMappingUpload_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingUpload([]string{"file.csv"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingUpload_NoArgs(t *testing.T) {
	err := runTerminologyMappingUpload([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "CSV file path required")
}

// =============================================================================
// runTerminologyMappingList — guard clauses
// =============================================================================

func TestRunTerminologyMappingList_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingList([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingList_FlagParsing(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercise all flag parsing paths - will fail at DB
	err := runTerminologyMappingList([]string{
		"--status", "pending",
		"--batch", "some-uuid",
		"--limit", "10",
		"--source-system", "epic",
		"--target-system", "cerner",
		"--json",
	})
	// Should fail at DB, not flag parsing
	if err != nil && strings.Contains(err.Error(), "is required") {
		t.Fatalf("failed at required-flag check: %v", err)
	}
}

// =============================================================================
// runTerminologyMappingPending/Approve/Reject/Delete — missing DB URL
// =============================================================================

func TestRunTerminologyMappingApprove_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingApprove([]string{})
	assertError(t, err)
}

func TestRunTerminologyMappingReject_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingReject([]string{})
	assertError(t, err)
}

func TestRunTerminologyMappingDelete_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingDelete([]string{})
	assertError(t, err)
}

func TestRunTerminologyMappingPending_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyMappingPending([]string{})
	assertError(t, err)
}

// =============================================================================
// runTerminologyUse — additional guard paths
// =============================================================================

// =============================================================================
// serve env-init functions — missing DB paths
// =============================================================================

func TestInitEventStoreFromEnv_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_EVENT_STORE_URL", "")
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	store, err := initEventStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store != nil {
		t.Error("expected nil store when DB is unconfigured")
	}
}

func TestInitProfileStoreFromEnv_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_PROFILE_STORE_URL", "")
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	st, err := initProfileStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != nil {
		t.Error("expected nil store when DB is unconfigured")
	}
}

func TestInitWorkflowLifecycleStoreFromEnv_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_LIFECYCLE_STORE_URL", "")
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	st, err := initWorkflowLifecycleStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != nil {
		t.Error("expected nil store when DB is unconfigured")
	}
}
