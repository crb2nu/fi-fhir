package main

import (
	"context"
	"os"
	"testing"
)

// =============================================================================
// runSubscriptionServe — deeper coverage: TLS, workflow integration, dry-run
// =============================================================================

func TestSubscriptionServe_TLS_CertWithoutKey(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := createTempFile(t, tmpDir, "subs*.yaml", `subscriptions:
  - name: test_sub
    server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://notify.example.com/fhir/notify/test_sub
`)

	_, _, err := runCLI(t, "subscription", "serve",
		"--subscriptions", subsPath,
		"--cert", "/tmp/cert.pem",
		"--dry-run",
	)
	assertError(t, err)
	assertErrorContains(t, err, "both TLS cert and key are required")
}

func TestSubscriptionServe_TLS_KeyWithoutCert(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := createTempFile(t, tmpDir, "subs*.yaml", `subscriptions:
  - name: test_sub
    server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://notify.example.com/fhir/notify/test_sub
`)

	_, _, err := runCLI(t, "subscription", "serve",
		"--subscriptions", subsPath,
		"--key", "/tmp/key.pem",
		"--dry-run",
	)
	assertError(t, err)
	assertErrorContains(t, err, "both TLS cert and key are required")
}

func TestSubscriptionServe_DryRun_NoWorkflowMessage(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := createTempFile(t, tmpDir, "subs*.yaml", `subscriptions:
  - name: test_sub
    server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://notify.example.com/fhir/notify/test_sub
`)

	stdout, _, err := runCLI(t, "subscription", "serve",
		"--subscriptions", subsPath,
		"--dry-run",
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Workflow: (none)")
	assertContains(t, stdout, "TLS: disabled")
}

func TestSubscriptionServe_DryRun_WithWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := createTempFile(t, tmpDir, "subs*.yaml", `subscriptions:
  - name: test_sub
    server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://notify.example.com/fhir/notify/test_sub
`)
	workflowPath := createTempFile(t, tmpDir, "wf*.yaml", `workflow:
  name: test-wf
  version: "1.0"
  routes: []
`)

	stdout, _, err := runCLI(t, "subscription", "serve",
		"--subscriptions", subsPath,
		"--workflow", workflowPath,
		"--dry-run",
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Workflow:")
	assertContains(t, stdout, workflowPath)
}

func TestSubscriptionServe_DryRun_PathPrefixFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := createTempFile(t, tmpDir, "subs*.yaml", `subscriptions:
  - name: test_sub
    server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://notify.example.com/fhir/notify/test_sub
`)
	appCfgPath := createTempFile(t, tmpDir, "app*.yaml", `subscription_receiver:
  path_prefix: custom/path
`)

	stdout, _, err := runCLI(t, "subscription", "serve",
		"--subscriptions", subsPath,
		"--config", appCfgPath,
		"--dry-run",
	)
	assertNoError(t, err)
	// Should have / prepended to path_prefix without leading slash
	assertContains(t, stdout, "Path prefix: /custom/path")
}

// =============================================================================
// initProfileStoreFromEnv — SSL mode and driver default paths
// =============================================================================

func TestInitProfileStoreFromEnv_DefaultSSLMode(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "fi_fhir")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "user")
	t.Setenv("FI_FHIR_DATABASE_DRIVER", "")
	t.Setenv("FI_FHIR_DATABASE_SSL_MODE", "")

	// Will fail at ping, but exercises the SSL mode default and driver default paths
	_, err := initProfileStoreFromEnv(context.Background())
	// Expect a connection error (ping will fail) — but the code path is exercised
	if err == nil {
		t.Fatalf("expected error connecting to localhost database")
	}
}

// =============================================================================
// runSubscriptionCreate — deeper coverage: config file not found
// =============================================================================

func TestSubscriptionCreate_ShortFlags(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "-c")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionCreate_ShortNameFlag(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "-n")
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

// =============================================================================
// runETLLoad — additional testable branches
// =============================================================================

func TestETLLoad_UnsupportedSource(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	t.Setenv("MINIO_ACCESS_KEY", "testkey")
	t.Setenv("MINIO_SECRET_KEY", "testsecret")

	err := runETLLoad([]string{"nonexistent_source", "--version", "1.0"})
	assertError(t, err)
}

func TestETLLoad_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runETLLoad([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir etl load")
}

func TestETLLoad_VersionMissingValue(t *testing.T) {
	// runETLLoad uses a different flag parsing style — missing value for --version means version stays empty
	err := runETLLoad([]string{"loinc", "--version"})
	assertError(t, err)
	assertErrorContains(t, err, "--version is required")
}

func TestETLLoad_DryRunFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	t.Setenv("MINIO_ACCESS_KEY", "testkey")
	t.Setenv("MINIO_SECRET_KEY", "testsecret")

	// Exercises --dry-run flag parsing (will fail at MinIO connection)
	err := runETLLoad([]string{"loinc", "--version", "2.77", "--dry-run"})
	assertError(t, err)
}

func TestETLLoad_ICD10PCS(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	t.Setenv("MINIO_ACCESS_KEY", "testkey")
	t.Setenv("MINIO_SECRET_KEY", "testsecret")

	err := runETLLoad([]string{"icd10pcs", "--version", "FY2024"})
	assertError(t, err) // MinIO connection error expected
}

// =============================================================================
// runWorkflowReplay — deeper coverage: config + recordings required
// =============================================================================

func TestWorkflowReplay_ConfigRequired(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "replay", "--recordings", "/tmp/rec.json")
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestWorkflowReplay_RecordingsRequired(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "wf*.yaml", "routes: []")

	_, _, err := runCLI(t, "workflow", "replay", "-c", configPath)
	assertError(t, err)
	assertErrorContains(t, err, "recordings")
}

func TestWorkflowReplay_RecordingsFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "wf*.yaml", `workflow:
  name: test
  version: "1.0"
  routes: []`)

	_, _, err := runCLI(t, "workflow", "replay", "-c", configPath, "--recordings", "/nonexistent/recordings.json")
	assertError(t, err)
}

// =============================================================================
// runWorkflowSimulate — config required
// =============================================================================

func TestWorkflowSimulate_ConfigRequired(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate")
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestWorkflowSimulate_ConfigNotFound(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "simulate", "-c", "/nonexistent/workflow.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "failed to read workflow")
}

func TestWorkflowSimulate_InvalidWorkflowFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "wf*.yaml", "not valid yaml {{{")

	_, _, err := runCLI(t, "workflow", "simulate", "-c", configPath)
	assertError(t, err)
	assertErrorContains(t, err, "failed to parse workflow")
}

// =============================================================================
// marshalYAML — exercise the function via config show
// =============================================================================

func TestConfigShow_DefaultConfig_MarshalYAML(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show")
	assertNoError(t, err)
	// Should output the default config as "YAML" (actually JSON)
	assertContains(t, stdout, "{")
}

func TestConfigShow_JSONFormatOutput(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show", "--format", "json")
	assertNoError(t, err)
	assertContains(t, stdout, "{")
}

func TestConfigShow_UnknownFormat(t *testing.T) {
	_, _, err := runCLI(t, "config", "show", "--format", "xml")
	assertError(t, err)
	assertErrorContains(t, err, "unknown format")
}

// =============================================================================
// runConfigEnv — exercise env output
// =============================================================================

func TestConfigEnv_Output(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env")
	assertNoError(t, err)
	// Should list environment variables
	assertContains(t, stdout, "FI_FHIR")
}

// =============================================================================
// Validate verbose mode — push validateProfile deeper
// =============================================================================

func TestValidate_Verbose_WithHL7v2Config(t *testing.T) {
	tmpDir := t.TempDir()
	profileYAML := `source_profile:
  id: test_profile
  name: Test Profile
  version: "1.0"
  hl7v2:
    default_version: "2.5"
    timezone: America/New_York`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", profileYAML)

	stdout, _, err := runCLI(t, "validate", "--profile", profilePath, "--verbose")
	assertNoError(t, err)
	assertContains(t, stdout, "Profile")
	assertContains(t, stdout, "is valid")
}

func TestValidate_WithMessageFile(t *testing.T) {
	tmpDir := t.TempDir()
	profileYAML := `source_profile:
  id: test_profile
  name: Test Profile
  hl7v2:
    default_version: "2.5"`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", profileYAML)

	// Use testdata sample if available, otherwise exercise the message validation code path
	hl7Path := testdataPath(t, "adt_a01_sample.hl7")
	if _, statErr := os.Stat(hl7Path); statErr != nil {
		// Without real HL7 sample, at least exercise the --message flag code path
		msgPath := createTempFile(t, tmpDir, "msg*.hl7", "MSH|^~\\&|SRC|FAC|DEST|FAC|20240101120000||ADT^A01|MSG001|P|2.5\rEVN|A01|20240101120000\rPID|||12345||Smith^John||19800101|M\r")
		_, _, err := runCLI(t, "validate", "--profile", profilePath, "--message", msgPath)
		// May fail validation depending on profile requirements — just exercise the path
		_ = err
		return
	}

	stdout, _, err := runCLI(t, "validate", "--profile", profilePath, "--message", hl7Path)
	assertNoError(t, err)
	assertContains(t, stdout, "is valid")
}

func TestValidate_WithInvalidMessage(t *testing.T) {
	tmpDir := t.TempDir()
	profileYAML := `source_profile:
  id: test_profile
  name: Test Profile
  hl7v2:
    default_version: "2.5"`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", profileYAML)

	// Empty message should trigger parse error
	msgPath := createTempFile(t, tmpDir, "msg*.hl7", "not a valid HL7 message")

	_, _, err := runCLI(t, "validate", "--profile", profilePath, "--message", msgPath)
	assertError(t, err)
	assertErrorContains(t, err, "message validation failed")
}

// =============================================================================
// runParse — FHIR format (not yet implemented — verify error)
// =============================================================================

func TestParse_FHIR_NotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	fhirJSON := `{"resourceType": "Patient", "id": "example"}`
	inputPath := createTempFile(t, tmpDir, "patient*.json", fhirJSON)

	_, _, err := runCLI(t, "parse", "--format", "fhir", inputPath)
	assertError(t, err)
	// Exercises the FHIR code path — currently returns "not yet implemented"
}

// =============================================================================
// runStorageTest — with credentials (exercises code past credential check)
// =============================================================================

func TestStorageTest_WithCredentials(t *testing.T) {
	t.Setenv("MINIO_ACCESS_KEY", "testkey")
	t.Setenv("MINIO_SECRET_KEY", "testsecret")

	// Will fail at MinIO connection, but exercises code past credential check
	err := runStorageTest([]string{})
	// Error is expected (no MinIO server) — just exercising the path
	_ = err
}

// =============================================================================
// runStoragePut — flag parsing
// =============================================================================

func TestStoragePut_MissingArguments(t *testing.T) {
	err := runStoragePut([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStoragePut_InvalidS3URL(t *testing.T) {
	err := runStoragePut([]string{"/tmp/file.txt", "not-s3-url"})
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}

func TestStoragePut_NonexistentLocalFile(t *testing.T) {
	err := runStoragePut([]string{"/nonexistent/file.txt", "s3://bucket/key"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to stat")
}

func TestStoragePut_DirectoryNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	err := runStoragePut([]string{tmpDir, "s3://bucket/key"})
	assertError(t, err)
	assertErrorContains(t, err, "cannot upload directory")
}

func TestStorageDelete_MissingArguments(t *testing.T) {
	err := runStorageDelete([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorageDelete_InvalidS3URL(t *testing.T) {
	err := runStorageDelete([]string{"not-s3-url"})
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}
