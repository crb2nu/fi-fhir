package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/eventsourcing"
)

// EventStoreManager manages event store connections for workflow actions.
type EventStoreManager struct {
	mu     sync.RWMutex
	stores map[string]*eventsourcing.PostgresStore
	dbs    map[string]*sql.DB
}

// NewEventStoreManager creates a new event store manager.
func NewEventStoreManager() *EventStoreManager {
	return &EventStoreManager{
		stores: make(map[string]*eventsourcing.PostgresStore),
		dbs:    make(map[string]*sql.DB),
	}
}

// GetStore returns an event store for the given connection string.
func (m *EventStoreManager) GetStore(dsn string, tableName string) (*eventsourcing.PostgresStore, error) {
	key := dsn + ":" + tableName

	m.mu.RLock()
	store, exists := m.stores[key]
	m.mu.RUnlock()

	if exists {
		// Verify connection is still alive
		db := m.dbs[key]
		if err := db.Ping(); err == nil {
			return store, nil
		}
		// Connection dead, remove it
		m.mu.Lock()
		db.Close() //nolint:gosec // G104: cleanup of dead connection
		delete(m.stores, key)
		delete(m.dbs, key)
		m.mu.Unlock()
	}

	return m.createStore(dsn, tableName)
}

// createStore creates a new event store connection.
func (m *EventStoreManager) createStore(dsn string, tableName string) (*eventsourcing.PostgresStore, error) {
	key := dsn + ":" + tableName

	// Open database connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close() //nolint:gosec // G104: cleanup on failed connection
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	// Create event store
	config := eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	}
	store := eventsourcing.NewPostgresStore(db, config)

	// Cache connection
	m.mu.Lock()
	m.stores[key] = store
	m.dbs[key] = db
	m.mu.Unlock()

	return store, nil
}

// Close closes all event store connections.
func (m *EventStoreManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for key, db := range m.dbs {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing %s: %w", key, err))
		}
	}
	m.stores = make(map[string]*eventsourcing.PostgresStore)
	m.dbs = make(map[string]*sql.DB)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// Global event store manager instance.
var globalEventStoreManager = NewEventStoreManager()

// EventStoreConfig holds configuration for an event_store action.
type EventStoreConfig struct {
	Connection     string            // Database DSN (required)
	Table          string            // Events table name (default: "events")
	StreamTemplate string            // Template for stream ID (required, e.g., "patient:{{.Patient.MRN}}")
	EventType      string            // Event type override (default: uses event's Type field)
	Metadata       map[string]string // Additional metadata to include
}

// parseEventStoreConfig parses event_store action configuration.
func parseEventStoreConfig(config map[string]string) (*EventStoreConfig, error) {
	esConfig := &EventStoreConfig{
		Table:    "events",
		Metadata: make(map[string]string),
	}

	for key, value := range config {
		switch key {
		case "connection", "dsn", "db":
			esConfig.Connection = value
		case "table":
			esConfig.Table = value
		case "stream", "stream_template":
			esConfig.StreamTemplate = value
		case "event_type", "type":
			esConfig.EventType = value
		default:
			// Check for metadata_ prefix
			if strings.HasPrefix(key, "metadata_") {
				metaKey := strings.TrimPrefix(key, "metadata_")
				esConfig.Metadata[metaKey] = value
			}
		}
	}

	// Validate required fields
	if esConfig.Connection == "" {
		return nil, fmt.Errorf("event_store action requires 'connection' config")
	}
	if esConfig.StreamTemplate == "" {
		return nil, fmt.Errorf("event_store action requires 'stream' or 'stream_template' config")
	}

	return esConfig, nil
}

// eventStoreAction stores events in the event store.
// Config options:
//   - connection: PostgreSQL connection string (required). Also accepts 'dsn' or 'db'.
//   - table: Events table name (default: "events")
//   - stream: Stream ID template (required, e.g., "patient:{{.Patient.MRN}}")
//   - event_type: Event type override (default: uses event's Type field)
//   - metadata_<key>: Additional metadata to include (e.g., metadata_source: "adt")
//
// The action stores the event in an append-only event store with:
//   - Automatic stream versioning (optimistic concurrency with VersionAny)
//   - Event type detection from the event's Type field or override
//   - JSON serialization of the full event payload
//   - Configurable metadata enrichment
func eventStoreAction(ctx context.Context, event interface{}, config map[string]string) error {
	// Parse configuration
	esConfig, err := parseEventStoreConfig(config)
	if err != nil {
		return err
	}

	// Get event store
	store, err := globalEventStoreManager.GetStore(esConfig.Connection, esConfig.Table)
	if err != nil {
		return fmt.Errorf("failed to get event store: %w", err)
	}

	// Render stream ID from template
	streamID := renderTemplate(esConfig.StreamTemplate, event)
	if streamID == "" {
		return fmt.Errorf("stream template rendered to empty string")
	}

	// Determine event type
	eventType := esConfig.EventType
	if eventType == "" {
		eventType = extractEventType(event)
	}
	if eventType == "" {
		eventType = "unknown"
	}

	// Render event type template if it contains template syntax
	if strings.Contains(eventType, "{{") {
		eventType = renderTemplate(eventType, event)
	}

	// Marshal event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Build metadata
	metadata := make(map[string]string)
	for key, value := range esConfig.Metadata {
		// Render metadata values as templates
		metadata[key] = renderTemplate(value, event)
	}

	// Add event metadata from the event itself if available
	if em, ok := event.(interface{ GetMetadata() map[string]string }); ok {
		for k, v := range em.GetMetadata() {
			if _, exists := metadata[k]; !exists {
				metadata[k] = v
			}
		}
	}

	// Append to event store
	eventToStore := eventsourcing.EventData{
		EventType: eventType,
		Data:      eventData,
		Metadata:  metadata,
	}

	// Use VersionAny for append-only behavior (no concurrency checks)
	_, err = store.Append(ctx, streamID, eventsourcing.VersionAny, []eventsourcing.EventData{eventToStore})
	if err != nil {
		return fmt.Errorf("failed to append event to store: %w", err)
	}

	return nil
}

// Note: extractEventType is defined in replay.go and reused here.

// RegisterEventStoreAction registers the event_store action with an engine.
// This is called automatically by NewEngine.
func RegisterEventStoreAction(e *Engine) {
	e.RegisterAction("event_store", ContextActionHandlerFunc(eventStoreAction))
}
