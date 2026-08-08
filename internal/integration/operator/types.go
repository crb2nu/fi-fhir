// Package operator exposes the durable engine records an operator needs to
// complete the failure/replay and operator-audit golden journeys without SQL
// or filesystem access. It owns no delivery or lifecycle state of its own: it
// reads the Slice 2.3 delivery tables and the Slice 2.1 lifecycle catalog, and
// it delegates every write to their existing idempotent, append-only machinery.
package operator

import (
	"errors"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// ReadRole authorizes the bounded, PHI-minimal control-plane read surface.
	ReadRole = "integration.operator"
	// DeploymentOperatorRole authorizes lifecycle pause/resume/retire/deploy.
	DeploymentOperatorRole = "integration.deployment.operator"

	// DefaultPageSize bounds an unspecified page request.
	DefaultPageSize = 25
	// MaxPageSize is the hard server-side ceiling for any control-plane page.
	MaxPageSize = 100
	// MaxPayloadFields bounds one event's structural payload summary.
	MaxPayloadFields = 200
	// maxLifecycleSnapshots bounds the deployment inventory read.
	maxLifecycleSnapshots = 200
)

var (
	// ErrUnavailable means the durable control plane is not configured.
	ErrUnavailable = errors.New("operator control plane unavailable")
	// ErrUnauthenticated means no verified caller identity reached the service.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden means the verified caller lacks the required operator role.
	ErrForbidden = errors.New("operator control-plane action forbidden")
	// ErrInvalidRequest means a filter, cursor, page size, or reason is invalid.
	ErrInvalidRequest = errors.New("invalid operator control-plane request")
	// ErrNotFound hides control-plane inventory from unauthorized or wrong-tenant
	// callers. Cross-tenant reads and writes are indistinguishable from absence.
	ErrNotFound = errors.New("operator control-plane record not found")
)

// PrincipalSummary is the catalog-safe projection of a recorded actor.
type PrincipalSummary struct {
	ID         string
	Kind       string
	AuthMethod string
	Roles      []string
}

func summarizePrincipal(principal integration.Principal) PrincipalSummary {
	return PrincipalSummary{
		ID:         principal.ID,
		Kind:       string(principal.Kind),
		AuthMethod: principal.AuthMethod,
		Roles:      append([]string(nil), principal.Roles...),
	}
}

// ReceiptSummary is one durable admission record without its raw envelope,
// request fingerprint, or execution result document.
type ReceiptSummary struct {
	TenantID            string
	ReceiptID           string
	Status              string
	RecordedAt          time.Time
	CorrelationID       string
	RawRetentionMode    string
	IntegrationRevision integration.ArtifactRevisionRef
	Principal           PrincipalSummary
	Reason              string
	EventCount          int
	AttemptCount        int
	FailedAttemptCount  int
	DeadLetterCount     int
}

// PayloadField is one structural coordinate of a canonical event payload.
// It carries the shape of the document and never a stored value.
type PayloadField struct {
	Path     string
	Kind     string
	Repeated bool
}

// EventSummary renders a canonical event semantically: its type, its
// patient-safe correlation identifiers, and the structure of its payload.
type EventSummary struct {
	TenantID         string
	EventID          string
	ReceiptID        string
	EventType        string
	SourceMessageID  string
	CorrelationID    string
	Classification   string
	RecordedAt       time.Time
	PayloadFields    []PayloadField
	PayloadTruncated bool
}

// RouteSummary is one persisted routing outcome for an event.
type RouteSummary struct {
	Route           string
	Matched         bool
	Skipped         bool
	SkipReason      string
	TransformCount  int
	PlannedActions  []string
	DiagnosticCodes []string
}

// DiagnosticSummary is one persisted, message-free diagnostic classification.
type DiagnosticSummary struct {
	Severity       string
	Stage          string
	Code           string
	Path           string
	Classification string
}

// LineageSummary links one receipt and event to the exact artifact revisions
// that produced it, plus the routes and diagnostics recorded at admission.
type LineageSummary struct {
	LineageID         string
	ReceiptID         string
	EventID           string
	TraceID           string
	CorrelationID     string
	SourceMessageID   string
	ArtifactRevisions integration.ExecutionArtifactRevisions
	Routes            []RouteSummary
	Diagnostics       []DiagnosticSummary
	RecordedAt        time.Time
}

// DeliveryAttemptSummary joins one attempt with its outbox and DLQ state.
type DeliveryAttemptSummary struct {
	TenantID        string
	AttemptID       string
	ParentAttemptID string
	ReceiptID       string
	EventID         string
	TraceID         string
	Destination     integration.DestinationRevisionRef
	Route           string
	Action          string
	Status          string
	AttemptCount    int
	RecordedAt      time.Time
	ScheduledAt     time.Time
	CompletedAt     *time.Time
	LastErrorCode   string
	LastErrorDetail string
	OutboxStatus    string
	Topic           string
	LeaseOwner      string
	LeaseExpiresAt  *time.Time
	DeadLetter      *DeadLetterSummary
}

// DeadLetterSummary is one durable dead-letter record and its resolution.
type DeadLetterSummary struct {
	AttemptID      string
	Active         bool
	FailureCode    string
	FailureDetail  string
	FailedAt       time.Time
	ReplayCount    int
	LastReplayedAt *time.Time
	Resolution     string
	ResolvedAt     *time.Time
}

// CircuitSummary is one destination circuit-breaker state.
type CircuitSummary struct {
	Destination integration.ArtifactRevisionRef
	State       string
	Failures    int
	OpenUntil   *time.Time
	UpdatedAt   time.Time
}

// AuditSummary is one append-only delivery audit record.
type AuditSummary struct {
	AuditID      int64
	AttemptID    string
	EventKind    string
	AttemptCount int
	Principal    PrincipalSummary
	Reason       string
	Detail       map[string]any
	RecordedAt   time.Time
}

// MessageTrace is the full receipt-to-delivery lineage for one message.
type MessageTrace struct {
	Receipt  ReceiptSummary
	Events   []EventSummary
	Lineage  []LineageSummary
	Attempts []DeliveryAttemptSummary
	Audit    []AuditSummary
}

// DeploymentSummary is one lifecycle snapshot projected for operator controls.
type DeploymentSummary struct {
	DefinitionRevision  integration.ArtifactRevisionRef
	State               string
	Version             int64
	ReleaseID           string
	Health              string
	ValidationPassed    bool
	ValidationExpiresAt *time.Time
	UpdatedBy           PrincipalSummary
	UpdatedReason       string
	UpdatedAt           time.Time
}

// LifecycleEventSummary is one append-only lifecycle transition.
type LifecycleEventSummary struct {
	EventID    string
	Version    int64
	Action     string
	FromState  string
	ToState    string
	Health     string
	ReleaseID  string
	Actor      PrincipalSummary
	Reason     string
	OccurredAt time.Time
}

// Page is one bounded result window with an opaque forward cursor.
type Page[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

// ReceiptFilter bounds a receipt browse request.
type ReceiptFilter struct {
	Status                string
	IntegrationArtifactID string
	CorrelationID         string
	SourceMessageID       string
	From                  *time.Time
	To                    *time.Time
}

// AttemptFilter bounds a delivery attempt browse request.
type AttemptFilter struct {
	Status                string
	DestinationArtifactID string
	ReceiptID             string
	Route                 string
	From                  *time.Time
	To                    *time.Time
}

// PageRequest carries the caller's bounded window.
type PageRequest struct {
	First  int
	Cursor string
}

// ControlRequest is one reason-required, idempotent delivery control action.
type ControlRequest struct {
	AttemptID      string
	Reason         string
	IdempotencyKey string
}

// DeploymentCommand is one reason-required, expected-version lifecycle action.
type DeploymentCommand struct {
	DefinitionID    string
	RevisionID      string
	ExpectedVersion int64
	Reason          string
}

// ControlResult reports the durable outcome of one control action.
type ControlResult struct {
	Kind            string
	SourceAttemptID string
	ResultAttemptID string
	Attempt         DeliveryAttemptSummary
	Reason          string
	IdempotencyKey  string
	Actor           PrincipalSummary
}
