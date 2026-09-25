package requestsecurity

import (
	"context"
	"crypto/subtle"
	"fmt"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// StaticBearerPairAuthenticator accepts the existing human credential and one
// separate service credential. Identities are selected, never combined.
type StaticBearerPairAuthenticator struct {
	primary *StaticBearerAuthenticator
	service *StaticBearerAuthenticator
}

// NewStaticBearerPairAuthenticator adds one service identity in the same tenant.
// Distinct credentials and principal IDs keep authentication and attribution
// unambiguous. The caller supplies the service's deployment-owned roles.
func NewStaticBearerPairAuthenticator(primary *StaticBearerAuthenticator, serviceConfig StaticBearerConfig) (*StaticBearerPairAuthenticator, error) {
	if primary == nil {
		return nil, fmt.Errorf("primary static bearer authenticator is required")
	}
	service, err := NewStaticBearerAuthenticator(serviceConfig)
	if err != nil {
		return nil, err
	}
	if service.security.TenantID != primary.security.TenantID {
		return nil, fmt.Errorf("service bearer tenant must match the deployment tenant")
	}
	if service.security.Principal.ID == primary.security.Principal.ID {
		return nil, fmt.Errorf("service bearer principal must differ from the primary principal")
	}
	if subtle.ConstantTimeCompare(primary.tokenHash[:], service.tokenHash[:]) == 1 {
		return nil, fmt.Errorf("service bearer credential must differ from the primary credential")
	}
	service.security.Principal.Kind = integration.PrincipalKindService
	service.security.Principal.AuthMethod = "service-bearer"
	return &StaticBearerPairAuthenticator{primary: primary, service: service}, nil
}

func (a *StaticBearerPairAuthenticator) Authenticate(ctx context.Context, authorization string) (integration.SecurityContext, error) {
	if a == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	// Always evaluate both constant-time token comparisons. The shared parser
	// preserves malformed-credential and cancellation behavior for both entries.
	primarySecurity, primaryErr := a.primary.Authenticate(ctx, authorization)
	serviceSecurity, serviceErr := a.service.Authenticate(ctx, authorization)
	if primaryErr == nil {
		return primarySecurity, nil
	}
	if serviceErr == nil {
		return serviceSecurity, nil
	}
	return integration.SecurityContext{}, primaryErr
}
