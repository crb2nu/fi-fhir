package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const runtimeProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const runtimeWorkflowYAML = `dsl_version: "1"
name: adt-preview
version: "1"
routes:
  - name: admit
    filter:
      event_type: patient_admit
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
`

func TestLoadPreviewRuntimeFromEnv(t *testing.T) {
	registryPath := writeRuntimeRegistry(t, "tenant-a")
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview,author")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "https://ide.example.test,http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", registryPath)

	runtime, err := loadPreviewRuntimeFromEnv()
	if err != nil {
		t.Fatalf("loadPreviewRuntimeFromEnv: %v", err)
	}
	if runtime.previewService == nil || runtime.authenticator == nil {
		t.Fatalf("incomplete preview runtime: %#v", runtime)
	}
	if got := strings.Join(runtime.allowedOrigins, ","); got != "https://ide.example.test,http://localhost:5173" {
		t.Fatalf("allowed origins = %q", got)
	}
	security, err := runtime.authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "engineer-1" {
		t.Fatalf("security = %#v", security)
	}
}

func TestCheckedInPreviewRegistryLoads(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json")
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", path)

	runtime, err := loadPreviewRuntimeFromEnv()
	if err != nil {
		t.Fatalf("load checked-in preview registry: %v", err)
	}
	if runtime.previewService == nil {
		t.Fatal("checked-in preview registry did not configure the preview service")
	}
}

func TestLoadPreviewRuntimeFromEnvFailsClosed(t *testing.T) {
	validRegistry := writeRuntimeRegistry(t, "tenant-a")
	valid := map[string]string{
		"FI_FHIR_DEPLOYMENT_TENANT_ID":      "tenant-a",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN":      "correct-horse-battery-staple",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE": "",
		"FI_FHIR_GRAPHQL_PRINCIPAL_ID":      "engineer-1",
		"FI_FHIR_GRAPHQL_ROLES":             "integration:preview",
		"FI_FHIR_GRAPHQL_ALLOWED_ORIGINS":   "https://ide.example.test",
		"FI_FHIR_INTEGRATION_REGISTRY_PATH": validRegistry,
	}

	for missing := range valid {
		if missing == "FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE" {
			continue
		}
		t.Run("missing "+missing, func(t *testing.T) {
			for key, value := range valid {
				t.Setenv(key, value)
			}
			t.Setenv(missing, "")
			if _, err := loadPreviewRuntimeFromEnv(); err == nil {
				t.Fatal("incomplete preview runtime was accepted")
			}
		})
	}

	t.Run("tenant mismatch", func(t *testing.T) {
		for key, value := range valid {
			t.Setenv(key, value)
		}
		t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-b")
		if _, err := loadPreviewRuntimeFromEnv(); err == nil {
			t.Fatal("cross-tenant registry was accepted")
		}
	})

	t.Run("ambiguous token sources", func(t *testing.T) {
		for key, value := range valid {
			t.Setenv(key, value)
		}
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte("another-correct-bearer-token"), 0o600); err != nil {
			t.Fatalf("write token: %v", err)
		}
		t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", tokenFile)
		if _, err := loadPreviewRuntimeFromEnv(); err == nil {
			t.Fatal("multiple bearer token sources were accepted")
		}
	})
}

func configurePreviewRuntimeForTest(t *testing.T) {
	t.Helper()
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", writeRuntimeRegistry(t, "tenant-a"))
}

func writeRuntimeRegistry(t *testing.T, tenantID string) string {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(runtimeProfileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-version-1", []byte(runtimeWorkflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "definition-revision-1",
		TenantID:     tenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d")},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Created: integration.AuditEnvelope{
			TenantID: tenantID, Principal: integration.Principal{ID: "publisher", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}}, Reason: "publish", OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	document, err := json.MarshalIndent(map[string]any{
		"tenant_id": tenantID,
		"integrations": []map[string]any{{
			"integration_id": "adt-east",
			"definition":     revision,
			"profile":        json.RawMessage(runtimeProfileJSON),
			"workflow":       runtimeWorkflowYAML,
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}
