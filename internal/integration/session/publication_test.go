package session

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestPublicationServicePublishesVerifiesApprovesAndDeploys(t *testing.T) {
	fixture := newPublicationFixture(t, PHIPolicyRedact, false)

	publication, err := fixture.service.Publish(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	manifest, err := fixture.service.Verify(context.Background(), *publication)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if manifest.DefinitionRevision != fixture.definition.Reference() || manifest.DefinitionVersion != 2 || len(manifest.Fixtures) != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(manifest.ExpectedMatchedRoutes) != 1 || len(manifest.ExpectedTransforms) != 1 || len(manifest.ExpectedActions) != 1 {
		t.Fatalf("workflow evidence = routes %v transforms %v actions %v", manifest.ExpectedMatchedRoutes, manifest.ExpectedTransforms, manifest.ExpectedActions)
	}
	if manifest.Fixtures[0].SampleDigest == "" || len(publication.Signature) != ed25519.SignatureSize {
		t.Fatalf("publication evidence = %#v", publication)
	}

	actor := publicationActor()
	snapshot, err := fixture.service.Approve(context.Background(), PromotePublicationRequest{
		SessionID: fixture.request.SessionID, PublicationID: publication.ID,
		ExpectedVersion: 2, Actor: actor, Reason: "approve tested publication",
	})
	if err != nil || snapshot.State != integration.DeploymentStateApproved || snapshot.Version != 3 {
		t.Fatalf("Approve = %#v, %v", snapshot, err)
	}
	snapshot, err = fixture.service.Deploy(context.Background(), PromotePublicationRequest{
		SessionID: fixture.request.SessionID, PublicationID: publication.ID,
		ExpectedVersion: 3, Actor: actor, Reason: "deploy tested publication",
	})
	if err != nil || snapshot.State != integration.DeploymentStateDeployed || snapshot.Version != 5 {
		t.Fatalf("Deploy = %#v, %v", snapshot, err)
	}
	if fixture.lifecycle.approvals != 1 || fixture.lifecycle.publications != 1 || fixture.lifecycle.deployments != 1 {
		t.Fatalf("lifecycle calls = approve %d publish %d deploy %d", fixture.lifecycle.approvals, fixture.lifecycle.publications, fixture.lifecycle.deployments)
	}
}

func TestPublicationServiceRejectsLifecycleChangeAfterPublication(t *testing.T) {
	fixture := newPublicationFixture(t, PHIPolicyRedact, false)
	publication, err := fixture.service.Publish(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	// A repeated connection validation changed the optimistic snapshot after the
	// signed evidence was created, even though the state is still validated.
	fixture.lifecycle.snapshot.Version = 3
	_, err = fixture.service.Approve(context.Background(), PromotePublicationRequest{
		SessionID: fixture.request.SessionID, PublicationID: publication.ID,
		ExpectedVersion: 3, Actor: publicationActor(), Reason: "must republish after validation changes",
	})
	if !errors.Is(err, lifecycle.ErrVersionConflict) {
		t.Fatalf("Approve error = %v, want ErrVersionConflict", err)
	}
	if fixture.lifecycle.approvals != 0 {
		t.Fatalf("Approve calls = %d, want 0", fixture.lifecycle.approvals)
	}
}

func TestPublicationServiceRejectsProductionContentMismatch(t *testing.T) {
	fixture := newPublicationFixture(t, PHIPolicyRedact, true)

	_, err := fixture.service.Publish(context.Background(), fixture.request)
	if !errors.Is(err, ErrPublicationMismatch) {
		t.Fatalf("Publish error = %v, want ErrPublicationMismatch", err)
	}
}

func TestPublicationServiceRejectsRetainedRawFixture(t *testing.T) {
	fixture := newPublicationFixture(t, PHIPolicyRetain, false)

	_, err := fixture.service.Publish(context.Background(), fixture.request)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Publish error = %v, want ErrInvalid", err)
	}
}

func TestPublicationServiceRejectsTamperedManifestBeforeLifecycleTransition(t *testing.T) {
	fixture := newPublicationFixture(t, PHIPolicyRedact, false)
	publication, err := fixture.service.Publish(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	publication.Manifest[len(publication.Manifest)-1] ^= 1
	fixture.store.mu.Lock()
	fixture.store.publications[publication.ID] = clonePublication(publication)
	fixture.store.mu.Unlock()

	_, err = fixture.service.Approve(context.Background(), PromotePublicationRequest{
		SessionID: fixture.request.SessionID, PublicationID: publication.ID,
		ExpectedVersion: 2, Actor: publicationActor(), Reason: "must not approve tampered evidence",
	})
	if !errors.Is(err, ErrPublicationSignature) {
		t.Fatalf("Approve error = %v, want ErrPublicationSignature", err)
	}
	if fixture.lifecycle.approvals != 0 {
		t.Fatalf("Approve calls = %d, want 0", fixture.lifecycle.approvals)
	}
}

func TestPublicationCryptoRejectsMismatchedTrustRoot(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(private): %v", err)
	}
	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(trust): %v", err)
	}
	if _, err := NewPublicationCrypto("release-key", privateKey, map[string]ed25519.PublicKey{"release-key": otherPublicKey}); !errors.Is(err, ErrPublicationUnavailable) {
		t.Fatalf("NewPublicationCrypto error = %v, want ErrPublicationUnavailable", err)
	}
}

type publicationFixture struct {
	store      *MemoryStore
	service    *PublicationService
	lifecycle  *fakePublicationLifecycle
	definition integration.IntegrationDefinitionRevision
	request    PublishRequest
}

func newPublicationFixture(t *testing.T, phiPolicy PHIPolicy, productionMismatch bool) publicationFixture {
	t.Helper()
	ctx := context.Background()
	fixedTime := time.Date(2026, 7, 18, 16, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	store.now = func() time.Time { return fixedTime }
	workspace, err := store.CreateSession(ctx, CreateSessionRequest{Name: "publish fixture"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sample, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "ADT fixture", Format: events.FormatHL7v2, PHIPolicy: phiPolicy,
		Raw: "MSH|^~\\&|SEND|FAC|RECV|FAC|202607181200||ADT^A01|MSG1|P|2.5\rPID|1||12345^^^MRN||Doe^Jane\rPV1|1|I\r",
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	profileContent := []byte(`{"name":"adt-profile","version":"1"}`)
	profile, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{Kind: ArtifactKindMappingProfile, Name: "ADT profile", Content: profileContent})
	if err != nil {
		t.Fatalf("Save profile: %v", err)
	}
	workflowContent := []byte("name: route-admit\nroutes:\n  - name: fhir\n    condition: 'true'\n    actions:\n      - id: send\n        type: webhook\n")
	workflowRevision, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{Kind: ArtifactKindWorkflowDraft, Name: "ADT workflow", Content: workflowContent})
	if err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	run, err := store.CreateRun(ctx, workspace.ID, sample.ID, "adt-east")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run.Status = RunStatusSucceeded
	run.ProfileID = profile.ID
	run.ProfileRevisionID = profile.RevisionID
	run.ProfileRevisionDigest = profile.Digest
	run.Events = []ParsedEvent{{ID: "event-1", Type: string(events.EventPatientAdmit), Payload: json.RawMessage(`{"id":"event-1","type":"patient_admit"}`)}}
	run.Diagnostics = []Diagnostic{{ID: "diag-1", Code: "MISSING_PV1", Severity: "warning", Phase: "semantic", Message: "fixture", Source: "hl7v2_parser", CreatedAt: fixedTime}}
	run.FinishedAt = &fixedTime
	run, err = store.UpdateRun(ctx, *run)
	if err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	simulation, err := store.CreateWorkflowSimulation(ctx, workspace.ID, CreateWorkflowSimulationRequest{
		WorkflowArtifactID: workflowRevision.ID, WorkflowRevisionID: workflowRevision.RevisionID,
		WorkflowRevisionDigest: workflowRevision.Digest, SourceRunIDs: []string{run.ID},
		Events: []WorkflowEventTrace{{RunID: run.ID, EventID: "event-1", EventType: string(events.EventPatientAdmit), Routes: []WorkflowRouteTrace{{
			Name: "fhir", Matched: true,
			Transforms: []WorkflowTransformTrace{{Index: 0, Type: "set_field", Status: "planned"}},
			Actions:    []WorkflowActionTrace{{ID: "send", Type: "webhook", DestinationArtifactID: "fhir-destination"}},
		}}}},
	})
	if err != nil {
		t.Fatalf("CreateWorkflowSimulation: %v", err)
	}

	productionProfileContent := profileContent
	if productionMismatch {
		productionProfileContent = []byte(`{"name":"other-profile","version":"1"}`)
	}
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, productionProfileContent)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", workflowContent)
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	definition := publicationDefinition(t, fixedTime, profileRef, workflowRef)
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	catalog := &fakePublicationLifecycle{
		definitionJSON: definitionJSON,
		snapshot:       lifecycle.Snapshot{TenantID: "tenant-a", DefinitionRevision: definition.Reference(), State: integration.DeploymentStateValidated, Version: 2},
	}
	resolver, err := processor.NewRevisionResolver("tenant-a", publicationArtifactLoader{profile: productionProfileContent, workflow: workflowContent})
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	crypto, err := NewPublicationCrypto("release-key", privateKey, map[string]ed25519.PublicKey{"release-key": publicKey})
	if err != nil {
		t.Fatalf("NewPublicationCrypto: %v", err)
	}
	service, err := NewPublicationService(store, "tenant-a", resolver, catalog, crypto, func() time.Time { return fixedTime })
	if err != nil {
		t.Fatalf("NewPublicationService: %v", err)
	}
	return publicationFixture{
		store: store, service: service, lifecycle: catalog, definition: definition,
		request: PublishRequest{
			SessionID: workspace.ID, ProfileRevisionID: profile.RevisionID, WorkflowSimulationID: simulation.ID,
			DefinitionID: definition.DefinitionID, DefinitionRevisionID: definition.RevisionID,
			PublishedBy: "engineer-1", Reason: "publish tested ADT integration",
		},
	}
}

type publicationArtifactLoader struct {
	profile  []byte
	workflow []byte
}

func (l publicationArtifactLoader) LoadProfileRevision(context.Context, string, string) ([]byte, error) {
	return append([]byte(nil), l.profile...), nil
}

func (l publicationArtifactLoader) LoadWorkflowRevision(context.Context, string, string) ([]byte, error) {
	return append([]byte(nil), l.workflow...), nil
}

type fakePublicationLifecycle struct {
	definitionJSON []byte
	snapshot       lifecycle.Snapshot
	approvals      int
	publications   int
	deployments    int
}

func (f *fakePublicationLifecycle) LoadDefinitionRevision(context.Context, string, string, string) ([]byte, error) {
	return append([]byte(nil), f.definitionJSON...), nil
}

func (f *fakePublicationLifecycle) GetSnapshot(context.Context, string, string, string) (lifecycle.Snapshot, error) {
	return f.snapshot, nil
}

func (f *fakePublicationLifecycle) Approve(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	if command.ExpectedVersion != f.snapshot.Version || f.snapshot.State != integration.DeploymentStateValidated {
		return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	f.approvals++
	f.snapshot.State = integration.DeploymentStateApproved
	f.snapshot.Version++
	return f.snapshot, nil
}

func (f *fakePublicationLifecycle) Publish(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	if command.ExpectedVersion != f.snapshot.Version || f.snapshot.State != integration.DeploymentStateApproved {
		return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	f.publications++
	f.snapshot.State = integration.DeploymentStatePublished
	f.snapshot.Version++
	return f.snapshot, nil
}

func (f *fakePublicationLifecycle) Deploy(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	if command.ExpectedVersion != f.snapshot.Version || f.snapshot.State != integration.DeploymentStatePublished {
		return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	f.deployments++
	f.snapshot.State = integration.DeploymentStateDeployed
	f.snapshot.Version++
	return f.snapshot, nil
}

func publicationDefinition(t *testing.T, createdAt time.Time, profile, workflow integration.ArtifactRevisionRef) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	deployment := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 30, CheckIntervalSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 32, MaxQueued: 1024, MaxMessagesPerSecond: 250},
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "adt-http", RevisionID: "definition-1", TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest('1')}, SourceID: "adt-east"},
		Format: events.FormatHL7v2, Profile: profile, Workflow: workflow,
		Destinations: []integration.DestinationRevisionRef{{ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-destination", RevisionID: "destination-1", Digest: digest('4')}, Class: integration.DestinationClassProduction}},
		Policy:       integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Deployment:   &deployment,
		Created:      integration.AuditEnvelope{TenantID: "tenant-a", Principal: publicationActor(), Reason: "create publication fixture", OccurredAt: createdAt},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}

func publicationActor() integration.Principal {
	return integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:operator"}}
}
