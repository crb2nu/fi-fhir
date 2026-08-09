//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// TestMigrationCompatibility_NegativeControls is the part that proves the
// kill-test can fail.
//
// A green proof is worthless if it would stay green with the mechanism removed.
// Each control here removes exactly one of slice 4.4a's two fixes and asserts
// that the corresponding assertion goes red. A control that passes means the
// proof is not exercising the mechanism, and the lane has shipped a test that
// watches nothing.
//
// Both controls run in the same job as the proof, following the S3-A pattern
// (.gitlab-ci.yml test:observability-replicas), rather than behind a build flag
// that nobody would remember to run.
func TestMigrationCompatibility_NegativeControls(t *testing.T) {
	ctx := t.Context()
	dsn := requireCompatDSN(t)

	t.Run("without the DEFAULTs the rollback insert fails again", func(t *testing.T) {
		controlRollbackDefaults(ctx, t, dsn)
	})

	t.Run("without the advisory lock the terminology migrator does not serialize", func(t *testing.T) {
		controlTerminologyAdvisoryLock(ctx, t, dsn)
	})

	t.Run("the pre-slice unlocked migrator races on a fresh database", func(t *testing.T) {
		controlUnlockedMigratorRaces(ctx, t, dsn)
	})
}

// controlRollbackDefaults removes task 2's mechanism — and only that — from a
// fully migrated schema, then reruns the day-1 gate's insert. It must fail with
// the original not-null violation.
func controlRollbackDefaults(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()
	controlDSN, _ := newCompatDatabase(t, dsn, "migcompat_control_defaults")
	db := openCompatDB(t, controlDSN)

	store, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: compatTenantID})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate session ledger: %v", err)
	}

	// Revert exactly the three DEFAULTs 0007 adds. Nothing else changes: the
	// columns keep their NOT NULL, their CHECKs, and their trigger.
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE integration_session_exports
			ALTER COLUMN principal_json DROP DEFAULT,
			ALTER COLUMN reason DROP DEFAULT,
			ALTER COLUMN include_raw_payload DROP DEFAULT
	`); err != nil {
		t.Fatalf("revert the 0007 defaults: %v", err)
	}

	seedSession(t, db, "sess-control-defaults")
	_, insertErr := db.ExecContext(ctx, `
		INSERT INTO integration_session_exports
			(tenant_id, session_id, export_id, exported_at, record_json)
		VALUES ($1, $2, $3, $4, '{"id":"export-control"}')
	`, compatTenantID, "sess-control-defaults", "export-control",
		time.Date(2026, 8, 8, 9, 5, 0, 0, time.UTC))

	if insertErr == nil {
		t.Fatal("CONTROL PASSED, WHICH MEANS THE PROOF IS BROKEN: the rollback-era insert " +
			"succeeded with the DEFAULTs removed. Something other than 0007's DEFAULTs is " +
			"making TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback " +
			"green, so that test is not watching the mechanism it claims to watch.")
	}
	var pqErr *pq.Error
	if !errors.As(insertErr, &pqErr) || pqErr.Code != notNullViolation {
		t.Fatalf("control failed for the wrong reason: %v (want SQLSTATE 23502)", insertErr)
	}
	t.Logf("control CONFIRMED: with 0007's DEFAULTs removed the rollback insert fails again "+
		"with %s on %q", pqErr.Code, pqErr.Column)
}

// controlTerminologyAdvisoryLock proves the lock is on the migrator's path,
// deterministically. An external session holds the same advisory key; a
// migrator that takes the lock must block until its context expires, and a
// migrator that does not — which is exactly what `pkg/terminology/db` did
// before slice 4.4a — would sail past and return nil.
func controlTerminologyAdvisoryLock(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()
	controlDSN, _ := newCompatDatabase(t, dsn, "migcompat_control_lock")
	db := openCompatDB(t, controlDSN)

	// A second pool so the blocker's transaction cannot be handed the same
	// connection the migrator uses.
	blockerDB := openCompatDB(t, controlDSN)
	blocker, err := blockerDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin blocking transaction: %v", err)
	}
	defer func() { _ = blocker.Rollback() }()

	// The same key pkg/terminology/db.Migrator.Initialize takes. Duplicated
	// here rather than exported: an exported lock key invites a caller to take
	// it, and the only thing a test needs is the number.
	const terminologyLockKey = int64(5064657639792058903)
	if _, err := blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, terminologyLockKey); err != nil {
		t.Fatalf("acquire the terminology migration lock externally: %v", err)
	}

	blocked, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	_, initErr := termdb.NewMigrator(db).Initialize(blocked)
	elapsed := time.Since(start)

	if initErr == nil {
		t.Fatalf("CONTROL PASSED, WHICH MEANS THE PROOF IS BROKEN: Initialize returned nil in %s "+
			"while another session held the terminology migration lock. It is not taking the "+
			"lock, so the concurrency assertion in "+
			"TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore is green "+
			"by luck rather than by mechanism.", elapsed)
	}
	if elapsed < time.Second {
		t.Fatalf("Initialize failed in %s, too fast to have waited on the lock: %v", elapsed, initErr)
	}

	// The terminology schema must not exist: a migrator that blocked on the
	// lock cannot have applied anything.
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology')
	`).Scan(&exists); err != nil {
		t.Fatalf("check terminology schema existence: %v", err)
	}
	if exists {
		t.Fatal("the blocked migrator created the terminology schema anyway; the lock is not " +
			"guarding the apply")
	}
	t.Logf("control CONFIRMED: Initialize waited %s on the externally held lock and applied "+
		"nothing (%v)", elapsed, initErr)
}

// controlUnlockedMigratorRaces reproduces the pre-slice algorithm verbatim —
// read the version outside any lock, then apply on the bare *sql.DB — and shows
// it breaking under concurrent replica startup.
//
// `IF NOT EXISTS` is not atomic: two sessions creating the same object race
// between the catalog check and the insert, and the loser gets a duplicate-key
// error on a pg_catalog index or "tuple concurrently updated". That is the
// defect .loom/32 correction 25 names, and it lived in the one migrator of six
// that took no lock.
//
// The race is probabilistic per attempt, so this control retries and requires
// only that it reproduce at least once. It is a demonstration that the hazard
// is real; controlTerminologyAdvisoryLock is the deterministic proof that the
// fix is on the path.
func controlUnlockedMigratorRaces(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()
	const attempts = 6
	const replicas = 4

	var lastFailure error
	for attempt := range attempts {
		raceDSN, _ := newCompatDatabase(t, dsn, "migcompat_control_race")
		db := openCompatDB(t, raceDSN)

		var wg sync.WaitGroup
		errs := make([]error, replicas)
		start := make(chan struct{})
		for i := range replicas {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = preSliceUnlockedInitialize(ctx, db)
			}()
		}
		close(start)
		wg.Wait()

		for _, err := range errs {
			if err != nil {
				lastFailure = err
				break
			}
		}
		if lastFailure != nil {
			t.Logf("control CONFIRMED on attempt %d: the pre-slice unlocked migrator failed "+
				"under %d concurrent replicas: %v", attempt+1, replicas, lastFailure)
			return
		}
	}

	t.Fatalf("CONTROL PASSED, WHICH MEANS THE HAZARD IS UNPROVEN: %d attempts at %d concurrent "+
		"replicas never made the unlocked pre-slice migrator fail. Either PostgreSQL now "+
		"serializes these statements, or this reproduction no longer matches the code that "+
		"shipped before slice 4.4a. Re-derive the control before trusting the lock.",
		attempts, replicas)
}

// preSliceUnlockedInitialize is pkg/terminology/db.Migrator.Initialize exactly
// as it stood at origin/main before slice 4.4a: version read outside any lock,
// each migration executed on the bare pool, no transaction.
func preSliceUnlockedInitialize(ctx context.Context, db *sql.DB) error {
	var schemaExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology')
	`).Scan(&schemaExists); err != nil {
		return err
	}
	current := 0
	if schemaExists {
		var tableExists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'terminology' AND table_name = 'schema_version'
			)
		`).Scan(&tableExists); err != nil {
			return err
		}
		if tableExists {
			if err := db.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(version), 0) FROM terminology.schema_version`,
			).Scan(&current); err != nil {
				return err
			}
		}
	}
	if current >= termdb.SchemaVersion {
		return nil
	}
	if current == 0 {
		if _, err := db.ExecContext(ctx, termdb.Schema); err != nil {
			return err
		}
	}
	if current < 2 {
		if _, err := db.ExecContext(ctx, termdb.SchemaV2Migration); err != nil {
			return err
		}
	}
	if current < 3 {
		if _, err := db.ExecContext(ctx, termdb.SchemaV3Migration); err != nil {
			return err
		}
	}
	return nil
}
