package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
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
	observe := sessionStreamObserver(metrics, nil)
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
	if sessionStreamObserver(nil, nil) != nil {
		t.Fatal("a nil registry must not produce an observer")
	}
}

func TestBindObservationsToleratesAbsentComponents(t *testing.T) {
	metrics := observability.NewMetrics("test")
	// Every binder must be safe on a deployment that configures none of these
	// components, which is the default.
	bindMLLPObservation(nil, metrics, nil)
	bindDeliveryObservation(nil, metrics, nil)
	bindBatchObservation(nil, metrics, nil)
	bindMLLPObservation(nil, nil, nil)

	var (
		dispatcher *integrationdelivery.Dispatcher
		runner     *integrationbatch.Runner
	)
	bindDeliveryObservation(dispatcher, metrics, nil)
	bindBatchObservation(runner, metrics, nil)
}

func TestAutorouteObserversRecordBoundedOutcomes(t *testing.T) {
	metrics := observability.NewMetrics("test")

	autorouteSweepObserver(metrics, nil)(autoroute.SweepResult{Expired: 4, Duration: time.Millisecond}, nil)
	autorouteSweepObserver(metrics, nil)(autoroute.SweepResult{}, context.Canceled)
	autorouteNotifyObserver(metrics, nil)(autoroute.NotifyResult{Queued: 1, Eligible: 2, New: 2}, nil)
	autorouteNotifyObserver(metrics, nil)(autoroute.NotifyResult{Dropped: 1}, nil)
	autorouteNotifyObserver(metrics, nil)(autoroute.NotifyResult{}, nil)
	autorouteNotifyObserver(metrics, nil)(autoroute.NotifyResult{}, context.Canceled)
	autorouteDeliveryObserver(metrics, nil)(autoroute.DeliveryResult{Items: 3}, nil)
	autorouteDeliveryObserver(metrics, nil)(autoroute.DeliveryResult{}, context.Canceled)

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
	relay, err := newSessionStreamRelay(memory, integrationsession.NewHub(), metrics, observability.ModeCurrent, nil)
	if err != nil {
		t.Fatalf("build relay: %v", err)
	}
	if relay != nil {
		t.Fatal("a store without a durable log must not get a relay")
	}

	// Legacy mode restores process-local fanout even when a log exists.
	hub := integrationsession.NewDurableHub(stubStreamLog{}, nil)
	relay, err = newSessionStreamRelay(memory, hub, metrics, observability.ModeLegacy, nil)
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

	if reporter := newLifecycleHealthReporter(nil, "tenant-a", []string{"def"}, "p", health, time.Minute, nil); reporter != nil {
		t.Fatal("a nil catalog must not produce a reporter")
	}
	if reporter := newLifecycleHealthReporter(nil, "tenant-a", nil, "p", health, time.Minute, nil); reporter != nil {
		t.Fatal("no definitions must not produce a reporter")
	}
	if reporter := newLifecycleHealthReporter(nil, "tenant-a", []string{"def"}, "p", nil, time.Minute, nil); reporter != nil {
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

type recordingObservationStore struct {
	rows []connection.Observation
	err  error
}

func (s *recordingObservationStore) UpsertObservation(_ context.Context, observation connection.Observation) error {
	s.rows = append(s.rows, observation)
	return s.err
}

func observedReplicaDescription() *connection.RuntimeDescription {
	description := &connection.RuntimeDescription{TenantID: "tenant-a", ReplicaID: "host-1-42"}
	for index, kind := range connection.AdapterOrder {
		description.Adapters[index] = connection.RuntimeAdapter{Kind: kind}
	}
	description.Adapters[1] = connection.RuntimeAdapter{Kind: connection.AdapterMLLP, Enabled: true,
		DefinitionID: "adt-mllp", SourceID: "adt-east", SourceRevisionID: "r1", SourceDigest: "sha256:mllp"}
	description.Adapters[2] = connection.RuntimeAdapter{Kind: connection.AdapterBatch, Enabled: true,
		DefinitionID: "claims-batch", SourceID: "claims", SourceRevisionID: "r3", SourceDigest: "sha256:batch"}
	description.DestinationIdentity = &connection.RuntimeDestinationIdentity{Mode: "strict", Destinations: []connection.RuntimeDestination{
		{ArtifactID: "dest-fhir", RevisionID: "1", Digest: "sha256:fhir"},
		{ArtifactID: "dest-kafka", RevisionID: "2", Digest: "sha256:kafka"},
	}}
	return description
}

func TestRuntimeObservationReporterWritesOneRowPerAdapterAndDestination(t *testing.T) {
	store := &recordingObservationStore{}
	reporter := newRuntimeObservationReporter(store, observedReplicaDescription(), 0, nil)
	if reporter == nil {
		t.Fatal("reporter is nil with a store and a description")
	}
	if reporter.interval != time.Minute {
		t.Fatalf("default interval = %s, want 1m", reporter.interval)
	}
	reporter.reportOnce(t.Context())
	wantAdapters := []string{"http", "mllp", "batch", "delivery", "destination:dest-fhir", "destination:dest-kafka"}
	if len(store.rows) != len(wantAdapters) {
		t.Fatalf("wrote %d rows, want %d: %+v", len(store.rows), len(wantAdapters), store.rows)
	}
	for index, row := range store.rows {
		if row.Adapter != wantAdapters[index] || row.ReplicaID != "host-1-42" || row.TenantID != "tenant-a" {
			t.Errorf("row %d = %+v", index, row)
		}
	}
	if store.rows[1].Digest != "sha256:mllp" || store.rows[5].Digest != "sha256:kafka" {
		t.Fatalf("digests not reported: %+v", store.rows)
	}
}

func TestRuntimeObservationReporterFailureIsLoggedNotFatal(t *testing.T) {
	store := &recordingObservationStore{err: errors.New("connection refused")}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	reporter := newRuntimeObservationReporter(store, observedReplicaDescription(), time.Minute, logger)
	reporter.reportOnce(t.Context())
	if len(store.rows) != 6 {
		t.Fatalf("a failed row stopped the tick after %d rows, want all 6 attempted", len(store.rows))
	}
	if got := strings.Count(logs.String(), "runtime observation heartbeat failed"); got != 1 {
		t.Fatalf("logged %d warnings for one failed tick, want 1:\n%s", got, logs.String())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := reporter.Run(ctx); err != nil {
		t.Fatalf("Run after cancellation: %v", err)
	}
}

func TestNewRuntimeObservationReporterIsNilWithoutAStoreOrReplica(t *testing.T) {
	if reporter := newRuntimeObservationReporter(nil, observedReplicaDescription(), time.Minute, nil); reporter != nil {
		t.Fatal("reporter without a store")
	}
	if reporter := newRuntimeObservationReporter(&recordingObservationStore{}, nil, time.Minute, nil); reporter != nil {
		t.Fatal("reporter without a description")
	}
	anonymous := observedReplicaDescription()
	anonymous.ReplicaID = ""
	if reporter := newRuntimeObservationReporter(&recordingObservationStore{}, anonymous, time.Minute, nil); reporter != nil {
		t.Fatal("reporter without a replica id")
	}
	var reporter *runtimeObservationReporter
	if err := reporter.Run(t.Context()); err != nil {
		t.Fatalf("nil reporter Run: %v", err)
	}
}
