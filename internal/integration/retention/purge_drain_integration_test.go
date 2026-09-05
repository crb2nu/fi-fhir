//go:build integration

package retention

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// drainProofBacklog is the acceptance criterion's number: "a tenant with 10,000
// expired canonical events reaches zero backlog within a bounded, documented
// number of purge ticks — asserted, not reasoned about"
// (.loom/33-sprint5-execution-specs.md, Lane S5-F).
//
// It is 50 batches at the shipped defaultBatchSize, so a per-tick ceiling of one
// batch would need 50 hourly ticks — two days — to clear it. That is the
// condition D1 describes.
const drainProofBacklog = 10_000

// documentedTickBound is the bound the acceptance criterion asks to be
// documented and asserted: ONE tick, provided the tick's wall-clock drain
// budget is not exhausted. It is stated in docs/operations/PHI-RETENTION.md
// alongside the arithmetic that produces it.
const documentedTickBound = 1

// TestPurgeThroughput_TenThousandRecordBacklogDrainsWithinTheDocumentedTickBound
// is Lane S5-F's primary kill-test for the D1 repair.
//
// The day-1 gate (purge_throughput_gate_integration_test.go) reproduced the
// defect at 500 records and one batch. This asserts the repaired property at
// the acceptance criterion's scale, and it asserts the three things a drain
// loop can get wrong that a smaller fixture would hide: that the tick actually
// drains rather than merely purging more, that the backlog gauge tracks the
// drain down to zero, and that one failing class cannot stop the others.
//
// NEGATIVE CONTROL. `make phi-retention-throughput-negative-control` rebuilds
// this file with `-tags retentionnodrain`, which restores the single-pass loop
// (purger_nodrain.go), and requires it to FAIL. A control that passes means the
// assertions below are not on the mechanism.
func TestPurgeThroughput_TenThousandRecordBacklogDrainsWithinTheDocumentedTickBound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db := newRetentionGateSchema(t, ctx, requireRetentionPostgres(t), "purge_drain_proof")
	migrateDurableSchema(t, ctx, db)
	seedExpiredCanonicalEventBacklog(t, ctx, db, drainProofBacklog)

	store := newRetentionStore(t, db)
	if _, err := store.PutPolicy(ctx, throughputPolicy()); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	purger, err := NewPurger(PurgerConfig{Store: store, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	if purger.BatchSize() != defaultBatchSize {
		t.Fatalf("purger batch size = %d, want the shipped %d", purger.BatchSize(), defaultBatchSize)
	}

	t.Run("the_gauge_is_non_zero_before_the_purge_runs", func(t *testing.T) {
		backlog, err := store.Backlog(ctx)
		if err != nil {
			t.Fatalf("Backlog: %v", err)
		}
		if backlog.CanonicalEvents != drainProofBacklog {
			t.Fatalf("backlog gauge = %d before the first purge, want %d. The gauge counts what "+
				"the purge is eligible to act on, recomputed from recorded_at and the policy "+
				"window, so an unstamped row must still count — that is the half of D1 the "+
				"counters could not show", backlog.CanonicalEvents, drainProofBacklog)
		}
	})

	var ticks int
	t.Run("the_backlog_reaches_zero_within_the_documented_tick_bound", func(t *testing.T) {
		var purged int64
		var passes int
		for ticks = 1; ticks <= documentedTickBound; ticks++ {
			result, err := purger.PurgeOnce(ctx)
			if err != nil {
				t.Fatalf("PurgeOnce tick %d: %v", ticks, err)
			}
			purged += result.CanonicalEvents
			passes += result.Passes
			if result.Backlog.Total() == 0 {
				t.Logf("backlog drained in %d tick(s), %d store passes, %d records tombstoned",
					ticks, passes, purged)
				return
			}
			if result.BudgetExhausted {
				t.Fatalf("tick %d spent its whole %s drain budget with %d records still eligible; "+
					"either the budget is too small for the documented bound or the drain is "+
					"slower than the fixture assumes", ticks, purger.DrainBudget(),
					result.Backlog.Total())
			}
		}
		remaining := countPurgeableCanonicalEvents(t, ctx, db)
		t.Fatalf("after %d tick(s) the backlog is %d, not zero: %d records tombstoned across %d "+
			"store passes. At one batch per tick this would take %d hourly ticks, which is D1",
			documentedTickBound, remaining, purged, passes,
			(drainProofBacklog+defaultBatchSize-1)/defaultBatchSize)
	})

	t.Run("and_the_gauge_returns_to_zero_with_it", func(t *testing.T) {
		backlog, err := store.Backlog(ctx)
		if err != nil {
			t.Fatalf("Backlog: %v", err)
		}
		if backlog.Total() != 0 {
			t.Fatalf("backlog gauge = %+v after the drain, want zero across every class", backlog)
		}
		if tombstoned := countTombstonedCanonicalEvents(t, ctx, db); tombstoned != drainProofBacklog {
			t.Fatalf("%d records tombstoned, want %d", tombstoned, drainProofBacklog)
		}
	})

	// Every tombstone still carries its audit row, and the exemption is still
	// exactly as narrow as Slice 4.1e left it. A drain loop that purged 10,000
	// records by widening the guard would be a regression, not a repair.
	t.Run("every_tombstone_is_still_audited_and_the_guard_is_still_narrow", func(t *testing.T) {
		var audited int64
		if err := db.QueryRowContext(ctx, `
			SELECT count(*) FROM integration_retention_purge_audit
			WHERE record_class = 'canonical_event' AND purge_mode = 'tombstone'
		`).Scan(&audited); err != nil {
			t.Fatalf("count purge audit rows: %v", err)
		}
		if audited != drainProofBacklog {
			t.Fatalf("%d audit rows for %d tombstones; the audit is written by the same statement "+
				"as the tombstone, so a mismatch means the drain found another write path",
				audited, drainProofBacklog)
		}
		_, err := db.ExecContext(ctx,
			`UPDATE integration_canonical_events SET classification = 'phi', event_type = 'rewritten'
			 WHERE event_id = 'evt-throughput-000001'`)
		assertRaised(t, err, "append-only",
			"a non-tombstone mutation succeeded after the drain; the D1 repair widened the exemption")
	})
}

// TestPurgeThroughput_OnePoisonedClassDoesNotStopTheOthers is the S3 repair:
// PurgeExpired returned on the first class's error, so one broken class stopped
// every remaining class for that pass. On an hourly cadence that is an hour of
// retention not enforced for classes that were perfectly healthy, and it
// compounds D1.
//
// The poison is a revoked privilege rather than a fake error, so the failure
// arrives through the same path a real one would: the store issues its real
// statement and PostgreSQL refuses it.
func TestPurgeThroughput_OnePoisonedClassDoesNotStopTheOthers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db := newRetentionGateSchema(t, ctx, requireRetentionPostgres(t), "purge_poison_proof")
	migrateDurableSchema(t, ctx, db)
	seedExpiredCanonicalEventBacklog(t, ctx, db, 10)
	seedExpiredSessionSamples(t, ctx, db, 10)

	store := newRetentionStore(t, db)
	policy := throughputPolicy()
	policy.SessionSampleRetain = time.Hour
	if _, err := store.PutPolicy(ctx, policy); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}

	// Poison the canonical-event class. The session-sample class is untouched
	// and must still drain in the same pass.
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE integration_canonical_events RENAME TO integration_canonical_events_hidden`,
	); err != nil {
		t.Fatalf("poison the canonical event class: %v", err)
	}

	counts, err := store.PurgeExpired(ctx, defaultBatchSize)
	if err == nil {
		t.Fatal("the poisoned class did not fail, so the isolation assertion would be vacuous")
	}
	if !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("the pass failed with %v, which does not name the poisoned class", err)
	}
	if counts.SessionSamples != 10 {
		t.Fatalf("the healthy class purged %d of 10 records while another class was failing. "+
			"Before Sprint 5 PurgeExpired returned on the first class's error, so every "+
			"remaining class was skipped for the whole pass (S3)", counts.SessionSamples)
	}
	// And a failing pass must not report saturation: draining against a broken
	// class would spin the loop for the whole budget instead of letting the next
	// tick retry.
	if counts.Saturated {
		t.Fatal("a pass that failed reported Saturated, so the drain loop would spin on the error")
	}
	t.Logf("one class failed (%v) and the healthy class still purged all 10 of its records", err)
}

// seedExpiredSessionSamples writes count session samples backdated past every
// window the proofs use. integration_session_samples carries no immutability
// trigger, so direct SQL is the honest way to place rows the purge will delete.
func seedExpiredSessionSamples(t *testing.T, ctx context.Context, db *sql.DB, count int) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
		VALUES ('tenant-a', 'sess-drain-proof', 'active', now() - interval '30 days',
			'{"id":"sess-drain-proof","name":"drain proof","status":"active"}'::jsonb)
	`); err != nil {
		t.Fatalf("seed drain-proof session: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_session_samples (
			tenant_id, session_id, sample_id, created_at, record_json, raw_cipher
		)
		SELECT 'tenant-a', 'sess-drain-proof', 'sample-drain-'||lpad(g::text, 6, '0'),
			now() - interval '30 days',
			jsonb_build_object('id', 'sample-drain-'||g, 'name', 'admit-'||g),
			decode('deadbeef', 'hex')
		FROM generate_series(1, $1) AS g
	`, count); err != nil {
		t.Fatalf("seed %d expired session samples: %v", count, err)
	}
}
