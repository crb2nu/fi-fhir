package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func testDigest(ch byte) string {
	return "sha256:" + strings.Repeat(string(ch), 64)
}

func testArtifact(id, revision string, digestByte byte) integration.ArtifactRevisionRef {
	return integration.ArtifactRevisionRef{
		ArtifactID: id,
		RevisionID: revision,
		Digest:     testDigest(digestByte),
	}
}

func testHumanPrincipal() integration.Principal {
	return integration.Principal{
		ID:         "engineer-42",
		Kind:       integration.PrincipalKindHuman,
		AuthMethod: "oidc",
		Roles:      []string{"integration_engineer"},
	}
}

func testServicePrincipal() integration.Principal {
	return integration.Principal{
		ID:         "mllp-adapter",
		Kind:       integration.PrincipalKindService,
		AuthMethod: "mtls",
		SourceID:   "adt-east",
	}
}

func validRevisionInput() integration.IntegrationDefinitionRevisionInput {
	return integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "adt-to-fhir",
		RevisionID:   "rev-001",
		TenantID:     "acme-health",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: testArtifact("source-adt-east", "source-rev-001", '1'),
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  testArtifact("strict-adt-profile", "profile-rev-001", '2'),
		Workflow: testArtifact("adt-fhir-workflow", "workflow-rev-001", '3'),
		Destinations: []integration.DestinationRevisionRef{
			{
				ArtifactRevisionRef: testArtifact("fhir-r4-primary", "destination-rev-001", '4'),
				Class:               integration.DestinationClassProduction,
			},
			{
				ArtifactRevisionRef: testArtifact("fhir-r4-sandbox", "destination-rev-001", '5'),
				Class:               integration.DestinationClassSandbox,
			},
		},
		SecretBindings: []integration.SecretBinding{
			{
				Name: "fhir_oauth_client_secret",
				Reference: integration.SecretReference{
					Provider: integration.SecretProviderKubernetes,
					Key:      "fi-fhir/fhir-client-secret",
					Version:  "1",
				},
			},
		},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention: integration.RawRetentionPolicy{
				Mode: integration.RawRetentionModeEphemeral,
			},
		},
		Created: integration.AuditEnvelope{
			TenantID:   "acme-health",
			Principal:  testHumanPrincipal(),
			Reason:     "Golden Path 001 foundation fixture",
			OccurredAt: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestMinimalIntegrationRevisionFixtureValid(t *testing.T) {
	fixture, err := os.Open("../../testdata/golden/integration/adt-http/integration-revision.json")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fixture.Close()

	revision, err := integration.DecodeIntegrationDefinitionRevision(fixture)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("validate fixture: %v", err)
	}

	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	for _, forbidden := range []string{"plaintext-secret-sentinel", "MSH|^~\\&"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("fixture contract leaked forbidden value %q", forbidden)
		}
	}
}

func TestIntegrationRevisionRequiresBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*integration.IntegrationDefinitionRevisionInput)
	}{
		{name: "definition", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.DefinitionID = "" }},
		{name: "revision", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.RevisionID = "" }},
		{name: "tenant", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.TenantID = "" }},
		{name: "source", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Source.SourceID = "" }},
		{name: "format", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Format = events.FormatUnknown }},
		{name: "profile", mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
			input.Profile = integration.ArtifactRevisionRef{}
		}},
		{name: "workflow", mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
			input.Workflow = integration.ArtifactRevisionRef{}
		}},
		{name: "destination", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Destinations = nil }},
		{name: "classification", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Policy.Classification = "" }},
		{name: "audit tenant", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Created.TenantID = "" }},
		{name: "audit actor", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Created.Principal.ID = "" }},
		{name: "audit time", mutate: func(input *integration.IntegrationDefinitionRevisionInput) { input.Created.OccurredAt = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validRevisionInput()
			tt.mutate(&input)
			if _, err := integration.NewIntegrationDefinitionRevision(input); err == nil {
				t.Fatalf("expected %s validation failure", tt.name)
			}
		})
	}
}

func TestArtifactRevisionDigestFormat(t *testing.T) {
	for _, digest := range []string{
		"",
		"sha256:abc",
		"sha256:" + strings.Repeat("A", 64),
		"md5:" + strings.Repeat("a", 32),
	} {
		t.Run(digest, func(t *testing.T) {
			input := validRevisionInput()
			input.Profile.Digest = digest
			if _, err := integration.NewIntegrationDefinitionRevision(input); err == nil {
				t.Fatalf("expected invalid digest %q to fail", digest)
			}
		})
	}
}

func TestIntegrationRevisionRejectsEmbeddedSecretValues(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/integration/adt-http/integration-revision.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	withSecret := bytes.Replace(
		raw,
		[]byte(`"version": "1"`),
		[]byte(`"version": "1", "value": "plaintext-secret-sentinel"`),
		1,
	)
	if bytes.Equal(raw, withSecret) {
		t.Fatal("fixture replacement did not add secret value")
	}
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withSecret)); err == nil {
		t.Fatal("expected inline secret value to fail strict decoding")
	}

	withUnknown := bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"unexpected\": true,\n"), 1)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withUnknown)); err == nil {
		t.Fatal("expected unknown top-level field to fail strict decoding")
	}

	withTrailingValue := append(append([]byte(nil), raw...), []byte(` {"second":true}`)...)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withTrailingValue)); err == nil {
		t.Fatal("expected a trailing JSON value to fail strict decoding")
	}

	withDuplicateTenant := bytes.Replace(
		raw,
		[]byte(`"tenant_id": "acme-health",`),
		[]byte(`"tenant_id": "acme-health", "tenant_id": "acme-health",`),
		1,
	)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withDuplicateTenant)); err == nil {
		t.Fatal("expected a duplicate JSON key to fail strict decoding")
	}
	withDuplicateProvider := bytes.Replace(
		raw,
		[]byte(`"provider": "k8s",`),
		[]byte(`"provider": "k8s", "provider": "k8s",`),
		1,
	)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withDuplicateProvider)); err == nil {
		t.Fatal("expected a nested duplicate JSON key to fail strict decoding")
	}
	withNonCanonicalTenant := bytes.Replace(
		raw,
		[]byte(`"tenant_id": "acme-health",`),
		[]byte(`"TENANT_ID": "acme-health",`),
		1,
	)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withNonCanonicalTenant)); err == nil {
		t.Fatal("expected noncanonical JSON key spelling to fail strict decoding")
	}
	withSemanticDuplicate := bytes.Replace(
		raw,
		[]byte(`"tenant_id": "acme-health",`),
		[]byte(`"tenant_id": "acme-health", "TENANT_ID": "acme-health",`),
		1,
	)
	if _, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(withSemanticDuplicate)); err == nil {
		t.Fatal("expected case-variant semantic duplicate to fail strict decoding")
	}
}

func TestIntegrationRevisionRejectsDuplicateBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*integration.IntegrationDefinitionRevisionInput)
	}{
		{
			name: "destination",
			mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
				input.Destinations = append(input.Destinations, input.Destinations[0])
			},
		},
		{
			name: "destination artifact with another revision",
			mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
				duplicate := input.Destinations[0]
				duplicate.RevisionID = "destination-rev-002"
				duplicate.Digest = testDigest('9')
				duplicate.Class = integration.DestinationClassSandbox
				input.Destinations = append(input.Destinations, duplicate)
			},
		},
		{
			name: "secret",
			mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
				input.SecretBindings = append(input.SecretBindings, input.SecretBindings[0])
			},
		},
		{
			name: "principal role",
			mutate: func(input *integration.IntegrationDefinitionRevisionInput) {
				input.Created.Principal.Roles = append(input.Created.Principal.Roles, input.Created.Principal.Roles[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validRevisionInput()
			tt.mutate(&input)
			if _, err := integration.NewIntegrationDefinitionRevision(input); err == nil {
				t.Fatalf("expected duplicate %s to fail", tt.name)
			}
		})
	}
}

func TestIntegrationRevisionGoldenDigestIsStable(t *testing.T) {
	revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
	if err != nil {
		t.Fatalf("construct revision: %v", err)
	}
	const want = "sha256:b0de3dec889161d4af95f2bc51fa77bbb6b2adababd4eb7dd491fbec60fbc925"
	if revision.Digest != want {
		t.Fatalf("golden digest changed: got %s want %s", revision.Digest, want)
	}
}

func TestIntegrationRevisionConstructorCopiesInputs(t *testing.T) {
	input := validRevisionInput()
	revision, err := integration.NewIntegrationDefinitionRevision(input)
	if err != nil {
		t.Fatalf("construct revision: %v", err)
	}

	input.Destinations[0].ArtifactID = "mutated-destination"
	input.SecretBindings[0].Name = "mutated-secret"
	input.Created.Principal.Roles[0] = "mutated-role"

	if got := revision.Destinations[0].ArtifactID; got != "fhir-r4-primary" {
		t.Fatalf("destination alias leaked into revision: %q", got)
	}
	if got := revision.SecretBindings[0].Name; got != "fhir_oauth_client_secret" {
		t.Fatalf("secret binding alias leaked into revision: %q", got)
	}
	if got := revision.Created.Principal.Roles[0]; got != "integration_engineer" {
		t.Fatalf("principal role alias leaked into revision: %q", got)
	}
}

func TestIntegrationRevisionDigestDetectsMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*integration.IntegrationDefinitionRevision)
	}{
		{name: "definition", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.DefinitionID = "adt-to-fhir-copy" }},
		{name: "revision", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.RevisionID = "rev-002" }},
		{name: "parent", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.ParentRevisionID = "rev-000" }},
		{name: "tenant", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.TenantID = "other-tenant"
			revision.Created.TenantID = "other-tenant"
		}},
		{name: "format", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.Format = events.FormatCSV }},
		{name: "source artifact", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Source.ArtifactID = "source-adt-west"
		}},
		{name: "source revision", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Source.RevisionID = "source-rev-002"
		}},
		{name: "source digest", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.Source.Digest = testDigest('a') }},
		{name: "source", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.Source.SourceID = "other-source" }},
		{name: "profile artifact", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Profile.ArtifactID = "strict-adt-profile-copy"
		}},
		{name: "profile revision", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Profile.RevisionID = "profile-rev-002"
		}},
		{name: "profile digest", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.Profile.Digest = testDigest('b') }},
		{name: "workflow artifact", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Workflow.ArtifactID = "adt-fhir-workflow-copy"
		}},
		{name: "workflow revision", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Workflow.RevisionID = "workflow-rev-002"
		}},
		{name: "workflow digest", mutate: func(revision *integration.IntegrationDefinitionRevision) { revision.Workflow.Digest = testDigest('c') }},
		{name: "destination artifact", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Destinations[0].ArtifactID = "fhir-r4-secondary"
		}},
		{name: "destination revision", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Destinations[0].RevisionID = "destination-rev-002"
		}},
		{name: "destination digest", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Destinations[0].Digest = testDigest('d')
		}},
		{name: "destination class", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Destinations[0].Class = integration.DestinationClassSandbox
		}},
		{name: "secret name", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.SecretBindings[0].Name = "other_binding"
		}},
		{name: "secret provider", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.SecretBindings[0].Reference.Provider = integration.SecretProviderVault
		}},
		{name: "secret key", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.SecretBindings[0].Reference.Key = "other-secret"
		}},
		{name: "secret version", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.SecretBindings[0].Reference.Version = "2"
		}},
		{name: "retention", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Policy.RawRetention = integration.RawRetentionPolicy{
				Mode:            integration.RawRetentionModeEncrypted,
				TTLSeconds:      3600,
				Purpose:         "incident replay",
				StorageRevision: ptr(testArtifact("encrypted-raw-store", "storage-rev-001", '7')),
				EncryptionKey: &integration.SecretReference{
					Provider: integration.SecretProviderKubernetes,
					Key:      "fi-fhir/raw-retention-key",
				},
				AuthorizedBy:        testHumanPrincipal(),
				AccessAuditRequired: true,
			}
		}},
		{name: "audit principal", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Created.Principal.ID = "engineer-43"
		}},
		{name: "audit role", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Created.Principal.Roles[0] = "publisher"
		}},
		{name: "audit reason", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Created.Reason = "approved corrected mapping"
		}},
		{name: "audit timestamp", mutate: func(revision *integration.IntegrationDefinitionRevision) {
			revision.Created.OccurredAt = revision.Created.OccurredAt.Add(time.Second)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
			if err != nil {
				t.Fatalf("construct revision: %v", err)
			}
			tt.mutate(&revision)
			err = revision.Validate()
			if err == nil {
				t.Fatalf("expected %s mutation to invalidate digest", tt.name)
			}
			var validationErr *integration.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected structured validation error, got %T: %v", err, err)
			}
			digestMismatch := false
			for _, violation := range validationErr.Violations {
				if violation.Code == "DIGEST_MISMATCH" {
					digestMismatch = true
					break
				}
			}
			if !digestMismatch {
				t.Fatalf("%s mutation failed without a digest mismatch: %v", tt.name, err)
			}
		})
	}
}

func TestIntegrationRevisionDigestProtectsCreationTimestamp(t *testing.T) {
	first := validRevisionInput()
	second := validRevisionInput()
	second.Created.OccurredAt = second.Created.OccurredAt.Add(24 * time.Hour)

	firstRevision, err := integration.NewIntegrationDefinitionRevision(first)
	if err != nil {
		t.Fatalf("construct first revision: %v", err)
	}
	secondRevision, err := integration.NewIntegrationDefinitionRevision(second)
	if err != nil {
		t.Fatalf("construct second revision: %v", err)
	}
	if firstRevision.Digest == secondRevision.Digest {
		t.Fatal("creation timestamp did not change revision digest")
	}

	firstRevision.Created.OccurredAt = firstRevision.Created.OccurredAt.Add(time.Second)
	if err := firstRevision.Validate(); err == nil {
		t.Fatal("creation timestamp mutation did not invalidate revision digest")
	}
}

func TestIntegrationRevisionDigestChangesWithBinding(t *testing.T) {
	first := validRevisionInput()
	second := validRevisionInput()
	second.Profile.RevisionID = "profile-rev-002"

	firstRevision, err := integration.NewIntegrationDefinitionRevision(first)
	if err != nil {
		t.Fatalf("construct first revision: %v", err)
	}
	secondRevision, err := integration.NewIntegrationDefinitionRevision(second)
	if err != nil {
		t.Fatalf("construct second revision: %v", err)
	}
	if firstRevision.Digest == secondRevision.Digest {
		t.Fatal("profile binding did not change semantic digest")
	}
}

func TestIntegrationRevisionDigestUsesSetOrdering(t *testing.T) {
	first := validRevisionInput()
	first.Created.Principal.Roles = []string{"publisher", "integration_engineer"}
	first.SecretBindings = append(first.SecretBindings, integration.SecretBinding{
		Name: "fhir_oauth_client_id",
		Reference: integration.SecretReference{
			Provider: integration.SecretProviderKubernetes,
			Key:      "fi-fhir/fhir-client-id",
		},
	})

	second := first
	second.Destinations = []integration.DestinationRevisionRef{first.Destinations[1], first.Destinations[0]}
	second.SecretBindings = []integration.SecretBinding{first.SecretBindings[1], first.SecretBindings[0]}
	second.Created.Principal.Roles = []string{"integration_engineer", "publisher"}

	firstRevision, err := integration.NewIntegrationDefinitionRevision(first)
	if err != nil {
		t.Fatalf("construct first revision: %v", err)
	}
	secondRevision, err := integration.NewIntegrationDefinitionRevision(second)
	if err != nil {
		t.Fatalf("construct reordered revision: %v", err)
	}
	if firstRevision.Digest != secondRevision.Digest {
		t.Fatalf("set ordering changed semantic digest: %s != %s", firstRevision.Digest, secondRevision.Digest)
	}
}

func TestEphemeralPolicyNormalizesEmptyAuthorizerRoles(t *testing.T) {
	first := validRevisionInput()
	second := validRevisionInput()
	second.Policy.RawRetention.AuthorizedBy.Roles = []string{}

	firstRevision, err := integration.NewIntegrationDefinitionRevision(first)
	if err != nil {
		t.Fatalf("construct nil-role revision: %v", err)
	}
	secondRevision, err := integration.NewIntegrationDefinitionRevision(second)
	if err != nil {
		t.Fatalf("construct empty-role revision: %v", err)
	}
	if firstRevision.Digest != secondRevision.Digest {
		t.Fatalf("zero-equivalent ephemeral policies diverged: %s != %s", firstRevision.Digest, secondRevision.Digest)
	}
}
