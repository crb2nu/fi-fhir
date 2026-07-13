package store

import (
	"context"
	"strings"
	"testing"
)

func TestArtifactRevisionLoaderLoadsExactDefensiveProfileRevision(t *testing.T) {
	t.Parallel()

	record := &ProfileRevision{ID: 7, ProfileID: "profile-adt", Config: []byte(`{"name":"ADT"}`)}
	profiles := &profileRevisionReaderStub{revision: record}
	loader := NewArtifactRevisionLoader(profiles, &workflowVersionReaderStub{})

	config, err := loader.LoadProfileRevision(context.Background(), "profile-adt", "7")
	if err != nil {
		t.Fatalf("LoadProfileRevision: %v", err)
	}
	if profiles.profileID != "profile-adt" || profiles.revisionID != 7 {
		t.Fatalf("profile lookup was not exact: profile=%q revision=%d", profiles.profileID, profiles.revisionID)
	}
	config[0] = '!'
	if got := string(record.Config); got != `{"name":"ADT"}` {
		t.Fatalf("returned config aliases store record: %q", got)
	}
}

func TestArtifactRevisionLoaderRejectsProfileOwnerMismatch(t *testing.T) {
	t.Parallel()

	profiles := &profileRevisionReaderStub{revision: &ProfileRevision{
		ID:        7,
		ProfileID: "profile-other",
		Config:    []byte(`{}`),
	}}
	loader := NewArtifactRevisionLoader(profiles, &workflowVersionReaderStub{})

	_, err := loader.LoadProfileRevision(context.Background(), "profile-adt", "7")
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected profile ownership error, got %v", err)
	}
}

func TestArtifactRevisionLoaderLoadsExactDefensiveWorkflowRevision(t *testing.T) {
	t.Parallel()

	record := &WorkflowVersionRecord{ID: "version-7", WorkflowID: "workflow-adt", Yaml: "name: adt\n"}
	workflows := &workflowVersionReaderStub{version: record}
	loader := NewArtifactRevisionLoader(&profileRevisionReaderStub{}, workflows)

	yaml, err := loader.LoadWorkflowRevision(context.Background(), "workflow-adt", "version-7")
	if err != nil {
		t.Fatalf("LoadWorkflowRevision: %v", err)
	}
	if workflows.workflowID != "workflow-adt" || workflows.versionID != "version-7" {
		t.Fatalf("workflow lookup was not exact: workflow=%q version=%q", workflows.workflowID, workflows.versionID)
	}
	yaml[0] = '!'
	if got := record.Yaml; got != "name: adt\n" {
		t.Fatalf("returned YAML aliases store record: %q", got)
	}
}

func TestArtifactRevisionLoaderRejectsWorkflowOwnerMismatch(t *testing.T) {
	t.Parallel()

	workflows := &workflowVersionReaderStub{version: &WorkflowVersionRecord{
		ID:         "version-7",
		WorkflowID: "workflow-other",
		Yaml:       "name: other\n",
	}}
	loader := NewArtifactRevisionLoader(&profileRevisionReaderStub{}, workflows)

	_, err := loader.LoadWorkflowRevision(context.Background(), "workflow-adt", "version-7")
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected workflow ownership error, got %v", err)
	}
}

func TestArtifactRevisionLoaderRejectsMalformedOrMissingRevisions(t *testing.T) {
	t.Parallel()

	loader := NewArtifactRevisionLoader(&profileRevisionReaderStub{}, &workflowVersionReaderStub{})
	if _, err := loader.LoadProfileRevision(context.Background(), "profile-adt", "01"); err == nil {
		t.Fatal("expected noncanonical profile revision ID to fail")
	}
	if _, err := loader.LoadProfileRevision(context.Background(), "profile-adt", "1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing profile error, got %v", err)
	}
	if _, err := loader.LoadWorkflowRevision(context.Background(), "workflow-adt", "version-1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing workflow error, got %v", err)
	}
}

type profileRevisionReaderStub struct {
	revision   *ProfileRevision
	err        error
	profileID  string
	revisionID int
}

func (s *profileRevisionReaderStub) GetProfileRevision(_ context.Context, profileID string, revisionID int) (*ProfileRevision, error) {
	s.profileID = profileID
	s.revisionID = revisionID
	return s.revision, s.err
}

type workflowVersionReaderStub struct {
	workflowID string
	version    *WorkflowVersionRecord
	err        error
	versionID  string
}

func (s *workflowVersionReaderStub) GetWorkflowVersion(_ context.Context, versionID string) (*WorkflowVersionRecord, error) {
	s.versionID = versionID
	return s.version, s.err
}

func (s *workflowVersionReaderStub) GetWorkflowVersionForWorkflow(
	_ context.Context,
	workflowID string,
	versionID string,
) (*WorkflowVersionRecord, error) {
	s.workflowID = workflowID
	s.versionID = versionID
	return s.version, s.err
}
