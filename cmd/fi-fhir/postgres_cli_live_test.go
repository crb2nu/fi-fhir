//go:build live
// +build live

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

func writeRRFLine(t *testing.T, path string, expectedColumns int, fields []string) {
	t.Helper()

	if len(fields) != expectedColumns {
		t.Fatalf("expected %d RRF columns, got %d", expectedColumns, len(fields))
	}
	if fields[len(fields)-1] == "" {
		t.Fatalf("last RRF column must be non-empty (trailing pipe is trimmed on read)")
	}

	line := strings.Join(fields, "|") + "|\n"
	assertNoError(t, os.WriteFile(path, []byte(line), 0o600))
}

func waitForPostgres(t *testing.T, dsn string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		_ = db.Close()

		if err == nil {
			return
		}

		lastErr = err
		time.Sleep(2 * time.Second)
	}

	t.Skipf("skipping: postgres not reachable within %s: %v", timeout, lastErr)
}

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

	waitForPostgres(t, maintenance.String(), 60*time.Second)

	maintDB, err := sql.Open("postgres", maintenance.String())
	if err != nil {
		t.Skipf("skipping: open maintenance DB failed: %v", err)
	}
	t.Cleanup(func() { _ = maintDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

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

	testDSN := testURL.String()
	waitForPostgres(t, testDSN, 60*time.Second)
	return testDSN
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
	assertNoError(t, os.Rename(loincTable, loincDir+"/LoincTable.csv"))
	assertNoError(t, os.WriteFile(loincDir+"/PanelHierarchy.csv", []byte(strings.Join([]string{
		"PARENTLOINC,LOINC,SEQUENCE,CARDINALITY",
		"1234-5,1234-5,1,1..1",
	}, "\n")), 0o600))

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

func TestTerminology_Live_Load_UMLS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	requireEnv(t, "FI_FHIR_POSTGRES_ADMIN_URL")
	dsn := createEphemeralPostgresDB(t, os.Getenv("FI_FHIR_POSTGRES_ADMIN_URL"))

	metaDir := t.TempDir()

	// MRCONSO: 18 columns
	writeRRFLine(t, metaDir+"/MRCONSO.RRF", 18, []string{
		"C0000005",     // CUI
		"ENG",          // LAT
		"P",            // TS
		"L0000005",     // LUI
		"PF",           // STT
		"S0000005",     // SUI
		"Y",            // ISPREF
		"A0000005",     // AUI
		"",             // SAUI
		"",             // SCUI
		"",             // SDUI
		"SNOMEDCT_US",  // SAB
		"PT",           // TTY
		"12345",        // CODE
		"Test concept", // STR
		"0",            // SRL
		"N",            // SUPPRESS
		"0",            // CVF
	})

	// MRREL: 16 columns
	writeRRFLine(t, metaDir+"/MRREL.RRF", 16, []string{
		"C0000005", // CUI1
		"",         // AUI1
		"CUI",      // STYPE1
		"RB",       // REL
		"C0000006", // CUI2
		"",         // AUI2
		"CUI",      // STYPE2
		"",         // RELA
		"",         // RUI
		"",         // SRUI
		"SNOMEDCT_US",
		"",  // SL
		"",  // RG
		"",  // DIR
		"N", // SUPPRESS
		"0", // CVF
	})

	// MRSTY: 6 columns
	writeRRFLine(t, metaDir+"/MRSTY.RRF", 6, []string{
		"C0000005",            // CUI
		"T047",                // TUI
		"1",                   // STN
		"Disease or Syndrome", // STY
		"",                    // ATUI
		"0",                   // CVF
	})

	stdout, _, err := runCLI(t, "terminology", "load", "umls", metaDir, "--version", "0.0-test", "--date", "2026-01-14", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "UMLS load complete")
	assertContains(t, stdout, "Concepts (MRCONSO):")
	assertContains(t, stdout, "Relations (MRREL):")
	assertContains(t, stdout, "Semantic Types (MRSTY):")
}

func TestTerminology_Live_Load_RxNorm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	requireEnv(t, "FI_FHIR_POSTGRES_ADMIN_URL")
	dsn := createEphemeralPostgresDB(t, os.Getenv("FI_FHIR_POSTGRES_ADMIN_URL"))

	rrfDir := t.TempDir()

	// RXNCONSO: 18 columns
	writeRRFLine(t, rrfDir+"/RXNCONSO.RRF", 18, []string{
		"316076",                // RXCUI
		"ENG",                   // LAT
		"P",                     // TS
		"L0000001",              // LUI
		"PF",                    // STT
		"S0000001",              // SUI
		"Y",                     // ISPREF
		"RXA000001",             // RXAUI
		"",                      // SAUI
		"",                      // SCUI
		"",                      // SDUI
		"RXNORM",                // SAB
		"SCD",                   // TTY
		"123456",                // CODE
		"Test drug 1 MG Tablet", // STR
		"0",                     // SRL
		"N",                     // SUPPRESS
		"0",                     // CVF
	})

	// RXNREL: 16 columns
	writeRRFLine(t, rrfDir+"/RXNREL.RRF", 16, []string{
		"316076",    // RXCUI1
		"RXA000001", // RXAUI1
		"RXAUI",     // STYPE1
		"RO",        // REL
		"316077",    // RXCUI2
		"RXA000002", // RXAUI2
		"RXAUI",     // STYPE2
		"",          // RELA
		"",          // RUI
		"",          // SRUI
		"RXNORM",    // SAB
		"",          // SL
		"",          // RG
		"",          // DIR
		"N",         // SUPPRESS
		"0",         // CVF
	})

	// RXNSAT: 13 columns (NDC attribute)
	writeRRFLine(t, rrfDir+"/RXNSAT.RRF", 13, []string{
		"316076",        // RXCUI
		"",              // LUI
		"",              // SUI
		"RXA000001",     // RXAUI
		"RXAUI",         // STYPE
		"",              // CODE
		"",              // ATUI
		"",              // SATUI
		"NDC",           // ATN
		"RXNORM",        // SAB
		"00002-3234-01", // ATV
		"N",             // SUPPRESS
		"0",             // CVF
	})

	stdout, _, err := runCLI(t, "terminology", "load", "rxnorm", rrfDir, "--version", "0.0-test", "--date", "2026-01-14", "--db", dsn)
	assertNoError(t, err)
	assertContains(t, stdout, "RxNorm load complete")
	assertContains(t, stdout, "NDC Cross-refs:")
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

	// Projection status before any events are appended.
	stdout, _, err = runCLI(t, "projection", "status", "--db", dsn, "--table", table)
	assertNoError(t, err)
	assertContains(t, stdout, "Projection Status")
	assertContains(t, stdout, "patient_timeline")

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

	// Run projections against the event store + checkpoint store.
	stdout, _, err = runCLI(t, "projection", "run", "--db", dsn, "--table", table)
	assertNoError(t, err)
	assertContains(t, stdout, "Projections updated successfully")
}
