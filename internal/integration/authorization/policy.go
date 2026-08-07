// Package authorization evaluates server-constructed integration actions.
package authorization

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Action identifies one operation whose target is constructed by the server.
type Action string

// ObjectKind identifies the server-owned resource under authorization.
type ObjectKind string

const (
	ActionSubmit              Action     = "integration.submit"
	ObjectIntegrationRevision ObjectKind = "integration_revision"

	// HTTPSubmitGrant is the compatibility grant assigned to authenticated HTTP sources.
	HTTPSubmitGrant = "integration:submit"
	// MLLPSubmitGrant is the compatibility grant assigned to authenticated MLLP sources.
	MLLPSubmitGrant = "integration:mllp"
	// BatchSubmitGrant is the compatibility grant assigned to authenticated batch sources.
	BatchSubmitGrant = "integration:batch"
)

var (
	ErrForbidden  = errors.New("integration operation forbidden")
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	submitGrants  = map[string]struct{}{
		HTTPSubmitGrant:  {},
		MLLPSubmitGrant:  {},
		BatchSubmitGrant: {},
	}
)

// ObjectRef is an exact, server-resolved authorization target.
type ObjectRef struct {
	TenantID            string
	Kind                ObjectKind
	IntegrationRevision integration.ArtifactRevisionRef
	SourceID            string
}

// Request combines authenticated identity with one server-selected action and object.
type Request struct {
	Security integration.SecurityContext
	Action   Action
	Object   ObjectRef
}

// Authorize denies malformed identity, tenant/source drift, and absent grants.
// It deliberately returns one catalog-safe error for every denial.
func Authorize(request Request) error {
	if request.Action != ActionSubmit || request.Object.Kind != ObjectIntegrationRevision {
		return ErrForbidden
	}
	if !validIdentity(request.Object.TenantID) || !validIdentity(request.Object.SourceID) ||
		!validRevision(request.Object.IntegrationRevision) {
		return ErrForbidden
	}
	security := request.Security
	principal := security.Principal
	if security.TenantID != request.Object.TenantID || !validIdentity(security.TenantID) ||
		principal.Kind != integration.PrincipalKindService || !validIdentity(principal.ID) ||
		!validIdentity(principal.AuthMethod) || principal.SourceID != request.Object.SourceID ||
		!validIdentity(principal.SourceID) || !hasSubmitGrant(principal.Roles) {
		return ErrForbidden
	}
	return nil
}

// AuthorizeSubmission evaluates the shared production-admission action.
func AuthorizeSubmission(
	security integration.SecurityContext,
	tenantID string,
	revision integration.ArtifactRevisionRef,
	sourceID string,
) error {
	return Authorize(Request{
		Security: security,
		Action:   ActionSubmit,
		Object: ObjectRef{
			TenantID:            tenantID,
			Kind:                ObjectIntegrationRevision,
			IntegrationRevision: revision,
			SourceID:            sourceID,
		},
	})
}

func hasSubmitGrant(roles []string) bool {
	seen := make(map[string]struct{}, len(roles))
	authorized := false
	for _, role := range roles {
		if !validIdentity(role) {
			return false
		}
		if _, duplicate := seen[role]; duplicate {
			return false
		}
		seen[role] = struct{}{}
		if _, ok := submitGrants[role]; ok {
			authorized = true
		}
	}
	return authorized
}

func validRevision(reference integration.ArtifactRevisionRef) bool {
	return validIdentity(reference.ArtifactID) && validIdentity(reference.RevisionID) &&
		digestPattern.MatchString(reference.Digest)
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
