package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// .loom/42 E-1 kill-test (a): a definition authored through the API path
// (compiled connection documents + a static-registry entry proven by the
// runtime resolver + explicit binding references, the way the editor
// assembles one) is byte-for-byte the definition `lifecycle seed` writes for
// the same inputs: equal semantic digest and equal canonical JSON, for both
// batch providers the seed supports. MLLP, which the seed does not author, is
// held against a hand-built revision in authoring's service_test.go.

func parityCreated() integration.AuditEnvelope {
	return integration.AuditEnvelope{
		TenantID: seedTestTenant,
		Principal: integration.Principal{
			ID: seedTestPrincipal, Kind: integration.PrincipalKindHuman,
			AuthMethod: lifecycleSeedAuthMethod, Roles: []string{lifecycleSeedDefaultRole},
		},
		Reason: seedTestReason, OccurredAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	}
}

// seedPathDefinition runs the CLI's own argument parser, input loader, and
// builder.
func seedPathDefinition(t *testing.T, fixture seedFixture) integration.IntegrationDefinitionRevision {
	t.Helper()
	parsed, err := parseLifecycleSeedArgs(fixture.args("--validate", "skip"))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := loadLifecycleSeedInputs(context.Background(), parsed, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := buildSeedDefinition(inputs, parityCreated())
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

// apiPathDefinition assembles the same definition the way the editor does:
// the source and destination from their compiled connection documents, the
// profile and workflow from the registry entry the runtime resolver proves,
// and bindings as the explicit references a connection draft carries.
func apiPathDefinition(t *testing.T, fixture seedFixture) integration.IntegrationDefinitionRevision {
	t.Helper()
	sourceDocument, err := json.Marshal(fixture.source)
	if err != nil {
		t.Fatal(err)
	}
	kind := connection.KindBatchSFTP
	if fixture.source.Provider == integrationbatch.ProviderS3 {
		kind = connection.KindBatchS3
	}
	source, err := authoring.SourceFromDocument(kind, sourceDocument)
	if err != nil {
		t.Fatal(err)
	}
	destinationDocument, err := json.Marshal(fixture.destination)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := authoring.DestinationFromDocument(connection.KindFHIR, destinationDocument)
	if err != nil {
		t.Fatal(err)
	}
	staticRegistry, err := loadSeedRegistry(fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	proven, err := authoring.NewRegistry(seedTestTenant, staticRegistry)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := proven.Artifact(context.Background(), "adt-east")
	if err != nil {
		t.Fatal(err)
	}
	fileRef := func(key string) integration.SecretReference {
		return integration.SecretReference{Provider: integration.SecretProviderFile, Key: key}
	}
	// Listed out of order on purpose: the editor sends them as the author
	// arranged them.
	bindings := []integration.SecretBinding{
		{Name: fixture.destination.ArtifactID + "-token", Reference: fileRef("destinations/" + fixture.destination.ArtifactID + "-token")},
	}
	for _, name := range fixture.source.SecretBindingNames() {
		bindings = append([]integration.SecretBinding{{Name: name, Reference: fileRef("batch/" + name)}}, bindings...)
	}
	revision, err := authoring.BuildDefinition(authoring.Draft{
		DefinitionID: seedTestDefinition, RevisionID: "v1", TenantID: seedTestTenant,
		Source: source, Profile: artifact.Profile, Workflow: artifact.Workflow,
		Destinations:   []authoring.Destination{destination},
		SecretBindings: bindings,
		Policy:         authoring.DefaultPolicy(),
		Deployment:     authoring.DefaultDeploymentPolicy(0),
		Created:        parityCreated(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func assertSameDefinition(t *testing.T, seeded, authored integration.IntegrationDefinitionRevision) {
	t.Helper()
	if seeded.Digest != authored.Digest {
		t.Fatalf("digest: seed %s, authoring %s", seeded.Digest, authored.Digest)
	}
	seededJSON, err := json.Marshal(seeded)
	if err != nil {
		t.Fatal(err)
	}
	authoredJSON, err := json.Marshal(authored)
	if err != nil {
		t.Fatal(err)
	}
	if string(seededJSON) != string(authoredJSON) {
		t.Fatalf("canonical JSON differs:\nseed      %s\nauthoring %s", seededJSON, authoredJSON)
	}
}

func s3ParitySource(t *testing.T) integrationbatch.SourceRevision {
	t.Helper()
	source, err := integrationbatch.NewSourceRevision(integrationbatch.SourceRevisionInput{
		ArtifactID: "s3-seed", RevisionID: "r1", SourceID: "s3-seed", Provider: integrationbatch.ProviderS3,
		PollSeconds: 15, LeaseSeconds: 120, ProcessSeconds: 60, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &integrationbatch.S3Policy{
			Endpoint: "objects.example.test:443", Region: "us-east-1", Bucket: "adt-drop",
			InputPrefix: "incoming", ArchivePrefix: "archive", UseTLS: true,
			AccessKeyBinding: "s3-seed-access-key", SecretAccessKeyBinding: "s3-seed-secret-key",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestDefinitionAuthoringParity_SeedAndEditorAuthorTheSameBytes(t *testing.T) {
	for _, test := range []struct {
		name   string
		source func(t *testing.T) integrationbatch.SourceRevision
	}{
		{name: "sftp", source: nil},
		{name: "s3", source: s3ParitySource},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateSeedEnv(t)
			fixture := newSeedFixture(t)
			if test.source != nil {
				fixture.source = test.source(t)
				fixture.sourcePath = writeSeedJSON(t, fixture.dir, "source-"+test.name+".json", fixture.source)
			}
			assertParity(t, fixture)
		})
	}
}

func assertParity(t *testing.T, fixture seedFixture) {
	t.Helper()
	seeded := seedPathDefinition(t, fixture)
	authored := apiPathDefinition(t, fixture)
	assertSameDefinition(t, seeded, authored)
	if err := authored.ValidateForDeployment(); err != nil {
		t.Fatalf("authored definition is not deployable: %v", err)
	}

	// Negative control: one destination class changed, same everything else.
	// Both paths must move, and move to the same digest.
	sandbox, err := integrationdestination.NewRevision(integrationdestination.RevisionInput{
		ArtifactID: fixture.destination.ArtifactID, RevisionID: fixture.destination.RevisionID,
		DestinationID: fixture.destination.DestinationID, Class: integration.DestinationClassSandbox,
		Transport: fixture.destination.Transport, FHIR: fixture.destination.FHIR, Identity: fixture.destination.Identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture
	changed.destination = sandbox
	changed.destPath = writeSeedJSON(t, fixture.dir, "destination-sandbox.json", sandbox)
	seededChanged := seedPathDefinition(t, changed)
	authoredChanged := apiPathDefinition(t, changed)
	if seededChanged.Digest == seeded.Digest || authoredChanged.Digest == authored.Digest {
		t.Fatalf("a destination class change did not move the digest: seed %s -> %s, authoring %s -> %s",
			seeded.Digest, seededChanged.Digest, authored.Digest, authoredChanged.Digest)
	}
	assertSameDefinition(t, seededChanged, authoredChanged)
}
