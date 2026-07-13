package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	integrationpreview "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
)

const (
	maxBearerTokenFileBytes = 4096
	graphqlRequestBodyLimit = 1 << 20
)

type previewRuntime struct {
	authenticator  requestsecurity.Authenticator
	allowedOrigins []string
	previewService *integrationpreview.Service
}

func loadPreviewRuntimeFromEnv() (*previewRuntime, error) {
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
	messageProcessor, err := processor.NewMessageProcessor(definitionResolver, artifactResolver)
	if err != nil {
		return nil, fmt.Errorf("configure message processor: %w", err)
	}
	previewService, err := integrationpreview.NewService(staticRegistry, messageProcessor, func() time.Time {
		return time.Now().UTC()
	})
	if err != nil {
		return nil, fmt.Errorf("configure integration preview: %w", err)
	}
	return &previewRuntime{
		authenticator:  authenticator,
		allowedOrigins: allowedOrigins,
		previewService: previewService,
	}, nil
}

func loadBearerToken() (string, error) {
	direct := os.Getenv("FI_FHIR_GRAPHQL_BEARER_TOKEN")
	path := os.Getenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE")
	if direct != "" && path != "" {
		return "", fmt.Errorf("configure exactly one GraphQL bearer token source")
	}
	if direct != "" {
		return direct, nil
	}
	if path == "" {
		return "", fmt.Errorf("FI_FHIR_GRAPHQL_BEARER_TOKEN or FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open GraphQL bearer token file: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxBearerTokenFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("read GraphQL bearer token file: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close GraphQL bearer token file: %w", closeErr)
	}
	if len(raw) == 0 || len(raw) > maxBearerTokenFileBytes {
		return "", fmt.Errorf("GraphQL bearer token file has an invalid size")
	}
	token := strings.TrimSuffix(string(raw), "\n")
	token = strings.TrimSuffix(token, "\r")
	if strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("GraphQL bearer token file contains multiple lines")
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
