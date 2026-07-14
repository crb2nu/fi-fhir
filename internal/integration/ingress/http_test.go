package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestHandlerBearerAcceptedProjectsRawFreeDurableResponse(t *testing.T) {
	processorFake := &fakeProcessor{result: acceptedResult()}
	handler := newTestHandler(t, AuthModeBearer, processorFake, DefaultMaxBodyBytes)
	body := []byte("MSH|^~\\&|RAW-PHI-SENTINEL|FAC")
	request := validRequest(body)
	request.Header.Set("Authorization", "Bearer "+testBearerSecret)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if processorFake.calls != 1 {
		t.Fatalf("processor calls = %d", processorFake.calls)
	}
	processed := processorFake.request
	if processed.Mode != integration.ExecutionModeProduction || processed.Security.Principal.Kind != integration.PrincipalKindService || processed.Security.Principal.SourceID != "adt-east" {
		t.Fatalf("trusted production request = %#v", processed)
	}
	if !bytes.Equal(processed.Envelope.Bytes(), body) || processed.IdempotencyKey != "golden-idem" || processed.CorrelationID != "golden-correlation" {
		t.Fatalf("request transport facts drifted: %#v", processed)
	}
	for _, required := range []string{`"receipt"`, `"warnings"`, `"MISSING_PV1"`, `"deliveries"`, `"queued"`, `"profile-1"`, `"workflow-1"`} {
		if !strings.Contains(recorder.Body.String(), required) {
			t.Fatalf("response lacks %s: %s", required, recorder.Body.String())
		}
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", testBearerSecret, `"security"`, `"payload"`} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("response disclosed %q: %s", forbidden, recorder.Body.String())
		}
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers = %#v", recorder.Header())
	}
}

func TestHandlerHMACAcceptedAndTamperRejected(t *testing.T) {
	processorFake := &fakeProcessor{result: acceptedResult()}
	handler := newTestHandler(t, AuthModeHMAC, processorFake, DefaultMaxBodyBytes)
	body := []byte("MSH|^~\\&|APP|FAC")
	request := validRequest(body)
	request.Header.Set(signatureHeader, hmacHeader(testHMACKey, "adt-tolerant", "golden-idem", "golden-correlation", body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("valid status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	tampered := validRequest(append(body, 'X'))
	tampered.Header.Set(signatureHeader, request.Header.Get(signatureHeader))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, tampered)
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("tampered status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if challenge := recorder.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Fatalf("HMAC rejection emitted bearer challenge %q", challenge)
	}
}

func TestHandlerStructuredTransportRejections(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*http.Request)
		body       []byte
		maxBytes   int64
		wantStatus int
		wantCode   string
	}{
		{name: "wrong method", mutate: func(r *http.Request) { r.Method = http.MethodGet }, wantStatus: http.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
		{name: "query", mutate: func(r *http.Request) { r.URL.RawQuery = "integration=other" }, wantStatus: http.StatusNotFound, wantCode: "ENDPOINT_NOT_FOUND"},
		{name: "origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, wantStatus: http.StatusForbidden, wantCode: "BROWSER_ORIGIN_FORBIDDEN"},
		{name: "compression", mutate: func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, wantStatus: http.StatusUnsupportedMediaType, wantCode: "CONTENT_ENCODING_UNSUPPORTED"},
		{name: "media type", mutate: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, wantStatus: http.StatusUnsupportedMediaType, wantCode: "MEDIA_TYPE_UNSUPPORTED"},
		{name: "media parameters", mutate: func(r *http.Request) { r.Header.Set("Content-Type", hl7MediaType+"; charset=utf-8") }, wantStatus: http.StatusUnsupportedMediaType, wantCode: "MEDIA_TYPE_UNSUPPORTED"},
		{name: "wrong integration", mutate: func(r *http.Request) { r.Header.Set(integrationHeader, "other") }, wantStatus: http.StatusNotFound, wantCode: "INTEGRATION_UNAVAILABLE"},
		{name: "missing auth", mutate: func(r *http.Request) { r.Header.Del("Authorization") }, wantStatus: http.StatusUnauthorized, wantCode: "AUTHENTICATION_REQUIRED"},
		{name: "wrong auth", mutate: func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_CREDENTIALS"},
		{name: "empty body", body: []byte{}, wantStatus: http.StatusBadRequest, wantCode: "BODY_REQUIRED"},
		{name: "fixed oversized", body: []byte("12345"), maxBytes: 4, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PAYLOAD_TOO_LARGE"},
		{name: "chunked oversized", body: []byte("12345"), maxBytes: 4, mutate: func(r *http.Request) { r.ContentLength = -1 }, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PAYLOAD_TOO_LARGE"},
		{name: "duplicate idempotency", mutate: func(r *http.Request) { r.Header.Add(idempotencyHeader, "other") }, wantStatus: http.StatusBadRequest, wantCode: "INVALID_IDEMPOTENCY_KEY"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := test.body
			if body == nil {
				body = []byte("MSH|^~\\&|APP|FAC")
			}
			maxBytes := test.maxBytes
			if maxBytes == 0 {
				maxBytes = DefaultMaxBodyBytes
			}
			handler := newTestHandler(t, AuthModeBearer, &fakeProcessor{result: acceptedResult()}, maxBytes)
			request := validRequest(body)
			request.Header.Set("Authorization", "Bearer "+testBearerSecret)
			if test.mutate != nil {
				test.mutate(request)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status/body = %d %s, want %d %s", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
			var response errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Error.Diagnostics) != 1 || response.Error.Diagnostics[0].Path == "" {
				t.Fatalf("structured error = %#v err=%v", response, err)
			}
			if test.wantStatus == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") != `Bearer realm="fi-fhir-ingress"` {
				t.Fatalf("bearer challenge = %q", recorder.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestHandlerDoesNotRevealBoundIntegrationBeforeAuthentication(t *testing.T) {
	handler := newTestHandler(t, AuthModeBearer, &fakeProcessor{result: acceptedResult()}, DefaultMaxBodyBytes)
	for _, integrationID := range []string{"adt-tolerant", "other"} {
		request := validRequest([]byte("MSH|^~\\&|APP|FAC"))
		request.Header.Set(integrationHeader, integrationID)
		request.Header.Del("Authorization")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "AUTHENTICATION_REQUIRED") {
			t.Fatalf("integration %q status/body = %d %s", integrationID, recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandlerOptionalIdempotencyAndCorrelationHeaders(t *testing.T) {
	processorFake := &fakeProcessor{result: acceptedResult()}
	handler := newTestHandler(t, AuthModeBearer, processorFake, DefaultMaxBodyBytes)
	request := validRequest([]byte("MSH|^~\\&|APP|FAC"))
	request.Header.Set("Authorization", "Bearer "+testBearerSecret)
	request.Header.Del(idempotencyHeader)
	request.Header.Del(correlationHeader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if processorFake.request.IdempotencyKey != "" || processorFake.request.CorrelationID != "generated-correlation" {
		t.Fatalf("optional transport facts = %#v", processorFake.request)
	}
}

func TestHandlerMapsSubmissionErrorsWithoutLeakingDetails(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
		retryable  bool
	}{
		{err: processor.ErrInvalidSourceMessage, wantStatus: http.StatusUnprocessableEntity, wantCode: "INVALID_HL7V2_MESSAGE"},
		{err: processor.ErrIdempotencyConflict, wantStatus: http.StatusConflict, wantCode: "IDEMPOTENCY_CONFLICT"},
		{err: processor.ErrDurableSubmissionFailed, wantStatus: http.StatusServiceUnavailable, wantCode: "SUBMISSION_UNAVAILABLE", retryable: true},
		{err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantCode: "SUBMISSION_TIMEOUT", retryable: true},
	}
	for _, test := range tests {
		processorFake := &fakeProcessor{err: test.err}
		handler := newTestHandler(t, AuthModeBearer, processorFake, DefaultMaxBodyBytes)
		request := validRequest([]byte("MSH|^~\\&|RAW-DETAIL|FAC"))
		request.Header.Set("Authorization", "Bearer "+testBearerSecret)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
			t.Fatalf("error %v status/body = %d %s", test.err, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "RAW-DETAIL") || strings.Contains(recorder.Body.String(), test.err.Error()) {
			t.Fatalf("error leaked detail: %s", recorder.Body.String())
		}
		var response errorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Error.Retryable != test.retryable {
			t.Fatalf("retryable response = %#v err=%v", response, err)
		}
		if test.retryable && recorder.Header().Get("Retry-After") != "1" {
			t.Fatalf("retryable response lacks Retry-After: %#v", recorder.Header())
		}
	}
}

func newTestHandler(t *testing.T, mode AuthMode, processorFake *fakeProcessor, maxBytes int64) *Handler {
	t.Helper()
	secret := testBearerSecret
	if mode == AuthModeHMAC {
		secret = testHMACKey
	}
	authenticator := mustAuthenticator(t, AuthConfig{
		Mode: mode, Secret: secret, PrincipalID: "source-service", IntegrationID: "adt-tolerant",
	})
	service, err := NewService(ServiceConfig{
		TenantID: "tenant-a", PrincipalID: authenticator.PrincipalID(), AuthMethod: authenticator.AuthMethod(),
		Registry: fakeRegistry{}, Processor: processorFake,
		Clock: func() time.Time { return time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC) },
		NewID: func() string { return "generated-correlation" },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHandler(HandlerConfig{MaxBodyBytes: maxBytes, Authenticator: authenticator, Service: service})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func validRequest(body []byte) *http.Request {
	request := httptest.NewRequest(http.MethodPost, Path, bytes.NewReader(body))
	request.Header.Set("Content-Type", hl7MediaType)
	request.Header.Set(integrationHeader, "adt-tolerant")
	request.Header.Set(idempotencyHeader, "golden-idem")
	request.Header.Set(correlationHeader, "golden-correlation")
	return request
}

type fakeRegistry struct{}

func (fakeRegistry) LookupPreviewBinding(_ context.Context, tenantID, integrationID string) (Binding, error) {
	if tenantID != "tenant-a" {
		return Binding{}, registry.ErrTenantMismatch
	}
	if integrationID != "adt-tolerant" {
		return Binding{}, registry.ErrIntegrationNotFound
	}
	return Binding{
		IntegrationRevision: integration.ArtifactRevisionRef{ArtifactID: "definition", RevisionID: "definition-1", Digest: digest("a")},
		SourceID:            "adt-east",
		Format:              events.FormatHL7v2,
		Classification:      integration.DataClassificationPHI,
	}, nil
}

type fakeProcessor struct {
	result  integration.ProcessResult
	err     error
	calls   int
	request integration.ProcessRequest
}

func (f *fakeProcessor) Process(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	f.calls++
	f.request = request
	if f.err != nil {
		switch {
		case errors.Is(f.err, processor.ErrInvalidSourceMessage):
			return integration.ProcessResult{}, processor.ErrInvalidSourceMessage
		case errors.Is(f.err, processor.ErrIdempotencyConflict):
			return integration.ProcessResult{}, processor.ErrIdempotencyConflict
		case errors.Is(f.err, context.DeadlineExceeded):
			return integration.ProcessResult{}, context.DeadlineExceeded
		default:
			return integration.ProcessResult{}, f.err
		}
	}
	return f.result, nil
}

func acceptedResult() integration.ProcessResult {
	definition := integration.ArtifactRevisionRef{ArtifactID: "definition", RevisionID: "definition-1", Digest: digest("a")}
	profile := integration.ArtifactRevisionRef{ArtifactID: "profile", RevisionID: "profile-1", Digest: digest("b")}
	workflow := integration.ArtifactRevisionRef{ArtifactID: "workflow", RevisionID: "workflow-1", Digest: digest("c")}
	destination := integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir", RevisionID: "destination-1", Digest: digest("d")},
		Class:               integration.DestinationClassProduction,
	}
	return integration.ProcessResult{
		Mode: integration.ExecutionModeProduction, TenantID: "tenant-a", IntegrationRevision: definition,
		ArtifactRevisions: &integration.ExecutionArtifactRevisions{Source: integration.ArtifactRevisionRef{ArtifactID: "source", RevisionID: "source-1", Digest: digest("e")}, Profile: profile, Workflow: workflow},
		Receipt:           &integration.Receipt{ID: "receipt-1", Status: integration.ReceiptStatusAccepted, IdempotencyKey: "golden-idem", RecordedAt: time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)},
		Events:            []integration.ProcessedEvent{{ID: "event-1", Type: events.EventPatientAdmit, SourceMessageID: "control-1"}},
		Diagnostics:       []integration.Diagnostic{{Severity: integration.DiagnosticSeverityWarning, Stage: "semantic", Code: "MISSING_PV1", Path: "PV1"}},
		Routes:            []integration.RouteResult{{EventID: "event-1", Route: "admit", Matched: true, PlannedActions: []string{"send-fhir"}}},
		Deliveries:        []integration.DeliveryResult{{EventID: "event-1", Route: "admit", Action: "send-fhir", Status: integration.DeliveryStatusQueued, Destination: destination}},
		Correlations:      integration.CorrelationIDs{TenantID: "tenant-a", CorrelationID: "golden-correlation", TraceID: "trace-1", SourceMessageID: "control-1", EventIDs: []string{"event-1"}},
	}
}

func digest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
