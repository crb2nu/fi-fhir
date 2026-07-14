package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	integrationingress "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	integrationpreview "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
)

const (
	maxBearerTokenFileBytes = 4096
	graphqlRequestBodyLimit = 1 << 20
)

type previewRuntime struct {
	authenticator    requestsecurity.Authenticator
	allowedOrigins   []string
	previewService   *integrationpreview.Service
	messageProcessor *processor.MessageProcessor
	ingressPath      string
	ingressHandler   http.Handler
	submissionDB     *sql.DB
}

func loadPreviewRuntimeFromEnv() (*previewRuntime, error) {
	return loadIntegrationRuntimeFromEnv(context.Background(), false)
}

func loadServeIntegrationRuntimeFromEnv(ctx context.Context) (*previewRuntime, error) {
	return loadIntegrationRuntimeFromEnv(ctx, true)
}

func loadIntegrationRuntimeFromEnv(ctx context.Context, allowProductionIngress bool) (*previewRuntime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("integration runtime context is required")
	}
	tenantID, err := requiredEnv("FI_FHIR_DEPLOYMENT_TENANT_ID")
	if err != nil {
		return nil, err
	}
	principalID, err := requiredEnv("FI_FHIR_GRAPHQL_PRINCIPAL_ID")
	if err != nil {
		return nil, err
	}
	rolesValue, err := requiredEnv("FI_FHIR_GRAPHQL_ROLES")
	if err != nil {
		return nil, err
	}
	roles, err := parseCSVConfig("FI_FHIR_GRAPHQL_ROLES", rolesValue)
	if err != nil {
		return nil, err
	}
	if !containsExact(roles, integrationpreview.PreviewRole) {
		return nil, fmt.Errorf("FI_FHIR_GRAPHQL_ROLES must include %q", integrationpreview.PreviewRole)
	}
	originsValue, err := requiredEnv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS")
	if err != nil {
		return nil, err
	}
	allowedOrigins, err := parseCSVConfig("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", originsValue)
	if err != nil {
		return nil, err
	}
	token, err := loadBearerToken()
	if err != nil {
		return nil, err
	}
	authenticator, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token:       token,
		TenantID:    tenantID,
		PrincipalID: principalID,
		Roles:       roles,
	})
	if err != nil {
		return nil, fmt.Errorf("configure GraphQL authenticator: %w", err)
	}

	registryPath, err := requiredEnv("FI_FHIR_INTEGRATION_REGISTRY_PATH")
	if err != nil {
		return nil, err
	}
	file, err := os.Open(registryPath)
	if err != nil {
		return nil, fmt.Errorf("open integration registry: %w", err)
	}
	staticRegistry, decodeErr := registry.DecodeStaticRegistry(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, fmt.Errorf("load integration registry: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close integration registry: %w", closeErr)
	}
	if staticRegistry.DeploymentTenantID() != tenantID {
		return nil, fmt.Errorf("integration registry tenant does not match deployment tenant")
	}
	definitionResolver, err := processor.NewDefinitionRevisionResolver(tenantID, staticRegistry)
	if err != nil {
		return nil, fmt.Errorf("configure definition resolver: %w", err)
	}
	artifactResolver, err := processor.NewRevisionResolver(tenantID, staticRegistry)
	if err != nil {
		return nil, fmt.Errorf("configure artifact resolver: %w", err)
	}

	var (
		messageProcessor *processor.MessageProcessor
		ingressHandler   http.Handler
		submissionDB     *sql.DB
	)
	ingressMode := os.Getenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE")
	if allowProductionIngress && ingressMode != "" {
		ingressAuthenticator, maxBodyBytes, err := loadHTTPIngressAuthenticatorFromEnv()
		if err != nil {
			return nil, err
		}
		submissionDB, err = openSubmissionDatabaseFromEnv(ctx)
		if err != nil {
			return nil, fmt.Errorf("configure HTTP ingress database: %w", err)
		}
		closeOnError := true
		defer func() {
			if closeOnError {
				_ = submissionDB.Close()
			}
		}()
		submissionStore, err := processor.NewPostgresSubmissionStore(submissionDB, processor.PostgresSubmissionConfig{})
		if err != nil {
			return nil, fmt.Errorf("configure durable submission store: %w", err)
		}
		if err := submissionStore.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("migrate durable submission store: %w", err)
		}
		messageProcessor, err = processor.NewDurableMessageProcessor(definitionResolver, artifactResolver, submissionStore)
		if err != nil {
			return nil, fmt.Errorf("configure durable message processor: %w", err)
		}
		ingressHandler, err = newHTTPIngressHandler(
			tenantID,
			staticRegistry,
			messageProcessor,
			ingressAuthenticator,
			maxBodyBytes,
		)
		if err != nil {
			return nil, err
		}
		closeOnError = false
	} else {
		messageProcessor, err = processor.NewMessageProcessor(definitionResolver, artifactResolver)
		if err != nil {
			return nil, fmt.Errorf("configure message processor: %w", err)
		}
	}
	previewService, err := integrationpreview.NewService(staticRegistry, messageProcessor, func() time.Time {
		return time.Now().UTC()
	})
	if err != nil {
		return nil, fmt.Errorf("configure integration preview: %w", err)
	}
	ingressPath := ""
	if ingressHandler != nil {
		ingressPath = integrationingress.Path
	}
	return &previewRuntime{
		authenticator:    authenticator,
		allowedOrigins:   allowedOrigins,
		previewService:   previewService,
		messageProcessor: messageProcessor,
		ingressPath:      ingressPath,
		ingressHandler:   ingressHandler,
		submissionDB:     submissionDB,
	}, nil
}

func (r *previewRuntime) Close() error {
	if r == nil || r.submissionDB == nil {
		return nil
	}
	err := r.submissionDB.Close()
	r.submissionDB = nil
	return err
}

func loadHTTPIngressAuthenticatorFromEnv() (*integrationingress.Authenticator, int64, error) {
	mode := integrationingress.AuthMode(os.Getenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE"))
	principalID, err := requiredEnv("FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID")
	if err != nil {
		return nil, 0, err
	}
	integrationID, err := requiredEnv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID")
	if err != nil {
		return nil, 0, err
	}
	secret, err := loadSingleLineSecret(
		"FI_FHIR_HTTP_INGRESS_SECRET",
		"FI_FHIR_HTTP_INGRESS_SECRET_FILE",
		"HTTP ingress secret",
	)
	if err != nil {
		return nil, 0, err
	}
	authenticator, err := integrationingress.NewAuthenticator(integrationingress.AuthConfig{
		Mode:          mode,
		Secret:        secret,
		PrincipalID:   principalID,
		IntegrationID: integrationID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("configure HTTP ingress authenticator: %w", err)
	}
	maxBodyBytes := integrationingress.DefaultMaxBodyBytes
	if raw := os.Getenv("FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES must be a base-10 integer")
		}
		maxBodyBytes = parsed
	}
	if maxBodyBytes <= 0 || maxBodyBytes > integrationingress.DefaultMaxBodyBytes {
		return nil, 0, fmt.Errorf("FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES must be between 1 and %d", integrationingress.DefaultMaxBodyBytes)
	}
	return authenticator, maxBodyBytes, nil
}

func newHTTPIngressHandler(
	tenantID string,
	staticRegistry *registry.StaticRegistry,
	messageProcessor *processor.MessageProcessor,
	authenticator *integrationingress.Authenticator,
	maxBodyBytes int64,
) (http.Handler, error) {
	service, err := integrationingress.NewService(integrationingress.ServiceConfig{
		TenantID:    tenantID,
		PrincipalID: authenticator.PrincipalID(),
		AuthMethod:  authenticator.AuthMethod(),
		Registry:    staticRegistry,
		Processor:   messageProcessor,
	})
	if err != nil {
		return nil, fmt.Errorf("configure HTTP ingress service: %w", err)
	}
	handler, err := integrationingress.NewHandler(integrationingress.HandlerConfig{
		MaxBodyBytes:  maxBodyBytes,
		Authenticator: authenticator,
		Service:       service,
	})
	if err != nil {
		return nil, fmt.Errorf("configure HTTP ingress handler: %w", err)
	}
	return handler, nil
}

func openSubmissionDatabaseFromEnv(ctx context.Context) (*sql.DB, error) {
	cfg := config.LoadFromEnv()
	if cfg.Database.Username == "" {
		cfg.Database.Username = os.Getenv("FI_FHIR_DATABASE_USER")
	}
	if cfg.Database.Host == "" || cfg.Database.Database == "" || cfg.Database.Username == "" {
		return nil, fmt.Errorf("FI_FHIR_DATABASE_HOST, FI_FHIR_DATABASE_NAME, and FI_FHIR_DATABASE_USERNAME are required")
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "postgres"
	}
	if cfg.Database.Driver != "postgres" {
		return nil, fmt.Errorf("durable HTTP ingress requires the postgres database driver")
	}
	dsn := cfg.DatabaseDSN()
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is empty")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if cfg.Database.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

func loadBearerToken() (string, error) {
	return loadSingleLineSecret(
		"FI_FHIR_GRAPHQL_BEARER_TOKEN",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE",
		"GraphQL bearer token",
	)
}

func loadSingleLineSecret(directName, fileName, label string) (string, error) {
	direct := os.Getenv(directName)
	path := os.Getenv(fileName)
	if direct != "" && path != "" {
		return "", fmt.Errorf("configure exactly one %s source", label)
	}
	if direct != "" {
		return direct, nil
	}
	if path == "" {
		return "", fmt.Errorf("%s or %s is required", directName, fileName)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s file: %w", label, err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxBearerTokenFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("read %s file: %w", label, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close %s file: %w", label, closeErr)
	}
	if len(raw) == 0 || len(raw) > maxBearerTokenFileBytes {
		return "", fmt.Errorf("%s file has an invalid size", label)
	}
	token := strings.TrimSuffix(string(raw), "\n")
	token = strings.TrimSuffix(token, "\r")
	if strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("%s file contains multiple lines", label)
	}
	return token, nil
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("%s is required and must be canonical", name)
	}
	return value, nil
}

func parseCSVConfig(name, value string) ([]string, error) {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return nil, fmt.Errorf("%s contains an invalid value", name)
		}
		if _, duplicate := seen[part]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate value %q", name, part)
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result, nil
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
