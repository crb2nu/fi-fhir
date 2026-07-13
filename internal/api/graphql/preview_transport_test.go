package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	integrationpreview "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const previewMutation = `mutation PreviewIntegrationMessage($input: PreviewIntegrationMessageInput!) {
  previewIntegrationMessage(input: $input) {
    mode
    tenantId
    integrationRevision { artifactId revisionId digest }
    artifactRevisions {
      source { artifactId revisionId digest }
      profile { artifactId revisionId digest }
      workflow { artifactId revisionId digest }
    }
    events { tenantId id type sourceMessageId correlationId classification payload }
    diagnostics { tenantId severity stage code message path source classification }
    routes { tenantId eventId route matched skipped skipReason transformCount plannedActions diagnosticCodes }
    deliveries {
      tenantId eventId route action status diagnosticCodes
      destination { artifactId revisionId digest class }
    }
    correlations { tenantId correlationId traceId sourceMessageId eventIds workflowRunId }
  }
}`

func TestAuthenticatedPreviewTransportMatchesKernelAndContainsLegacyExecution(t *testing.T) {
	now := time.Date(2026, 7, 13, 17, 0, 0, 0, time.UTC)
	staticRegistry := loadPreviewTransportRegistry(t)
	messageProcessor := newPreviewTransportProcessor(t, staticRegistry)
	service, err := integrationpreview.NewService(staticRegistry, messageProcessor, func() time.Time { return now })
	if err != nil {
		t.Fatalf("preview.NewService: %v", err)
	}
	authenticator := testAuthenticator(t)
	config := secureServerConfig(authenticator)
	config.MaxRequestBodyBytes = 16 << 10
	server, err := graphqlapi.NewServer(
		resolvers.NewResolver(resolvers.WithPreviewService(service)),
		config,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	payload := previewTransportA01()
	input := integrationpreview.Input{
		IntegrationID: "adt-east",
		Payload:       []byte(payload),
		CorrelationID: "transport-correlation-1",
		Reason:        "verify authenticated transport parity",
	}
	security := integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID:         "engineer-1",
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: "bearer",
			Roles:      []string{integrationpreview.PreviewRole},
		},
	}
	direct, err := service.Preview(context.Background(), security, input)
	if err != nil {
		t.Fatalf("direct preview service: %v", err)
	}

	body := graphQLBody(t, previewMutation, map[string]any{"input": map[string]any{
		"integrationId": input.IntegrationID,
		"data":          string(input.Payload),
		"correlationId": input.CorrelationID,
		"reason":        input.Reason,
	}})
	recorder := postPreviewGraphQL(t, server.Handler(), body, "Bearer "+testBearerToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data struct {
			Preview struct {
				Mode                string `json:"mode"`
				TenantID            string `json:"tenantId"`
				IntegrationRevision struct {
					ArtifactID string `json:"artifactId"`
					RevisionID string `json:"revisionId"`
					Digest     string `json:"digest"`
				} `json:"integrationRevision"`
				ArtifactRevisions struct {
					Profile struct {
						RevisionID string `json:"revisionId"`
						Digest     string `json:"digest"`
					} `json:"profile"`
					Workflow struct {
						RevisionID string `json:"revisionId"`
						Digest     string `json:"digest"`
					} `json:"workflow"`
				} `json:"artifactRevisions"`
				Events     []json.RawMessage `json:"events"`
				Deliveries []struct {
					Status string `json:"status"`
				} `json:"deliveries"`
				Correlations struct {
					CorrelationID string   `json:"correlationId"`
					EventIDs      []string `json:"eventIds"`
				} `json:"correlations"`
			} `json:"previewIntegrationMessage"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Errors) != 0 {
		t.Fatalf("GraphQL errors = %#v", response.Errors)
	}
	got := response.Data.Preview
	if got.Mode != string(direct.Mode) || got.TenantID != direct.TenantID {
		t.Fatalf("transport mode/tenant = %q/%q, direct = %q/%q", got.Mode, got.TenantID, direct.Mode, direct.TenantID)
	}
	if got.IntegrationRevision.ArtifactID != direct.IntegrationRevision.ArtifactID ||
		got.IntegrationRevision.RevisionID != direct.IntegrationRevision.RevisionID ||
		got.IntegrationRevision.Digest != direct.IntegrationRevision.Digest {
		t.Fatalf("transport integration provenance drifted: %#v != %#v", got.IntegrationRevision, direct.IntegrationRevision)
	}
	if direct.ArtifactRevisions == nil ||
		got.ArtifactRevisions.Profile.RevisionID != direct.ArtifactRevisions.Profile.RevisionID ||
		got.ArtifactRevisions.Profile.Digest != direct.ArtifactRevisions.Profile.Digest ||
		got.ArtifactRevisions.Workflow.RevisionID != direct.ArtifactRevisions.Workflow.RevisionID ||
		got.ArtifactRevisions.Workflow.Digest != direct.ArtifactRevisions.Workflow.Digest {
		t.Fatalf("transport artifact provenance drifted: %#v != %#v", got.ArtifactRevisions, direct.ArtifactRevisions)
	}
	if len(got.Events) != len(direct.Events) || len(got.Deliveries) != len(direct.Deliveries) || len(got.Deliveries) != 1 {
		t.Fatalf("transport cardinality drifted: events=%d/%d deliveries=%d/%d", len(got.Events), len(direct.Events), len(got.Deliveries), len(direct.Deliveries))
	}
	if got.Deliveries[0].Status != string(integration.DeliveryStatusSuppressed) || got.Deliveries[0].Status != string(direct.Deliveries[0].Status) {
		t.Fatalf("transport delivery was not suppressed: %#v", got.Deliveries)
	}
	if got.Correlations.CorrelationID != direct.Correlations.CorrelationID || len(got.Correlations.EventIDs) != len(direct.Correlations.EventIDs) {
		t.Fatalf("transport correlations drifted: %#v != %#v", got.Correlations, direct.Correlations)
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", testBearerToken, `"security"`, `"receipt"`, `"rawPayload"`} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("transport response disclosed %q: %s", forbidden, recorder.Body.String())
		}
	}

	legacyBody := graphQLBody(t, `mutation Legacy($input: SubmitMessageInput!) {
  submitMessage(input: $input) { success eventId }
}`, map[string]any{"input": map[string]any{
		"format": "HL7V2", "data": payload, "source": "browser-owned-source",
	}})
	legacyRecorder := postPreviewGraphQL(t, server.Handler(), legacyBody, "Bearer "+testBearerToken)
	if legacyRecorder.Code != http.StatusOK || !strings.Contains(legacyRecorder.Body.String(), "GraphQL operation forbidden") {
		t.Fatalf("legacy submit did not fail closed: status=%d body=%s", legacyRecorder.Code, legacyRecorder.Body.String())
	}
	if strings.Contains(legacyRecorder.Body.String(), "RAW-PHI-SENTINEL") {
		t.Fatalf("legacy rejection reflected raw payload: %s", legacyRecorder.Body.String())
	}
}

func TestPreviewTransportCatalogSafeRejections(t *testing.T) {
	staticRegistry := loadPreviewTransportRegistry(t)
	messageProcessor := newPreviewTransportProcessor(t, staticRegistry)
	service, err := integrationpreview.NewService(staticRegistry, messageProcessor, time.Now)
	if err != nil {
		t.Fatalf("preview.NewService: %v", err)
	}
	authenticator, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token:       testBearerToken,
		TenantID:    "tenant-a",
		PrincipalID: "engineer-1",
		Roles:       []string{"author"},
	})
	if err != nil {
		t.Fatalf("NewStaticBearerAuthenticator: %v", err)
	}
	config := secureServerConfig(authenticator)
	config.MaxRequestBodyBytes = 16 << 10
	server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithPreviewService(service)), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	body := graphQLBody(t, previewMutation, map[string]any{"input": map[string]any{
		"integrationId": "adt-east",
		"data":          previewTransportA01(),
		"correlationId": "rejection-correlation",
		"reason":        "verify forbidden response",
	}})
	recorder := postPreviewGraphQL(t, server.Handler(), body, "Bearer "+testBearerToken)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "GraphQL operation forbidden") {
		t.Fatalf("role rejection = status %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "tenant-a") || strings.Contains(recorder.Body.String(), "adt-east") {
		t.Fatalf("catalog-safe rejection disclosed registry context: %s", recorder.Body.String())
	}
}

func loadPreviewTransportRegistry(t *testing.T) *registry.StaticRegistry {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open preview registry: %v", err)
	}
	staticRegistry, decodeErr := registry.DecodeStaticRegistry(file)
	closeErr := file.Close()
	if decodeErr != nil {
		t.Fatalf("decode preview registry: %v", decodeErr)
	}
	if closeErr != nil {
		t.Fatalf("close preview registry: %v", closeErr)
	}
	return staticRegistry
}

func newPreviewTransportProcessor(t *testing.T, staticRegistry *registry.StaticRegistry) *processor.MessageProcessor {
	t.Helper()
	definitionResolver, err := processor.NewDefinitionRevisionResolver("tenant-a", staticRegistry)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifactResolver, err := processor.NewRevisionResolver("tenant-a", staticRegistry)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	messageProcessor, err := processor.NewMessageProcessor(definitionResolver, artifactResolver)
	if err != nil {
		t.Fatalf("NewMessageProcessor: %v", err)
	}
	return messageProcessor
}

func graphQLBody(t *testing.T, query string, variables map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		t.Fatalf("marshal GraphQL body: %v", err)
	}
	return body
}

func postPreviewGraphQL(t *testing.T, handler http.Handler, body []byte, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Origin", "https://ide.example.test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func previewTransportA01() string {
	return strings.Join([]string{
		`MSH|^~\&|RAW-PHI-SENTINEL|FAC|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|control-123|P|2.5.1`,
		`EVN|A01|20260713120000||||20260713115900-0400`,
		`PID|1||MRN-123^^^HOSP^MR||Patient^Test||19800101|F`,
		`PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-123|||||||||||||||||||||||||20260713120000`,
	}, "\r")
}
