package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

var postgresDBNameSafe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func createEphemeralPostgresDB(t *testing.T, adminURL string) string {
	t.Helper()

	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("invalid FI_FHIR_POSTGRES_ADMIN_URL: %v", err)
	}

	dbName := fmt.Sprintf("fi_fhir_cli_%d", time.Now().UnixNano())
	if !postgresDBNameSafe.MatchString(dbName) {
		t.Fatalf("generated unsafe db name: %q", dbName)
	}

	maintenance := *parsed
	maintenance.Path = "/postgres"

	maintDB, err := sql.Open("postgres", maintenance.String())
	if err != nil {
		t.Fatalf("open maintenance DB: %v", err)
	}
	t.Cleanup(func() { _ = maintDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := maintDB.PingContext(ctx); err != nil {
		t.Fatalf("ping maintenance DB: %v", err)
	}

	// CREATE/DROP DATABASE cannot be parameterized; dbName is generated and validated.
	_, err = maintDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, dbName))
	if err != nil {
		// In some setups the CI DB user may not have CREATEDB; skip instead of failing.
		t.Skipf("skipping: unable to create test database (need CREATEDB): %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Postgres 13+ supports FORCE; fall back if not supported/allowed.
		if _, err := maintDB.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)); err != nil {
			_, _ = maintDB.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, dbName))
		}
	})

	testURL := *parsed
	testURL.Path = "/" + dbName
	return testURL.String()
}

func TestTerminology_Live_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	requireEnv(t, "FI_FHIR_POSTGRES_ADMIN_URL")
	dsn := createEphemeralPostgresDB(t, os.Getenv("FI_FHIR_POSTGRES_ADMIN_URL"))

	// init
	stdout, _, err := runCLI(t, "terminology", "init", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "Terminology schema")
	assertContains(t, stdout, "Schema version:")

	// status (empty)
	stdout, _, err = runCLI(t, "terminology", "status", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "Terminology Database Status")
	assertContains(t, stdout, "No terminology data loaded yet.")

	// drop (force)
	stdout, _, err = runCLI(t, "terminology", "drop", "--db", dsn, "--force")
	assertNoError(t, err)
	assertContains(t, stdout, "Terminology schema dropped successfully")

	// status after drop
	stdout, _, err = runCLI(t, "terminology", "status", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "Schema not initialized")
}

func TestTerminology_Live_Load_LOINC_And_ICD10CM(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	requireEnv(t, "FI_FHIR_POSTGRES_ADMIN_URL")
	dsn := createEphemeralPostgresDB(t, os.Getenv("FI_FHIR_POSTGRES_ADMIN_URL"))

	// Minimal LOINC table + panel hierarchy.
	loincDir := t.TempDir()
	loincTable := createTempFile(t, loincDir, "LoincTable.csv", strings.Join([]string{
		"LOINC_NUM,LONG_COMMON_NAME,STATUS",
		"1234-5,Example test,ACTIVE",
	}, "\n"))
	_ = os.Rename(loincTable, loincDir+"/LoincTable.csv")
	_ = os.WriteFile(loincDir+"/PanelHierarchy.csv", []byte(strings.Join([]string{
		"PARENTLOINC,LOINC,SEQUENCE,CARDINALITY",
		"1234-5,1234-5,1,1..1",
	}, "\n")), 0o600)

	stdout, _, err := runCLI(t, "terminology", "load", "loinc", loincDir+"/LoincTable.csv", "--version", "0.0-test", "--date", "2026-01-14", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "LOINC load complete")
	assertContains(t, stdout, "Found PanelHierarchy.csv")

	// Minimal ICD-10-CM file.
	icdDir := t.TempDir()
	icdPath := icdDir + "/icd10cm.csv"
	err = os.WriteFile(icdPath, []byte(strings.Join([]string{
		"CODE,DESCRIPTION,CATEGORY,CHAPTER,IS_BILLABLE",
		"E11.9,Type 2 diabetes mellitus without complications,E11,Chapter 4: Endocrine,1",
		"E11,Type 2 diabetes mellitus,E11,Chapter 4: Endocrine,0",
	}, "\n")), 0o600)
	assertNoError(t, err)

	stdout, _, err = runCLI(t, "terminology", "load", "icd10cm", icdPath, "--version", "0.0-test", "--date", "2026-01-14", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "ICD-10-CM load complete")
}

func TestEventStore_Live_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	requireEnv(t, "FI_FHIR_POSTGRES_ADMIN_URL")
	dsn := createEphemeralPostgresDB(t, os.Getenv("FI_FHIR_POSTGRES_ADMIN_URL"))

	table := "events_cli_test"

	stdout, _, err := runCLI(t, "eventstore", "init", "--db", dsn, "--table", table)
	assertNoError(t, err)
	assertContains(t, stdout, "Event store schema initialized successfully")

	stdout, _, err = runCLI(t, "eventstore", "append",
		"--db", dsn,
		"--table", table,
		"--stream", "patient:MRN001",
		"--type", "patient_update",
		"--data", `{"mrn":"MRN001","field":"value"}`,
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Event appended successfully")

	stdout, _, err = runCLI(t, "eventstore", "read",
		"--db", dsn,
		"--table", table,
		"--stream", "patient:MRN001",
		"--pretty",
	)
	assertNoError(t, err)
	assertContains(t, stdout, `"stream_id":`)
	assertContains(t, stdout, `"event_type":`)

	stdout, _, err = runCLI(t, "eventstore", "stats", "--db", dsn, "--table", table)
	assertNoError(t, err)
	assertContains(t, stdout, "Event Store Statistics")
	assertContains(t, stdout, "Total Events:")

	stdout, _, err = runCLI(t, "eventstore", "streams", "--db", dsn, "--table", table, "--limit", "10")
	assertNoError(t, err)
	assertContains(t, stdout, "STREAM ID")
}
