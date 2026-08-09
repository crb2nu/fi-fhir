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

	"github.com/lib/pq"
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

// newCompatSchema creates an isolated PostgreSQL schema and returns a DSN whose
// search_path points at it, so each proof migrates a genuinely fresh ledger set
// without a dedicated database per test.
func newCompatSchema(t *testing.T, dsn, prefix string) string {
	t.Helper()
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres to create compat schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatalf("create compat schema %s: %v", schema, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close compat schema bootstrap connection: %v", err)
	}

	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.ExecContext(context.Background(), `DROP SCHEMA `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})

	return schema
}

// compatSchemaDSN rewrites a URL-form DSN into keyword form so a search_path
// can be appended, matching internal/integration/processor's helper.
func compatSchemaDSN(t *testing.T, dsn, schema string) string {
	t.Helper()
	connectionString := dsn
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			t.Fatalf("parse PostgreSQL test URL: %v", err)
		}
		connectionString = parsed
	}
	return connectionString + " search_path=" + schema
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
