package observability

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseModeDefaultsToCurrent(t *testing.T) {
	for _, value := range []string{"", "current", "Legacy ", "nonsense", "LEGACY"} {
		mode := ParseMode(value)
		wantLegacy := strings.EqualFold(strings.TrimSpace(value), "legacy")
		if mode.Legacy() != wantLegacy {
			t.Fatalf("ParseMode(%q).Legacy() = %v, want %v", value, mode.Legacy(), wantLegacy)
		}
	}
}

func TestReadinessReportsNotConfiguredRatherThanHealthy(t *testing.T) {
	health := NewHealth("test", time.Second)
	health.RegisterDatabase("submission_db", nil, "durable submission database is not configured")

	report := health.Ready(context.Background())
	if report.Status != StatusHealthy {
		t.Fatalf("overall status = %q, want %q", report.Status, StatusHealthy)
	}
	if len(report.Components) != 1 {
		t.Fatalf("components = %d, want 1", len(report.Components))
	}
	// The point of the slice: an absent dependency must not read as a working
	// one, but must also not fail a deployment that legitimately omits it.
	if report.Components[0].Status != StatusNotConfigured {
		t.Fatalf("component status = %q, want %q", report.Components[0].Status, StatusNotConfigured)
	}
}

func TestReadinessHandlerReturns503OnUnhealthyComponent(t *testing.T) {
	health := NewHealth("test", time.Second)
	health.RegisterReadiness("submission_db", func(context.Context) Component {
		return Component{Status: StatusUnhealthy, Message: "database is unreachable"}
	})

	recorder := httptest.NewRecorder()
	ReadinessHandler(health).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready status = %d, want 503", recorder.Code)
	}

	var report Report
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode readiness body: %v", err)
	}
	if report.Status != StatusUnhealthy {
		t.Fatalf("readiness status = %q, want %q", report.Status, StatusUnhealthy)
	}
}

func TestLivenessStaysHealthyWhenADependencyIsDown(t *testing.T) {
	health := NewHealth("test", time.Second)
	health.RegisterReadiness("submission_db", func(context.Context) Component {
		return Component{Status: StatusUnhealthy}
	})

	recorder := httptest.NewRecorder()
	LivenessHandler(health).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	// A liveness probe that fails on a dependency outage converts an outage into
	// a pod restart loop, which is strictly worse than serving 503 on readiness.
	if recorder.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200 while a dependency is unhealthy", recorder.Code)
	}
}

func TestComponentStateDrivesReadinessAndMetrics(t *testing.T) {
	health := NewHealth("test", time.Second)
	metrics := NewMetrics("test")

	health.SetComponentState(ComponentMLLP, ComponentRunning)
	metrics.SetComponentState(ComponentMLLP, ComponentRunning)
	if got := componentStatus(t, health, ComponentMLLP); got != StatusHealthy {
		t.Fatalf("running component status = %q, want %q", got, StatusHealthy)
	}

	health.SetComponentState(ComponentMLLP, ComponentStopped)
	metrics.SetComponentState(ComponentMLLP, ComponentStopped)
	// A configured listener that stopped is a readiness failure: the replica is
	// advertising an ingress it no longer serves.
	if got := componentStatus(t, health, ComponentMLLP); got != StatusUnhealthy {
		t.Fatalf("stopped component status = %q, want %q", got, StatusUnhealthy)
	}

	exposition := gather(t, metrics)
	if !strings.Contains(exposition, `fi_fhir_component_up{component="mllp"} 0`) {
		t.Fatalf("stopped component gauge missing from exposition:\n%s", exposition)
	}
}

func TestUnknownOutcomeIsRefusedRatherThanEmitted(t *testing.T) {
	metrics := NewMetrics("test")
	metrics.RecordIngressSubmission(Outcome("mrn-123456"))

	values, err := GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, value := range values {
		if value == "mrn-123456" {
			t.Fatal("an unbounded label value reached the exposition")
		}
	}
	if !strings.Contains(gather(t, metrics), `fi_fhir_http_ingress_submissions_total{outcome="error"} 1`) {
		t.Fatal("refused outcome was not charged to the bounded error label")
	}
}

func TestEveryLabelValueIsDrawnFromABoundedSet(t *testing.T) {
	metrics := NewMetrics("1.2.3")
	metrics.RecordIngressSubmission(OutcomeAccepted)
	metrics.RecordMLLPMessage(OutcomeRejected)
	metrics.RecordDeliveryAttempt(OutcomeRetried)
	metrics.RecordBatchObject(OutcomeProcessed)
	metrics.RecordSessionStreamEvent(OutcomeReplayed)
	metrics.RecordAutorouteSweep(OutcomeProcessed, 3)
	metrics.RecordAutorouteNotification(OutcomeQueued)
	metrics.RecordRetentionPurge(OutcomeProcessed, 7)
	metrics.SetComponentState(ComponentDelivery, ComponentRunning)
	metrics.ObserveReadiness(Report{Components: []Component{{Name: ComponentSubmissionDB, Status: StatusHealthy}}})
	metrics.SetSchemaLedgerVersion(SchemaLedgerSession, 6)
	metrics.SetSchemaLedgerVersion(SchemaLedgerTerminology, 3)

	allowed := map[string]struct{}{"1.2.3": {}}
	for outcome := range allOutcomes {
		allowed[outcome] = struct{}{}
	}
	for ledger := range allSchemaLedgers {
		allowed[ledger] = struct{}{}
	}
	for _, component := range []string{
		ComponentGraphQL, ComponentMetrics, ComponentMLLP, ComponentDelivery, ComponentBatch,
		ComponentAutorouteSweep, ComponentAutorouteNotify, ComponentSessionStream,
		ComponentSubmissionDB, ComponentTerminologyDB, ComponentSessionStore, ComponentProfileStore,
		ComponentWorkflowStore, ComponentEventStore, ComponentMappingStore, ComponentProcessLiveness,
		ComponentLifecycleCatalog, ComponentRetentionPurge,
	} {
		allowed[component] = struct{}{}
	}

	values, err := GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("no label values were gathered; the assertion would be vacuous")
	}
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			t.Fatalf("label value %q is outside the declared bounded set", value)
		}
	}
}

func TestIngressMiddlewareClassifiesByResponseStatus(t *testing.T) {
	metrics := NewMetrics("test")
	for status, want := range map[int]string{
		http.StatusAccepted:            "accepted",
		http.StatusUnauthorized:        "rejected",
		http.StatusInternalServerError: "error",
	} {
		handler := metrics.IngressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/hl7v2", nil))
		if !strings.Contains(gather(t, metrics), `outcome="`+want+`"} 1`) {
			t.Fatalf("status %d was not counted as %q", status, want)
		}
	}
}

func TestMetricsServerServesOnlyItsConfiguredPath(t *testing.T) {
	metrics := NewMetrics("test")
	server, err := NewMetricsServer(MetricsServerConfig{Host: "127.0.0.1", Port: 0, Handler: metrics.Handler()})
	if err == nil {
		_ = server.Close()
		t.Fatal("port 0 must be refused: an unpredictable metrics port cannot be scraped")
	}

	server, err = NewMetricsServer(MetricsServerConfig{
		Host: "127.0.0.1", Port: freePort(t), Path: "/metrics", Handler: metrics.Handler(),
	})
	if err != nil {
		t.Fatalf("bind metrics server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	base := "http://" + server.Addr()
	if body := getBody(t, base+"/metrics"); !strings.Contains(body, "fi_fhir_build_info") {
		t.Fatalf("metrics path did not serve the exposition:\n%s", body)
	}
	resp, err := http.Get(base + "/graphql")
	if err != nil {
		t.Fatalf("probe non-metrics path: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-metrics path status = %d, want 404; the scrape listener must expose nothing else", resp.StatusCode)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("metrics server shutdown reported an error: %v", err)
	}
}

func componentStatus(t *testing.T, health *Health, name string) Status {
	t.Helper()
	for _, component := range health.Ready(context.Background()).Components {
		if component.Name == name {
			return component.Status
		}
	}
	t.Fatalf("component %q is absent from the readiness report", name)
	return ""
}

func gather(t *testing.T, metrics *Metrics) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics handler status = %d", recorder.Code)
	}
	return recorder.Body.String()
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	return string(body)
}

// freePort reserves and releases a port so the metrics server can bind a
// predictable address in tests without racing another suite.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// TestUnknownSchemaLedgerIsRefusedRatherThanEmitted mirrors the outcome-label
// proof for the `ledger` label slice 4.4a added. The cardinality contract is
// the same: a label value outside the declared set is a programming error, and
// the registry must drop it rather than publish it.
func TestUnknownSchemaLedgerIsRefusedRatherThanEmitted(t *testing.T) {
	metrics := NewMetrics("test")
	metrics.SetSchemaLedgerVersion("tenant-acme-shard-7", 42)
	metrics.SetSchemaLedgerVersion(SchemaLedgerBatch, 3)

	exposition := gather(t, metrics)
	if strings.Contains(exposition, "tenant-acme-shard-7") {
		t.Fatalf("an unbounded ledger label reached the exposition:\n%s", exposition)
	}
	if !strings.Contains(exposition, `fi_fhir_schema_ledger_version{ledger="batch"} 3`) {
		t.Fatalf("the declared ledger version is missing from the exposition:\n%s", exposition)
	}
}
