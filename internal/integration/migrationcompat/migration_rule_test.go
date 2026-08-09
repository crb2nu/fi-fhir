package migrationcompat

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// migrationDirs is every forward-only migration directory in the repository.
// A new one has to be added here, which is the point: a ledger nobody
// registered is a ledger nobody checks.
var migrationDirs = []string{
	"internal/integration/processor/migrations",
	"internal/integration/session/migrations",
	"internal/integration/lifecycle/migrations",
	"internal/integration/batch/migrations",
	"internal/integration/destination/migrations",
}

// setNotNull matches `ALTER COLUMN <name> SET NOT NULL`, which is how a
// migration tightens an existing column.
var setNotNull = regexp.MustCompile(`(?i)ALTER\s+COLUMN\s+([a-z_][a-z0-9_]*)\s+SET\s+NOT\s+NULL`)

// setDefault matches `ALTER COLUMN <name> SET DEFAULT`.
var setDefault = regexp.MustCompile(`(?i)ALTER\s+COLUMN\s+([a-z_][a-z0-9_]*)\s+SET\s+DEFAULT`)

// knownRollbackUnsafeColumns are the columns that already violated the rule
// when it was written, with the reason each is not fixed here.
//
// A baseline rather than an exemption: every entry is dated, reasoned, and
// meant to shrink. An empty allowlist would be better; a rule that silently
// tolerates violations would be worse than either.
var knownRollbackUnsafeColumns = map[string]string{
	// Introduced at processor ledger version 2. The processor ledger head is 4,
	// so a binary one version back (ledger 3) already has these columns and the
	// rule's one-version window is not violated today. Repairing them anyway
	// needs a new processor migration, and processor 0005 is Lane S4-B's number
	// this sprint (.loom/32 file-ownership map). Filed for 4.4c.
	"0002_delivery_reliability.sql:scheduled_at": "processor ledger 2, outside the one-version window; needs processor 0005, owned by Lane S4-B",
	"0002_delivery_reliability.sql:updated_at":   "processor ledger 2, outside the one-version window; needs processor 0005, owned by Lane S4-B",
}

// TestMigrationRule_NotNullOnExistingColumnCarriesADefault mechanically enforces
// slice 4.4a's migration-authoring rule (AGENTS.md, "Migration authoring").
//
// The rule: a migration that makes an existing column NOT NULL must also give
// it a DEFAULT, or it breaks one-version rollback. During a rolling upgrade
// both binaries run against the migrated schema, and a rollback runs the older
// one against it indefinitely. The older binary's INSERT does not name the new
// column, so without a DEFAULT it dies on a not-null violation — which is
// exactly what slice 4.1d C1's export attribution migration did, and what the
// day-1 gate reproduced before this slice fixed it.
//
// This runs as an ordinary unit test with no database, so it fails in
// `test:unit` on the merge request that introduces the violation rather than in
// a customer's rollback. Writing the rule down in AGENTS.md was the assignment;
// making it mechanical is what stops it from being ignored.
func TestMigrationRule_NotNullOnExistingColumnCarriesADefault(t *testing.T) {
	root := moduleRoot(t)

	checked := 0
	for _, dir := range migrationDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("read migration directory %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			checked++
			path := filepath.Join(root, dir, entry.Name())
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			assertTightenedColumnsHaveDefaults(t, dir, entry.Name(), string(body), root)
		}
	}

	if checked == 0 {
		t.Fatal("no migration files were checked; the rule would be vacuous")
	}
	t.Logf("checked %d migration files across %d ledgers", checked, len(migrationDirs))
}

// assertTightenedColumnsHaveDefaults reports any column a migration makes
// NOT NULL without a DEFAULT, either in the same file or in a later one for the
// same ledger.
func assertTightenedColumnsHaveDefaults(t *testing.T, dir, name, body, root string) {
	t.Helper()

	tightened := map[string]struct{}{}
	for _, match := range setNotNull.FindAllStringSubmatch(body, -1) {
		tightened[strings.ToLower(match[1])] = struct{}{}
	}
	if len(tightened) == 0 {
		return
	}

	// A DEFAULT may arrive later in the same ledger — as slice 4.4a's session
	// 0006 does for 0004's three columns — so search every file in the
	// directory, not just this one.
	defaulted := map[string]struct{}{}
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		t.Fatalf("re-read migration directory %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		peer, err := os.ReadFile(filepath.Join(root, dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, match := range setDefault.FindAllStringSubmatch(string(peer), -1) {
			defaulted[strings.ToLower(match[1])] = struct{}{}
		}
		// A column created with an inline DEFAULT in the same ledger is fine too.
		for _, match := range regexp.MustCompile(
			`(?im)^\s*([a-z_][a-z0-9_]*)\s+[A-Za-z][A-Za-z0-9_ ()]*\s+NOT\s+NULL\s+DEFAULT`,
		).FindAllStringSubmatch(string(peer), -1) {
			defaulted[strings.ToLower(match[1])] = struct{}{}
		}
	}

	for column := range tightened {
		if _, ok := defaulted[column]; ok {
			continue
		}
		key := name + ":" + column
		if reason, ok := knownRollbackUnsafeColumns[key]; ok {
			t.Logf("known rollback-unsafe column %s — %s", key, reason)
			continue
		}
		t.Fatalf("%s/%s makes column %q NOT NULL with no DEFAULT anywhere in the ledger.\n\n"+
			"  That breaks one-version rollback: during a rolling upgrade and after a\n"+
			"  rollback, a binary one version behind runs against this migrated schema and\n"+
			"  its INSERT does not name %q. It will die on SQLSTATE 23502.\n\n"+
			"  Fix: add a server-side DEFAULT that makes the older binary's row *visibly*\n"+
			"  incomplete rather than impossible — the same idiom\n"+
			"  internal/integration/session/migrations/0007_export_attribution_defaults.sql\n"+
			"  uses for slice 4.1d's three export-attribution columns.\n\n"+
			"  If the column genuinely cannot carry a DEFAULT, add it to\n"+
			"  knownRollbackUnsafeColumns with a dated reason and record the decision in\n"+
			"  .loom/40-decisions.md. Do not delete this test.\n\n"+
			"  Rule: AGENTS.md \"Migration authoring\"; origin: .loom/32 correction 23.",
			dir, name, column, column)
	}
}

// moduleRoot walks up from this source file to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
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
	t.Fatal("cannot locate the module root")
	return ""
}
