package processor

import (
	"context"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type Phase string

const (
	PhaseAdmission    Phase = "admission"
	PhaseWorkflowPlan Phase = "workflow_plan"
)

// Observation deliberately excludes payloads, source message IDs, and error
// text. CorrelationID is caller-controlled; consumers must not log it verbatim.
type Observation struct {
	Phase         Phase
	Mode          integration.ExecutionMode
	CorrelationID string
	ReceiptID     string
	TraceID       string
	Failed        bool
}

// Observer starts an operation and returns its completion callback. Both must
// be non-blocking. Implementations can propagate context without coupling the
// semantic processor to a logger or tracing SDK.
type Observer func(context.Context, Observation) (context.Context, func(Observation))

func (p *MessageProcessor) SetObserver(observer Observer) {
	p.observerMu.Lock()
	defer p.observerMu.Unlock()
	p.observer = observer
}

func (p *MessageProcessor) beginObservation(ctx context.Context, observation Observation) (context.Context, func(Observation)) {
	p.observerMu.RLock()
	observer := p.observer
	p.observerMu.RUnlock()
	if observer == nil {
		return ctx, func(Observation) {}
	}
	return observer(ctx, observation)
}
