//go:build integration

package processor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	graphqlstore "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const integrationTolerantProfile = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const integrationStrictProfile = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const integrationWorkflowV1 = `dsl_version: "1"
name: adt-v1
version: "1"
routes:
  - name: admit
    filter:
      event_type: patient_admit
    actions:
      - id: audit
        type: log
      - id: send-fhir
        type: fhir
        destination: fhir-primary
  - name: invalid-cel
    filter:
      condition: "event.???"
    actions:
      - id: never
        type: log
`

const integrationWorkflowV2 = `dsl_version: "1"
name: adt-v2
version: "2"
routes:
  - name: labs-only
    filter:
      event_type: lab_result
    actions:
      - id: audit
        type: log
`

func TestMessageProcessorPreviewKernel_PostgresV1SurvivesV2(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for message processor integration tests")
	}

	schema := fmt.Sprintf("message_processor_%d", time.Now().UnixNano())
	createArtifactResolverSchema(t, dsn, schema)
	db := openArtifactResolverDB(t, dsn, schema)
	profiles := graphqlstore.NewPostgresProfileStore(db)
	workflows := graphqlstore.NewPostgresWorkflowLifecycleStore(db)
	if err := profiles.InitSchema(ctx); err != nil {
		t.Fatalf("profile InitSchema: %v", err)
	}
	if err := workflows.InitSchema(ctx); err != nil {
		t.Fatalf("workflow InitSchema: %v", err)
	}

	profileRecord := &graphqlstore.Profile{
		ID:        "profile-adt",
		Name:      "ADT executable profile",
		Version:   "v1",
		Config:    json.RawMessage(integrationTolerantProfile),
		CreatedBy: "integration-test",
	}
	if err := profiles.CreateProfile(ctx, profileRecord); err != nil {
		t.Fatalf("CreateProfile(v1): %v", err)
	}
	profileV1, err := profiles.GetCurrentProfileRevision(ctx, profileRecord.ID)
	if err != nil || profileV1 == nil {
		t.Fatalf("GetCurrentProfileRevision(v1): revision=%#v err=%v", profileV1, err)
	}
	profileV1Ref, err := processor.NewProfileRevisionReference(profileRecord.ID, profileV1.ID, profileV1.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(v1): %v", err)
	}

	workflowRecord, err := workflows.CreateWorkflowDefinition(ctx, &graphqlstore.WorkflowDefinitionRecord{
		ID:   "workflow-adt",
		Name: fmt.Sprintf("processor_adt_%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition: %v", err)
	}
	workflowV1, err := workflows.SaveWorkflowVersion(ctx, &graphqlstore.WorkflowVersionRecord{
		WorkflowID: workflowRecord.ID,
		Yaml:       integrationWorkflowV1,
		CreatedBy:  "integration-test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v1): %v", err)
	}
	if _, err := workflows.PublishWorkflowVersion(ctx, &graphqlstore.WorkflowReleaseRecord{
		WorkflowID:  workflowRecord.ID,
		VersionID:   workflowV1.ID,
		Environment: "production",
		PublishedBy: "integration-test",
	}); err != nil {
		t.Fatalf("PublishWorkflowVersion(v1): %v", err)
	}
	workflowV1Ref, err := processor.NewWorkflowRevisionReference(workflowRecord.ID, workflowV1.ID, []byte(workflowV1.Yaml))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(v1): %v", err)
	}

	revisionV1 := executableDefinitionRevision(t, "definition-v1", profileV1Ref, workflowV1Ref)
	definitions := newStaticDefinitionLoader(t, revisionV1)
	processorV1 := newPostgresMessageProcessor(t, "tenant-a", definitions, profiles, workflows)
	requestV1 := executableProcessRequest(t, revisionV1, executableA01(false))
	first, err := processorV1.Process(ctx, requestV1)
	if err != nil {
		t.Fatalf("Process(v1 initial): %v", err)
	}
	assertProcessorV1Result(t, first, requestV1, revisionV1)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal v1 initial: %v", err)
	}

	profileRecord.Version = "v2"
	profileRecord.Config = json.RawMessage(integrationStrictProfile)
	profileRecord.ChangeSummary = "Advance message processor fixture"
	if err := profiles.UpdateProfile(ctx, profileRecord); err != nil {
		t.Fatalf("UpdateProfile(v2): %v", err)
	}
	profileV2, err := profiles.GetCurrentProfileRevision(ctx, profileRecord.ID)
	if err != nil || profileV2 == nil || profileV2.ID == profileV1.ID {
		t.Fatalf("GetCurrentProfileRevision(v2): revision=%#v err=%v", profileV2, err)
	}
	profileV2Ref, err := processor.NewProfileRevisionReference(profileRecord.ID, profileV2.ID, profileV2.Config)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(v2): %v", err)
	}
	workflowV2, err := workflows.SaveWorkflowVersion(ctx, &graphqlstore.WorkflowVersionRecord{
		WorkflowID: workflowRecord.ID,
		Yaml:       integrationWorkflowV2,
		CreatedBy:  "integration-test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v2): %v", err)
	}
	if _, err := workflows.PublishWorkflowVersion(ctx, &graphqlstore.WorkflowReleaseRecord{
		WorkflowID:  workflowRecord.ID,
		VersionID:   workflowV2.ID,
		Environment: "production",
		PublishedBy: "integration-test",
	}); err != nil {
		t.Fatalf("PublishWorkflowVersion(v2): %v", err)
	}
	publishedV2, err := workflows.GetPublishedWorkflowRelease(ctx, workflowRecord.ID, "production")
	if err != nil || publishedV2 == nil || publishedV2.VersionID != workflowV2.ID {
		t.Fatalf("published workflow pointer did not advance: release=%#v err=%v", publishedV2, err)
	}
	workflowV2Ref, err := processor.NewWorkflowRevisionReference(workflowRecord.ID, workflowV2.ID, []byte(workflowV2.Yaml))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(v2): %v", err)
	}
	revisionV2 := executableDefinitionRevision(t, "definition-v2", profileV2Ref, workflowV2Ref)
	definitions.add(t, revisionV2)

	if err := db.Close(); err != nil {
		t.Fatalf("close first database handle: %v", err)
	}
	db = openArtifactResolverDB(t, dsn, schema)
	freshProfiles := graphqlstore.NewPostgresProfileStore(db)
	freshWorkflows := graphqlstore.NewPostgresWorkflowLifecycleStore(db)
	if err := freshProfiles.InitSchema(ctx); err != nil {
		t.Fatalf("fresh profile InitSchema: %v", err)
	}
	if err := freshWorkflows.InitSchema(ctx); err != nil {
		t.Fatalf("fresh workflow InitSchema: %v", err)
	}
	freshProcessor := newPostgresMessageProcessor(t, "tenant-a", definitions, freshProfiles, freshWorkflows)

	again, err := freshProcessor.Process(ctx, requestV1)
	if err != nil {
		t.Fatalf("Process(v1 after v2): %v", err)
	}
	againJSON, err := json.Marshal(again)
	if err != nil {
		t.Fatalf("marshal v1 after v2: %v", err)
	}
	if !bytes.Equal(firstJSON, againJSON) {
		t.Fatalf("v1 changed after v2 pointers advanced:\nfirst: %s\nagain: %s", firstJSON, againJSON)
	}

	requestV2MissingPV1 := executableProcessRequest(t, revisionV2, executableA01(false))
	if _, err := freshProcessor.Process(ctx, requestV2MissingPV1); !errors.Is(err, processor.ErrInvalidSourceMessage) {
		t.Fatalf("strict v2 accepted missing PV1: %v", err)
	}
	requestV2 := executableProcessRequest(t, revisionV2, executableA01(true))
	resultV2, err := freshProcessor.Process(ctx, requestV2)
	if err != nil {
		t.Fatalf("Process(v2 full A01): %v", err)
	}
	if len(resultV2.Routes) != 1 || resultV2.Routes[0].Route != "labs-only" || resultV2.Routes[0].Matched {
		t.Fatalf("v2 workflow revision was not selected: %#v", resultV2.Routes)
	}
	if resultV2.ArtifactRevisions == nil || resultV2.ArtifactRevisions.Profile != profileV2Ref || resultV2.ArtifactRevisions.Workflow != workflowV2Ref {
		t.Fatalf("v2 provenance mismatch: %#v", resultV2.ArtifactRevisions)
	}
}

func assertProcessorV1Result(t *testing.T, result integration.ProcessResult, request integration.ProcessRequest, revision integration.IntegrationDefinitionRevision) {
	t.Helper()
	if err := result.ValidatePreviewFor(request, revision); err != nil {
		t.Fatalf("ValidatePreviewFor(v1): %v", err)
	}
	if len(result.Events) != 1 || len(result.Routes) != 2 || !result.Routes[0].Matched || len(result.Deliveries) != 1 {
		t.Fatalf("unexpected v1 result shape: events=%d routes=%#v deliveries=%#v", len(result.Events), result.Routes, result.Deliveries)
	}
	codes := make(map[string]bool)
	for _, diagnostic := range result.Diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, required := range []string{"MISSING_PV1", "INVALID_CEL"} {
		if !codes[required] {
			t.Fatalf("v1 result lacks %s: %#v", required, result.Diagnostics)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal v1 result: %v", err)
	}
	if bytes.Contains(encoded, []byte("RAW-POSTGRES-SENTINEL")) || bytes.Contains(encoded, []byte("event.???")) {
		t.Fatalf("v1 result leaked raw/workflow content: %s", encoded)
	}
}

type staticDefinitionLoader struct {
	revisions map[string][]byte
}

func newStaticDefinitionLoader(t *testing.T, revisions ...integration.IntegrationDefinitionRevision) *staticDefinitionLoader {
	t.Helper()
	loader := &staticDefinitionLoader{revisions: make(map[string][]byte)}
	for _, revision := range revisions {
		loader.add(t, revision)
	}
	return loader
}

func (l *staticDefinitionLoader) add(t *testing.T, revision integration.IntegrationDefinitionRevision) {
	t.Helper()
	raw, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition revision: %v", err)
	}
	l.revisions[definitionLoaderKey(revision.TenantID, revision.DefinitionID, revision.RevisionID)] = raw
}

func (l *staticDefinitionLoader) LoadDefinitionRevision(_ context.Context, tenantID, definitionID, revisionID string) ([]byte, error) {
	raw, found := l.revisions[definitionLoaderKey(tenantID, definitionID, revisionID)]
	if !found {
		return nil, processor.ErrDefinitionRevisionNotFound
	}
	return append([]byte(nil), raw...), nil
}

func definitionLoaderKey(tenantID, definitionID, revisionID string) string {
	return tenantID + "\x00" + definitionID + "\x00" + revisionID
}

func newPostgresMessageProcessor(
	t *testing.T,
	tenantID string,
	definitions processor.DefinitionRevisionLoader,
	profiles graphqlstore.ProfileRevisionReader,
	workflows graphqlstore.WorkflowVersionReader,
) *processor.MessageProcessor {
	t.Helper()
	definitionResolver, err := processor.NewDefinitionRevisionResolver(tenantID, definitions)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifactResolver, err := processor.NewRevisionResolver(tenantID, graphqlstore.NewArtifactRevisionLoader(profiles, workflows))
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	messageProcessor, err := processor.NewMessageProcessor(definitionResolver, artifactResolver)
	if err != nil {
		t.Fatalf("NewMessageProcessor: %v", err)
	}
	return messageProcessor
}

func executableDefinitionRevision(
	t *testing.T,
	revisionID string,
	profileRef integration.ArtifactRevisionRef,
	workflowRef integration.ArtifactRevisionRef,
) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   revisionID,
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
			Principal:  integration.Principal{ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}

func executableProcessRequest(t *testing.T, revision integration.IntegrationDefinitionRevision, raw []byte) integration.ProcessRequest {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       revision.TenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC),
		Classification: integration.DataClassificationPHI,
	}, raw)
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID: revision.TenantID,
			Principal: integration.Principal{
				ID:         "source-service",
				Kind:       integration.PrincipalKindService,
				AuthMethod: "mtls",
				SourceID:   revision.Source.SourceID,
			},
		},
		Envelope:      envelope,
		CorrelationID: "correlation-123",
	}
	if err := request.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid process request: %v", err)
	}
	return request
}

func executableA01(includePV1 bool) []byte {
	msh := `MSH|^~\&|RAW-POSTGRES-SENTINEL|FAC|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|control-123|P|2.5.1`
	evn := executableSegment("EVN", 6, map[int]string{1: "A01", 2: "20260713120000", 6: "20260713115900-0400"})
	pid := executableSegment("PID", 8, map[int]string{1: "1", 3: "MRN-123^^^HOSP^MR", 5: "Patient^Test", 7: "19800101", 8: "F"})
	segments := []string{msh, evn, pid}
	if includePV1 {
		segments = append(segments, executableSegment("PV1", 44, map[int]string{1: "1", 2: "I", 3: "UNIT^101^A^FAC", 19: "visit-123", 44: "20260713120000"}))
	}
	return []byte(strings.Join(segments, "\r"))
}

func executableSegment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}
