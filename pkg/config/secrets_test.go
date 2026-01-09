package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvSecretProvider(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_SECRET", "secret_value")
	os.Setenv("MYAPP_SECRET", "prefixed_value")
	defer os.Unsetenv("TEST_SECRET")
	defer os.Unsetenv("MYAPP_SECRET")

	t.Run("no prefix", func(t *testing.T) {
		provider := NewEnvSecretProvider("")

		val, err := provider.Get("TEST_SECRET")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "secret_value" {
			t.Errorf("Expected secret_value, got %s", val)
		}

		if provider.Name() != "env" {
			t.Errorf("Expected name 'env', got %s", provider.Name())
		}
	})

	t.Run("with prefix", func(t *testing.T) {
		provider := NewEnvSecretProvider("MYAPP_")

		val, err := provider.Get("SECRET")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "prefixed_value" {
			t.Errorf("Expected prefixed_value, got %s", val)
		}

		if provider.Name() != "env:MYAPP_" {
			t.Errorf("Expected name 'env:MYAPP_', got %s", provider.Name())
		}
	})

	t.Run("not found", func(t *testing.T) {
		provider := NewEnvSecretProvider("")

		val, err := provider.Get("NONEXISTENT")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "" {
			t.Errorf("Expected empty string, got %s", val)
		}
	})

	t.Run("required not found", func(t *testing.T) {
		provider := NewEnvSecretProvider("")

		_, err := provider.GetRequired("NONEXISTENT")
		if err == nil {
			t.Error("Expected error for missing required secret")
		}
	})
}

func TestFileSecretProvider(t *testing.T) {
	// Create temp directory with secrets
	tmpDir := t.TempDir()

	// Create secret files
	os.WriteFile(filepath.Join(tmpDir, "db_password"), []byte("dbpass123\n"), 0600)
	os.WriteFile(filepath.Join(tmpDir, "api_key"), []byte("apikey456"), 0600)

	provider := NewFileSecretProvider(tmpDir)

	t.Run("read secret with newline", func(t *testing.T) {
		val, err := provider.Get("db_password")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// Should trim trailing newline
		if val != "dbpass123" {
			t.Errorf("Expected dbpass123, got %s", val)
		}
	})

	t.Run("read secret without newline", func(t *testing.T) {
		val, err := provider.Get("api_key")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "apikey456" {
			t.Errorf("Expected apikey456, got %s", val)
		}
	})

	t.Run("not found", func(t *testing.T) {
		val, err := provider.Get("nonexistent")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "" {
			t.Errorf("Expected empty string, got %s", val)
		}
	})

	t.Run("required not found", func(t *testing.T) {
		_, err := provider.GetRequired("nonexistent")
		if err == nil {
			t.Error("Expected error for missing required secret")
		}
	})

	t.Run("name", func(t *testing.T) {
		expected := "file:" + tmpDir
		if provider.Name() != expected {
			t.Errorf("Expected name %q, got %s", expected, provider.Name())
		}
	})
}

func TestMapSecretProvider(t *testing.T) {
	secrets := map[string]string{
		"username": "admin",
		"password": "secret",
	}
	provider := NewMapSecretProvider(secrets)

	t.Run("get existing", func(t *testing.T) {
		val, err := provider.Get("username")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "admin" {
			t.Errorf("Expected admin, got %s", val)
		}
	})

	t.Run("get missing", func(t *testing.T) {
		val, err := provider.Get("nonexistent")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "" {
			t.Errorf("Expected empty string, got %s", val)
		}
	})

	t.Run("required existing", func(t *testing.T) {
		val, err := provider.GetRequired("password")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "secret" {
			t.Errorf("Expected secret, got %s", val)
		}
	})

	t.Run("required missing", func(t *testing.T) {
		_, err := provider.GetRequired("nonexistent")
		if err == nil {
			t.Error("Expected error for missing required secret")
		}
	})

	t.Run("name", func(t *testing.T) {
		if provider.Name() != "map" {
			t.Errorf("Expected name 'map', got %s", provider.Name())
		}
	})
}

func TestChainSecretProvider(t *testing.T) {
	// First provider has some secrets
	first := NewMapSecretProvider(map[string]string{
		"shared":     "from_first",
		"first_only": "first_value",
	})

	// Second provider has others
	second := NewMapSecretProvider(map[string]string{
		"shared":      "from_second", // Should be overridden by first
		"second_only": "second_value",
	})

	provider := NewChainSecretProvider(first, second)

	t.Run("from first provider", func(t *testing.T) {
		val, err := provider.Get("first_only")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "first_value" {
			t.Errorf("Expected first_value, got %s", val)
		}
	})

	t.Run("from second provider", func(t *testing.T) {
		val, err := provider.Get("second_only")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "second_value" {
			t.Errorf("Expected second_value, got %s", val)
		}
	})

	t.Run("first wins for shared", func(t *testing.T) {
		val, err := provider.Get("shared")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "from_first" {
			t.Errorf("Expected from_first, got %s", val)
		}
	})

	t.Run("not found in any", func(t *testing.T) {
		val, err := provider.Get("nonexistent")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if val != "" {
			t.Errorf("Expected empty string, got %s", val)
		}
	})

	t.Run("required not found", func(t *testing.T) {
		_, err := provider.GetRequired("nonexistent")
		if err == nil {
			t.Error("Expected error for missing required secret")
		}
	})

	t.Run("name", func(t *testing.T) {
		name := provider.Name()
		if name != "chain:map,map" {
			t.Errorf("Expected name 'chain:map,map', got %s", name)
		}
	})
}

func TestSecretResolver(t *testing.T) {
	provider := NewMapSecretProvider(map[string]string{
		"DB_PASSWORD":   "secret123",
		"API_KEY":       "key456",
		"OAUTH_SECRET":  "oauth789",
	})
	resolver := NewSecretResolver(provider)

	t.Run("single reference", func(t *testing.T) {
		result, err := resolver.Resolve("password=${secret:DB_PASSWORD}")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != "password=secret123" {
			t.Errorf("Expected password=secret123, got %s", result)
		}
	})

	t.Run("multiple references", func(t *testing.T) {
		result, err := resolver.Resolve("db=${secret:DB_PASSWORD}&key=${secret:API_KEY}")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != "db=secret123&key=key456" {
			t.Errorf("Expected db=secret123&key=key456, got %s", result)
		}
	})

	t.Run("no references", func(t *testing.T) {
		result, err := resolver.Resolve("plain text value")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != "plain text value" {
			t.Errorf("Expected 'plain text value', got %s", result)
		}
	})

	t.Run("only secret reference", func(t *testing.T) {
		result, err := resolver.Resolve("${secret:DB_PASSWORD}")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != "secret123" {
			t.Errorf("Expected secret123, got %s", result)
		}
	})

	t.Run("missing secret returns empty", func(t *testing.T) {
		result, err := resolver.Resolve("${secret:NONEXISTENT}")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// Missing secret returns empty string
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	t.Run("required missing fails", func(t *testing.T) {
		_, err := resolver.ResolveRequired("${secret:NONEXISTENT}")
		if err == nil {
			t.Error("Expected error for missing required secret")
		}
	})

	t.Run("required existing succeeds", func(t *testing.T) {
		result, err := resolver.ResolveRequired("${secret:DB_PASSWORD}")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != "secret123" {
			t.Errorf("Expected secret123, got %s", result)
		}
	})
}

func TestNewSecretProvider(t *testing.T) {
	t.Run("env provider", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "env"}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if provider.Name() != "env" {
			t.Errorf("Expected env provider, got %s", provider.Name())
		}
	})

	t.Run("env provider with prefix", func(t *testing.T) {
		cfg := SecretsConfig{
			Provider: "env",
			Options:  map[string]string{"prefix": "APP_"},
		}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if provider.Name() != "env:APP_" {
			t.Errorf("Expected env:APP_ provider, got %s", provider.Name())
		}
	})

	t.Run("file provider", func(t *testing.T) {
		cfg := SecretsConfig{
			Provider: "file",
			Options:  map[string]string{"path": "/tmp/secrets"},
		}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if provider.Name() != "file:/tmp/secrets" {
			t.Errorf("Expected file:/tmp/secrets provider, got %s", provider.Name())
		}
	})

	t.Run("file provider default path", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "file"}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if provider.Name() != "file:/run/secrets" {
			t.Errorf("Expected file:/run/secrets provider, got %s", provider.Name())
		}
	})

	t.Run("k8s provider", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "k8s"}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// K8s uses file provider under the hood
		if provider.Name() != "file:/var/run/secrets/kubernetes.io/serviceaccount" {
			t.Errorf("Unexpected provider name: %s", provider.Name())
		}
	})

	t.Run("vault provider not implemented", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "vault"}
		_, err := NewSecretProvider(cfg)
		if err == nil {
			t.Error("Expected error for unimplemented vault provider")
		}
	})

	t.Run("aws-ssm provider not implemented", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "aws-ssm"}
		_, err := NewSecretProvider(cfg)
		if err == nil {
			t.Error("Expected error for unimplemented aws-ssm provider")
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		cfg := SecretsConfig{Provider: "unknown"}
		_, err := NewSecretProvider(cfg)
		if err == nil {
			t.Error("Expected error for unknown provider")
		}
	})

	t.Run("empty provider defaults to env", func(t *testing.T) {
		cfg := SecretsConfig{Provider: ""}
		provider, err := NewSecretProvider(cfg)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if provider.Name() != "env" {
			t.Errorf("Expected env provider for empty, got %s", provider.Name())
		}
	})
}

func TestConfig_ApplySecrets(t *testing.T) {
	provider := NewMapSecretProvider(map[string]string{
		"FHIR_PASSWORD":  "fhir_secret",
		"DB_PASSWORD":    "db_secret",
		"QUEUE_PASSWORD": "queue_secret",
		"OAUTH_SECRET":   "oauth_secret",
	})

	cfg := Default()
	cfg.FHIR.Password = "${secret:FHIR_PASSWORD}"
	cfg.FHIR.OAuth2Config.ClientSecret = "${secret:OAUTH_SECRET}"
	cfg.Database.Password = "${secret:DB_PASSWORD}"
	cfg.Queue.Password = "${secret:QUEUE_PASSWORD}"

	err := cfg.ApplySecrets(provider)
	if err != nil {
		t.Fatalf("ApplySecrets failed: %v", err)
	}

	if cfg.FHIR.Password != "fhir_secret" {
		t.Errorf("Expected FHIR password 'fhir_secret', got %s", cfg.FHIR.Password)
	}
	if cfg.FHIR.OAuth2Config.ClientSecret != "oauth_secret" {
		t.Errorf("Expected OAuth secret 'oauth_secret', got %s", cfg.FHIR.OAuth2Config.ClientSecret)
	}
	if cfg.Database.Password != "db_secret" {
		t.Errorf("Expected DB password 'db_secret', got %s", cfg.Database.Password)
	}
	if cfg.Queue.Password != "queue_secret" {
		t.Errorf("Expected queue password 'queue_secret', got %s", cfg.Queue.Password)
	}
}

func TestConfig_ApplySecrets_PlainValues(t *testing.T) {
	provider := NewMapSecretProvider(map[string]string{})

	cfg := Default()
	cfg.FHIR.Password = "plaintext_password"
	cfg.Database.Password = "another_plain"

	err := cfg.ApplySecrets(provider)
	if err != nil {
		t.Fatalf("ApplySecrets failed: %v", err)
	}

	// Plain values should remain unchanged
	if cfg.FHIR.Password != "plaintext_password" {
		t.Errorf("Expected plain password to remain unchanged, got %s", cfg.FHIR.Password)
	}
	if cfg.Database.Password != "another_plain" {
		t.Errorf("Expected plain password to remain unchanged, got %s", cfg.Database.Password)
	}
}

func TestLoadWithSecrets(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
server:
  port: 8080
database:
  password: "${secret:DB_PASSWORD}"
secrets:
  provider: env
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	// Set the secret in environment
	os.Setenv("DB_PASSWORD", "env_db_secret")
	defer os.Unsetenv("DB_PASSWORD")

	cfg, err := LoadWithSecrets(configPath)
	if err != nil {
		t.Fatalf("LoadWithSecrets failed: %v", err)
	}

	if cfg.Database.Password != "env_db_secret" {
		t.Errorf("Expected password 'env_db_secret', got %s", cfg.Database.Password)
	}
}
