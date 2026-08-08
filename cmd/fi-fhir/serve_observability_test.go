package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	integrationsession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
)

func TestResolveBatchWorkerIDDerivesAUniqueIdentity(t *testing.T) {
	t.Setenv("FI_FHIR_BATCH_WORKER_ID", "")

	workerID, err := resolveBatchWorkerID(observability.ModeCurrent)
	if err != nil {
		t.Fatalf("resolve worker ID: %v", err)
	}
	hostname, hostErr := os.Hostname()
	if hostErr != nil {
		t.Skipf("hostname unavailable: %v", hostErr)
	}
	// The derivation must include the PID: two replicas on one host that differ
	// only by hostname would still share a batch lease owner and steal each
	// other's live leases.
	if !strings.HasPrefix(workerID, hostname+"-") {
		t.Fatalf("worker ID %q is not derived from hostname %q", workerID, hostname)
	}
	if workerID == hostname {
		t.Fatal("worker ID carries no per-process component")
	}
}

func TestResolveBatchWorkerIDPrefersAnExplicitOverride(t *testing.T) {
	t.Setenv("FI_FHIR_BATCH_WORKER_ID", "  operator-minted-worker-7  ")

	workerID, err := resolveBatchWorkerID(observability.ModeCurrent)
	if err != nil {
		t.Fatalf("resolve worker ID: %v", err)
	}
	if workerID != "operator-minted-worker-7" {
		t.Fatalf("worker ID = %q, want the trimmed override", workerID)
	}
}

func TestResolveBatchWorkerIDLegacyModeRequiresTheVariable(t *testing.T) {
	t.Setenv("FI_FHIR_BATCH_WORKER_ID", "")

	// The negative control keeps the pre-Slice-4.3 contract so the kill-test can
	// demonstrate the shared-identity defect the documentation prescribed.
	if _, err := resolveBatchWorkerID(observability.ModeLegacy); err == nil {
		t.Fatal("legacy mode derived a worker ID; the negative control needs the old required-variable contract")
	}
}

func TestSessionStreamObserverMapsEveryOutcome(t *testing.T) {
	metrics := observability.NewMetrics("test")
	observe := sessionStreamObserver(metrics)
	if observe == nil {
		t.Fatal("observer is nil for a configured registry")
	}

	observe(integrationsession.StreamOutcomePublished, nil)
	observe(integrationsession.StreamOutcomeReplayed, nil)
	observe(integrationsession.StreamOutcomeDropped, nil)
	observe(integrationsession.StreamOutcome("unknown"), nil)
	observe(integrationsession.StreamOutcomePublished, context.Canceled)

	values, err := observability.GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, value := range values {
		if !observability.KnownOutcome(value) && !strings.Contains(value, "test") &&
			value != observability.ComponentSessionStream {
			// Component and version labels are allowed; anything else must be a
			// declared outcome.
			if !isDeclaredComponent(value) {
				t.Fatalf("label value %q escaped the bounded set", value)
			}
		}
	}
}

func TestSessionStreamObserverIsNilWithoutARegistry(t *testing.T) {
	if sessionStreamObserver(nil) != nil {
		t.Fatal("a nil registry must not produce an observer")
	}
}

func TestBindObservationsToleratesAbsentComponents(t *testing.T) {
	metrics := observability.NewMetrics("test")
	// Every binder must be safe on a deployment that configures none of these
	// components, which is the default.
	bindMLLPObservation(nil, metrics)
	bindDeliveryObservation(nil, metrics)
	bindBatchObservation(nil, metrics)
	bindMLLPObservation(nil, nil)

	var (
		dispatcher *integrationdelivery.Dispatcher
		runner     *integrationbatch.Runner
	)
	bindDeliveryObservation(dispatcher, metrics)
	bindBatchObservation(runner, metrics)
}

func TestAutorouteObserversRecordBoundedOutcomes(t *testing.T) {
	metrics := observability.NewMetrics("test")

	autorouteSweepObserver(metrics)(autoroute.SweepResult{Expired: 4, Duration: time.Millisecond}, nil)
	autorouteSweepObserver(metrics)(autoroute.SweepResult{}, context.Canceled)
	autorouteNotifyObserver(metrics)(autoroute.NotifyResult{Queued: 1, Eligible: 2, New: 2}, nil)
	autorouteNotifyObserver(metrics)(autoroute.NotifyResult{Dropped: 1}, nil)
	autorouteNotifyObserver(metrics)(autoroute.NotifyResult{}, nil)
	autorouteNotifyObserver(metrics)(autoroute.NotifyResult{}, context.Canceled)
	autorouteDeliveryObserver(metrics)(autoroute.DeliveryResult{Items: 3}, nil)
	autorouteDeliveryObserver(metrics)(autoroute.DeliveryResult{}, context.Canceled)

	values, err := observability.GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("no labels gathered; the assertion would be vacuous")
	}
	for _, value := range values {
		if !observability.KnownOutcome(value) && value != "test" && !isDeclaredComponent(value) {
			t.Fatalf("label value %q escaped the bounded set", value)
		}
	}
}

func TestNewSessionStreamRelayIsAbsentWithoutADurableLog(t *testing.T) {
	metrics := observability.NewMetrics("test")

	// In-memory workspace: correct in one process, so no relay is built.
	memory := integrationsession.NewMemoryStore()
	relay, err := newSessionStreamRelay(memory, integrationsession.NewHub(), metrics, observability.ModeCurrent)
	if err != nil {
		t.Fatalf("build relay: %v", err)
	}
	if relay != nil {
		t.Fatal("a store without a durable log must not get a relay")
	}

	// Legacy mode restores process-local fanout even when a log exists.
	hub := integrationsession.NewDurableHub(stubStreamLog{}, nil)
	relay, err = newSessionStreamRelay(memory, hub, metrics, observability.ModeLegacy)
	if err != nil {
		t.Fatalf("build relay in legacy mode: %v", err)
	}
	if relay != nil {
		t.Fatal("legacy mode must not build a durable relay")
	}
}

func TestServeDurableDefinitionIDsReportsOnlyServedIngress(t *testing.T) {
	if ids := serveDurableDefinitionIDs(nil); ids != nil {
		t.Fatalf("nil runtime returned %v, want nil", ids)
	}
	// A replica running neither ingress has nothing truthful to report about
	// either definition.
	if ids := serveDurableDefinitionIDs(&previewRuntime{}); len(ids) != 0 {
		t.Fatalf("runtime with no ingress returned %v, want none", ids)
	}
}

func TestNewLifecycleHealthReporterRefusesIncompleteWiring(t *testing.T) {
	health := observability.NewHealth("test", time.Second)

	if reporter := newLifecycleHealthReporter(nil, "tenant-a", []string{"def"}, "p", health, time.Minute); reporter != nil {
		t.Fatal("a nil catalog must not produce a reporter")
	}
	if reporter := newLifecycleHealthReporter(nil, "tenant-a", nil, "p", health, time.Minute); reporter != nil {
		t.Fatal("no definitions must not produce a reporter")
	}
	if reporter := newLifecycleHealthReporter(nil, "tenant-a", []string{"def"}, "p", nil, time.Minute); reporter != nil {
		t.Fatal("a nil readiness source must not produce a reporter")
	}
}

// isDeclaredComponent reports whether a label value is one of the component
// names the observability package declares.
func isDeclaredComponent(value string) bool {
	for _, name := range []string{
		observability.ComponentGraphQL, observability.ComponentMetrics, observability.ComponentMLLP,
		observability.ComponentDelivery, observability.ComponentBatch, observability.ComponentAutorouteSweep,
		observability.ComponentAutorouteNotify, observability.ComponentSessionStream,
		observability.ComponentSubmissionDB, observability.ComponentTerminologyDB,
		observability.ComponentSessionStore, observability.ComponentProfileStore,
		observability.ComponentWorkflowStore, observability.ComponentEventStore,
		observability.ComponentMappingStore, observability.ComponentProcessLiveness,
		observability.ComponentLifecycleCatalog,
	} {
		if value == name {
			return true
		}
	}
	return false
}

type stubStreamLog struct{}

func (stubStreamLog) AppendStreamEvent(context.Context, integrationsession.StreamEvent) (int64, error) {
	return 1, nil
}

func (stubStreamLog) ListStreamEventsAfter(context.Context, int64, int) ([]integrationsession.StreamEvent, error) {
	return nil, nil
}

func (stubStreamLog) LatestStreamSeq(context.Context) (int64, error) { return 0, nil }
