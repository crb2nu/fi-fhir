package graphql_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

func TestIntegrationSessionSummariesWireDefaultsAndBounds(t *testing.T) {
	store := enginesession.NewMemoryStore()
	for index := 0; index < 30; index++ {
		if _, err := store.CreateSession(context.Background(), enginesession.CreateSessionRequest{Name: fmt.Sprintf("Session %02d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	config := secureServerConfig(testOperatorAuthenticator(t))
	config.MaxRequestBodyBytes = 1 << 16
	server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithIntegrationSessionStore(store)), config)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, args string
		count      int
		invalid    bool
	}{
		{"omitted input", "", 25, false},
		{"empty input defaults", "(input: {})", 25, false},
		{"explicit limit", "(input: {limit: 1})", 1, false},
		{"maximum limit", "(input: {limit: 100})", 30, false},
		{"remaining page", "(input: {offset: 25})", 5, false},
		{"zero limit", "(input: {limit: 0})", 0, true},
		{"large limit", "(input: {limit: 101})", 0, true},
		{"negative offset", "(input: {offset: -1})", 0, true},
		{"large offset", "(input: {offset: 10001})", 0, true},
		{"large search", `(input: {search: "` + strings.Repeat("PRIVATE-SEARCH-", 25) + `"})`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := `query { integrationSessionSummaries` + tc.args + ` { nodes { id name archived createdAt updatedAt latestRun { id status createdAt } } hasMore nextOffset } }`
			response := postTransportGate(t, server.Handler(), config.Path, testBearerToken, query)
			var body struct {
				Data struct {
					Page struct {
						Nodes []struct {
							ID        string          `json:"id"`
							LatestRun json.RawMessage `json:"latestRun"`
						} `json:"nodes"`
						HasMore    bool `json:"hasMore"`
						NextOffset *int `json:"nextOffset"`
					} `json:"integrationSessionSummaries"`
				} `json:"data"`
				Errors []struct{ Message string } `json:"errors"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.invalid {
				if len(body.Errors) != 1 || body.Errors[0].Message != "invalid integration session summary request" || strings.Contains(response.Body.String(), "PRIVATE-SEARCH-") {
					t.Fatalf("validation response = %s", response.Body.String())
				}
				return
			}
			if len(body.Errors) != 0 || len(body.Data.Page.Nodes) != tc.count {
				t.Fatalf("response = %s, want %d nodes", response.Body.String(), tc.count)
			}
			for _, row := range body.Data.Page.Nodes {
				if row.ID == "" || string(row.LatestRun) != "null" {
					t.Fatalf("summary without runs = %#v", row)
				}
			}
			more := tc.count == 25 || tc.count == 1
			if body.Data.Page.HasMore != more || (more && (body.Data.Page.NextOffset == nil || *body.Data.Page.NextOffset != tc.count)) || (!more && body.Data.Page.NextOffset != nil) {
				t.Fatalf("page continuation = %s", response.Body.String())
			}
		})
	}
}
