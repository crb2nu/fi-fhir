package processor

import (
	"context"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrProcessorUnavailable means the shared processor was not fully configured.
	ErrProcessorUnavailable = errors.New("message processor unavailable")
	// ErrProductionCommitterRequired keeps production fail-closed until Slice 1.2 durability exists.
	ErrProductionCommitterRequired = errors.New("production processing requires durable committer")
	// ErrDurableSubmissionFailed maps PostgreSQL admission failures to a catalog-safe error.
	ErrDurableSubmissionFailed = errors.New("durable production submission failed")
	// ErrIdempotencyConflict means an effective key was reused for different request content.
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with durable submission")
	// ErrInvalidProcessRequest means the request does not match the server-owned revision.
	ErrInvalidProcessRequest = errors.New("invalid process request")
	// ErrDefinitionResolutionFailed maps server definition lookup details to a catalog-safe error.
	ErrDefinitionResolutionFailed = errors.New("integration definition resolution failed")
	// ErrArtifactResolutionFailed maps executable artifact lookup details to a catalog-safe error.
	ErrArtifactResolutionFailed = errors.New("execution artifact resolution failed")
	// ErrProfileCompilationFailed means exact profile bytes are not executable by the v1 kernel.
	ErrProfileCompilationFailed = errors.New("source profile compilation failed")
	// ErrInvalidSourceMessage means raw bytes cannot produce the supported canonical A01 event.
	ErrInvalidSourceMessage = errors.New("source message is invalid or unsupported")
	// ErrWorkflowPlanningFailed means exact workflow bytes cannot produce a bound pure plan.
	ErrWorkflowPlanningFailed = errors.New("workflow planning failed")
	// ErrInvalidProcessResult means the processor violated its public preview contract.
	ErrInvalidProcessResult = errors.New("invalid process result")
)

// MaxPreviewSourceBytes is the hard kernel limit for the initial ADT A01 path.
// Transport adapters must apply an equal or smaller limit before buffering.
const MaxPreviewSourceBytes int64 = 1 << 20

// MessageProcessor is the single preview and production semantic path. Preview
// owns no side effects. Production is available only when the PostgreSQL
// submission store is explicitly configured; destination execution remains an
// outbox consumer responsibility.
type MessageProcessor struct {
	definitions *DefinitionRevisionResolver
	artifacts   *RevisionResolver
	submissions *PostgresSubmissionStore
}

type durableSubmissionError struct {
	cause error
}

func (e *durableSubmissionError) Error() string {
	return ErrDurableSubmissionFailed.Error()
}

func (e *durableSubmissionError) Unwrap() error {
	return e.cause
}

func (e *durableSubmissionError) Is(target error) bool {
	return target == ErrDurableSubmissionFailed
}

func durableSubmissionCause(err error) error {
	var submissionError *durableSubmissionError
	if !errors.As(err, &submissionError) {
		return nil
	}
	return submissionError.cause
}

// NewMessageProcessor composes server-owned definition and exact artifact resolvers.
func NewMessageProcessor(
	definitions *DefinitionRevisionResolver,
	artifacts *RevisionResolver,
) (*MessageProcessor, error) {
	if definitions == nil || artifacts == nil {
		return nil, ErrProcessorUnavailable
	}
	return &MessageProcessor{definitions: definitions, artifacts: artifacts}, nil
}

// NewDurableMessageProcessor composes the shared evaluator with the only
// supported production admission implementation: PostgreSQL.
func NewDurableMessageProcessor(
	definitions *DefinitionRevisionResolver,
	artifacts *RevisionResolver,
	submissions *PostgresSubmissionStore,
) (*MessageProcessor, error) {
	if definitions == nil || artifacts == nil || submissions == nil {
		return nil, ErrProcessorUnavailable
	}
	return &MessageProcessor{
		definitions: definitions,
		artifacts:   artifacts,
		submissions: submissions,
	}, nil
}

// Process evaluates one exact request. Preview is side-effect-free; production
// returns only after its complete admission unit commits in PostgreSQL.
func (p *MessageProcessor) Process(
	ctx context.Context,
	request integration.ProcessRequest,
) (integration.ProcessResult, error) {
	if request.Mode == integration.ExecutionModeProduction && (p == nil || p.submissions == nil) {
		return integration.ProcessResult{}, ErrProductionCommitterRequired
	}
	if request.Mode != integration.ExecutionModePreview && request.Mode != integration.ExecutionModeProduction {
		return integration.ProcessResult{}, ErrInvalidProcessRequest
	}
	if p == nil || p.definitions == nil || p.artifacts == nil || ctx == nil {
		return integration.ProcessResult{}, ErrProcessorUnavailable
	}
	if err := ctx.Err(); err != nil {
		return integration.ProcessResult{}, err
	}
	if size := request.Envelope.PayloadSizeBytes(); size <= 0 || size > MaxPreviewSourceBytes {
		return integration.ProcessResult{}, ErrInvalidProcessRequest
	}

	revision, err := p.definitions.Resolve(ctx, request.Security.TenantID, request.IntegrationRevision)
	if err != nil {
		if cancellation := contextError(err, ctx); cancellation != nil {
			return integration.ProcessResult{}, cancellation
		}
		if errors.Is(err, ErrTenantMismatch) {
			return integration.ProcessResult{}, ErrTenantMismatch
		}
		return integration.ProcessResult{}, ErrDefinitionResolutionFailed
	}
	if err := request.ValidateAgainst(revision); err != nil {
		return integration.ProcessResult{}, ErrInvalidProcessRequest
	}

	resolved, err := p.artifacts.Resolve(ctx, revision.TenantID, revision.Profile, revision.Workflow)
	if err != nil {
		if cancellation := contextError(err, ctx); cancellation != nil {
			return integration.ProcessResult{}, cancellation
		}
		if errors.Is(err, ErrTenantMismatch) {
			return integration.ProcessResult{}, ErrTenantMismatch
		}
		return integration.ProcessResult{}, ErrArtifactResolutionFailed
	}
	compiledProfile, err := compileSourceProfile(resolved)
	if err != nil {
		return integration.ProcessResult{}, ErrProfileCompilationFailed
	}
	if revision.Format != events.FormatHL7v2 {
		return integration.ProcessResult{}, ErrInvalidSourceMessage
	}

	parser := hl7v2.NewParser(revision.Source.SourceID, hl7v2.ParserConfig{
		DefaultTimezone:  compiledProfile.timezone,
		ExtractZSegments: false,
		StrictValidation: true,
	})
	parser.SetProfile(compiledProfile.source)
	parsed, err := parser.ParseWithResult(string(request.Envelope.Bytes()))
	if err != nil {
		return integration.ProcessResult{}, ErrInvalidSourceMessage
	}
	event, parseDiagnostics, err := projectADTA01(parsed, request, revision, 0)
	if err != nil {
		return integration.ProcessResult{}, ErrInvalidSourceMessage
	}

	routes, deliveries, workflowDiagnostics, err := planWorkflow(resolved, event, revision, request.Mode)
	if err != nil {
		return integration.ProcessResult{}, ErrWorkflowPlanningFailed
	}
	security := request.Security
	security.Principal.Roles = append([]string(nil), request.Security.Principal.Roles...)
	result := integration.ProcessResult{
		Mode:                request.Mode,
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		ArtifactRevisions: &integration.ExecutionArtifactRevisions{
			Source:   revision.Source.ArtifactRevisionRef,
			Profile:  resolved.ProfileReference(),
			Workflow: resolved.WorkflowReference(),
		},
		Security:    security,
		Events:      []integration.ProcessedEvent{event},
		Diagnostics: append(parseDiagnostics, workflowDiagnostics...),
		Routes:      routes,
		Deliveries:  deliveries,
		Correlations: integration.CorrelationIDs{
			TenantID:        revision.TenantID,
			CorrelationID:   request.CorrelationID,
			SourceMessageID: event.SourceMessageID,
			EventIDs:        []string{event.ID},
		},
	}
	if request.Mode == integration.ExecutionModePreview {
		if err := result.ValidatePreviewFor(request, revision); err != nil {
			return integration.ProcessResult{}, ErrInvalidProcessResult
		}
		return result, nil
	}
	durable, err := p.submissions.commit(ctx, request, revision, result)
	if err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return integration.ProcessResult{}, ErrIdempotencyConflict
		}
		return integration.ProcessResult{}, &durableSubmissionError{cause: err}
	}
	return durable, nil
}

func contextError(err error, ctx context.Context) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case ctx != nil && ctx.Err() != nil:
		return ctx.Err()
	default:
		return nil
	}
}
