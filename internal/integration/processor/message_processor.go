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

// MessageProcessor is the single preview semantic path. It owns no committer,
// destination client, action handler, event store, session store, or clock.
type MessageProcessor struct {
	definitions *DefinitionRevisionResolver
	artifacts   *RevisionResolver
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

// Process evaluates one exact, side-effect-free preview request.
func (p *MessageProcessor) Process(
	ctx context.Context,
	request integration.ProcessRequest,
) (integration.ProcessResult, error) {
	if request.Mode != integration.ExecutionModePreview {
		return integration.ProcessResult{}, ErrProductionCommitterRequired
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

	routes, deliveries, workflowDiagnostics, err := planPreviewWorkflow(resolved, event, revision)
	if err != nil {
		return integration.ProcessResult{}, ErrWorkflowPlanningFailed
	}
	security := request.Security
	security.Principal.Roles = append([]string(nil), request.Security.Principal.Roles...)
	result := integration.ProcessResult{
		Mode:                integration.ExecutionModePreview,
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
	if err := result.ValidatePreviewFor(request, revision); err != nil {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	return result, nil
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
