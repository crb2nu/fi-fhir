//go:build integration
// +build integration

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ===========================================================================
// Integration Test Configuration
// ===========================================================================
//
// These tests are opt-in and require infrastructure credentials via env vars.
//
// Required:
//   FI_FHIR_DATABASE_URL     - PostgreSQL connection URL
//   FI_FHIR_MINIO_ENDPOINT   - MinIO endpoint (host:port)
//   FI_FHIR_MINIO_ACCESS_KEY - MinIO access key
//   FI_FHIR_MINIO_SECRET_KEY - MinIO secret key
//
// Optional:
//   FI_FHIR_FHIR_SERVER_URL  - HAPI FHIR server URL (for subscription tests)
//
// Run with: go test -tags=integration ./cmd/fi-fhir/...
//
// ===========================================================================

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	val := os.Getenv(key)
	if val == "" {
		t.Skipf("missing %s (integration infrastructure not configured)", key)
	}
	return val
}

func getDatabaseURL(t *testing.T) string {
	t.Helper()
	return requireEnv(t, "FI_FHIR_DATABASE_URL")
}

func getMinioConfig(t *testing.T) (endpoint, accessKey, secretKey string) {
	t.Helper()
	endpoint = requireEnv(t, "FI_FHIR_MINIO_ENDPOINT")
	accessKey = requireEnv(t, "FI_FHIR_MINIO_ACCESS_KEY")
	secretKey = requireEnv(t, "FI_FHIR_MINIO_SECRET_KEY")
	return endpoint, accessKey, secretKey
}

// ===========================================================================
// Storage Integration Tests (MinIO)
// ===========================================================================

func TestIntegration_Storage_Test_MinIO(t *testing.T) {
	minioEndpoint, minioAccessKey, minioSecretKey := getMinioConfig(t)

	stdout, _, err := runCLI(t, "storage", "test",
		"--provider", "minio",
		"--endpoint", minioEndpoint,
		"--access-key", minioAccessKey,
		"--secret-key", minioSecretKey,
		"--bucket", "fi-fhir-test",
		"--insecure")
	if err != nil {
		t.Fatalf("Storage test failed: %v", err)
	}
	t.Logf("Storage test output: %s", stdout)
}

func TestIntegration_Storage_List_MinIO(t *testing.T) {
	minioEndpoint, minioAccessKey, minioSecretKey := getMinioConfig(t)

	stdout, _, err := runCLI(t, "storage", "list",
		"--provider", "minio",
		"--endpoint", minioEndpoint,
		"--access-key", minioAccessKey,
		"--secret-key", minioSecretKey,
		"--bucket", "fi-fhir-test",
		"--insecure")
	if err != nil {
		t.Logf("Storage list error (bucket may not exist): %v", err)
	}
	t.Logf("Storage list output: %s", stdout)
}

func TestIntegration_Storage_PutGetDelete_MinIO(t *testing.T) {
	minioEndpoint, minioAccessKey, minioSecretKey := getMinioConfig(t)

	// Create a test file
	tmpDir := t.TempDir()
	testContent := "Hello, MinIO integration test!"
	testFile := filepath.Join(tmpDir, "test-upload.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Put file
	_, _, err := runCLI(t, "storage", "put",
		"--provider", "minio",
		"--endpoint", minioEndpoint,
		"--access-key", minioAccessKey,
		"--secret-key", minioSecretKey,
		"--bucket", "fi-fhir-test",
		"--insecure",
		"--path", "integration-test/test-upload.txt",
		"--file", testFile)
	if err != nil {
		t.Fatalf("Storage put failed: %v", err)
	}

	// Get file
	downloadFile := filepath.Join(tmpDir, "test-download.txt")
	_, _, err = runCLI(t, "storage", "get",
		"--provider", "minio",
		"--endpoint", minioEndpoint,
		"--access-key", minioAccessKey,
		"--secret-key", minioSecretKey,
		"--bucket", "fi-fhir-test",
		"--insecure",
		"--path", "integration-test/test-upload.txt",
		"--output", downloadFile)
	if err != nil {
		t.Fatalf("Storage get failed: %v", err)
	}

	// Verify content
	downloaded, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(downloaded) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(downloaded), testContent)
	}

	// Delete file
	_, _, err = runCLI(t, "storage", "delete",
		"--provider", "minio",
		"--endpoint", minioEndpoint,
		"--access-key", minioAccessKey,
		"--secret-key", minioSecretKey,
		"--bucket", "fi-fhir-test",
		"--insecure",
		"--path", "integration-test/test-upload.txt")
	if err != nil {
		t.Fatalf("Storage delete failed: %v", err)
	}
}

// ===========================================================================
// EventStore Integration Tests (PostgreSQL)
// ===========================================================================

func TestIntegration_EventStore_FullLifecycle(t *testing.T) {
	dbURL := getDatabaseURL(t)
	tableName := "integration_test_events"

	// Init
	stdout, _, err := runCLI(t, "eventstore", "init", "--db", dbURL, "--table", tableName)
	if err != nil {
		t.Fatalf("EventStore init failed: %v", err)
	}
	t.Logf("EventStore init: %s", stdout)

	// Append event
	stdout, _, err = runCLI(t, "eventstore", "append",
		"--db", dbURL,
		"--table", tableName,
		"--stream", "patient-integration-test",
		"--type", "PatientAdmitted",
		"--data", `{"patient_id":"P-INT-001","facility":"ICU","reason":"integration test"}`)
	if err != nil {
		t.Fatalf("EventStore append failed: %v", err)
	}
	t.Logf("EventStore append: %s", stdout)

	// Read events
	stdout, _, err = runCLI(t, "eventstore", "read",
		"--db", dbURL,
		"--table", tableName,
		"--stream", "patient-integration-test",
		"--json")
	if err != nil {
		t.Fatalf("EventStore read failed: %v", err)
	}
	assertContains(t, stdout, "PatientAdmitted")
	assertContains(t, stdout, "P-INT-001")

	// Stats
	stdout, _, err = runCLI(t, "eventstore", "stats",
		"--db", dbURL,
		"--table", tableName)
	if err != nil {
		t.Fatalf("EventStore stats failed: %v", err)
	}
	t.Logf("EventStore stats: %s", stdout)

	// Streams
	stdout, _, err = runCLI(t, "eventstore", "streams",
		"--db", dbURL,
		"--table", tableName)
	if err != nil {
		t.Fatalf("EventStore streams failed: %v", err)
	}
	assertContains(t, stdout, "patient-integration-test")
}

// ===========================================================================
// Projection Integration Tests (PostgreSQL)
// ===========================================================================

func TestIntegration_Projection_RunAll(t *testing.T) {
	dbURL := getDatabaseURL(t)
	tableName := "integration_test_projections"

	// Init event store first
	_, _, err := runCLI(t, "eventstore", "init", "--db", dbURL, "--table", tableName)
	if err != nil {
		t.Fatalf("EventStore init failed: %v", err)
	}

	// Add some events for projections
	events := []struct {
		stream    string
		eventType string
		data      string
	}{
		{"patient-proj-001", "PatientAdmitted", `{"patient_id":"P001","facility":"ER"}`},
		{"patient-proj-001", "PatientTransferred", `{"patient_id":"P001","from":"ER","to":"ICU"}`},
		{"patient-proj-002", "PatientAdmitted", `{"patient_id":"P002","facility":"Ward"}`},
	}

	for _, e := range events {
		_, _, err = runCLI(t, "eventstore", "append",
			"--db", dbURL,
			"--table", tableName,
			"--stream", e.stream,
			"--type", e.eventType,
			"--data", e.data)
		if err != nil {
			t.Fatalf("EventStore append failed: %v", err)
		}
	}

	// Run projections
	projections := []string{"patient_timeline", "statistics", "active_encounters"}
	for _, proj := range projections {
		stdout, _, err := runCLI(t, "projection", "run",
			"--db", dbURL,
			"--table", tableName,
			"--projection", proj)
		if err != nil {
			t.Logf("Projection %s error (may not have matching events): %v", proj, err)
		} else {
			t.Logf("Projection %s: %s", proj, stdout)
		}
	}

	// Projection status
	stdout, _, err := runCLI(t, "projection", "status",
		"--db", dbURL,
		"--table", tableName,
		"--json")
	if err != nil {
		t.Logf("Projection status error: %v", err)
	}
	t.Logf("Projection status: %s", stdout)
}

// ===========================================================================
// Terminology Integration Tests (PostgreSQL)
// ===========================================================================

func TestIntegration_Terminology_InitAndStatus(t *testing.T) {
	dbURL := getDatabaseURL(t)

	// Set terminology DB URL
	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// Init
	stdout, _, err := runCLI(t, "terminology", "init")
	if err != nil {
		t.Fatalf("Terminology init failed: %v", err)
	}
	t.Logf("Terminology init: %s", stdout)

	// Status
	stdout, _, err = runCLI(t, "terminology", "status", "--json")
	if err != nil {
		t.Fatalf("Terminology status failed: %v", err)
	}
	t.Logf("Terminology status: %s", stdout)
}

func TestIntegration_Terminology_Crosswalk(t *testing.T) {
	dbURL := getDatabaseURL(t)

	oldVal := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL")
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", dbURL)
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_TERMINOLOGY_DB_URL")
		}
	}()

	// Ensure init
	_, _, _ = runCLI(t, "terminology", "init")

	// Try crosswalk (will likely find no mappings without loaded data)
	stdout, _, err := runCLI(t, "terminology", "crosswalk",
		"--source", "snomed",
		"--target", "icd10",
		"386661006") // Fever SNOMED code
	if err != nil {
		t.Logf("Terminology crosswalk error (expected without loaded data): %v", err)
	}
	t.Logf("Terminology crosswalk: %s", stdout)
}

// ===========================================================================
// ETL Integration Tests
// ===========================================================================

func TestIntegration_ETL_Status(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "status", "--verbose")
	if err != nil {
		t.Logf("ETL status error: %v", err)
	}
	t.Logf("ETL status: %s", stdout)
}

func TestIntegration_ETL_Sources(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "sources", "--verbose")
	if err != nil {
		t.Logf("ETL sources error: %v", err)
	}
	t.Logf("ETL sources: %s", stdout)
	// Should list available ETL sources
}

// ===========================================================================
// Workflow Integration Tests
// ===========================================================================

func TestIntegration_Workflow_DryRun_WithEvents(t *testing.T) {
	tmpDir := t.TempDir()

	workflowYAML := `
id: integration_workflow
description: Integration test workflow
version: "1.0"
triggers:
  - type: adt
    event_types: ["admission", "discharge", "transfer"]
routes:
  - id: log_all_events
    filter: "true"
    actions:
      - type: log
        level: info
        message: "Processing event: ${event.type}"
  - id: admission_handler
    filter: "event.type == 'admission'"
    actions:
      - type: log
        level: info
        message: "New admission for patient ${event.patient.id}"
`
	workflowPath := filepath.Join(tmpDir, "workflow.yaml")
	if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0600); err != nil {
		t.Fatalf("Failed to write workflow: %v", err)
	}

	inputEvents := `[
		{"type": "admission", "patient": {"id": "P001"}, "facility": "ER"},
		{"type": "transfer", "patient": {"id": "P001"}, "from": "ER", "to": "ICU"},
		{"type": "discharge", "patient": {"id": "P001"}, "disposition": "home"}
	]`
	inputPath := filepath.Join(tmpDir, "events.json")
	if err := os.WriteFile(inputPath, []byte(inputEvents), 0600); err != nil {
		t.Fatalf("Failed to write input: %v", err)
	}

	stdout, _, err := runCLI(t, "workflow", "dry-run",
		"--workflow", workflowPath,
		"--input", inputPath,
		"--verbose")
	if err != nil {
		t.Fatalf("Workflow dry-run failed: %v", err)
	}
	t.Logf("Workflow dry-run output: %s", stdout)
}

// ===========================================================================
// Parse Integration Tests (with real files)
// ===========================================================================

func TestIntegration_Parse_HL7v2_ADT(t *testing.T) {
	tmpDir := t.TempDir()

	// Real-world ADT A01 message
	hl7Message := "MSH|^~\\&|EPIC|HOSPITAL|FI-FHIR|INTEGRATION|20240115120000||ADT^A01^ADT_A01|MSG00001|P|2.5.1|||AL|NE\r" +
		"EVN|A01|20240115120000|||JSMITH^Smith^John^A^MD\r" +
		"PID|1||MRN123456^^^HOSPITAL^MR~SSN123456789^^^SSA^SS||DOE^JOHN^WILLIAM^^JR|SMITH^JANE|19850315|M||2106-3^White^HL70005|123 MAIN ST^^ANYTOWN^CA^90210^USA||555-123-4567|555-987-6543||S||ACCT987654|SSN123456789\r" +
		"PV1|1|I|ICU^101^A^HOSPITAL||||ATTEN001^ATTENDING^DOCTOR|||MED||||ADM|A0||VISIT001|||||||||||||||||||||||||20240115120000\r" +
		"PV2|||^Chest pain||||||20240115|20240120\r" +
		"DG1|1||I21.9^Acute myocardial infarction, unspecified^ICD10|||A\r"

	msgPath := filepath.Join(tmpDir, "adt_a01.hl7")
	if err := os.WriteFile(msgPath, []byte(hl7Message), 0600); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	stdout, _, err := runCLI(t, "parse",
		"--format", "hl7v2",
		"--source", "epic_integration_test",
		"--output", "json",
		msgPath)
	if err != nil {
		t.Fatalf("Parse HL7v2 failed: %v", err)
	}

	// Verify extracted data
	assertContains(t, stdout, "MRN123456")
	assertContains(t, stdout, "DOE")
	assertContains(t, stdout, "JOHN")
	assertContains(t, stdout, "admission")
	t.Logf("Parse HL7v2 ADT output length: %d bytes", len(stdout))
}

func TestIntegration_Parse_EDI_837P(t *testing.T) {
	ediPath := filepath.Join("..", "..", "testdata", "edi", "837p_minimal.edi")
	if _, err := os.Stat(ediPath); os.IsNotExist(err) {
		t.Skip("EDI test file not found")
	}

	stdout, _, err := runCLI(t, "parse",
		"--format", "edi",
		"--source", "clearinghouse_test",
		"--edi-companion", "auto",
		ediPath)
	if err != nil {
		t.Fatalf("Parse EDI 837P failed: %v", err)
	}

	t.Logf("Parse EDI 837P output length: %d bytes", len(stdout))
}

// ===========================================================================
// Validate Integration Tests
// ===========================================================================

func TestIntegration_Validate_HL7v2_WithProfile(t *testing.T) {
	tmpDir := t.TempDir()

	profileYAML := `
source_profile:
  id: integration_test_profile
  name: Integration Test Profile
  format: hl7v2
  hl7v2:
    version: "2.5.1"
    tolerance:
      missing_segments:
        - NK1
        - IN1
        - IN2
    z_segments:
      - segment: ZPD
        fields:
          - index: 1
            name: custom_patient_type
            mapping: patient.extension.custom_type
`
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0600); err != nil {
		t.Fatalf("Failed to write profile: %v", err)
	}

	hl7Message := "MSH|^~\\&|SENDER|FACILITY|RECEIVER|DEST|20240115120000||ADT^A01|MSG001|P|2.5.1\r" +
		"EVN|A01|20240115120000\r" +
		"PID|||MRN789^^^HOSP||INTEGRATION^TEST||19900101|F\r" +
		"PV1||I|ICU\r" +
		"ZPD|CUSTOM_TYPE_VALUE\r"

	msgPath := filepath.Join(tmpDir, "message.hl7")
	if err := os.WriteFile(msgPath, []byte(hl7Message), 0600); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	stdout, _, err := runCLI(t, "validate",
		"--message", msgPath,
		"--format", "hl7v2",
		"--profile", profilePath)
	if err != nil {
		t.Logf("Validate with profile error (may have warnings): %v", err)
	}
	t.Logf("Validate output: %s", stdout)
}
