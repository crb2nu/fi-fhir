package destination

import (
	"context"
	"errors"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Non-retryable refusal codes. They are the DLQ failure codes the dispatch
// worker records, so they stay bounded, catalog-safe tokens.
const (
	// RefusalForbidden marks a dispatch whose destination identity was not
	// authorized: an unknown destination, an unbound destination under strict
	// mode, or a principal without a deliver grant.
	RefusalForbidden = "DELIVERY_FORBIDDEN"
	// RefusalUnverified marks a dispatch whose attempt reference did not match
	// the deployed destination revision byte for byte.
	RefusalUnverified = "DELIVERY_DESTINATION_UNVERIFIED"
)

var (
	// ErrAuthorizerUnavailable means the destination identity decision is not
	// configured. It is never a signal to publish.
	ErrAuthorizerUnavailable = errors.New("destination identity authorizer unavailable")
	// ErrDeliveryRefused is the catalog-safe kind every Refusal unwraps to.
	ErrDeliveryRefused = errors.New("delivery destination identity refused")
)

// RefusalError is a decision not to publish one claimed dispatch, as opposed to
// a failure to decide.
//
// Its two accessor methods are the contract the dispatch worker asserts on. They
// let the worker turn a refusal into a non-retryable dead letter without this
// package importing the worker, or the worker importing this package.
type RefusalError struct {
	Code   string
	Detail string
}

func (r *RefusalError) Error() string {
	if r == nil {
		return ErrDeliveryRefused.Error()
	}
	return r.Detail
}

// Is makes every refusal match ErrDeliveryRefused.
func (r *RefusalError) Is(target error) bool { return target == ErrDeliveryRefused }

// DeliveryRefusalCode reports the DLQ failure code for this refusal.
func (r *RefusalError) DeliveryRefusalCode() string {
	if r == nil {
		return RefusalForbidden
	}
	return r.Code
}

// DeliveryRefusalDetail reports the DLQ failure detail for this refusal.
func (r *RefusalError) DeliveryRefusalDetail() string {
	if r == nil {
		return "delivery destination identity is not authorized"
	}
	return r.Detail
}

// Decision is the server-owned provenance of one integration.deliver decision.
//
// Every field except EndpointAdvisory is produced by this process from the
// deployed revision and the verified reference. EndpointAdvisory is the
// destination's declared remote address, carried for operator diagnostics and
// never consulted by the decision.
type Decision struct {
	TenantID                  string
	AttemptID                 string
	Authorized                bool
	Mode                      Mode
	Subject                   string
	AuthMethod                string
	GrantedRole               string
	DestinationArtifactID     string
	DestinationRevisionID     string
	DestinationDigestVerified string
	DestinationClass          string
	DenialCode                string
	EndpointAdvisory          string
	DecidedAt                 time.Time
}

// ProvenanceRecorder durably records one decision. Recording happens for
// authorized and denied decisions alike, so a dead letter can be explained
// without re-deriving the decision from configuration that may since have moved.
type ProvenanceRecorder interface {
	RecordDecision(ctx context.Context, decision Decision) error
}

// AuthorizerConfig binds the deployed destination set to the identity mode.
type AuthorizerConfig struct {
	Registry             *Registry
	Recorder             ProvenanceRecorder
	CompatibilitySubject string
	Clock                func() time.Time
}

// Authorizer evaluates integration.deliver for one claimed delivery work item.
// It structurally satisfies the dispatch worker's DestinationDecider without
// importing it.
type Authorizer struct {
	registry             *Registry
	recorder             ProvenanceRecorder
	compatibilitySubject string
	clock                func() time.Time
}

// NewAuthorizer validates that the identity mode and its configuration agree.
//
// The two modes reject each other's configuration: strict refuses a
// compatibility subject, and compatibility requires one. Combined with
// LoadRegistry rejecting an unbound destination in strict mode, a deployment
// that mixes the two fails to start rather than silently choosing one.
func NewAuthorizer(config AuthorizerConfig) (*Authorizer, error) {
	if config.Registry == nil || config.Recorder == nil {
		return nil, ErrAuthorizerUnavailable
	}
	switch config.Registry.Mode() {
	case ModeStrict:
		if config.CompatibilitySubject != "" {
			return nil, errors.New(
				"FI_FHIR_DELIVERY_IDENTITY_MODE=strict rejects FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT")
		}
	case ModeCompatibility:
		if !validIdentity(config.CompatibilitySubject) {
			return nil, errors.New(
				"FI_FHIR_DELIVERY_IDENTITY_MODE=compatibility requires FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT")
		}
	default:
		return nil, ErrAuthorizerUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Authorizer{
		registry:             config.Registry,
		recorder:             config.Recorder,
		compatibilitySubject: config.CompatibilitySubject,
		clock:                clock,
	}, nil
}

// Mode reports the configured identity mode for startup reporting.
func (a *Authorizer) Mode() Mode {
	if a == nil {
		return ""
	}
	return a.registry.Mode()
}

// Decide resolves the attempt's destination against the deployed set, derives
// the principal from that resolved revision, and evaluates the shared deliver
// decision.
//
// The identity is a function of the destination revision alone. Nothing the work
// item asserts — its route, its action, or the destination reference it carries
// — can select a subject: a mismatched reference fails resolution before any
// principal exists.
//
// It returns a *Refusal for a decision and an ordinary error for an
// infrastructure failure, so a provenance outage retries rather than discarding
// work.
func (a *Authorizer) Decide(
	ctx context.Context,
	tenantID string,
	attemptID string,
	reference integration.DestinationRevisionRef,
) error {
	if a == nil || a.registry == nil || a.recorder == nil || ctx == nil {
		return ErrAuthorizerUnavailable
	}
	revision, err := a.registry.Resolve(tenantID, reference)
	if err != nil {
		return a.refuse(ctx, tenantID, attemptID, reference, resolutionRefusal(err), Revision{})
	}
	principal, grant, ok := a.principalFor(revision)
	if !ok {
		return a.refuse(ctx, tenantID, attemptID, reference, &RefusalError{
			Code:   RefusalForbidden,
			Detail: "destination declares no client identity and the deployment is strict",
		}, revision)
	}
	security := integration.SecurityContext{TenantID: tenantID, Principal: principal}
	if err := authorization.AuthorizeDelivery(
		security, tenantID, a.registry.IntegrationRevision(), revision.Reference(),
	); err != nil {
		return a.refuse(ctx, tenantID, attemptID, reference, &RefusalError{
			Code:   RefusalForbidden,
			Detail: "delivery principal is not authorized for this destination revision",
		}, revision)
	}
	return a.recorder.RecordDecision(ctx, Decision{
		TenantID:                  tenantID,
		AttemptID:                 attemptID,
		Authorized:                true,
		Mode:                      a.registry.Mode(),
		Subject:                   principal.ID,
		AuthMethod:                principal.AuthMethod,
		GrantedRole:               grant,
		DestinationArtifactID:     revision.ArtifactID,
		DestinationRevisionID:     revision.RevisionID,
		DestinationDigestVerified: revision.Digest,
		DestinationClass:          string(revision.Class),
		EndpointAdvisory:          revision.EndpointAdvisory(),
		DecidedAt:                 a.clock().UTC(),
	})
}

func (a *Authorizer) principalFor(revision Revision) (integration.Principal, string, bool) {
	if revision.IdentityBound() {
		return integration.Principal{
			ID:         revision.Identity.Subject,
			Kind:       integration.PrincipalKindService,
			AuthMethod: AuthMethodClientIdentity,
			Roles:      append([]string(nil), revision.Identity.Grants...),
		}, authorization.DestinationClientGrant, true
	}
	if a.registry.Mode() != ModeCompatibility {
		return integration.Principal{}, "", false
	}
	return integration.Principal{
		ID:         a.compatibilitySubject,
		Kind:       integration.PrincipalKindService,
		AuthMethod: AuthMethodCompatibility,
		Roles:      []string{authorization.DestinationCompatibilityGrant},
	}, authorization.DestinationCompatibilityGrant, true
}

// refuse records the refusal and returns it. A recorder failure supersedes the
// refusal so the dispatcher retries the infrastructure problem instead of
// dead-lettering an attempt whose refusal was never written down.
func (a *Authorizer) refuse(
	ctx context.Context,
	tenantID string,
	attemptID string,
	reference integration.DestinationRevisionRef,
	refusal *RefusalError,
	revision Revision,
) error {
	decision := Decision{
		TenantID:              tenantID,
		AttemptID:             attemptID,
		Authorized:            false,
		Mode:                  a.registry.Mode(),
		DestinationArtifactID: reference.ArtifactID,
		DestinationRevisionID: reference.RevisionID,
		DestinationClass:      string(reference.Class),
		DenialCode:            refusal.Code,
		DecidedAt:             a.clock().UTC(),
	}
	if revision.Digest != "" {
		decision.DestinationDigestVerified = revision.Digest
		decision.EndpointAdvisory = revision.EndpointAdvisory()
	}
	if err := a.recorder.RecordDecision(ctx, decision); err != nil {
		return err
	}
	return refusal
}

func resolutionRefusal(err error) *RefusalError {
	if errors.Is(err, ErrDestinationUnverified) {
		return &RefusalError{
			Code:   RefusalUnverified,
			Detail: "destination reference does not match the deployed destination revision",
		}
	}
	return &RefusalError{
		Code:   RefusalForbidden,
		Detail: "destination is not in the deployed integration revision",
	}
}
