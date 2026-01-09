// Package config provides secrets management for fi-fhir.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SecretProvider defines the interface for secret retrieval.
// Implementations handle different secret backends (env, file, Vault, AWS SSM, K8s).
type SecretProvider interface {
	// Get retrieves a secret by key. Returns empty string if not found.
	Get(key string) (string, error)

	// GetRequired retrieves a secret by key. Returns error if not found.
	GetRequired(key string) (string, error)

	// Name returns the provider name for logging/debugging.
	Name() string
}

// SecretResolver resolves secret references in configuration values.
// Secret references use the format ${secret:key_name}.
type SecretResolver struct {
	provider SecretProvider
	pattern  *regexp.Regexp
}

// NewSecretResolver creates a resolver with the given provider.
func NewSecretResolver(provider SecretProvider) *SecretResolver {
	return &SecretResolver{
		provider: provider,
		pattern:  regexp.MustCompile(`\$\{secret:([^}]+)\}`),
	}
}

// Resolve replaces secret references in a string with their values.
// Format: ${secret:MY_SECRET_KEY} -> actual secret value
func (r *SecretResolver) Resolve(value string) (string, error) {
	var lastErr error

	result := r.pattern.ReplaceAllStringFunc(value, func(match string) string {
		// Extract key from ${secret:KEY}
		key := r.pattern.FindStringSubmatch(match)[1]
		secret, err := r.provider.Get(key)
		if err != nil {
			lastErr = err
			return match // Keep original on error
		}
		return secret
	})

	return result, lastErr
}

// ResolveRequired replaces secret references, failing if any secret is missing.
func (r *SecretResolver) ResolveRequired(value string) (string, error) {
	matches := r.pattern.FindAllStringSubmatch(value, -1)
	result := value

	for _, match := range matches {
		key := match[1]
		secret, err := r.provider.GetRequired(key)
		if err != nil {
			return "", fmt.Errorf("failed to resolve secret %q: %w", key, err)
		}
		result = strings.Replace(result, match[0], secret, 1)
	}

	return result, nil
}

// EnvSecretProvider retrieves secrets from environment variables.
// This is the default provider suitable for development and container environments.
type EnvSecretProvider struct {
	prefix string // Optional prefix for env var names
}

// NewEnvSecretProvider creates a provider that reads from environment variables.
// If prefix is provided, it's prepended to all key lookups (e.g., prefix="APP_" -> APP_MY_SECRET).
func NewEnvSecretProvider(prefix string) *EnvSecretProvider {
	return &EnvSecretProvider{prefix: prefix}
}

func (p *EnvSecretProvider) Get(key string) (string, error) {
	envKey := p.prefix + key
	return os.Getenv(envKey), nil
}

func (p *EnvSecretProvider) GetRequired(key string) (string, error) {
	envKey := p.prefix + key
	value := os.Getenv(envKey)
	if value == "" {
		return "", fmt.Errorf("required secret %q not found in environment", envKey)
	}
	return value, nil
}

func (p *EnvSecretProvider) Name() string {
	if p.prefix != "" {
		return "env:" + p.prefix
	}
	return "env"
}

// FileSecretProvider retrieves secrets from files in a directory.
// Each secret is stored as a separate file where filename = key, content = value.
// This pattern is used by Kubernetes secrets mounted as volumes.
type FileSecretProvider struct {
	basePath string
}

// NewFileSecretProvider creates a provider that reads secrets from files.
// Each secret key maps to a file: basePath/key -> secret value
func NewFileSecretProvider(basePath string) *FileSecretProvider {
	return &FileSecretProvider{basePath: basePath}
}

func (p *FileSecretProvider) Get(key string) (string, error) {
	path := p.basePath + "/" + key
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}
	// Trim trailing newline (common in secret files)
	return strings.TrimSuffix(string(data), "\n"), nil
}

func (p *FileSecretProvider) GetRequired(key string) (string, error) {
	path := p.basePath + "/" + key
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("required secret %q not found at %s", key, path)
		}
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

func (p *FileSecretProvider) Name() string {
	return "file:" + p.basePath
}

// MapSecretProvider retrieves secrets from an in-memory map.
// Useful for testing and configuration where secrets are already loaded.
type MapSecretProvider struct {
	secrets map[string]string
}

// NewMapSecretProvider creates a provider from a map of secrets.
func NewMapSecretProvider(secrets map[string]string) *MapSecretProvider {
	return &MapSecretProvider{secrets: secrets}
}

func (p *MapSecretProvider) Get(key string) (string, error) {
	return p.secrets[key], nil
}

func (p *MapSecretProvider) GetRequired(key string) (string, error) {
	value, ok := p.secrets[key]
	if !ok {
		return "", fmt.Errorf("required secret %q not found", key)
	}
	return value, nil
}

func (p *MapSecretProvider) Name() string {
	return "map"
}

// ChainSecretProvider tries multiple providers in order until one returns a value.
// Useful for fallback chains like: Vault -> AWS SSM -> env vars
type ChainSecretProvider struct {
	providers []SecretProvider
}

// NewChainSecretProvider creates a provider that chains multiple providers.
// Providers are tried in order; first non-empty result wins.
func NewChainSecretProvider(providers ...SecretProvider) *ChainSecretProvider {
	return &ChainSecretProvider{providers: providers}
}

func (p *ChainSecretProvider) Get(key string) (string, error) {
	for _, provider := range p.providers {
		value, err := provider.Get(key)
		if err != nil {
			continue // Try next provider
		}
		if value != "" {
			return value, nil
		}
	}
	return "", nil
}

func (p *ChainSecretProvider) GetRequired(key string) (string, error) {
	for _, provider := range p.providers {
		value, err := provider.Get(key)
		if err != nil {
			continue
		}
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("required secret %q not found in any provider", key)
}

func (p *ChainSecretProvider) Name() string {
	names := make([]string, len(p.providers))
	for i, provider := range p.providers {
		names[i] = provider.Name()
	}
	return "chain:" + strings.Join(names, ",")
}

// NewSecretProvider creates a SecretProvider based on SecretsConfig.
func NewSecretProvider(cfg SecretsConfig) (SecretProvider, error) {
	switch cfg.Provider {
	case "env", "":
		prefix := cfg.Options["prefix"]
		return NewEnvSecretProvider(prefix), nil

	case "file":
		basePath := cfg.Options["path"]
		if basePath == "" {
			basePath = "/run/secrets" // Default for Docker/K8s secrets
		}
		return NewFileSecretProvider(basePath), nil

	case "vault":
		// Vault integration would require additional dependencies
		// Return a placeholder that will fail gracefully
		return nil, fmt.Errorf("vault provider not yet implemented - use VAULT_ADDR and vault CLI")

	case "aws-ssm":
		// AWS SSM integration would require AWS SDK
		return nil, fmt.Errorf("aws-ssm provider not yet implemented - use AWS_PROFILE and aws CLI")

	case "k8s":
		// Kubernetes secrets are typically mounted as files
		basePath := cfg.Options["path"]
		if basePath == "" {
			basePath = "/var/run/secrets/kubernetes.io/serviceaccount"
		}
		return NewFileSecretProvider(basePath), nil

	default:
		return nil, fmt.Errorf("unknown secrets provider: %s", cfg.Provider)
	}
}

// ApplySecrets resolves secret references in the configuration.
// Fields that may contain secrets: passwords, tokens, client secrets.
func (c *Config) ApplySecrets(provider SecretProvider) error {
	resolver := NewSecretResolver(provider)

	// Resolve FHIR secrets
	if c.FHIR.Password != "" {
		resolved, err := resolver.Resolve(c.FHIR.Password)
		if err != nil {
			return fmt.Errorf("failed to resolve FHIR password: %w", err)
		}
		c.FHIR.Password = resolved
	}
	if c.FHIR.BearerToken != "" {
		resolved, err := resolver.Resolve(c.FHIR.BearerToken)
		if err != nil {
			return fmt.Errorf("failed to resolve FHIR bearer token: %w", err)
		}
		c.FHIR.BearerToken = resolved
	}
	if c.FHIR.OAuth2Config.ClientSecret != "" {
		resolved, err := resolver.Resolve(c.FHIR.OAuth2Config.ClientSecret)
		if err != nil {
			return fmt.Errorf("failed to resolve OAuth2 client secret: %w", err)
		}
		c.FHIR.OAuth2Config.ClientSecret = resolved
	}

	// Resolve database secrets
	if c.Database.Password != "" {
		resolved, err := resolver.Resolve(c.Database.Password)
		if err != nil {
			return fmt.Errorf("failed to resolve database password: %w", err)
		}
		c.Database.Password = resolved
	}

	// Resolve queue secrets
	if c.Queue.Password != "" {
		resolved, err := resolver.Resolve(c.Queue.Password)
		if err != nil {
			return fmt.Errorf("failed to resolve queue password: %w", err)
		}
		c.Queue.Password = resolved
	}

	return nil
}

// LoadWithSecrets loads configuration and resolves secrets.
func LoadWithSecrets(configPath string) (*Config, error) {
	cfg, err := Load(configPath)
	if err != nil {
		return nil, err
	}

	provider, err := NewSecretProvider(cfg.Secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets provider: %w", err)
	}

	if err := cfg.ApplySecrets(provider); err != nil {
		return nil, fmt.Errorf("failed to apply secrets: %w", err)
	}

	return cfg, nil
}
