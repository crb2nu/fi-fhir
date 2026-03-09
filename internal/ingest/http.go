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

// DefaultMaxBodySize is the default maximum webhook body size (10 MB).
const DefaultMaxBodySize int64 = 10 * 1024 * 1024

// HandlerConfig configures the generic webhook handler.
type HandlerConfig struct {
	// MaxBodySize limits the request body size in bytes. 0 uses DefaultMaxBodySize.
	MaxBodySize int64
	// Auth configures request authentication. Nil or zero-value disables auth.
	Auth *AuthConfig
}

// maxBodySize returns the effective body size limit.
func (c *HandlerConfig) maxBodySize() int64 {
	if c != nil && c.MaxBodySize > 0 {
		return c.MaxBodySize
	}
	return DefaultMaxBodySize
}

// GenericWebhookEvent acts as a container for webhooks that haven't been mapped to a complex FHIR canonical type.
type GenericWebhookEvent struct {
	events.EventMeta
	Payload json.RawMessage `json:"payload"`
}

// Handler handles generic incoming HTTP webhooks.
type Handler struct {
	logger workflow.Logger
	engine *workflow.Engine
	config *HandlerConfig
}

// NewHandler creates a new webhook handler with the given configuration.
// If config is nil, defaults are used (10MB body limit, no auth).
func NewHandler(logger workflow.Logger, engine *workflow.Engine, config *HandlerConfig) *Handler {
	return &Handler{
		logger: logger,
		engine: engine,
		config: config,
	}
}

// ServeHTTP implements http.Handler to accept generic JSON payloads.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Enforce body size limit
	maxSize := h.config.maxBodySize()
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			h.logger.Warn(ctx, "Webhook body exceeds size limit",
				workflow.F("max_bytes", maxSize),
			)
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		h.logger.Error(ctx, "Failed to read webhook body", workflow.F("error", err.Error()))
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer func() {
		_ = r.Body.Close()
	}()

	if len(body) == 0 {
		h.logger.Warn(ctx, "Received empty webhook body")
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	// Verify request authentication
	if h.config != nil && h.config.Auth != nil {
		if authErr := h.config.Auth.VerifyRequest(r, body); authErr != nil {
			h.logger.Warn(ctx, "Webhook authentication failed",
				workflow.F("error", authErr.Error()),
				workflow.F("remote_addr", r.RemoteAddr),
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
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
	_, _ = fmt.Fprintln(w, `{"status":"accepted"}`)
}
