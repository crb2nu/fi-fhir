package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
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

func TestLoadHTTPIngressAuthenticatorFromEnv(t *testing.T) {
	t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "bearer")
	t.Setenv("FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID", "adt-service")
	t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "correct-http-ingress-token-001")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES", "4096")

	authenticator, maxBodyBytes, err := loadHTTPIngressAuthenticatorFromEnv()
	if err != nil {
		t.Fatalf("loadHTTPIngressAuthenticatorFromEnv: %v", err)
	}
	if authenticator.PrincipalID() != "adt-service" || authenticator.IntegrationID() != "adt-east" || maxBodyBytes != 4096 {
		t.Fatalf("ingress configuration = principal %q integration %q max %d", authenticator.PrincipalID(), authenticator.IntegrationID(), maxBodyBytes)
	}

	secretFile := filepath.Join(t.TempDir(), "ingress-token")
	if err := os.WriteFile(secretFile, []byte("file-backed-http-ingress-token-001\n"), 0o600); err != nil {
		t.Fatalf("write ingress secret: %v", err)
	}
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", secretFile)
	if _, _, err := loadHTTPIngressAuthenticatorFromEnv(); err != nil {
		t.Fatalf("file-backed ingress secret: %v", err)
	}
}

func TestLoadServeIntegrationRuntimeFailsBeforeDatabaseMutation(t *testing.T) {
	configurePreviewRuntimeForTest(t)
	t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "bearer")
	t.Setenv("FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID", "adt-service")
	t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", "")
	for _, name := range []string{
		"FI_FHIR_DATABASE_HOST",
		"FI_FHIR_DATABASE_NAME",
		"FI_FHIR_DATABASE_USERNAME",
		"FI_FHIR_DATABASE_USER",
	} {
		t.Setenv(name, "")
	}

	_, err := loadServeIntegrationRuntimeFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FI_FHIR_HTTP_INGRESS_SECRET") {
		t.Fatalf("missing ingress secret error = %v", err)
	}

	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "correct-http-ingress-token-001")
	_, err = loadServeIntegrationRuntimeFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FI_FHIR_DATABASE_HOST") {
		t.Fatalf("missing PostgreSQL configuration error = %v", err)
	}
}

func TestLoadMLLPRuntimeFromEnv(t *testing.T) {
	source := writeMLLPSourceRevision(t, mllp.TLSPolicy{Mode: mllp.TLSModeDisabled})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "integration-mllp")
	t.Setenv("FI_FHIR_MLLP_PRINCIPAL_ID", "mllp-listener")
	loaded, definitionID, principalID, material, err := loadMLLPRuntimeFromEnv(source)
	if err != nil {
		t.Fatalf("loadMLLPRuntimeFromEnv: %v", err)
	}
	if loaded.SourceID != "adt-east" || definitionID != "integration-mllp" || principalID != "mllp-listener" {
		t.Fatalf("unexpected MLLP runtime: %#v %q %q", loaded, definitionID, principalID)
	}
	if len(material.CertificatePEM) != 0 || len(material.PrivateKeyPEM) != 0 || len(material.ClientCAPEM) != 0 {
		t.Fatalf("plaintext mode loaded TLS material: %#v", material)
	}
}

func TestLoadMLLPRuntimeFromEnvFailsClosedBeforeDatabase(t *testing.T) {
	source := writeMLLPSourceRevision(t, mllp.TLSPolicy{Mode: mllp.TLSModeDisabled})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "")
	t.Setenv("FI_FHIR_MLLP_PRINCIPAL_ID", "mllp-listener")
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(source); err == nil || !strings.Contains(err.Error(), "FI_FHIR_MLLP_DEFINITION_ID") {
		t.Fatalf("missing definition error = %v", err)
	}

	mutualSource := writeMLLPSourceRevision(t, mllp.TLSPolicy{
		Mode: mllp.TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "integration-mllp")
	t.Setenv("FI_FHIR_MLLP_TLS_CERT_FILE", "")
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(mutualSource); err == nil || !strings.Contains(err.Error(), "FI_FHIR_MLLP_TLS_CERT_FILE") {
		t.Fatalf("missing TLS file error = %v", err)
	}
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

func writeMLLPSourceRevision(t *testing.T, tlsPolicy mllp.TLSPolicy) string {
	t.Helper()
	revision, err := mllp.NewSourceRevision(mllp.SourceRevisionInput{
		ArtifactID: "source-adt", RevisionID: "source-1", SourceID: "adt-east",
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:  mllp.FramingPolicy{StartByte: mllp.StandardStartByte, EndByte: mllp.StandardEndByte, TrailerByte: mllp.StandardTrailerByte},
		Timeouts: mllp.TimeoutPolicy{ReadSeconds: 5, WriteSeconds: 5, IdleSeconds: 30, ProcessSeconds: 30},
		TLS:      tlsPolicy, Clients: mllp.ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}},
		Acknowledgements: mllp.AcknowledgementPolicy{Mode: mllp.AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  1048576, MaxConnections: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mllp-source.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
