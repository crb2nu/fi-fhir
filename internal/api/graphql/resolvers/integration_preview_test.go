package resolvers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const resolverPreviewProfile = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const resolverPreviewWorkflow = `dsl_version: "1"
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

func TestPreviewIntegrationMessageMatchesDirectKernelProjection(t *testing.T) {
	fixture := newResolverPreviewFixture(t)
	resolver := NewResolver(WithPreviewService(fixture.service))
	mutation := &mutationResolver{resolver}
	ctx := requestsecurity.WithSecurityContext(context.Background(), fixture.security)
	input := model.PreviewIntegrationMessageInput{
		IntegrationID: "adt-east",
		Data:          string(fixture.payload),
		CorrelationID: "correlation-123",
		Reason:        "verify ADT mapping",
	}

	got, err := mutation.PreviewIntegrationMessage(ctx, input)
	if err != nil {
		t.Fatalf("PreviewIntegrationMessage: %v", err)
	}

	direct, err := fixture.processor.Process(context.Background(), fixture.directRequest(t, input))
	if err != nil {
		t.Fatalf("direct MessageProcessor.Process: %v", err)
	}
	want, err := projectIntegrationPreview(direct)
	if err != nil {
		t.Fatalf("project direct result: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("adapter/kernel projection mismatch\ngot: %s\nwant: %s", gotJSON, wantJSON)
	}
	if got.ArtifactRevisions.Profile.RevisionID != "1" || got.ArtifactRevisions.Workflow.RevisionID != "workflow-version-1" {
		t.Fatalf("exact artifact provenance missing: %#v", got.ArtifactRevisions)
	}
	if len(got.Deliveries) != 1 || got.Deliveries[0].Status != string(integration.DeliveryStatusSuppressed) {
		t.Fatalf("preview delivery was not suppressed: %#v", got.Deliveries)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal preview projection: %v", err)
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", "correct-horse-battery-staple", `"security"`, `"receipt"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("preview response disclosed forbidden data %q: %s", forbidden, encoded)
		}
	}
}

func TestPreviewIntegrationMessageRequiresAuthenticatedContextAndService(t *testing.T) {
	input := model.PreviewIntegrationMessageInput{IntegrationID: "adt-east", Data: "message", CorrelationID: "correlation-1", Reason: "preview"}
	resolver := NewResolver()
	mutation := &mutationResolver{resolver}
	if _, err := mutation.PreviewIntegrationMessage(context.Background(), input); err == nil || err.Error() != "authentication required" {
		t.Fatalf("missing-auth error = %v", err)
	}

	ctx := requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID:  "tenant-a",
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer", Roles: []string{preview.PreviewRole}},
	})
	if _, err := mutation.PreviewIntegrationMessage(ctx, input); err == nil || err.Error() != "integration preview unavailable" {
		t.Fatalf("missing-service error = %v", err)
	}
}

type resolverPreviewFixture struct {
	registry  *registry.StaticRegistry
	processor *processor.MessageProcessor
	service   *preview.Service
	revision  integration.IntegrationDefinitionRevision
	security  integration.SecurityContext
	payload   []byte
	now       time.Time
}

func newResolverPreviewFixture(t *testing.T) resolverPreviewFixture {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(resolverPreviewProfile))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-version-1", []byte(resolverPreviewWorkflow))
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
		Policy: integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a", Principal: integration.Principal{ID: "publisher", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}}, Reason: "publish", OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	staticRegistry, err := registry.NewStaticRegistry("tenant-a", []registry.Entry{{
		IntegrationID:  "adt-east",
		DefinitionJSON: definitionJSON,
		ProfileJSON:    []byte(resolverPreviewProfile),
		WorkflowYAML:   []byte(resolverPreviewWorkflow),
	}})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}
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
	now := time.Date(2026, 7, 13, 16, 30, 0, 0, time.UTC)
	service, err := preview.NewService(staticRegistry, messageProcessor, func() time.Time { return now })
	if err != nil {
		t.Fatalf("preview.NewService: %v", err)
	}
	security := integration.SecurityContext{
		TenantID:  "tenant-a",
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer", Roles: []string{preview.PreviewRole}},
	}
	return resolverPreviewFixture{
		registry:  staticRegistry,
		processor: messageProcessor,
		service:   service,
		revision:  revision,
		security:  security,
		payload:   resolverA01(),
		now:       now,
	}
}

func (f resolverPreviewFixture) directRequest(t *testing.T, input model.PreviewIntegrationMessageInput) integration.ProcessRequest {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       f.security.TenantID,
		SourceID:       f.revision.Source.SourceID,
		Format:         f.revision.Format,
		ContentType:    "application/hl7-v2+er7",
		ReceivedAt:     f.now,
		Classification: f.revision.Policy.Classification,
	}, []byte(input.Data))
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	security := f.security
	security.Reason = input.Reason
	return integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: f.revision.Reference(),
		Security:            security,
		Envelope:            envelope,
		CorrelationID:       input.CorrelationID,
	}
}

func resolverA01() []byte {
	segments := []string{
		`MSH|^~\&|RAW-PHI-SENTINEL|FAC|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|control-123|P|2.5.1`,
		resolverSegment("EVN", 6, map[int]string{1: "A01", 2: "20260713120000", 6: "20260713115900-0400"}),
		resolverSegment("PID", 8, map[int]string{1: "1", 3: "MRN-123^^^HOSP^MR", 5: "Patient^Test", 7: "19800101", 8: "F"}),
		resolverSegment("PV1", 44, map[int]string{1: "1", 2: "I", 3: "UNIT^101^A^FAC", 19: "visit-123", 44: "20260713120000"}),
	}
	return []byte(strings.Join(segments, "\r"))
}

func resolverSegment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}
