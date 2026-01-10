package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NotificationBundle represents a FHIR notification Bundle.
type NotificationBundle struct {
	ResourceType string              `json:"resourceType"`
	Type         string              `json:"type"`
	Timestamp    string              `json:"timestamp,omitempty"`
	Entry        []NotificationEntry `json:"entry"`
}

// NotificationEntry represents an entry in a notification Bundle.
type NotificationEntry struct {
	FullURL  string                 `json:"fullUrl,omitempty"`
	Resource map[string]interface{} `json:"resource"`
	Request  *EntryRequest          `json:"request,omitempty"`
}

// EntryRequest contains the action that triggered the notification.
type EntryRequest struct {
	Method string `json:"method"` // POST, PUT, DELETE
	URL    string `json:"url"`
}

// EventRouter routes canonical events for processing.
type EventRouter interface {
	Route(ctx context.Context, event interface{}) error
}

// ReceiverMetrics collects metrics for the notification receiver.
type ReceiverMetrics interface {
	NotificationReceived(subscription, resourceType string)
	NotificationProcessed(subscription string, success bool, duration time.Duration)
	NotificationError(subscription, errorType string)
}

// NoOpReceiverMetrics discards all metrics.
type NoOpReceiverMetrics struct{}

func (NoOpReceiverMetrics) NotificationReceived(subscription, resourceType string) {}
func (NoOpReceiverMetrics) NotificationProcessed(subscription string, success bool, d time.Duration) {
}
func (NoOpReceiverMetrics) NotificationError(subscription, errorType string) {}

// SubscriptionConfig holds configuration for a subscription handler.
type SubscriptionConfig struct {
	Name           string
	AllowedSources []string // List of allowed FHIR server origins
	Secret         string   // Optional secret for signature verification
	EventMapping   EventMappingConfig
}

// EventMappingConfig defines how FHIR resources map to canonical events.
type EventMappingConfig struct {
	CreateEvent string             // Event type for create operations
	UpdateEvent string             // Event type for update operations
	DeleteEvent string             // Event type for delete operations
	Rules       []EventMappingRule // Custom rules (evaluated first)
}

// EventMappingRule defines a conditional event mapping.
type EventMappingRule struct {
	Condition string // CEL expression
	EventType string
}

// Receiver handles incoming FHIR subscription notifications.
type Receiver struct {
	mu            sync.RWMutex
	subscriptions map[string]*SubscriptionConfig
	mapper        *FHIRMapper
	router        EventRouter
	metrics       ReceiverMetrics

	pathPrefix     string
	maxBundleSize  int
	allowedSources map[string]bool
	verifySource   bool
}

// ReceiverOptions configures the notification receiver at creation time.
// Note: ReceiverConfig in config.go is for YAML-based configuration.
type ReceiverOptions struct {
	PathPrefix     string
	MaxBundleSize  int
	AllowedSources []string
	VerifySource   bool
	Metrics        ReceiverMetrics
}

// NewReceiver creates a new notification receiver.
func NewReceiver(router EventRouter, opts *ReceiverOptions) *Receiver {
	if opts == nil {
		opts = &ReceiverOptions{}
	}

	pathPrefix := opts.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "/fhir/notify"
	}

	maxBundleSize := opts.MaxBundleSize
	if maxBundleSize == 0 {
		maxBundleSize = 100
	}

	metrics := opts.Metrics
	if metrics == nil {
		metrics = NoOpReceiverMetrics{}
	}

	allowed := make(map[string]bool)
	for _, src := range opts.AllowedSources {
		allowed[strings.TrimSuffix(src, "/")] = true
	}

	return &Receiver{
		subscriptions:  make(map[string]*SubscriptionConfig),
		mapper:         NewFHIRMapper(),
		router:         router,
		metrics:        metrics,
		pathPrefix:     pathPrefix,
		maxBundleSize:  maxBundleSize,
		allowedSources: allowed,
		verifySource:   opts.VerifySource,
	}
}

// RegisterSubscription adds a subscription handler.
func (r *Receiver) RegisterSubscription(config *SubscriptionConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions[config.Name] = config
}

// UnregisterSubscription removes a subscription handler.
func (r *Receiver) UnregisterSubscription(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subscriptions, name)
}

// Handler returns an http.Handler for the receiver.
func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()

	// Handle notifications: /fhir/notify/{subscription_name}
	mux.HandleFunc(r.pathPrefix+"/", r.handleNotification)

	// Health check
	mux.HandleFunc(r.pathPrefix+"/health", r.handleHealth)

	return mux
}

// ServeHTTP implements http.Handler.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Handler().ServeHTTP(w, req)
}

func (r *Receiver) handleNotification(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract subscription name from path
	path := strings.TrimPrefix(req.URL.Path, r.pathPrefix+"/")
	subscriptionName := strings.Split(path, "/")[0]

	if subscriptionName == "" {
		http.Error(w, "Subscription name required", http.StatusBadRequest)
		return
	}

	// Get subscription config
	r.mu.RLock()
	config, exists := r.subscriptions[subscriptionName]
	r.mu.RUnlock()

	if !exists {
		http.Error(w, "Unknown subscription", http.StatusNotFound)
		return
	}

	// Verify source if configured
	if r.verifySource {
		origin := req.Header.Get("Origin")
		if origin == "" {
			origin = req.Header.Get("X-Forwarded-For")
		}
		if !r.isAllowedSource(origin, config.AllowedSources) {
			r.metrics.NotificationError(subscriptionName, "unauthorized_source")
			http.Error(w, "Unauthorized source", http.StatusForbidden)
			return
		}
	}

	// Read and parse bundle
	body, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		r.metrics.NotificationError(subscriptionName, "read_error")
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var bundle NotificationBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		r.metrics.NotificationError(subscriptionName, "parse_error")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if bundle.ResourceType != "Bundle" {
		r.metrics.NotificationError(subscriptionName, "invalid_resource")
		http.Error(w, "Expected Bundle resource", http.StatusBadRequest)
		return
	}

	if len(bundle.Entry) > r.maxBundleSize {
		r.metrics.NotificationError(subscriptionName, "bundle_too_large")
		http.Error(w, fmt.Sprintf("Bundle exceeds maximum size of %d", r.maxBundleSize), http.StatusRequestEntityTooLarge)
		return
	}

	// Process each entry
	ctx := req.Context()
	start := time.Now()
	var processErr error

	for _, entry := range bundle.Entry {
		resourceType, _ := entry.Resource["resourceType"].(string)
		r.metrics.NotificationReceived(subscriptionName, resourceType)

		// Determine action from request
		action := "update"
		if entry.Request != nil {
			switch entry.Request.Method {
			case "POST":
				action = "create"
			case "PUT":
				action = "update"
			case "DELETE":
				action = "delete"
			}
		}

		// Map to canonical event
		event, err := r.mapper.MapResource(entry.Resource, action, &config.EventMapping)
		if err != nil {
			// Log but continue processing
			processErr = err
			continue
		}

		if event == nil {
			// No mapping for this resource/action combination
			continue
		}

		// Route the event
		if err := r.router.Route(ctx, event); err != nil {
			processErr = err
			// Continue processing other entries
		}
	}

	duration := time.Since(start)
	r.metrics.NotificationProcessed(subscriptionName, processErr == nil, duration)

	// Return 200 even if some events failed to route
	// This prevents the FHIR server from retrying the entire bundle
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "accepted"}`))
}

func (r *Receiver) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	r.mu.RLock()
	count := len(r.subscriptions)
	r.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "healthy",
		"subscriptions": count,
	})
}

func (r *Receiver) isAllowedSource(source string, configSources []string) bool {
	if source == "" {
		return false
	}

	// Check config-specific sources first
	for _, allowed := range configSources {
		if strings.HasPrefix(source, allowed) {
			return true
		}
	}

	// Check global allowed sources
	for allowed := range r.allowedSources {
		if strings.HasPrefix(source, allowed) {
			return true
		}
	}

	return false
}

// Server wraps the receiver with TLS and lifecycle management.
type Server struct {
	receiver   *Receiver
	httpServer *http.Server
}

// ServerConfig configures the notification server.
type ServerConfig struct {
	Host         string
	Port         int
	TLSCertFile  string
	TLSKeyFile   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewServer creates a notification server.
func NewServer(receiver *Receiver, config *ServerConfig) *Server {
	if config == nil {
		config = &ServerConfig{}
	}

	host := config.Host
	if host == "" {
		host = "0.0.0.0"
	}

	port := config.Port
	if port == 0 {
		port = 8081
	}

	readTimeout := config.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}

	writeTimeout := config.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 30 * time.Second
	}

	return &Server{
		receiver: receiver,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			Handler:      receiver,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
	}
}

// Start begins listening for notifications.
func (s *Server) Start(certFile, keyFile string) error {
	if certFile != "" && keyFile != "" {
		return s.httpServer.ListenAndServeTLS(certFile, keyFile)
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
