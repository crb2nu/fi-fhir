// Package config provides configuration management for fi-fhir.
// It supports layered configuration: defaults → config file → environment → CLI flags.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration.
type Config struct {
	// Server configuration for HTTP endpoints
	Server ServerConfig `yaml:"server" json:"server"`

	// Workflow engine configuration
	Workflow WorkflowConfig `yaml:"workflow" json:"workflow"`

	// FHIR server configuration
	FHIR FHIRConfig `yaml:"fhir" json:"fhir"`

	// Terminology configuration (version pins, DB URL, enforcement policy)
	Terminology TerminologyConfig `yaml:"terminology" json:"terminology"`

	// Database configuration
	Database DatabaseConfig `yaml:"database" json:"database"`

	// Queue configuration
	Queue QueueConfig `yaml:"queue" json:"queue"`

	// Observability configuration
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`

	// Secrets provider configuration
	Secrets SecretsConfig `yaml:"secrets" json:"secrets"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" json:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	TLSCertFile     string        `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile      string        `yaml:"tls_key_file" json:"tls_key_file"`
}

// WorkflowConfig holds workflow engine settings.
type WorkflowConfig struct {
	ConfigPath       string        `yaml:"config_path" json:"config_path"`
	DryRun           bool          `yaml:"dry_run" json:"dry_run"`
	MaxConcurrency   int           `yaml:"max_concurrency" json:"max_concurrency"`
	ActionTimeout    time.Duration `yaml:"action_timeout" json:"action_timeout"`
	RetryMaxAttempts int           `yaml:"retry_max_attempts" json:"retry_max_attempts"`
	RetryInitialWait time.Duration `yaml:"retry_initial_wait" json:"retry_initial_wait"`
	RetryMaxWait     time.Duration `yaml:"retry_max_wait" json:"retry_max_wait"`
	DLQEnabled       bool          `yaml:"dlq_enabled" json:"dlq_enabled"`
	DLQMaxSize       int           `yaml:"dlq_max_size" json:"dlq_max_size"`
}

// FHIRConfig holds FHIR server connection settings.
type FHIRConfig struct {
	BaseURL      string        `yaml:"base_url" json:"base_url"`
	Timeout      time.Duration `yaml:"timeout" json:"timeout"`
	AuthType     string        `yaml:"auth_type" json:"auth_type"` // none, basic, bearer, oauth2
	Username     string        `yaml:"username" json:"username"`
	Password     string        `yaml:"password" json:"password"` // Use secrets provider in production
	BearerToken  string        `yaml:"bearer_token" json:"bearer_token"`
	OAuth2Config OAuth2Config  `yaml:"oauth2" json:"oauth2"`
}

// OAuth2Config holds OAuth2 client credentials settings.
type OAuth2Config struct {
	TokenURL     string   `yaml:"token_url" json:"token_url"`
	ClientID     string   `yaml:"client_id" json:"client_id"`
	ClientSecret string   `yaml:"client_secret" json:"client_secret"` // Use secrets provider
	Scopes       []string `yaml:"scopes" json:"scopes"`
}

// TerminologyConfig holds terminology DB and version enforcement settings.
type TerminologyConfig struct {
	// DBURL is a PostgreSQL connection string for the terminology database.
	DBURL string `yaml:"db_url" json:"db_url"`

	// Pins maps vocabulary -> expected active version (e.g. loinc: 2.77).
	Pins map[string]string `yaml:"pins" json:"pins"`

	// Policy controls behavior when pins do not match the active DB release:
	// pass (ignore), warn (emit warnings), error (fail).
	Policy string `yaml:"policy" json:"policy"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver          string        `yaml:"driver" json:"driver"` // postgres, mysql, sqlite
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Database        string        `yaml:"database" json:"database"`
	Username        string        `yaml:"username" json:"username"`
	Password        string        `yaml:"password" json:"password"` // Use secrets provider
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
}

// QueueConfig holds message queue settings.
type QueueConfig struct {
	Driver   string            `yaml:"driver" json:"driver"` // kafka, rabbitmq, sqs
	Brokers  []string          `yaml:"brokers" json:"brokers"`
	Username string            `yaml:"username" json:"username"`
	Password string            `yaml:"password" json:"password"` // Use secrets provider
	TLS      bool              `yaml:"tls" json:"tls"`
	Options  map[string]string `yaml:"options" json:"options"`
}

// ObservabilityConfig holds metrics, tracing, and logging settings.
type ObservabilityConfig struct {
	// Metrics
	MetricsEnabled  bool   `yaml:"metrics_enabled" json:"metrics_enabled"`
	MetricsEndpoint string `yaml:"metrics_endpoint" json:"metrics_endpoint"`
	MetricsPort     int    `yaml:"metrics_port" json:"metrics_port"`

	// Tracing
	TracingEnabled  bool    `yaml:"tracing_enabled" json:"tracing_enabled"`
	TracingEndpoint string  `yaml:"tracing_endpoint" json:"tracing_endpoint"`
	TracingSampler  float64 `yaml:"tracing_sampler" json:"tracing_sampler"` // 0.0-1.0

	// Logging
	LogLevel  string `yaml:"log_level" json:"log_level"`   // debug, info, warn, error
	LogFormat string `yaml:"log_format" json:"log_format"` // json, text
}

// SecretsConfig holds secrets provider settings.
type SecretsConfig struct {
	Provider string            `yaml:"provider" json:"provider"` // env, file, vault, aws-ssm, k8s
	Options  map[string]string `yaml:"options" json:"options"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Workflow: WorkflowConfig{
			MaxConcurrency:   10,
			ActionTimeout:    30 * time.Second,
			RetryMaxAttempts: 3,
			RetryInitialWait: 1 * time.Second,
			RetryMaxWait:     30 * time.Second,
			DLQEnabled:       true,
			DLQMaxSize:       10000,
		},
		FHIR: FHIRConfig{
			Timeout:  30 * time.Second,
			AuthType: "none",
		},
		Terminology: TerminologyConfig{
			Pins:   make(map[string]string),
			Policy: "warn",
		},
		Database: DatabaseConfig{
			Driver:          "postgres",
			Port:            5432,
			SSLMode:         "require",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		Queue: QueueConfig{
			Driver:  "kafka",
			Options: make(map[string]string),
		},
		Observability: ObservabilityConfig{
			MetricsEnabled:  true,
			MetricsEndpoint: "/metrics",
			MetricsPort:     9090,
			TracingEnabled:  false,
			TracingSampler:  0.1,
			LogLevel:        "info",
			LogFormat:       "json",
		},
		Secrets: SecretsConfig{
			Provider: "env",
			Options:  make(map[string]string),
		},
	}
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from YAML bytes.
func Parse(data []byte) (*Config, error) {
	cfg := Default()

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	return cfg, nil
}

// LoadFromEnv loads configuration from environment variables.
// Environment variables use the prefix FI_FHIR_ and underscores for nesting.
// Example: FI_FHIR_SERVER_PORT=8080, FI_FHIR_DATABASE_HOST=localhost
func LoadFromEnv() *Config {
	cfg := Default()
	cfg.ApplyEnv()
	return cfg
}

// Load loads configuration with layered precedence:
// defaults → config file (if exists) → environment variables.
func Load(configPath string) (*Config, error) {
	cfg := Default()

	// Load from file if path provided and exists
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			fileCfg, err := LoadFromFile(configPath)
			if err != nil {
				return nil, err
			}
			cfg = fileCfg
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to stat config file: %w", err)
		}
	}

	// Apply environment variable overrides
	cfg.ApplyEnv()

	return cfg, nil
}

// ApplyEnv applies environment variable overrides to the config.
// Uses FI_FHIR_ prefix with uppercase names and underscores.
func (c *Config) ApplyEnv() {
	// Server
	c.Server.Host = getEnvString("FI_FHIR_SERVER_HOST", c.Server.Host)
	c.Server.Port = getEnvInt("FI_FHIR_SERVER_PORT", c.Server.Port)
	c.Server.ReadTimeout = getEnvDuration("FI_FHIR_SERVER_READ_TIMEOUT", c.Server.ReadTimeout)
	c.Server.WriteTimeout = getEnvDuration("FI_FHIR_SERVER_WRITE_TIMEOUT", c.Server.WriteTimeout)
	c.Server.ShutdownTimeout = getEnvDuration("FI_FHIR_SERVER_SHUTDOWN_TIMEOUT", c.Server.ShutdownTimeout)
	c.Server.TLSCertFile = getEnvString("FI_FHIR_SERVER_TLS_CERT_FILE", c.Server.TLSCertFile)
	c.Server.TLSKeyFile = getEnvString("FI_FHIR_SERVER_TLS_KEY_FILE", c.Server.TLSKeyFile)

	// Workflow
	c.Workflow.ConfigPath = getEnvString("FI_FHIR_WORKFLOW_CONFIG_PATH", c.Workflow.ConfigPath)
	c.Workflow.DryRun = getEnvBool("FI_FHIR_WORKFLOW_DRY_RUN", c.Workflow.DryRun)
	c.Workflow.MaxConcurrency = getEnvInt("FI_FHIR_WORKFLOW_MAX_CONCURRENCY", c.Workflow.MaxConcurrency)
	c.Workflow.ActionTimeout = getEnvDuration("FI_FHIR_WORKFLOW_ACTION_TIMEOUT", c.Workflow.ActionTimeout)
	c.Workflow.RetryMaxAttempts = getEnvInt("FI_FHIR_WORKFLOW_RETRY_MAX_ATTEMPTS", c.Workflow.RetryMaxAttempts)
	c.Workflow.RetryInitialWait = getEnvDuration("FI_FHIR_WORKFLOW_RETRY_INITIAL_WAIT", c.Workflow.RetryInitialWait)
	c.Workflow.RetryMaxWait = getEnvDuration("FI_FHIR_WORKFLOW_RETRY_MAX_WAIT", c.Workflow.RetryMaxWait)
	c.Workflow.DLQEnabled = getEnvBool("FI_FHIR_WORKFLOW_DLQ_ENABLED", c.Workflow.DLQEnabled)
	c.Workflow.DLQMaxSize = getEnvInt("FI_FHIR_WORKFLOW_DLQ_MAX_SIZE", c.Workflow.DLQMaxSize)

	// FHIR
	c.FHIR.BaseURL = getEnvString("FI_FHIR_FHIR_BASE_URL", c.FHIR.BaseURL)
	c.FHIR.Timeout = getEnvDuration("FI_FHIR_FHIR_TIMEOUT", c.FHIR.Timeout)
	c.FHIR.AuthType = getEnvString("FI_FHIR_FHIR_AUTH_TYPE", c.FHIR.AuthType)
	c.FHIR.Username = getEnvString("FI_FHIR_FHIR_USERNAME", c.FHIR.Username)
	c.FHIR.Password = getEnvString("FI_FHIR_FHIR_PASSWORD", c.FHIR.Password)
	c.FHIR.BearerToken = getEnvString("FI_FHIR_FHIR_BEARER_TOKEN", c.FHIR.BearerToken)
	c.FHIR.OAuth2Config.TokenURL = getEnvString("FI_FHIR_FHIR_OAUTH2_TOKEN_URL", c.FHIR.OAuth2Config.TokenURL)
	c.FHIR.OAuth2Config.ClientID = getEnvString("FI_FHIR_FHIR_OAUTH2_CLIENT_ID", c.FHIR.OAuth2Config.ClientID)
	c.FHIR.OAuth2Config.ClientSecret = getEnvString("FI_FHIR_FHIR_OAUTH2_CLIENT_SECRET", c.FHIR.OAuth2Config.ClientSecret)

	// Terminology
	c.Terminology.DBURL = getEnvString("FI_FHIR_TERMINOLOGY_DB_URL", c.Terminology.DBURL)
	c.Terminology.Policy = getEnvString("FI_FHIR_TERMINOLOGY_POLICY", c.Terminology.Policy)
	if pinsRaw := os.Getenv("FI_FHIR_TERMINOLOGY_PINS"); pinsRaw != "" {
		c.Terminology.Pins = parseKeyValueList(pinsRaw)
	}

	// Database
	c.Database.Driver = getEnvString("FI_FHIR_DATABASE_DRIVER", c.Database.Driver)
	c.Database.Host = getEnvString("FI_FHIR_DATABASE_HOST", c.Database.Host)
	c.Database.Port = getEnvInt("FI_FHIR_DATABASE_PORT", c.Database.Port)
	c.Database.Database = getEnvString("FI_FHIR_DATABASE_NAME", c.Database.Database)
	c.Database.Username = getEnvString("FI_FHIR_DATABASE_USERNAME", c.Database.Username)
	c.Database.Password = getEnvString("FI_FHIR_DATABASE_PASSWORD", c.Database.Password)
	c.Database.SSLMode = getEnvString("FI_FHIR_DATABASE_SSL_MODE", c.Database.SSLMode)
	c.Database.MaxOpenConns = getEnvInt("FI_FHIR_DATABASE_MAX_OPEN_CONNS", c.Database.MaxOpenConns)
	c.Database.MaxIdleConns = getEnvInt("FI_FHIR_DATABASE_MAX_IDLE_CONNS", c.Database.MaxIdleConns)
	c.Database.ConnMaxLifetime = getEnvDuration("FI_FHIR_DATABASE_CONN_MAX_LIFETIME", c.Database.ConnMaxLifetime)

	// Queue
	c.Queue.Driver = getEnvString("FI_FHIR_QUEUE_DRIVER", c.Queue.Driver)
	if brokers := os.Getenv("FI_FHIR_QUEUE_BROKERS"); brokers != "" {
		c.Queue.Brokers = strings.Split(brokers, ",")
	}
	c.Queue.Username = getEnvString("FI_FHIR_QUEUE_USERNAME", c.Queue.Username)
	c.Queue.Password = getEnvString("FI_FHIR_QUEUE_PASSWORD", c.Queue.Password)
	c.Queue.TLS = getEnvBool("FI_FHIR_QUEUE_TLS", c.Queue.TLS)

	// Observability
	c.Observability.MetricsEnabled = getEnvBool("FI_FHIR_METRICS_ENABLED", c.Observability.MetricsEnabled)
	c.Observability.MetricsEndpoint = getEnvString("FI_FHIR_METRICS_ENDPOINT", c.Observability.MetricsEndpoint)
	c.Observability.MetricsPort = getEnvInt("FI_FHIR_METRICS_PORT", c.Observability.MetricsPort)
	c.Observability.TracingEnabled = getEnvBool("FI_FHIR_TRACING_ENABLED", c.Observability.TracingEnabled)
	c.Observability.TracingEndpoint = getEnvString("FI_FHIR_TRACING_ENDPOINT", c.Observability.TracingEndpoint)
	c.Observability.TracingSampler = getEnvFloat("FI_FHIR_TRACING_SAMPLER", c.Observability.TracingSampler)
	c.Observability.LogLevel = getEnvString("FI_FHIR_LOG_LEVEL", c.Observability.LogLevel)
	c.Observability.LogFormat = getEnvString("FI_FHIR_LOG_FORMAT", c.Observability.LogFormat)

	// Secrets
	c.Secrets.Provider = getEnvString("FI_FHIR_SECRETS_PROVIDER", c.Secrets.Provider)
}

// Validate checks the configuration for errors.
func (c *Config) Validate() []error {
	var errs []error

	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port must be between 1 and 65535"))
	}
	if c.Server.ReadTimeout < 1*time.Second {
		errs = append(errs, fmt.Errorf("server.read_timeout must be at least 1 second"))
	}

	// Workflow validation
	if c.Workflow.MaxConcurrency < 1 {
		errs = append(errs, fmt.Errorf("workflow.max_concurrency must be at least 1"))
	}
	if c.Workflow.RetryMaxAttempts < 0 {
		errs = append(errs, fmt.Errorf("workflow.retry_max_attempts cannot be negative"))
	}

	// FHIR validation
	if c.FHIR.BaseURL != "" {
		if !strings.HasPrefix(c.FHIR.BaseURL, "http://") && !strings.HasPrefix(c.FHIR.BaseURL, "https://") {
			errs = append(errs, fmt.Errorf("fhir.base_url must start with http:// or https://"))
		}
	}
	validAuthTypes := map[string]bool{"none": true, "basic": true, "bearer": true, "oauth2": true}
	if !validAuthTypes[c.FHIR.AuthType] {
		errs = append(errs, fmt.Errorf("fhir.auth_type must be one of: none, basic, bearer, oauth2"))
	}

	// Terminology validation
	validPolicies := map[string]bool{"pass": true, "warn": true, "error": true, "": true}
	if !validPolicies[c.Terminology.Policy] {
		errs = append(errs, fmt.Errorf("terminology.policy must be one of: pass, warn, error"))
	}
	for vocab, ver := range c.Terminology.Pins {
		if strings.TrimSpace(vocab) == "" {
			errs = append(errs, fmt.Errorf("terminology.pins contains an empty vocabulary key"))
			continue
		}
		if strings.TrimSpace(ver) == "" {
			errs = append(errs, fmt.Errorf("terminology.pins.%s must be non-empty", strings.TrimSpace(vocab)))
		}
	}

	// Database validation
	validDrivers := map[string]bool{"postgres": true, "mysql": true, "sqlite": true}
	if c.Database.Driver != "" && !validDrivers[c.Database.Driver] {
		errs = append(errs, fmt.Errorf("database.driver must be one of: postgres, mysql, sqlite"))
	}
	if c.Database.MaxOpenConns < 1 {
		errs = append(errs, fmt.Errorf("database.max_open_conns must be at least 1"))
	}

	// Queue validation
	validQueueDrivers := map[string]bool{"kafka": true, "rabbitmq": true, "sqs": true, "": true}
	if !validQueueDrivers[c.Queue.Driver] {
		errs = append(errs, fmt.Errorf("queue.driver must be one of: kafka, rabbitmq, sqs"))
	}

	// Observability validation
	if c.Observability.TracingSampler < 0 || c.Observability.TracingSampler > 1 {
		errs = append(errs, fmt.Errorf("observability.tracing_sampler must be between 0.0 and 1.0"))
	}
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Observability.LogLevel] {
		errs = append(errs, fmt.Errorf("observability.log_level must be one of: debug, info, warn, error"))
	}
	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[c.Observability.LogFormat] {
		errs = append(errs, fmt.Errorf("observability.log_format must be one of: json, text"))
	}

	// Secrets validation
	validProviders := map[string]bool{"env": true, "file": true, "vault": true, "aws-ssm": true, "k8s": true}
	if !validProviders[c.Secrets.Provider] {
		errs = append(errs, fmt.Errorf("secrets.provider must be one of: env, file, vault, aws-ssm, k8s"))
	}

	return errs
}

// DatabaseDSN returns a database connection string.
func (c *Config) DatabaseDSN() string {
	switch c.Database.Driver {
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			c.Database.Host, c.Database.Port, c.Database.Username,
			c.Database.Password, c.Database.Database, c.Database.SSLMode)
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			c.Database.Username, c.Database.Password,
			c.Database.Host, c.Database.Port, c.Database.Database)
	case "sqlite":
		return c.Database.Database
	default:
		return ""
	}
}

// ServerAddr returns the server address in host:port format.
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// Helper functions for environment variable parsing

func getEnvString(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func parseKeyValueList(raw string) map[string]string {
	result := make(map[string]string)
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		result[k] = v
	}
	return result
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
