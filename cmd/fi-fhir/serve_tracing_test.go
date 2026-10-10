package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestServeTelemetryRecordsProcessorAndDeniedDeliveryWithoutPHI(t *testing.T) {
	var mu sync.Mutex
	var spans []*tracepb.Span
	collectorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer collectorServer.Close()
	var logs bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{TenantID: "tenant-a", Level: "debug", Output: &logs})
	tracing, err := observability.NewTracing(context.Background(), observability.TraceConfig{Enabled: true, Endpoint: collectorServer.URL, SampleRatio: 1, TenantID: "tenant-a", Logger: logger})
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
		p.SetObserver(processorObservation(logger, tracing))
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
	if len(spans) != 3 {
		t.Fatalf("got %d spans; expected admission, plan, denied dispatch, no idle", len(spans))
	}
	evidence := logs.String()
	for _, span := range spans {
		evidence += span.String()
		for _, attr := range span.Attributes {
			if !observability.KnownLogField(observability.LogField(attr.Key)) {
				t.Errorf("unknown attribute %q", attr.Key)
			}
		}
	}
	for _, sentinel := range []string{"PHI_PATIENT_SENTINEL", "PHI_SOURCE_ID_SENTINEL", "PHI_HEADER_SENTINEL", "RAW-GOLDEN-PHI-SENTINEL", "PHI_DENIAL_SENTINEL"} {
		if strings.Contains(evidence, sentinel) {
			t.Errorf("PHI reached telemetry: %s", sentinel)
		}
	}
	if !strings.Contains(logs.String(), `"outcome":"rejected"`) || !strings.Contains(logs.String(), `"receipt_id":"receipt-1"`) || !strings.Contains(logs.String(), `"lineage_trace_id":"ledger-trace-1"`) {
		t.Fatalf("missing denied-delivery lineage: %s", &logs)
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
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["tenant_id"] != "tenant-a" || record["trace_id"] == nil || record["span_id"] == nil {
			t.Errorf("missing correlation: %s", line)
		}
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
