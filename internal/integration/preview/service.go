// Package preview constructs trusted, side-effect-free ProcessRequests for all
// API and IDE preview adapters.
package preview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const PreviewRole = "integration:preview"

var (
	ErrUnavailable            = errors.New("integration preview is unavailable")
	ErrUnauthenticated        = errors.New("authenticated preview identity is required")
	ErrForbidden              = errors.New("preview is forbidden")
	ErrInvalidInput           = errors.New("invalid preview input")
	ErrPayloadTooLarge        = errors.New("preview payload exceeds the allowed size")
	ErrIntegrationUnavailable = errors.New("integration is unavailable")
	ErrPreviewFailed          = errors.New("integration preview failed")
)

// Binding aliases the registry-owned metadata consumed by this adapter.
type Binding = registry.PreviewBinding

// Registry resolves a browser-safe integration key inside an authenticated tenant.
type Registry interface {
	LookupPreviewBinding(ctx context.Context, tenantID, integrationID string) (Binding, error)
}

// Processor is the exact Slice 1.1b semantic kernel boundary.
type Processor interface {
	Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error)
}

// Input contains only caller-owned preview facts. Executable metadata is absent.
type Input struct {
	IntegrationID string
	Payload       []byte
	CorrelationID string
	Reason        string
}

// Service is stateless and owns no persistence, destination, or workflow action client.
type Service struct {
	registry  Registry
	processor Processor
	now       func() time.Time
}

// NewService composes the shared adapter with an injectable clock for parity tests.
func NewService(registry Registry, messageProcessor Processor, now func() time.Time) (*Service, error) {
	if registry == nil || messageProcessor == nil || now == nil {
		return nil, ErrUnavailable
	}
	return &Service{registry: registry, processor: messageProcessor, now: now}, nil
}

// Preview builds one trusted request and invokes only the shared MessageProcessor.
func (s *Service) Preview(ctx context.Context, security integration.SecurityContext, input Input) (integration.ProcessResult, error) {
	if s == nil || s.registry == nil || s.processor == nil || s.now == nil || ctx == nil {
		return integration.ProcessResult{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return integration.ProcessResult{}, err
	}
	if !authenticated(security) {
		return integration.ProcessResult{}, ErrUnauthenticated
	}
	if !hasRole(security.Principal.Roles, PreviewRole) {
		return integration.ProcessResult{}, ErrForbidden
	}
	if err := validateInput(security, input); err != nil {
		return integration.ProcessResult{}, err
	}

	binding, err := s.registry.LookupPreviewBinding(ctx, security.TenantID, input.IntegrationID)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return integration.ProcessResult{}, err
		case errors.Is(err, registry.ErrTenantMismatch):
			return integration.ProcessResult{}, ErrForbidden
		default:
			return integration.ProcessResult{}, ErrIntegrationUnavailable
		}
	}
	receivedAt := s.now().UTC()
	if receivedAt.IsZero() {
		return integration.ProcessResult{}, ErrUnavailable
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       security.TenantID,
		SourceID:       binding.SourceID,
		Format:         binding.Format,
		ContentType:    contentType(binding.Format),
		ReceivedAt:     receivedAt,
		Classification: binding.Classification,
	}, input.Payload)
	if err != nil {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	trustedSecurity := security
	trustedSecurity.Reason = input.Reason
	trustedSecurity.Principal.Roles = append([]string(nil), security.Principal.Roles...)
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: binding.IntegrationRevision,
		Security:            trustedSecurity,
		Envelope:            envelope,
		CorrelationID:       input.CorrelationID,
	}
	result, err := s.processor.Process(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return integration.ProcessResult{}, err
		}
		if errors.Is(err, processor.ErrTenantMismatch) {
			return integration.ProcessResult{}, ErrForbidden
		}
		return integration.ProcessResult{}, ErrPreviewFailed
	}
	return result, nil
}

func authenticated(security integration.SecurityContext) bool {
	return canonicalIdentity(security.TenantID) && canonicalIdentity(security.Principal.ID) &&
		(security.Principal.Kind == integration.PrincipalKindHuman || security.Principal.Kind == integration.PrincipalKindService) &&
		canonicalIdentity(security.Principal.AuthMethod)
}

func validateInput(security integration.SecurityContext, input Input) error {
	if !canonicalIdentity(input.IntegrationID) || !canonicalIdentity(input.CorrelationID) {
		return ErrInvalidInput
	}
	if len(input.Payload) == 0 {
		return ErrInvalidInput
	}
	if int64(len(input.Payload)) > processor.MaxPreviewSourceBytes {
		return ErrPayloadTooLarge
	}
	if security.Principal.Kind == integration.PrincipalKindHuman && !canonicalReason(input.Reason) {
		return ErrInvalidInput
	}
	if input.Reason != "" && !canonicalReason(input.Reason) {
		return ErrInvalidInput
	}
	return nil
}

func canonicalIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func canonicalReason(value string) bool {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\t' {
			return false
		}
	}
	return true
}

func hasRole(roles []string, required string) bool {
	for _, role := range roles {
		if role == required {
			return true
		}
	}
	return false
}

func contentType(format events.SourceFormat) string {
	switch format {
	case events.FormatHL7v2:
		return "application/hl7-v2+er7"
	default:
		return fmt.Sprintf("application/vnd.fi-fhir.%s", format)
	}
}
