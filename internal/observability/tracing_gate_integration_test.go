//go:build integration

package observability_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
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

func serveTraceCollector(t *testing.T, requiredNames ...string) (string, func() []*tracepb.Span) {
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
			seen := make(map[string]bool)
			for _, span := range captured {
				seen[span.Name] = true
			}
			complete := true
			for _, name := range requiredNames {
				complete = complete && seen[name]
			}
			if complete {
				return captured
			}
			select {
			case <-received:
			case <-deadline.C:
				t.Fatalf("expected spans %v, received %v", requiredNames, captured)
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
			// dropped_attributes is written by the Tracing itself when a call site
			// passed something outside the span allowlist: the refusal is correct
			// behaviour, and the attempt is a defect to fix — the span counterpart
			// of the proof's dropped_fields assertion.
			if attr.Key == "dropped_attributes" {
				t.Errorf("serve tried to write %d unlisted span attribute(s) on %s", attr.Value.GetIntValue(), span.Name)
				continue
			}
			if !observability.KnownSpanAttribute(observability.LogField(attr.Key)) {
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

// assertCallerCorrelationHashed is the collector-side half of negative control
// (c): the proof submits its HL7 message under an X-Correlation-ID carrying
// `sentinel`, and only the SHA-256 of that header may reach a span. The positive
// half — the admission span carries exactly that hash — is what keeps the
// control honest: a header that never reached the span could not leak under
// `tracingleak` either, and the control would pass for the wrong reason.
func assertCallerCorrelationHashed(t *testing.T, spans []*tracepb.Span, sentinel string) {
	t.Helper()
	want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(sentinel)))
	var admission *tracepb.Span
	for _, span := range spans {
		if strings.Contains(span.String(), sentinel) {
			t.Errorf("the raw caller correlation header reached exported span %s", span.Name)
		}
		if span.Name == "integration.admission" {
			admission = span
		}
	}
	if admission == nil {
		t.Fatal("missing admission span")
	}
	if got := spanStringField(admission, "correlation_id"); got != want {
		t.Errorf("admission span correlation_id = %q, want the SHA-256 of the caller header, %q", got, want)
	}
}

// assertDeliveryJoinsAdmission proves that asynchronous work retains durable
// lineage without inheriting the request's OTel trace.
func assertDeliveryJoinsAdmission(t *testing.T, spans []*tracepb.Span) *tracepb.Span {
	t.Helper()
	var admission, delivery *tracepb.Span
	for _, span := range spans {
		switch span.Name {
		case "integration.admission":
			admission = span
		case "integration.delivery.dispatch":
			delivery = span
		}
	}
	if admission == nil || delivery == nil {
		t.Fatal("missing admission/delivery spans")
	}
	for _, key := range []string{"receipt_id", "lineage_trace_id"} {
		want := spanStringField(admission, key)
		if got := spanStringField(delivery, key); want == "" || got != want {
			t.Errorf("delivery %s = %q, admission = %q", key, got, want)
		}
	}
	if spanStringField(delivery, "delivery_attempt_id") == "" {
		t.Error("delivery span lacks delivery_attempt_id")
	}
	if len(delivery.TraceId) != 16 || bytes.Equal(delivery.TraceId, make([]byte, 16)) || bytes.Equal(delivery.TraceId, admission.TraceId) {
		t.Error("delivery must have a valid separate OTel trace")
	}
	if bytes.Equal(delivery.ParentSpanId, admission.SpanId) {
		t.Error("delivery must not be parented under admission")
	}
	return delivery
}

func deliveryAttemptIDs(t *testing.T, dsn, receiptID string) []string {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT attempt_id FROM integration_delivery_attempts WHERE tenant_id = $1 AND receipt_id = $2`, documentedTenant, receiptID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
