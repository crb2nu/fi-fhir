package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// testCorrelationKey is a fixed 32-byte key for the keyed correlation digest.
var testCorrelationKey = []byte("0123456789abcdef0123456789abcdef")

func TestServeTelemetryRecordsProcessorAndDeniedDeliveryWithoutPHI(t *testing.T) {
	runServeTelemetryProof(t, true, false)
}

func TestServeTracingDefaultOffExportsNothingAndKeepsLineage(t *testing.T) {
	for _, endpoint := range []bool{false, true} {
		name := "without_endpoint"
		if endpoint {
			name = "with_endpoint"
		}
		t.Run(name, func(t *testing.T) { runServeTelemetryProof(t, false, endpoint) })
	}
}

func runServeTelemetryProof(t *testing.T, enabled, endpoint bool) {
	t.Helper()
	var connections, requests atomic.Int64
	var mu sync.Mutex
	var spans []*tracepb.Span
	collectorServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request collector.ExportTraceServiceRequest
		if err := proto.Unmarshal(raw, &request); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				spans = append(spans, scope.Spans...)
			}
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	collectorServer.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	collectorServer.Start()
	defer collectorServer.Close()
	var logs bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{TenantID: "tenant-a", Level: "debug", Output: &logs})
	// Clear inherited tracing settings, including future additions to the surface.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "FI_FHIR_TRACING_") {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, key := range []string{"FI_FHIR_TRACING_ENABLED", "FI_FHIR_TRACING_ENDPOINT", "FI_FHIR_TRACING_SAMPLER"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	if enabled {
		t.Setenv("FI_FHIR_TRACING_ENABLED", "true")
		t.Setenv("FI_FHIR_TRACING_SAMPLER", "1")
	}
	if enabled || endpoint {
		t.Setenv("FI_FHIR_TRACING_ENDPOINT", collectorServer.URL)
	}
	runtimeConfig := config.Default()
	runtimeConfig.ApplyEnv()
	if err := runtimeConfig.EnvAliasError(); err != nil {
		t.Fatal(err)
	}
	if runtimeConfig.Observability.TracingEnabled != enabled {
		t.Fatal("unexpected tracing enablement")
	}
	tracing, err := newServeTracing(context.Background(), runtimeConfig, "tenant-a", logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tracing.Shutdown(context.Background()) })
	clearGraphQLAuthenticationEnv(t)
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", writeRuntimeRegistry(t, "tenant-a"))
	runtime, err := loadPreviewRuntimeFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if len(runtime.processors) != 1 {
		t.Fatalf("processor wiring: %d", len(runtime.processors))
	}
	for _, p := range runtime.processors {
		p.SetObserver(processorObservation(logger, tracing, testCorrelationKey))
	}
	security, err := runtime.authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile("../../testdata/golden/integration/adt-http/input.hl7")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, []byte("\rPV1|1|I|WARD^101^1")...)
	payload = bytes.ReplaceAll(payload, []byte("Patient^Golden"), []byte("PHI_PATIENT_SENTINEL^Golden"))
	payload = bytes.ReplaceAll(payload, []byte("golden-control-001"), []byte("PHI_SOURCE_ID_SENTINEL"))
	result, err := runtime.previewService.Preview(context.Background(), security, preview.Input{IntegrationID: "adt-east", Payload: payload, CorrelationID: "PHI_HEADER_SENTINEL", Reason: "test preview"})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(result.Events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(canonical, []byte("PHI_PATIENT_SENTINEL")) {
		t.Fatal("sentinel was not present in canonical data")
	}
	item := delivery.WorkItem{TenantID: "tenant-a", AttemptID: "attempt-1", ReceiptID: "receipt-1", TraceID: "ledger-trace-1", EventPayload: canonical}
	store := &telemetryDeliveryStore{item: &item}
	dispatcher, err := delivery.NewDispatcherWithIdentity(store, telemetryPublisher{t}, "worker-test", delivery.DefaultConfig(), telemetryDenier{})
	if err != nil {
		t.Fatal(err)
	}
	metrics := observability.NewMetrics("test")
	bindDeliveryObservation(dispatcher, metrics, logger, tracing)
	if outcome, err := dispatcher.RunOnce(context.Background()); err != nil || outcome != delivery.OutcomeForbidden {
		t.Fatalf("denial outcome=%s err=%v", outcome, err)
	}
	if outcome, err := dispatcher.RunOnce(context.Background()); err != nil || outcome != delivery.OutcomeIdle {
		t.Fatalf("idle outcome=%s err=%v", outcome, err)
	}
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		time.Sleep(100 * time.Millisecond)
		if tracing != nil {
			t.Error("disabled serve constructed tracing provider")
		}
		if connections.Load() != 0 || requests.Load() != 0 {
			t.Errorf("disabled tracing contacted collector: connections=%d requests=%d", connections.Load(), requests.Load())
		}
	}
	expectedSpans := 0
	if enabled {
		expectedSpans = 3
	}
	if len(spans) != expectedSpans {
		t.Fatalf("got %d spans; expected admission, plan, denied dispatch, no idle", len(spans))
	}
	evidence := logs.String()
	for _, span := range spans {
		evidence += span.String()
		for _, attr := range span.Attributes {
			if attr.Key == "dropped_attributes" {
				t.Errorf("a serve call site passed %d attribute(s) outside the span allowlist on %s", attr.Value.GetIntValue(), span.Name)
				continue
			}
			if !observability.KnownSpanAttribute(observability.LogField(attr.Key)) {
				t.Errorf("unknown attribute %q", attr.Key)
			}
		}
	}
	for _, sentinel := range []string{"PHI_PATIENT_SENTINEL", "PHI_SOURCE_ID_SENTINEL", "PHI_HEADER_SENTINEL", "RAW-GOLDEN-PHI-SENTINEL", "PHI_DENIAL_SENTINEL"} {
		if strings.Contains(evidence, sentinel) {
			t.Errorf("PHI reached telemetry: %s", sentinel)
		}
	}
	// The caller header is joinable without being readable: its keyed digest is
	// on the admission telemetry even though its text is not.
	if !strings.Contains(evidence, spanCorrelationID(testCorrelationKey, "PHI_HEADER_SENTINEL")) {
		t.Error("admission telemetry does not carry the hashed caller correlation header")
	}
	if !strings.Contains(logs.String(), `"outcome":"rejected"`) || !strings.Contains(logs.String(), `"receipt_id":"receipt-1"`) || !strings.Contains(logs.String(), `"lineage_trace_id":"ledger-trace-1"`) {
		t.Fatalf("missing denied-delivery lineage: %s", &logs)
	}
	for _, key := range []string{"receipt_id", "lineage_trace_id", "delivery_attempt_id"} {
		if !strings.Contains(logs.String(), "\""+key+"\":") {
			t.Errorf("missing lineage key %s", key)
		}
	}
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	rejected := false
	for _, family := range families {
		if family.GetName() == "fi_fhir_delivery_attempts_total" {
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "outcome" && label.GetValue() == "rejected" && metric.GetCounter().GetValue() == 1 {
						rejected = true
					}
				}
			}
		}
	}
	if !rejected {
		t.Fatal("identity denial was not counted as rejected")
	}
	processorLines, deliveryLines := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		for key := range record {
			if key != "time" && key != "level" && key != "msg" && !observability.KnownLogField(observability.LogField(key)) {
				t.Errorf("unknown log key %q", key)
			}
		}
		if record["component"] == "message-processor" {
			processorLines++
			if record["correlation_id"] != spanCorrelationID(testCorrelationKey, "PHI_HEADER_SENTINEL") {
				t.Errorf("processor lost hashed correlation: %s", line)
			}
		}
		if record["component"] == "delivery-worker" {
			deliveryLines++
			if record["receipt_id"] != "receipt-1" || record["lineage_trace_id"] != "ledger-trace-1" || record["delivery_attempt_id"] != "attempt-1" {
				t.Errorf("delivery lost lineage: %s", line)
			}
		}
		if !enabled {
			for _, key := range []string{"trace_id", "span_id"} {
				if _, ok := record[key]; ok {
					t.Errorf("disabled tracing emitted %s: %s", key, line)
				}
			}
		}
		if record["tenant_id"] != "tenant-a" || (enabled && (record["trace_id"] == nil || record["span_id"] == nil)) {
			t.Errorf("missing correlation: %s", line)
		}
	}
	if processorLines == 0 || deliveryLines == 0 {
		t.Errorf("per-component lineage checks were vacuous: processor=%d delivery=%d", processorLines, deliveryLines)
	}
}

type telemetryDeliveryStore struct{ item *delivery.WorkItem }

func (s *telemetryDeliveryStore) Claim(context.Context, string, time.Duration) (*delivery.WorkItem, error) {
	item := s.item
	s.item = nil
	return item, nil
}
func (s *telemetryDeliveryStore) MarkPublished(context.Context, delivery.WorkItem) error { return nil }
func (s *telemetryDeliveryStore) MarkFailed(context.Context, delivery.WorkItem, delivery.Failure, delivery.Config) (bool, error) {
	return false, nil
}

type telemetryPublisher struct{ t *testing.T }

func (p telemetryPublisher) Publish(context.Context, delivery.Message) error {
	p.t.Error("denied delivery reached publisher")
	return nil
}
func (telemetryPublisher) Close() error { return nil }

type telemetryDenier struct{}

func (telemetryDenier) Decide(context.Context, string, string, integration.DestinationRevisionRef) error {
	return delivery.NewDenial(delivery.ForbiddenFailureCode, "PHI_DENIAL_SENTINEL")
}
