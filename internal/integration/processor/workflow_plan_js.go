//go:build js

package processor

import (
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// ErrWorkflowUnavailable means this build of the processor does not link the
// workflow planner. Only the browser kernel (GOOS=js, cmd/fi-fhir-wasm) is
// built this way: it compiles Source Profiles and parses messages, and the
// planner's dependency closure (internal/workflow) would multiply the module's
// size by fourteen. MessageProcessor.Process reports it as
// ErrWorkflowPlanningFailed, so a js build fails closed rather than planning
// nothing.
var ErrWorkflowUnavailable = errors.New("workflow planner is not linked into this build")

func planWorkflow(
	ResolvedArtifactRevisions,
	integration.ProcessedEvent,
	integration.IntegrationDefinitionRevision,
	integration.ExecutionMode,
) ([]integration.RouteResult, []integration.DeliveryResult, []integration.Diagnostic, error) {
	return nil, nil, nil, ErrWorkflowUnavailable
}
