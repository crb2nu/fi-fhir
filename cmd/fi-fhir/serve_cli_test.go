package main

import (
	"strings"
	"testing"
)

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
