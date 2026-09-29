//go:build integration

package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func authoringCaller(roles ...string) context.Context {
	return requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID: testTenant,
		Principal: integration.Principal{
			ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: roles,
		},
	})
}

type catalogFixture struct {
	Kind           connection.Kind             `json:"kind"`
	Spec           json.RawMessage             `json:"spec"`
	SecretBindings []integration.SecretBinding `json:"secret_bindings"`
}

// compileFixture declares and compiles one connection from the connection
// catalog's own testdata, returning its revision.
func compileFixture(t *testing.T, service *connection.Service, ctx context.Context, kind connection.Kind, id string) connection.Revision {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "connection", "testdata", string(kind)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture catalogFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	direction, _ := kind.Direction()
	created, err := service.Create(ctx, connection.CreateRequest{
		ID: id, Direction: direction, Kind: kind, Name: "fixture " + id,
		Spec: fixture.Spec, SecretBindings: fixture.SecretBindings, Reason: "declare " + id,
	})
	if err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	result, err := service.Compile(ctx, connection.CommandRequest{ID: id, ExpectedVersion: created.Version, Reason: "compile " + id})
	if err != nil || result.Revision == nil {
		t.Fatalf("compile %s: %v %v", id, err, result.Problems)
	}
	return *result.Revision
}

func problemCodes(problems []connection.Problem) []string {
	codes := make([]string, 0, len(problems))
	for _, problem := range problems {
		codes = append(codes, problem.Code)
	}
	return codes
}

// The definition editor end to end against PostgreSQL, composed as serve
// composes it: the lifecycle catalog with the mode-dispatching validator over
// CatalogFacts, the connection catalog, and the golden static registry. Compiled MLLP and
// FHIR connections plus registry entry adt-east become a draft; the
// pre-flight refuses an unbound secret, a destination the workflow does not
// deliver to, and a taken revision ID without writing; STATIC fails while no
// replica mounts the source and passes once one reports it; a stale version is
// a conflict; approve and publish produce the approval event and a
// digest-verified release; REAL is refused for a source this replica does not
// mount, SKIP with a short reason is refused, and a caller without the
// deployment grant cannot write.
func TestDefinitionAuthoringPostgres_EditorAuthorsValidatesApprovesAndPublishes(t *testing.T) {
	ctx := t.Context()
	db := authoringDB(t)
	store, err := connection.NewPostgresStore(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// The same facts serve's STATIC check reads, bound late as serve binds them.
	var facts CatalogFacts
	validators := Validators{Skip: SkipValidator(), Static: StaticValidator(facts.Checks())}
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{ValidateConnection: validators.Func()})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connections, err := connection.NewService(store, catalog, nil, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	facts.Bind(connections, nil)
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	static, err := registry.DecodeStaticRegistry(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	proven, err := NewRegistry(testTenant, static)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(Config{Catalog: catalog, Revisions: connections, Registry: proven, TenantID: testTenant})
	if err != nil {
		t.Fatal(err)
	}

	writer := authoringCaller(ReadRole, WriteRole)
	source := compileFixture(t, connections, writer, connection.KindMLLP, "adt-mllp")
	primary := compileFixture(t, connections, writer, connection.KindFHIR, "fhir-primary")
	other := compileFixture(t, connections, writer, connection.KindFHIR, "fhir-other")

	artifacts, err := service.RegistryArtifacts(writer)
	if err != nil {
		t.Fatal(err)
	}
	var adtEast *RegistryArtifact
	for index := range artifacts {
		if artifacts[index].IntegrationID == "adt-east" {
			adtEast = &artifacts[index]
		}
	}
	if adtEast == nil {
		t.Fatalf("registry artifacts %+v lack adt-east", artifacts)
	}

	reference := func(provider integration.SecretProviderKind, key string) integration.SecretReference {
		return integration.SecretReference{Provider: provider, Key: key}
	}
	input := DraftInput{
		DefinitionID: "adt-east-mllp", RevisionID: "v1",
		Source:       RevisionRef{ArtifactID: source.ArtifactID, RevisionID: source.RevisionID},
		Destinations: []RevisionRef{{ArtifactID: primary.ArtifactID, RevisionID: primary.RevisionID}},
		Profile:      adtEast.Profile, Workflow: adtEast.Workflow,
		SecretBindings: []integration.SecretBinding{
			{Name: "mllp-server-cert", Reference: reference(integration.SecretProviderFile, "mllp/server-cert.pem")},
			{Name: "mllp-server-key", Reference: reference(integration.SecretProviderFile, "mllp/server-key.pem")},
			{Name: "mllp-client-ca", Reference: reference(integration.SecretProviderFile, "mllp/client-ca.pem")},
			{Name: "fhir-token", Reference: reference(integration.SecretProviderEnvironment, "FHIR_PRIMARY_TOKEN")},
			{Name: "fhir-ca", Reference: reference(integration.SecretProviderFile, "destinations/fhir-ca.pem")},
		},
	}

	// Pre-flight refusals write nothing.
	unbound := input
	unbound.SecretBindings = input.SecretBindings[1:]
	wrongDestination := input
	wrongDestination.Destinations = []RevisionRef{{ArtifactID: other.ArtifactID, RevisionID: other.RevisionID}}
	for _, test := range []struct {
		name  string
		input DraftInput
		code  string
	}{
		{"unbound secret", unbound, connection.CodeUnboundSecret},
		{"workflow destination missing", wrongDestination, CodeWorkflowDestinationMissing},
	} {
		result, err := service.CreateDraft(writer, test.input, "author the east MLLP definition")
		if err != nil || result.Definition != nil || !contains(problemCodes(result.Problems), test.code) {
			t.Fatalf("%s: result %+v, %v; want problem %s", test.name, result, err, test.code)
		}
	}
	if rows, err := catalog.ListDefinitions(ctx, testTenant, true, 10); err != nil || len(rows) != 0 {
		t.Fatalf("a refused draft wrote %d rows (%v)", len(rows), err)
	}

	problems, err := service.Check(writer, input)
	if err != nil || connection.HasBlocking(problems) {
		t.Fatalf("check = %+v, %v", problems, err)
	}
	created, err := service.CreateDraft(writer, input, "author the east MLLP definition")
	if err != nil || created.Definition == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	definition := created.Definition
	if definition.Snapshot.State != integration.DeploymentStateDraft || definition.Snapshot.Version != 1 ||
		definition.Revision.Source.Digest != source.Digest || definition.Revision.Source.SourceID != "adt-east" ||
		definition.Revision.Deployment.ConnectionValidation.MaxAgeSeconds != DefaultValidationMaxAgeSeconds {
		t.Fatalf("draft = %+v", definition)
	}
	// Identical content is the stored revision (the seed's resume rule);
	// different content under the same ID is refused.
	again, err := service.CreateDraft(writer, input, "author the east MLLP definition")
	if err != nil || again.Definition == nil || again.Definition.Revision.Digest != definition.Revision.Digest {
		t.Fatalf("identical second create = %+v, %v", again, err)
	}
	changed := input
	changed.SecretBindings = append([]integration.SecretBinding(nil), input.SecretBindings...)
	changed.SecretBindings[0].Reference.Key = "mllp/another-cert.pem"
	conflict, err := service.CreateDraft(writer, changed, "author the east MLLP definition")
	if err != nil || conflict.Definition != nil || !contains(problemCodes(conflict.Problems), CodeAlreadyExists) {
		t.Fatalf("changed second create = %+v, %v", conflict, err)
	}

	command := Command{DefinitionID: "adt-east-mllp", RevisionID: "v1", ExpectedVersion: 1, Reason: "static check of the east listener"}
	validated, err := service.Validate(writer, command, ModeStatic)
	if err != nil || validated.Snapshot.State != integration.DeploymentStateDraft || validated.Validation == nil ||
		validated.Validation.Passed || !reflect.DeepEqual(validated.Validation.Codes, []string{CodeStatic, CodeSourceNotMounted, CodeBindingsNotChecked}) {
		t.Fatalf("unmounted static = %+v, %v", validated, err)
	}
	if err := store.UpsertObservation(ctx, connection.Observation{
		TenantID: testTenant, ReplicaID: "replica-a", Adapter: connection.AdapterMLLP, DefinitionID: "adt-east-mllp",
		ArtifactID: source.ArtifactID, RevisionID: source.RevisionID, Digest: source.Digest,
	}); err != nil {
		t.Fatal(err)
	}
	command.ExpectedVersion = validated.Snapshot.Version
	validated, err = service.Validate(writer, command, ModeStatic)
	if err != nil || validated.Snapshot.State != integration.DeploymentStateValidated || !validated.Validation.Passed ||
		!reflect.DeepEqual(validated.Validation.Codes, []string{CodeStatic, CodeSourceMounted, CodeBindingsNotChecked}) {
		t.Fatalf("mounted static = %+v, %v", validated, err)
	}

	if _, err := service.Validate(writer, command, ModeReal); !errors.Is(err, ErrRealUnavailable) {
		t.Fatalf("real for an unmounted batch source = %v", err)
	}
	short := command
	short.ExpectedVersion = validated.Snapshot.Version
	short.Reason = "too short"
	if _, err := service.Validate(writer, short, ModeSkip); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("skip with a short reason = %v", err)
	}
	if _, err := service.Approve(authoringCaller(ReadRole), Command{
		DefinitionID: "adt-east-mllp", RevisionID: "v1", ExpectedVersion: validated.Snapshot.Version, Reason: "approve",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("approve without the deployment grant = %v", err)
	}
	if _, err := service.Approve(writer, Command{
		DefinitionID: "adt-east-mllp", RevisionID: "v1", ExpectedVersion: 1, Reason: "approve",
	}); !errors.Is(err, lifecycle.ErrVersionConflict) {
		t.Fatalf("approve at a stale version = %v", err)
	}
	approved, err := service.Approve(writer, Command{
		DefinitionID: "adt-east-mllp", RevisionID: "v1", ExpectedVersion: validated.Snapshot.Version, Reason: "approve the east listener",
	})
	if err != nil || approved.Snapshot.State != integration.DeploymentStateApproved || approved.Approval == nil ||
		approved.Approval.Audit.Reason != "approve the east listener" {
		t.Fatalf("approve = %+v, %v", approved, err)
	}
	published, err := service.Publish(writer, Command{
		DefinitionID: "adt-east-mllp", RevisionID: "v1", ExpectedVersion: approved.Snapshot.Version, Reason: "publish the east listener",
	})
	if err != nil || published.Snapshot.State != integration.DeploymentStatePublished || published.Release == nil ||
		published.Release.ApprovalEventID != approved.Approval.ID || published.Release.ValidationID != validated.Validation.ID {
		t.Fatalf("publish = %+v, %v", published, err)
	}
	rows, err := service.List(authoringCaller(ReadRole), false)
	if err != nil || len(rows) != 1 || rows[0].Revision.Digest != definition.Revision.Digest {
		t.Fatalf("list = %+v, %v", rows, err)
	}
	// The definition now appears in both connections' references.
	used, err := connections.Get(writer, source.ArtifactID)
	if err != nil || len(used.References) != 1 || used.References[0].State != string(integration.DeploymentStatePublished) {
		t.Fatalf("source references = %+v, %v", used.References, err)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
