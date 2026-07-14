package ingress

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const SubmitRole = "integration:submit"

var (
	ErrUnavailable            = errors.New("HTTP ingress is unavailable")
	ErrInvalidInput           = errors.New("invalid HTTP ingress input")
	ErrPayloadTooLarge        = errors.New("HTTP ingress payload exceeds the allowed size")
	ErrIntegrationUnavailable = errors.New("integration is unavailable")
	ErrForbidden              = errors.New("HTTP ingress is forbidden")
	ErrInvalidMessage         = errors.New("invalid source message")
	ErrIdempotencyConflict    = errors.New("idempotency key conflicts with a different request")
	ErrRetryable              = errors.New("durable submission is temporarily unavailable")
)

type Binding = registry.PreviewBinding

type Registry interface {
	LookupPreviewBinding(ctx context.Context, tenantID, integrationID string) (Binding, error)
}

type Processor interface {
	Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error)
}

type Input struct {
	IntegrationID  string
	Payload        []byte
	IdempotencyKey string
	CorrelationID  string
}

type Service struct {
	tenantID    string
	principalID string
	authMethod  string
	registry    Registry
	processor   Processor
	now         func() time.Time
	newID       func() string
}

type ServiceConfig struct {
	TenantID    string
	PrincipalID string
	AuthMethod  string
	Registry    Registry
	Processor   Processor
	Clock       func() time.Time
	NewID       func() string
}

func NewService(config ServiceConfig) (*Service, error) {
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}
	if err := validateIdentity("principal ID", config.PrincipalID); err != nil {
		return nil, err
	}
	if err := validateIdentity("auth method", config.AuthMethod); err != nil {
		return nil, err
	}
	if config.Registry == nil || config.Processor == nil {
		return nil, ErrUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	newID := config.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	return &Service{
		tenantID:    config.TenantID,
		principalID: config.PrincipalID,
		authMethod:  config.AuthMethod,
		registry:    config.Registry,
		processor:   config.Processor,
		now:         clock,
		newID:       newID,
	}, nil
}

func (s *Service) Submit(ctx context.Context, input Input) (integration.ProcessResult, error) {
	if s == nil || s.registry == nil || s.processor == nil || s.now == nil || s.newID == nil || ctx == nil {
		return integration.ProcessResult{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return integration.ProcessResult{}, err
	}
	if err := validateIdentity("integration ID", input.IntegrationID); err != nil {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	if len(input.Payload) == 0 {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	if int64(len(input.Payload)) > processor.MaxPreviewSourceBytes {
		return integration.ProcessResult{}, ErrPayloadTooLarge
	}
	if input.IdempotencyKey != "" && !validHeaderValue(input.IdempotencyKey, 512) {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	if input.CorrelationID != "" && !validHeaderValue(input.CorrelationID, 256) {
		return integration.ProcessResult{}, ErrInvalidInput
	}

	binding, err := s.registry.LookupPreviewBinding(ctx, s.tenantID, input.IntegrationID)
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
	if binding.Format != events.FormatHL7v2 {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	receivedAt := s.now().UTC()
	if receivedAt.IsZero() {
		return integration.ProcessResult{}, ErrUnavailable
	}
	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = s.newID()
		if !validHeaderValue(correlationID, 256) {
			return integration.ProcessResult{}, ErrUnavailable
		}
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       s.tenantID,
		SourceID:       binding.SourceID,
		Format:         binding.Format,
		ContentType:    "application/hl7-v2+er7",
		ReceivedAt:     receivedAt,
		Classification: binding.Classification,
	}, input.Payload)
	if err != nil {
		return integration.ProcessResult{}, ErrInvalidInput
	}
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModeProduction,
		IntegrationRevision: binding.IntegrationRevision,
		Security: integration.SecurityContext{
			TenantID: s.tenantID,
			Principal: integration.Principal{
				ID:         s.principalID,
				Kind:       integration.PrincipalKindService,
				AuthMethod: s.authMethod,
				Roles:      []string{SubmitRole},
				SourceID:   binding.SourceID,
			},
		},
		Envelope:       envelope,
		IdempotencyKey: input.IdempotencyKey,
		CorrelationID:  correlationID,
	}
	result, err := s.processor.Process(ctx, request)
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return integration.ProcessResult{}, err
	case errors.Is(err, processor.ErrTenantMismatch):
		return integration.ProcessResult{}, ErrForbidden
	case errors.Is(err, processor.ErrInvalidSourceMessage):
		return integration.ProcessResult{}, ErrInvalidMessage
	case errors.Is(err, processor.ErrIdempotencyConflict):
		return integration.ProcessResult{}, ErrIdempotencyConflict
	case errors.Is(err, processor.ErrInvalidProcessRequest):
		return integration.ProcessResult{}, ErrInvalidInput
	default:
		return integration.ProcessResult{}, ErrRetryable
	}
}

func validHeaderValue(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	return !containsControl(value)
}
