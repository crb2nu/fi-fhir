package subscription

import (
	"context"
	"fmt"
	"sync"

	"github.com/crb2nu/fi-fhir/internal/workflow"
)

// WorkflowRouter routes canonical events through the workflow engine.
type WorkflowRouter struct {
	engine *workflow.Engine
}

// NewWorkflowRouter creates a router that sends events to the workflow engine.
func NewWorkflowRouter(engine *workflow.Engine) *WorkflowRouter {
	return &WorkflowRouter{
		engine: engine,
	}
}

// Route sends an event through the workflow engine.
func (r *WorkflowRouter) Route(ctx context.Context, event interface{}) error {
	if r.engine == nil {
		return fmt.Errorf("workflow engine not configured")
	}

	result := r.engine.ProcessWithContext(ctx, event)

	// Check for errors in route results
	for _, rr := range result.RouteResults {
		if len(rr.ActionErrors) > 0 {
			return rr.ActionErrors[0]
		}
		if len(rr.TransformErrors) > 0 {
			return rr.TransformErrors[0]
		}
	}

	return nil
}

// MultiRouter routes events to multiple destinations.
type MultiRouter struct {
	routers []EventRouter
	mode    MultiRouterMode
}

// MultiRouterMode determines how events are routed to multiple destinations.
type MultiRouterMode int

const (
	// MultiRouterAll routes to all destinations, continuing on error
	MultiRouterAll MultiRouterMode = iota
	// MultiRouterFailFast stops on first error
	MultiRouterFailFast
)

// NewMultiRouter creates a router that sends events to multiple destinations.
func NewMultiRouter(mode MultiRouterMode, routers ...EventRouter) *MultiRouter {
	return &MultiRouter{
		routers: routers,
		mode:    mode,
	}
}

// Route sends an event to all configured routers.
func (r *MultiRouter) Route(ctx context.Context, event interface{}) error {
	var errs []error

	for _, router := range r.routers {
		if err := router.Route(ctx, event); err != nil {
			if r.mode == MultiRouterFailFast {
				return err
			}
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return &MultiRouterError{Errors: errs}
	}

	return nil
}

// MultiRouterError contains errors from multiple routers.
type MultiRouterError struct {
	Errors []error
}

func (e *MultiRouterError) Error() string {
	return fmt.Sprintf("routing failed to %d destination(s)", len(e.Errors))
}

// ChannelRouter routes events to a Go channel for async processing.
type ChannelRouter struct {
	events chan interface{}
	buffer int
}

// NewChannelRouter creates a router that sends events to a channel.
func NewChannelRouter(bufferSize int) *ChannelRouter {
	return &ChannelRouter{
		events: make(chan interface{}, bufferSize),
		buffer: bufferSize,
	}
}

// Route sends an event to the channel.
func (r *ChannelRouter) Route(ctx context.Context, event interface{}) error {
	select {
	case r.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("event channel full")
	}
}

// Events returns the channel for consuming events.
func (r *ChannelRouter) Events() <-chan interface{} {
	return r.events
}

// Close closes the event channel.
func (r *ChannelRouter) Close() {
	close(r.events)
}

// CallbackRouter invokes a callback function for each event.
type CallbackRouter struct {
	callback func(ctx context.Context, event interface{}) error
}

// NewCallbackRouter creates a router that invokes a callback.
func NewCallbackRouter(callback func(ctx context.Context, event interface{}) error) *CallbackRouter {
	return &CallbackRouter{
		callback: callback,
	}
}

// Route invokes the callback with the event.
func (r *CallbackRouter) Route(ctx context.Context, event interface{}) error {
	return r.callback(ctx, event)
}

// FilterRouter wraps a router with a filter function.
type FilterRouter struct {
	router EventRouter
	filter func(event interface{}) bool
}

// NewFilterRouter creates a router that only routes events matching the filter.
func NewFilterRouter(router EventRouter, filter func(event interface{}) bool) *FilterRouter {
	return &FilterRouter{
		router: router,
		filter: filter,
	}
}

// Route sends the event if it passes the filter.
func (r *FilterRouter) Route(ctx context.Context, event interface{}) error {
	if !r.filter(event) {
		return nil // Event filtered out, not an error
	}
	return r.router.Route(ctx, event)
}

// MetricsRouter wraps a router with metrics collection.
type MetricsRouter struct {
	router  EventRouter
	metrics ReceiverMetrics
	name    string
}

// NewMetricsRouter creates a router that collects routing metrics.
func NewMetricsRouter(router EventRouter, metrics ReceiverMetrics, name string) *MetricsRouter {
	return &MetricsRouter{
		router:  router,
		metrics: metrics,
		name:    name,
	}
}

// Route sends the event and records metrics.
func (r *MetricsRouter) Route(ctx context.Context, event interface{}) error {
	err := r.router.Route(ctx, event)
	if err != nil {
		r.metrics.NotificationError(r.name, "routing_error")
	}
	return err
}

// NoOpRouter discards all events.
type NoOpRouter struct{}

// Route does nothing and returns nil.
func (NoOpRouter) Route(ctx context.Context, event interface{}) error {
	return nil
}

// --- Subscription Manager ---

// Manager manages subscriptions and their receivers.
type Manager struct {
	mu sync.RWMutex

	// clients maps server URL to subscription client
	clients map[string]*Client

	// configs maps subscription name to config
	configs map[string]*SubscriptionDefinition

	// receiver handles incoming notifications
	receiver *Receiver

	// router routes events after mapping
	router EventRouter
}

// SubscriptionDefinition defines a subscription to create.
type SubscriptionDefinition struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description,omitempty"`
	Server      string                 `yaml:"server"`
	Auth        AuthConfig             `yaml:"auth,omitempty"`
	Criteria    string                 `yaml:"criteria"`
	Channel     ChannelConfig          `yaml:"channel"`
	Mapping     EventMappingConfig     `yaml:"event_mapping,omitempty"`
	Extra       map[string]interface{} `yaml:"-,inline"`
}

// AuthConfig defines authentication for a FHIR server.
type AuthConfig struct {
	Type         string   `yaml:"type"` // bearer, oauth2, basic
	Token        string   `yaml:"token,omitempty"`
	TokenURL     string   `yaml:"token_url,omitempty"`
	ClientID     string   `yaml:"client_id,omitempty"`
	ClientSecret string   `yaml:"client_secret,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty"`
	Username     string   `yaml:"username,omitempty"`
	Password     string   `yaml:"password,omitempty"`
}

// OAuth2Auth provides OAuth2 client credentials authentication.
// Uses the workflow package's token manager for caching and automatic refresh.
type OAuth2Auth struct {
	config workflow.OAuthConfig
}

// NewOAuth2Auth creates a new OAuth2 auth provider.
func NewOAuth2Auth(tokenURL, clientID, clientSecret string, scopes []string) *OAuth2Auth {
	return &OAuth2Auth{
		config: workflow.OAuthConfig{
			TokenURL:     tokenURL,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       scopes,
		},
	}
}

// GetAuthHeader returns the OAuth2 Bearer token header.
func (a *OAuth2Auth) GetAuthHeader(ctx context.Context) (string, error) {
	token, err := workflow.GetOAuthToken(a.config)
	if err != nil {
		return "", fmt.Errorf("failed to get OAuth token: %w", err)
	}
	return "Bearer " + token, nil
}

// ChannelConfig defines the notification channel.
type ChannelConfig struct {
	Endpoint string   `yaml:"endpoint"`
	Payload  string   `yaml:"payload,omitempty"`
	Headers  []string `yaml:"headers,omitempty"`
}

// NewManager creates a subscription manager.
func NewManager(router EventRouter) *Manager {
	receiver := NewReceiver(router, nil)

	return &Manager{
		clients:  make(map[string]*Client),
		configs:  make(map[string]*SubscriptionDefinition),
		receiver: receiver,
		router:   router,
	}
}

// RegisterSubscription adds a subscription definition.
func (m *Manager) RegisterSubscription(def *SubscriptionDefinition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configs[def.Name] = def

	// Register with receiver
	m.receiver.RegisterSubscription(&SubscriptionConfig{
		Name:         def.Name,
		EventMapping: def.Mapping,
	})

	return nil
}

// CreateSubscription creates a subscription on the FHIR server.
func (m *Manager) CreateSubscription(ctx context.Context, name string) (*Subscription, error) {
	m.mu.RLock()
	def, exists := m.configs[name]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("subscription %q not registered", name)
	}

	// Get or create client for server
	client, err := m.getOrCreateClient(def)
	if err != nil {
		return nil, err
	}

	// Build subscription resource
	sub := &Subscription{
		Status:   StatusRequested,
		Reason:   def.Description,
		Criteria: def.Criteria,
		Channel: Channel{
			Type:     ChannelRestHook,
			Endpoint: def.Channel.Endpoint,
			Payload:  def.Channel.Payload,
			Header:   def.Channel.Headers,
		},
	}

	return client.Create(ctx, sub)
}

// DeleteSubscription removes a subscription from the FHIR server.
func (m *Manager) DeleteSubscription(ctx context.Context, name, subscriptionID string) error {
	m.mu.RLock()
	def, exists := m.configs[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("subscription %q not registered", name)
	}

	client, err := m.getOrCreateClient(def)
	if err != nil {
		return err
	}

	return client.Delete(ctx, subscriptionID)
}

// GetReceiver returns the notification receiver.
func (m *Manager) GetReceiver() *Receiver {
	return m.receiver
}

func (m *Manager) getOrCreateClient(def *SubscriptionDefinition) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, exists := m.clients[def.Server]; exists {
		return client, nil
	}

	// Create auth provider based on config
	var auth AuthProvider
	switch def.Auth.Type {
	case "bearer":
		auth = &StaticTokenAuth{Token: def.Auth.Token}
	case "oauth2":
		auth = NewOAuth2Auth(
			def.Auth.TokenURL,
			def.Auth.ClientID,
			def.Auth.ClientSecret,
			def.Auth.Scopes,
		)
	default:
		// No auth
	}

	client, err := NewClient(&ClientConfig{
		FHIREndpoint: def.Server,
		AuthProvider: auth,
	})
	if err != nil {
		return nil, err
	}

	m.clients[def.Server] = client
	return client, nil
}
