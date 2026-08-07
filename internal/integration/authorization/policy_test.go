package authorization

import (
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestAuthorizeSubmissionAcceptsEachExistingChannelGrant(t *testing.T) {
	t.Parallel()
	for _, grant := range []string{HTTPSubmitGrant, MLLPSubmitGrant, BatchSubmitGrant} {
		grant := grant
		t.Run(grant, func(t *testing.T) {
			t.Parallel()
			if err := AuthorizeSubmission(validSecurity(grant), "tenant-a", validRevisionRef(), "adt-east"); err != nil {
				t.Fatalf("AuthorizeSubmission(%q): %v", grant, err)
			}
		})
	}
}

func TestAuthorizeSubmissionPreservesCanonicalLegacyIdentifiers(t *testing.T) {
	t.Parallel()
	security := validSecurity(MLLPSubmitGrant)
	security.TenantID = "tenant#a"
	security.Principal.ID = "source#service"
	security.Principal.SourceID = "adt#east"
	revision := validRevisionRef()
	revision.ArtifactID = "integration%adt"
	revision.RevisionID = "revision~1"
	if err := AuthorizeSubmission(security, "tenant#a", revision, "adt#east"); err != nil {
		t.Fatalf("AuthorizeSubmission() rejected an existing canonical identifier: %v", err)
	}
}

func TestAuthorizeSubmissionFailsClosed(t *testing.T) {
	t.Parallel()
	valid := validSecurity(HTTPSubmitGrant)
	tests := []struct {
		name     string
		security integration.SecurityContext
		tenant   string
		revision integration.ArtifactRevisionRef
		source   string
	}{
		{name: "missing grant", security: validSecurity("integration:read"), tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "empty roles", security: validSecurity(), tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "duplicate role", security: validSecurity(HTTPSubmitGrant, HTTPSubmitGrant), tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "noncanonical role", security: validSecurity(" integration:submit"), tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "human principal", security: integration.SecurityContext{TenantID: "tenant-a", Principal: integration.Principal{ID: "operator", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{HTTPSubmitGrant}}}, tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "missing principal", security: integration.SecurityContext{TenantID: "tenant-a"}, tenant: "tenant-a", revision: validRevisionRef(), source: "adt-east"},
		{name: "tenant mismatch", security: valid, tenant: "tenant-b", revision: validRevisionRef(), source: "adt-east"},
		{name: "source mismatch", security: valid, tenant: "tenant-a", revision: validRevisionRef(), source: "adt-west"},
		{name: "missing source", security: valid, tenant: "tenant-a", revision: validRevisionRef(), source: ""},
		{name: "invalid object id", security: valid, tenant: "tenant-a", revision: integration.ArtifactRevisionRef{ArtifactID: "bad id", RevisionID: "revision-1", Digest: validRevisionRef().Digest}, source: "adt-east"},
		{name: "invalid digest", security: valid, tenant: "tenant-a", revision: integration.ArtifactRevisionRef{ArtifactID: "integration-adt", RevisionID: "revision-1", Digest: "sha256:" + strings.Repeat("g", 64)}, source: "adt-east"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := AuthorizeSubmission(test.security, test.tenant, test.revision, test.source); !errors.Is(err, ErrForbidden) {
				t.Fatalf("AuthorizeSubmission() error = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestAuthorizeRejectsCallerSelectedActionOrObjectKind(t *testing.T) {
	t.Parallel()
	base := Request{
		Security: validSecurity(HTTPSubmitGrant),
		Action:   ActionSubmit,
		Object: ObjectRef{
			TenantID:            "tenant-a",
			Kind:                ObjectIntegrationRevision,
			IntegrationRevision: validRevisionRef(),
			SourceID:            "adt-east",
		},
	}
	wrongAction := base
	wrongAction.Action = "integration.deploy"
	if err := Authorize(wrongAction); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong action error = %v", err)
	}
	wrongKind := base
	wrongKind.Object.Kind = "destination_revision"
	if err := Authorize(wrongKind); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong object kind error = %v", err)
	}
}

func validSecurity(roles ...string) integration.SecurityContext {
	return integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID:         "source-service",
			Kind:       integration.PrincipalKindService,
			AuthMethod: "oauth2-client-credentials",
			Roles:      append([]string(nil), roles...),
			SourceID:   "adt-east",
		},
	}
}

func validRevisionRef() integration.ArtifactRevisionRef {
	return integration.ArtifactRevisionRef{
		ArtifactID: "integration-adt",
		RevisionID: "revision-1",
		Digest:     "sha256:" + strings.Repeat("a", 64),
	}
}
