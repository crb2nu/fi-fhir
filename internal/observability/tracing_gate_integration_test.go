//go:build integration

package observability_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func serveTraceCollector(t *testing.T) (string, func() []*tracepb.Span) {
	t.Helper()
	var mu sync.Mutex
	var spans []*tracepb.Span
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
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
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				spans = append(spans, scope.Spans...)
			}
		}
		mu.Unlock()
		select {
		case received <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	t.Cleanup(server.Close)
	return server.URL, func() []*tracepb.Span {
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		for {
			mu.Lock()
			captured := append([]*tracepb.Span(nil), spans...)
			mu.Unlock()
			admission, plan := false, false
			for _, span := range captured {
				admission = admission || span.Name == "integration.admission"
				plan = plan || span.Name == "integration.workflow.plan"
			}
			if admission && plan {
				return captured
			}
			select {
			case <-received:
			case <-deadline.C:
				t.Fatalf("expected admission and planning spans, received %v", captured)
			}
		}
	}
}

func spanStringField(span *tracepb.Span, key string) string {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func assertDurableAdmissionSpans(t *testing.T, spans []*tracepb.Span) {
	t.Helper()
	var admission, plan *tracepb.Span
	for _, span := range spans {
		for _, sentinel := range phiSentinels {
			if strings.Contains(span.String(), sentinel) {
				t.Errorf("PHI %q reached exported span", sentinel)
			}
		}
		for _, attr := range span.Attributes {
			if !observability.KnownLogField(observability.LogField(attr.Key)) {
				t.Errorf("unlisted span attribute %s", attr.Key)
			}
		}
		if spanStringField(span, "tenant_id") != documentedTenant {
			t.Error("span lacks deployment tenant")
		}
		if span.Name == "integration.admission" {
			admission = span
		}
		if span.Name == "integration.workflow.plan" {
			plan = span
		}
	}
	if admission == nil || plan == nil {
		t.Fatal("missing admission/planning spans")
	}
	if spanStringField(admission, "receipt_id") == "" || spanStringField(admission, "lineage_trace_id") == "" || spanStringField(admission, "outcome") != "accepted" {
		t.Fatalf("admission was not durably accepted: %s", admission)
	}
	if !bytes.Equal(admission.TraceId, plan.TraceId) || !bytes.Equal(admission.SpanId, plan.ParentSpanId) {
		t.Error("planning span is not a child of admission")
	}
}
