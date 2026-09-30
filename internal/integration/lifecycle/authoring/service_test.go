package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The authoring service over fakes: what Service.Check and Service.CreateDraft
// author from the editor's inputs, against revisions built by hand, so drift
// in assemble's defaults (policy, max age, retention, audit) fails here.

var serviceClock = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

type fakeRevisions struct {
	revisions map[string]connection.Revision
}

func (f *fakeRevisions) GetRevision(_ context.Context, artifactID, revisionID string) (connection.Revision, error) {
	revision, ok := f.revisions[artifactID+"/"+revisionID]
	if !ok {
		return connection.Revision{}, connection.ErrNotFound
	}
	return revision, nil
}

type fakeCatalog struct {
	mu        sync.Mutex
	rows      map[string]lifecycle.DefinitionRow
	creates   int
	validate  lifecycle.ConnectionValidatorFunc
	validated chan struct{}
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{rows: map[string]lifecycle.DefinitionRow{}}
}

func (c *fakeCatalog) CreateDraft(_ context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := revision.DefinitionID + "/" + revision.RevisionID
	if _, ok := c.rows[key]; ok {
		return lifecycle.Snapshot{}, lifecycle.ErrAlreadyExists
	}
	c.creates++
	snapshot := lifecycle.Snapshot{
		TenantID: revision.TenantID, DefinitionRevision: revision.Reference(),
		State: integration.DeploymentStateDraft, Version: 1, Health: integration.DeploymentHealthUnknown,
		Updated: revision.Created,
	}
	c.rows[key] = lifecycle.DefinitionRow{Snapshot: snapshot, Revision: revision}
	return snapshot, nil
}

func (c *fakeCatalog) GetSnapshot(_ context.Context, _, definitionID, revisionID string) (lifecycle.Snapshot, error) {
	row, err := c.GetDefinition(context.Background(), "", definitionID, revisionID)
	return row.Snapshot, err
}

func (c *fakeCatalog) GetDefinition(_ context.Context, _, definitionID, revisionID string) (lifecycle.DefinitionRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.rows[definitionID+"/"+revisionID]
	if !ok {
		return lifecycle.DefinitionRow{}, lifecycle.ErrNotFound
	}
	return row, nil
}

func (c *fakeCatalog) ListDefinitions(context.Context, string, bool, int) ([]lifecycle.DefinitionRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]lifecycle.DefinitionRow, 0, len(c.rows))
	for _, row := range c.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

func (c *fakeCatalog) GetValidation(context.Context, string) (lifecycle.ValidationRecord, error) {
	return lifecycle.ValidationRecord{}, lifecycle.ErrNotFound
}

func (c *fakeCatalog) GetRelease(context.Context, string) (lifecycle.ReleaseRecord, error) {
	return lifecycle.ReleaseRecord{}, lifecycle.ErrNotFound
}

func (c *fakeCatalog) ListEvents(context.Context, string, string, string) ([]lifecycle.EventRecord, error) {
	return nil, nil
}

// ValidateConnection calls the validator the way the catalog does, with the
// caller's context, and records nothing.
func (c *fakeCatalog) ValidateConnection(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	row, err := c.GetDefinition(ctx, command.TenantID, command.DefinitionID, command.RevisionID)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	if c.validate != nil {
		_, _ = c.validate(ctx, row.Revision)
	}
	if c.validated != nil {
		c.validated <- struct{}{}
	}
	return row.Snapshot, nil
}

func (c *fakeCatalog) Approve(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
}

func (c *fakeCatalog) Publish(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
}

func tenantCaller(tenantID string, roles ...string) context.Context {
	return requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID:  tenantID,
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: roles},
	})
}

func authorCaller() context.Context { return tenantCaller(testTenant, ReadRole, WriteRole) }

type catalogFixtureFile struct {
	Kind           connection.Kind             `json:"kind"`
	Spec           json.RawMessage             `json:"spec"`
	SecretBindings []integration.SecretBinding `json:"secret_bindings"`
}

// compiledFixture compiles one connection catalog testdata fixture with the
// catalog's own BuildRevision, so the document is the one the catalog stores.
func compiledFixture(t *testing.T, kind connection.Kind, id string) (connection.Revision, []integration.SecretBinding) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "connection", "testdata", string(kind)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture catalogFixtureFile
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	direction, _ := kind.Direction()
	revision, problems := connection.BuildRevision(connection.Draft{
		TenantID: testTenant, ID: id, Direction: direction, Kind: kind, Name: id,
		Spec: fixture.Spec, SecretBindings: fixture.SecretBindings, Version: 1,
	}, 1, integration.AuditEnvelope{TenantID: testTenant, Principal: testPrincipal(), Reason: "compile", OccurredAt: serviceClock})
	if revision == nil {
		t.Fatalf("compile %s: %v", id, problems)
	}
	return *revision, fixture.SecretBindings
}

func goldenRegistry(t *testing.T) *Registry {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	static, err := registry.DecodeStaticRegistry(file)
	if err != nil {
		t.Fatal(err)
	}
	proven, err := NewRegistry(testTenant, static)
	if err != nil {
		t.Fatal(err)
	}
	return proven
}

// The golden registry's adt-east pair, written out rather than read back.
var (
	goldenProfile = integration.ArtifactRevisionRef{
		ArtifactID: "profile-adt", RevisionID: "1",
		Digest: "sha256:79c8f575ae135f6d6c10d46fcb57c75a6094ac65c8ab6d10912e5050b26315d3",
	}
	goldenWorkflow = integration.ArtifactRevisionRef{
		ArtifactID: "workflow-adt", RevisionID: "workflow-version-1",
		Digest: "sha256:7e20cd2b8e591c507a313eb9a422318384d7e7b7c895798c2a264182dad688dc",
	}
)

type serviceFixture struct {
	service     *Service
	catalog     *fakeCatalog
	destination connection.Revision
	destBinds   []integration.SecretBinding
	sources     map[connection.Kind]connection.Revision
	sourceBinds map[connection.Kind][]integration.SecretBinding
}

func newServiceFixture(t *testing.T, config Config) serviceFixture {
	t.Helper()
	fixture := serviceFixture{
		catalog: newFakeCatalog(), sources: map[connection.Kind]connection.Revision{},
		sourceBinds: map[connection.Kind][]integration.SecretBinding{},
	}
	revisions := &fakeRevisions{revisions: map[string]connection.Revision{}}
	fixture.destination, fixture.destBinds = compiledFixture(t, connection.KindFHIR, "fhir-primary")
	revisions.revisions["fhir-primary/1"] = fixture.destination
	for _, kind := range []connection.Kind{connection.KindMLLP, connection.KindBatchSFTP, connection.KindBatchS3} {
		id := "src-" + strings.ReplaceAll(string(kind), "_", "-")
		fixture.sources[kind], fixture.sourceBinds[kind] = compiledFixture(t, kind, id)
		revisions.revisions[id+"/1"] = fixture.sources[kind]
	}
	config.Catalog, config.Revisions, config.Registry, config.TenantID = fixture.catalog, revisions, goldenRegistry(t), testTenant
	config.Clock = func() time.Time { return serviceClock.Add(400 * time.Millisecond) }
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	return fixture
}

func (f serviceFixture) input(kind connection.Kind) DraftInput {
	source := f.sources[kind]
	bindings := append(append([]integration.SecretBinding(nil), f.sourceBinds[kind]...), f.destBinds...)
	return DraftInput{
		DefinitionID: "def-" + source.ArtifactID, RevisionID: "v1",
		Source:       RevisionRef{ArtifactID: source.ArtifactID, RevisionID: "1"},
		Destinations: []RevisionRef{{ArtifactID: "fhir-primary", RevisionID: "1"}},
		Profile:      goldenProfile, Workflow: goldenWorkflow, SecretBindings: bindings,
	}
}

func problemCodesOf(problems []connection.Problem) []string {
	codes := make([]string, 0, len(problems))
	for _, problem := range problems {
		codes = append(codes, problem.Code+"@"+problem.Path)
	}
	return codes
}

func TestServiceCreateDraft_AuthorsTheHandBuiltRevision(t *testing.T) {
	fixture := newServiceFixture(t, Config{ValidationMaxAgeSeconds: 600})
	for _, test := range []struct {
		kind      connection.Kind
		sourceID  string
		retention *integration.RawRetentionPolicy
	}{
		{kind: connection.KindMLLP, sourceID: "adt-east"},
		{kind: connection.KindBatchSFTP, sourceID: "adt-west"},
		{kind: connection.KindBatchS3, sourceID: "adt-east", retention: &integration.RawRetentionPolicy{
			Mode: integration.RawRetentionModeEncrypted, TTLSeconds: 86400, Purpose: "replay after outage",
			StorageRevision: &integration.ArtifactRevisionRef{ArtifactID: "raw-store", RevisionID: "1", Digest: testDigest('e')},
			EncryptionKey:   &integration.SecretReference{Provider: integration.SecretProviderVault, Key: "fi-fhir/raw-key"},
		}},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			input := fixture.input(test.kind)
			input.RawRetention = test.retention
			problems, err := fixture.service.Check(authorCaller(), input)
			if err != nil || connection.HasBlocking(problems) {
				t.Fatalf("check = %v, %v", problemCodesOf(problems), err)
			}
			result, err := fixture.service.CreateDraft(authorCaller(), input, "author the definition")
			if err != nil || result.Definition == nil {
				t.Fatalf("create = %v, %v", problemCodesOf(result.Problems), err)
			}

			source := fixture.sources[test.kind]
			principal := integration.Principal{
				ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{ReadRole, WriteRole},
			}
			policy := integration.IntegrationPolicy{
				Classification: integration.DataClassificationPHI,
				RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
			}
			if test.retention != nil {
				policy.RawRetention = *test.retention
				policy.RawRetention.AuthorizedBy = principal
				policy.RawRetention.AccessAuditRequired = true
			}
			expected, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
				DefinitionID: input.DefinitionID, RevisionID: "v1", TenantID: testTenant,
				Source: integration.SourceRevisionRef{
					ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: source.ArtifactID, RevisionID: "1", Digest: source.Digest},
					SourceID:            test.sourceID,
				},
				Format:  events.FormatHL7v2,
				Profile: goldenProfile, Workflow: goldenWorkflow,
				Destinations: []integration.DestinationRevisionRef{{
					ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "1", Digest: fixture.destination.Digest},
					Class:               integration.DestinationClassProduction,
				}},
				SecretBindings: input.SecretBindings,
				Policy:         policy,
				Deployment: &integration.IntegrationDeploymentPolicy{
					ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 600},
					Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
					Health:               integration.HealthPolicy{StartupGraceSeconds: 5, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
					Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 10, MaxMessagesPerSecond: 100},
				},
				Created: integration.AuditEnvelope{TenantID: testTenant, Principal: principal, Reason: "author the definition", OccurredAt: serviceClock},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Definition.Revision.Digest; got != expected.Digest {
				t.Fatalf("authored digest %s, hand-built %s", got, expected.Digest)
			}
		})
	}
}

func TestServiceCheck_RefusesSecretMaterialInEveryReference(t *testing.T) {
	fixture := newServiceFixture(t, Config{})
	input := fixture.input(connection.KindMLLP)
	input.SecretBindings = append([]integration.SecretBinding(nil), input.SecretBindings...)
	input.SecretBindings[0].Reference.Key = "-----BEGIN PRIVATE KEY-----MIIEvQ"
	input.SecretBindings[1].Reference.Key = "a key with spaces"
	input.RawRetention = &integration.RawRetentionPolicy{
		Mode: integration.RawRetentionModeEncrypted, TTLSeconds: 60, Purpose: "replay",
		StorageRevision: &integration.ArtifactRevisionRef{ArtifactID: "raw-store", RevisionID: "1", Digest: testDigest('e')},
		EncryptionKey:   &integration.SecretReference{Provider: integration.SecretProviderFile, Key: "-----BEGIN RSA"},
	}
	problems, err := fixture.service.Check(authorCaller(), input)
	if err != nil {
		t.Fatal(err)
	}
	codes := strings.Join(problemCodesOf(problems), " ")
	for _, want := range []string{
		"SECRET_VALUE_FORBIDDEN@secretBindings[0].key", "INVALID_VALUE@secretBindings[1].key",
		"SECRET_VALUE_FORBIDDEN@rawRetention.encryptionKey.key",
	} {
		if !strings.Contains(codes, want) {
			t.Fatalf("problems %s lack %s", codes, want)
		}
	}
	for _, problem := range problems {
		if strings.Contains(problem.Message, "BEGIN") || strings.Contains(problem.Message, "spaces") {
			t.Fatalf("a problem message repeats the value: %q", problem.Message)
		}
	}
	result, err := fixture.service.CreateDraft(authorCaller(), input, "author the definition")
	if err != nil || result.Definition != nil || fixture.catalog.creates != 0 {
		t.Fatalf("create wrote %d drafts (%v, %v)", fixture.catalog.creates, result.Definition, err)
	}

	tooMany := fixture.input(connection.KindMLLP)
	for index := 0; index <= connection.MaxSecretBindings; index++ {
		tooMany.SecretBindings = append(tooMany.SecretBindings, integration.SecretBinding{
			Name: "extra-" + strings.Repeat("x", index+1), Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: "K"},
		})
	}
	problems, _ = fixture.service.Check(authorCaller(), tooMany)
	if !strings.Contains(strings.Join(problemCodesOf(problems), " "), "OUT_OF_RANGE@secretBindings") {
		t.Fatalf("no count cap: %v", problemCodesOf(problems))
	}
}

func TestServiceCreateDraft_IsIdempotentForAnIdenticalRevisionOnly(t *testing.T) {
	fixture := newServiceFixture(t, Config{})
	input := fixture.input(connection.KindMLLP)
	first, err := fixture.service.CreateDraft(authorCaller(), input, "author the definition")
	if err != nil || first.Definition == nil {
		t.Fatalf("first = %v, %v", first.Problems, err)
	}
	// Another author, later, same content: the stored revision comes back.
	fixture.service.clock = func() time.Time { return serviceClock.Add(time.Hour) }
	again, err := fixture.service.CreateDraft(tenantCaller(testTenant, WriteRole, ReadRole), input, "again")
	if err != nil || again.Definition == nil || again.Definition.Revision.Digest != first.Definition.Revision.Digest || fixture.catalog.creates != 1 {
		t.Fatalf("identical re-create = %v (creates %d), %v", again.Problems, fixture.catalog.creates, err)
	}
	changed := input
	changed.SecretBindings = append([]integration.SecretBinding(nil), input.SecretBindings...)
	changed.SecretBindings[0].Reference.Key = "mllp/another-cert.pem"
	result, err := fixture.service.CreateDraft(authorCaller(), changed, "author the definition")
	if err != nil || result.Definition != nil || !strings.Contains(strings.Join(problemCodesOf(result.Problems), " "), "ALREADY_EXISTS@revisionId") {
		t.Fatalf("changed re-create = %v, %v", problemCodesOf(result.Problems), err)
	}
	if problems, _ := fixture.service.Check(authorCaller(), changed); !strings.Contains(strings.Join(problemCodesOf(problems), " "), "ALREADY_EXISTS") {
		t.Fatalf("check of changed content = %v", problemCodesOf(problems))
	}
}

func TestServiceCheck_RefusesAMixedRegistryPair(t *testing.T) {
	fixture := newServiceFixture(t, Config{})
	input := fixture.input(connection.KindMLLP)
	input.Workflow.RevisionID = "workflow-version-2"
	problems, err := fixture.service.Check(authorCaller(), input)
	if err != nil || !strings.Contains(strings.Join(problemCodesOf(problems), " "), CodeArtifactUnresolved) {
		t.Fatalf("mixed pair = %v, %v", problemCodesOf(problems), err)
	}
}

func TestService_RefusesAnotherTenantAndMissingRoles(t *testing.T) {
	fixture := newServiceFixture(t, Config{})
	input := fixture.input(connection.KindMLLP)
	if _, err := fixture.service.List(tenantCaller("tenant-b", ReadRole, WriteRole), false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list as tenant-b = %v", err)
	}
	if _, err := fixture.service.Check(tenantCaller("tenant-b", ReadRole, WriteRole), input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("check as tenant-b = %v", err)
	}
	if _, err := fixture.service.CreateDraft(tenantCaller(testTenant, ReadRole), input, "reason"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("create without the deployment grant = %v", err)
	}
	if _, err := fixture.service.List(context.Background(), false); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("list without identity = %v", err)
	}
}

func TestValidReasonAndIdentityBounds(t *testing.T) {
	for _, test := range []struct {
		reason string
		ok     bool
	}{
		{"x", true}, {"  padded  ", true}, {"line one\nline two", true}, {"", false}, {"   ", false},
		{strings.Repeat("r", MaxReasonBytes), true}, {strings.Repeat("r", MaxReasonBytes+1), false}, {"bell\a", false},
	} {
		if _, err := validReason(test.reason); (err == nil) != test.ok {
			t.Errorf("validReason(%q) = %v", test.reason, err)
		}
	}
	for _, test := range []struct {
		value string
		ok    bool
	}{
		{"v1", true}, {strings.Repeat("i", 256), true}, {strings.Repeat("i", 257), false}, {"", false},
		{" v1", false}, {"v\x01", false}, {"v\x7f", false},
	} {
		if validIdentity(test.value) != test.ok {
			t.Errorf("validIdentity(%q) != %v", test.value, test.ok)
		}
	}
}

// REAL is single-flight per replica, and the slot is held until the probe
// goroutine exits — not merely until the request returns.
func TestServiceValidateReal_IsSingleFlightUntilTheProbeExits(t *testing.T) {
	sftp, _ := compiledFixture(t, connection.KindBatchSFTP, "src-batch-sftp")
	decoded, err := batch.DecodeSourceRevision(strings.NewReader(string(sftp.Document)))
	if err != nil {
		t.Fatal(err)
	}
	realSource := integration.SourceRevisionRef{ArtifactRevisionRef: decoded.Reference(), SourceID: decoded.SourceID}
	fixture := newServiceFixture(t, Config{RealSource: &realSource})
	input := fixture.input(connection.KindBatchSFTP)
	if result, err := fixture.service.CreateDraft(authorCaller(), input, "author"); err != nil || result.Definition == nil {
		t.Fatalf("create = %v, %v", problemCodesOf(result.Problems), err)
	}
	release := make(chan struct{})
	probeExited := make(chan struct{})
	build := func(batch.SourceRevision, BatchSecrets) (batch.Provider, error) {
		<-release
		return nil, errors.New("unreachable")
	}
	validator := BatchValidatorWithLimit(decoded, BatchSecrets{}, build, nil, 50*time.Millisecond)
	fixture.catalog.validate = func(ctx context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		return validator(ctx, revision)
	}
	command := Command{DefinitionID: input.DefinitionID, RevisionID: "v1", ExpectedVersion: 1, Reason: "real check of the drop"}
	// The first request returns at the limit while its probe is still dialing.
	if _, err := fixture.service.Validate(authorCaller(), command, ModeReal); err != nil {
		t.Fatalf("first real = %v", err)
	}
	if _, err := fixture.service.Validate(authorCaller(), command, ModeReal); !errors.Is(err, ErrRealBusy) {
		t.Fatalf("second real while the probe dials = %v, want ErrRealBusy", err)
	}
	go func() {
		close(release)
		close(probeExited)
	}()
	<-probeExited
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := fixture.service.Validate(authorCaller(), command, ModeReal)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrRealBusy) || time.Now().After(deadline) {
			t.Fatalf("real after the probe exited = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// STATIC and SKIP never touch the slot.
	if _, err := fixture.service.Validate(authorCaller(), command, ModeStatic); err != nil {
		t.Fatalf("static = %v", err)
	}
}
