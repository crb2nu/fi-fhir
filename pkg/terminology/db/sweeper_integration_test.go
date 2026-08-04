//go:build integration

// Kill-test for the serve-time pending-autoroute expiry sweep.
//
// This lives in the external db_test package on purpose: the sweeper is in
// internal/terminology/autoroute, which transitively imports this package, so an
// in-package test importing it would be an import cycle.
//
// Run with the same CI-compatible DSN path as the rest of the package:
//
//	POSTGRES_TEST_URL=postgres://user:pass@localhost:5432/testdb \
//	    go test -tags=integration -p 1 ./pkg/terminology/db/... -run ExpirySweep
package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// newSweepTestStore builds a migrated store against POSTGRES_TEST_URL.
//
// Unlike the in-package helper this has no testcontainers path: the sweep
// kill-test only needs the DSN route that CI uses, and skipping loudly is more
// honest than a Docker dependency that silently rots.
func newSweepTestStore(t *testing.T) (*termdb.MappingStore, context.Context) {
	t.Helper()

	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_URL not set; skipping pending autoroute sweep integration test")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	ctx := context.Background()
	if _, err := sqlDB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := termdb.NewMigrator(sqlDB).Initialize(ctx); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	return termdb.NewMappingStore(sqlDB), ctx
}

// TestAutorouteExpirySweep_FlipsStoredStatus is the Lane C kill-test.
//
// It runs ONLY the sweep runner and asserts the stored status column changes.
// It deliberately never calls ListPendingAutoroutes: that read already hides
// time-expired rows, so a list-based assertion would pass even with no sweeper
// at all and would prove nothing about the sweep.
func TestAutorouteExpirySweep_FlipsStoredStatus(t *testing.T) {
	store, ctx := newSweepTestStore(t)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	expired := &termdb.PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "SWEEP001", TargetSystem: "http://loinc.org",
		SuggestedCode: "7777-1", Confidence: 0.91, ExpiresAt: &past,
	}
	stillPending := &termdb.PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "SWEEP002", TargetSystem: "http://loinc.org",
		SuggestedCode: "7777-2", Confidence: 0.92, ExpiresAt: &future,
	}
	for _, p := range []*termdb.PendingAutoroute{expired, stillPending} {
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.SourceCode, err)
		}
	}

	// Baseline: the row is stored as pending even though its expiry has passed.
	// Without this the test could pass on a row that was never pending.
	before, err := store.GetPendingAutoroute(ctx, expired.ID)
	if err != nil {
		t.Fatalf("get expired row before sweep: %v", err)
	}
	if before.Status != termdb.StatusPending {
		t.Fatalf("stored status before sweep = %s, want pending", before.Status)
	}

	sweeper, err := autoroute.NewSweeper(autoroute.SweeperConfig{
		Store:    store,
		Interval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSweeper: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sweeper.Run(runCtx) }()

	deadline := time.Now().Add(10 * time.Second)
	var got *termdb.PendingAutoroute
	for time.Now().Before(deadline) {
		got, err = store.GetPendingAutoroute(ctx, expired.ID)
		if err != nil {
			t.Fatalf("get expired row: %v", err)
		}
		if got.Status == termdb.StatusExpired {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil || got.Status != termdb.StatusExpired {
		t.Fatalf("stored status after sweep = %v, want expired (sweep did not reconcile the column)", got.Status)
	}

	// The sweep must not touch rows that have not reached their expiry.
	untouched, err := store.GetPendingAutoroute(ctx, stillPending.ID)
	if err != nil {
		t.Fatalf("get future-expiry row: %v", err)
	}
	if untouched.Status != termdb.StatusPending {
		t.Errorf("future-expiry status = %s, want pending", untouched.Status)
	}

	// Shutdown must be clean: cancellation is not a component failure.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not stop after cancellation")
	}
}
