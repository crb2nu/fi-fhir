package ingress

import (
	"context"
	"errors"
	"net/http"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// OAuthRequestAuthenticator adapts the shared signed-access-token verifier to
// the production HTTP ingress transport.
type OAuthRequestAuthenticator struct {
	integrationID string
	authenticator requestsecurity.Authenticator
}

func NewOAuthRequestAuthenticator(integrationID string, authenticator requestsecurity.Authenticator) (*OAuthRequestAuthenticator, error) {
	if err := validateIdentity("integration ID", integrationID); err != nil {
		return nil, err
	}
	if authenticator == nil {
		return nil, ErrInvalidCredentials
	}
	return &OAuthRequestAuthenticator{integrationID: integrationID, authenticator: authenticator}, nil
}

func (a *OAuthRequestAuthenticator) IntegrationID() string {
	if a == nil {
		return ""
	}
	return a.integrationID
}

func (*OAuthRequestAuthenticator) RequiresBody() bool { return false }

func (a *OAuthRequestAuthenticator) AuthenticateRequest(ctx context.Context, request *http.Request, _ []byte) (integration.SecurityContext, error) {
	if ctx == nil || request == nil || a == nil || a.authenticator == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	authorization, found := singleHeader(request.Header, "Authorization")
	if !found || authorization == "" {
		return integration.SecurityContext{}, ErrMissingCredentials
	}
	security, err := a.authenticator.Authenticate(ctx, authorization)
	if err == nil {
		security.Principal.Roles = append([]string(nil), security.Principal.Roles...)
		return security, nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return integration.SecurityContext{}, err
	case errors.Is(err, requestsecurity.ErrMissingCredentials):
		return integration.SecurityContext{}, ErrMissingCredentials
	default:
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
}
