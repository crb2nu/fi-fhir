package requestsecurity

import (
	"sort"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// ConfiguredPrincipals lists the deployment-owned identities an authenticator
// can stamp on a caller, with the roles each carries, so startup can check the
// configured grants before any request arrives. Token-issued identities (OIDC)
// are not deployment-owned and have no equivalent. The result is a copy and
// carries no credential material.

// ConfiguredPrincipals returns the static bearer's single identity.
func (a *StaticBearerAuthenticator) ConfiguredPrincipals() []integration.Principal {
	if a == nil {
		return nil
	}
	return []integration.Principal{cloneSecurityContext(a.security).Principal}
}

// ConfiguredPrincipals returns the IDE identity followed by the service identity.
func (a *StaticBearerPairAuthenticator) ConfiguredPrincipals() []integration.Principal {
	if a == nil {
		return nil
	}
	return append(a.primary.ConfiguredPrincipals(), a.service.ConfiguredPrincipals()...)
}

// ConfiguredPrincipals returns the identity every allowlisted address receives.
func (a *TrustedNetworkAuthenticator) ConfiguredPrincipals() []integration.Principal {
	if a == nil {
		return nil
	}
	return []integration.Principal{cloneSecurityContext(a.security).Principal}
}

// ConfiguredPrincipals returns one identity per mapped email, sorted by email.
func (a *CloudflareAccessAuthenticator) ConfiguredPrincipals() []integration.Principal {
	if a == nil {
		return nil
	}
	emails := make([]string, 0, len(a.principals))
	for email := range a.principals {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	principals := make([]integration.Principal, 0, len(emails))
	for _, email := range emails {
		principals = append(principals, integration.Principal{
			ID:         email,
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: cloudflareAccessAuthMethod,
			Roles:      append([]string(nil), a.principals[email]...),
		})
	}
	return principals
}
