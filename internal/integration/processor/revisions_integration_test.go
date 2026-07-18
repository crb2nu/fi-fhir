//go:build integration

package processor_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	graphqlstore "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

func TestArtifactRevisionResolver_PostgresV1SurvivesV2(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for artifact resolver integration tests")
	}

	schema := fmt.Sprintf("artifact_resolver_%d", time.Now().UnixNano())
	createArtifactResolverSchema(t, dsn, schema)
	db := openArtifactResolverDB(t, dsn, schema)

	profiles := graphqlstore.NewPostgresProfileStore(db)
	workflows := graphqlstore.NewPostgresWorkflowLifecycleStore(db)
	if err := profiles.InitSchema(ctx); err != nil {
		t.Fatalf("profile InitSchema: %v", err)
	}
	if err := workflows.InitSchema(ctx); err != nil {
		t.Fatalf("workflow InitSchema: %v", err)
	}

	profile := &graphqlstore.Profile{
		ID:        "profile-adt",
		Name:      "ADT profile",
		Version:   "v1",
		Config:    json.RawMessage(`{"generation":1,"mapping":{"event":"patient_admit"},"threshold":1.230e-5}`),
		CreatedBy: "integration-test",
	}
	if err := profiles.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("CreateProfile(v1): %v", err)
	}
	profileV1Ref, err := processor.NewProfileRevisionReference(profile.ID, profile.CurrentRevisionID, profile.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(v1 caller state): %v", err)
	}
	profileV1, err := profiles.GetCurrentProfileRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentProfileRevision(v1): %v", err)
	}
	if profileV1 == nil {
		t.Fatal("profile v1 current revision is missing")
	}
	storedProfileV1Ref, err := processor.NewProfileRevisionReference(profile.ID, profileV1.ID, profileV1.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(stored v1): %v", err)
	}
	if storedProfileV1Ref != profileV1Ref {
		t.Fatalf("profile reference changed across JSONB storage: caller=%#v stored=%#v", profileV1Ref, storedProfileV1Ref)
	}

	workflow, err := workflows.CreateWorkflowDefinition(ctx, &graphqlstore.WorkflowDefinitionRecord{
		ID:   "workflow-adt",
		Name: fmt.Sprintf("artifact_resolver_adt_%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition: %v", err)
	}
	workflowV1, err := workflows.SaveWorkflowVersion(ctx, &graphqlstore.WorkflowVersionRecord{
		WorkflowID: workflow.ID,
		Yaml:       "name: adt-v1\nroutes: []\n",
		CreatedBy:  "integration-test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v1): %v", err)
	}
	if _, err := workflows.PublishWorkflowVersion(ctx, &graphqlstore.WorkflowReleaseRecord{
		WorkflowID:  workflow.ID,
		VersionID:   workflowV1.ID,
		Environment: "production",
		PublishedBy: "integration-test",
	}); err != nil {
		t.Fatalf("PublishWorkflowVersion(v1): %v", err)
	}
	workflowV1Ref, err := processor.NewWorkflowRevisionReference(workflow.ID, workflowV1.ID, []byte(workflowV1.Yaml))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(v1): %v", err)
	}

	profile.Version = "v2"
	profile.Config = json.RawMessage(`{"generation":2,"mapping":{"event":"patient_update"},"threshold":2.460e-5}`)
	profile.ChangeSummary = "Advance processor revision fixture"
	if err := profiles.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("UpdateProfile(v2): %v", err)
	}
	profileV2, err := profiles.GetCurrentProfileRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentProfileRevision(v2): %v", err)
	}
	if profileV2 == nil || profileV2.ID == profileV1.ID {
		t.Fatalf("profile current pointer did not advance: v1=%#v v2=%#v", profileV1, profileV2)
	}

	workflowV2, err := workflows.SaveWorkflowVersion(ctx, &graphqlstore.WorkflowVersionRecord{
		WorkflowID: workflow.ID,
		Yaml:       "name: adt-v2\nroutes: []\n",
		CreatedBy:  "integration-test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v2): %v", err)
	}
	if _, err := workflows.PublishWorkflowVersion(ctx, &graphqlstore.WorkflowReleaseRecord{
		WorkflowID:  workflow.ID,
		VersionID:   workflowV2.ID,
		Environment: "production",
		PublishedBy: "integration-test",
	}); err != nil {
		t.Fatalf("PublishWorkflowVersion(v2): %v", err)
	}
	publishedV2, err := workflows.GetPublishedWorkflowRelease(ctx, workflow.ID, "production")
	if err != nil {
		t.Fatalf("GetPublishedWorkflowRelease(v2): %v", err)
	}
	if publishedV2 == nil || publishedV2.VersionID != workflowV2.ID {
		t.Fatalf("workflow publication pointer did not advance to v2: release=%#v v2=%#v", publishedV2, workflowV2)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close first database handle: %v", err)
	}

	// Reopen the database and construct fresh stores, adapter, and resolver. The
	// resolver must not follow either mutable current/published pointer to v2.
	db = openArtifactResolverDB(t, dsn, schema)
	freshProfiles := graphqlstore.NewPostgresProfileStore(db)
	freshWorkflows := graphqlstore.NewPostgresWorkflowLifecycleStore(db)
	if err := freshProfiles.InitSchema(ctx); err != nil {
		t.Fatalf("fresh profile InitSchema: %v", err)
	}
	if err := freshWorkflows.InitSchema(ctx); err != nil {
		t.Fatalf("fresh workflow InitSchema: %v", err)
	}
	loader := graphqlstore.NewArtifactRevisionLoader(freshProfiles, freshWorkflows)
	resolver, err := processor.NewRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}

	resolved, err := resolver.Resolve(ctx, "tenant-a", profileV1Ref, workflowV1Ref)
	if err != nil {
		t.Fatalf("Resolve(v1 after v2): %v", err)
	}
	if !bytes.Equal(resolved.ProfileJSON(), profileV1.Config) {
		t.Fatalf("resolved profile v1 changed: got %s want %s", resolved.ProfileJSON(), profileV1.Config)
	}
	if !bytes.Equal(resolved.WorkflowYAML(), []byte(workflowV1.Yaml)) {
		t.Fatalf("resolved workflow v1 changed: got %q want %q", resolved.WorkflowYAML(), workflowV1.Yaml)
	}

	badDigest := profileV1Ref
	badDigest.Digest = mutateDigest(badDigest.Digest)
	if _, err := resolver.Resolve(ctx, "tenant-a", badDigest, workflowV1Ref); !errors.Is(err, processor.ErrArtifactDigestMismatch) {
		t.Fatalf("expected changed digest to fail closed, got %v", err)
	}

	notFoundProfileRef, err := processor.NewProfileRevisionReference(profile.ID, profileV2.ID+1000000, profileV1.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(not found): %v", err)
	}
	if _, err := resolver.Resolve(ctx, "tenant-a", notFoundProfileRef, workflowV1Ref); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected nonexistent profile revision to fail, got %v", err)
	}

	otherProfile := &graphqlstore.Profile{
		ID:      "profile-other",
		Name:    "Other profile",
		Version: "v1",
		Config:  json.RawMessage(`{"generation":1}`),
	}
	if err := freshProfiles.CreateProfile(ctx, otherProfile); err != nil {
		t.Fatalf("CreateProfile(other): %v", err)
	}
	wrongProfileOwnerRef, err := processor.NewProfileRevisionReference(otherProfile.ID, profileV1.ID, profileV1.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(wrong owner): %v", err)
	}
	if _, err := resolver.Resolve(ctx, "tenant-a", wrongProfileOwnerRef, workflowV1Ref); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected wrong profile owner to fail, got %v", err)
	}

	otherWorkflow, err := freshWorkflows.CreateWorkflowDefinition(ctx, &graphqlstore.WorkflowDefinitionRecord{
		ID:   "workflow-other",
		Name: fmt.Sprintf("artifact_resolver_other_%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition(other): %v", err)
	}
	wrongWorkflowOwnerRef, err := processor.NewWorkflowRevisionReference(otherWorkflow.ID, workflowV1.ID, []byte(workflowV1.Yaml))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(wrong owner): %v", err)
	}
	if _, err := resolver.Resolve(ctx, "tenant-a", profileV1Ref, wrongWorkflowOwnerRef); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected wrong workflow owner to fail, got %v", err)
	}

	if _, err := resolver.Resolve(ctx, "tenant-other", profileV1Ref, workflowV1Ref); !errors.Is(err, processor.ErrTenantMismatch) {
		t.Fatalf("expected wrong deployment tenant to fail, got %v", err)
	}
}

func createArtifactResolverSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres for schema creation: %v", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatalf("create artifact resolver schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close schema creation database handle: %v", err)
	}

	t.Cleanup(func() {
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanupDB.Close() }()
		_, _ = cleanupDB.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
	})
}

func openArtifactResolverDB(t *testing.T, dsn, schema string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open artifact resolver postgres: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("ping artifact resolver postgres: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatalf("set artifact resolver search path: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mutateDigest(digest string) string {
	if strings.HasSuffix(digest, "0") {
		return digest[:len(digest)-1] + "1"
	}
	return digest[:len(digest)-1] + "0"
}
