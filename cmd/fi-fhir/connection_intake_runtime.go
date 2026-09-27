package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	integrationingress "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	integrationsession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// connectionIntakeRuntime is .loom/38 Lane C-2's composition in serve: the
// capture taps around the two admission processors, and — once the connection
// catalog and the session workspace exist — the armed-capture cache, the peek's
// secret resolver, and the observer that meters both.
//
// It is built in two steps because the pieces exist at two different points.
// loadIntegrationRuntimeFromEnv constructs the MLLP listener and the HTTP
// ingress service with their processors, before the session store and long
// before runServe builds the catalog service; the taps are wrapped there and
// stay pass-throughs until bind. bind runs in runServe beside the catalog
// service C-0 builds, the one point where everything intake needs exists.
type connectionIntakeRuntime struct {
	// taps is nil when this replica has no session workspace, and then the
	// processors are not wrapped at all.
	taps *connection.CaptureTaps
}

// newConnectionIntakeRuntime decides, at the start of composition, whether
// admission is tapped. Intake needs the session workspace (there is nowhere
// else to put a sample) and the connection catalog; serve builds the catalog
// whenever the durable database exists, and a session workspace always opens
// it, so the session switch alone decides.
func newConnectionIntakeRuntime(sessionWorkspaceEnabled bool) *connectionIntakeRuntime {
	if !sessionWorkspaceEnabled {
		return &connectionIntakeRuntime{}
	}
	return &connectionIntakeRuntime{taps: connection.NewCaptureTaps()}
}

// tapMLLP wraps the MLLP listener's processor where preview_runtime.go builds
// the listener. The listener's contract is unchanged: the tap returns exactly
// what the processor returned.
func (r *connectionIntakeRuntime) tapMLLP(inner mllp.MessageProcessor) mllp.MessageProcessor {
	if r == nil || r.taps == nil {
		return inner
	}
	return r.taps.Wrap(inner)
}

// tapIngress wraps the HTTP ingress service's processor the same way.
func (r *connectionIntakeRuntime) tapIngress(inner integrationingress.Processor) integrationingress.Processor {
	if r == nil || r.taps == nil {
		return inner
	}
	return r.taps.Wrap(inner)
}

// connectionIntakeBinding is what bind needs from runServe.
type connectionIntakeBinding struct {
	tenantID string
	service  *connection.Service
	store    *connection.PostgresStore
	sessions integrationsession.Store
	metrics  *observability.Metrics
	logger   *slog.Logger
}

// bind enables sample intake on the catalog service and activates the taps.
// It starts the armed-capture cache's refresh on ctx. Like the lifecycle
// health reporter, the refresh is not in the background component table: a
// failed refresh is counted and logged, and must never stop the process.
// It reports whether intake is on.
func (r *connectionIntakeRuntime) bind(ctx context.Context, binding connectionIntakeBinding) (bool, error) {
	if r == nil || r.taps == nil || binding.service == nil || binding.store == nil || binding.sessions == nil {
		return false, nil
	}
	observer := captureObserver(binding.metrics, binding.logger)
	registry, err := connection.NewCaptureRegistry(connection.CaptureRegistryConfig{
		Store: binding.store, TenantID: binding.tenantID, Observer: observer,
	})
	if err != nil {
		return false, fmt.Errorf("configure connection capture cache: %w", err)
	}
	// The destination identity runtime's env/file resolver: env references
	// resolve from the process environment, file references under
	// FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR when it is set, and nothing else.
	resolver, err := newDestinationSecretResolver(strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR")))
	if err != nil {
		return false, fmt.Errorf("configure connection peek secret resolver: %w", err)
	}
	if err := binding.service.EnableSampleIntake(connection.IntakeConfig{
		Sessions: binding.sessions, Secrets: resolver, Registry: registry, Observer: observer,
	}); err != nil {
		return false, fmt.Errorf("enable connection sample intake: %w", err)
	}
	if err := r.taps.Bind(connection.CaptureTapBinding{
		Registry: registry, Sessions: binding.sessions, Observer: observer,
	}); err != nil {
		return false, fmt.Errorf("bind connection capture taps: %w", err)
	}
	go func() { _ = registry.Run(ctx) }()
	return true, nil
}

// captureObserver meters and logs sample intake. The error text a tap
// failure carries comes from a store or the tap itself, never from a message.
func captureObserver(metrics *observability.Metrics, logger *slog.Logger) connection.CaptureObserver {
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	return connection.CaptureObserver{
		Captured: func(mode connection.CaptureMode, messages int) {
			metrics.RecordConnectionCaptureMessages(string(mode), messages)
		},
		TapFailed: func(reason connection.TapFailure, err error) {
			metrics.RecordConnectionCaptureTapError(string(reason))
			logger.Warn("connection capture tap failure; admission was not affected",
				observability.F(observability.FieldComponent, "connection-capture"),
				observability.F(observability.FieldReason, string(reason)),
				observability.F(observability.FieldError, observability.Errf(err)))
		},
	}
}
