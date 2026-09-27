package connection

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// unreachableConnector makes every database call fail loudly, so a test that
// expects a refusal *before* the store can tell the two apart.
type unreachableConnector struct{}

var errUnreachableStore = errors.New("unit test: the connection store is never queried")

func (unreachableConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errUnreachableStore
}
func (unreachableConnector) Driver() driver.Driver { return unreachableDriver{} }

type unreachableDriver struct{}

func (unreachableDriver) Open(string) (driver.Conn, error) { return nil, errUnreachableStore }

func unitService(t *testing.T) *Service {
	t.Helper()
	db := sql.OpenDB(unreachableConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewPostgresStore(db, nil)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := NewService(store, nil, describedRuntime(), "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func callerContext(tenantID string, roles ...string) context.Context {
	return requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID: tenantID,
		Principal: integration.Principal{
			ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: roles,
		},
	})
}

func validCreate() CreateRequest {
	return CreateRequest{
		ID: "adt-mllp", Direction: DirectionSource, Kind: KindMLLP, Name: "ADT east MLLP",
		Spec: json.RawMessage(`{"source_id":"adt-east"}`), Reason: "declare the east listener",
	}
}

// TestServiceRefusesBeforeTheStore proves every refusal below happens at the
// authorization or request boundary: the store behind this service fails any
// query with errUnreachableStore, so reaching it would change the error.
func TestServiceRefusesBeforeTheStore(t *testing.T) {
	service := unitService(t)
	reader := callerContext("tenant-a", ReadRole)
	writer := callerContext("tenant-a", ReadRole, WriteRole)
	cases := []struct {
		name string
		call func() error
		want error
	}{
		{"list unauthenticated", func() error { _, err := service.List(context.Background(), ListFilter{}); return err }, ErrUnauthenticated},
		{"list another tenant's caller", func() error {
			_, err := service.List(callerContext("tenant-b", ReadRole), ListFilter{})
			return err
		}, ErrForbidden},
		{"list without the read role", func() error {
			_, err := service.List(callerContext("tenant-a", WriteRole), ListFilter{})
			return err
		}, ErrForbidden},
		{"get without the read role", func() error {
			_, err := service.Get(callerContext("tenant-a", "integration:preview"), "adt-mllp")
			return err
		}, ErrForbidden},
		{"create with the read role only", func() error { _, err := service.Create(reader, validCreate()); return err }, ErrForbidden},
		{"update with the read role only", func() error {
			_, err := service.Update(reader, UpdateRequest{ID: "adt-mllp", ExpectedVersion: 1, Reason: "r"})
			return err
		}, ErrForbidden},
		{"archive with the read role only", func() error {
			_, err := service.Archive(reader, CommandRequest{ID: "adt-mllp", ExpectedVersion: 1, Reason: "r"})
			return err
		}, ErrForbidden},
		{"compile with the read role only", func() error {
			_, err := service.Compile(reader, CommandRequest{ID: "adt-mllp", ExpectedVersion: 1, Reason: "r"})
			return err
		}, ErrForbidden},
		{"validate with the read role only", func() error {
			_, err := service.ValidateSpec(reader, ValidateRequest{Kind: KindKafka, Spec: json.RawMessage(`{}`)})
			return err
		}, ErrForbidden},
		{"write with the deployment role only", func() error {
			_, err := service.Create(callerContext("tenant-a", WriteRole), validCreate())
			return err
		}, ErrForbidden},
		{"list with an unknown direction", func() error {
			_, err := service.List(reader, ListFilter{Direction: "sideways"})
			return err
		}, ErrInvalidRequest},
		{"create with a mismatched direction", func() error {
			request := validCreate()
			request.Direction = DirectionDestination
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create with an unknown kind", func() error {
			request := validCreate()
			request.Kind = "smtp"
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create with a path in the id", func() error {
			request := validCreate()
			request.ID = "../adt"
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create without a reason", func() error {
			request := validCreate()
			request.Reason = "   "
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create with an oversized reason", func() error {
			request := validCreate()
			request.Reason = strings.Repeat("r", MaxReasonBytes+1)
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create with a control character in the name", func() error {
			request := validCreate()
			request.Name = "ADT\x00east"
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"create with a spec that is not an object", func() error {
			request := validCreate()
			request.Spec = json.RawMessage(`["x"]`)
			_, err := service.Create(writer, request)
			return err
		}, ErrInvalidRequest},
		{"update without an expected version", func() error {
			_, err := service.Update(writer, UpdateRequest{ID: "adt-mllp", Reason: "r"})
			return err
		}, ErrInvalidRequest},
		{"compile without an expected version", func() error {
			_, err := service.Compile(writer, CommandRequest{ID: "adt-mllp", Reason: "r"})
			return err
		}, ErrInvalidRequest},
		{"validate an unknown kind", func() error {
			_, err := service.ValidateSpec(writer, ValidateRequest{Kind: "smtp", Spec: json.RawMessage(`{}`)})
			return err
		}, ErrInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if errors.Is(err, errUnreachableStore) {
				t.Fatal("the refusal happened after a store query")
			}
		})
	}
}

// TestServiceRefusesASecretValueBeforeItIsPersisted: a draft may be
// incomplete, but a spec key that looks like a value never reaches the store.
func TestServiceRefusesASecretValueBeforeItIsPersisted(t *testing.T) {
	service := unitService(t)
	writer := callerContext("tenant-a", ReadRole, WriteRole)
	request := CreateRequest{
		ID: "fhir-primary", Direction: DirectionDestination, Kind: KindFHIR, Name: "FHIR primary",
		Spec:   json.RawMessage(`{"destination_id":"fhir-primary","fhir":{"base_url":"https://fhir.example.org","token":"abc"}}`),
		Reason: "declare the FHIR server",
	}
	_, err := service.Create(writer, request)
	var specErr *SpecError
	if !errors.As(err, &specErr) || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want a SpecError that is an invalid request", err)
	}
	if !hasCode(specErr.Problems, CodeSecretValueForbidden, "fhir.token") {
		t.Fatalf("problems = %+v", specErr.Problems)
	}
	if strings.Contains(err.Error(), "abc") {
		t.Fatal("the refusal echoes the secret value")
	}

	request.Spec = json.RawMessage(`{"destination_id":"fhir-primary"}`)
	request.SecretBindings = []integration.SecretBinding{{
		Name: "fhir-ca", Reference: integration.SecretReference{Provider: integration.SecretProviderFile,
			Key: "-----BEGIN CERTIFICATE-----"},
	}}
	if _, err := service.Create(writer, request); !errors.As(err, &specErr) ||
		!hasCode(specErr.Problems, CodeSecretValueForbidden, "secret_bindings[0].key") {
		t.Fatalf("PEM in a binding key: error = %v", err)
	}
}

func TestValidateSpecReadsNothingAndReturnsAList(t *testing.T) {
	service := unitService(t)
	writer := callerContext("tenant-a", ReadRole, WriteRole)
	fixture := loadSpecFixture(t, KindKafka)
	problems, err := service.ValidateSpec(writer, ValidateRequest{Kind: KindKafka, Spec: fixture.specJSON(t)})
	if err != nil || problems == nil || len(problems) != 0 {
		t.Fatalf("problems = %#v, err = %v; want an empty non-nil list", problems, err)
	}
	problems, err = service.ValidateSpec(writer, ValidateRequest{Kind: KindKafka, Spec: json.RawMessage(`{"kafka":{}}`)})
	if err != nil || !hasCode(problems, CodeRequired, "kafka.topic") || !hasCode(problems, CodeRequired, "destination_id") {
		t.Fatalf("problems = %+v, err = %v", problems, err)
	}
}

func TestAuthorizeRuntimeRead(t *testing.T) {
	description := describedRuntime()
	if err := AuthorizeRuntimeRead(callerContext("tenant-a", ReadRole), description); err != nil {
		t.Fatalf("reader refused: %v", err)
	}
	for name, ctx := range map[string]context.Context{
		"unauthenticated": context.Background(),
		"no read role":    callerContext("tenant-a", "integration:preview"),
		"another tenant":  callerContext("tenant-b", ReadRole),
	} {
		if err := AuthorizeRuntimeRead(ctx, description); err == nil {
			t.Errorf("%s: engine runtime readable", name)
		}
	}
	if err := AuthorizeRuntimeRead(callerContext("tenant-a", ReadRole), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil description: %v", err)
	}
}

// fakeDefinitionCatalog serves definition revisions by (definition, revision).
type fakeDefinitionCatalog struct {
	snapshots []lifecycle.Snapshot
	documents map[string]string
	listErr   error
	loads     int
	lists     int
}

func (f *fakeDefinitionCatalog) ListSnapshots(_ context.Context, _ string, limit int) ([]lifecycle.Snapshot, error) {
	f.lists++
	if limit != maxDefinitionSnapshots {
		return nil, errors.New("unexpected snapshot bound")
	}
	return f.snapshots, f.listErr
}

func (f *fakeDefinitionCatalog) LoadDefinitionRevision(_ context.Context, _, definitionID, revisionID string) ([]byte, error) {
	f.loads++
	document, ok := f.documents[definitionID+"/"+revisionID]
	if !ok {
		return nil, lifecycle.ErrNotFound
	}
	return []byte(document), nil
}

func snapshot(definitionID, revisionID string, state integration.DeploymentState, health integration.DeploymentHealthStatus) lifecycle.Snapshot {
	return lifecycle.Snapshot{
		DefinitionRevision: integration.ArtifactRevisionRef{ArtifactID: definitionID, RevisionID: revisionID},
		State:              state, Health: health,
	}
}

func TestReferencesByDigestMatchesSourcesAndDestinations(t *testing.T) {
	source := "sha256:" + strings.Repeat("a", 64)
	destination := "sha256:" + strings.Repeat("b", 64)
	catalog := &fakeDefinitionCatalog{
		snapshots: []lifecycle.Snapshot{
			snapshot("adt-http", "rev-1", integration.DeploymentStateDeployed, integration.DeploymentHealthHealthy),
			snapshot("adt-http", "rev-2", integration.DeploymentStateDraft, integration.DeploymentHealthUnknown),
			snapshot("gone", "rev-1", integration.DeploymentStateDraft, integration.DeploymentHealthUnknown),
		},
		documents: map[string]string{
			"adt-http/rev-1": `{"source":{"digest":"` + source + `"},"destinations":[{"digest":"` + destination + `"}]}`,
			"adt-http/rev-2": `{"source":{"digest":"sha256:` + strings.Repeat("c", 64) + `"},"destinations":[{"digest":"` + destination + `"}]}`,
		},
	}
	service := unitService(t)
	service.catalog = catalog
	references, err := service.referencesByDigest(context.Background(), "tenant-a", true)
	if err != nil {
		t.Fatalf("referencesByDigest: %v", err)
	}
	if got := references[source]; len(got) != 1 || got[0] != (Reference{
		DefinitionID: "adt-http", RevisionID: "rev-1", Digest: source, State: "deployed", Health: "healthy",
	}) {
		t.Fatalf("source references = %+v", got)
	}
	if got := references[destination]; len(got) != 2 || got[1].RevisionID != "rev-2" || got[1].State != "draft" {
		t.Fatalf("destination references = %+v", got)
	}

	projected := project(Draft{ID: "dest"}, nil, []RevisionDigest{
		{RevisionID: "2", Digest: destination}, {RevisionID: "1", Digest: "sha256:" + strings.Repeat("f", 64)},
	}, references, nil)
	if len(projected.References) != 2 || projected.Runtime.Mounted {
		t.Fatalf("projection = %+v", projected)
	}

	loads := catalog.loads
	if _, err := service.referencesByDigest(context.Background(), "tenant-a", false); err != nil || catalog.loads != loads {
		t.Fatal("references were read for a connection with no revisions")
	}
	catalog.listErr = lifecycle.ErrUnavailable
	if got, err := service.referencesByDigest(context.Background(), "tenant-a", true); err != nil || len(got) != 0 {
		t.Fatalf("an unavailable lifecycle catalog must read as no references: %v, %v", got, err)
	}
	catalog.listErr = errors.New("database is down")
	if _, err := service.referencesByDigest(context.Background(), "tenant-a", true); err == nil {
		t.Fatal("a lifecycle read failure was reported as no references")
	}
	service.catalog = nil
	if got, err := service.referencesByDigest(context.Background(), "tenant-a", true); err != nil || len(got) != 0 {
		t.Fatalf("no catalog must read as no references: %v, %v", got, err)
	}
}

func TestNewServiceFailsClosed(t *testing.T) {
	if _, err := NewService(nil, nil, nil, "tenant-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil store: %v", err)
	}
	db := sql.OpenDB(unreachableConnector{})
	defer func() { _ = db.Close() }()
	store, _ := NewPostgresStore(db, nil)
	if _, err := NewService(store, nil, nil, " tenant "); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bad tenant: %v", err)
	}
	var missing *Service
	if _, err := missing.List(callerContext("tenant-a", ReadRole), ListFilter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service: %v", err)
	}
}
