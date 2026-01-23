package resolvers

import (
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/projections"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/fhir/subscription"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/explain"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

// SubscriptionRecord tracks a FHIR subscription with its metadata.
type SubscriptionRecord struct {
	ID        string
	Name      string
	Server    string
	Criteria  string
	Endpoint  string
	Status    string
	CreatedAt time.Time
}

// workflowSubscriber represents a subscriber to workflow events.
type workflowSubscriber struct {
	ch           chan *model.WorkflowEventNotification
	workflowName string // empty means all workflows
}

// Resolver is the root resolver for the GraphQL API.
// It holds dependencies needed by all resolvers.
type Resolver struct {
	// Store provides event and patient storage
	Store store.EventStore

	// ProfileStore provides source profile storage
	ProfileStore store.ProfileStore

	// WorkflowEngine executes workflow rules
	WorkflowEngine *workflow.Engine

	// Projections provides access to event sourcing projections
	Projections *projections.Service

	// WarningExplainer provides LLM-powered warning explanations
	WarningExplainer *explain.WarningExplainer

	// SubscriptionClients maps FHIR server URLs to clients
	subscriptionClients map[string]*subscription.Client
	subscriptionRecords map[string]*SubscriptionRecord
	subscriptionMu      sync.RWMutex

	// Workflow event subscribers
	workflowSubscribers []*workflowSubscriber
	workflowSubMu       sync.RWMutex

	// Server metadata
	Version   string
	StartTime time.Time
}

// GetProfileStore exposes the profile store to non-GraphQL HTTP handlers in the graphql server.
func (r *Resolver) GetProfileStore() store.ProfileStore {
	return r.ProfileStore
}

// NewResolver creates a new resolver with all dependencies.
func NewResolver(opts ...ResolverOption) *Resolver {
	r := &Resolver{
		Store:               store.NewMemoryStore(),
		Projections:         projections.NewService(nil), // In-memory projections by default
		subscriptionClients: make(map[string]*subscription.Client),
		subscriptionRecords: make(map[string]*SubscriptionRecord),
		Version:             "0.1.0",
		StartTime:           time.Now(),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// ResolverOption configures the resolver.
type ResolverOption func(*Resolver)

// WithStore sets the event store.
func WithStore(s store.EventStore) ResolverOption {
	return func(r *Resolver) {
		r.Store = s
	}
}

// WithWorkflowEngine sets the workflow engine.
func WithWorkflowEngine(e *workflow.Engine) ResolverOption {
	return func(r *Resolver) {
		r.WorkflowEngine = e
	}
}

// WithVersion sets the server version.
func WithVersion(v string) ResolverOption {
	return func(r *Resolver) {
		r.Version = v
	}
}

// WithProjectionService sets the projection service.
func WithProjectionService(p *projections.Service) ResolverOption {
	return func(r *Resolver) {
		r.Projections = p
	}
}

// WithProfileStore sets the profile store.
func WithProfileStore(s store.ProfileStore) ResolverOption {
	return func(r *Resolver) {
		r.ProfileStore = s
	}
}

// WithWarningExplainer sets the LLM-powered warning explainer.
func WithWarningExplainer(e *explain.WarningExplainer) ResolverOption {
	return func(r *Resolver) {
		r.WarningExplainer = e
	}
}

// getOrCreateSubscriptionClient returns an existing client for the server or creates a new one.
func (r *Resolver) getOrCreateSubscriptionClient(serverURL string) (*subscription.Client, error) {
	r.subscriptionMu.RLock()
	client, exists := r.subscriptionClients[serverURL]
	r.subscriptionMu.RUnlock()

	if exists {
		return client, nil
	}

	r.subscriptionMu.Lock()
	defer r.subscriptionMu.Unlock()

	// Double-check after acquiring write lock
	if client, exists = r.subscriptionClients[serverURL]; exists {
		return client, nil
	}

	// Create new client for this server
	newClient, err := subscription.NewClient(&subscription.ClientConfig{
		FHIREndpoint: serverURL,
	})
	if err != nil {
		return nil, err
	}

	r.subscriptionClients[serverURL] = newClient
	return newClient, nil
}

// storeSubscriptionRecord saves a subscription record.
func (r *Resolver) storeSubscriptionRecord(record *SubscriptionRecord) {
	r.subscriptionMu.Lock()
	defer r.subscriptionMu.Unlock()
	r.subscriptionRecords[record.ID] = record
}

// getSubscriptionRecord retrieves a subscription record by ID.
func (r *Resolver) getSubscriptionRecord(id string) (*SubscriptionRecord, bool) {
	r.subscriptionMu.RLock()
	defer r.subscriptionMu.RUnlock()
	record, exists := r.subscriptionRecords[id]
	return record, exists
}

// deleteSubscriptionRecord removes a subscription record.
func (r *Resolver) deleteSubscriptionRecord(id string) {
	r.subscriptionMu.Lock()
	defer r.subscriptionMu.Unlock()
	delete(r.subscriptionRecords, id)
}

// updateSubscriptionStatus updates the status of a subscription record.
func (r *Resolver) updateSubscriptionStatus(id, status string) bool {
	r.subscriptionMu.Lock()
	defer r.subscriptionMu.Unlock()
	if record, exists := r.subscriptionRecords[id]; exists {
		record.Status = status
		return true
	}
	return false
}

// subscribeToWorkflowEvents creates a subscription channel for workflow events.
func (r *Resolver) subscribeToWorkflowEvents(workflowName string) chan *model.WorkflowEventNotification {
	ch := make(chan *model.WorkflowEventNotification, 100)
	sub := &workflowSubscriber{
		ch:           ch,
		workflowName: workflowName,
	}

	r.workflowSubMu.Lock()
	r.workflowSubscribers = append(r.workflowSubscribers, sub)
	r.workflowSubMu.Unlock()

	return ch
}

// unsubscribeFromWorkflowEvents removes a subscription channel.
func (r *Resolver) unsubscribeFromWorkflowEvents(ch chan *model.WorkflowEventNotification) {
	r.workflowSubMu.Lock()
	defer r.workflowSubMu.Unlock()

	for i, sub := range r.workflowSubscribers {
		if sub.ch == ch {
			r.workflowSubscribers = append(r.workflowSubscribers[:i], r.workflowSubscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// broadcastWorkflowEvent sends a workflow event notification to all matching subscribers.
func (r *Resolver) broadcastWorkflowEvent(notification *model.WorkflowEventNotification) {
	r.workflowSubMu.RLock()
	defer r.workflowSubMu.RUnlock()

	for _, sub := range r.workflowSubscribers {
		// Match if subscriber wants all workflows or this specific workflow
		if sub.workflowName == "" || sub.workflowName == notification.Workflow {
			// Non-blocking send to avoid slow subscribers blocking others
			select {
			case sub.ch <- notification:
			default:
				// Channel full, skip this notification for this subscriber
			}
		}
	}
}
