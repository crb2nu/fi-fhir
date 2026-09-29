//go:build integration

package authoring

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// authoringDB opens a fresh schema in POSTGRES_TEST_URL for one proof.
func authoringDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for definition authoring integration tests")
	}
	schema := fmt.Sprintf("definition_authoring_%d", time.Now().UnixNano())
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(context.Background(), `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()
	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.ExecContext(context.Background(), `DROP SCHEMA `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
	connectionString := dsn
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
		connectionString = parsed
	}
	db, err := sql.Open("postgres", connectionString+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func testCommand(revision integration.IntegrationDefinitionRevision, version int64, reason string) lifecycle.Command {
	return lifecycle.Command{
		TenantID: revision.TenantID, DefinitionID: revision.DefinitionID, RevisionID: revision.RevisionID,
		ExpectedVersion: version, Principal: testPrincipal(), Reason: reason,
	}
}
