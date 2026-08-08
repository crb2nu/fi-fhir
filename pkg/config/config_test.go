package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// Verify server defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}

	// Verify workflow defaults
	if cfg.Workflow.MaxConcurrency != 10 {
		t.Errorf("Expected max concurrency 10, got %d", cfg.Workflow.MaxConcurrency)
	}
	if cfg.Workflow.RetryMaxAttempts != 3 {
		t.Errorf("Expected retry max attempts 3, got %d", cfg.Workflow.RetryMaxAttempts)
	}
	if !cfg.Workflow.DLQEnabled {
		t.Error("Expected DLQ to be enabled by default")
	}

	// Verify database defaults
	if cfg.Database.Driver != "postgres" {
		t.Errorf("Expected driver postgres, got %s", cfg.Database.Driver)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Expected port 5432, got %d", cfg.Database.Port)
	}

	// Verify observability defaults
	if !cfg.Observability.MetricsEnabled {
		t.Error("Expected metrics to be enabled by default")
	}
	if cfg.Observability.LogLevel != "info" {
		t.Errorf("Expected log level info, got %s", cfg.Observability.LogLevel)
	}

	// Verify terminology defaults
	if cfg.Terminology.Policy != "warn" {
		t.Errorf("Expected terminology policy warn, got %s", cfg.Terminology.Policy)
	}
	if cfg.Terminology.Pins == nil {
		t.Error("Expected terminology pins map to be initialized")
	}
}

func TestParse(t *testing.T) {
	yaml := `
server:
  host: localhost
  port: 9000
  read_timeout: 60s
workflow:
  config_path: /etc/workflow.yaml
  max_concurrency: 20
database:
  driver: mysql
  host: db.example.com
  port: 3306
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Server.Host != "localhost" {
		t.Errorf("Expected host localhost, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 60*time.Second {
		t.Errorf("Expected read timeout 60s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Workflow.ConfigPath != "/etc/workflow.yaml" {
		t.Errorf("Expected config path /etc/workflow.yaml, got %s", cfg.Workflow.ConfigPath)
	}
	if cfg.Workflow.MaxConcurrency != 20 {
		t.Errorf("Expected max concurrency 20, got %d", cfg.Workflow.MaxConcurrency)
	}
	if cfg.Database.Driver != "mysql" {
		t.Errorf("Expected driver mysql, got %s", cfg.Database.Driver)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Expected host db.example.com, got %s", cfg.Database.Host)
	}
}

func TestParse_Invalid(t *testing.T) {
	_, err := Parse([]byte("invalid: yaml: ["))
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
server:
  port: 8888
observability:
  log_level: debug
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if cfg.Server.Port != 8888 {
		t.Errorf("Expected port 8888, got %d", cfg.Server.Port)
	}
	if cfg.Observability.LogLevel != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Observability.LogLevel)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestApplyEnv(t *testing.T) {
	// Set environment variables
	envVars := map[string]string{
		"FI_FHIR_SERVER_PORT":              "9999",
		"FI_FHIR_SERVER_HOST":              "127.0.0.1",
		"FI_FHIR_DATABASE_HOST":            "localhost",
		"FI_FHIR_DATABASE_PASSWORD":        "secret123",
		"FI_FHIR_WORKFLOW_DRY_RUN":         "true",
		"FI_FHIR_WORKFLOW_MAX_CONCURRENCY": "50",
		"FI_FHIR_LOG_LEVEL":                "warn",
		"FI_FHIR_TRACING_SAMPLER":          "0.5",
		"FI_FHIR_QUEUE_BROKERS":            "kafka1:9092,kafka2:9092",
		"FI_FHIR_TERMINOLOGY_DB_URL":       "postgres://localhost:5432/terminology",
		"FI_FHIR_TERMINOLOGY_POLICY":       "error",
		"FI_FHIR_TERMINOLOGY_PINS":         "loinc=2.77,icd10cm=FY2024",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
		defer os.Unsetenv(k)
	}

	cfg := Default()
	cfg.ApplyEnv()

	if cfg.Server.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Expected database host localhost, got %s", cfg.Database.Host)
	}
	if cfg.Database.Password != "secret123" {
		t.Errorf("Expected database password secret123, got %s", cfg.Database.Password)
	}
	if !cfg.Workflow.DryRun {
		t.Error("Expected dry run to be true")
	}
	if cfg.Workflow.MaxConcurrency != 50 {
		t.Errorf("Expected max concurrency 50, got %d", cfg.Workflow.MaxConcurrency)
	}
	if cfg.Observability.LogLevel != "warn" {
		t.Errorf("Expected log level warn, got %s", cfg.Observability.LogLevel)
	}
	if cfg.Observability.TracingSampler != 0.5 {
		t.Errorf("Expected tracing sampler 0.5, got %f", cfg.Observability.TracingSampler)
	}
	if len(cfg.Queue.Brokers) != 2 {
		t.Errorf("Expected 2 brokers, got %d", len(cfg.Queue.Brokers))
	}

	if cfg.Terminology.DBURL != "postgres://localhost:5432/terminology" {
		t.Errorf("Expected terminology DB URL to be set, got %q", cfg.Terminology.DBURL)
	}
	if cfg.Terminology.Policy != "error" {
		t.Errorf("Expected terminology policy error, got %q", cfg.Terminology.Policy)
	}
	if cfg.Terminology.Pins["loinc"] != "2.77" {
		t.Errorf("Expected terminology pin loinc=2.77, got %q", cfg.Terminology.Pins["loinc"])
	}
	if cfg.Terminology.Pins["icd10cm"] != "FY2024" {
		t.Errorf("Expected terminology pin icd10cm=FY2024, got %q", cfg.Terminology.Pins["icd10cm"])
	}
}

func TestApplyEnvLLMCanonicalPrecedence(t *testing.T) {
	t.Setenv("FI_FHIR_LLM_BASE_URL", "https://canonical.example.com/v1")
	t.Setenv("FI_FHIR_LLM_API_KEY", "canonical-key")
	t.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", "canonical-default")
	t.Setenv("FI_FHIR_LLM_QUALITY_MODEL", "canonical-quality")
	t.Setenv("LLM_BASE_URL", "https://legacy.example.com/v1")
	t.Setenv("LLM_API_KEY", "legacy-key")
	t.Setenv("LLM_DEFAULT_MODEL", "legacy-default")
	t.Setenv("LLM_QUALITY_MODEL", "legacy-quality")
	t.Setenv("OPENAI_API_KEY", "openai-fallback")

	cfg := Default()
	cfg.ApplyEnv()

	if cfg.LLM.BaseURL != "https://canonical.example.com/v1" {
		t.Errorf("Expected canonical base URL, got %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "canonical-key" {
		t.Errorf("Expected canonical API key, got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.DefaultModel != "canonical-default" {
		t.Errorf("Expected canonical default model, got %q", cfg.LLM.DefaultModel)
	}
	if cfg.LLM.QualityModel != "canonical-quality" {
		t.Errorf("Expected canonical quality model, got %q", cfg.LLM.QualityModel)
	}
}

func TestApplyEnvLLMLegacyFallback(t *testing.T) {
	t.Setenv("FI_FHIR_LLM_BASE_URL", "")
	t.Setenv("FI_FHIR_LLM_API_KEY", "")
	t.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", "")
	t.Setenv("FI_FHIR_LLM_QUALITY_MODEL", "")
	t.Setenv("LLM_BASE_URL", "https://legacy.example.com/v1")
	t.Setenv("LLM_API_KEY", "legacy-key")
	t.Setenv("LLM_DEFAULT_MODEL", "legacy-default")
	t.Setenv("LLM_QUALITY_MODEL", "legacy-quality")

	cfg := Default()
	cfg.ApplyEnv()

	if cfg.LLM.BaseURL != "https://legacy.example.com/v1" {
		t.Errorf("Expected legacy base URL fallback, got %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "legacy-key" {
		t.Errorf("Expected legacy API key fallback, got %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.DefaultModel != "legacy-default" {
		t.Errorf("Expected legacy default model fallback, got %q", cfg.LLM.DefaultModel)
	}
	if cfg.LLM.QualityModel != "legacy-quality" {
		t.Errorf("Expected legacy quality model fallback, got %q", cfg.LLM.QualityModel)
	}
}

func TestApplyEnvAutorouteSweepInterval(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"unset keeps default", "", DefaultAutorouteSweepInterval},
		{"override", "90s", 90 * time.Second},
		{"zero disables", "0", 0},
		{"explicit zero duration disables", "0s", 0},
		{"unparseable keeps default", "not-a-duration", DefaultAutorouteSweepInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_SWEEP_INTERVAL", tt.env)

			cfg := Default()
			cfg.ApplyEnv()

			if cfg.Terminology.AutorouteSweepInterval != tt.want {
				t.Errorf("AutorouteSweepInterval = %s, want %s",
					cfg.Terminology.AutorouteSweepInterval, tt.want)
			}
		})
	}
}

func TestApplyEnvAutorouteNotify(t *testing.T) {
	t.Run("unset keeps disabled defaults", func(t *testing.T) {
		cfg := Default()
		cfg.ApplyEnv()

		notify := cfg.Terminology.AutorouteNotify
		if notify.Webhook != "" {
			t.Errorf("Webhook = %q, want empty (notifications disabled by default)", notify.Webhook)
		}
		if notify.Interval != DefaultAutorouteNotifyInterval {
			t.Errorf("Interval = %s, want %s", notify.Interval, DefaultAutorouteNotifyInterval)
		}
		if notify.MinConfidence != DefaultAutorouteNotifyMinConfidence {
			t.Errorf("MinConfidence = %v, want %v", notify.MinConfidence, DefaultAutorouteNotifyMinConfidence)
		}
		if notify.Timeout != DefaultAutorouteNotifyTimeout {
			t.Errorf("Timeout = %s, want %s", notify.Timeout, DefaultAutorouteNotifyTimeout)
		}
	})

	t.Run("env overrides", func(t *testing.T) {
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_WEBHOOK", "https://hooks.example.com/review")
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_INTERVAL", "90s")
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_MIN_CONFIDENCE", "0.75")
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_TIMEOUT", "2s")

		cfg := Default()
		cfg.ApplyEnv()

		notify := cfg.Terminology.AutorouteNotify
		if notify.Webhook != "https://hooks.example.com/review" {
			t.Errorf("Webhook = %q, want the configured URL", notify.Webhook)
		}
		if notify.Interval != 90*time.Second {
			t.Errorf("Interval = %s, want 90s", notify.Interval)
		}
		if notify.MinConfidence != 0.75 {
			t.Errorf("MinConfidence = %v, want 0.75", notify.MinConfidence)
		}
		if notify.Timeout != 2*time.Second {
			t.Errorf("Timeout = %s, want 2s", notify.Timeout)
		}
	})

	t.Run("unparseable values keep defaults", func(t *testing.T) {
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_INTERVAL", "not-a-duration")
		t.Setenv("FI_FHIR_TERMINOLOGY_AUTOROUTE_NOTIFY_MIN_CONFIDENCE", "not-a-float")

		cfg := Default()
		cfg.ApplyEnv()

		notify := cfg.Terminology.AutorouteNotify
		if notify.Interval != DefaultAutorouteNotifyInterval {
			t.Errorf("Interval = %s, want %s", notify.Interval, DefaultAutorouteNotifyInterval)
		}
		if notify.MinConfidence != DefaultAutorouteNotifyMinConfidence {
			t.Errorf("MinConfidence = %v, want %v", notify.MinConfidence, DefaultAutorouteNotifyMinConfidence)
		}
	})
}

func TestValidateAutorouteNotify(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AutorouteNotifyConfig)
		wantErr bool
	}{
		{"disabled by default", func(_ *AutorouteNotifyConfig) {}, false},
		{"valid https webhook", func(c *AutorouteNotifyConfig) {
			c.Webhook = "https://hooks.example.com/review"
		}, false},
		{"valid http webhook", func(c *AutorouteNotifyConfig) {
			c.Webhook = "http://localhost:9000/review"
		}, false},
		{"non-http scheme", func(c *AutorouteNotifyConfig) {
			c.Webhook = "ftp://hooks.example.com/review"
		}, true},
		{"zero interval with webhook", func(c *AutorouteNotifyConfig) {
			c.Webhook = "https://hooks.example.com/review"
			c.Interval = 0
		}, true},
		{"zero timeout with webhook", func(c *AutorouteNotifyConfig) {
			c.Webhook = "https://hooks.example.com/review"
			c.Timeout = 0
		}, true},
		{"confidence above range", func(c *AutorouteNotifyConfig) {
			c.Webhook = "https://hooks.example.com/review"
			c.MinConfidence = 1.5
		}, true},
		{"confidence below range", func(c *AutorouteNotifyConfig) {
			c.Webhook = "https://hooks.example.com/review"
			c.MinConfidence = -0.5
		}, true},
		// A bad interval with no webhook is inert: the feature is off.
		{"bad interval without webhook", func(c *AutorouteNotifyConfig) {
			c.Interval = 0
			c.MinConfidence = 42
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg.Terminology.AutorouteNotify)

			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
server:
  port: 7777
database:
  host: fromfile.example.com
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	// Set env var to override file
	os.Setenv("FI_FHIR_SERVER_PORT", "8888")
	defer os.Unsetenv("FI_FHIR_SERVER_PORT")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Port should be from env (8888), not file (7777)
	if cfg.Server.Port != 8888 {
		t.Errorf("Expected port 8888 (from env), got %d", cfg.Server.Port)
	}
	// Host should be from file
	if cfg.Database.Host != "fromfile.example.com" {
		t.Errorf("Expected host fromfile.example.com, got %s", cfg.Database.Host)
	}
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with no file failed: %v", err)
	}

	// Should have defaults
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Default()
	errs := cfg.Validate()

	if len(errs) > 0 {
		t.Errorf("Expected no validation errors for defaults, got: %v", errs)
	}
}

func TestValidate_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		errMsg string
	}{
		{
			name:   "invalid port low",
			modify: func(c *Config) { c.Server.Port = 0 },
			errMsg: "server.port must be between",
		},
		{
			name:   "invalid port high",
			modify: func(c *Config) { c.Server.Port = 70000 },
			errMsg: "server.port must be between",
		},
		{
			name:   "invalid read timeout",
			modify: func(c *Config) { c.Server.ReadTimeout = 100 * time.Millisecond },
			errMsg: "server.read_timeout must be at least",
		},
		{
			name:   "invalid max concurrency",
			modify: func(c *Config) { c.Workflow.MaxConcurrency = 0 },
			errMsg: "workflow.max_concurrency must be at least",
		},
		{
			name:   "invalid retry attempts",
			modify: func(c *Config) { c.Workflow.RetryMaxAttempts = -1 },
			errMsg: "workflow.retry_max_attempts cannot be negative",
		},
		{
			name:   "invalid FHIR URL",
			modify: func(c *Config) { c.FHIR.BaseURL = "ftp://invalid" },
			errMsg: "fhir.base_url must start with http",
		},
		{
			name:   "invalid auth type",
			modify: func(c *Config) { c.FHIR.AuthType = "invalid" },
			errMsg: "fhir.auth_type must be one of",
		},
		{
			name:   "invalid database driver",
			modify: func(c *Config) { c.Database.Driver = "oracle" },
			errMsg: "database.driver must be one of",
		},
		{
			name:   "invalid max open conns",
			modify: func(c *Config) { c.Database.MaxOpenConns = 0 },
			errMsg: "database.max_open_conns must be at least",
		},
		{
			name:   "invalid queue driver",
			modify: func(c *Config) { c.Queue.Driver = "redis" },
			errMsg: "queue.driver must be one of",
		},
		{
			name:   "invalid tracing sampler low",
			modify: func(c *Config) { c.Observability.TracingSampler = -0.1 },
			errMsg: "observability.tracing_sampler must be between",
		},
		{
			name:   "invalid tracing sampler high",
			modify: func(c *Config) { c.Observability.TracingSampler = 1.5 },
			errMsg: "observability.tracing_sampler must be between",
		},
		{
			name:   "invalid log level",
			modify: func(c *Config) { c.Observability.LogLevel = "verbose" },
			errMsg: "observability.log_level must be one of",
		},
		{
			name:   "invalid log format",
			modify: func(c *Config) { c.Observability.LogFormat = "xml" },
			errMsg: "observability.log_format must be one of",
		},
		{
			name:   "invalid secrets provider",
			modify: func(c *Config) { c.Secrets.Provider = "hashicorp" },
			errMsg: "secrets.provider must be one of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.modify(cfg)
			errs := cfg.Validate()

			if len(errs) == 0 {
				t.Error("Expected validation error")
				return
			}

			found := false
			for _, err := range errs {
				if contains(err.Error(), tc.errMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error containing %q, got: %v", tc.errMsg, errs)
			}
		})
	}
}

func TestDatabaseDSN(t *testing.T) {
	tests := []struct {
		driver   string
		host     string
		port     int
		database string
		username string
		password string
		sslMode  string
		expected string
	}{
		{
			driver:   "postgres",
			host:     "localhost",
			port:     5432,
			database: "mydb",
			username: "user",
			password: "pass",
			sslMode:  "require",
			expected: "host=localhost port=5432 user=user password=pass dbname=mydb sslmode=require",
		},
		{
			driver:   "mysql",
			host:     "localhost",
			port:     3306,
			database: "mydb",
			username: "user",
			password: "pass",
			expected: "user:pass@tcp(localhost:3306)/mydb",
		},
		{
			driver:   "sqlite",
			database: "/data/app.db",
			expected: "/data/app.db",
		},
		{
			driver:   "unknown",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.driver, func(t *testing.T) {
			cfg := Default()
			cfg.Database.Driver = tc.driver
			cfg.Database.Host = tc.host
			cfg.Database.Port = tc.port
			cfg.Database.Database = tc.database
			cfg.Database.Username = tc.username
			cfg.Database.Password = tc.password
			cfg.Database.SSLMode = tc.sslMode

			dsn := cfg.DatabaseDSN()
			if dsn != tc.expected {
				t.Errorf("Expected DSN %q, got %q", tc.expected, dsn)
			}
		})
	}
}

func TestServerAddr(t *testing.T) {
	cfg := Default()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8080

	addr := cfg.ServerAddr()
	if addr != "0.0.0.0:8080" {
		t.Errorf("Expected 0.0.0.0:8080, got %s", addr)
	}
}

func TestEnvHelpers(t *testing.T) {
	// Test getEnvString
	os.Setenv("TEST_STRING", "hello")
	defer os.Unsetenv("TEST_STRING")
	if v := getEnvString("TEST_STRING", "default"); v != "hello" {
		t.Errorf("Expected hello, got %s", v)
	}
	if v := getEnvString("NONEXISTENT", "default"); v != "default" {
		t.Errorf("Expected default, got %s", v)
	}

	// Test getEnvInt
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")
	if v := getEnvInt("TEST_INT", 0); v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}
	os.Setenv("TEST_INT_INVALID", "notanumber")
	defer os.Unsetenv("TEST_INT_INVALID")
	if v := getEnvInt("TEST_INT_INVALID", 10); v != 10 {
		t.Errorf("Expected default 10, got %d", v)
	}

	// Test getEnvBool
	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")
	if v := getEnvBool("TEST_BOOL", false); !v {
		t.Error("Expected true")
	}

	// Test getEnvFloat
	os.Setenv("TEST_FLOAT", "3.14")
	defer os.Unsetenv("TEST_FLOAT")
	if v := getEnvFloat("TEST_FLOAT", 0); v != 3.14 {
		t.Errorf("Expected 3.14, got %f", v)
	}

	// Test getEnvDuration
	os.Setenv("TEST_DURATION", "5m")
	defer os.Unsetenv("TEST_DURATION")
	if v := getEnvDuration("TEST_DURATION", 0); v != 5*time.Minute {
		t.Errorf("Expected 5m, got %v", v)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
