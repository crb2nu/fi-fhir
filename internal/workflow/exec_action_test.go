package workflow

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecAction_RunsAllowedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture not supported on windows")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "echo.sh")

	script := `#!/bin/sh
read input
echo "ok:$input"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	//nolint:gosec // G302: test fixture must be executable
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		t.Fatalf("chmod script: %v", err)
	}

	cfg := map[string]string{
		"command":        scriptPath,
		"allowlist":      scriptPath,
		"stdin":          "template",
		"stdin_template": `{"hello":"world"}`,
		"timeout":        "2s",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := execAction(ctx, map[string]any{"x": "y"}, cfg); err != nil {
		t.Fatalf("execAction: %v", err)
	}
}

func TestExecAction_BlocksNonAllowlistedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture not supported on windows")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "noop.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	//nolint:gosec // G302: test fixture must be executable
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		t.Fatalf("chmod script: %v", err)
	}

	cfg := map[string]string{
		"command":   scriptPath,
		"allowlist": filepath.Join(dir, "other.sh"),
	}
	err := execAction(context.Background(), map[string]any{}, cfg)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "not in allowlist") {
		t.Fatalf("unexpected error: %v", err)
	}
}
