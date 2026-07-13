package model

// PreviewIntegrationMessageInput deliberately excludes all trusted binding and
// identity fields; the server supplies those from registry and auth context.
type PreviewIntegrationMessageInput struct {
	IntegrationID string `json:"integrationId"`
	Data          string `json:"data"`
	CorrelationID string `json:"correlationId"`
	Reason        string `json:"reason"`
}

type IntegrationArtifactRevision struct {
	ArtifactID string `json:"artifactId"`
	RevisionID string `json:"revisionId"`
	Digest     string `json:"digest"`
}

type IntegrationExecutionArtifactRevisions struct {
	Source   IntegrationArtifactRevision `json:"source"`
	Profile  IntegrationArtifactRevision `json:"profile"`
	Workflow IntegrationArtifactRevision `json:"workflow"`
}

type IntegrationPreviewEvent struct {
	TenantID        string         `json:"tenantId"`
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	SourceMessageID *string        `json:"sourceMessageId,omitempty"`
	CorrelationID   string         `json:"correlationId"`
	Classification  string         `json:"classification"`
	Payload         map[string]any `json:"payload"`
}

type IntegrationPreviewDiagnostic struct {
	TenantID       string  `json:"tenantId"`
	Severity       string  `json:"severity"`
	Stage          string  `json:"stage"`
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	Path           *string `json:"path,omitempty"`
	Source         *string `json:"source,omitempty"`
	Classification string  `json:"classification"`
}

type IntegrationPreviewRoute struct {
	TenantID        string   `json:"tenantId"`
	EventID         string   `json:"eventId"`
	Route           string   `json:"route"`
	Matched         bool     `json:"matched"`
	Skipped         bool     `json:"skipped"`
	SkipReason      *string  `json:"skipReason,omitempty"`
	TransformCount  int      `json:"transformCount"`
	PlannedActions  []string `json:"plannedActions"`
	DiagnosticCodes []string `json:"diagnosticCodes"`
}

type IntegrationPreviewDestination struct {
	ArtifactID string `json:"artifactId"`
	RevisionID string `json:"revisionId"`
	Digest     string `json:"digest"`
	Class      string `json:"class"`
}

type IntegrationPreviewDelivery struct {
	TenantID        string                        `json:"tenantId"`
	EventID         string                        `json:"eventId"`
	Destination     IntegrationPreviewDestination `json:"destination"`
	Route           string                        `json:"route"`
	Action          string                        `json:"action"`
	Status          string                        `json:"status"`
	DiagnosticCodes []string                      `json:"diagnosticCodes"`
}

type IntegrationPreviewCorrelations struct {
	TenantID        string   `json:"tenantId"`
	CorrelationID   string   `json:"correlationId"`
	TraceID         *string  `json:"traceId,omitempty"`
	SourceMessageID *string  `json:"sourceMessageId,omitempty"`
	EventIDs        []string `json:"eventIds"`
	WorkflowRunID   *string  `json:"workflowRunId,omitempty"`
}

type IntegrationPreviewResult struct {
	Mode                string                                `json:"mode"`
	TenantID            string                                `json:"tenantId"`
	IntegrationRevision IntegrationArtifactRevision           `json:"integrationRevision"`
	ArtifactRevisions   IntegrationExecutionArtifactRevisions `json:"artifactRevisions"`
	Events              []IntegrationPreviewEvent             `json:"events"`
	Diagnostics         []IntegrationPreviewDiagnostic        `json:"diagnostics"`
	Routes              []IntegrationPreviewRoute             `json:"routes"`
	Deliveries          []IntegrationPreviewDelivery          `json:"deliveries"`
	Correlations        IntegrationPreviewCorrelations        `json:"correlations"`
}
