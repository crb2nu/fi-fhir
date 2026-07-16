package model

import "time"

type IntegrationSession struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	Description          *string             `json:"description,omitempty"`
	Archived             bool                `json:"archived"`
	CreatedAt            time.Time           `json:"createdAt"`
	UpdatedAt            time.Time           `json:"updatedAt"`
	Samples              []SessionSample     `json:"samples"`
	Artifacts            []SessionArtifact   `json:"artifacts"`
	Runs                 []SessionRun        `json:"runs"`
	Diagnostics          []SessionDiagnostic `json:"diagnostics"`
	CurrentProfileDraft  *SessionArtifact    `json:"currentProfileDraft,omitempty"`
	CurrentWorkflowDraft *SessionArtifact    `json:"currentWorkflowDraft,omitempty"`
}

type SessionSample struct {
	ID              string       `json:"id"`
	SessionID       string       `json:"sessionId"`
	Name            string       `json:"name"`
	Format          SourceFormat `json:"format"`
	Source          *string      `json:"source,omitempty"`
	RawPayload      *string      `json:"rawPayload,omitempty"`
	PayloadChecksum string       `json:"payloadChecksum"`
	PayloadRef      *string      `json:"payloadRef,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
}

type SessionArtifact struct {
	ID         string    `json:"id"`
	RevisionID string    `json:"revisionId"`
	SessionID  string    `json:"sessionId"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	Content    string    `json:"content"`
	Version    int       `json:"version"`
	Digest     string    `json:"digest"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type SessionRun struct {
	ID                    string              `json:"id"`
	SessionID             string              `json:"sessionId"`
	SampleID              *string             `json:"sampleId,omitempty"`
	Status                string              `json:"status"`
	ProfileRevisionID     *string             `json:"profileRevisionId,omitempty"`
	ProfileRevisionDigest *string             `json:"profileRevisionDigest,omitempty"`
	CreatedAt             time.Time           `json:"createdAt"`
	CompletedAt           *time.Time          `json:"completedAt,omitempty"`
	Stages                []RunStage          `json:"stages"`
	Diagnostics           []SessionDiagnostic `json:"diagnostics"`
	Events                []Event             `json:"events"`
	Warnings              []ParseWarning      `json:"warnings"`
}

type RunStage struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DurationMs  int        `json:"durationMs"`
	Summary     *string    `json:"summary,omitempty"`
}

type SessionDiagnostic struct {
	ID            string        `json:"id"`
	SessionID     string        `json:"sessionId"`
	RunID         *string       `json:"runId,omitempty"`
	SampleID      *string       `json:"sampleId,omitempty"`
	Severity      string        `json:"severity"`
	Code          string        `json:"code"`
	Message       string        `json:"message"`
	Path          *string       `json:"path,omitempty"`
	FixSuggestion *string       `json:"fixSuggestion,omitempty"`
	Accepted      bool          `json:"accepted"`
	AcceptedAt    *time.Time    `json:"acceptedAt,omitempty"`
	Lineage       []LineageLink `json:"lineage"`
}

type LineageLink struct {
	SourcePath  string  `json:"sourcePath"`
	TargetPath  *string `json:"targetPath,omitempty"`
	Description *string `json:"description,omitempty"`
}

type IntegrationBundle struct {
	SessionID   string              `json:"sessionId"`
	ExportedAt  time.Time           `json:"exportedAt"`
	Session     IntegrationSession  `json:"session"`
	Samples     []SessionSample     `json:"samples"`
	Artifacts   []SessionArtifact   `json:"artifacts"`
	Runs        []SessionRun        `json:"runs"`
	Diagnostics []SessionDiagnostic `json:"diagnostics"`
}

type IntegrationSessionEvent struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`
	SessionID string              `json:"sessionId"`
	RunID     *string             `json:"runId,omitempty"`
	Message   string              `json:"message"`
	Timestamp time.Time           `json:"timestamp"`
	Session   *IntegrationSession `json:"session,omitempty"`
	Run       *SessionRun         `json:"run,omitempty"`
}

type CreateIntegrationSessionInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type AddSessionSampleInput struct {
	SessionID        string       `json:"sessionId"`
	Name             string       `json:"name"`
	Format           SourceFormat `json:"format"`
	Data             string       `json:"data"`
	Source           *string      `json:"source,omitempty"`
	RetainRawPayload *bool        `json:"retainRawPayload,omitempty"`
	PayloadRef       *string      `json:"payloadRef,omitempty"`
}

type UpdateSessionArtifactInput struct {
	SessionID string  `json:"sessionId"`
	Name      *string `json:"name,omitempty"`
	Content   string  `json:"content"`
}

type RunSessionPreviewInput struct {
	SessionID string        `json:"sessionId"`
	SampleID  *string       `json:"sampleId,omitempty"`
	Data      *string       `json:"data,omitempty"`
	Format    *SourceFormat `json:"format,omitempty"`
	Source    *string       `json:"source,omitempty"`
}

type AcceptDiagnosticFixInput struct {
	SessionID    string  `json:"sessionId"`
	DiagnosticID string  `json:"diagnosticId"`
	AcceptedBy   *string `json:"acceptedBy,omitempty"`
}

type ExportIntegrationBundleInput struct {
	SessionID         string `json:"sessionId"`
	IncludeRawPayload *bool  `json:"includeRawPayload,omitempty"`
}
