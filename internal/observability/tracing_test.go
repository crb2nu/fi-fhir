package observability

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestTracingExportsCorrelatedAllowlistedSpans(t *testing.T) {
	var mu sync.Mutex
	var spans []*tracepb.Span
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var request collector.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.URL.Path)
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				spans = append(spans, scope.Spans...)
			}
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	globalProvider := otel.GetTracerProvider()
	tracing, err := NewTracing(context.Background(), TraceConfig{Enabled: true, Endpoint: server.URL, SampleRatio: 1, TenantID: "tenant-a", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if otel.GetTracerProvider() != globalProvider {
		t.Fatal("serve tracing replaced the global provider")
	}
	var logs bytes.Buffer
	logger := NewLogger(LogConfig{Output: &logs, Level: "debug", TenantID: "tenant-a"})
	ctx, finish := tracing.Start(context.Background(), "integration.admission", F(FieldReceiptID, "receipt-a"), slog.String("payload", "PHI_SENTINEL"), slog.Any("reason", map[string]string{"payload": "PHI_SENTINEL"}))
	child, finishChild := tracing.Start(ctx, "integration.workflow.plan")
	logger.DebugContext(child, "planned")
	finishChild(false, F(FieldOutcome, "accepted"))
	finish(false, F(FieldOutcome, "accepted"))
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracing.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	var root, plan *tracepb.Span
	for _, span := range spans {
		if span.Name == "integration.admission" {
			root = span
		} else {
			plan = span
		}
		if strings.Contains(span.String(), "PHI_SENTINEL") {
			t.Fatalf("payload escaped: %s", span)
		}
		tenant := false
		for _, attr := range span.Attributes {
			if !KnownLogField(LogField(attr.Key)) {
				t.Errorf("unlisted attribute: %s", attr.Key)
			}
			if attr.Key == "tenant_id" && attr.Value.GetStringValue() == "tenant-a" {
				tenant = true
			}
		}
		if !tenant {
			t.Error("missing tenant attribute")
		}
	}
	if root == nil || plan == nil || !bytes.Equal(root.TraceId, plan.TraceId) || !bytes.Equal(root.SpanId, plan.ParentSpanId) {
		t.Fatalf("span tree not correlated: %v", spans)
	}
	if !strings.Contains(logs.String(), trace.SpanContextFromContext(child).TraceID().String()) {
		t.Fatalf("log missing OTel correlation: %s", &logs)
	}
	for _, path := range paths {
		if path != "/v1/traces" {
			t.Errorf("path = %s", path)
		}
	}
}

func TestTracingDisabledAndZeroSamplingSendNothing(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "zero sampling"}[enabled], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected OTLP request") }))
			defer server.Close()
			tracing, err := NewTracing(context.Background(), TraceConfig{Enabled: enabled, Endpoint: server.URL, SampleRatio: 0})
			if err != nil {
				t.Fatal(err)
			}
			ctx, finish := tracing.Start(context.Background(), "integration.admission")
			if trace.SpanFromContext(ctx).IsRecording() {
				t.Error("unexpected recording span")
			}
			if !enabled && trace.SpanContextFromContext(ctx).IsValid() {
				t.Error("disabled tracing created a context")
			}
			finish(false)
			if err := tracing.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTracingRejectsInvalidConfiguration(t *testing.T) {
	for _, endpoint := range []string{"", "localhost:4318", "ftp://localhost", "http://user:secret@host", "https://host?token=secret", "https://host/#secret"} {
		if _, err := NewTracing(context.Background(), TraceConfig{Enabled: true, Endpoint: endpoint, SampleRatio: 1}); err == nil {
			t.Errorf("accepted endpoint %q", endpoint)
		}
	}
	for _, ratio := range []float64{-1, 2, math.NaN(), math.Inf(1)} {
		if _, err := NewTracing(context.Background(), TraceConfig{Enabled: true, Endpoint: "http://localhost:4318", SampleRatio: ratio}); err == nil {
			t.Errorf("accepted sampler %v", ratio)
		}
	}
}

func TestTracingCollectorErrorsAreRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "PHI_COLLECTOR_SENTINEL", http.StatusBadRequest)
	}))
	defer server.Close()
	var logs bytes.Buffer
	tracer, err := NewTracing(context.Background(), TraceConfig{Enabled: true, Endpoint: server.URL, SampleRatio: 1, Logger: NewLogger(LogConfig{Output: &logs})})
	if err != nil {
		t.Fatal(err)
	}
	_, finish := tracer.Start(context.Background(), "integration.admission")
	finish(false)
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "trace export failed") || strings.Contains(logs.String(), "PHI_COLLECTOR_SENTINEL") {
		t.Fatalf("unexpected exporter error log: %s", &logs)
	}
}
