package workflow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DatabaseManager manages database connections for workflow actions.
type DatabaseManager struct {
	mu          sync.RWMutex
	connections map[string]*sql.DB
	maxLifetime time.Duration
	maxIdleTime time.Duration
	maxOpenConn int
	maxIdleConn int
}

// NewDatabaseManager creates a new database connection manager.
func NewDatabaseManager() *DatabaseManager {
	return &DatabaseManager{
		connections: make(map[string]*sql.DB),
		maxLifetime: 30 * time.Minute,
		maxIdleTime: 10 * time.Minute,
		maxOpenConn: 10,
		maxIdleConn: 5,
	}
}

// GetConnection returns a database connection for the given DSN.
// The caller must have registered the appropriate driver before calling this.
func (m *DatabaseManager) GetConnection(dsn string) (*sql.DB, error) {
	m.mu.RLock()
	db, exists := m.connections[dsn]
	m.mu.RUnlock()

	if exists {
		// Verify connection is still alive
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Connection dead, remove it
		m.mu.Lock()
		db.Close() //nolint:gosec // G104: cleanup of dead connection
		delete(m.connections, dsn)
		m.mu.Unlock()
	}

	// Create new connection
	return m.createConnection(dsn)
}

// createConnection creates a new database connection.
func (m *DatabaseManager) createConnection(dsn string) (*sql.DB, error) {
	// Detect driver from DSN
	driver := detectDriver(dsn)
	if driver == "" {
		return nil, fmt.Errorf("cannot detect database driver from DSN; supported prefixes: postgres://, mysql://, sqlite://")
	}

	// Open connection
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetConnMaxLifetime(m.maxLifetime)
	db.SetConnMaxIdleTime(m.maxIdleTime)
	db.SetMaxOpenConns(m.maxOpenConn)
	db.SetMaxIdleConns(m.maxIdleConn)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close() //nolint:gosec // G104: cleanup on failed connection
		return nil, fmt.Errorf("database ping failed (is driver registered?): %w", err)
	}

	// Cache connection
	m.mu.Lock()
	m.connections[dsn] = db
	m.mu.Unlock()

	return db, nil
}

// detectDriver detects the database driver from the DSN.
func detectDriver(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "postgres"
	case strings.HasPrefix(dsn, "mysql://"):
		return "mysql"
	case strings.HasPrefix(dsn, "sqlite://"), strings.HasPrefix(dsn, "file:"):
		return "sqlite3"
	case strings.Contains(dsn, "@tcp("):
		// MySQL DSN format: user:password@tcp(host:port)/dbname
		return "mysql"
	default:
		return ""
	}
}

// Close closes all database connections.
func (m *DatabaseManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for dsn, db := range m.connections {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing %s: %w", dsn, err))
		}
	}
	m.connections = make(map[string]*sql.DB)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// DatabaseConfig holds configuration for a database action.
type DatabaseConfig struct {
	Connection string            // DSN or connection string
	Table      string            // Target table name
	Operation  string            // "insert" or "upsert"
	Mapping    map[string]string // column -> event field path
	ConflictOn []string          // For upsert: columns that define conflict
}

// Global database manager instance.
var globalDatabaseManager = NewDatabaseManager()

// databaseAction stores events in a database.
// Config options:
//   - connection: Database connection string/DSN (required)
//   - table: Target table name (required)
//   - operation: "insert" or "upsert" (default: insert)
//   - mapping_<column>: Event field path for column (e.g., mapping_event_id: id)
//   - conflict_on: Comma-separated list of columns for upsert conflict detection
func databaseAction(event interface{}, config map[string]string) error {
	// Parse configuration
	dbConfig, err := parseDatabaseConfig(config)
	if err != nil {
		return err
	}

	// Get database connection
	db, err := globalDatabaseManager.GetConnection(dbConfig.Connection)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Convert event to map for field extraction
	eventMap, err := toMapForDB(event)
	if err != nil {
		return fmt.Errorf("failed to convert event: %w", err)
	}

	// Build and execute query
	switch dbConfig.Operation {
	case "insert":
		return executeInsert(db, dbConfig, eventMap, event)
	case "upsert":
		return executeUpsert(db, dbConfig, eventMap, event)
	default:
		return fmt.Errorf("unknown database operation: %s", dbConfig.Operation)
	}
}

// parseDatabaseConfig parses database configuration from action config.
func parseDatabaseConfig(config map[string]string) (*DatabaseConfig, error) {
	connection := config["connection"]
	if connection == "" {
		return nil, fmt.Errorf("database action requires 'connection' config")
	}

	table := config["table"]
	if table == "" {
		return nil, fmt.Errorf("database action requires 'table' config")
	}

	operation := config["operation"]
	if operation == "" {
		operation = "insert"
	}
	if operation != "insert" && operation != "upsert" {
		return nil, fmt.Errorf("database operation must be 'insert' or 'upsert', got '%s'", operation)
	}

	// Parse column mappings (mapping_<column> = <path>)
	mapping := make(map[string]string)
	for key, value := range config {
		if strings.HasPrefix(key, "mapping_") {
			column := strings.TrimPrefix(key, "mapping_")
			mapping[column] = value
		}
	}

	if len(mapping) == 0 {
		return nil, fmt.Errorf("database action requires at least one mapping (mapping_<column>)")
	}

	// Parse conflict columns for upsert
	var conflictOn []string
	if conflict := config["conflict_on"]; conflict != "" {
		for _, col := range strings.Split(conflict, ",") {
			col = strings.TrimSpace(col)
			if col != "" {
				conflictOn = append(conflictOn, col)
			}
		}
	}

	return &DatabaseConfig{
		Connection: connection,
		Table:      table,
		Operation:  operation,
		Mapping:    mapping,
		ConflictOn: conflictOn,
	}, nil
}

// executeInsert executes an INSERT statement.
func executeInsert(db *sql.DB, config *DatabaseConfig, eventMap map[string]interface{}, rawEvent interface{}) error {
	columns, values, err := buildColumnsAndValues(config.Mapping, eventMap, rawEvent)
	if err != nil {
		return err
	}

	// Build INSERT query
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1) // PostgreSQL style
	}

	query := fmt.Sprintf( //nolint:gosec // G201: table/columns from trusted config
		"INSERT INTO %s (%s) VALUES (%s)",
		config.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err = db.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}

	return nil
}

// executeUpsert executes an upsert (INSERT ... ON CONFLICT) statement.
func executeUpsert(db *sql.DB, config *DatabaseConfig, eventMap map[string]interface{}, rawEvent interface{}) error {
	if len(config.ConflictOn) == 0 {
		return fmt.Errorf("upsert requires 'conflict_on' config specifying conflict columns")
	}

	columns, values, err := buildColumnsAndValues(config.Mapping, eventMap, rawEvent)
	if err != nil {
		return err
	}

	// Build placeholders
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// Build UPDATE SET clause (exclude conflict columns)
	updateSets := make([]string, 0)
	for i, col := range columns {
		isConflict := false
		for _, conflictCol := range config.ConflictOn {
			if col == conflictCol {
				isConflict = true
				break
			}
		}
		if !isConflict {
			updateSets = append(updateSets, fmt.Sprintf("%s = $%d", col, i+1))
		}
	}

	// Build PostgreSQL-style upsert query
	query := fmt.Sprintf( //nolint:gosec // G201: table/columns from trusted config
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		config.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(config.ConflictOn, ", "),
		strings.Join(updateSets, ", "),
	)

	_, err = db.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("upsert failed: %w", err)
	}

	return nil
}

// buildColumnsAndValues builds column names and values from mapping.
func buildColumnsAndValues(mapping map[string]string, eventMap map[string]interface{}, rawEvent interface{}) ([]string, []interface{}, error) {
	// Sort columns for deterministic query generation
	columns := make([]string, 0, len(mapping))
	for col := range mapping {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	values := make([]interface{}, len(columns))
	for i, col := range columns {
		path := mapping[col]
		value, err := extractValueForDB(path, eventMap, rawEvent)
		if err != nil {
			return nil, nil, fmt.Errorf("extracting value for column %s: %w", col, err)
		}
		values[i] = value
	}

	return columns, values, nil
}

// extractValueForDB extracts a value from the event for database insertion.
func extractValueForDB(path string, eventMap map[string]interface{}, rawEvent interface{}) (interface{}, error) {
	// Special __raw__ value returns full JSON
	if path == "__raw__" {
		jsonBytes, err := json.Marshal(rawEvent)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal raw event: %w", err)
		}
		return string(jsonBytes), nil
	}

	// Extract nested value using dot notation
	return getNestedValueForDB(eventMap, path)
}

// getNestedValueForDB extracts a nested value from a map using dot notation.
func getNestedValueForDB(m map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = m

	for _, key := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, nil // Return nil for missing paths
		}
		next, exists := currentMap[key]
		if !exists {
			return nil, nil // Return nil for missing fields
		}
		current = next
	}

	return current, nil
}

// toMapForDB converts an event to a map for database field extraction.
func toMapForDB(event interface{}) (map[string]interface{}, error) {
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

// SetDatabaseManager sets the global database manager (for testing).
func SetDatabaseManager(m *DatabaseManager) {
	globalDatabaseManager = m
}

// GetDatabaseManager returns the global database manager.
func GetDatabaseManager() *DatabaseManager {
	return globalDatabaseManager
}
