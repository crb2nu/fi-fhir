package main

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
)

func TestServe_PortInUse_ReturnsError(t *testing.T) {
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

	assertContains(t, stderr, "profile store disabled")
	assertContains(t, stderr, "mapping store disabled")
	assertContains(t, stderr, "Temporal client disabled")
}
