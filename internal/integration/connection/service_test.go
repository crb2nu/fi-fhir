package connection

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

	// Refused before the store too: a key the kind does not define, and a
	// malformed binding reference. unitService's store fails every query, so
	// reaching it would surface errUnreachableStore instead of a SpecError.
	for name, tc := range map[string]struct {
		spec     string
		bindings []integration.SecretBinding
		code     string
		path     string
	}{
		"unknown key": {`{"destination_id":"fhir-primary","fhir":{"base_url":"https://fhir.example.org","headers":{"x":"y"}}}`,
			nil, CodeUnknownField, "fhir.headers"},
		"binding provider": {`{"destination_id":"fhir-primary"}`,
			[]integration.SecretBinding{{Name: "fhir-token", Reference: integration.SecretReference{Provider: "keychain", Key: "k"}}},
			CodeInvalidEnum, "secret_bindings[0].provider"},
		"token in the base url": {`{"fhir":{"base_url":"https://fhir.example.org/r4?access_token=synthetic"}}`,
			nil, CodeSecretValueForbidden, "fhir.base_url"},
	} {
		request.Spec = json.RawMessage(tc.spec)
		request.SecretBindings = tc.bindings
		_, err := service.Create(writer, request)
		if !errors.As(err, &specErr) || !hasCode(specErr.Problems, tc.code, tc.path) || errors.Is(err, errUnreachableStore) {
			t.Fatalf("%s: error = %v, want %s at %s before any store query", name, err, tc.code, tc.path)
		}
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

// fakeDefinitionCatalog answers the one reference query from a fixed list and
// records what it was asked.
type fakeDefinitionCatalog struct {
	references []lifecycle.DigestReference
	err        error
	calls      int
	asked      []string
}

func (f *fakeDefinitionCatalog) ListDigestReferences(_ context.Context, _ string, digests []string) ([]lifecycle.DigestReference, error) {
	f.calls++
	f.asked = append([]string(nil), digests...)
	if f.err != nil {
		return nil, f.err
	}
	wanted := make(map[string]bool, len(digests))
	for _, digest := range digests {
		wanted[digest] = true
	}
	matched := make([]lifecycle.DigestReference, 0)
	for _, reference := range f.references {
		if wanted[reference.Digest] {
			matched = append(matched, reference)
		}
	}
	return matched, nil
}

func digestReference(definitionID, revisionID, digest string, state integration.DeploymentState, health integration.DeploymentHealthStatus) lifecycle.DigestReference {
	return lifecycle.DigestReference{DefinitionID: definitionID, RevisionID: revisionID, Digest: digest, State: state, Health: health}
}

// TestReferencesByDigestIsOneCatalogQuery: every reference of every asked
// digest comes back from one catalog call — no per-definition reads, no
// truncation — grouped by digest in the catalog's order.
func TestReferencesByDigestIsOneCatalogQuery(t *testing.T) {
	source := "sha256:" + strings.Repeat("a", 64)
	destination := "sha256:" + strings.Repeat("b", 64)
	catalog := &fakeDefinitionCatalog{}
	for index := range 250 {
		catalog.references = append(catalog.references, digestReference(
			fmt.Sprintf("definition-%03d", index), "rev-1", destination,
			integration.DeploymentStateDraft, integration.DeploymentHealthUnknown))
	}
	catalog.references = append(catalog.references,
		digestReference("adt-http", "rev-1", source, integration.DeploymentStateDeployed, integration.DeploymentHealthHealthy))
	service := unitService(t)
	service.catalog = catalog
	references, err := service.referencesByDigest(context.Background(), "tenant-a", []string{source, destination})
	if err != nil {
		t.Fatalf("referencesByDigest: %v", err)
	}
	if catalog.calls != 1 || len(catalog.asked) != 2 {
		t.Fatalf("catalog calls = %d asked %v, want one call for both digests", catalog.calls, catalog.asked)
	}
	if got := references[source]; len(got) != 1 || got[0] != (Reference{
		DefinitionID: "adt-http", RevisionID: "rev-1", Digest: source, State: "deployed", Health: "healthy",
	}) {
		t.Fatalf("source references = %+v", got)
	}
	if got := references[destination]; len(got) != 250 || got[249].DefinitionID != "definition-249" || got[0].State != "draft" {
		t.Fatalf("destination references = %d, want all 250 in catalog order", len(got))
	}

	projected := project(Draft{ID: "dest"}, nil, []RevisionDigest{
		{RevisionID: "2", Digest: destination}, {RevisionID: "1", Digest: "sha256:" + strings.Repeat("f", 64)},
	}, references, nil, nil)
	if len(projected.References) != 250 || projected.Runtime.Mounted {
		t.Fatalf("projection = %d references, mounted %v", len(projected.References), projected.Runtime.Mounted)
	}

	if _, err := service.referencesByDigest(context.Background(), "tenant-a", nil); err != nil || catalog.calls != 1 {
		t.Fatal("references were read for a connection with no revisions")
	}
	catalog.err = lifecycle.ErrUnavailable
	if got, err := service.referencesByDigest(context.Background(), "tenant-a", []string{source}); err != nil || len(got) != 0 {
		t.Fatalf("an unavailable lifecycle catalog must read as no references: %v, %v", got, err)
	}
	catalog.err = errors.New("database is down")
	if _, err := service.referencesByDigest(context.Background(), "tenant-a", []string{source}); err == nil {
		t.Fatal("a lifecycle read failure was reported as no references")
	}
	service.catalog = nil
	if got, err := service.referencesByDigest(context.Background(), "tenant-a", []string{source}); err != nil || len(got) != 0 {
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

func TestObservedFleetCountsFreshReplicasThatMountAConnection(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	interval := time.Minute
	source := "sha256:" + strings.Repeat("a", 64)
	older := "sha256:" + strings.Repeat("b", 64)
	other := "sha256:" + strings.Repeat("c", 64)
	observations := []Observation{
		// Fresh, mounts the newest revision.
		{ReplicaID: "host-1-1", Adapter: AdapterMLLP, Digest: source, HeartbeatAt: now.Add(-30 * time.Second)},
		{ReplicaID: "host-1-1", Adapter: AdapterBatch, HeartbeatAt: now.Add(-30 * time.Second)},
		// Fresh at exactly 3x the interval, mounts an older revision of it.
		{ReplicaID: "host-2-2", Adapter: AdapterMLLP, Digest: older, HeartbeatAt: now.Add(-3 * interval)},
		// Fresh, mounts something else.
		{ReplicaID: "host-3-3", Adapter: AdapterMLLP, Digest: other, HeartbeatAt: now},
		// Stale: mounted it once, stopped reporting. Counts nowhere.
		{ReplicaID: "host-4-4", Adapter: AdapterMLLP, Digest: source, HeartbeatAt: now.Add(-3*interval - time.Second)},
	}
	fleet := newObservedFleet(observations, now, interval)
	revisions := []RevisionDigest{{RevisionID: "2", Digest: source}, {RevisionID: "1", Digest: older}}
	if observed, total := fleet.replicas(revisions); observed != 2 || total != 3 {
		t.Fatalf("replicas = %d/%d, want 2/3", observed, total)
	}
	if observed, total := fleet.replicas(nil); observed != 0 || total != 3 {
		t.Fatalf("never-compiled connection = %d/%d, want 0/3", observed, total)
	}
	var none observedFleet
	if observed, total := none.replicas(revisions); observed != 0 || total != 0 {
		t.Fatalf("no observations = %d/%d, want 0/0", observed, total)
	}

	views := observationViews(observations, now, interval)
	var stale []string
	for _, view := range views {
		if view.Stale {
			stale = append(stale, view.ReplicaID)
		}
	}
	if len(views) != len(observations) || len(stale) != 1 || stale[0] != "host-4-4" {
		t.Fatalf("stale rows = %v of %d, want only host-4-4", stale, len(views))
	}
}

func TestServiceObservationsRequiresReadRole(t *testing.T) {
	service := unitService(t)
	if _, err := service.Observations(context.Background()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
	if _, err := service.Observations(callerContext("tenant-b", ReadRole)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other tenant: %v", err)
	}
	if _, err := service.Observations(callerContext("tenant-a", ReadRole)); !errors.Is(err, errUnreachableStore) {
		t.Fatalf("authorized caller must reach the store: %v", err)
	}
	service.SetObservationInterval(0)
	if service.observationInterval != DefaultObservationInterval {
		t.Fatalf("a zero interval replaced the default: %s", service.observationInterval)
	}
	service.SetObservationInterval(30 * time.Second)
	if service.observationInterval != 30*time.Second {
		t.Fatalf("interval = %s", service.observationInterval)
	}
}
