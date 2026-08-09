package main

import (
	"context"
	"fmt"
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

// bindMLLPObservation reports every admitted or rejected frame.
func bindMLLPObservation(service *mllp.Service, metrics *observability.Metrics) {
	if service == nil || metrics == nil {
		return
	}
	service.SetObserver(func(result mllp.SubmitResult, err error) {
		switch {
		case err != nil:
			metrics.RecordMLLPMessage(observability.OutcomeRejected)
		case result.Accepted:
			metrics.RecordMLLPMessage(observability.OutcomeAccepted)
		default:
			metrics.RecordMLLPMessage(observability.OutcomeError)
		}
	})
}

// bindDeliveryObservation reports the typed Outcome the dispatcher already
// computes and Run previously discarded.
func bindDeliveryObservation(dispatcher *integrationdelivery.Dispatcher, metrics *observability.Metrics) {
	if dispatcher == nil || metrics == nil {
		return
	}
	dispatcher.SetObserver(func(result integrationdelivery.DispatchResult, err error) {
		if err != nil {
			metrics.RecordDeliveryAttempt(observability.OutcomeError)
			return
		}
		switch result.Outcome {
		case integrationdelivery.OutcomePublished:
			metrics.RecordDeliveryAttempt(observability.OutcomeDelivered)
		case integrationdelivery.OutcomeRetry:
			metrics.RecordDeliveryAttempt(observability.OutcomeRetried)
		case integrationdelivery.OutcomeDLQ:
			metrics.RecordDeliveryAttempt(observability.OutcomeFailed)
		default:
			metrics.RecordDeliveryAttempt(observability.OutcomeIdle)
		}
	})
}

// bindBatchObservation reports each batch poll cycle.
func bindBatchObservation(runner *integrationbatch.Runner, metrics *observability.Metrics) {
	if runner == nil || metrics == nil {
		return
	}
	runner.SetObserver(func(result integrationbatch.PollResult, err error) {
		switch {
		case err != nil:
			metrics.RecordBatchObject(observability.OutcomeFailed)
		case result.Objects > 0:
			for i := 0; i < result.Objects; i++ {
				metrics.RecordBatchObject(observability.OutcomeProcessed)
			}
		default:
			metrics.RecordBatchObject(observability.OutcomeIdle)
		}
	})
}

// sessionStreamObserver reports durable session fanout transitions.
func sessionStreamObserver(metrics *observability.Metrics) func(integrationsession.StreamOutcome, error) {
	if metrics == nil {
		return nil
	}
	return func(outcome integrationsession.StreamOutcome, err error) {
		if err != nil {
			metrics.RecordSessionStreamEvent(observability.OutcomeError)
			return
		}
		switch outcome {
		case integrationsession.StreamOutcomePublished:
			metrics.RecordSessionStreamEvent(observability.OutcomePublished)
		case integrationsession.StreamOutcomeReplayed:
			metrics.RecordSessionStreamEvent(observability.OutcomeReplayed)
		case integrationsession.StreamOutcomeDropped:
			metrics.RecordSessionStreamEvent(observability.OutcomeDropped)
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
		Observe: sessionStreamObserver(metrics),
	})
}

// autorouteSweepObserver keeps the existing stderr/stdout reporting and adds
// metrics, rather than replacing operator-visible output with a counter.
func autorouteSweepObserver(metrics *observability.Metrics) func(autoroute.SweepResult, error) {
	return func(result autoroute.SweepResult, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: pending autoroute sweep failed: %v\n", err)
			metrics.RecordAutorouteSweep(observability.OutcomeError, 0)
			return
		}
		metrics.RecordAutorouteSweep(observability.OutcomeProcessed, int(result.Expired))
		if result.Expired > 0 {
			fmt.Printf("Pending autoroute sweep: expired=%d duration=%s\n",
				result.Expired, result.Duration.Round(time.Millisecond))
		}
	}
}

func autorouteNotifyObserver(metrics *observability.Metrics) func(autoroute.NotifyResult, error) {
	return func(result autoroute.NotifyResult, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: pending autoroute review scan failed: %v\n", err)
			metrics.RecordAutorouteNotification(observability.OutcomeError)
			return
		}
		if result.Dropped > 0 {
			metrics.RecordAutorouteNotification(observability.OutcomeDropped)
			fmt.Fprintf(os.Stderr,
				"Warning: pending autoroute review notification dropped (dispatch queue full): eligible=%d new=%d\n",
				result.Eligible, result.New)
			return
		}
		if result.Queued > 0 {
			metrics.RecordAutorouteNotification(observability.OutcomeQueued)
			fmt.Printf("Pending autoroute review: eligible=%d new=%d queued duration=%s\n",
				result.Eligible, result.New, result.Duration.Round(time.Millisecond))
			return
		}
		// Nothing new above the threshold: another replica already claimed the
		// backlog, or nothing changed since the last scan.
		metrics.RecordAutorouteNotification(observability.OutcomeSkipped)
	}
}

func autorouteDeliveryObserver(metrics *observability.Metrics) func(autoroute.DeliveryResult, error) {
	return func(result autoroute.DeliveryResult, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: pending autoroute review notification delivery failed: %v\n", err)
			metrics.RecordAutorouteNotification(observability.OutcomeFailed)
			return
		}
		metrics.RecordAutorouteNotification(observability.OutcomeDelivered)
		fmt.Printf("Pending autoroute review notification delivered: items=%d duration=%s\n",
			result.Items, result.Duration.Round(time.Millisecond))
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
}

func newLifecycleHealthReporter(
	catalog *lifecycle.PostgresCatalog,
	tenantID string,
	definitionIDs []string,
	principalID string,
	readiness observability.Reporter,
	interval time.Duration,
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
			fmt.Fprintf(os.Stderr, "Warning: lifecycle health report failed for %s: %v\n", definitionID, err)
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
