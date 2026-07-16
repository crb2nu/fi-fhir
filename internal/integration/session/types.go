package session

import (
	"encoding/json"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusArchived SessionStatus = "archived"
)

type PHIPolicy string

const (
	PHIPolicyRetain PHIPolicy = "retain"
	PHIPolicyRedact PHIPolicy = "redact"
)

type ArtifactKind string

const (
	ArtifactKindMappingProfile ArtifactKind = "mapping_profile"
	ArtifactKindWorkflowDraft  ArtifactKind = "workflow_draft"
	ArtifactKindNotes          ArtifactKind = "notes"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
)

type StageStatus string

const (
	StageStatusPending   StageStatus = "pending"
	StageStatusRunning   StageStatus = "running"
	StageStatusSucceeded StageStatus = "succeeded"
	StageStatusFailed    StageStatus = "failed"
)

type Session struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Status      SessionStatus     `json:"status"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ArchivedAt  *time.Time        `json:"archived_at,omitempty"`
}

type CreateSessionRequest struct {
	Name        string
	Description string
	Tags        []string
	Metadata    map[string]string
}

type UpdateSessionRequest struct {
	Name        *string
	Description *string
	Tags        []string
	Metadata    map[string]string
}

type ListSessionsOptions struct {
	IncludeArchived bool
}

type Sample struct {
	ID          string              `json:"id"`
	SessionID   string              `json:"session_id"`
	Name        string              `json:"name"`
	Format      events.SourceFormat `json:"format"`
	Source      string              `json:"source,omitempty"`
	Raw         string              `json:"raw"`
	PHIPolicy   PHIPolicy           `json:"phi_policy"`
	PHIRedacted bool                `json:"phi_redacted"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type AddSampleRequest struct {
	Name      string
	Format    events.SourceFormat
	Source    string
	Raw       string
	PHIPolicy PHIPolicy
}

type ArtifactDraft struct {
	ID         string          `json:"id"`
	RevisionID string          `json:"revision_id"`
	SessionID  string          `json:"session_id"`
	Kind       ArtifactKind    `json:"kind"`
	Name       string          `json:"name"`
	Content    json.RawMessage `json:"content"`
	Version    int             `json:"version"`
	Digest     string          `json:"digest"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type SaveArtifactDraftRequest struct {
	ID      string
	Kind    ArtifactKind
	Name    string
	Content json.RawMessage
}

type Run struct {
	ID                    string        `json:"id"`
	SessionID             string        `json:"session_id"`
	SampleID              string        `json:"sample_id"`
	Status                RunStatus     `json:"status"`
	Source                string        `json:"source,omitempty"`
	ProfileID             string        `json:"profile_id,omitempty"`
	ProfileRevisionID     string        `json:"profile_revision_id,omitempty"`
	ProfileRevisionDigest string        `json:"profile_revision_digest,omitempty"`
	Stages                []RunStage    `json:"stages,omitempty"`
	Diagnostics           []Diagnostic  `json:"diagnostics,omitempty"`
	Lineage               []LineageLink `json:"lineage,omitempty"`
	Events                []ParsedEvent `json:"events,omitempty"`
	Error                 string        `json:"error,omitempty"`
	StartedAt             *time.Time    `json:"started_at,omitempty"`
	FinishedAt            *time.Time    `json:"finished_at,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

type RunStage struct {
	Name       string      `json:"name"`
	Status     StageStatus `json:"status"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type ParsedEvent struct {
	ID              string          `json:"id,omitempty"`
	Type            string          `json:"type,omitempty"`
	SourceMessageID string          `json:"source_message_id,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

type Diagnostic struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"`
	Phase     string    `json:"phase"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Path      string    `json:"path,omitempty"`
	Source    string    `json:"source"`
	Original  string    `json:"original,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LineageLink struct {
	SourcePath    string `json:"source_path"`
	SourceSegment string `json:"source_segment"`
	SourceField   int    `json:"source_field"`
	TargetPath    string `json:"target_path"`
	ValuePreview  string `json:"value_preview,omitempty"`
}

type ExportBundle struct {
	ID         string          `json:"id"`
	Session    Session         `json:"session"`
	Samples    []Sample        `json:"samples,omitempty"`
	Drafts     []ArtifactDraft `json:"drafts,omitempty"`
	Runs       []Run           `json:"runs,omitempty"`
	Decisions  []Decision      `json:"decisions,omitempty"`
	ExportedAt time.Time       `json:"exported_at"`
}

type RunRequest struct {
	SessionID         string
	SampleID          string
	Source            string
	ProfileRevisionID string
}

// Decision records one accepted diagnostic outcome without mutating the run.
type Decision struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	RunID        string    `json:"run_id"`
	DiagnosticID string    `json:"diagnostic_id"`
	AcceptedBy   string    `json:"accepted_by"`
	Reason       string    `json:"reason,omitempty"`
	AcceptedAt   time.Time `json:"accepted_at"`
}

type AcceptDecisionRequest struct {
	SessionID    string
	RunID        string
	DiagnosticID string
	AcceptedBy   string
	Reason       string
}
