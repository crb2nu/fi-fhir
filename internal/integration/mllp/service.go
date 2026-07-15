package mllp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const SubmitRole = "integration:mllp"

var (
	ErrUnavailable         = errors.New("MLLP source is unavailable")
	ErrInvalidMessage      = errors.New("invalid MLLP HL7v2 message")
	ErrIdempotencyConflict = errors.New("MLLP message identity conflicts with a different request")
	ErrRetryable           = errors.New("MLLP durable submission is temporarily unavailable")
)

type RunnableResolver interface {
	ResolveRunnable(ctx context.Context, tenantID, definitionID string) (lifecycle.RunnableBinding, error)
}

type MessageProcessor interface {
	Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error)
}

type ServiceConfig struct {
	TenantID     string
	DefinitionID string
	PrincipalID  string
	Source       SourceRevision
	Resolver     RunnableResolver
	Processor    MessageProcessor
	Clock        func() time.Time
	NewID        func() string
}

type Service struct {
	tenantID     string
	definitionID string
	principalID  string
	authMethod   string
	source       SourceRevision
	resolver     RunnableResolver
	processor    MessageProcessor
	capacity     *capacityGate
	now          func() time.Time
	newID        func() string
}

func NewService(config ServiceConfig) (*Service, error) {
	if !validIdentity(config.TenantID) || !validIdentity(config.DefinitionID) ||
		!validIdentity(config.PrincipalID) || config.Source.Validate() != nil ||
		config.Resolver == nil || config.Processor == nil {
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
	authMethod := "mllp-allowlist"
	if config.Source.TLS.Mode == TLSModeMutual {
		authMethod = "mllp-mtls"
	}
	return &Service{
		tenantID: config.TenantID, definitionID: config.DefinitionID,
		principalID: config.PrincipalID, authMethod: authMethod,
		source: config.Source, resolver: config.Resolver, processor: config.Processor,
		capacity: newCapacityGate(clock), now: clock, newID: newID,
	}, nil
}

func (s *Service) Submit(ctx context.Context, payload []byte) (integration.ProcessResult, error) {
	if s == nil || s.resolver == nil || s.processor == nil || s.capacity == nil ||
		s.now == nil || s.newID == nil || ctx == nil {
		return integration.ProcessResult{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return integration.ProcessResult{}, err
	}
	if len(payload) == 0 || int64(len(payload)) > s.source.MaxMessageBytes {
		return integration.ProcessResult{}, ErrInvalidMessage
	}
	binding, err := s.resolver.ResolveRunnable(ctx, s.tenantID, s.definitionID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return integration.ProcessResult{}, err
		}
		return integration.ProcessResult{}, ErrUnavailable
	}
	if err := s.source.ValidateAgainst(binding); err != nil {
		return integration.ProcessResult{}, ErrUnavailable
	}

	processCtx, cancel := context.WithTimeout(ctx, time.Duration(s.source.Timeouts.ProcessSeconds)*time.Second)
	defer cancel()
	release, err := s.capacity.acquire(processCtx, binding.Deployment.Capacity, binding.IntegrationRevision.Digest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return integration.ProcessResult{}, ErrRetryable
		}
		return integration.ProcessResult{}, err
	}
	defer release()

	receivedAt := s.now().UTC()
	correlationID := s.newID()
	if receivedAt.IsZero() || !validToken(correlationID) {
		return integration.ProcessResult{}, ErrUnavailable
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID: s.tenantID, SourceID: binding.SourceID, Format: events.FormatHL7v2,
		ContentType: "application/hl7-v2+er7", ReceivedAt: receivedAt,
		Classification: binding.Classification,
	}, payload)
	if err != nil {
		return integration.ProcessResult{}, ErrInvalidMessage
	}
	request := integration.ProcessRequest{
		Mode: integration.ExecutionModeProduction, IntegrationRevision: binding.IntegrationRevision,
		Security: integration.SecurityContext{
			TenantID: s.tenantID,
			Principal: integration.Principal{
				ID: s.principalID, Kind: integration.PrincipalKindService,
				AuthMethod: s.authMethod, Roles: []string{SubmitRole}, SourceID: binding.SourceID,
			},
		},
		Envelope: envelope, CorrelationID: correlationID,
	}
	result, err := s.processor.Process(processCtx, request)
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return integration.ProcessResult{}, ErrRetryable
	case errors.Is(err, processor.ErrInvalidSourceMessage),
		errors.Is(err, processor.ErrInvalidProcessRequest):
		return integration.ProcessResult{}, ErrInvalidMessage
	case errors.Is(err, processor.ErrIdempotencyConflict):
		return integration.ProcessResult{}, ErrIdempotencyConflict
	case errors.Is(err, lifecycle.ErrNotRunnable):
		return integration.ProcessResult{}, ErrUnavailable
	default:
		return integration.ProcessResult{}, ErrRetryable
	}
}
