package resolvers

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.temporal.io/sdk/client"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/projections"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/fhir/subscription"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/explain"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/extract"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/quality"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	termworkflow "gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/copilot"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
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

	// WorkflowLifecycleStore provides managed workflow definitions/versions/releases.
	WorkflowLifecycleStore store.WorkflowLifecycleStore

	// Parsed workflow engines by immutable workflow version ID.
	workflowVersionEngines map[string]*workflow.Engine
	workflowVersionMu      sync.RWMutex

	// Projections provides access to event sourcing projections
	Projections *projections.Service

	// WarningExplainer provides LLM-powered warning explanations
	WarningExplainer *explain.WarningExplainer

	// ClinicalExtractor provides LLM-powered clinical entity extraction
	ClinicalExtractor *extract.Extractor

	// QualityAnalyzer provides LLM-powered data quality analysis
	QualityAnalyzer *quality.Analyzer

	// WorkflowCopilot provides LLM-powered workflow generation
	WorkflowCopilot *copilot.WorkflowCopilot

	// WorkflowExplainer provides LLM-powered workflow explanations
	WorkflowExplainer *explain.WorkflowExplainer

	// LLMCapability describes safe runtime LLM status for UI/API callers.
	LLMCapability *model.LLMCapability

	// MappingStore provides custom terminology mapping storage
	MappingStore *termdb.MappingStore

	// AutorouteEngine provides LLM-powered mapping suggestions
	AutorouteEngine *autoroute.Engine

	// TemporalClient provides access to Temporal workflow orchestration
	TemporalClient client.Client

	// TemporalWorker manages the Temporal worker for terminology workflows
	TemporalWorker *termworkflow.Worker

	// SubscriptionClients maps FHIR server URLs to clients
	subscriptionClients map[string]*subscription.Client
	subscriptionRecords map[string]*SubscriptionRecord
	subscriptionMu      sync.RWMutex

	// Debug sessions
	debugSessions   map[string]*workflow.DebugSession
	debugSessionsMu sync.RWMutex

	// Recorded workflow run traces keyed by workflow run ID.
	workflowRunTraces   map[string][]model.TraceSpanModel
	workflowRunTracesMu sync.RWMutex

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
		Store:                  store.NewMemoryStore(),
		WorkflowLifecycleStore: store.NewMemoryWorkflowLifecycleStore(),
		workflowVersionEngines: make(map[string]*workflow.Engine),
		debugSessions:          make(map[string]*workflow.DebugSession),
		workflowRunTraces:      make(map[string][]model.TraceSpanModel),
		Projections:            projections.NewService(nil), // In-memory projections by default
		subscriptionClients:    make(map[string]*subscription.Client),
		subscriptionRecords:    make(map[string]*SubscriptionRecord),
		LLMCapability:          DefaultLLMCapability(),
		Version:                "0.1.0",
		StartTime:              time.Now(),
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

// WithWorkflowLifecycleStore sets the workflow lifecycle store.
func WithWorkflowLifecycleStore(s store.WorkflowLifecycleStore) ResolverOption {
	return func(r *Resolver) {
		r.WorkflowLifecycleStore = s
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

// WithClinicalExtractor sets the LLM-powered clinical entity extractor.
func WithClinicalExtractor(e *extract.Extractor) ResolverOption {
	return func(r *Resolver) {
		r.ClinicalExtractor = e
	}
}

// WithQualityAnalyzer sets the LLM-powered data quality analyzer.
func WithQualityAnalyzer(a *quality.Analyzer) ResolverOption {
	return func(r *Resolver) {
		r.QualityAnalyzer = a
	}
}

// WithWorkflowCopilot sets the LLM-powered workflow copilot.
func WithWorkflowCopilot(c *copilot.WorkflowCopilot) ResolverOption {
	return func(r *Resolver) {
		r.WorkflowCopilot = c
	}
}

// WithWorkflowExplainer sets the LLM-powered workflow explainer.
func WithWorkflowExplainer(e *explain.WorkflowExplainer) ResolverOption {
	return func(r *Resolver) {
		r.WorkflowExplainer = e
	}
}

// WithLLMCapability sets safe runtime LLM capability metadata.
func WithLLMCapability(capability *model.LLMCapability) ResolverOption {
	return func(r *Resolver) {
		r.LLMCapability = cloneLLMCapability(capability)
	}
}

// DefaultLLMCapability returns the safe default when LLM runtime state is not wired.
func DefaultLLMCapability() *model.LLMCapability {
	return &model.LLMCapability{
		Enabled:    false,
		Configured: false,
		Status:     "disabled",
		Warnings:   []string{"LLM features are disabled"},
	}
}

// NewLLMCapability builds a secret-safe capability view from runtime configuration.
func NewLLMCapability(enabled bool, cfg llm.Config, status string, warnings []string) *model.LLMCapability {
	configured := cfg.Validate() == nil
	copiedWarnings := append([]string(nil), warnings...)
	host := providerBaseURLHost(cfg.BaseURL)
	if cfg.BaseURL != "" && host == nil {
		copiedWarnings = append(copiedWarnings, "LLM provider base URL is invalid; host unavailable")
	}
	if status == "" {
		switch {
		case !enabled:
			status = "disabled"
		case !configured:
			status = "unavailable"
		default:
			status = "available"
		}
	}

	return &model.LLMCapability{
		Enabled:             enabled,
		Configured:          configured,
		ProviderBaseURLHost: host,
		DefaultModel:        optionalString(cfg.DefaultModel),
		QualityModel:        optionalString(cfg.QualityModel),
		Status:              status,
		Warnings:            copiedWarnings,
	}
}

func cloneLLMCapability(capability *model.LLMCapability) *model.LLMCapability {
	if capability == nil {
		return DefaultLLMCapability()
	}
	clone := *capability
	clone.Warnings = append([]string(nil), capability.Warnings...)
	return &clone
}

func providerBaseURLHost(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil
	}
	host := parsed.Host
	return &host
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// WithMappingStore sets the terminology mapping store.
func WithMappingStore(s *termdb.MappingStore) ResolverOption {
	return func(r *Resolver) {
		r.MappingStore = s
	}
}

// WithAutorouteEngine sets the LLM-powered autoroute engine.
func WithAutorouteEngine(e *autoroute.Engine) ResolverOption {
	return func(r *Resolver) {
		r.AutorouteEngine = e
	}
}

// WithTemporalClient sets the Temporal client for workflow orchestration.
func WithTemporalClient(c client.Client) ResolverOption {
	return func(r *Resolver) {
		r.TemporalClient = c
	}
}

// WithTemporalWorker sets the Temporal worker for terminology workflows.
func WithTemporalWorker(w *termworkflow.Worker) ResolverOption {
	return func(r *Resolver) {
		r.TemporalWorker = w
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

// getOrBuildVersionEngine returns a cached workflow engine for a saved workflow version.
func (r *Resolver) getOrBuildVersionEngine(versionID, yamlContent string) (*workflow.Engine, error) {
	r.workflowVersionMu.RLock()
	engine, ok := r.workflowVersionEngines[versionID]
	r.workflowVersionMu.RUnlock()
	if ok {
		return engine, nil
	}

	parsed, err := workflow.ParseWorkflow([]byte(yamlContent))
	if err != nil {
		return nil, fmt.Errorf("parse workflow version yaml: %w", err)
	}

	engine, err = workflow.NewEngine(parsed)
	if err != nil {
		return nil, fmt.Errorf("create workflow engine from version %s: %w", versionID, err)
	}

	r.workflowVersionMu.Lock()
	r.workflowVersionEngines[versionID] = engine
	r.workflowVersionMu.Unlock()

	return engine, nil
}
