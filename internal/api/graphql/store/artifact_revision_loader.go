package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ProfileRevisionReader is the narrow profile persistence surface needed by the runtime.
type ProfileRevisionReader interface {
	GetProfileRevision(ctx context.Context, profileID string, revisionID int) (*ProfileRevision, error)
}

// WorkflowVersionReader is the narrow workflow persistence surface needed by the runtime.
type WorkflowVersionReader interface {
	GetWorkflowVersionForWorkflow(ctx context.Context, workflowID, versionID string) (*WorkflowVersionRecord, error)
}

// ArtifactRevisionLoader adapts GraphQL lifecycle storage to storage-neutral artifact bytes.
type ArtifactRevisionLoader struct {
	profiles  ProfileRevisionReader
	workflows WorkflowVersionReader
}

// NewArtifactRevisionLoader creates an exact immutable artifact loader.
func NewArtifactRevisionLoader(
	profiles ProfileRevisionReader,
	workflows WorkflowVersionReader,
) *ArtifactRevisionLoader {
	return &ArtifactRevisionLoader{profiles: profiles, workflows: workflows}
}

// LoadProfileRevision loads a profile only when both its owner and serial revision ID match.
func (l *ArtifactRevisionLoader) LoadProfileRevision(
	ctx context.Context,
	artifactID string,
	revisionID string,
) ([]byte, error) {
	if l == nil || l.profiles == nil {
		return nil, fmt.Errorf("profile revision reader is not configured")
	}
	if strings.TrimSpace(artifactID) == "" || strings.TrimSpace(artifactID) != artifactID {
		return nil, fmt.Errorf("profile artifact ID is invalid")
	}
	parsedRevisionID, err := strconv.Atoi(revisionID)
	if err != nil || parsedRevisionID <= 0 || strconv.Itoa(parsedRevisionID) != revisionID {
		return nil, fmt.Errorf("profile revision ID %q is not a canonical positive decimal integer", revisionID)
	}

	revision, err := l.profiles.GetProfileRevision(ctx, artifactID, parsedRevisionID)
	if err != nil {
		return nil, fmt.Errorf("get profile revision %s/%s: %w", artifactID, revisionID, err)
	}
	if revision == nil {
		return nil, fmt.Errorf("profile revision %s/%s not found", artifactID, revisionID)
	}
	if revision.ID != parsedRevisionID || revision.ProfileID != artifactID {
		return nil, fmt.Errorf(
			"profile revision %s/%s does not belong to requested profile",
			artifactID,
			revisionID,
		)
	}
	return append([]byte(nil), revision.Config...), nil
}

// LoadWorkflowRevision loads a workflow version and verifies version ownership.
func (l *ArtifactRevisionLoader) LoadWorkflowRevision(
	ctx context.Context,
	artifactID string,
	revisionID string,
) ([]byte, error) {
	if l == nil || l.workflows == nil {
		return nil, fmt.Errorf("workflow version reader is not configured")
	}
	if strings.TrimSpace(artifactID) == "" || strings.TrimSpace(artifactID) != artifactID {
		return nil, fmt.Errorf("workflow artifact ID is invalid")
	}
	if strings.TrimSpace(revisionID) == "" || strings.TrimSpace(revisionID) != revisionID {
		return nil, fmt.Errorf("workflow revision ID is invalid")
	}

	version, err := l.workflows.GetWorkflowVersionForWorkflow(ctx, artifactID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("get workflow revision %s/%s: %w", artifactID, revisionID, err)
	}
	if version == nil {
		return nil, fmt.Errorf("workflow revision %s/%s not found", artifactID, revisionID)
	}
	if version.ID != revisionID || version.WorkflowID != artifactID {
		return nil, fmt.Errorf(
			"workflow revision %s/%s does not belong to requested workflow",
			artifactID,
			revisionID,
		)
	}
	return append([]byte(nil), []byte(version.Yaml)...), nil
}
