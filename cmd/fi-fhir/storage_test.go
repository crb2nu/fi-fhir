package main

import (
	"os"
	"testing"
)

// =============================================================================
// Storage Command Tests
// =============================================================================

func TestStorage_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "storage")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir storage - Object Storage Management")
	assertContains(t, stdout, "test")
	assertContains(t, stdout, "ls")
	assertContains(t, stdout, "get")
	assertContains(t, stdout, "put")
	assertContains(t, stdout, "rm")
	assertContains(t, stdout, "stat")
}

func TestStorage_HelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "storage", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir storage - Object Storage Management")
}

func TestStorage_HelpSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "storage", "help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir storage - Object Storage Management")
}

func TestStorage_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "storage", "badcmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown storage subcommand")
}

func TestStorage_List_MissingPath(t *testing.T) {
	_, _, err := runCLI(t, "storage", "ls")
	assertError(t, err)
	assertErrorContains(t, err, "path required")
}

func TestStorage_List_LocalPath(t *testing.T) {
	_, _, err := runCLI(t, "storage", "ls", "/local/path")
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL required")
}

func TestStorage_Get_MissingArgs(t *testing.T) {
	_, _, err := runCLI(t, "storage", "get")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Get_MissingDestination(t *testing.T) {
	_, _, err := runCLI(t, "storage", "get", "s3://bucket/key")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Get_NotS3URL(t *testing.T) {
	_, _, err := runCLI(t, "storage", "get", "/local/path", "/dest")
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}

func TestStorage_Put_MissingArgs(t *testing.T) {
	_, _, err := runCLI(t, "storage", "put")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Put_MissingDestination(t *testing.T) {
	_, _, err := runCLI(t, "storage", "put", "/local/file")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Put_NotS3URL(t *testing.T) {
	// Create temp file to pass the os.Stat check
	tmpFile := createTempFile(t, t.TempDir(), "test-*.txt", "test content")
	_, _, err := runCLI(t, "storage", "put", tmpFile, "/local/path")
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}

func TestStorage_Delete_MissingPath(t *testing.T) {
	_, _, err := runCLI(t, "storage", "rm")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Delete_NotS3URL(t *testing.T) {
	_, _, err := runCLI(t, "storage", "rm", "/local/path")
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}

func TestStorage_Stat_MissingPath(t *testing.T) {
	_, _, err := runCLI(t, "storage", "stat")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestStorage_Stat_NotS3URL(t *testing.T) {
	_, _, err := runCLI(t, "storage", "stat", "/local/path")
	assertError(t, err)
	assertErrorContains(t, err, "S3 URL")
}

func TestStorage_Test_MissingCredentials(t *testing.T) {
	// Unset credentials for this test
	oldAccess := os.Getenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_ACCESS_KEY")
	defer func() {
		if oldAccess != "" {
			os.Setenv("MINIO_ACCESS_KEY", oldAccess)
		}
	}()

	_, _, err := runCLI(t, "storage", "test")
	assertError(t, err)
	assertErrorContains(t, err, "MINIO_ACCESS_KEY")
}

func TestStorage_Test_SucceedsWithoutBucketCheck(t *testing.T) {
	// Provide credentials but no default bucket, so the command does not attempt
	// a networked BucketExists check and remains an offline test.
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("MINIO_USE_SSL", "false")
	t.Setenv("MINIO_ACCESS_KEY", "testaccess")
	t.Setenv("MINIO_SECRET_KEY", "testsecret")
	t.Setenv("MINIO_BUCKET", "")

	stdout, _, err := runCLI(t, "storage", "test")
	assertNoError(t, err)
	assertContains(t, stdout, "Testing MinIO connection")
	assertContains(t, stdout, "Connected successfully")
}

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestHumanizeSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1572864, "1.50 MB"},
		{1073741824, "1.00 GB"},
		{1610612736, "1.50 GB"},
		{10737418240, "10.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := humanizeSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("humanizeSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestGetMinIOConfig_Defaults(t *testing.T) {
	// Save and clear env vars
	savedVars := map[string]string{}
	envVars := []string{"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_USE_SSL", "MINIO_BUCKET"}
	for _, v := range envVars {
		savedVars[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for k, v := range savedVars {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	cfg := getMinIOConfig()

	if cfg.Endpoint != "localhost:9000" {
		t.Errorf("Expected default endpoint localhost:9000, got %s", cfg.Endpoint)
	}
	if cfg.AccessKeyID != "" {
		t.Errorf("Expected empty access key, got %s", cfg.AccessKeyID)
	}
	if cfg.SecretAccessKey != "" {
		t.Errorf("Expected empty secret key, got %s", cfg.SecretAccessKey)
	}
	if cfg.UseSSL {
		t.Error("Expected UseSSL to be false by default")
	}
	if cfg.DefaultBucket != "" {
		t.Errorf("Expected empty bucket, got %s", cfg.DefaultBucket)
	}
}

func TestGetMinIOConfig_WithEnvVars(t *testing.T) {
	// Save existing env vars
	savedVars := map[string]string{}
	envVars := []string{"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_USE_SSL", "MINIO_BUCKET"}
	for _, v := range envVars {
		savedVars[v] = os.Getenv(v)
	}
	defer func() {
		for k, v := range savedVars {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Set test values
	os.Setenv("MINIO_ENDPOINT", "test.example.com:9000")
	os.Setenv("MINIO_ACCESS_KEY", "testaccess")
	os.Setenv("MINIO_SECRET_KEY", "testsecret")
	os.Setenv("MINIO_USE_SSL", "true")
	os.Setenv("MINIO_BUCKET", "testbucket")

	cfg := getMinIOConfig()

	if cfg.Endpoint != "test.example.com:9000" {
		t.Errorf("Expected endpoint test.example.com:9000, got %s", cfg.Endpoint)
	}
	if cfg.AccessKeyID != "testaccess" {
		t.Errorf("Expected access key testaccess, got %s", cfg.AccessKeyID)
	}
	if cfg.SecretAccessKey != "testsecret" {
		t.Errorf("Expected secret key testsecret, got %s", cfg.SecretAccessKey)
	}
	if !cfg.UseSSL {
		t.Error("Expected UseSSL to be true")
	}
	if cfg.DefaultBucket != "testbucket" {
		t.Errorf("Expected bucket testbucket, got %s", cfg.DefaultBucket)
	}
}

func TestCreateMinIOProvider_MissingCredentials(t *testing.T) {
	// Save and clear env vars
	savedAccess := os.Getenv("MINIO_ACCESS_KEY")
	savedSecret := os.Getenv("MINIO_SECRET_KEY")
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	defer func() {
		if savedAccess != "" {
			os.Setenv("MINIO_ACCESS_KEY", savedAccess)
		}
		if savedSecret != "" {
			os.Setenv("MINIO_SECRET_KEY", savedSecret)
		}
	}()

	_, err := createMinIOProvider()
	if err == nil {
		t.Error("Expected error when credentials are missing")
	}
	if err != nil && !contains(err.Error(), "MINIO_ACCESS_KEY") && !contains(err.Error(), "MINIO_SECRET_KEY") {
		t.Errorf("Expected error about credentials, got: %v", err)
	}
}

// contains is a simple string contains helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
