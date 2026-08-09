package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	integrationsession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Slice 4.4d: the observation adapters are where the process logger reaches the
// durable components.
//
// The rule this file exists to keep is recorded at the bottom of
// `internal/observability/metrics.go`: `internal/integration/*` holds no
// observability dependency, so the middleware and the observers are wrapped
// here, in `cmd/`. A logger is the same kind of dependency as a metrics
// registry, so it obeys the same rule — the adapters close over `logger`, and
// no `internal/integration` package imports one.
//
// # What these lines can and cannot correlate
//
// Every line carries `tenant_id`, because the logger binds it. None of them
// carries `correlation_id` or `trace_id`, and that is a property of the seam
// rather than an oversight: the four observation callbacks receive
// `(Result, error)` and no context, and none of the four Result types carries a
// lineage identifier — `mllp.SubmitResult` is `{Accepted, Duration}`,
// `delivery.DispatchResult` is `{Outcome, Duration}`, `batch.PollResult` is
// `{Objects, Duration}`, and `session.StreamOutcome` is a bare enum. Emitting a
// correlation identifier from here means widening those structs, which is an
// `internal/integration/**` edit this slice deliberately does not make.
//
// That is consistent with the position `README.md` already takes: correlation
// comes from the durable identifiers, and a stage line joins to a submission
// through the ledger rows, not through a value the counter callback happened to
// carry. See the Slice 4.4d entry in `.loom/40-decisions.md`.

// bindMLLPObservation reports every admitted or rejected frame.
func bindMLLPObservation(service *mllp.Service, metrics *observability.Metrics, logger *slog.Logger) {
	if service == nil || metrics == nil {
		return
	}
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	service.SetObserver(func(result mllp.SubmitResult, err error) {
		switch {
		case err != nil:
			metrics.RecordMLLPMessage(observability.OutcomeRejected)
			logger.Warn("MLLP frame rejected",
				observability.F(observability.FieldComponent, "mllp"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeRejected)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()),
				observability.F(observability.FieldError, observability.Errf(err)))
		case result.Accepted:
			metrics.RecordMLLPMessage(observability.OutcomeAccepted)
			// One line per admitted frame is the ingress rate, so it is debug.
			logger.Debug("MLLP frame accepted",
				observability.F(observability.FieldComponent, "mllp"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeAccepted)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		default:
			metrics.RecordMLLPMessage(observability.OutcomeError)
			logger.Error("MLLP frame neither accepted nor rejected",
				observability.F(observability.FieldComponent, "mllp"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeError)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		}
	})
}

// bindMLLPRateQuotaObservation reports each claim attempt, and distinguishes a
// replica running on its authoritative share from one running on the
// conservative fallback.
//
// Claim-interval cardinality, not frame cardinality. If this counter ever rises
// in step with fi_fhir_mllp_messages_total, admission has started taking a
// database round trip — the design slice 4.4e rejected — and that is visible
// from the two series alone.
func bindMLLPRateQuotaObservation(coordinator *mllp.QuotaCoordinator, metrics *observability.Metrics) {
	if coordinator == nil || metrics == nil {
		return
	}
	coordinator.SetObserver(func(outcome mllp.QuotaOutcome) {
		switch {
		case outcome.Released:
			// A released share is a clean handback, not a claim.
			return
		case outcome.Degraded:
			metrics.RecordMLLPRateClaim(observability.OutcomeDegraded)
		case outcome.Err != nil:
			metrics.RecordMLLPRateClaim(observability.OutcomeError)
		default:
			metrics.RecordMLLPRateClaim(observability.OutcomeProcessed)
		}
	})
}

// bindDeliveryObservation reports the typed Outcome the dispatcher already
// computes and Run previously discarded.
func bindDeliveryObservation(dispatcher *integrationdelivery.Dispatcher, metrics *observability.Metrics, logger *slog.Logger) {
	if dispatcher == nil || metrics == nil {
		return
	}
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	dispatcher.SetObserver(func(result integrationdelivery.DispatchResult, err error) {
		if err != nil {
			metrics.RecordDeliveryAttempt(observability.OutcomeError)
			logger.Error("delivery dispatch failed",
				observability.F(observability.FieldComponent, "delivery-worker"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeError)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()),
				observability.F(observability.FieldError, observability.Errf(err)))
			return
		}
		switch result.Outcome {
		case integrationdelivery.OutcomePublished:
			metrics.RecordDeliveryAttempt(observability.OutcomeDelivered)
			logger.Debug("delivery published",
				observability.F(observability.FieldComponent, "delivery-worker"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeDelivered)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		case integrationdelivery.OutcomeRetry:
			metrics.RecordDeliveryAttempt(observability.OutcomeRetried)
			logger.Warn("delivery scheduled for retry",
				observability.F(observability.FieldComponent, "delivery-worker"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeRetried)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		case integrationdelivery.OutcomeDLQ:
			metrics.RecordDeliveryAttempt(observability.OutcomeFailed)
			logger.Error("delivery exhausted its retries and moved to the dead-letter path",
				observability.F(observability.FieldComponent, "delivery-worker"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeFailed)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		default:
			metrics.RecordDeliveryAttempt(observability.OutcomeIdle)
		}
	})
}

// bindBatchObservation reports each batch poll cycle.
func bindBatchObservation(runner *integrationbatch.Runner, metrics *observability.Metrics, logger *slog.Logger) {
	if runner == nil || metrics == nil {
		return
	}
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	runner.SetObserver(func(result integrationbatch.PollResult, err error) {
		switch {
		case err != nil:
			metrics.RecordBatchObject(observability.OutcomeFailed)
			logger.Error("batch poll failed",
				observability.F(observability.FieldComponent, "batch-runner"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeFailed)),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()),
				observability.F(observability.FieldError, observability.Errf(err)))
		case result.Objects > 0:
			for i := 0; i < result.Objects; i++ {
				metrics.RecordBatchObject(observability.OutcomeProcessed)
			}
			// One line per poll cycle, not per object: the object count is a
			// field, so a busy poll does not become N lines.
			logger.Info("batch poll processed objects",
				observability.F(observability.FieldComponent, "batch-runner"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeProcessed)),
				observability.F(observability.FieldCount, result.Objects),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		default:
			metrics.RecordBatchObject(observability.OutcomeIdle)
		}
	})
}

// sessionStreamObserver reports durable session fanout transitions.
func sessionStreamObserver(metrics *observability.Metrics, logger *slog.Logger) func(integrationsession.StreamOutcome, error) {
	if metrics == nil {
		return nil
	}
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	return func(outcome integrationsession.StreamOutcome, err error) {
		if err != nil {
			metrics.RecordSessionStreamEvent(observability.OutcomeError)
			logger.Error("session stream fanout failed",
				observability.F(observability.FieldComponent, "session-stream-relay"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeError)),
				observability.F(observability.FieldError, observability.Errf(err)))
			return
		}
		switch outcome {
		case integrationsession.StreamOutcomePublished:
			metrics.RecordSessionStreamEvent(observability.OutcomePublished)
		case integrationsession.StreamOutcomeReplayed:
			metrics.RecordSessionStreamEvent(observability.OutcomeReplayed)
		case integrationsession.StreamOutcomeDropped:
			metrics.RecordSessionStreamEvent(observability.OutcomeDropped)
			logger.Warn("session stream event dropped",
				observability.F(observability.FieldComponent, "session-stream-relay"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeDropped)))
		default:
			metrics.RecordSessionStreamEvent(observability.OutcomeError)
		}
	}
}

// newSessionStreamRelay builds the cross-replica fanout relay when the session
// store can carry the durable envelope log.
//
// A store that cannot returns (nil, nil): the in-memory workspace keeps
// in-process fanout, which is correct for a single process.
func newSessionStreamRelay(
	store integrationsession.Store,
	hub *integrationsession.Hub,
	metrics *observability.Metrics,
	mode observability.Mode,
	logger *slog.Logger,
) (*integrationsession.StreamRelay, error) {
	if store == nil || hub == nil || mode.Legacy() || !hub.Durable() {
		return nil, nil
	}
	log, ok := store.(integrationsession.StreamLog)
	if !ok {
		return nil, nil
	}
	return integrationsession.NewStreamRelay(integrationsession.RelayConfig{
		Log:     log,
		Hub:     hub,
		Observe: sessionStreamObserver(metrics, logger),
	})
}

// autorouteSweepObserver keeps the operator-visible reporting the Slice 4.3
// adapters introduced and adds metrics, rather than replacing operator-visible
// output with a counter. Slice 4.4d converts that output from ad-hoc
// stderr/stdout writes to the process logger; the three adapters below were the
// only observers that already printed, which makes them the model for the rest.
func autorouteSweepObserver(metrics *observability.Metrics, logger *slog.Logger) func(autoroute.SweepResult, error) {
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	return func(result autoroute.SweepResult, err error) {
		if err != nil {
			metrics.RecordAutorouteSweep(observability.OutcomeError, 0)
			logger.Warn("pending autoroute sweep failed",
				observability.F(observability.FieldComponent, "autoroute-sweep"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeError)),
				observability.F(observability.FieldError, observability.Errf(err)))
			return
		}
		metrics.RecordAutorouteSweep(observability.OutcomeProcessed, int(result.Expired))
		if result.Expired > 0 {
			logger.Info("pending autoroute sweep expired candidates",
				observability.F(observability.FieldComponent, "autoroute-sweep"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeProcessed)),
				observability.F(observability.FieldCount, result.Expired),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
		}
	}
}

func autorouteNotifyObserver(metrics *observability.Metrics, logger *slog.Logger) func(autoroute.NotifyResult, error) {
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	return func(result autoroute.NotifyResult, err error) {
		if err != nil {
			metrics.RecordAutorouteNotification(observability.OutcomeError)
			logger.Warn("pending autoroute review scan failed",
				observability.F(observability.FieldComponent, "autoroute-notify"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeError)),
				observability.F(observability.FieldError, observability.Errf(err)))
			return
		}
		if result.Dropped > 0 {
			metrics.RecordAutorouteNotification(observability.OutcomeDropped)
			logger.Warn("pending autoroute review notification dropped: dispatch queue full",
				observability.F(observability.FieldComponent, "autoroute-notify"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeDropped)),
				observability.F(observability.FieldCount, result.Eligible),
				observability.F(observability.FieldAttempt, result.New))
			return
		}
		if result.Queued > 0 {
			metrics.RecordAutorouteNotification(observability.OutcomeQueued)
			logger.Info("pending autoroute review notification queued",
				observability.F(observability.FieldComponent, "autoroute-notify"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeQueued)),
				observability.F(observability.FieldCount, result.Eligible),
				observability.F(observability.FieldAttempt, result.New),
				observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
			return
		}
		// Nothing new above the threshold: another replica already claimed the
		// backlog, or nothing changed since the last scan.
		metrics.RecordAutorouteNotification(observability.OutcomeSkipped)
	}
}

func autorouteDeliveryObserver(metrics *observability.Metrics, logger *slog.Logger) func(autoroute.DeliveryResult, error) {
	if logger == nil {
		logger = observability.NewDiscardLogger()
	}
	return func(result autoroute.DeliveryResult, err error) {
		if err != nil {
			metrics.RecordAutorouteNotification(observability.OutcomeFailed)
			logger.Warn("pending autoroute review notification delivery failed",
				observability.F(observability.FieldComponent, "autoroute-notify"),
				observability.F(observability.FieldOutcome, string(observability.OutcomeFailed)),
				observability.F(observability.FieldError, observability.Errf(err)))
			return
		}
		metrics.RecordAutorouteNotification(observability.OutcomeDelivered)
		logger.Info("pending autoroute review notification delivered",
			observability.F(observability.FieldComponent, "autoroute-notify"),
			observability.F(observability.FieldOutcome, string(observability.OutcomeDelivered)),
			observability.F(observability.FieldCount, result.Items),
			observability.F(observability.FieldDurationMs, result.Duration.Milliseconds()))
	}
}

// lifecycleHealthReporter writes this replica's runtime health onto the
// deployed integration's lifecycle snapshot.
//
// integration_lifecycle_snapshots.health was permanently stale before Slice 4.3
// because PostgresCatalog.ReportHealth had zero non-test callers, so a deployed
// release advertised the health it was given at deploy time forever. This
// component is the caller.
//
// It reports only for integrations this replica actually serves — the MLLP
// listener's and the batch runner's definition IDs — because a replica that
// serves neither has nothing truthful to say about them.
type lifecycleHealthReporter struct {
	catalog       *lifecycle.PostgresCatalog
	tenantID      string
	definitionIDs []string
	principal     integration.Principal
	interval      time.Duration
	readiness     observability.Reporter
	logger        *slog.Logger
}

func newLifecycleHealthReporter(
	catalog *lifecycle.PostgresCatalog,
	tenantID string,
	definitionIDs []string,
	principalID string,
	readiness observability.Reporter,
	interval time.Duration,
	logger *slog.Logger,
) *lifecycleHealthReporter {
	if catalog == nil || tenantID == "" || len(definitionIDs) == 0 || readiness == nil {
		return nil
	}
	if interval <= 0 {
		interval = time.Minute
	}
	unique := make([]string, 0, len(definitionIDs))
	seen := make(map[string]struct{}, len(definitionIDs))
	for _, id := range definitionIDs {
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil
	}
	return &lifecycleHealthReporter{
		catalog:       catalog,
		tenantID:      tenantID,
		definitionIDs: unique,
		principal: integration.Principal{
			ID:         principalID,
			Kind:       integration.PrincipalKindService,
			AuthMethod: "runtime",
		},
		interval:  interval,
		readiness: readiness,
		logger:    logger,
	}
}

// Run reports on start and then on every interval tick until cancellation.
//
// A report failure is never fatal: the snapshot may have moved out of the
// deployed state under us, and losing a health update must not take an ingress
// listener down.
func (r *lifecycleHealthReporter) Run(ctx context.Context) error {
	if r == nil || ctx == nil {
		return nil
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		r.reportOnce(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *lifecycleHealthReporter) reportOnce(ctx context.Context) {
	health := integration.DeploymentHealthHealthy
	switch r.readiness.Ready(ctx).Status {
	case observability.StatusUnhealthy:
		health = integration.DeploymentHealthUnhealthy
	case observability.StatusDegraded:
		health = integration.DeploymentHealthDegraded
	}
	for _, definitionID := range r.definitionIDs {
		binding, err := r.catalog.ResolveRunnable(ctx, r.tenantID, definitionID)
		if err != nil {
			continue
		}
		if binding.Health == health {
			// Nothing changed; do not append a lifecycle event per tick.
			continue
		}
		if _, err := r.catalog.ReportHealth(ctx, lifecycle.Command{
			TenantID:        r.tenantID,
			DefinitionID:    definitionID,
			RevisionID:      binding.IntegrationRevision.RevisionID,
			ExpectedVersion: binding.SnapshotVersion,
			Principal:       r.principal,
			Reason:          "runtime readiness report",
		}, health); err != nil {
			logger := r.logger
			if logger == nil {
				logger = observability.NewDiscardLogger()
			}
			logger.WarnContext(ctx, "lifecycle health report failed",
				observability.F(observability.FieldComponent, "lifecycle-health"),
				observability.F(observability.FieldDefinitionID, definitionID),
				observability.F(observability.FieldStatus, string(health)),
				observability.F(observability.FieldError, observability.Errf(err)))
		}
	}
}

// serveDurableDefinitionIDs lists the lifecycle definitions this replica
// actually serves.
//
// A replica reports runtime health only for integrations it runs an ingress
// for; reporting on an integration this process never touches would be a claim
// it cannot back.
func serveDurableDefinitionIDs(runtime *previewRuntime) []string {
	if runtime == nil {
		return nil
	}
	ids := make([]string, 0, 2)
	if runtime.mllpServer != nil {
		ids = append(ids, os.Getenv("FI_FHIR_MLLP_DEFINITION_ID"))
	}
	if runtime.batchRunner != nil {
		ids = append(ids, os.Getenv("FI_FHIR_BATCH_DEFINITION_ID"))
	}
	return ids
}
