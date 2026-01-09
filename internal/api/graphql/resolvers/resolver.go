package resolvers

import (
	"time"

	"github.com/cblevins/fi-fhir/internal/api/graphql/projections"
	"github.com/cblevins/fi-fhir/internal/api/graphql/store"
	"github.com/cblevins/fi-fhir/internal/workflow"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

// Resolver is the root resolver for the GraphQL API.
// It holds dependencies needed by all resolvers.
type Resolver struct {
	// Store provides event and patient storage
	Store store.EventStore

	// WorkflowEngine executes workflow rules
	WorkflowEngine *workflow.Engine

	// Projections provides access to event sourcing projections
	Projections *projections.Service

	// Server metadata
	Version   string
	StartTime time.Time
}

// NewResolver creates a new resolver with all dependencies.
func NewResolver(opts ...ResolverOption) *Resolver {
	r := &Resolver{
		Store:       store.NewMemoryStore(),
		Projections: projections.NewService(nil), // In-memory projections by default
		Version:     "0.1.0",
		StartTime:   time.Now(),
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
