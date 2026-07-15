//go:build integration

package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestPostgresDeploymentLifecycle_RaceRestartImmutableRelease(t *testing.T) {
	ctx := t.Context()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for deployment lifecycle integration tests")
	}
	schema := fmt.Sprintf("deployment_lifecycle_%d", time.Now().UnixNano())
	createLifecycleSchema(t, dsn, schema)
	schemaDSN := lifecycleSchemaDSN(t, dsn, schema)

	baseTime := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)
	var clockNanos atomic.Int64
	clockNanos.Store(baseTime.UnixNano())
	clock := func() time.Time { return time.Unix(0, clockNanos.Load()).UTC() }
	var validationCalls atomic.Int64
	validator := func(_ context.Context, revision integration.IntegrationDefinitionRevision) (ConnectionValidationOutcome, error) {
		if revision.Source.SourceID != "adt-east" || revision.Deployment == nil {
			return ConnectionValidationOutcome{}, errors.New("unexpected exact revision")
		}
		if validationCalls.Add(1) == 1 {
			return ConnectionValidationOutcome{Codes: []string{"SOURCE_UNREACHABLE"}}, nil
		}
		return ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}}, nil
	}

	db := openLifecycleDB(t, schemaDSN)
	catalog := newLifecycleCatalog(t, db, clock, validator)
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(first): %v", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(idempotent): %v", err)
	}
	revision := lifecycleRevision(t, baseTime)
	snapshot, err := catalog.CreateDraft(ctx, revision)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateDraft, 1)
	if _, err := catalog.CreateDraft(ctx, revision); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate CreateDraft error = %v", err)
	}
	if _, err := catalog.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft ResolveRunnable error = %v", err)
	}
	if _, err := catalog.Approve(ctx, lifecycleCommand(revision, 1, "skip validation")); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("draft approval error = %v", err)
	}

	snapshot, err = catalog.ValidateConnection(ctx, lifecycleCommand(revision, 1, "validate source"))
	if !errors.Is(err, ErrConnectionValidationFailed) {
		t.Fatalf("failed validation error = %v", err)
	}
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateDraft, 2)
	failedValidation, err := catalog.GetValidation(ctx, snapshot.LastValidationID)
	if err != nil || failedValidation.Passed || failedValidation.SourceRevision != revision.Source.ArtifactRevisionRef {
		t.Fatalf("failed validation record = %#v, %v", failedValidation, err)
	}
	if _, err := catalog.ValidateConnection(ctx, lifecycleCommand(revision, 1, "stale validation")); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale validation error = %v", err)
	}

	snapshot, err = catalog.ValidateConnection(ctx, lifecycleCommand(revision, 2, "validate corrected connection"))
	if err != nil {
		t.Fatalf("ValidateConnection(pass): %v", err)
	}
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateValidated, 3)
	snapshot, err = catalog.Approve(ctx, lifecycleCommand(revision, 3, "approve tested connection"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateApproved, 4)
	snapshot, err = catalog.Publish(ctx, lifecycleCommand(revision, 4, "publish exact tested revision"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStatePublished, 5)
	if snapshot.ReleaseID == "" {
		t.Fatal("publish did not create a release")
	}
	release, err := catalog.GetRelease(ctx, snapshot.ReleaseID)
	if err != nil || release.DefinitionRevision != revision.Reference() {
		t.Fatalf("GetRelease = %#v, %v", release, err)
	}
	expectedReleaseJSON, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("marshal release evidence: %v", err)
	}
	assertImmutableLifecycleRows(t, db, release.ID, revision)
	if _, err := catalog.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("published ResolveRunnable error = %v", err)
	}

	snapshot, err = catalog.Deploy(ctx, lifecycleCommand(revision, 5, "deploy published release"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateDeployed, 6)
	binding, err := catalog.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID)
	if err != nil || binding.IntegrationRevision != revision.Reference() || binding.ReleaseID != release.ID {
		t.Fatalf("deployed binding = %#v, %v", binding, err)
	}
	snapshot, err = catalog.ReportHealth(
		ctx,
		lifecycleCommand(revision, 6, "readiness checks passed"),
		integration.DeploymentHealthHealthy,
	)
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateDeployed, 7)

	revision2 := lifecycleRevisionVersion(t, baseTime.Add(time.Second), "rev-2", revision.RevisionID)
	snapshot2, err := catalog.CreateDraft(ctx, revision2)
	if err != nil {
		t.Fatalf("CreateDraft(v2): %v", err)
	}
	snapshot2, err = catalog.ValidateConnection(ctx, lifecycleCommand(revision2, 1, "validate replacement connection"))
	snapshot2 = advanceLifecycle(t, snapshot2, err)
	snapshot2, err = catalog.Approve(ctx, lifecycleCommand(revision2, 2, "approve replacement"))
	snapshot2 = advanceLifecycle(t, snapshot2, err)
	snapshot2, err = catalog.Publish(ctx, lifecycleCommand(revision2, 3, "publish replacement"))
	snapshot2 = advanceLifecycle(t, snapshot2, err)
	assertLifecycleSnapshot(t, snapshot2, integration.DeploymentStatePublished, 4)
	if _, err := catalog.Deploy(ctx, lifecycleCommand(revision2, 4, "deploy conflicting replacement")); !errors.Is(err, ErrActiveDeployment) {
		t.Fatalf("active-release deployment error = %v", err)
	}

	const contenders = 32
	results := make(chan error, contenders)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < contenders; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, pauseErr := catalog.Pause(ctx, lifecycleCommand(revision, 7, "operator pause"))
			results <- pauseErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for pauseErr := range results {
		switch {
		case pauseErr == nil:
			successes++
		case errors.Is(pauseErr, ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("pause contender error = %v", pauseErr)
		}
	}
	if successes != 1 || conflicts != contenders-1 {
		t.Fatalf("pause race successes=%d conflicts=%d", successes, conflicts)
	}
	snapshot, err = catalog.GetSnapshot(ctx, revision.TenantID, revision.DefinitionID, revision.RevisionID)
	if err != nil {
		t.Fatalf("GetSnapshot(paused): %v", err)
	}
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStatePaused, 8)
	if _, err := catalog.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("paused ResolveRunnable error = %v", err)
	}

	clockNanos.Store(baseTime.Add(10 * time.Minute).UnixNano())
	if _, err := catalog.Resume(ctx, lifecycleCommand(revision, 8, "resume after pause")); !errors.Is(err, ErrConnectionValidationRequired) {
		t.Fatalf("stale-validation resume error = %v", err)
	}
	snapshot, err = catalog.ValidateConnection(ctx, lifecycleCommand(revision, 8, "refresh connection validation"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStatePaused, 9)
	snapshot, err = catalog.Resume(ctx, lifecycleCommand(revision, 9, "resume validated release"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateDeployed, 10)
	if snapshot.ReleaseID != release.ID {
		t.Fatalf("resume changed immutable release: got %q want %q", snapshot.ReleaseID, release.ID)
	}

	eventsBeforeRestart, err := catalog.ListEvents(ctx, revision.TenantID, revision.DefinitionID, revision.RevisionID)
	if err != nil || len(eventsBeforeRestart) != 10 {
		t.Fatalf("events before restart = %d, %v", len(eventsBeforeRestart), err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close lifecycle database: %v", err)
	}
	restartedDB := openLifecycleDB(t, schemaDSN)
	restarted := newLifecycleCatalog(t, restartedDB, clock, validator)
	if err := restarted.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(restart): %v", err)
	}
	restartedRelease, err := restarted.GetRelease(ctx, release.ID)
	if err != nil {
		t.Fatalf("GetRelease(restart): %v", err)
	}
	restartedReleaseJSON, _ := json.Marshal(restartedRelease)
	if string(restartedReleaseJSON) != string(expectedReleaseJSON) {
		t.Fatalf("release changed across restart:\nwant %s\n got %s", expectedReleaseJSON, restartedReleaseJSON)
	}
	eventsAfterRestart, err := restarted.ListEvents(ctx, revision.TenantID, revision.DefinitionID, revision.RevisionID)
	if err != nil || len(eventsAfterRestart) != len(eventsBeforeRestart) {
		t.Fatalf("events after restart = %d, %v", len(eventsAfterRestart), err)
	}
	for index, event := range eventsAfterRestart {
		if event.Version != int64(index+1) {
			t.Fatalf("event %d version = %d", index, event.Version)
		}
	}
	binding, err = restarted.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID)
	if err != nil || binding.IntegrationRevision != revision.Reference() {
		t.Fatalf("restarted binding = %#v, %v", binding, err)
	}
	snapshot, err = restarted.Retire(ctx, lifecycleCommand(revision, 10, "retire completed release"))
	snapshot = advanceLifecycle(t, snapshot, err)
	assertLifecycleSnapshot(t, snapshot, integration.DeploymentStateRetired, 11)
	if _, err := restarted.ResolveRunnable(ctx, revision.TenantID, revision.DefinitionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retired ResolveRunnable error = %v", err)
	}
	assertLifecycleCounts(t, restartedDB, 2, 4, 2, 2, 15)
	assertLifecyclePersistenceSafe(t, restartedDB)
	if err := restartedDB.Close(); err != nil {
		t.Fatalf("close restarted lifecycle database: %v", err)
	}
}

func lifecycleRevision(t *testing.T, createdAt time.Time) integration.IntegrationDefinitionRevision {
	t.Helper()
	return lifecycleRevisionVersion(t, createdAt, "rev-1", "")
}

func lifecycleRevisionVersion(t *testing.T, createdAt time.Time, revisionID, parentRevisionID string) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	policy := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health: integration.HealthPolicy{
			StartupGraceSeconds: 30, CheckIntervalSeconds: 15,
			TimeoutSeconds: 5, FailureThreshold: 3,
		},
		Capacity: integration.CapacityPolicy{
			MaxInFlight: 32, MaxQueued: 1024, MaxMessagesPerSecond: 250,
		},
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "adt-http", RevisionID: revisionID,
		ParentRevisionID: parentRevisionID, TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest('1')},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: digest('2')},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest('3')},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "destination-fhir", RevisionID: "destination-1", Digest: digest('4')},
			Class:               integration.DestinationClassProduction,
		}},
		SecretBindings: []integration.SecretBinding{{
			Name: "source_credential",
			Reference: integration.SecretReference{
				Provider: integration.SecretProviderKubernetes,
				Key:      "fi-fhir/source-credential",
				Version:  "1",
			},
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: &policy,
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "engineer-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"integration:engineer"},
			},
			Reason: "create deployment lifecycle fixture", OccurredAt: createdAt,
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}

func lifecycleCommand(revision integration.IntegrationDefinitionRevision, version int64, reason string) Command {
	return Command{
		TenantID: revision.TenantID, DefinitionID: revision.DefinitionID,
		RevisionID: revision.RevisionID, ExpectedVersion: version,
		Principal: integration.Principal{
			ID: "operator-1", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{"integration:operator"},
		},
		Reason: reason,
	}
}

func advanceLifecycle(t *testing.T, snapshot Snapshot, err error) Snapshot {
	t.Helper()
	if err != nil {
		t.Fatalf("lifecycle transition: %v", err)
	}
	return snapshot
}

func assertLifecycleSnapshot(t *testing.T, snapshot Snapshot, state integration.DeploymentState, version int64) {
	t.Helper()
	if snapshot.State != state || snapshot.Version != version {
		t.Fatalf("snapshot state/version = %s/%d, want %s/%d", snapshot.State, snapshot.Version, state, version)
	}
}

func assertImmutableLifecycleRows(t *testing.T, db *sql.DB, releaseID string, revision integration.IntegrationDefinitionRevision) {
	t.Helper()
	statements := []string{
		`UPDATE integration_release_records SET approval_event_id = 'mutated' WHERE release_id = $1`,
		`DELETE FROM integration_release_records WHERE release_id = $1`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, releaseID); err == nil {
			t.Fatalf("immutable release accepted statement %q", statement)
		}
	}
	if _, err := db.Exec(`UPDATE integration_definition_revisions SET digest = $1 WHERE tenant_id = $2 AND definition_id = $3 AND revision_id = $4`,
		"sha256:"+strings.Repeat("9", 64), revision.TenantID, revision.DefinitionID, revision.RevisionID); err == nil {
		t.Fatal("immutable definition revision accepted update")
	}
}

func assertLifecycleCounts(t *testing.T, db *sql.DB, revisions, validations, releases, snapshots, events int) {
	t.Helper()
	wants := map[string]int{
		"integration_definition_revisions":   revisions,
		"integration_connection_validations": validations,
		"integration_release_records":        releases,
		"integration_lifecycle_snapshots":    snapshots,
		"integration_lifecycle_events":       events,
	}
	for table, want := range wants {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + pq.QuoteIdentifier(table)).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
}

func assertLifecyclePersistenceSafe(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`
		SELECT revision_json::text FROM integration_definition_revisions
		UNION ALL SELECT source_revision::text FROM integration_connection_validations
		UNION ALL SELECT codes::text FROM integration_connection_validations
		UNION ALL SELECT audit_json::text FROM integration_connection_validations
		UNION ALL SELECT published_json::text FROM integration_release_records
		UNION ALL SELECT audit_json::text FROM integration_lifecycle_events
	`)
	if err != nil {
		t.Fatalf("query lifecycle JSON: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan lifecycle JSON: %v", err)
		}
		for _, forbidden := range []string{"MSH|^~\\&", "plaintext-secret-sentinel"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("lifecycle persistence leaked forbidden value %q", forbidden)
			}
		}
	}
}

func createLifecycleSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL for lifecycle schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatalf("create lifecycle schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close lifecycle schema database: %v", err)
	}
	t.Cleanup(func() {
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanupDB.Close() }()
		_, _ = cleanupDB.ExecContext(context.Background(), `DROP SCHEMA `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
}

func lifecycleSchemaDSN(t *testing.T, dsn, schema string) string {
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

func openLifecycleDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open lifecycle PostgreSQL: %v", err)
	}
	db.SetMaxOpenConns(40)
	db.SetMaxIdleConns(40)
	if err := db.PingContext(t.Context()); err != nil {
		_ = db.Close()
		t.Fatalf("ping lifecycle PostgreSQL: %v", err)
	}
	return db
}

func newLifecycleCatalog(t *testing.T, db *sql.DB, clock func() time.Time, validator ConnectionValidatorFunc) *PostgresCatalog {
	t.Helper()
	catalog, err := NewPostgresCatalog(db, Config{Clock: clock, ValidateConnection: validator})
	if err != nil {
		t.Fatalf("NewPostgresCatalog: %v", err)
	}
	return catalog
}
