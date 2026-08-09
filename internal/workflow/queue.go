package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"
)

// QueuePublisher defines the interface for message queue implementations.
// Users implement this interface with their preferred queue client (Kafka, RabbitMQ, NATS, SQS).
type QueuePublisher interface {
	// Publish sends a message to the specified topic/queue.
	// Returns error if publishing fails.
	Publish(topic string, key []byte, value []byte, headers map[string]string) error

	// Close releases any resources held by the publisher.
	Close() error
}

// QueueConfig holds configuration for queue publishing.
type QueueConfig struct {
	Driver  string            // Queue driver name (kafka, rabbitmq, nats, sqs)
	Topic   string            // Topic/queue name (supports templates)
	Key     string            // Message key path (for partitioning)
	Headers map[string]string // Static headers to add to messages
	Config  map[string]string // Driver-specific configuration
}

// QueueDriverFactory creates a QueuePublisher from configuration.
type QueueDriverFactory func(config map[string]string) (QueuePublisher, error)

// QueueRegistry manages queue driver registrations.
type QueueRegistry struct {
	mu        sync.RWMutex
	drivers   map[string]QueueDriverFactory
	instances map[string]QueuePublisher // Cached publisher instances
}

// NewQueueRegistry creates a new queue registry.
func NewQueueRegistry() *QueueRegistry {
	return &QueueRegistry{
		drivers:   make(map[string]QueueDriverFactory),
		instances: make(map[string]QueuePublisher),
	}
}

// RegisterDriver registers a queue driver factory.
func (r *QueueRegistry) RegisterDriver(name string, factory QueueDriverFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[name] = factory
}

// GetPublisher returns a publisher for the given driver and config.
// Publishers are cached by driver name + config hash.
func (r *QueueRegistry) GetPublisher(driver string, config map[string]string) (QueuePublisher, error) {
	r.mu.RLock()
	factory, exists := r.drivers[driver]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("queue driver '%s' not registered; available drivers: %v", driver, r.listDrivers())
	}

	// Generate cache key from driver + config
	cacheKey := r.cacheKey(driver, config)

	r.mu.RLock()
	publisher, cached := r.instances[cacheKey]
	r.mu.RUnlock()

	if cached {
		return publisher, nil
	}

	// Create new publisher
	publisher, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s publisher: %w", driver, err)
	}

	// Cache the publisher
	r.mu.Lock()
	r.instances[cacheKey] = publisher
	r.mu.Unlock()

	return publisher, nil
}

// cacheKey generates a cache key from driver and config.
func (r *QueueRegistry) cacheKey(driver string, config map[string]string) string {
	// Simple key generation - could be improved with hash
	parts := []string{driver}
	for k, v := range config {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ":")
}

// listDrivers returns registered driver names.
func (r *QueueRegistry) listDrivers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.drivers))
	for name := range r.drivers {
		names = append(names, name)
	}
	return names
}

// Close closes all cached publishers.
func (r *QueueRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for key, publisher := range r.instances {
		if err := publisher.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing %s: %w", key, err))
		}
	}
	r.instances = make(map[string]QueuePublisher)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing publishers: %v", errs)
	}
	return nil
}

// Global queue registry instance.
var globalQueueRegistry = NewQueueRegistry()

// RegisterQueueDriver registers a queue driver globally.
func RegisterQueueDriver(name string, factory QueueDriverFactory) {
	globalQueueRegistry.RegisterDriver(name, factory)
}

// GetQueueRegistry returns the global queue registry.
func GetQueueRegistry() *QueueRegistry {
	return globalQueueRegistry
}

// queueAction publishes events to a message queue.
// Config options:
//   - driver: Queue driver name (kafka, rabbitmq, nats, sqs) - required
//   - topic: Topic/queue name, supports Go templates (required)
//   - key: Event field path for message key (optional, for partitioning)
//   - header_<name>: Static header values (optional)
//   - Additional driver-specific options passed to factory
func queueAction(event interface{}, config map[string]string) error {
	queueConfig, err := parseQueueConfig(config)
	if err != nil {
		return err
	}

	// Get or create publisher
	publisher, err := globalQueueRegistry.GetPublisher(queueConfig.Driver, queueConfig.Config)
	if err != nil {
		return fmt.Errorf("queue connection failed: %w", err)
	}

	// Convert event to map for field extraction
	eventMap, err := toMapForQueue(event)
	if err != nil {
		return fmt.Errorf("failed to convert event: %w", err)
	}

	// Render topic template
	topic, err := renderQueueTemplate(queueConfig.Topic, eventMap)
	if err != nil {
		return fmt.Errorf("failed to render topic template: %w", err)
	}

	// Extract message key if specified
	var key []byte
	if queueConfig.Key != "" {
		keyValue, err := extractQueueValue(queueConfig.Key, eventMap)
		if err == nil && keyValue != nil {
			key = []byte(fmt.Sprintf("%v", keyValue))
		}
	}

	// Serialize event to JSON
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish message
	if err := publisher.Publish(topic, key, value, queueConfig.Headers); err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	return nil
}

// parseQueueConfig parses queue configuration from action config.
func parseQueueConfig(config map[string]string) (*QueueConfig, error) {
	driver := config["driver"]
	if driver == "" {
		return nil, fmt.Errorf("queue action requires 'driver' config (kafka, rabbitmq, nats, sqs)")
	}

	topic := config["topic"]
	if topic == "" {
		return nil, fmt.Errorf("queue action requires 'topic' config")
	}

	queueConfig := &QueueConfig{
		Driver:  driver,
		Topic:   topic,
		Key:     config["key"],
		Headers: make(map[string]string),
		Config:  make(map[string]string),
	}

	// Parse headers (header_<name> = value)
	for key, value := range config {
		if strings.HasPrefix(key, "header_") {
			headerName := strings.TrimPrefix(key, "header_")
			queueConfig.Headers[headerName] = value
		}
	}

	// Pass remaining config to driver (exclude our reserved keys)
	reservedKeys := map[string]bool{
		"driver": true,
		"topic":  true,
		"key":    true,
	}
	for key, value := range config {
		if !reservedKeys[key] && !strings.HasPrefix(key, "header_") {
			queueConfig.Config[key] = value
		}
	}

	return queueConfig, nil
}

// renderQueueTemplate renders a Go template string with event data.
func renderQueueTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	// Check if template contains any placeholders
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	tmpl, err := template.New("topic").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("invalid topic template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render topic template: %w", err)
	}

	return buf.String(), nil
}

// extractQueueValue extracts a value from the event map using dot notation.
func extractQueueValue(path string, eventMap map[string]interface{}) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = eventMap

	for _, key := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("not a map at path segment")
		}
		next, exists := currentMap[key]
		if !exists {
			return nil, nil
		}
		current = next
	}

	return current, nil
}

// toMapForQueue converts an event to a map for queue processing.
func toMapForQueue(event interface{}) (map[string]interface{}, error) {
	// If already a map, return as-is
	if m, ok := event.(map[string]interface{}); ok {
		return m, nil
	}

	// Convert via JSON
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return m, nil
}

// LogQueuePublisher is a simple publisher that logs messages (for testing/debugging).
type LogQueuePublisher struct {
	name string
}

// NewLogQueuePublisher creates a new logging publisher.
func NewLogQueuePublisher(config map[string]string) (QueuePublisher, error) {
	name := config["name"]
	if name == "" {
		name = "log"
	}
	return &LogQueuePublisher{name: name}, nil
}

// redactedHeaderNames renders the header name set, sorted so the line is stable
// across runs. It never renders a header value.
func redactedHeaderNames(headers map[string]string) string {
	if len(headers) == 0 {
		return "[]"
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ",") + "]"
}

// Close is a no-op for the log publisher.
func (p *LogQueuePublisher) Close() error {
	return nil
}

func init() {
	// Register the log driver by default (useful for testing)
	RegisterQueueDriver("log", NewLogQueuePublisher)
}
