package authoring

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testTenant = "tenant-a"

func testDigest(fill byte) string {
	digest := make([]byte, 64)
	for index := range digest {
		digest[index] = fill
	}
	return "sha256:" + string(digest)
}

func testPrincipal() integration.Principal {
	return integration.Principal{
		ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc",
		Roles: []string{"integration.operator", "integration.deployment.operator"},
	}
}

// testDefinition builds a deployable definition with synthetic refs: the
// catalog never resolves profile or workflow bytes.
func testDefinition(t *testing.T, definitionID, sourceArtifactID string) integration.IntegrationDefinitionRevision {
	t.Helper()
	revision, err := BuildDefinition(Draft{
		DefinitionID: definitionID, RevisionID: "v1", TenantID: testTenant,
		Source: Source{
			Ref:      integration.ArtifactRevisionRef{ArtifactID: sourceArtifactID, RevisionID: "1", Digest: testDigest('a')},
			SourceID: sourceArtifactID, Kind: SourceKindMLLP,
		},
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: testDigest('b')},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "1", Digest: testDigest('c')},
		Destinations: []Destination{{Ref: integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "1", Digest: testDigest('d')},
			Class:               integration.DestinationClassSandbox,
		}}},
		Policy:     DefaultPolicy(),
		Deployment: DefaultDeploymentPolicy(0),
		Created: integration.AuditEnvelope{
			TenantID: testTenant, Principal: testPrincipal(), Reason: "author a test definition",
			OccurredAt: time.Now().UTC().Truncate(time.Second),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}
