package authorization

import (
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestAuthorizeDeliveryAcceptsEachDeliverGrant(t *testing.T) {
	t.Parallel()

	for _, grant := range []string{DestinationClientGrant, DestinationCompatibilityGrant} {
		if err := AuthorizeDelivery(
			validDeliverySecurity(grant), "tenant-a", validRevisionRef(), validDestinationRef(),
		); err != nil {
			t.Fatalf("AuthorizeDelivery(%s): %v", grant, err)
		}
	}
	for _, class := range []integration.DestinationClass{
		integration.DestinationClassProduction, integration.DestinationClassSandbox,
	} {
		destination := validDestinationRef()
		destination.Class = class
		if err := AuthorizeDelivery(
			validDeliverySecurity(DestinationClientGrant), "tenant-a", validRevisionRef(), destination,
		); err != nil {
			t.Fatalf("AuthorizeDelivery(class %s): %v", class, err)
		}
	}
}

func TestAuthorizeDeliveryFailsClosed(t *testing.T) {
	t.Parallel()

	withSecurity := func(mutate func(*integration.SecurityContext)) integration.SecurityContext {
		security := validDeliverySecurity(DestinationClientGrant)
		mutate(&security)
		return security
	}
	tests := map[string]struct {
		security    integration.SecurityContext
		tenant      string
		revision    integration.ArtifactRevisionRef
		destination integration.DestinationRevisionRef
	}{
		"submit grant only": {
			security: withSecurity(func(s *integration.SecurityContext) {
				s.Principal.Roles = []string{HTTPSubmitGrant}
			}),
			tenant: "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"operator grant only": {
			security: withSecurity(func(s *integration.SecurityContext) {
				s.Principal.Roles = []string{"integration.delivery.operator"}
			}),
			tenant: "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"no grants": {
			security: withSecurity(func(s *integration.SecurityContext) { s.Principal.Roles = nil }),
			tenant:   "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"duplicate grants": {
			security: withSecurity(func(s *integration.SecurityContext) {
				s.Principal.Roles = []string{DestinationClientGrant, DestinationClientGrant}
			}),
			tenant: "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"source principal": {
			security: withSecurity(func(s *integration.SecurityContext) { s.Principal.SourceID = "adt-east" }),
			tenant:   "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"human principal": {
			security: withSecurity(func(s *integration.SecurityContext) {
				s.Principal.Kind = integration.PrincipalKindHuman
			}),
			tenant: "tenant-a", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"tenant drift": {
			security: validDeliverySecurity(DestinationClientGrant),
			tenant:   "tenant-b", revision: validRevisionRef(), destination: validDestinationRef(),
		},
		"unverifiable destination digest": {
			security: validDeliverySecurity(DestinationClientGrant),
			tenant:   "tenant-a", revision: validRevisionRef(),
			destination: func() integration.DestinationRevisionRef {
				destination := validDestinationRef()
				destination.Digest = "sha256:" + strings.Repeat("Z", 64)
				return destination
			}(),
		},
		"unknown destination class": {
			security: validDeliverySecurity(DestinationClientGrant),
			tenant:   "tenant-a", revision: validRevisionRef(),
			destination: func() integration.DestinationRevisionRef {
				destination := validDestinationRef()
				destination.Class = "internal"
				return destination
			}(),
		},
		"unverifiable integration revision": {
			security:    validDeliverySecurity(DestinationClientGrant),
			tenant:      "tenant-a",
			revision:    integration.ArtifactRevisionRef{ArtifactID: "integration-adt", RevisionID: "revision-1"},
			destination: validDestinationRef(),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := AuthorizeDelivery(
				test.security, test.tenant, test.revision, test.destination,
			); !errors.Is(err, ErrForbidden) {
				t.Fatalf("AuthorizeDelivery() error = %v, want ErrForbidden", err)
			}
		})
	}
}

// TestActionsCannotBeCrossedProvesTheTwoDecisionsStayIsolated is the guard that
// widening Authorize for integration.deliver did not weaken integration.submit
// or vice versa. Each identity is legal for exactly one action.
func TestActionsCannotBeCrossedProvesTheTwoDecisionsStayIsolated(t *testing.T) {
	t.Parallel()

	submitSecurity := validSecurity(HTTPSubmitGrant)
	deliverSecurity := validDeliverySecurity(DestinationClientGrant)

	// A submit-granted source principal cannot deliver.
	if err := AuthorizeDelivery(
		submitSecurity, "tenant-a", validRevisionRef(), validDestinationRef(),
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("submit principal authorized a delivery: %v", err)
	}
	// A deliver-granted destination client cannot submit.
	if err := AuthorizeSubmission(
		deliverSecurity, "tenant-a", validRevisionRef(), "adt-east",
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("delivery principal authorized a submission: %v", err)
	}
	// Mixed (action, object kind) pairs are denied in both directions.
	crossings := []Request{
		{Security: submitSecurity, Action: ActionSubmit, Object: ObjectRef{
			TenantID: "tenant-a", Kind: ObjectDestinationRevision,
			IntegrationRevision: validRevisionRef(), SourceID: "adt-east",
		}},
		{Security: deliverSecurity, Action: ActionDeliver, Object: ObjectRef{
			TenantID: "tenant-a", Kind: ObjectIntegrationRevision,
			IntegrationRevision: validRevisionRef(), DestinationRevision: validDestinationRef(),
		}},
		{Security: deliverSecurity, Action: "integration.export", Object: ObjectRef{
			TenantID: "tenant-a", Kind: ObjectDestinationRevision,
			IntegrationRevision: validRevisionRef(), DestinationRevision: validDestinationRef(),
		}},
	}
	for index, request := range crossings {
		if err := Authorize(request); !errors.Is(err, ErrForbidden) {
			t.Fatalf("crossing %d authorized: %v", index, err)
		}
	}
}

// TestSubmitPathIgnoresDestinationFields proves the new ObjectRef field is inert
// on the submit path, so every existing caller keeps its exact behavior.
func TestSubmitPathIgnoresDestinationFields(t *testing.T) {
	t.Parallel()

	request := Request{
		Security: validSecurity(HTTPSubmitGrant),
		Action:   ActionSubmit,
		Object: ObjectRef{
			TenantID: "tenant-a", Kind: ObjectIntegrationRevision,
			IntegrationRevision: validRevisionRef(), SourceID: "adt-east",
		},
	}
	if err := Authorize(request); err != nil {
		t.Fatalf("submit without a destination: %v", err)
	}
	withDestination := request
	withDestination.Object.DestinationRevision = validDestinationRef()
	if err := Authorize(withDestination); err != nil {
		t.Fatalf("submit with an inert destination field: %v", err)
	}
	garbage := request
	garbage.Object.DestinationRevision = integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: " bad", Digest: "nope"},
		Class:               "internal",
	}
	if err := Authorize(garbage); err != nil {
		t.Fatalf("submit is sensitive to destination fields it must not read: %v", err)
	}
}

func validDeliverySecurity(roles ...string) integration.SecurityContext {
	return integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID:         "alpha-client",
			Kind:       integration.PrincipalKindService,
			AuthMethod: "destination-client-identity",
			Roles:      append([]string(nil), roles...),
		},
	}
}

func validDestinationRef() integration.DestinationRevisionRef {
	return integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{
			ArtifactID: "dest-alpha",
			RevisionID: "destination-1",
			Digest:     "sha256:" + strings.Repeat("d", 64),
		},
		Class: integration.DestinationClassProduction,
	}
}
