//go:build integration

package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupPostgresWorkflowLifecycleStore(t *testing.T) *PostgresWorkflowLifecycleStore {
	t.Helper()

	eventStore := setupPgEventStore(t)
	store := NewPostgresWorkflowLifecycleStore(eventStore.db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return store
}

func TestPostgresWorkflowLifecycleStore_PublishRejectsVersionOwnedByAnotherWorkflow(t *testing.T) {
	s := setupPostgresWorkflowLifecycleStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	owner, err := s.CreateWorkflowDefinition(ctx, &WorkflowDefinitionRecord{Name: "pg_owner_" + suffix})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition(owner): %v", err)
	}
	other, err := s.CreateWorkflowDefinition(ctx, &WorkflowDefinitionRecord{Name: "pg_other_" + suffix})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition(other): %v", err)
	}
	version, err := s.SaveWorkflowVersion(ctx, &WorkflowVersionRecord{
		WorkflowID: owner.ID,
		Yaml:       "name: pg_owner\nroutes: []\n",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion: %v", err)
	}

	_, err = s.PublishWorkflowVersion(ctx, &WorkflowReleaseRecord{
		WorkflowID:  other.ID,
		VersionID:   version.ID,
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected cross-workflow publication to fail")
	}
	if !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected ownership error, got %v", err)
	}
	releases, err := s.ListWorkflowReleases(ctx, other.ID)
	if err != nil {
		t.Fatalf("ListWorkflowReleases: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected no release after rejected publication, got %d", len(releases))
	}
}

func TestPostgresWorkflowLifecycleStore_ConcurrentSavesAllocateUniqueVersions(t *testing.T) {
	s := setupPostgresWorkflowLifecycleStore(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	workflowID := "wf_concurrent_" + suffix
	definition, err := s.CreateWorkflowDefinition(ctx, &WorkflowDefinitionRecord{
		ID:   workflowID,
		Name: "concurrent_" + suffix,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition: %v", err)
	}

	functionName := "delay_workflow_version_" + suffix
	triggerName := "delay_workflow_version_insert_" + suffix
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.workflow_id = '%s' THEN
				PERFORM pg_sleep(0.25);
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER %s
		BEFORE INSERT ON workflow_versions
		FOR EACH ROW EXECUTE FUNCTION %s();
	`, functionName, workflowID, triggerName, functionName))
	if err != nil {
		t.Fatalf("install concurrency barrier: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.db.ExecContext(context.Background(), fmt.Sprintf(`
			DROP TRIGGER IF EXISTS %s ON workflow_versions;
			DROP FUNCTION IF EXISTS %s();
		`, triggerName, functionName))
	})

	const workers = 8
	start := make(chan struct{})
	versions := make(chan int, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			version, saveErr := s.SaveWorkflowVersion(ctx, &WorkflowVersionRecord{
				WorkflowID: definition.ID,
				Yaml:       fmt.Sprintf("name: concurrent\nworker: %d\n", worker),
			})
			if saveErr != nil {
				errs <- saveErr
				return
			}
			versions <- version.VersionNumber
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	close(versions)

	for saveErr := range errs {
		t.Errorf("concurrent SaveWorkflowVersion failed: %v", saveErr)
	}
	got := make([]int, 0, workers)
	for version := range versions {
		got = append(got, version)
	}
	if len(got) != workers {
		t.Fatalf("expected %d saved versions, got %d", workers, len(got))
	}
	sort.Ints(got)
	for i, version := range got {
		if want := i + 1; version != want {
			t.Fatalf("version numbers = %v; want contiguous 1..%d", got, workers)
		}
	}
}
