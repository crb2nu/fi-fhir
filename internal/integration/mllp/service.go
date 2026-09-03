package mllp

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const SubmitRole = authorization.MLLPSubmitGrant

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
	tenantID       string
	definitionID   string
	principalID    string
	authMethod     string
	identityMapped bool
	source         SourceRevision
	resolver       RunnableResolver
	processor      MessageProcessor
	capacity       *capacityGate
	now            func() time.Time
	newID          func() string

	observeMu sync.RWMutex
	observe   func(SubmitResult, error)
}

// SubmitResult reports one MLLP frame's admission outcome.
//
// Same shape as autoroute.SweeperConfig.Observe: a typed result, an optional
// non-blocking hook, no metrics dependency inside this package. Nothing
// message-derived is carried: an observer that wanted a message control ID
// would be building an unbounded metric label.
type SubmitResult struct {
	// Accepted is true when the frame reached durable admission.
	Accepted bool
	// Duration is how long the submission took.
	Duration time.Duration
}

// SetObserver binds an observation hook. It must not block.
func (s *Service) SetObserver(observe func(SubmitResult, error)) {
	if s == nil {
		return
	}
	s.observeMu.Lock()
	defer s.observeMu.Unlock()
	s.observe = observe
}

func (s *Service) report(result SubmitResult, err error) {
	if s == nil {
		return
	}
	s.observeMu.RLock()
	observe := s.observe
	s.observeMu.RUnlock()
	if observe == nil {
		return
	}
	observe(result, err)
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
	authMethod := AuthMethodAllowlist
	if config.Source.TLS.Mode == TLSModeMutual {
		authMethod = AuthMethodMutualTLS
	}
	return &Service{
		tenantID: config.TenantID, definitionID: config.DefinitionID,
		principalID: config.PrincipalID, authMethod: authMethod,
		identityMapped: config.Source.Clients.IdentityMappingEnabled(),
		source:         config.Source, resolver: config.Resolver, processor: config.Processor,
		capacity: newCapacityGate(clock), now: clock, newID: newID,
	}, nil
}

// resolvePrincipal converts the verified per-connection certificate identity
// into server-owned principal fields. It fails closed rather than degrading a
// mapped listener to the deployment-fixed compatibility principal.
func (s *Service) resolvePrincipal(identity ConnectionIdentity) (integration.Principal, error) {
	if !s.identityMapped {
		if !identity.zero() {
			return integration.Principal{}, ErrUnavailable
		}
		return integration.Principal{
			ID: s.principalID, Kind: integration.PrincipalKindService,
			AuthMethod: s.authMethod, Roles: []string{SubmitRole},
		}, nil
	}
	if !validIdentity(identity.Subject) || identity.AuthMethod != AuthMethodCertificateIdentity {
		return integration.Principal{}, ErrUnavailable
	}
	return integration.Principal{
		ID: identity.Subject, Kind: integration.PrincipalKindService,
		AuthMethod: identity.AuthMethod, Roles: append([]string(nil), identity.Grants...),
	}, nil
}

// Submit admits one framed HL7v2 payload under the verified connection
// identity. In compatibility mode the identity is the zero value and the
// deployment-fixed principal applies.
// Submit admits one MLLP frame and observes the outcome.
//
// Observation wraps the whole admission path, including the fail-closed
// rejections above the durable write, so a listener that is rejecting every
// frame is visible rather than merely quiet.
func (s *Service) Submit(ctx context.Context, identity ConnectionIdentity, payload []byte) (integration.ProcessResult, error) {
	if s == nil {
		return integration.ProcessResult{}, ErrUnavailable
	}
	started := time.Now()
	result, err := s.submit(ctx, identity, payload)
	s.report(SubmitResult{Accepted: err == nil, Duration: time.Since(started)}, err)
	return result, err
}

func (s *Service) submit(ctx context.Context, identity ConnectionIdentity, payload []byte) (integration.ProcessResult, error) {
	if s == nil || s.resolver == nil || s.processor == nil || s.capacity == nil ||
		s.now == nil || s.newID == nil || ctx == nil {
		return integration.ProcessResult{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return integration.ProcessResult{}, err
	}
	principal, err := s.resolvePrincipal(identity)
	if err != nil {
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
	// Source identity stays server-owned: it is bound from the exact deployed
	// release, never from the certificate or the message.
	principal.SourceID = binding.SourceID
	security := integration.SecurityContext{TenantID: s.tenantID, Principal: principal}
	if err := authorization.AuthorizeSubmission(
		security,
		s.tenantID,
		binding.IntegrationRevision,
		binding.SourceID,
	); err != nil {
		return integration.ProcessResult{}, ErrUnavailable
	}

	// Queued messages have already been admitted by MaxQueued. Do not spend
	// their processing budget while they wait for an in-flight slot.
	release, err := s.capacity.acquire(ctx, binding.Deployment.Capacity, binding.IntegrationRevision.Digest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return integration.ProcessResult{}, ErrRetryable
		}
		return integration.ProcessResult{}, err
	}
	defer release()
	processCtx, cancel := context.WithTimeout(ctx, time.Duration(s.source.Timeouts.ProcessSeconds)*time.Second)
	defer cancel()

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
		Security: security, Envelope: envelope, CorrelationID: correlationID,
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
