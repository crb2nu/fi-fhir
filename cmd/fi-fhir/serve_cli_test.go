package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// signalTestMu serialises tests that send process-wide signals (SIGINT/SIGTERM)
// so the race detector does not flag concurrent signal.Notify registrations.
var signalTestMu sync.Mutex

func TestServe_DryRun_PrintsConfig(t *testing.T) {
	stdout, _, err := runCLI(t,
		"serve",
		"--host", "127.0.0.1",
		"--port", "9999",
		"--path", "/gql",
		"--playground-path", "/play",
		"--no-playground",
		"--no-introspection",
		"--max-depth", "15",
		"--max-complexity", "1234",
		"--timeout", "45s",
		"--dry-run",
	)
	assertNoError(t, err)

	// Minimal assertions; JSON formatting/ordering can evolve without breaking intent.
	assertContains(t, stdout, `"host": "127.0.0.1"`)
	assertContains(t, stdout, `"port": 9999`)
	assertContains(t, stdout, `"path": "/gql"`)
	assertContains(t, stdout, `"playground_path": "/play"`)
	assertContains(t, stdout, `"playground_enabled": false`)
	assertContains(t, stdout, `"introspection": false`)
	assertContains(t, stdout, `"websocket_path": "/gql/ws"`)
	assertContains(t, stdout, `"max_depth": 15`)
	assertContains(t, stdout, `"max_complexity": 1234`)
	assertContains(t, stdout, `"timeout": "45s"`)

	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("expected JSON object output, got: %s", stdout)
	}
}

func TestServe_DryRun_WithWorkflow_PrintsWorkflowInfo(t *testing.T) {
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "workflow.yaml")
	workflowYAML := `workflow:
  name: test
  version: "1.0"
  routes: []
`
	if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	stdout, stderr, err := runCLI(t,
		"serve",
		"--workflow", workflowPath,
		"--dry-run",
	)
	assertNoError(t, err)
	assertContains(t, stderr, "Workflow validation warnings")

	assertContains(t, stdout, `"workflow": {`)
	assertContains(t, stdout, `"path": "`+workflowPath+`"`)
	assertContains(t, stdout, `"name": "test"`)
	assertContains(t, stdout, `"routes": 0`)
}

func TestServe_DryRun_InvalidWorkflow_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "workflow.yaml")
	if err := os.WriteFile(workflowPath, []byte("not yaml"), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	_, _, err := runCLI(t,
		"serve",
		"--workflow", workflowPath,
		"--dry-run",
	)
	assertError(t, err)
	assertErrorContains(t, err, "failed to load workflow")
}

func TestRunServe_Execution_GracefulShutdown(t *testing.T) {
	if raceEnabled {
		t.Skip("Skipping: process-level SIGINT is inherently racy under -race detector")
	}
	signalTestMu.Lock()
	defer signalTestMu.Unlock()

	// Temporarily disable stores via env vars to avoid DB connection delays/errors
	configurePreviewRuntimeForTest(t)
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_CORS_ORIGINS", "")

	// Start a goroutine to send SIGINT and gracefully shut down the server
	go func() {
		time.Sleep(500 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT)
	}()

	// Run on ephemeral port to avoid conflict
	err := runServe([]string{"--port", "0"})
	assertNoError(t, err)
}

func TestRunServe_Execution_InvalidHost(t *testing.T) {
	signalTestMu.Lock()
	defer signalTestMu.Unlock()
	configurePreviewRuntimeForTest(t)

	// An invalid host should trigger an immediate error from the server
	err := runServe([]string{"--host", "invalid-host-name-123456", "--port", "0"})
	assertError(t, err)
}
