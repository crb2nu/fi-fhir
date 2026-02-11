//go:build integration
// +build integration

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// These are opt-in integration tests meant to exercise real code paths against
// PostgreSQL + MinIO when available.
//
// Run:
//   go test -tags=integration ./cmd/fi-fhir/...
//
// Config:
// - If FI_FHIR_DATABASE_URL / FI_FHIR_MINIO_* are set, those are used.
// - Otherwise, cmd/fi-fhir/integration_helpers_test.go will attempt to provision
//   testcontainers (Docker required). If Docker isn't available, tests are skipped.

func TestIntegration_Storage_PutGetDelete_MinIO(t *testing.T) {
	setupTestInfra(t)

	// Use the default bucket created by setupTestInfra.
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "terminology"
	}

	tmpDir := t.TempDir()
	testContent := "Hello, MinIO integration test!"
	localFile := filepath.Join(tmpDir, "test-upload.txt")
	if err := os.WriteFile(localFile, []byte(testContent), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	s3Path := "s3://" + bucket + "/integration-test/test-upload.txt"

	_, _, err := runCLI(t, "storage", "put", localFile, s3Path)
	assertNoError(t, err)

	downloadPath := filepath.Join(tmpDir, "test-download.txt")
	_, _, err = runCLI(t, "storage", "get", s3Path, downloadPath)
	assertNoError(t, err)

	got, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if string(got) != testContent {
		t.Fatalf("download content mismatch: got %q want %q", string(got), testContent)
	}

	_, _, err = runCLI(t, "storage", "rm", s3Path)
	assertNoError(t, err)
}

func TestIntegration_EventStore_FullLifecycle_Postgres(t *testing.T) {
	setupTestInfra(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")
	if dbURL == "" {
		t.Skip("FI_FHIR_DATABASE_URL not configured (and containers not provisioned)")
	}

	tableName := "integration_test_events"

	// Init schema
	_, _, err := runCLI(t, "eventstore", "init", "--db", dbURL, "--table", tableName)
	assertNoError(t, err)

	// Append event
	_, _, err = runCLI(t, "eventstore", "append",
		"--db", dbURL,
		"--table", tableName,
		"--stream", "patient:integration",
		"--type", "PatientAdmitted",
		"--data", `{"patient_id":"P-INT-001","facility":"ICU","reason":"integration test"}`,
	)
	assertNoError(t, err)

	// Read back
	stdout, _, err := runCLI(t, "eventstore", "read",
		"--db", dbURL,
		"--table", tableName,
		"--stream", "patient:integration",
		"--limit", "10",
	)
	assertNoError(t, err)
	assertContains(t, stdout, "PatientAdmitted")
	assertContains(t, stdout, "P-INT-001")

	// Stats
	_, _, err = runCLI(t, "eventstore", "stats", "--db", dbURL, "--table", tableName)
	assertNoError(t, err)
}

func TestIntegration_Terminology_InitAndStatus_Postgres(t *testing.T) {
	setupTestInfra(t)
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")
	if dbURL == "" {
		t.Skip("FI_FHIR_DATABASE_URL not configured (and containers not provisioned)")
	}

	_, _, err := runCLI(t, "terminology", "init", "--db", dbURL)
	assertNoError(t, err)

	stdout, _, err := runCLI(t, "terminology", "status", "--db", dbURL)
	assertNoError(t, err)
	assertContains(t, stdout, "Terminology Database Status")
}

func TestIntegration_Workflow_Validate_Sample(t *testing.T) {
	// Offline: validate a minimal workflow that should always be valid.
	// The shipped examples may be intentionally incomplete or environment-specific.
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	data := []byte(`name: integration-test
version: v1
routes:
  - name: all
    filter: {}
    actions:
      - type: webhook
        url: http://example.invalid
        method: POST
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	_, _, err := runCLI(t, "workflow", "validate", "--config", path)
	assertNoError(t, err)
}
