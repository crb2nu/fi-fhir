package resolvers

import (
	"context"
	"errors"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func callerWithRoles(roles ...string) context.Context {
	return requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: roles,
		},
	})
}

// TestSessionSampleRedactedPayloadIsGatedOnIntegrationOperator is review C1's
// resolver test: a captured sample's text reaches a caller whose verified
// roles include integration.operator, and no one else — not the legacy
// graphql:operator role that gates session reads, not an unauthenticated
// context — while a sample with no captured text reads null for everyone.
func TestSessionSampleRedactedPayloadIsGatedOnIntegrationOperator(t *testing.T) {
	resolver := NewResolver()
	text := "PID|1||REDACTED||REDACTED"
	captured := &model.SessionSample{ID: "sample_capture_c-1_1", CapturedText: &text}
	pasted := &model.SessionSample{ID: "sample_pasted"}

	got, err := resolver.SessionSample().RedactedPayload(callerWithRoles(connection.ReadRole), captured)
	if err != nil || got == nil || *got != text {
		t.Fatalf("integration.operator read %v, %v; want the captured text", got, err)
	}
	*got = "mutated"
	if *captured.CapturedText != text {
		t.Fatal("the resolver returned the model's own pointer")
	}
	for name, ctx := range map[string]context.Context{
		"graphql:operator only":            callerWithRoles(graphqlapi.GraphQLOperatorRole),
		"deployment and delivery grants":   callerWithRoles(connection.WriteRole, "integration.delivery.operator"),
		"integration.phi.export":           callerWithRoles("integration.phi.export"),
		"no verified caller":               context.Background(),
		"a role that only looks like it":   callerWithRoles("integration.operator "),
		"the colon-separated legacy spell": callerWithRoles("integration:operator"),
	} {
		if got, err := resolver.SessionSample().RedactedPayload(ctx, captured); err != nil || got != nil {
			t.Errorf("%s read %v, %v; want null", name, got, err)
		}
	}
	if got, err := resolver.SessionSample().RedactedPayload(callerWithRoles(connection.ReadRole), pasted); err != nil || got != nil {
		t.Fatalf("a sample without captured text read %v, %v", got, err)
	}
}

// TestIntakeConnectionErrorCarriesSourceUnavailable: a capture of a source the
// tap cannot see is the one intake refusal with a code.
func TestIntakeConnectionErrorCarriesSourceUnavailable(t *testing.T) {
	var presented *gqlerror.Error
	if !errors.As(intakeConnectionError(connection.ErrSourceUnavailable), &presented) {
		t.Fatal("ErrSourceUnavailable was not mapped to a GraphQL error")
	}
	if presented.Message != graphqlapi.ConnectionCaptureSourceUnavailableMessage ||
		presented.Extensions["code"] != connection.CodeSourceUnavailable || len(presented.Extensions) != 1 {
		t.Fatalf("mapped = %q %v", presented.Message, presented.Extensions)
	}
	if got := intakeConnectionError(connection.ErrPeekUnrecorded); got.Error() != "connection sample intake request failed" {
		t.Fatalf("an unrecorded peek maps to %q, want the generic failure", got)
	}
}
