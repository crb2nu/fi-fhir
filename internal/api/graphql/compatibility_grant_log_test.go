package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// Slice 4.4d, task 6: the compatibility grant leaves a trace.
//
// Sprint 5's Found Defects names the gap: 115 of the 131 root fields are
// reachable only through `graphql:operator`, and nothing recorded when a
// request used it. A grant that expands to everything and leaves no evidence
// cannot be narrowed later, because nobody can tell which fields are in use.
//
// This slice adds the line and nothing else. No field moves from the grant to a
// fine-grained role, which is why the assertions below are about the log stream
// and never about admission — `TestTransportGate_CompatibilityGrantBehavesExactlyAsBefore`
// still owns that.

func newLoggedTransportGateHandler(t *testing.T) (http.Handler, string, *oidctest.Fixture, *bytes.Buffer) {
	t.Helper()
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	logs := &bytes.Buffer{}
	config := secureServerConfig(authenticator)
	config.MaxRequestBodyBytes = 1 << 16
	config.Logger = observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: "tenant-a", Output: logs,
	})
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server.Handler(), config.Path, issuer, logs
}

func grantLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("transport gate wrote a line that is not JSON: %q", line)
		}
		if entry["grant"] == graphqlapi.GraphQLOperatorRole {
			out = append(out, entry)
		}
	}
	return out
}

func TestCompatibilityGrantAdmissionEmitsExactlyOneLine(t *testing.T) {
	handler, path, issuer, logs := newLoggedTransportGateHandler(t)
	token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)

	assertGatePassed(t, postTransportGate(t, handler, path, token,
		`query Op { operatorReceipts { pageInfo { hasNextPage } } }`), "operatorReceipts")

	lines := grantLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("expected exactly one grant line, got %d: %s", len(lines), logs.String())
	}
	entry := lines[0]
	if entry["field"] != "operatorReceipts" {
		t.Errorf("grant line names the wrong field: %v", entry)
	}
	if entry["principal_id"] != "legacy-operator" {
		t.Errorf("grant line names the wrong principal: %v", entry)
	}
	if entry["component"] != "transport-gate" {
		t.Errorf("grant line is not attributed to the transport gate: %v", entry)
	}
	if entry["tenant_id"] != "tenant-a" {
		t.Errorf("grant line carries no tenant: %v", entry)
	}

	// What the line must NOT carry. The token is the credential; the role list
	// beyond the grant name is not the reader's business; nothing here may be
	// request-derived beyond a schema constant and a principal identity.
	raw := logs.String()
	if strings.Contains(raw, token) {
		t.Error("the grant line leaks the bearer token")
	}
	for _, forbidden := range []string{"roles", "authorization", "Bearer"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the grant line carries %q: %s", forbidden, raw)
		}
	}
}

// TestFineGrainedAdmissionEmitsNoGrantLine is the assertion that makes the line
// worth emitting: the volume of grant lines is the size of the migration still
// owed, so a request that did not need the grant must not appear in it.
func TestFineGrainedAdmissionEmitsNoGrantLine(t *testing.T) {
	handler, path, issuer, logs := newLoggedTransportGateHandler(t)
	token := transportGateToken(t, issuer, "least-privilege", operator.ReadRole)

	assertGatePassed(t, postTransportGate(t, handler, path, token,
		`query Op { operatorReceipts { pageInfo { hasNextPage } } }`), "operatorReceipts")

	if lines := grantLines(t, logs); len(lines) != 0 {
		t.Errorf("a fine-grained admission emitted %d grant lines: %s", len(lines), logs.String())
	}
}

// TestRefusedRequestEmitsNoGrantLine: a refusal is not a use of the grant.
func TestRefusedRequestEmitsNoGrantLine(t *testing.T) {
	handler, path, issuer, logs := newLoggedTransportGateHandler(t)
	token := transportGateToken(t, issuer, "author-only", "author")

	assertGateRefused(t, postTransportGate(t, handler, path, token,
		`query Op { operatorReceipts { pageInfo { hasNextPage } } }`), "operatorReceipts")

	if lines := grantLines(t, logs); len(lines) != 0 {
		t.Errorf("a refused request emitted %d grant lines: %s", len(lines), logs.String())
	}
}

// TestCompatibilityGrantLineIsPerRootField: a multi-field operation is a use of
// the grant for each field, because that is the unit a later narrowing works in.
// The lines are sorted so a log-volume comparison across runs is a comparison.
func TestCompatibilityGrantLineIsPerRootField(t *testing.T) {
	handler, path, issuer, logs := newLoggedTransportGateHandler(t)
	token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)

	assertGatePassed(t, postTransportGate(t, handler, path, token,
		`query Op { operatorReceipts { pageInfo { hasNextPage } } operatorCircuits { state } __typename }`),
		"operatorReceipts")

	lines := grantLines(t, logs)
	if len(lines) != 2 {
		t.Fatalf("expected one line per root field, got %d: %s", len(lines), logs.String())
	}
	if lines[0]["field"] != "operatorCircuits" || lines[1]["field"] != "operatorReceipts" {
		t.Errorf("grant lines are not sorted by field: %v, %v", lines[0]["field"], lines[1]["field"])
	}
	// `__typename` is a schema meta-field, not a product surface anyone would
	// mint a role for, so it is excluded rather than inflating the count.
	for _, entry := range lines {
		if entry["field"] == "__typename" {
			t.Error("__typename should not be reported as a grant use")
		}
	}
}

// TestGateWithoutALoggerStillAdmits: a Server constructed without a logger — an
// in-package test, a fixture — must behave identically. A nil logger is not a
// reason to change an authorization decision.
func TestGateWithoutALoggerStillAdmits(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)
	token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)
	assertGatePassed(t, postTransportGate(t, handler, path, token,
		`query Op { operatorReceipts { pageInfo { hasNextPage } } }`), "operatorReceipts")
}
