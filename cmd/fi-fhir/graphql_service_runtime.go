package main

import (
	"fmt"
	"os"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
)

const (
	envGraphQLServiceBearerFile  = "FI_FHIR_GRAPHQL_SERVICE_BEARER_TOKEN_FILE"
	envGraphQLServicePrincipalID = "FI_FHIR_GRAPHQL_SERVICE_PRINCIPAL_ID"
)

// loadGraphQLServiceAuthenticationFromEnv optionally adds one fixed-role
// service identity alongside the static IDE identity. The deployment owns its
// tenant and permissions; neither is configurable by a service caller.
func loadGraphQLServiceAuthenticationFromEnv(tenantID string, primary *requestsecurity.StaticBearerAuthenticator) (requestsecurity.Authenticator, error) {
	if os.Getenv(envGraphQLServiceBearerFile) == "" && os.Getenv(envGraphQLServicePrincipalID) == "" {
		return primary, nil
	}
	path, err := requiredEnv(envGraphQLServiceBearerFile)
	if err != nil {
		return nil, err
	}
	principalID, err := requiredEnv(envGraphQLServicePrincipalID)
	if err != nil {
		return nil, err
	}
	token, err := loadSingleLineSecretFile(path, "GraphQL service bearer token")
	if err != nil {
		return nil, err
	}
	authenticator, err := requestsecurity.NewStaticBearerPairAuthenticator(primary, requestsecurity.StaticBearerConfig{
		Token:       token,
		TenantID:    tenantID,
		PrincipalID: principalID,
		Roles:       []string{operator.ReadRole, preview.PreviewRole},
	})
	if err != nil {
		return nil, fmt.Errorf("configure GraphQL service authenticator: %w", err)
	}
	return authenticator, nil
}
