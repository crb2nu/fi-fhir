//go:build integration

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

func setupProfileStoreDatabase(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for profile-store integration tests")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("ping postgres: %v", err)
	}

	schema := fmt.Sprintf("profile_store_%d", time.Now().UnixNano())
	quotedSchema := pq.QuoteIdentifier(schema)
	if _, err := db.Exec(`CREATE SCHEMA ` + quotedSchema); err != nil {
		_ = db.Close()
		t.Fatalf("create test schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + quotedSchema); err != nil {
		_ = db.Close()
		t.Fatalf("set test search path: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA ` + quotedSchema + ` CASCADE`)
		_ = db.Close()
	})

	return db
}

func TestPostgresProfileStore_ImmutableRevisionLifecycleAndBackfill(t *testing.T) {
	ctx := context.Background()
	db := setupProfileStoreDatabase(t)

	// Reproduce the pre-revision-pointer schema so InitSchema must upgrade live data.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE source_profiles (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			version VARCHAR(32) NOT NULL DEFAULT '1.0.0',
			config JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by VARCHAR(255),
			is_active BOOLEAN NOT NULL DEFAULT true
		);
		CREATE TABLE profile_revisions (
			id SERIAL PRIMARY KEY,
			profile_id VARCHAR(64) REFERENCES source_profiles(id) ON DELETE CASCADE,
			version VARCHAR(32) NOT NULL,
			config JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by VARCHAR(255),
			change_summary TEXT
		);
		INSERT INTO source_profiles (id, name, version, config, created_by)
		VALUES ('legacy', 'Legacy feed', 'release', '{"generation":2}', 'migration-user');
		INSERT INTO profile_revisions (profile_id, version, config, created_by, change_summary)
		VALUES ('legacy', 'release', '{"generation":1}', 'migration-user', 'Legacy history');
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	store := NewPostgresProfileStore(db)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("second InitSchema must be idempotent: %v", err)
	}

	// Emulate an old pod that remains live during a rolling deployment. Its SQL
	// knows nothing about current_revision_id and must still leave exact current
	// revisions after the new schema has been installed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO source_profiles (id, name, version, config, created_by, is_active)
		VALUES ('rolling-legacy', 'Rolling legacy', 'v1', '{"generation":1}', 'old-pod', true)
	`)
	if err != nil {
		t.Fatalf("legacy create after migration: %v", err)
	}
	rollingV1, err := store.GetCurrentProfileRevision(ctx, "rolling-legacy")
	if err != nil {
		t.Fatalf("get rolling legacy v1: %v", err)
	}
	if rollingV1 == nil {
		t.Fatal("legacy create after migration did not create a current revision")
	}
	assertJSONEqual(t, rollingV1.Config, `{"generation":1}`)

	_, err = db.ExecContext(ctx, `
		UPDATE source_profiles
		SET version = 'v2', config = '{"generation":2}', updated_at = NOW()
		WHERE id = 'rolling-legacy'
	`)
	if err != nil {
		t.Fatalf("legacy update after migration: %v", err)
	}
	rollingV2, err := store.GetCurrentProfileRevision(ctx, "rolling-legacy")
	if err != nil {
		t.Fatalf("get rolling legacy v2: %v", err)
	}
	if rollingV2 == nil || rollingV2.ID == rollingV1.ID {
		t.Fatalf("legacy update did not advance current revision: v1=%#v v2=%#v", rollingV1, rollingV2)
	}
	assertJSONEqual(t, rollingV2.Config, `{"generation":2}`)

	legacyCurrent, err := store.GetCurrentProfileRevision(ctx, "legacy")
	if err != nil {
		t.Fatalf("get backfilled current revision: %v", err)
	}
	if legacyCurrent == nil {
		t.Fatal("expected a backfilled current revision")
	}
	assertJSONEqual(t, legacyCurrent.Config, `{"generation":2}`)

	legacyRevisions, err := store.GetProfileRevisions(ctx, "legacy")
	if err != nil {
		t.Fatalf("get legacy revisions: %v", err)
	}
	if len(legacyRevisions) != 2 {
		t.Fatalf("idempotent backfill should retain one historical and one current revision, got %d", len(legacyRevisions))
	}
	for _, revision := range legacyRevisions {
		if revision.Version != "release" {
			t.Fatalf("legacy duplicate version labels must be preserved, got %q", revision.Version)
		}
	}

	profile := &Profile{
		ID:        "adt-feed",
		Name:      "ADT feed",
		Version:   "release",
		Config:    json.RawMessage(`{"generation":1,"mapping":{"event":"patient_admit"}}`),
		CreatedBy: "alice",
	}
	if err := store.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	created, err := store.GetProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetProfile after create: %v", err)
	}
	if created == nil || created.CurrentRevisionID == 0 {
		t.Fatalf("created profile must point to its initial revision: %#v", created)
	}

	v1, err := store.GetCurrentProfileRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentProfileRevision v1: %v", err)
	}
	if v1 == nil || v1.ID != created.CurrentRevisionID {
		t.Fatalf("current v1 mismatch: profile=%#v revision=%#v", created, v1)
	}
	assertJSONEqual(t, v1.Config, `{"generation":1,"mapping":{"event":"patient_admit"}}`)

	// A revision failure must roll back the mutable row created in the same transaction.
	installProfileRevisionFailureTrigger(t, ctx, db, "rollback-create")
	err = store.CreateProfile(ctx, &Profile{
		ID:      "rollback-create",
		Name:    "must roll back",
		Version: "release",
		Config:  json.RawMessage(`{"generation":1}`),
	})
	if err == nil {
		t.Fatal("expected forced revision insert failure")
	}
	rolledBack, getErr := store.GetProfile(ctx, "rollback-create")
	if getErr != nil {
		t.Fatalf("GetProfile after failed create: %v", getErr)
	}
	if rolledBack != nil {
		t.Fatalf("failed create left a mutable profile without a revision: %#v", rolledBack)
	}
	removeProfileRevisionFailureTrigger(t, ctx, db)

	profile.Name = "ADT feed v2"
	profile.Version = "release" // Display labels are not immutable identities or uniqueness keys.
	profile.Config = json.RawMessage(`{"generation":2,"mapping":{"event":"patient_update"}}`)
	profile.CreatedBy = "bob"
	profile.ChangeSummary = "Route updates as patient_update events"
	if err := store.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	v2, err := store.GetCurrentProfileRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentProfileRevision v2: %v", err)
	}
	if v2 == nil || v2.ID == v1.ID {
		t.Fatalf("update must advance to a new immutable revision: v1=%#v v2=%#v", v1, v2)
	}
	if v2.ChangeSummary != profile.ChangeSummary || v2.CreatedBy != "bob" {
		t.Fatalf("revision audit metadata = actor %q summary %q", v2.CreatedBy, v2.ChangeSummary)
	}
	assertJSONEqual(t, v2.Config, `{"generation":2,"mapping":{"event":"patient_update"}}`)

	resolvedV1, err := store.GetProfileRevision(ctx, profile.ID, v1.ID)
	if err != nil {
		t.Fatalf("GetProfileRevision v1: %v", err)
	}
	if resolvedV1 == nil {
		t.Fatal("v1 must remain exactly addressable after v2 becomes current")
	}
	assertJSONEqual(t, resolvedV1.Config, `{"generation":1,"mapping":{"event":"patient_admit"}}`)

	other := &Profile{
		ID:      "other-feed",
		Name:    "Other feed",
		Version: "release",
		Config:  json.RawMessage(`{"generation":1}`),
	}
	if err := store.CreateProfile(ctx, other); err != nil {
		t.Fatalf("create other profile: %v", err)
	}
	wrongOwner, err := store.GetProfileRevision(ctx, other.ID, v1.ID)
	if err != nil {
		t.Fatalf("wrong-owner lookup: %v", err)
	}
	if wrongOwner != nil {
		t.Fatalf("exact lookup returned a revision owned by another profile: %#v", wrongOwner)
	}

	revisions, err := store.GetProfileRevisions(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetProfileRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revision history must include initial and current values, got %d", len(revisions))
	}
	for _, revision := range revisions {
		if revision.Version != "release" {
			t.Fatalf("duplicate display version changed unexpectedly: %#v", revision)
		}
	}

	// Returned JSON buffers must not alias later reads or store state.
	resolvedV1.Config[0] = '['
	v1Again, err := store.GetProfileRevision(ctx, profile.ID, v1.ID)
	if err != nil {
		t.Fatalf("GetProfileRevision v1 after caller mutation: %v", err)
	}
	assertJSONEqual(t, v1Again.Config, `{"generation":1,"mapping":{"event":"patient_admit"}}`)

	installProfileRevisionFailureTrigger(t, ctx, db, profile.ID)
	profile.Name = "must roll back"
	profile.Config = json.RawMessage(`{"generation":3}`)
	profile.ChangeSummary = "Exercise transactional rollback"
	err = store.UpdateProfile(ctx, profile)
	if err == nil {
		t.Fatal("expected forced update revision failure")
	}
	removeProfileRevisionFailureTrigger(t, ctx, db)

	afterFailedUpdate, err := store.GetCurrentProfileRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get current after failed update: %v", err)
	}
	if afterFailedUpdate == nil || afterFailedUpdate.ID != v2.ID {
		t.Fatalf("failed update advanced current revision: before=%#v after=%#v", v2, afterFailedUpdate)
	}
	assertJSONEqual(t, afterFailedUpdate.Config, `{"generation":2,"mapping":{"event":"patient_update"}}`)
}

func installProfileRevisionFailureTrigger(t *testing.T, ctx context.Context, db *sql.DB, profileID string) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION fail_selected_profile_revision() RETURNS trigger AS $$
		BEGIN
			IF NEW.profile_id = `+pq.QuoteLiteral(profileID)+` THEN
				RAISE EXCEPTION 'forced profile revision failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_selected_profile_revision
			BEFORE INSERT ON profile_revisions
			FOR EACH ROW EXECUTE FUNCTION fail_selected_profile_revision();
	`)
	if err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
}

func removeProfileRevisionFailureTrigger(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		DROP TRIGGER fail_selected_profile_revision ON profile_revisions;
		DROP FUNCTION fail_selected_profile_revision();
	`)
	if err != nil {
		t.Fatalf("remove failure trigger: %v", err)
	}
}

func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode got JSON %q: %v", got, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected JSON %q: %v", want, err)
	}

	gotCanonical, _ := json.Marshal(gotValue)
	wantCanonical, _ := json.Marshal(wantValue)
	if string(gotCanonical) != string(wantCanonical) {
		t.Fatalf("JSON mismatch: got %s want %s", gotCanonical, wantCanonical)
	}
}
