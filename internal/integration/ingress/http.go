package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	Path                = "/v1/hl7v2"
	DefaultMaxBodyBytes = int64(1 << 20)
	integrationHeader   = "X-Fi-Fhir-Integration-ID"
	idempotencyHeader   = "Idempotency-Key"
	correlationHeader   = "X-Correlation-ID"
	hl7MediaType        = "application/hl7-v2+er7"
)

type HandlerConfig struct {
	MaxBodyBytes  int64
	Authenticator *Authenticator
	Service       *Service
}

type Handler struct {
	maxBodyBytes  int64
	authenticator *Authenticator
	service       *Service
}

func NewHandler(config HandlerConfig) (*Handler, error) {
	if config.Authenticator == nil || config.Service == nil {
		return nil, ErrUnavailable
	}
	maxBodyBytes := config.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = DefaultMaxBodyBytes
	}
	if maxBodyBytes <= 0 || maxBodyBytes > DefaultMaxBodyBytes {
		return nil, errors.New("HTTP ingress body limit must be between 1 byte and 1 MiB")
	}
	return &Handler{
		maxBodyBytes:  maxBodyBytes,
		authenticator: config.Authenticator,
		service:       config.Service,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setResponseHeaders(w)
	if h == nil || h.authenticator == nil || h.service == nil || r == nil {
		writeError(w, http.StatusServiceUnavailable, "SUBMISSION_UNAVAILABLE", "durable submission is temporarily unavailable", true, "transport")
		return
	}
	if r.URL.Path != Path || r.URL.RawQuery != "" {
		writeError(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "endpoint not found", false, "request.path")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "HL7v2 ingress requires POST", false, "request.method")
		return
	}
	if r.Header.Get("Origin") != "" {
		writeError(w, http.StatusForbidden, "BROWSER_ORIGIN_FORBIDDEN", "browser-origin requests are not allowed", false, "request.headers.origin")
		return
	}
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		writeError(w, http.StatusUnsupportedMediaType, "CONTENT_ENCODING_UNSUPPORTED", "compressed HL7v2 bodies are not supported", false, "request.headers.content-encoding")
		return
	}
	mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != hl7MediaType || len(parameters) != 0 {
		writeError(w, http.StatusUnsupportedMediaType, "MEDIA_TYPE_UNSUPPORTED", "HL7v2 ingress requires application/hl7-v2+er7", false, "request.headers.content-type")
		return
	}
	integrationID, found := singleHeader(r.Header, integrationHeader)
	integrationAvailable := found && integrationID != "" && integrationID == h.authenticator.IntegrationID()
	if r.ContentLength > h.maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "HL7v2 payload exceeds the configured limit", false, "request.body")
		return
	}
	if !h.authenticator.RequiresBody() {
		if err := h.authenticator.Authenticate(r, nil); err != nil {
			writeAuthenticationError(w, err, true)
			return
		}
		if !integrationAvailable {
			writeError(w, http.StatusNotFound, "INTEGRATION_UNAVAILABLE", "integration is unavailable", false, "request.headers.x-fi-fhir-integration-id")
			return
		}
	}
	body, err := readBoundedBody(r.Body, h.maxBodyBytes)
	if err != nil {
		if errors.Is(err, ErrPayloadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "HL7v2 payload exceeds the configured limit", false, "request.body")
			return
		}
		writeError(w, http.StatusBadRequest, "BODY_UNREADABLE", "HL7v2 request body could not be read", false, "request.body")
		return
	}
	if h.authenticator.RequiresBody() {
		if err := h.authenticator.Authenticate(r, body); err != nil {
			writeAuthenticationError(w, err, false)
			return
		}
		if !integrationAvailable {
			writeError(w, http.StatusNotFound, "INTEGRATION_UNAVAILABLE", "integration is unavailable", false, "request.headers.x-fi-fhir-integration-id")
			return
		}
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "BODY_REQUIRED", "HL7v2 request body is required", false, "request.body")
		return
	}
	idempotencyKey, ok := optionalSingleHeader(r.Header, idempotencyHeader)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "idempotency key header is invalid", false, "request.headers.idempotency-key")
		return
	}
	correlationID, ok := optionalSingleHeader(r.Header, correlationHeader)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_CORRELATION_ID", "correlation header is invalid", false, "request.headers.x-correlation-id")
		return
	}
	result, err := h.service.Submit(r.Context(), Input{
		IntegrationID:  integrationID,
		Payload:        body,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  correlationID,
	})
	if err != nil {
		writeSubmissionError(w, err)
		return
	}
	response, err := projectAcceptedResponse(result)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "SUBMISSION_UNAVAILABLE", "durable submission is temporarily unavailable", true, "result")
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func readBoundedBody(body io.ReadCloser, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, ErrInvalidInput
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, ErrPayloadTooLarge
	}
	return raw, nil
}

func optionalSingleHeader(header http.Header, name string) (string, bool) {
	values := header.Values(name)
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 || values[0] == "" {
		return "", false
	}
	return values[0], true
}

func writeAuthenticationError(w http.ResponseWriter, err error, bearerChallenge bool) {
	if bearerChallenge {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fi-fhir-ingress"`)
	}
	code := "INVALID_CREDENTIALS"
	if errors.Is(err, ErrMissingCredentials) {
		code = "AUTHENTICATION_REQUIRED"
	}
	writeError(w, http.StatusUnauthorized, code, "authentication required", false, "request.credentials")
}

func writeSubmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		writeRetryableError(w, http.StatusServiceUnavailable, "REQUEST_CANCELED", "durable submission was not confirmed", "request")
	case errors.Is(err, context.DeadlineExceeded):
		writeRetryableError(w, http.StatusGatewayTimeout, "SUBMISSION_TIMEOUT", "durable submission was not confirmed", "request")
	case errors.Is(err, ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "HL7v2 submission request is invalid", false, "request")
	case errors.Is(err, ErrPayloadTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "HL7v2 payload exceeds the configured limit", false, "request.body")
	case errors.Is(err, ErrIntegrationUnavailable), errors.Is(err, ErrForbidden):
		writeError(w, http.StatusNotFound, "INTEGRATION_UNAVAILABLE", "integration is unavailable", false, "integration")
	case errors.Is(err, ErrInvalidMessage):
		writeError(w, http.StatusUnprocessableEntity, "INVALID_HL7V2_MESSAGE", "HL7v2 message does not satisfy the selected profile", false, "request.body")
	case errors.Is(err, ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was already used for a different request", false, "request.headers.idempotency-key")
	default:
		writeRetryableError(w, http.StatusServiceUnavailable, "SUBMISSION_UNAVAILABLE", "durable submission is temporarily unavailable", "durability")
	}
}

func writeRetryableError(w http.ResponseWriter, status int, code, message, path string) {
	w.Header().Set("Retry-After", "1")
	writeError(w, status, code, message, true, path)
}

type acceptedResponse struct {
	Receipt             receiptSummary                         `json:"receipt"`
	Events              []eventSummary                         `json:"events"`
	Warnings            []diagnosticSummary                    `json:"warnings"`
	IntegrationRevision integration.ArtifactRevisionRef        `json:"integration_revision"`
	ArtifactRevisions   integration.ExecutionArtifactRevisions `json:"artifact_revisions"`
	Routes              []routeSummary                         `json:"routes"`
	Deliveries          []deliverySummary                      `json:"deliveries"`
	Correlations        integration.CorrelationIDs             `json:"correlations"`
}

type receiptSummary struct {
	ID             string                    `json:"id"`
	Status         integration.ReceiptStatus `json:"status"`
	IdempotencyKey string                    `json:"idempotency_key"`
	RecordedAt     string                    `json:"recorded_at"`
}

type eventSummary struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	SourceMessageID string `json:"source_message_id,omitempty"`
}

type diagnosticSummary struct {
	Severity integration.DiagnosticSeverity `json:"severity"`
	Stage    string                         `json:"stage"`
	Code     string                         `json:"code"`
	Path     string                         `json:"path,omitempty"`
}

type routeSummary struct {
	EventID        string   `json:"event_id"`
	Route          string   `json:"route"`
	Matched        bool     `json:"matched"`
	Skipped        bool     `json:"skipped"`
	PlannedActions []string `json:"planned_actions"`
}

type deliverySummary struct {
	EventID     string                             `json:"event_id"`
	Route       string                             `json:"route"`
	Action      string                             `json:"action"`
	Status      integration.DeliveryStatus         `json:"status"`
	Destination integration.DestinationRevisionRef `json:"destination"`
}

func projectAcceptedResponse(result integration.ProcessResult) (acceptedResponse, error) {
	if result.Receipt == nil || result.ArtifactRevisions == nil {
		return acceptedResponse{}, ErrUnavailable
	}
	response := acceptedResponse{
		Receipt: receiptSummary{
			ID:             result.Receipt.ID,
			Status:         result.Receipt.Status,
			IdempotencyKey: result.Receipt.IdempotencyKey,
			RecordedAt:     result.Receipt.RecordedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
		},
		IntegrationRevision: result.IntegrationRevision,
		ArtifactRevisions:   *result.ArtifactRevisions,
		Correlations:        result.Correlations,
		Events:              make([]eventSummary, 0, len(result.Events)),
		Warnings:            make([]diagnosticSummary, 0, len(result.Diagnostics)),
		Routes:              make([]routeSummary, 0, len(result.Routes)),
		Deliveries:          make([]deliverySummary, 0, len(result.Deliveries)),
	}
	for _, event := range result.Events {
		response.Events = append(response.Events, eventSummary{ID: event.ID, Type: string(event.Type), SourceMessageID: event.SourceMessageID})
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity != integration.DiagnosticSeverityWarning {
			continue
		}
		response.Warnings = append(response.Warnings, diagnosticSummary{
			Severity: diagnostic.Severity,
			Stage:    diagnostic.Stage,
			Code:     diagnostic.Code,
			Path:     diagnostic.Path,
		})
	}
	for _, route := range result.Routes {
		response.Routes = append(response.Routes, routeSummary{
			EventID:        route.EventID,
			Route:          route.Route,
			Matched:        route.Matched,
			Skipped:        route.Skipped,
			PlannedActions: append([]string(nil), route.PlannedActions...),
		})
	}
	for _, delivery := range result.Deliveries {
		response.Deliveries = append(response.Deliveries, deliverySummary{
			EventID:     delivery.EventID,
			Route:       delivery.Route,
			Action:      delivery.Action,
			Status:      delivery.Status,
			Destination: delivery.Destination,
		})
	}
	return response, nil
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	Retryable   bool              `json:"retryable"`
	Diagnostics []errorDiagnostic `json:"diagnostics"`
}

type errorDiagnostic struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

func writeError(w http.ResponseWriter, status int, code, message string, retryable bool, path string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{
		Code:        code,
		Message:     message,
		Retryable:   retryable,
		Diagnostics: []errorDiagnostic{{Code: code, Path: path}},
	}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(`{"error":{"code":"RESPONSE_ENCODING_FAILED","message":"response encoding failed","retryable":true,"diagnostics":[]}}`)
		status = http.StatusInternalServerError
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, bytes.NewReader(append(encoded, '\n')))
}

func setResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
