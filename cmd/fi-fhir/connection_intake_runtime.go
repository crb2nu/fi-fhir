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
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
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
	// The destination identity runtime's env/file resolver, narrowed to the
	// secrets provisioned for connections (connectionSecretResolver).
	resolver, err := newDestinationSecretResolver(strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR")))
	if err != nil {
		return false, fmt.Errorf("configure connection peek secret resolver: %w", err)
	}
	if err := binding.service.EnableSampleIntake(connection.IntakeConfig{
		Sessions: binding.sessions, Secrets: connectionSecretResolver{inner: resolver}, Registry: registry, Observer: observer,
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

// The only secrets a connection peek may resolve. A peek resolves the
// bindings of a draft any integration.deployment.operator can write, and hands
// them to a provider that contacts an endpoint the same draft names — an S3
// request carries the access key in its Authorization header. Without these
// prefixes a draft could bind any process variable or any destination
// credential and read it back at an endpoint of its own.
const (
	// connectionSecretEnvPrefix names the env family a peek may read.
	connectionSecretEnvPrefix = "FI_FHIR_CONNECTION_SECRET_"
	// connectionSecretFilePrefix is the one subtree of
	// FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR a peek may read; the destination
	// credentials beside it are not a peek's.
	connectionSecretFilePrefix = "connections/"
)

// connectionSecretResolver narrows serve's env/file resolver to the secrets
// provisioned for connections: env keys named FI_FHIR_CONNECTION_SECRET_*,
// and files under connections/ in FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR (the
// inner resolver already refuses a path that escapes the directory). Every
// other reference is integration.ErrSecretUnresolvable before the inner
// resolver reads anything, so the peek reports SECRET_UNRESOLVABLE and
// contacts nothing.
type connectionSecretResolver struct {
	inner integration.SecretResolver
}

func (r connectionSecretResolver) Resolve(ctx context.Context, reference integration.SecretReference) ([]byte, error) {
	if r.inner == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	var allowed bool
	switch reference.Provider {
	case integration.SecretProviderEnvironment:
		name, ok := strings.CutPrefix(reference.Key, connectionSecretEnvPrefix)
		allowed = ok && name != ""
	case integration.SecretProviderFile:
		name, ok := strings.CutPrefix(reference.Key, connectionSecretFilePrefix)
		allowed = ok && name != ""
	}
	if !allowed {
		return nil, integration.ErrSecretUnresolvable
	}
	return r.inner.Resolve(ctx, reference)
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
