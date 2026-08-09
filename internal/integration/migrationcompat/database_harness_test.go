//go:build integration

package migrationcompat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this source file to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this source file")
	}
	dir := filepath.Dir(file)
	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("cannot locate the module root from the test source path")
	return ""
}

// runRoundTripScript executes the operator-facing dump/restore runbook step.
//
// The proof deliberately shells out to the same script docs/operations names
// rather than reimplementing pg_dump in Go: a restore proof that hand-rolls its
// own copy proves something about the test, not about the runbook. The script
// also refuses a client/server major-version mismatch, so a CI image whose
// default client is the wrong major fails here loudly instead of silently
// proving nothing.
func runRoundTripScript(t *testing.T, sourceDSN, targetDB string) {
	t.Helper()
	binDir := os.Getenv("FI_FHIR_PG_BIN_DIR")
	pgDump := "pg_dump"
	if binDir != "" {
		pgDump = filepath.Join(binDir, "pg_dump")
	}
	if _, err := exec.LookPath(pgDump); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("pg_dump is required in CI (the job installs a client matching the server major): %v", err)
		}
		t.Skipf("pg_dump is required for the restore round-trip proof: %v", err)
	}

	script := filepath.Join(repoRoot(t), "scripts", "pgdump-roundtrip.sh")
	cmd := exec.CommandContext(t.Context(), "bash", script,
		"--source-url", sourceDSN, "--target-db", targetDB)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scripts/pgdump-roundtrip.sh failed: %v\n%s", err, output)
	}
	t.Logf("restore round-trip:\n%s", strings.TrimSpace(string(output)))
}
