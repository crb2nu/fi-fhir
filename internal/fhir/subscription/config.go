package subscription

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the subscriptions configuration file.
type Config struct {
	Subscriptions []SubscriptionDefinition `yaml:"subscriptions"`
}

// LoadConfig loads subscription configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Validate
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	names := make(map[string]bool)

	for i, sub := range c.Subscriptions {
		if sub.Name == "" {
			return fmt.Errorf("subscriptions[%d]: name is required", i)
		}

		if names[sub.Name] {
			return fmt.Errorf("subscriptions[%d]: duplicate name %q", i, sub.Name)
		}
		names[sub.Name] = true

		if sub.Server == "" {
			return fmt.Errorf("subscriptions[%d] (%s): server is required", i, sub.Name)
		}

		if sub.Criteria == "" {
			return fmt.Errorf("subscriptions[%d] (%s): criteria is required", i, sub.Name)
		}

		if sub.Channel.Endpoint == "" {
			return fmt.Errorf("subscriptions[%d] (%s): channel.endpoint is required", i, sub.Name)
		}
	}

	return nil
}

// expandEnvVars replaces ${VAR} and ${VAR:-default} patterns with environment values.
func expandEnvVars(s string) string {
	// Pattern: ${VAR} or ${VAR:-default}
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

	return re.ReplaceAllStringFunc(s, func(match string) string {
		// Parse the match
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		varName := parts[1]
		defaultVal := ""
		if len(parts) > 2 {
			defaultVal = parts[2]
		}

		// Get environment variable
		if val := os.Getenv(varName); val != "" {
			return val
		}

		return defaultVal
	})
}

// ReceiverConfig configures the notification receiver server.
type ReceiverConfig struct {
	// Enabled enables the subscription receiver
	Enabled bool `yaml:"enabled"`

	// Host is the bind address
	Host string `yaml:"host"`

	// Port is the listen port
	Port int `yaml:"port"`

	// PathPrefix is the URL path prefix for notifications
	PathPrefix string `yaml:"path_prefix"`

	// TLS configuration
	TLS TLSConfig `yaml:"tls"`

	// Security settings
	VerifySource   bool     `yaml:"verify_source"`
	AllowedSources []string `yaml:"allowed_sources"`

	// Processing settings
	MaxBundleSize int    `yaml:"max_bundle_size"`
	Timeout       string `yaml:"timeout"`

	// Retry settings for failed routing
	Retry RetryConfig `yaml:"retry"`
}

// TLSConfig configures TLS for the receiver server.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// RetryConfig configures retry behavior for failed routing.
type RetryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	MaxAttempts  int    `yaml:"max_attempts"`
	InitialDelay string `yaml:"initial_delay"`
}

// DefaultReceiverConfig returns default receiver configuration.
func DefaultReceiverConfig() *ReceiverConfig {
	return &ReceiverConfig{
		Enabled:       false,
		Host:          "0.0.0.0",
		Port:          8081,
		PathPrefix:    "/fhir/notify",
		MaxBundleSize: 100,
		Timeout:       "30s",
		Retry: RetryConfig{
			Enabled:      true,
			MaxAttempts:  3,
			InitialDelay: "1s",
		},
	}
}

// FullConfig combines subscription definitions with receiver config.
type FullConfig struct {
	Subscriptions []SubscriptionDefinition `yaml:"subscriptions"`
	Receiver      ReceiverConfig           `yaml:"subscription_receiver"`
}

// LoadFullConfig loads the complete configuration.
func LoadFullConfig(subscriptionsPath, configPath string) (*FullConfig, error) {
	config := &FullConfig{
		Receiver: *DefaultReceiverConfig(),
	}

	// Load subscriptions if path provided
	if subscriptionsPath != "" {
		subConfig, err := LoadConfig(subscriptionsPath)
		if err != nil {
			return nil, fmt.Errorf("load subscriptions: %w", err)
		}
		config.Subscriptions = subConfig.Subscriptions
	}

	// Load receiver config if path provided
	if configPath != "" {
		data, err := os.ReadFile(configPath) //nolint:gosec // G304: path from trusted config
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		expanded := expandEnvVars(string(data))

		var appConfig struct {
			SubscriptionReceiver ReceiverConfig `yaml:"subscription_receiver"`
		}

		if err := yaml.Unmarshal([]byte(expanded), &appConfig); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}

		// Merge with defaults
		mergeReceiverConfig(&config.Receiver, &appConfig.SubscriptionReceiver)
	}

	return config, nil
}

func mergeReceiverConfig(dst, src *ReceiverConfig) {
	if src.Host != "" {
		dst.Host = src.Host
	}
	if src.Port != 0 {
		dst.Port = src.Port
	}
	if src.PathPrefix != "" {
		dst.PathPrefix = src.PathPrefix
	}
	if src.MaxBundleSize != 0 {
		dst.MaxBundleSize = src.MaxBundleSize
	}
	if src.Timeout != "" {
		dst.Timeout = src.Timeout
	}
	if src.TLS.Enabled {
		dst.TLS = src.TLS
	}
	if len(src.AllowedSources) > 0 {
		dst.AllowedSources = src.AllowedSources
	}
	dst.VerifySource = src.VerifySource
	dst.Enabled = src.Enabled
}

// BuildEndpointURL constructs the callback endpoint URL for a subscription.
func BuildEndpointURL(baseURL, pathPrefix, subscriptionName string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	pathPrefix = strings.TrimPrefix(pathPrefix, "/")
	pathPrefix = strings.TrimSuffix(pathPrefix, "/")

	return fmt.Sprintf("%s/%s/%s", baseURL, pathPrefix, subscriptionName)
}
