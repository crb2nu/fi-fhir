package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminologyMapping_Upload_NoArgs(t *testing.T) {
	_, _, err := runCLI(t, "terminology", "mapping", "upload")
	assertErrorContains(t, err, "CSV file path required")
}

func TestTerminologyMapping_Upload_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	tmpfile := filepath.Join(t.TempDir(), "dummy.csv")
	_ = os.WriteFile(tmpfile, []byte(""), 0600)

	_, _, err := runCLI(t, "terminology", "mapping", "upload", tmpfile)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_List_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "list")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Delete_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "delete", "uuid1")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Get_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	// Provide a valid int ID so it skips the 'expected integer' parse error
	_, _, err := runCLI(t, "terminology", "mapping", "get", "123")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Resolve_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "resolve", "uuid")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Pending_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "pending")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Approve_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "approve", "123")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyMapping_Reject_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "mapping", "reject", "123")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Status_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "status")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Drop_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	_, _, err := runCLI(t, "terminology", "drop")
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Autoroute_MissingSourceSystem(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	_, _, err := runCLI(t, "terminology", "autoroute", "CODE", "--target-system", "tgt")
	assertErrorContains(t, err, "--source-system is required")
}

func TestTerminology_Autoroute_MissingTargetSystem(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	_, _, err := runCLI(t, "terminology", "autoroute", "CODE", "--source-system", "src")
	assertErrorContains(t, err, "--target-system is required")
}

func TestTerminology_Autoroute_FailSemantic(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("QDRANT_URL", "")
	t.Setenv("LLM_EMBEDDING_BASE_URL", "http://invalid-url:9999")

	_, _, err := runCLI(t, "terminology", "autoroute", "CODE", "--source-system", "src", "--target-system", "tgt")
	if err == nil {
		t.Fatalf("expected error from semantic searcher")
	}
}

func TestTerminology_AutorouteViaWorkflow_FailTemporal(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("QDRANT_URL", "")
	t.Setenv("LLM_EMBEDDING_BASE_URL", "http://invalid-url:9999")

	_, _, err := runCLI(t, "terminology", "autoroute", "CODE", "--source-system", "src", "--target-system", "tgt", "--temporal", "localhost:99999")
	if err == nil {
		t.Fatalf("expected error from workflow init")
	}
}

func TestTerminology_Crosswalk_MissingFromTo(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	_, _, err := runCLI(t, "terminology", "crosswalk", "E11.9")
	assertErrorContains(t, err, "--from and --to vocabularies are required")
}

func TestTerminology_MappingResolve_MissingSource(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	_, _, err := runCLI(t, "terminology", "mapping", "resolve", "CODE", "--target-system", "tgt")
	assertErrorContains(t, err, "--source-system is required")
}

func TestTerminology_MappingResolve_MissingTarget(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/db")

	_, _, err := runCLI(t, "terminology", "mapping", "resolve", "CODE", "--source-system", "src")
	assertErrorContains(t, err, "--target-system is required")
}

func TestTerminology_MappingResolve_NoAutoroute(t *testing.T) {
	// Clear the priority env var so the fallback is actually used.
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@invalid-host.local:5432/invalid_db?connect_timeout=1")

	// Even with no-autoroute it hits DB first. Should fail on dial.
	_, _, err := runCLI(t, "terminology", "mapping", "resolve", "CODE", "--source-system", "src", "--target-system", "tgt", "--no-autoroute")
	assertError(t, err)
}

// Added to test initialization fail paths when valid DB syntax but no real DB connection or dialect.
func TestTerminology_Status_InvalidDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://invalid-host:5432/test?sslmode=disable")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("DATABASE_URL", "")

	// Expect it to dial and then fail ping or similar
	_, _, err := runCLI(t, "terminology", "status")
	assertErrorContains(t, err, "dial tcp")
}
