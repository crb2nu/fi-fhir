package session

import (
	"encoding/json"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
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
	ID         string       `json:"id"`
	RevisionID string       `json:"revision_id"`
	SessionID  string       `json:"session_id"`
	Kind       ArtifactKind `json:"kind"`
	Name       string       `json:"name"`
	Content    []byte       `json:"content"`
	Version    int          `json:"version"`
	Digest     string       `json:"digest"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type SaveArtifactDraftRequest struct {
	ID      string
	Kind    ArtifactKind
	Name    string
	Content []byte
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
	ID           string               `json:"id"`
	Session      Session              `json:"session"`
	Samples      []Sample             `json:"samples,omitempty"`
	Drafts       []ArtifactDraft      `json:"drafts,omitempty"`
	Runs         []Run                `json:"runs,omitempty"`
	Simulations  []WorkflowSimulation `json:"workflow_simulations,omitempty"`
	Publications []Publication        `json:"publications,omitempty"`
	Decisions    []Decision           `json:"decisions,omitempty"`
	ExportedAt   time.Time            `json:"exported_at"`
}

// Publication is immutable evidence that exact tested session revisions match
// one validated production definition. Manifest bytes are signed separately and
// must be verified before any lifecycle transition.
type Publication struct {
	ID                     string                          `json:"id"`
	SessionID              string                          `json:"session_id"`
	Version                int                             `json:"version"`
	ProfileArtifactID      string                          `json:"profile_artifact_id"`
	ProfileRevisionID      string                          `json:"profile_revision_id"`
	ProfileRevisionDigest  string                          `json:"profile_revision_digest"`
	WorkflowArtifactID     string                          `json:"workflow_artifact_id"`
	WorkflowRevisionID     string                          `json:"workflow_revision_id"`
	WorkflowRevisionDigest string                          `json:"workflow_revision_digest"`
	WorkflowSimulationID   string                          `json:"workflow_simulation_id"`
	DefinitionRevision     integration.ArtifactRevisionRef `json:"definition_revision"`
	DefinitionVersion      int64                           `json:"definition_version"`
	ProductionProfile      integration.ArtifactRevisionRef `json:"production_profile"`
	ProductionWorkflow     integration.ArtifactRevisionRef `json:"production_workflow"`
	SourceRunIDs           []string                        `json:"source_run_ids"`
	Manifest               []byte                          `json:"manifest"`
	ManifestDigest         string                          `json:"manifest_digest"`
	Signature              []byte                          `json:"signature"`
	SignatureAlgorithm     string                          `json:"signature_algorithm"`
	SigningKeyID           string                          `json:"signing_key_id"`
	PublishedBy            string                          `json:"published_by"`
	Reason                 string                          `json:"reason"`
	CreatedAt              time.Time                       `json:"created_at"`
}

// PublicationManifest is the bounded canonical document covered by a detached
// signature. It intentionally excludes raw/event payloads and action config.
type PublicationManifest struct {
	SchemaVersion         string                          `json:"schema_version"`
	PublicationID         string                          `json:"publication_id"`
	SessionID             string                          `json:"session_id"`
	SessionProfile        integration.ArtifactRevisionRef `json:"session_profile"`
	SessionWorkflow       integration.ArtifactRevisionRef `json:"session_workflow"`
	WorkflowSimulationID  string                          `json:"workflow_simulation_id"`
	DefinitionRevision    integration.ArtifactRevisionRef `json:"definition_revision"`
	DefinitionVersion     int64                           `json:"definition_version"`
	ProductionProfile     integration.ArtifactRevisionRef `json:"production_profile"`
	ProductionWorkflow    integration.ArtifactRevisionRef `json:"production_workflow"`
	Fixtures              []PublicationFixture            `json:"fixtures"`
	ExpectedMatchedRoutes []string                        `json:"expected_matched_routes,omitempty"`
	ExpectedTransforms    []string                        `json:"expected_transforms,omitempty"`
	ExpectedActions       []string                        `json:"expected_actions,omitempty"`
	PublishedBy           string                          `json:"published_by"`
	Reason                string                          `json:"reason"`
	CreatedAt             time.Time                       `json:"created_at"`
}

type PublicationFixture struct {
	RunID                   string              `json:"run_id"`
	SampleID                string              `json:"sample_id"`
	SampleFormat            events.SourceFormat `json:"sample_format"`
	SampleDigest            string              `json:"sample_digest"`
	ExpectedEventTypes      []string            `json:"expected_event_types"`
	ExpectedDiagnosticCodes []string            `json:"expected_diagnostic_codes,omitempty"`
}

type CreatePublicationRequest struct {
	ID                     string
	ProfileArtifactID      string
	ProfileRevisionID      string
	ProfileRevisionDigest  string
	WorkflowArtifactID     string
	WorkflowRevisionID     string
	WorkflowRevisionDigest string
	WorkflowSimulationID   string
	DefinitionRevision     integration.ArtifactRevisionRef
	DefinitionVersion      int64
	ProductionProfile      integration.ArtifactRevisionRef
	ProductionWorkflow     integration.ArtifactRevisionRef
	SourceRunIDs           []string
	Manifest               []byte
	ManifestDigest         string
	Signature              []byte
	SignatureAlgorithm     string
	SigningKeyID           string
	PublishedBy            string
	Reason                 string
	CreatedAt              time.Time
}

type PublishRequest struct {
	SessionID            string
	ProfileRevisionID    string
	WorkflowSimulationID string
	DefinitionID         string
	DefinitionRevisionID string
	PublishedBy          string
	Reason               string
}

type PromotePublicationRequest struct {
	SessionID       string
	PublicationID   string
	ExpectedVersion int64
	Actor           integration.Principal
	Reason          string
}

// WorkflowSimulation is immutable, PHI-minimal evidence that one exact session
// workflow revision was planned against an explicit set of immutable parse runs.
type WorkflowSimulation struct {
	ID                     string               `json:"id"`
	SessionID              string               `json:"session_id"`
	WorkflowArtifactID     string               `json:"workflow_artifact_id"`
	WorkflowRevisionID     string               `json:"workflow_revision_id"`
	WorkflowRevisionDigest string               `json:"workflow_revision_digest"`
	SourceRunIDs           []string             `json:"source_run_ids"`
	Events                 []WorkflowEventTrace `json:"events"`
	CreatedAt              time.Time            `json:"created_at"`
}

type WorkflowEventTrace struct {
	RunID     string               `json:"run_id"`
	EventID   string               `json:"event_id"`
	EventType string               `json:"event_type"`
	Routes    []WorkflowRouteTrace `json:"routes"`
}

type WorkflowRouteTrace struct {
	Name            string                   `json:"name"`
	Matched         bool                     `json:"matched"`
	SkipReason      string                   `json:"skip_reason,omitempty"`
	DiagnosticCodes []string                 `json:"diagnostic_codes,omitempty"`
	Transforms      []WorkflowTransformTrace `json:"transforms,omitempty"`
	Actions         []WorkflowActionTrace    `json:"actions,omitempty"`
}

type WorkflowTransformTrace struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type WorkflowActionTrace struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	DestinationArtifactID string `json:"destination_artifact_id,omitempty"`
}

type CreateWorkflowSimulationRequest struct {
	WorkflowArtifactID     string
	WorkflowRevisionID     string
	WorkflowRevisionDigest string
	SourceRunIDs           []string
	Events                 []WorkflowEventTrace
}

type SimulateWorkflowRequest struct {
	SessionID          string
	WorkflowRevisionID string
	SourceRunIDs       []string
}

type WorkflowSimulationDelta struct {
	BaselineSimulationID  string   `json:"baseline_simulation_id"`
	CandidateSimulationID string   `json:"candidate_simulation_id"`
	AddedEvents           []string `json:"added_events,omitempty"`
	RemovedEvents         []string `json:"removed_events,omitempty"`
	AddedMatchedRoutes    []string `json:"added_matched_routes,omitempty"`
	RemovedMatchedRoutes  []string `json:"removed_matched_routes,omitempty"`
	AddedTransforms       []string `json:"added_transforms,omitempty"`
	RemovedTransforms     []string `json:"removed_transforms,omitempty"`
	AddedActions          []string `json:"added_actions,omitempty"`
	RemovedActions        []string `json:"removed_actions,omitempty"`
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
