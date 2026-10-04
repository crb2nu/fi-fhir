//go:build integration

package session

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestPostgresSessionSummaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db := openSessionPostgres(t, ctx)
	store, err := NewPostgresStore(db, PostgresConfig{TenantID: "summary-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sessions, runs := summaryFixtures()
	seedPostgresSummaries(t, ctx, db, "summary-tenant", sessions, runs)
	otherSession := sessions[2]
	otherSession.Name = "Other tenant private name"
	seedPostgresSummaries(t, ctx, db, "other-tenant", []Session{otherSession}, []Run{{
		ID: "run-z", SessionID: otherSession.ID, Status: RunStatusFailed, CreatedAt: time.Now().UTC(),
	}})
	// A child that cannot be decoded must not affect a metadata-only read.
	if _, err := db.ExecContext(ctx, `INSERT INTO integration_session_samples
		(tenant_id, session_id, sample_id, created_at, record_json, raw_cipher)
		VALUES ('summary-tenant', 'session-old', 'sample-unreadable', now(), '{"id":42}'::jsonb, '\x01'::bytea)`); err != nil {
		t.Fatal(err)
	}
	assertSummaryBrowse(t, store)
	other, err := NewPostgresStore(db, PostgresConfig{TenantID: "other-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := other.ListSessionSummaries(ctx, SessionSummaryOptions{Limit: 25})
	if err != nil || len(page.Nodes) != 1 || page.Nodes[0].Name != otherSession.Name || page.Nodes[0].LatestRun == nil || page.Nodes[0].LatestRun.ID != "run-z" || page.Nodes[0].LatestRun.Status != RunStatusFailed {
		t.Fatalf("other tenant summary = %#v, %v", page, err)
	}

	// Search is evaluated before the offset cap, so a matching older session
	// remains findable even when it lies beyond the browse window.
	if _, err := db.ExecContext(ctx, `INSERT INTO integration_sessions
		(tenant_id, session_id, status, created_at, record_json)
		SELECT 'cap-tenant', 'session-' || lpad(n::text, 5, '0'), 'active', '2026-10-03T00:00:00Z',
		       jsonb_build_object('name', 'session-' || lpad(n::text, 5, '0'), 'updated_at', '2026-10-03T00:00:00Z')
		FROM generate_series(0, 10025) n`); err != nil {
		t.Fatal(err)
	}
	capped, err := NewPostgresStore(db, PostgresConfig{TenantID: "cap-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	page, err = capped.ListSessionSummaries(ctx, SessionSummaryOptions{Limit: 25, Offset: 10000})
	if err != nil || len(page.Nodes) != 25 || !page.HasMore || page.NextOffset != nil {
		t.Fatalf("browse cap page = %#v, %v", page, err)
	}
	page, err = capped.ListSessionSummaries(ctx, SessionSummaryOptions{Limit: 25, Search: "session-00000"})
	if err != nil || len(page.Nodes) != 1 || page.Nodes[0].ID != "session-00000" {
		t.Fatalf("search beyond browse cap = %#v, %v", page, err)
	}
}

func seedPostgresSummaries(t *testing.T, ctx context.Context, db *sql.DB, tenant string, sessions []Session, runs []Run) {
	t.Helper()
	for _, record := range sessions {
		raw, err := encodeRecord(record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
			VALUES ($1, $2, $3, $4, $5)`, tenant, record.ID, record.Status, record.CreatedAt, raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, run := range runs {
		// Deliberately undecodable payload: summaries need only the indexed
		// identity/status/created_at columns, never the full run document.
		if _, err := db.ExecContext(ctx, `INSERT INTO integration_session_runs (tenant_id, session_id, run_id, status, created_at, record_json)
			VALUES ($1, $2, $3, $4, $5, '{"id":42}'::jsonb)`, tenant, run.SessionID, run.ID, run.Status, run.CreatedAt); err != nil {
			t.Fatal(err)
		}
	}
}
