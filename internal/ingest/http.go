package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// GenericWebhookEvent acts as a container for webhooks that haven't been mapped to a complex FHIR canonical type.
type GenericWebhookEvent struct {
	events.EventMeta
	Payload json.RawMessage `json:"payload"`
}

// Handler handles generic incoming HTTP webhooks.
type Handler struct {
	logger workflow.Logger
	engine *workflow.Engine
}

// NewHandler creates a new webhook handler.
func NewHandler(logger workflow.Logger, engine *workflow.Engine) *Handler {
	return &Handler{
		logger: logger,
		engine: engine,
	}
}

// ServeHTTP implements http.Handler to accept generic JSON payloads.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error(ctx, "Failed to read webhook body", workflow.F("error", err.Error()))
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		h.logger.Warn(ctx, "Received empty webhook body")
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	// Try extracting standard event metadata if sent by a known system
	var partial struct {
		EventMeta *events.EventMeta `json:"meta,omitempty"`
		EventType string            `json:"event_type,omitempty"`
		Type      string            `json:"type,omitempty"` // Fallback type field
		Source    string            `json:"source,omitempty"`
	}

	// Not strict parsing since it could be anything
	_ = json.Unmarshal(body, &partial)

	// Determine semantic type
	eType := partial.EventType
	if eType == "" {
		eType = partial.Type
	}
	if eType == "" {
		eType = "webhook_received" // Default unknown events
	}

	eSource := partial.Source
	if eSource == "" {
		eSource = r.Header.Get("X-Source-System")
		if eSource == "" {
			eSource = "generic_webhook" // Default origin
		}
	}

	// Build the generic event model wrapping the raw payload
	meta := events.EventMeta{
		ID:           uuid.NewString(), // Allocate a new processing ID
		Type:         events.EventType(eType),
		ReceivedAt:   time.Now().UTC(),
		Source:       eSource,
		SourceFormat: events.FormatUnknown,
	}

	// Inherit provided meta if applicable
	if partial.EventMeta != nil {
		if partial.EventMeta.ID != "" {
			meta.SourceMessageID = partial.EventMeta.ID
		}
		if !partial.EventMeta.Timestamp.IsZero() {
			meta.Timestamp = partial.EventMeta.Timestamp
		}
	}
	if meta.Timestamp.IsZero() {
		meta.Timestamp = meta.ReceivedAt
	}

	event := &GenericWebhookEvent{
		EventMeta: meta,
		Payload:   body,
	}

	h.logger.Debug(ctx, "Received generic webhook event",
		workflow.F("event_id", event.ID),
		workflow.F("event_type", string(event.Type)),
		workflow.F("source", event.Source),
	)

	// Route to workflow engine
	if h.engine != nil {
		result := h.engine.ProcessWithContext(ctx, event)
		var processErr error
		for _, rr := range result.RouteResults {
			if len(rr.ActionErrors) > 0 {
				processErr = rr.ActionErrors[0]
				break
			}
			if len(rr.TransformErrors) > 0 {
				processErr = rr.TransformErrors[0]
				break
			}
		}

		if processErr != nil {
			h.logger.Error(ctx, "Workflow engine failed to process webhook event",
				workflow.F("error", processErr.Error()),
				workflow.F("event_id", event.ID),
			)
			http.Error(w, "Internal server error during workflow processing", http.StatusInternalServerError)
			return
		}
	} else {
		h.logger.Warn(ctx, "Webhook received but no workflow engine is configured")
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, `{"status":"accepted"}`)
}
