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

	// ActionDeliver authorizes one durable dispatch toward one destination
	// revision. It is a distinct action from ActionSubmit because its principal
	// is a destination client, not a message source: it carries no SourceID and
	// it cannot admit a message.
	ActionDeliver Action = "integration.deliver"
	// ObjectDestinationRevision is the server-owned destination artifact the
	// deliver decision is made about.
	ObjectDestinationRevision ObjectKind = "destination_revision"

	// HTTPSubmitGrant is the compatibility grant assigned to authenticated HTTP sources.
	HTTPSubmitGrant = "integration:submit"
	// MLLPSubmitGrant is the compatibility grant assigned to authenticated MLLP sources.
	MLLPSubmitGrant = "integration:mllp"
	// BatchSubmitGrant is the compatibility grant assigned to authenticated batch sources.
	BatchSubmitGrant = "integration:batch"

	// DestinationClientGrant authorizes a destination-declared client subject to
	// dispatch toward its own destination. It follows the dotted fine-grained
	// convention already used by integration.delivery.operator (Slice 2.3) and
	// integration.operator / integration.deployment.operator (Slice 4.2a), rather
	// than the colon-form compatibility grants above.
	DestinationClientGrant = "integration.destination.client"
	// DestinationCompatibilityGrant is the explicit, server-issued grant assigned
	// to an unbound destination while a deployment runs in compatibility mode. It
	// is never carried by a destination revision and never issued in strict mode.
	DestinationCompatibilityGrant = "integration.destination.compatibility"
)

var (
	ErrForbidden  = errors.New("integration operation forbidden")
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	submitGrants  = map[string]struct{}{
		HTTPSubmitGrant:  {},
		MLLPSubmitGrant:  {},
		BatchSubmitGrant: {},
	}
	deliverGrants = map[string]struct{}{
		DestinationClientGrant:        {},
		DestinationCompatibilityGrant: {},
	}
)

// ObjectRef is an exact, server-resolved authorization target.
//
// DestinationRevision is read only by the deliver path and SourceID only by the
// submit path, so neither action's requirements can leak onto the other.
type ObjectRef struct {
	TenantID            string
	Kind                ObjectKind
	IntegrationRevision integration.ArtifactRevisionRef
	SourceID            string
	DestinationRevision integration.DestinationRevisionRef
}

// Request combines authenticated identity with one server-selected action and object.
type Request struct {
	Security integration.SecurityContext
	Action   Action
	Object   ObjectRef
}

// Authorize dispatches one server-constructed action to its own decision.
//
// Every unrecognized (action, object kind) pair is denied, so adding an action
// cannot widen an existing one. It deliberately returns one catalog-safe error
// for every denial.
func Authorize(request Request) error {
	switch {
	case request.Action == ActionSubmit && request.Object.Kind == ObjectIntegrationRevision:
		return authorizeSubmit(request)
	case request.Action == ActionDeliver && request.Object.Kind == ObjectDestinationRevision:
		return authorizeDeliver(request)
	default:
		return ErrForbidden
	}
}

// authorizeSubmit denies malformed identity, tenant/source drift, and absent
// grants. Its conditions are unchanged from the single-action policy that
// Slices 4.1b1, 4.1b2, and 4.1b3 bound their transports to.
func authorizeSubmit(request Request) error {
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

// authorizeDeliver denies malformed identity, tenant drift, an unverifiable
// destination reference, and absent deliver grants.
//
// A delivery principal must carry no SourceID. That is the isolation boundary
// between the two actions: a source principal, which always names its source,
// can never be replayed as a destination client, and a destination client can
// never be replayed as a source.
func authorizeDeliver(request Request) error {
	object := request.Object
	if !validIdentity(object.TenantID) || !validRevision(object.IntegrationRevision) ||
		!validRevision(object.DestinationRevision.ArtifactRevisionRef) ||
		!validDestinationClass(object.DestinationRevision.Class) {
		return ErrForbidden
	}
	security := request.Security
	principal := security.Principal
	if security.TenantID != object.TenantID || !validIdentity(security.TenantID) ||
		principal.Kind != integration.PrincipalKindService || !validIdentity(principal.ID) ||
		!validIdentity(principal.AuthMethod) || principal.SourceID != "" ||
		!hasDeliverGrant(principal.Roles) {
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

// AuthorizeDelivery evaluates the durable dispatch action over an exact tenant,
// integration revision, and destination revision.
//
// It is called on the dispatch path after the outbox row is claimed and before
// any broker contact, so a denial costs one claimed lease and produces one
// non-retryable dead letter rather than a published command.
func AuthorizeDelivery(
	security integration.SecurityContext,
	tenantID string,
	integrationRevision integration.ArtifactRevisionRef,
	destinationRevision integration.DestinationRevisionRef,
) error {
	return Authorize(Request{
		Security: security,
		Action:   ActionDeliver,
		Object: ObjectRef{
			TenantID:            tenantID,
			Kind:                ObjectDestinationRevision,
			IntegrationRevision: integrationRevision,
			DestinationRevision: destinationRevision,
		},
	})
}

func hasSubmitGrant(roles []string) bool {
	return hasGrant(roles, submitGrants)
}

func hasDeliverGrant(roles []string) bool {
	return hasGrant(roles, deliverGrants)
}

func hasGrant(roles []string, authorizedGrants map[string]struct{}) bool {
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
		if _, ok := authorizedGrants[role]; ok {
			authorized = true
		}
	}
	return authorized
}

func validDestinationClass(class integration.DestinationClass) bool {
	return class == integration.DestinationClassProduction ||
		class == integration.DestinationClassSandbox
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
