package registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const testWorkflowYAML = `dsl_version: "1"
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

func TestStaticRegistrySelectsExactServerOwnedBindingAndArtifacts(t *testing.T) {
	entry, revision := testEntry(t, "adt-east")
	registry, err := NewStaticRegistry("tenant-a", []Entry{entry})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	binding, err := registry.LookupPreviewBinding(context.Background(), "tenant-a", "adt-east")
	if err != nil {
		t.Fatalf("LookupPreviewBinding: %v", err)
	}
	if binding.IntegrationRevision != revision.Reference() {
		t.Fatalf("integration revision = %#v, want %#v", binding.IntegrationRevision, revision.Reference())
	}
	if binding.SourceID != revision.Source.SourceID || binding.Format != revision.Format {
		t.Fatalf("binding source/format drifted: %#v", binding)
	}
	if binding.Classification != integration.DataClassificationPHI {
		t.Fatalf("classification = %q, want phi", binding.Classification)
	}

	definition, err := registry.LoadDefinitionRevision(context.Background(), "tenant-a", revision.DefinitionID, revision.RevisionID)
	if err != nil {
		t.Fatalf("LoadDefinitionRevision: %v", err)
	}
	profile, err := registry.LoadProfileRevision(context.Background(), revision.Profile.ArtifactID, revision.Profile.RevisionID)
	if err != nil {
		t.Fatalf("LoadProfileRevision: %v", err)
	}
	workflow, err := registry.LoadWorkflowRevision(context.Background(), revision.Workflow.ArtifactID, revision.Workflow.RevisionID)
	if err != nil {
		t.Fatalf("LoadWorkflowRevision: %v", err)
	}

	definition[0], profile[0], workflow[0] = 'x', 'x', 'x'
	definitionAgain, _ := registry.LoadDefinitionRevision(context.Background(), "tenant-a", revision.DefinitionID, revision.RevisionID)
	profileAgain, _ := registry.LoadProfileRevision(context.Background(), revision.Profile.ArtifactID, revision.Profile.RevisionID)
	workflowAgain, _ := registry.LoadWorkflowRevision(context.Background(), revision.Workflow.ArtifactID, revision.Workflow.RevisionID)
	if definitionAgain[0] == 'x' || profileAgain[0] == 'x' || workflowAgain[0] == 'x' {
		t.Fatal("registry returned mutable backing bytes")
	}
}

func TestDecodeStaticRegistryDocument(t *testing.T) {
	entry, _ := testEntry(t, "adt-east")
	document, err := json.Marshal(map[string]any{
		"tenant_id": "tenant-a",
		"integrations": []map[string]any{{
			"integration_id": entry.IntegrationID,
			"definition":     json.RawMessage(entry.DefinitionJSON),
			"profile":        json.RawMessage(entry.ProfileJSON),
			"workflow":       string(entry.WorkflowYAML),
		}},
	})
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	decoded, err := DecodeStaticRegistry(strings.NewReader(string(document)))
	if err != nil {
		t.Fatalf("DecodeStaticRegistry: %v", err)
	}
	if decoded.DeploymentTenantID() != "tenant-a" {
		t.Fatalf("deployment tenant = %q", decoded.DeploymentTenantID())
	}
	if _, err := decoded.LookupPreviewBinding(context.Background(), "tenant-a", "adt-east"); err != nil {
		t.Fatalf("lookup decoded registry: %v", err)
	}

	for _, invalid := range []string{
		``,
		`{"tenant_id":"tenant-a","integrations":[],"unknown":true}`,
		string(document) + `{}`,
		strings.Repeat("x", MaxRegistryBytes+1),
	} {
		if _, err := DecodeStaticRegistry(strings.NewReader(invalid)); err == nil {
			t.Fatal("invalid registry document was accepted")
		}
	}
}

func TestStaticRegistryFailsClosed(t *testing.T) {
	entry, _ := testEntry(t, "adt-east")

	tests := []struct {
		name    string
		mutate  func(*Entry)
		entries func(Entry) []Entry
	}{
		{name: "empty integration key", mutate: func(e *Entry) { e.IntegrationID = "" }},
		{name: "noncanonical integration key", mutate: func(e *Entry) { e.IntegrationID = " adt-east" }},
		{name: "malformed definition", mutate: func(e *Entry) { e.DefinitionJSON = []byte(`{"tenant_id":"tenant-a"}`) }},
		{name: "profile digest mismatch", mutate: func(e *Entry) { e.ProfileJSON = []byte(`{"hl7v2":{"timezone":"America/New_York"}}`) }},
		{name: "workflow digest mismatch", mutate: func(e *Entry) { e.WorkflowYAML = []byte("name: changed\n") }},
		{name: "oversized profile", mutate: func(e *Entry) { e.ProfileJSON = []byte(strings.Repeat("x", MaxArtifactBytes+1)) }},
		{name: "duplicate key", entries: func(e Entry) []Entry { return []Entry{e, e} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := entry
			candidate.DefinitionJSON = append([]byte(nil), entry.DefinitionJSON...)
			candidate.ProfileJSON = append([]byte(nil), entry.ProfileJSON...)
			candidate.WorkflowYAML = append([]byte(nil), entry.WorkflowYAML...)
			if tt.mutate != nil {
				tt.mutate(&candidate)
			}
			entries := []Entry{candidate}
			if tt.entries != nil {
				entries = tt.entries(candidate)
			}
			if _, err := NewStaticRegistry("tenant-a", entries); err == nil {
				t.Fatal("unsafe registry configuration was accepted")
			}
		})
	}
}

func TestStaticRegistryRejectsWrongTenantAndUnknownIntegration(t *testing.T) {
	entry, revision := testEntry(t, "adt-east")
	registry, err := NewStaticRegistry("tenant-a", []Entry{entry})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	if _, err := registry.LookupPreviewBinding(context.Background(), "tenant-b", "adt-east"); !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("wrong-tenant error = %v, want ErrTenantMismatch", err)
	}
	if _, err := registry.LookupPreviewBinding(context.Background(), "tenant-a", "missing"); !errors.Is(err, ErrIntegrationNotFound) {
		t.Fatalf("missing integration error = %v, want ErrIntegrationNotFound", err)
	}
	if _, err := registry.LoadDefinitionRevision(context.Background(), "tenant-b", revision.DefinitionID, revision.RevisionID); !errors.Is(err, processor.ErrDefinitionRevisionNotFound) {
		t.Fatalf("cross-tenant definition error = %v, want catalog-safe not found", err)
	}
}

func TestNilStaticRegistryArtifactLoadsFailClosed(t *testing.T) {
	var registry *StaticRegistry
	if _, err := registry.LoadProfileRevision(context.Background(), "profile-adt", "1"); err == nil {
		t.Fatal("nil registry loaded a profile")
	}
	if _, err := registry.LoadWorkflowRevision(context.Background(), "workflow-adt", "1"); err == nil {
		t.Fatal("nil registry loaded a workflow")
	}
}

func testEntry(t *testing.T, integrationID string) (Entry, integration.IntegrationDefinitionRevision) {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(testProfileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-version-1", []byte(testWorkflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "definition-revision-1",
		TenantID:     "tenant-a",
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
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID:   "tenant-a",
			Principal:  integration.Principal{ID: "publisher", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}},
			Reason:     "publish preview fixture",
			OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	return Entry{
		IntegrationID:  integrationID,
		DefinitionJSON: definitionJSON,
		ProfileJSON:    []byte(testProfileJSON),
		WorkflowYAML:   []byte(testWorkflowYAML),
	}, revision
}
