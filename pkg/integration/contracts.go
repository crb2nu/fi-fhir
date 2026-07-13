package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// ExecutionMode selects production delivery or side-effect-safe preview behavior.
type ExecutionMode string

const (
	ExecutionModeProduction ExecutionMode = "production"
	ExecutionModePreview    ExecutionMode = "preview"
)

// AllowsDelivery reports whether a destination class is safe in this mode.
func (m ExecutionMode) AllowsDelivery(class DestinationClass) bool {
	switch m {
	case ExecutionModeProduction:
		return class == DestinationClassProduction || class == DestinationClassSandbox
	default:
		return false
	}
}

// RawEnvelopeMetadata describes source bytes without containing them.
type RawEnvelopeMetadata struct {
	TenantID       string
	SourceID       string
	Format         events.SourceFormat
	ContentType    string
	ReceivedAt     time.Time
	Classification DataClassification
}

// RawEnvelope carries source bytes in memory while exposing only safe metadata to JSON.
type RawEnvelope struct {
	TenantID       string              `json:"tenant_id"`
	SourceID       string              `json:"source_id"`
	Format         events.SourceFormat `json:"format"`
	ContentType    string              `json:"content_type,omitempty"`
	ReceivedAt     time.Time           `json:"received_at"`
	Classification DataClassification  `json:"classification"`
	PayloadDigest  string              `json:"payload_digest"`
	SizeBytes      int64               `json:"size_bytes"`
	payload        []byte
}

// NewRawEnvelope copies source bytes and records their content digest.
func NewRawEnvelope(metadata RawEnvelopeMetadata, payload []byte) (RawEnvelope, error) {
	bytes := append([]byte(nil), payload...)
	sum := sha256.Sum256(bytes)
	envelope := RawEnvelope{
		TenantID:       metadata.TenantID,
		SourceID:       metadata.SourceID,
		Format:         metadata.Format,
		ContentType:    metadata.ContentType,
		ReceivedAt:     metadata.ReceivedAt,
		Classification: metadata.Classification,
		PayloadDigest:  "sha256:" + hex.EncodeToString(sum[:]),
		SizeBytes:      int64(len(bytes)),
		payload:        bytes,
	}
	if !envelope.ReceivedAt.IsZero() {
		envelope.ReceivedAt = envelope.ReceivedAt.UTC()
	}
	if err := envelope.Validate(); err != nil {
		return RawEnvelope{}, err
	}
	return envelope, nil
}

// Bytes returns a defensive copy of the non-serializable source payload.
func (e RawEnvelope) Bytes() []byte {
	return append([]byte(nil), e.payload...)
}

// PayloadSizeBytes returns the actual in-memory source length without copying it.
// Callers enforcing transport or processor limits must not trust mutable wire metadata.
func (e RawEnvelope) PayloadSizeBytes() int64 {
	return int64(len(e.payload))
}

// Validate verifies source metadata and the in-memory payload checksum.
func (e RawEnvelope) Validate() error {
	v := &validationCollector{}
	v.add(strings.TrimSpace(e.TenantID) != "", "REQUIRED", "tenant_id", "tenant ID is required")
	v.add(strings.TrimSpace(e.SourceID) != "", "REQUIRED", "source_id", "source ID is required")
	validateSourceFormat("format", e.Format, v)
	v.add(!e.ReceivedAt.IsZero(), "REQUIRED", "received_at", "received timestamp is required")
	v.add(e.Classification == DataClassificationPHI, "INVALID_CLASSIFICATION", "classification", "data classification must be phi")
	v.add(len(e.payload) > 0, "REQUIRED", "payload", "source payload is required")
	v.add(e.SizeBytes == int64(len(e.payload)), "SIZE_MISMATCH", "size_bytes", "payload size does not match source bytes")
	v.add(sha256DigestPattern.MatchString(e.PayloadDigest), "INVALID_DIGEST", "payload_digest", "payload digest must be sha256 followed by 64 lowercase hexadecimal characters")
	if len(e.payload) > 0 && sha256DigestPattern.MatchString(e.PayloadDigest) {
		sum := sha256.Sum256(e.payload)
		expected := "sha256:" + hex.EncodeToString(sum[:])
		v.add(e.PayloadDigest == expected, "DIGEST_MISMATCH", "payload_digest", "payload digest does not match source bytes")
	}
	return v.err()
}

// SecurityContext binds an authenticated principal to one tenant.
type SecurityContext struct {
	TenantID  string    `json:"tenant_id"`
	Principal Principal `json:"principal"`
	Reason    string    `json:"reason,omitempty"`
}

// ProcessRequest is the stable input boundary for production and preview processing.
type ProcessRequest struct {
	Mode                ExecutionMode       `json:"mode"`
	IntegrationRevision ArtifactRevisionRef `json:"integration_revision"`
	Security            SecurityContext     `json:"security"`
	Envelope            RawEnvelope         `json:"envelope"`
	IdempotencyKey      string              `json:"idempotency_key,omitempty"`
	CorrelationID       string              `json:"correlation_id"`
}

// ValidateAgainst proves the request resolves to the exact bound integration revision.
func (r ProcessRequest) ValidateAgainst(revision IntegrationDefinitionRevision) error {
	v := &validationCollector{}
	v.add(r.Mode == ExecutionModeProduction || r.Mode == ExecutionModePreview, "INVALID_MODE", "mode", "execution mode must be production or preview")
	v.merge("revision", revision.Validate())
	validateArtifactRevision("integration_revision", r.IntegrationRevision, v)
	v.add(r.IntegrationRevision == revision.Reference(), "REVISION_MISMATCH", "integration_revision", "request revision does not match the resolved integration revision")
	v.add(strings.TrimSpace(r.Security.TenantID) != "", "REQUIRED", "security.tenant_id", "security tenant ID is required")
	v.add(r.Security.TenantID == revision.TenantID, "TENANT_MISMATCH", "security.tenant_id", "security tenant must match integration revision tenant")
	validatePrincipal("security.principal", r.Security.Principal, v)
	if r.Security.Principal.Kind == PrincipalKindHuman {
		v.add(strings.TrimSpace(r.Security.Reason) != "", "REQUIRED", "security.reason", "human operations require a reason")
	}
	v.merge("envelope", r.Envelope.Validate())
	v.add(r.Envelope.TenantID == revision.TenantID, "TENANT_MISMATCH", "envelope.tenant_id", "envelope tenant must match integration revision tenant")
	v.add(r.Envelope.SourceID == revision.Source.SourceID, "SOURCE_MISMATCH", "envelope.source_id", "envelope source must match integration revision source")
	v.add(r.Envelope.Format == revision.Format, "FORMAT_MISMATCH", "envelope.format", "envelope format must match integration revision format")
	v.add(r.Envelope.Classification == revision.Policy.Classification, "CLASSIFICATION_MISMATCH", "envelope.classification", "envelope classification must match integration policy")
	if r.Security.Principal.Kind == PrincipalKindService {
		v.add(r.Security.Principal.SourceID == revision.Source.SourceID, "SOURCE_MISMATCH", "security.principal.source_id", "service principal source must match integration revision source")
	}
	v.add(strings.TrimSpace(r.CorrelationID) != "", "REQUIRED", "correlation_id", "correlation ID is required")
	if r.IdempotencyKey != "" {
		v.add(strings.TrimSpace(r.IdempotencyKey) != "", "INVALID_IDEMPOTENCY_KEY", "idempotency_key", "explicit idempotency key cannot be whitespace")
	}
	return v.err()
}

// ReceiptStatus records whether an ingress request was durably accepted or rejected.
type ReceiptStatus string

const (
	ReceiptStatusAccepted ReceiptStatus = "accepted"
	ReceiptStatusRejected ReceiptStatus = "rejected"
)

// DeliveryStatus records the durable lifecycle of one destination attempt.
type DeliveryStatus string

const (
	DeliveryStatusPlanned    DeliveryStatus = "planned"
	DeliveryStatusSuppressed DeliveryStatus = "suppressed"
	DeliveryStatusQueued     DeliveryStatus = "queued"
	DeliveryStatusSucceeded  DeliveryStatus = "succeeded"
	DeliveryStatusFailed     DeliveryStatus = "failed"
)

// DiagnosticSeverity is a stable, serializable diagnostic level.
type DiagnosticSeverity string

const (
	DiagnosticSeverityInfo    DiagnosticSeverity = "info"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
	DiagnosticSeverityError   DiagnosticSeverity = "error"
)

// Receipt is the durable admission record for a production request.
type Receipt struct {
	ID                  string              `json:"id"`
	TenantID            string              `json:"tenant_id"`
	IntegrationRevision ArtifactRevisionRef `json:"integration_revision"`
	Status              ReceiptStatus       `json:"status"`
	IdempotencyKey      string              `json:"idempotency_key,omitempty"`
	RecordedAt          time.Time           `json:"recorded_at"`
	CorrelationID       string              `json:"correlation_id"`
	RawRetentionMode    RawRetentionMode    `json:"raw_retention_mode"`
	RawExpiresAt        *time.Time          `json:"raw_expires_at,omitempty"`
	Principal           Principal           `json:"principal"`
	Reason              string              `json:"reason,omitempty"`
}

// ProcessedEventMetadata supplies security-domain facts absent from canonical events.
type ProcessedEventMetadata struct {
	TenantID       string
	Classification DataClassification
}

// ProcessedEvent is a package-controlled, raw-free canonical event projection.
type ProcessedEvent struct {
	TenantID        string             `json:"tenant_id"`
	ID              string             `json:"id"`
	Type            events.EventType   `json:"type"`
	SourceMessageID string             `json:"source_message_id,omitempty"`
	CorrelationID   string             `json:"correlation_id"`
	Classification  DataClassification `json:"classification"`
	payload         json.RawMessage
}

// NewProcessedEvent projects a concrete pkg/events event and strips raw source fields.
func NewProcessedEvent(metadata ProcessedEventMetadata, canonicalEvent any) (ProcessedEvent, error) {
	concreteType, err := concreteCanonicalEventType(canonicalEvent)
	if err != nil {
		return ProcessedEvent{}, err
	}
	projection := redactedCanonicalEventCopy(canonicalEvent, concreteType)
	encoded, err := json.Marshal(projection)
	if err != nil {
		return ProcessedEvent{}, fmt.Errorf("marshal canonical %s: %w", concreteType.Name(), err)
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return ProcessedEvent{}, fmt.Errorf("decode canonical %s projection: %w", concreteType.Name(), err)
	}
	removeRawPayloadFields(payload)
	sanitized, err := json.Marshal(payload)
	if err != nil {
		return ProcessedEvent{}, fmt.Errorf("marshal sanitized canonical %s: %w", concreteType.Name(), err)
	}

	id, _ := payload["id"].(string)
	typeName, _ := payload["type"].(string)
	sourceMessageID, _ := payload["source_message_id"].(string)
	correlationID, _ := payload["correlation_id"].(string)
	processed := ProcessedEvent{
		TenantID:        metadata.TenantID,
		ID:              id,
		Type:            events.EventType(typeName),
		SourceMessageID: sourceMessageID,
		CorrelationID:   correlationID,
		Classification:  metadata.Classification,
		payload:         sanitized,
	}
	expectedType, registered := canonicalEventRegistry[processed.Type]
	if !registered || expectedType != concreteType {
		return ProcessedEvent{}, fmt.Errorf("canonical %s cannot represent event type %q", concreteType.Name(), processed.Type)
	}
	v := &validationCollector{}
	validateProcessedEvent("event", processed, v)
	if err := v.err(); err != nil {
		return ProcessedEvent{}, err
	}
	return processed, nil
}

// PayloadJSON returns a defensive copy of the sanitized canonical event JSON.
func (e ProcessedEvent) PayloadJSON() json.RawMessage {
	return append(json.RawMessage(nil), e.payload...)
}

// MarshalJSON exposes the sanitized payload while keeping construction sealed.
func (e ProcessedEvent) MarshalJSON() ([]byte, error) {
	v := &validationCollector{}
	validateProcessedEvent("event", e, v)
	if err := v.err(); err != nil {
		return nil, err
	}
	return json.Marshal(processedEventWire{
		TenantID:        e.TenantID,
		ID:              e.ID,
		Type:            e.Type,
		SourceMessageID: e.SourceMessageID,
		CorrelationID:   e.CorrelationID,
		Classification:  e.Classification,
		Payload:         e.payload,
	})
}

// UnmarshalJSON preserves round-trip support while enforcing the raw-free wire shape.
func (e *ProcessedEvent) UnmarshalJSON(data []byte) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("decode processed event: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire processedEventWire
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode processed event: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode processed event: %w", err)
	}
	if err := validateCanonicalJSONKeys(data, wire); err != nil {
		return fmt.Errorf("decode processed event: %w", err)
	}
	canonicalEvent, err := decodeCanonicalEventPayload(wire.Type, wire.Payload)
	if err != nil {
		return fmt.Errorf("decode processed event: %w", err)
	}
	projected, err := NewProcessedEvent(ProcessedEventMetadata{
		TenantID:       wire.TenantID,
		Classification: wire.Classification,
	}, canonicalEvent)
	if err != nil {
		return err
	}
	if projected.ID != wire.ID || projected.Type != wire.Type || projected.SourceMessageID != wire.SourceMessageID || projected.CorrelationID != wire.CorrelationID {
		return fmt.Errorf("decode processed event: wrapper metadata does not match canonical payload")
	}
	*e = projected
	return nil
}

type processedEventWire struct {
	TenantID        string             `json:"tenant_id"`
	ID              string             `json:"id"`
	Type            events.EventType   `json:"type"`
	SourceMessageID string             `json:"source_message_id,omitempty"`
	CorrelationID   string             `json:"correlation_id"`
	Classification  DataClassification `json:"classification"`
	Payload         json.RawMessage    `json:"payload"`
}

// DiagnosticInput supplies a redacted processing diagnostic.
type DiagnosticInput struct {
	TenantID       string
	Severity       DiagnosticSeverity
	Stage          string
	Code           string
	Path           string
	Source         string
	Classification DataClassification
}

// Diagnostic is the PHI-classified, raw-free processing diagnostic contract.
type Diagnostic struct {
	TenantID       string             `json:"tenant_id"`
	Severity       DiagnosticSeverity `json:"severity"`
	Stage          string             `json:"stage"`
	Code           string             `json:"code"`
	Path           string             `json:"path,omitempty"`
	Classification DataClassification `json:"classification"`
	message        string
	source         string
}

// NewDiagnostic validates that diagnostic text is bounded and not a raw message.
func NewDiagnostic(input DiagnosticInput) (Diagnostic, error) {
	diagnostic := Diagnostic{
		TenantID:       input.TenantID,
		Severity:       input.Severity,
		Stage:          input.Stage,
		Code:           input.Code,
		Path:           input.Path,
		Classification: input.Classification,
		message:        diagnosticMessage(input.Code),
		source:         input.Source,
	}
	v := &validationCollector{}
	validateDiagnostic("diagnostic", diagnostic, v)
	if err := v.err(); err != nil {
		return Diagnostic{}, err
	}
	return diagnostic, nil
}

// Message returns the validated diagnostic explanation.
func (d Diagnostic) Message() string {
	return d.message
}

// Source returns the optional bounded subsystem identifier.
func (d Diagnostic) Source() string {
	return d.source
}

// MarshalJSON exposes only validated diagnostic fields.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	v := &validationCollector{}
	validateDiagnostic("diagnostic", d, v)
	if err := v.err(); err != nil {
		return nil, err
	}
	return json.Marshal(diagnosticWire{
		TenantID:       d.TenantID,
		Severity:       d.Severity,
		Stage:          d.Stage,
		Code:           d.Code,
		Message:        d.message,
		Path:           d.Path,
		Source:         d.source,
		Classification: d.Classification,
	})
}

// UnmarshalJSON applies the same redaction boundary to round-tripped diagnostics.
func (d *Diagnostic) UnmarshalJSON(data []byte) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("decode diagnostic: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire diagnosticWire
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode diagnostic: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("decode diagnostic: %w", err)
	}
	if err := validateCanonicalJSONKeys(data, wire); err != nil {
		return fmt.Errorf("decode diagnostic: %w", err)
	}
	candidate, err := NewDiagnostic(DiagnosticInput{
		TenantID:       wire.TenantID,
		Severity:       wire.Severity,
		Stage:          wire.Stage,
		Code:           wire.Code,
		Path:           wire.Path,
		Source:         wire.Source,
		Classification: wire.Classification,
	})
	if err != nil {
		return err
	}
	if wire.Message != candidate.message {
		return fmt.Errorf("decode diagnostic: message does not match code %q", wire.Code)
	}
	*d = candidate
	return nil
}

type diagnosticWire struct {
	TenantID       string             `json:"tenant_id"`
	Severity       DiagnosticSeverity `json:"severity"`
	Stage          string             `json:"stage"`
	Code           string             `json:"code"`
	Message        string             `json:"message"`
	Path           string             `json:"path,omitempty"`
	Source         string             `json:"source,omitempty"`
	Classification DataClassification `json:"classification"`
}

// RouteResult is the deterministic route plan produced before delivery.
// It deliberately contains no Go errors or execution-only destination state.
type RouteResult struct {
	TenantID        string   `json:"tenant_id"`
	EventID         string   `json:"event_id,omitempty"`
	Route           string   `json:"route"`
	Matched         bool     `json:"matched"`
	Skipped         bool     `json:"skipped,omitempty"`
	SkipReason      string   `json:"skip_reason,omitempty"`
	TransformCount  int      `json:"transform_count"`
	PlannedActions  []string `json:"planned_actions,omitempty"`
	DiagnosticCodes []string `json:"diagnostic_codes,omitempty"`
}

// DeliveryResult is the stable outcome for one route/action destination attempt.
type DeliveryResult struct {
	TenantID        string                 `json:"tenant_id"`
	EventID         string                 `json:"event_id,omitempty"`
	Destination     DestinationRevisionRef `json:"destination"`
	Route           string                 `json:"route,omitempty"`
	Action          string                 `json:"action,omitempty"`
	Status          DeliveryStatus         `json:"status"`
	AttemptID       string                 `json:"attempt_id,omitempty"`
	AttemptCount    int                    `json:"attempt_count,omitempty"`
	DiagnosticCodes []string               `json:"diagnostic_codes,omitempty"`
}

// CorrelationIDs preserves distinct identifiers across ingress, events, traces, and delivery.
type CorrelationIDs struct {
	TenantID           string   `json:"tenant_id"`
	CorrelationID      string   `json:"correlation_id"`
	TraceID            string   `json:"trace_id,omitempty"`
	SourceMessageID    string   `json:"source_message_id,omitempty"`
	ReceiptID          string   `json:"receipt_id,omitempty"`
	EventIDs           []string `json:"event_ids,omitempty"`
	WorkflowRunID      string   `json:"workflow_run_id,omitempty"`
	DeliveryAttemptIDs []string `json:"delivery_attempt_ids,omitempty"`
}

// ExecutionArtifactRevisions records the exact immutable inputs used for one execution.
type ExecutionArtifactRevisions struct {
	Source   ArtifactRevisionRef `json:"source"`
	Profile  ArtifactRevisionRef `json:"profile"`
	Workflow ArtifactRevisionRef `json:"workflow"`
}

// ProcessResult is the common output contract for production and preview execution.
type ProcessResult struct {
	Mode                ExecutionMode               `json:"mode"`
	TenantID            string                      `json:"tenant_id"`
	IntegrationRevision ArtifactRevisionRef         `json:"integration_revision"`
	ArtifactRevisions   *ExecutionArtifactRevisions `json:"artifact_revisions,omitempty"`
	Security            SecurityContext             `json:"security"`
	Receipt             *Receipt                    `json:"receipt,omitempty"`
	Events              []ProcessedEvent            `json:"events,omitempty"`
	Diagnostics         []Diagnostic                `json:"diagnostics,omitempty"`
	Routes              []RouteResult               `json:"routes,omitempty"`
	Deliveries          []DeliveryResult            `json:"deliveries,omitempty"`
	Correlations        CorrelationIDs              `json:"correlations"`
}

// Validate enforces tenant consistency and preview side-effect freedom.
func (r ProcessResult) Validate() error {
	v := &validationCollector{}
	v.add(r.Mode == ExecutionModeProduction || r.Mode == ExecutionModePreview, "INVALID_MODE", "mode", "execution mode must be production or preview")
	v.add(strings.TrimSpace(r.TenantID) != "", "REQUIRED", "tenant_id", "tenant ID is required")
	validateArtifactRevision("integration_revision", r.IntegrationRevision, v)
	if r.ArtifactRevisions != nil {
		validateArtifactRevision("artifact_revisions.source", r.ArtifactRevisions.Source, v)
		validateArtifactRevision("artifact_revisions.profile", r.ArtifactRevisions.Profile, v)
		validateArtifactRevision("artifact_revisions.workflow", r.ArtifactRevisions.Workflow, v)
	}
	validateSecurityContext("security", r.Security, v)
	v.add(r.Security.TenantID == r.TenantID, "TENANT_MISMATCH", "security.tenant_id", "security tenant must match result tenant")
	v.add(strings.TrimSpace(r.Correlations.TenantID) != "", "REQUIRED", "correlations.tenant_id", "correlation tenant ID is required")
	v.add(r.Correlations.TenantID == r.TenantID, "TENANT_MISMATCH", "correlations.tenant_id", "correlation tenant must match result tenant")
	v.add(strings.TrimSpace(r.Correlations.CorrelationID) != "", "REQUIRED", "correlations.correlation_id", "correlation ID is required")

	if r.Mode == ExecutionModePreview {
		v.add(r.Receipt == nil, "PREVIEW_SIDE_EFFECT", "receipt", "preview results cannot contain a durable receipt")
		v.add(r.Correlations.ReceiptID == "", "PREVIEW_SIDE_EFFECT", "correlations.receipt_id", "preview results cannot correlate a durable receipt")
	} else if r.Mode == ExecutionModeProduction {
		v.add(r.Receipt != nil, "REQUIRED", "receipt", "production results require a durable receipt")
	}
	if r.Receipt != nil {
		validateReceipt(*r.Receipt, r, v)
	}

	eventIDs := make([]string, 0, len(r.Events))
	for i, event := range r.Events {
		path := fmt.Sprintf("events[%d]", i)
		validateProcessedEvent(path, event, v)
		v.add(event.TenantID == r.TenantID, "TENANT_MISMATCH", joinPath(path, "tenant_id"), "event tenant must match result tenant")
		v.add(event.SourceMessageID == r.Correlations.SourceMessageID, "SOURCE_MESSAGE_MISMATCH", joinPath(path, "source_message_id"), "event source message ID must match result correlations")
		v.add(event.CorrelationID == r.Correlations.CorrelationID, "CORRELATION_MISMATCH", joinPath(path, "correlation_id"), "event correlation ID must match result correlations")
		eventIDs = append(eventIDs, event.ID)
	}
	validateExactIdentifiers("correlations.event_ids", r.Correlations.EventIDs, eventIDs, v)

	for i, diagnostic := range r.Diagnostics {
		path := fmt.Sprintf("diagnostics[%d]", i)
		validateDiagnostic(path, diagnostic, v)
		v.add(diagnostic.TenantID == r.TenantID, "TENANT_MISMATCH", joinPath(path, "tenant_id"), "diagnostic tenant must match result tenant")
	}
	routePlans := make(map[string]RouteResult, len(r.Routes))
	for i, route := range r.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		v.add(route.TenantID == r.TenantID, "TENANT_MISMATCH", joinPath(path, "tenant_id"), "route tenant must match result tenant")
		validateOptionalLineageID(joinPath(path, "event_id"), route.EventID, v)
		v.add(strings.TrimSpace(route.Route) != "", "REQUIRED", joinPath(path, "route"), "route name is required")
		v.add(route.TransformCount >= 0, "INVALID_COUNT", joinPath(path, "transform_count"), "transform count cannot be negative")
		if route.Skipped {
			v.add(strings.TrimSpace(route.SkipReason) != "", "REQUIRED", joinPath(path, "skip_reason"), "skipped routes require a reason")
		} else {
			v.add(strings.TrimSpace(route.SkipReason) == "", "FORBIDDEN", joinPath(path, "skip_reason"), "non-skipped routes cannot carry a skip reason")
		}
		_, duplicateRoute := routePlans[route.Route]
		v.add(!duplicateRoute, "DUPLICATE", joinPath(path, "route"), "route result is duplicated")
		routePlans[route.Route] = route
		plannedActions := make(map[string]struct{}, len(route.PlannedActions))
		for actionIndex, action := range route.PlannedActions {
			v.add(strings.TrimSpace(action) != "", "REQUIRED", fmt.Sprintf("%s.planned_actions[%d]", path, actionIndex), "planned action cannot be empty")
			_, duplicateAction := plannedActions[action]
			v.add(!duplicateAction, "DUPLICATE", fmt.Sprintf("%s.planned_actions[%d]", path, actionIndex), "planned action is duplicated")
			plannedActions[action] = struct{}{}
		}
	}

	deliveryAttemptIDs := make([]string, 0, len(r.Deliveries))
	for i, delivery := range r.Deliveries {
		path := fmt.Sprintf("deliveries[%d]", i)
		v.add(delivery.TenantID == r.TenantID, "TENANT_MISMATCH", joinPath(path, "tenant_id"), "delivery tenant must match result tenant")
		validateOptionalLineageID(joinPath(path, "event_id"), delivery.EventID, v)
		validateArtifactRevision(joinPath(path, "destination"), delivery.Destination.ArtifactRevisionRef, v)
		v.add(delivery.Destination.Class == DestinationClassProduction || delivery.Destination.Class == DestinationClassSandbox, "INVALID_DESTINATION_CLASS", joinPath(path, "destination.class"), "destination class must be production or sandbox")
		v.add(validDeliveryStatus(delivery.Status), "INVALID_DELIVERY_STATUS", joinPath(path, "status"), "delivery status is not supported")
		if r.Mode == ExecutionModePreview {
			v.add(delivery.Status == DeliveryStatusPlanned || delivery.Status == DeliveryStatusSuppressed, "PREVIEW_SIDE_EFFECT", joinPath(path, "status"), "preview delivery must be planned or suppressed")
		}
		v.add(strings.TrimSpace(delivery.Route) != "", "REQUIRED", joinPath(path, "route"), "delivery result requires route lineage")
		v.add(strings.TrimSpace(delivery.Action) != "", "REQUIRED", joinPath(path, "action"), "delivery result requires action lineage")
		plan, routeExists := routePlans[delivery.Route]
		v.add(routeExists, "UNBOUND_ROUTE", joinPath(path, "route"), "delivery route is absent from the route plan")
		if routeExists {
			v.add(plan.Matched && !plan.Skipped, "UNEXECUTABLE_ROUTE", joinPath(path, "route"), "delivery route must be matched and not skipped")
			planned := false
			for _, action := range plan.PlannedActions {
				if action == delivery.Action {
					planned = true
					break
				}
			}
			v.add(planned, "UNPLANNED_ACTION", joinPath(path, "action"), "delivery action is absent from the route plan")
		}
		executed := delivery.Status == DeliveryStatusQueued || delivery.Status == DeliveryStatusSucceeded || delivery.Status == DeliveryStatusFailed
		if executed {
			v.add(strings.TrimSpace(delivery.AttemptID) != "", "REQUIRED", joinPath(path, "attempt_id"), "executed delivery requires an attempt ID")
			v.add(delivery.AttemptCount > 0, "REQUIRED", joinPath(path, "attempt_count"), "executed delivery requires a positive attempt count")
			deliveryAttemptIDs = append(deliveryAttemptIDs, delivery.AttemptID)
		} else if delivery.Status == DeliveryStatusPlanned || delivery.Status == DeliveryStatusSuppressed {
			v.add(strings.TrimSpace(delivery.AttemptID) == "", "FORBIDDEN", joinPath(path, "attempt_id"), "non-executed delivery cannot carry an attempt ID")
			v.add(delivery.AttemptCount == 0, "FORBIDDEN", joinPath(path, "attempt_count"), "non-executed delivery cannot carry an attempt count")
		}
	}
	validateExactIdentifiers("correlations.delivery_attempt_ids", r.Correlations.DeliveryAttemptIDs, deliveryAttemptIDs, v)

	if r.Receipt != nil && r.Receipt.Status == ReceiptStatusRejected {
		v.add(len(r.Events) == 0, "REJECTED_SIDE_EFFECT", "events", "rejected receipts cannot contain events")
		v.add(len(r.Routes) == 0, "REJECTED_SIDE_EFFECT", "routes", "rejected receipts cannot contain route plans")
		v.add(len(r.Deliveries) == 0, "REJECTED_SIDE_EFFECT", "deliveries", "rejected receipts cannot contain deliveries")
	}
	return v.err()
}

// ValidateAgainst verifies result shape plus exact revision and policy bindings.
func (r ProcessResult) ValidateAgainst(revision IntegrationDefinitionRevision) error {
	v := &validationCollector{}
	v.merge("", r.Validate())
	v.merge("revision", revision.Validate())
	v.add(r.TenantID == revision.TenantID, "TENANT_MISMATCH", "tenant_id", "result tenant must match integration revision tenant")
	v.add(r.IntegrationRevision == revision.Reference(), "REVISION_MISMATCH", "integration_revision", "result revision does not match the resolved integration revision")
	if r.ArtifactRevisions != nil {
		v.add(r.ArtifactRevisions.Source == revision.Source.ArtifactRevisionRef, "REVISION_MISMATCH", "artifact_revisions.source", "source revision does not match the resolved integration revision")
		v.add(r.ArtifactRevisions.Profile == revision.Profile, "REVISION_MISMATCH", "artifact_revisions.profile", "profile revision does not match the resolved integration revision")
		v.add(r.ArtifactRevisions.Workflow == revision.Workflow, "REVISION_MISMATCH", "artifact_revisions.workflow", "workflow revision does not match the resolved integration revision")
	}
	if r.Security.Principal.Kind == PrincipalKindService {
		v.add(r.Security.Principal.SourceID == revision.Source.SourceID, "SOURCE_MISMATCH", "security.principal.source_id", "service principal source must match integration revision source")
	}
	for i, delivery := range r.Deliveries {
		bound := false
		for _, destination := range revision.Destinations {
			if delivery.Destination == destination {
				bound = true
				break
			}
		}
		v.add(bound, "UNBOUND_DESTINATION", fmt.Sprintf("deliveries[%d].destination", i), "delivery destination is not bound to the integration revision")
	}
	if r.Receipt != nil {
		v.add(r.Receipt.RawRetentionMode == revision.Policy.RawRetention.EffectiveMode(), "RETENTION_MISMATCH", "receipt.raw_retention_mode", "receipt retention mode must match integration policy")
		policy := revision.Policy.RawRetention
		if policy.EffectiveMode() == RawRetentionModeEncrypted && policy.TTLSeconds > 0 && r.Receipt.RawExpiresAt != nil && !r.Receipt.RecordedAt.IsZero() {
			recordedUnix := r.Receipt.RecordedAt.Unix()
			if recordedUnix > math.MaxInt64-policy.TTLSeconds {
				v.add(false, "INVALID_TTL", "revision.policy.raw_retention.ttl_seconds", "raw retention TTL overflows the receipt timestamp")
			} else {
				deadline := time.Unix(recordedUnix+policy.TTLSeconds, int64(r.Receipt.RecordedAt.Nanosecond()))
				v.add(!r.Receipt.RawExpiresAt.After(deadline), "RETENTION_EXCEEDED", "receipt.raw_expires_at", "raw expiry cannot exceed the integration policy TTL")
			}
		}
	}
	return v.err()
}

// ValidateFor binds a result to both the authenticated request and resolved revision.
func (r ProcessResult) ValidateFor(request ProcessRequest, revision IntegrationDefinitionRevision) error {
	v := &validationCollector{}
	v.merge("request", request.ValidateAgainst(revision))
	v.merge("result", r.ValidateAgainst(revision))
	v.add(r.Mode == request.Mode, "MODE_MISMATCH", "mode", "result mode must match request mode")
	v.add(r.TenantID == request.Security.TenantID, "TENANT_MISMATCH", "tenant_id", "result tenant must match request security tenant")
	v.add(r.IntegrationRevision == request.IntegrationRevision, "REVISION_MISMATCH", "integration_revision", "result revision must match request revision")
	v.add(principalsEquivalent(r.Security.Principal, request.Security.Principal), "PRINCIPAL_MISMATCH", "security.principal", "result principal must match authenticated request principal")
	v.add(r.Security.Reason == request.Security.Reason, "REASON_MISMATCH", "security.reason", "result reason must match request reason")
	v.add(r.Correlations.CorrelationID == request.CorrelationID, "CORRELATION_MISMATCH", "correlations.correlation_id", "result correlation ID must match request correlation ID")
	if request.IdempotencyKey != "" && r.Receipt != nil {
		v.add(r.Receipt.IdempotencyKey == request.IdempotencyKey, "IDEMPOTENCY_MISMATCH", "receipt.idempotency_key", "receipt must preserve the explicit request idempotency key")
	}
	return v.err()
}

// ValidatePreviewFor applies the strict, event-bound contract used by the shared preview kernel.
// Validate and ValidateFor remain compatible with legacy results that predate execution provenance.
func (r ProcessResult) ValidatePreviewFor(request ProcessRequest, revision IntegrationDefinitionRevision) error {
	v := &validationCollector{}
	v.merge("", r.ValidateFor(request, revision))
	v.add(request.Mode == ExecutionModePreview, "INVALID_MODE", "request.mode", "preview validation requires a preview request")
	v.add(r.Mode == ExecutionModePreview, "INVALID_MODE", "mode", "preview validation requires a preview result")
	v.add(r.ArtifactRevisions != nil, "REQUIRED", "artifact_revisions", "preview results require exact execution artifact revisions")
	v.add(r.Receipt == nil, "PREVIEW_SIDE_EFFECT", "receipt", "preview results cannot contain a durable receipt")
	v.add(r.Correlations.ReceiptID == "", "PREVIEW_SIDE_EFFECT", "correlations.receipt_id", "preview results cannot correlate a durable receipt")
	v.add(len(r.Correlations.DeliveryAttemptIDs) == 0, "PREVIEW_SIDE_EFFECT", "correlations.delivery_attempt_ids", "preview results cannot correlate delivery attempts")

	eventIDs := make(map[string]struct{}, len(r.Events))
	for _, event := range r.Events {
		eventIDs[event.ID] = struct{}{}
	}
	diagnosticCodes := make(map[string]struct{}, len(r.Diagnostics))
	for _, diagnostic := range r.Diagnostics {
		diagnosticCodes[diagnostic.Code] = struct{}{}
	}

	routePlans := make(map[previewRouteIdentity]RouteResult, len(r.Routes))
	for i, route := range r.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		v.add(route.EventID != "", "REQUIRED", joinPath(path, "event_id"), "preview routes require event lineage")
		_, eventExists := eventIDs[route.EventID]
		v.add(eventExists, "UNBOUND_EVENT", joinPath(path, "event_id"), "route event is absent from the preview result")
		key := previewRouteIdentity{eventID: route.EventID, route: route.Route}
		_, duplicate := routePlans[key]
		v.add(!duplicate, "DUPLICATE", path, "preview route lineage is duplicated")
		routePlans[key] = route
		if len(route.PlannedActions) > 0 {
			v.add(route.Matched && !route.Skipped, "UNEXECUTABLE_ROUTE", joinPath(path, "planned_actions"), "planned actions require a matched, non-skipped route")
		}
		validateDiagnosticCodeBindings(joinPath(path, "diagnostic_codes"), route.DiagnosticCodes, diagnosticCodes, v)
	}

	deliveries := make(map[previewDeliveryIdentity]struct{}, len(r.Deliveries))
	for i, delivery := range r.Deliveries {
		path := fmt.Sprintf("deliveries[%d]", i)
		v.add(delivery.EventID != "", "REQUIRED", joinPath(path, "event_id"), "preview deliveries require event lineage")
		_, eventExists := eventIDs[delivery.EventID]
		v.add(eventExists, "UNBOUND_EVENT", joinPath(path, "event_id"), "delivery event is absent from the preview result")
		v.add(delivery.Status == DeliveryStatusSuppressed, "PREVIEW_SIDE_EFFECT", joinPath(path, "status"), "preview deliveries must be suppressed")
		v.add(delivery.AttemptID == "", "PREVIEW_SIDE_EFFECT", joinPath(path, "attempt_id"), "preview deliveries cannot carry an attempt ID")
		v.add(delivery.AttemptCount == 0, "PREVIEW_SIDE_EFFECT", joinPath(path, "attempt_count"), "preview deliveries cannot carry an attempt count")
		key := previewDeliveryIdentity{eventID: delivery.EventID, route: delivery.Route, action: delivery.Action}
		_, duplicate := deliveries[key]
		v.add(!duplicate, "DUPLICATE", path, "preview delivery lineage is duplicated")
		deliveries[key] = struct{}{}
		plan, routeExists := routePlans[previewRouteIdentity{eventID: delivery.EventID, route: delivery.Route}]
		v.add(routeExists, "UNBOUND_ROUTE", joinPath(path, "route"), "delivery route is absent from the event-bound route plan")
		if routeExists {
			v.add(plan.Matched && !plan.Skipped, "UNEXECUTABLE_ROUTE", joinPath(path, "route"), "delivery route must be matched and not skipped")
			planned := false
			for _, action := range plan.PlannedActions {
				if action == delivery.Action {
					planned = true
					break
				}
			}
			v.add(planned, "UNPLANNED_ACTION", joinPath(path, "action"), "delivery action is absent from the event-bound route plan")
		}
		validateDiagnosticCodeBindings(joinPath(path, "diagnostic_codes"), delivery.DiagnosticCodes, diagnosticCodes, v)
	}
	return v.err()
}

func validateOptionalLineageID(path, id string, v *validationCollector) {
	if id == "" {
		return
	}
	valid := len(id) <= 256 && strings.TrimSpace(id) == id && strings.TrimSpace(id) != ""
	for _, character := range id {
		if unicode.IsControl(character) {
			valid = false
			break
		}
	}
	v.add(valid, "INVALID_IDENTIFIER", path, "lineage identifier must be bounded, nonempty, and free of control characters or surrounding whitespace")
}

func validateDiagnosticCodeBindings(path string, codes []string, available map[string]struct{}, v *validationCollector) {
	seen := make(map[string]struct{}, len(codes))
	for i, code := range codes {
		codePath := fmt.Sprintf("%s[%d]", path, i)
		v.add(validDiagnosticCode(code), "INVALID_CODE", codePath, "diagnostic binding code is invalid")
		_, duplicate := seen[code]
		v.add(!duplicate, "DUPLICATE", codePath, "diagnostic binding code is duplicated")
		seen[code] = struct{}{}
		_, exists := available[code]
		v.add(exists, "UNBOUND_DIAGNOSTIC", codePath, "diagnostic binding code is absent from the preview result")
	}
}

type previewRouteIdentity struct {
	eventID string
	route   string
}

type previewDeliveryIdentity struct {
	eventID string
	route   string
	action  string
}

func validateReceipt(receipt Receipt, result ProcessResult, v *validationCollector) {
	v.add(strings.TrimSpace(receipt.ID) != "", "REQUIRED", "receipt.id", "receipt ID is required")
	v.add(receipt.TenantID == result.TenantID, "TENANT_MISMATCH", "receipt.tenant_id", "receipt tenant must match result tenant")
	validateArtifactRevision("receipt.integration_revision", receipt.IntegrationRevision, v)
	v.add(receipt.IntegrationRevision == result.IntegrationRevision, "REVISION_MISMATCH", "receipt.integration_revision", "receipt revision must match result revision")
	v.add(receipt.Status == ReceiptStatusAccepted || receipt.Status == ReceiptStatusRejected, "INVALID_RECEIPT_STATUS", "receipt.status", "receipt status must be accepted or rejected")
	v.add(strings.TrimSpace(receipt.IdempotencyKey) != "", "REQUIRED", "receipt.idempotency_key", "durable receipts require an effective idempotency key")
	v.add(!receipt.RecordedAt.IsZero(), "REQUIRED", "receipt.recorded_at", "receipt timestamp is required")
	v.add(strings.TrimSpace(receipt.CorrelationID) != "", "REQUIRED", "receipt.correlation_id", "receipt correlation ID is required")
	v.add(receipt.CorrelationID == result.Correlations.CorrelationID, "CORRELATION_MISMATCH", "receipt.correlation_id", "receipt correlation ID must match result correlation ID")
	v.add(receipt.RawRetentionMode == RawRetentionModeEphemeral || receipt.RawRetentionMode == RawRetentionModeEncrypted, "INVALID_RETENTION_MODE", "receipt.raw_retention_mode", "receipt raw retention mode is not supported")
	if receipt.RawRetentionMode == RawRetentionModeEphemeral {
		v.add(receipt.RawExpiresAt == nil, "FORBIDDEN", "receipt.raw_expires_at", "ephemeral receipts cannot carry raw expiry")
	}
	if receipt.RawRetentionMode == RawRetentionModeEncrypted {
		v.add(receipt.RawExpiresAt != nil, "REQUIRED", "receipt.raw_expires_at", "encrypted retention receipts require raw expiry")
		if receipt.RawExpiresAt != nil && !receipt.RecordedAt.IsZero() {
			v.add(receipt.RawExpiresAt.After(receipt.RecordedAt), "INVALID_EXPIRY", "receipt.raw_expires_at", "raw expiry must follow receipt time")
		}
	}
	validatePrincipal("receipt.principal", receipt.Principal, v)
	if receipt.Principal.Kind == PrincipalKindHuman {
		v.add(strings.TrimSpace(receipt.Reason) != "", "REQUIRED", "receipt.reason", "human receipts require a reason")
	}
	v.add(principalsEquivalent(receipt.Principal, result.Security.Principal), "PRINCIPAL_MISMATCH", "receipt.principal", "receipt principal must match result security principal")
	v.add(receipt.Reason == result.Security.Reason, "REASON_MISMATCH", "receipt.reason", "receipt reason must match result security reason")
	v.add(result.Correlations.ReceiptID == receipt.ID, "CORRELATION_MISMATCH", "correlations.receipt_id", "correlated receipt ID must match receipt")
}

func validateSecurityContext(path string, security SecurityContext, v *validationCollector) {
	v.add(strings.TrimSpace(security.TenantID) != "", "REQUIRED", joinPath(path, "tenant_id"), "security tenant ID is required")
	validatePrincipal(joinPath(path, "principal"), security.Principal, v)
	if security.Principal.Kind == PrincipalKindHuman {
		v.add(strings.TrimSpace(security.Reason) != "", "REQUIRED", joinPath(path, "reason"), "human operations require a reason")
	}
}

func validateProcessedEvent(path string, event ProcessedEvent, v *validationCollector) {
	v.add(strings.TrimSpace(event.TenantID) != "", "REQUIRED", joinPath(path, "tenant_id"), "event tenant is required")
	v.add(strings.TrimSpace(event.ID) != "", "REQUIRED", joinPath(path, "id"), "event ID is required")
	v.add(event.Type != "", "REQUIRED", joinPath(path, "type"), "event type is required")
	v.add(strings.TrimSpace(event.SourceMessageID) != "", "REQUIRED", joinPath(path, "source_message_id"), "event source message ID is required")
	v.add(strings.TrimSpace(event.CorrelationID) != "", "REQUIRED", joinPath(path, "correlation_id"), "event correlation ID is required")
	v.add(event.Classification == DataClassificationPHI, "INVALID_CLASSIFICATION", joinPath(path, "classification"), "event classification must be phi")
	validateCanonicalPayload(joinPath(path, "payload"), event.payload, v)
	if json.Valid(event.payload) {
		var metadata struct {
			ID              string           `json:"id"`
			Type            events.EventType `json:"type"`
			SourceMessageID string           `json:"source_message_id"`
			CorrelationID   string           `json:"correlation_id"`
		}
		if err := json.Unmarshal(event.payload, &metadata); err == nil {
			v.add(metadata.ID == event.ID, "EVENT_ID_MISMATCH", joinPath(path, "payload.id"), "payload event ID must match processed event ID")
			v.add(metadata.Type == event.Type, "EVENT_TYPE_MISMATCH", joinPath(path, "payload.type"), "payload event type must match processed event type")
			v.add(metadata.SourceMessageID == event.SourceMessageID, "SOURCE_MESSAGE_MISMATCH", joinPath(path, "payload.source_message_id"), "payload source message ID must match processed event source message ID")
			v.add(metadata.CorrelationID == event.CorrelationID, "CORRELATION_MISMATCH", joinPath(path, "payload.correlation_id"), "payload correlation ID must match processed event correlation ID")
		}
	}
}

func concreteCanonicalEventType(canonicalEvent any) (reflect.Type, error) {
	if canonicalEvent == nil {
		return nil, fmt.Errorf("canonical event is required")
	}
	value := reflect.ValueOf(canonicalEvent)
	typeOf := value.Type()
	if typeOf.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("canonical event is nil")
		}
		typeOf = typeOf.Elem()
	}
	const canonicalEventsPackage = "gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	if typeOf.PkgPath() != canonicalEventsPackage || !strings.HasSuffix(typeOf.Name(), "Event") || typeOf.Name() == "Event" {
		return nil, fmt.Errorf("%T is not a supported concrete canonical event", canonicalEvent)
	}
	return typeOf, nil
}

var canonicalEventRegistry = map[events.EventType]reflect.Type{
	events.EventPatientAdmit:                   reflect.TypeOf(events.PatientAdmitEvent{}),
	events.EventPatientTransfer:                reflect.TypeOf(events.PatientAdmitEvent{}),
	events.EventPatientUpdate:                  reflect.TypeOf(events.PatientAdmitEvent{}),
	events.EventPatientDischarge:               reflect.TypeOf(events.PatientDischargeEvent{}),
	events.EventAppointmentScheduled:           reflect.TypeOf(events.AppointmentEvent{}),
	events.EventAppointmentCancelled:           reflect.TypeOf(events.AppointmentEvent{}),
	events.EventAppointmentRescheduled:         reflect.TypeOf(events.AppointmentEvent{}),
	events.EventAppointmentModified:            reflect.TypeOf(events.AppointmentEvent{}),
	events.EventAppointmentNoShow:              reflect.TypeOf(events.AppointmentEvent{}),
	events.EventAppointmentCheckedIn:           reflect.TypeOf(events.AppointmentEvent{}),
	events.EventLabResult:                      reflect.TypeOf(events.LabResultEvent{}),
	events.EventClaimSubmitted:                 reflect.TypeOf(events.ClaimSubmittedEvent{}),
	events.EventClaimAdjudicated:               reflect.TypeOf(events.ClaimAdjudicatedEvent{}),
	events.EventEligibilityInquiry:             reflect.TypeOf(events.EligibilityInquiryEvent{}),
	events.EventEligibilityResponse:            reflect.TypeOf(events.EligibilityResponseEvent{}),
	events.EventClaimStatusRequest:             reflect.TypeOf(events.ClaimStatusRequestEvent{}),
	events.EventClaimStatusResponse:            reflect.TypeOf(events.ClaimStatusResponseEvent{}),
	events.EventDocument:                       reflect.TypeOf(events.DocumentEvent{}),
	events.EventDocumentOriginal:               reflect.TypeOf(events.DocumentEvent{}),
	events.EventDocumentStatusChange:           reflect.TypeOf(events.DocumentEvent{}),
	events.EventDocumentAddendum:               reflect.TypeOf(events.DocumentEvent{}),
	events.EventDocumentEdit:                   reflect.TypeOf(events.DocumentEvent{}),
	events.EventDocumentReplacement:            reflect.TypeOf(events.DocumentEvent{}),
	events.EventVitalSign:                      reflect.TypeOf(events.VitalSignEvent{}),
	events.EventCondition:                      reflect.TypeOf(events.ConditionEvent{}),
	events.EventProcedure:                      reflect.TypeOf(events.ProcedureEvent{}),
	events.EventImmunization:                   reflect.TypeOf(events.ImmunizationEvent{}),
	events.EventMedicationRequest:              reflect.TypeOf(events.MedicationRequestEvent{}),
	events.EventAllergyIntolerance:             reflect.TypeOf(events.AllergyIntoleranceEvent{}),
	events.EventSocialHistory:                  reflect.TypeOf(events.SocialHistoryEvent{}),
	events.EventFinancialTransaction:           reflect.TypeOf(events.FinancialTransactionEvent{}),
	events.EventType("care_plan"):              reflect.TypeOf(events.CarePlanEvent{}),
	events.EventType("goal"):                   reflect.TypeOf(events.GoalEvent{}),
	events.EventType("care_team"):              reflect.TypeOf(events.CareTeamEvent{}),
	events.EventType("service_request"):        reflect.TypeOf(events.ServiceRequestEvent{}),
	events.EventType("document_reference"):     reflect.TypeOf(events.DocumentReferenceEvent{}),
	events.EventType("diagnostic_report_note"): reflect.TypeOf(events.DiagnosticReportNoteEvent{}),
	events.EventType("provenance"):             reflect.TypeOf(events.ProvenanceEvent{}),
	events.EventType("facility_location"):      reflect.TypeOf(events.FacilityLocationEvent{}),
	events.EventType("organization"):           reflect.TypeOf(events.OrganizationEvent{}),
	events.EventType("practitioner"):           reflect.TypeOf(events.PractitionerEvent{}),
	events.EventType("practitioner_role"):      reflect.TypeOf(events.PractitionerRoleEvent{}),
	events.EventType("related_person"):         reflect.TypeOf(events.RelatedPersonEvent{}),
}

func decodeCanonicalEventPayload(eventType events.EventType, payload json.RawMessage) (any, error) {
	concreteType, registered := canonicalEventRegistry[eventType]
	if !registered {
		return nil, fmt.Errorf("event type %q has no registered canonical schema", eventType)
	}
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return nil, err
	}
	var shape any
	if err := json.Unmarshal(payload, &shape); err != nil {
		return nil, err
	}
	if forbidden := findForbiddenRawKey(shape); forbidden != "" {
		return nil, fmt.Errorf("payload.%s: source raw-data fields are forbidden", forbidden)
	}
	value := reflect.New(concreteType)
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value.Interface()); err != nil {
		return nil, fmt.Errorf("payload does not match %s: %w", concreteType.Name(), err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := validateCanonicalJSONKeys(payload, value.Interface()); err != nil {
		return nil, err
	}
	return value.Interface(), nil
}

func redactedCanonicalEventCopy(canonicalEvent any, eventType reflect.Type) any {
	original := reflect.ValueOf(canonicalEvent)
	if original.Kind() == reflect.Pointer {
		original = original.Elem()
	}
	clone := reflect.New(eventType).Elem()
	clone.Set(original)
	for index := 0; index < eventType.NumField(); index++ {
		field := eventType.Field(index)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" {
			jsonName = field.Name
		}
		if _, blocked := forbiddenRawPayloadKeys[normalizeJSONKey(jsonName)]; blocked && clone.Field(index).CanSet() {
			clone.Field(index).SetZero()
		}
	}
	return clone.Interface()
}

func removeRawPayloadFields(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, blocked := forbiddenRawPayloadKeys[normalizeJSONKey(key)]; blocked {
				delete(typed, key)
				continue
			}
			removeRawPayloadFields(child)
		}
	case []any:
		for _, child := range typed {
			removeRawPayloadFields(child)
		}
	}
}

func validateDiagnostic(path string, diagnostic Diagnostic, v *validationCollector) {
	v.add(strings.TrimSpace(diagnostic.TenantID) != "", "REQUIRED", joinPath(path, "tenant_id"), "diagnostic tenant is required")
	v.add(validDiagnosticSeverity(diagnostic.Severity), "INVALID_SEVERITY", joinPath(path, "severity"), "diagnostic severity is not supported")
	v.add(diagnostic.Stage != "" && validDiagnosticSource(diagnostic.Stage), "INVALID_STAGE", joinPath(path, "stage"), "diagnostic stage must be a bounded subsystem identifier")
	v.add(validDiagnosticCode(diagnostic.Code), "INVALID_CODE", joinPath(path, "code"), "diagnostic code must use uppercase letters, digits, and underscores")
	v.add(strings.TrimSpace(diagnostic.message) != "", "REQUIRED", joinPath(path, "message"), "diagnostic message is required")
	v.add(diagnostic.message == diagnosticMessage(diagnostic.Code), "MESSAGE_MISMATCH", joinPath(path, "message"), "diagnostic message must come from the code catalog")
	v.add(validDiagnosticPath(diagnostic.Path), "INVALID_PATH", joinPath(path, "path"), "diagnostic path contains unsupported characters")
	v.add(validDiagnosticSource(diagnostic.source), "INVALID_SOURCE", joinPath(path, "source"), "diagnostic source must be a bounded subsystem identifier")
	v.add(diagnostic.Classification == DataClassificationPHI, "INVALID_CLASSIFICATION", joinPath(path, "classification"), "diagnostic classification must be phi")
}

func diagnosticMessage(code string) string {
	switch code {
	case "MISSING_PV1":
		return "PV1 was not present"
	case "INVALID_MESSAGE":
		return "Source message could not be parsed"
	default:
		return "Processing reported diagnostic " + code
	}
}

func validDiagnosticCode(code string) bool {
	if len(code) == 0 || len(code) > 128 || code[0] < 'A' || code[0] > 'Z' {
		return false
	}
	for _, character := range code[1:] {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validDiagnosticPath(path string) bool {
	if len(path) > 256 || strings.ContainsAny(path, "\r\n\x00") {
		return false
	}
	for _, character := range path {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("._-[]/", character) {
			continue
		}
		return false
	}
	return true
}

func validDiagnosticSource(source string) bool {
	if source == "" {
		return true
	}
	if len(source) > 64 || source != strings.ToLower(source) {
		return false
	}
	for index, character := range source {
		if (character >= 'a' && character <= 'z') || (index > 0 && character >= '0' && character <= '9') || (index > 0 && (character == '.' || character == '-' || character == '_')) {
			continue
		}
		return false
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func validateCanonicalPayload(path string, payload json.RawMessage, v *validationCollector) {
	if len(payload) == 0 || !json.Valid(payload) {
		v.add(false, "INVALID_JSON", path, "event payload must contain valid JSON")
		return
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		v.add(false, "INVALID_JSON", path, "event payload must contain valid JSON")
		return
	}
	_, object := decoded.(map[string]any)
	v.add(object, "INVALID_EVENT_PAYLOAD", path, "event payload must be a JSON object")
	if forbidden := findForbiddenRawKey(decoded); forbidden != "" {
		v.add(false, "RAW_PAYLOAD_FORBIDDEN", joinPath(path, forbidden), "canonical event payload cannot contain source raw-data fields")
	}
}

func findForbiddenRawKey(value any) string {
	var walk func(any, string) string
	walk = func(current any, path string) string {
		switch typed := current.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := typed[key]
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				if _, blocked := forbiddenRawPayloadKeys[normalizeJSONKey(key)]; blocked {
					return childPath
				}
				if found := walk(child, childPath); found != "" {
					return found
				}
			}
		case []any:
			for index, child := range typed {
				if found := walk(child, fmt.Sprintf("%s[%d]", path, index)); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(value, "")
}

var forbiddenRawPayloadKeys = map[string]struct{}{
	"original":        {},
	"originalpayload": {},
	"parsewarnings":   {},
	"raw":             {},
	"rawmessage":      {},
	"rawpayload":      {},
	"sourcepayload":   {},
	"sourceraw":       {},
}

func normalizeJSONKey(key string) string {
	var normalized strings.Builder
	for _, character := range key {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(unicode.ToLower(character))
		}
	}
	return normalized.String()
}

func validateExactIdentifiers(path string, actual, expected []string, v *validationCollector) {
	actualSet := make(map[string]struct{}, len(actual))
	for index, identifier := range actual {
		v.add(strings.TrimSpace(identifier) != "", "REQUIRED", fmt.Sprintf("%s[%d]", path, index), "correlated identifier cannot be empty")
		_, duplicate := actualSet[identifier]
		v.add(!duplicate, "DUPLICATE", fmt.Sprintf("%s[%d]", path, index), "correlated identifier is duplicated")
		actualSet[identifier] = struct{}{}
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, identifier := range expected {
		_, duplicate := expectedSet[identifier]
		v.add(!duplicate, "DUPLICATE", path, "result identifiers must be unique")
		expectedSet[identifier] = struct{}{}
	}
	v.add(len(actualSet) == len(expectedSet), "CORRELATION_MISMATCH", path, "correlated identifiers must exactly match result identifiers")
	for identifier := range expectedSet {
		_, present := actualSet[identifier]
		v.add(present, "CORRELATION_MISMATCH", path, "correlated identifiers must exactly match result identifiers")
	}
}

func principalsEquivalent(left, right Principal) bool {
	return left.ID == right.ID &&
		left.Kind == right.Kind &&
		left.AuthMethod == right.AuthMethod &&
		left.SourceID == right.SourceID &&
		stringSetsEqual(left.Roles, right.Roles)
}

func stringSetsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]int, len(left))
	for _, value := range left {
		values[value]++
	}
	for _, value := range right {
		values[value]--
		if values[value] < 0 {
			return false
		}
	}
	return true
}

func validDiagnosticSeverity(severity DiagnosticSeverity) bool {
	return severity == DiagnosticSeverityInfo || severity == DiagnosticSeverityWarning || severity == DiagnosticSeverityError
}

func validDeliveryStatus(status DeliveryStatus) bool {
	return status == DeliveryStatusPlanned ||
		status == DeliveryStatusSuppressed ||
		status == DeliveryStatusQueued ||
		status == DeliveryStatusSucceeded ||
		status == DeliveryStatusFailed
}
