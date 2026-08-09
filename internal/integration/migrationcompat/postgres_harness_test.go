//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// compatTenantID is the single logical security domain every proof in this
// package runs under. Slice 4.4a asserts nothing about tenant isolation; that
// is 4.1's contract and has its own kill-tests.
const compatTenantID = "tenant-migration-compat"

// requireCompatDSN mirrors the convention every other integration proof in this
// repository uses: skip locally when no database was provisioned, but fail hard
// in CI so a missing service container cannot turn a real regression into a
// green pipeline.
func requireCompatDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for migration compatibility integration tests")
	}
	return dsn
}

// newCompatDatabase provisions an empty database and returns its DSN and name.
//
// Every proof here gets a whole database rather than a `search_path` schema,
// and that is not fastidiousness. Five ledgers create unqualified tables, which
// a search_path does isolate — but `pkg/terminology/db` creates a PostgreSQL
// schema literally named `terminology`, which is database-wide and which
// search_path cannot isolate. Sharing a database across proofs therefore leaves
// the second one running against a terminology ledger the first already
// migrated, so "two replicas against a fresh database" quietly stops testing
// the fresh-install path for the one migrator this slice exists to fix. The
// negative control caught exactly that.
//
// The database is dropped on cleanup even when the test fails, so a red
// pipeline does not leave databases behind for the next run to collide with.
func newCompatDatabase(t *testing.T, dsn, prefix string) (string, string) {
	t.Helper()
	name := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())

	admin, err := sql.Open("postgres", splitDSN(dsn, "postgres"))
	if err != nil {
		t.Fatalf("open maintenance database: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if _, err := admin.ExecContext(context.Background(), `CREATE DATABASE `+name); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", splitDSN(dsn, "postgres"))
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})

	return splitDSN(dsn, name), name
}

// splitDSN returns the DSN with its database path replaced by name.
func splitDSN(dsn, name string) string {
	base, query := dsn, ""
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		base, query = dsn[:idx], dsn[idx:]
	}
	prefix := base[:strings.LastIndex(base, "/")]
	return prefix + "/" + name + query
}

func openCompatDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open compat PostgreSQL: %v", err)
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(12)
	if err := db.PingContext(t.Context()); err != nil {
		_ = db.Close()
		t.Fatalf("ping compat PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedSession writes an integration_sessions row with raw SQL rather than
// through session.PostgresStore.CreateSession. The export proofs assert on a
// column shape, so they must not be coupled to the current Go writer's shape.
func seedSession(t *testing.T, db *sql.DB, sessionID string) {
	t.Helper()
	record := fmt.Sprintf(`{"id":%q,"name":"migration compatibility fixture","status":"active"}`, sessionID)
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
		VALUES ($1, $2, 'active', $3, $4)
	`, compatTenantID, sessionID, time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC), record); err != nil {
		t.Fatalf("seed integration_sessions row: %v", err)
	}
}
