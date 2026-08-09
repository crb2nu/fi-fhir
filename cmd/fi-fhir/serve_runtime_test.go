package main

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServe_PortInUse_ReturnsError(t *testing.T) {
	configurePreviewRuntimeForTest(t)
	// Ensure optional integrations do not attempt external connections in this test.
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_PINS", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_POLICY", "pass")

	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	t.Setenv("TEMPORAL_ADDRESS", "")
	t.Setenv("TEMPORAL_NAMESPACE", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	_, _ = captureOutput(t, func() {
		err := runServe([]string{
			"--host", "127.0.0.1",
			"--port", strconv.Itoa(port),
			"--no-playground",
			"--no-introspection",
		})
		assertError(t, err)
	})
}

func TestServe_PortInUse_WithOptionalIntegrationWarnings(t *testing.T) {
	configurePreviewRuntimeForTest(t)
	// Trigger additional (but safe/fast-failing) branches in runServe:
	// - Profile store initialization fails (connection refused) -> warning path
	// - Terminology DB URL set but pins empty -> mapping store ping fails -> warning path
	// - Temporal address set -> Temporal client dial fails -> warning path
	// Server still exits quickly due to port already in use.

	t.Setenv("FI_FHIR_TERMINOLOGY_PINS", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_POLICY", "warn")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://127.0.0.1:1/fi_fhir_test?sslmode=disable&connect_timeout=1")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	t.Setenv("FI_FHIR_DATABASE_HOST", "127.0.0.1")
	t.Setenv("FI_FHIR_DATABASE_PORT", "1")
	t.Setenv("FI_FHIR_DATABASE_NAME", "fi_fhir_test")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "test")
	t.Setenv("FI_FHIR_DATABASE_PASSWORD", "test")
	t.Setenv("FI_FHIR_DATABASE_SSL_MODE", "disable")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	t.Setenv("TEMPORAL_ADDRESS", "127.0.0.1:1")
	t.Setenv("TEMPORAL_NAMESPACE", "terminology-mapping")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	workflowPath := filepath.Join("..", "..", "examples", "workflows", "adt-to-fhir.yaml")

	_, stderr, err := runCLI(t,
		"serve",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--workflow", workflowPath,
		"--no-playground",
		"--no-introspection",
	)
	assertError(t, err)

	// Slice 4.4d: these are structured lines now, so the assertion checks the
	// shape as well as the message. Every line runServe writes must be a JSON
	// object carrying the deployment tenant; a message assertion that passes
	// against an unstructured line would let the conversion silently regress.
	assertContains(t, stderr, "profile store disabled")
	assertContains(t, stderr, "mapping store disabled")
	assertContains(t, stderr, "temporal client disabled")

	lines := 0
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			t.Fatalf("runServe wrote a line that is not JSON: %q", trimmed)
		}
		if entry["tenant_id"] != "tenant-a" {
			t.Errorf("line carries no deployment tenant: %q", trimmed)
		}
		if entry["level"] == nil || entry["msg"] == nil {
			t.Errorf("line is missing level or msg: %q", trimmed)
		}
		lines++
	}
	if lines == 0 {
		t.Fatal("anti-vacuity: runServe wrote nothing, so 'all of it is JSON' proves nothing")
	}
}
