package migrationcompat

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

// addColumnClause matches `ADD COLUMN [IF NOT EXISTS] <name> <rest of clause>`
// and captures the name and the clause body separately.
//
// This is the form AGENTS.md's rule is named after — "a new NOT NULL column on
// an existing table" — and the form the rule did not match until Sprint 5 (found
// defect D4). It produces the identical SQLSTATE 23502 on rollback as
// `ALTER COLUMN … SET NOT NULL`.
//
// The clause body is `(?:[^,;()]|\([^()]*\))*` rather than a list of type names,
// because PostgreSQL types carry parentheses, spaces, brackets and schema
// qualifiers — `NUMERIC(10,2)`, `TIMESTAMP WITH TIME ZONE`, `TEXT[]`,
// `public.my_enum` — and a list that misses one silently admits a violation.
// Allowing a parenthesised group means a comma inside `NUMERIC(10,2)` does not
// end the clause while a comma between two ADD COLUMNs still does.
//
// Known limit: a comma inside a string literal (`DEFAULT 'a,b'`) ends the clause
// early. Splitting SQL properly needs a parser, and this file deliberately is
// not one; the case does not occur in this repository's 18 migrations.
var addColumnClause = regexp.MustCompile(
	`(?is)ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)((?:[^,;()]|\([^()]*\))*)`,
)

// notNullPhrase and defaultPhrase read one ADD COLUMN clause body. Two
// independent tests rather than one combined pattern, because PostgreSQL accepts
// the modifiers in either order: `TEXT NOT NULL DEFAULT 'x'` and
// `TEXT DEFAULT 'x' NOT NULL` are the same column, and a single regexp that
// assumes one ordering flags a correctly-written migration as a violation.
var (
	notNullPhrase = regexp.MustCompile(`(?is)\bNOT\s+NULL\b`)
	defaultPhrase = regexp.MustCompile(`(?i)\bDEFAULT\b`)
)

// sqlLineComment matches a `--` comment to the end of its line.
//
// Comments are stripped before any pattern runs. Migration files in this
// repository carry long prose rationales, and prose about a rule is where the
// rule's own keywords are most likely to appear — `-- an ADD COLUMN … NOT NULL
// here would break rollback` must not be read as a violation.
var sqlLineComment = regexp.MustCompile(`--[^\n]*`)

// setDefault matches `ALTER COLUMN <name> SET DEFAULT`.
var setDefault = regexp.MustCompile(`(?i)ALTER\s+COLUMN\s+([a-z_][a-z0-9_]*)\s+SET\s+DEFAULT`)

// inlineNotNullDefault matches a column declared NOT NULL with an inline
// DEFAULT inside a CREATE TABLE body. Such a column is rollback-safe by
// construction. ADD COLUMN clauses are handled by addColumnClause instead.
var inlineNotNullDefault = regexp.MustCompile(
	`(?im)^\s*([a-z_][a-z0-9_]*)\s+[A-Za-z][A-Za-z0-9_ ()]*\s+NOT\s+NULL\s+DEFAULT`,
)

// knownRollbackUnsafeColumns are the columns that already violated the rule
// when it was written, with the reason each is not fixed here.
//
// A baseline rather than an exemption: every entry is dated, reasoned, and
// meant to shrink. An empty allowlist would be better; a rule that silently
// tolerates violations would be worse than either.
var knownRollbackUnsafeColumns = map[string]string{
	// Re-baselined 2026-08-09 (Sprint 5, lane S5-0 MR 0c). The previous text was
	// stale in both of its load-bearing facts: it said the processor ledger head
	// was 4 when it is 5 (processor/postgres_submission.go SchemaVersion), and it
	// deferred the repair to "processor 0005, owned by Lane S4-B" — a number that
	// shipped in Sprint 4 as 0005_retention_expiry.sql without the repair, and a
	// lane that no longer exists. A baseline that names a future that already
	// happened is not policing anything.
	//
	// The columns: both are made NOT NULL by
	// processor/migrations/0002_delivery_reliability.sql:13,32 after a backfill,
	// and neither carries a DEFAULT in any processor migration through 0005.
	//
	// Why they stay exempt rather than being repaired here: the rule's window is
	// one version. The processor ledger head is 5, so an N-1 binary expects
	// ledger 4 and has had both columns since ledger 2. Nothing inside the window
	// can hit SQLSTATE 23502 on them. Repairing them needs processor 0006, and
	// processor 0006 belongs to Lane S5-F this sprint (.loom/33 file-ownership
	// map) — this MR is a test-only repair and must not consume a migration
	// number two other lanes are rebasing against.
	//
	// Re-check when the processor ledger reaches head 3 relative to these
	// columns' introduction — which it already has. The correct end state is
	// deleting these entries after S5-F's 0006 adds the DEFAULTs, or recording a
	// decision that they never will.
	"0002_delivery_reliability.sql:scheduled_at": "processor ledger 2, head 5; outside the one-version window. Repair needs processor 0006, which is Lane S5-F's number this sprint (re-baselined 2026-08-09)",
	"0002_delivery_reliability.sql:updated_at":   "processor ledger 2, head 5; outside the one-version window. Repair needs processor 0006, which is Lane S5-F's number this sprint (re-baselined 2026-08-09)",
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

	for _, column := range rollbackUnsafeColumns(t, dir, name, body, root) {
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

// rollbackUnsafeColumns returns, in a stable order, every column this migration
// makes NOT NULL without a DEFAULT anywhere in its ledger and without a dated
// entry in knownRollbackUnsafeColumns.
//
// It is separated from the assertion so a test can ask "would this be flagged?"
// without the answer being a t.Fatalf on the caller. That is what
// TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged needs, and a rule
// whose enforcement cannot itself be tested is how D4 survived a whole sprint.
func rollbackUnsafeColumns(t *testing.T, dir, name, body, root string) []string {
	t.Helper()

	tightened := tightenedColumns(body)
	if len(tightened) == 0 {
		return nil
	}

	// A DEFAULT may arrive later in the same ledger — as slice 4.4a's session
	// 0007 does for 0004's three columns — so search every file in the
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
		for column := range defaultedColumns(string(peer)) {
			defaulted[column] = struct{}{}
		}
	}

	unsafe := make([]string, 0, len(tightened))
	for _, column := range tightened {
		if _, ok := defaulted[column]; ok {
			continue
		}
		key := name + ":" + column
		if reason, ok := knownRollbackUnsafeColumns[key]; ok {
			t.Logf("known rollback-unsafe column %s — %s", key, reason)
			continue
		}
		unsafe = append(unsafe, column)
	}
	return unsafe
}

// tightenedColumns returns every column this migration body constrains to
// NOT NULL, sorted, deduplicated, and lowercased.
//
// Both forms count. `ALTER COLUMN … SET NOT NULL` tightens a column that
// already holds rows; `ADD COLUMN … NOT NULL` introduces one that the N-1
// binary's INSERT does not name. They produce the identical SQLSTATE 23502 on
// rollback, and the rule's own title in AGENTS.md — "a new NOT NULL column on
// an existing table" — names the second form, not the first.
func tightenedColumns(body string) []string {
	body = sqlLineComment.ReplaceAllString(body, "")

	seen := map[string]struct{}{}
	for _, match := range setNotNull.FindAllStringSubmatch(body, -1) {
		seen[strings.ToLower(match[1])] = struct{}{}
	}
	for _, match := range addColumnClause.FindAllStringSubmatch(body, -1) {
		if notNullPhrase.MatchString(match[2]) {
			seen[strings.ToLower(match[1])] = struct{}{}
		}
	}
	columns := make([]string, 0, len(seen))
	for column := range seen {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// defaultedColumns returns every column this migration body gives a server-side
// DEFAULT, in any of the three forms the repository uses: `ALTER COLUMN … SET
// DEFAULT`, an inline DEFAULT in a CREATE TABLE body, and an inline DEFAULT on
// an ADD COLUMN clause.
//
// The third form is the one D4's repair made load-bearing. Until `ADD COLUMN …
// NOT NULL` was flagged at all, whether its DEFAULT was recognised did not
// matter; now a correctly-written migration depends on it.
func defaultedColumns(body string) map[string]struct{} {
	body = sqlLineComment.ReplaceAllString(body, "")

	defaulted := map[string]struct{}{}
	for _, match := range setDefault.FindAllStringSubmatch(body, -1) {
		defaulted[strings.ToLower(match[1])] = struct{}{}
	}
	for _, match := range inlineNotNullDefault.FindAllStringSubmatch(body, -1) {
		defaulted[strings.ToLower(match[1])] = struct{}{}
	}
	for _, match := range addColumnClause.FindAllStringSubmatch(body, -1) {
		if defaultPhrase.MatchString(match[2]) {
			defaulted[strings.ToLower(match[1])] = struct{}{}
		}
	}
	return defaulted
}

// TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged is Sprint 5 lane
// S5-0's day-1 gate for found defect D4: the rule
// TestMigrationRule_NotNullOnExistingColumnCarriesADefault claims to enforce is
// not the rule it enforces.
//
// AGENTS.md states the rule as "a new NOT NULL column on an existing table
// carries a DEFAULT". The checker matched only `ALTER COLUMN … SET NOT NULL`.
// `ALTER TABLE t ADD COLUMN c TEXT NOT NULL;` — the form the rule's own title
// names — produced no tightened columns at all, so the early return in
// rollbackUnsafeColumns fired and the file was never inspected. It is the same
// SQLSTATE 23502 rollback failure, admitted silently.
//
// The cases below are three gates and six controls. The controls matter as much
// as the gates: they prove the harness can tell flagged from unflagged, so a red
// "add column" case is the rule's hole rather than a broken fixture. Three of
// them — modifier ordering, a comma inside a type, and a commented-out violation
// — are the ways a naive ADD COLUMN pattern gets this wrong in the other
// direction, by rejecting migrations that are correct.
func TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		// peer is a second migration in the same synthetic ledger, present only
		// for the case that proves a DEFAULT may arrive in a later file.
		peer        string
		wantFlagged bool
		why         string
	}{
		{
			name:        "add column not null without a default",
			sql:         "ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT NOT NULL;",
			wantFlagged: true,
			why: "the N-1 binary's INSERT does not name owner_id and there is no server-side " +
				"DEFAULT, so every insert from a rolled-back replica dies on SQLSTATE 23502 — " +
				"identical to the ALTER COLUMN form the rule already catches",
		},
		{
			name:        "add column if not exists, not null, without a default",
			sql:         "ALTER TABLE integration_widgets ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL;",
			wantFlagged: true,
			why:         "IF NOT EXISTS changes idempotency, not rollback safety",
		},
		{
			name:        "add column not null with an inline default",
			sql:         "ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'unattributed';",
			wantFlagged: false,
			why:         "the DEFAULT is what makes the N-1 row visibly incomplete rather than impossible",
		},
		{
			name:        "alter column set not null without a default",
			sql:         "ALTER TABLE integration_widgets ALTER COLUMN owner_id SET NOT NULL;",
			wantFlagged: true,
			why:         "the form the rule already caught; a control that this harness reports flagging at all",
		},
		{
			name:        "add a nullable column",
			sql:         "ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT;",
			wantFlagged: false,
			why:         "a nullable column is rollback-safe; flagging it would make the rule unusable",
		},
		{
			name:        "add column with the default written before the not null",
			sql:         "ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT DEFAULT 'unattributed' NOT NULL;",
			wantFlagged: false,
			why: "PostgreSQL accepts the modifiers in either order and this is the same " +
				"column as the case above; a checker that assumes one ordering rejects " +
				"correct migrations, which is how a rule gets deleted",
		},
		{
			name: "add column whose type carries a comma",
			sql: "ALTER TABLE integration_widgets\n" +
				"    ADD COLUMN amount NUMERIC(10,2) NOT NULL;",
			wantFlagged: true,
			why: "NUMERIC(10,2) must not end the clause at its own comma — a clause that " +
				"stops early loses the NOT NULL and admits the violation silently",
		},
		{
			name: "a commented-out violation",
			sql: "-- ADD COLUMN owner_id TEXT NOT NULL would break one-version rollback,\n" +
				"-- so this migration does not do it.\n" +
				"ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT;",
			wantFlagged: false,
			why: "migration files here carry long prose rationales, and prose about the " +
				"rule is exactly where the rule's keywords appear",
		},
		{
			name: "a later migration in the same ledger supplies the default",
			sql:  "ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT NOT NULL;",
			peer: "ALTER TABLE integration_widgets\n" +
				"    ALTER COLUMN owner_id SET DEFAULT 'unattributed_legacy_export';",
			wantFlagged: false,
			why: "the ledger-wide search is what lets a repair land in a later file, " +
				"which is what session 0007 does for 0004's three columns",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			dir := "migrations"
			name := "0001_synthetic.sql"
			if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
				t.Fatalf("create synthetic ledger: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, dir, name), []byte(testCase.sql), 0o600); err != nil {
				t.Fatalf("write synthetic migration: %v", err)
			}
			if testCase.peer != "" {
				peer := filepath.Join(root, dir, "0002_synthetic_repair.sql")
				if err := os.WriteFile(peer, []byte(testCase.peer), 0o600); err != nil {
					t.Fatalf("write synthetic peer migration: %v", err)
				}
			}

			flagged := rollbackUnsafeColumns(t, dir, name, testCase.sql, root)

			switch {
			case testCase.wantFlagged && len(flagged) == 0:
				t.Fatalf("the migration rule did NOT flag a rollback-unsafe column.\n\n"+
					"  SQL:      %s\n"+
					"  Why it is unsafe: %s\n\n"+
					"  tightenedColumns() returned nothing for this statement, so\n"+
					"  rollbackUnsafeColumns returns early and the file is never inspected.\n"+
					"  That is found defect D4: AGENTS.md \"Migration authoring\" rule 1 names\n"+
					"  this exact form and the checker does not match it.\n\n"+
					"  Fix: add an ADD COLUMN … NOT NULL pattern to tightenedColumns.\n"+
					"  Origin: .loom/33 Found Defects D4; .loom/32 correction 23 reappearing\n"+
					"  with the guard installed backwards.",
					testCase.sql, testCase.why)
			case !testCase.wantFlagged && len(flagged) != 0:
				t.Fatalf("the migration rule flagged a rollback-SAFE column %v.\n\n"+
					"  SQL:  %s\n"+
					"  Why it is safe: %s\n\n"+
					"  A rule that cries wolf is a rule the next lane deletes.",
					flagged, testCase.sql, testCase.why)
			}
		})
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
