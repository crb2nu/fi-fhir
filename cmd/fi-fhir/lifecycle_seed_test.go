package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	seedTestTenant     = "tenant-a"
	seedTestDefinition = "batch-seed-demo"
	seedTestReason     = "seed the batch demo definition"
	seedTestPrincipal  = "operator-1"
	seedTestCreatedAt  = "2026-09-28T12:00:00Z"
)

// The golden preview registry is byte-identical to the one the deployed API
// mounts; its adt-east workflow delivers to destination artifact fhir-primary.
func seedTestRegistryPath(t *testing.T) string {
	t.Helper()
	return testdataPath(t, filepath.Join("golden", "integration", "adt-http", "preview-registry.json"))
}

type seedFixture struct {
	dir         string
	sourcePath  string
	source      integrationbatch.SourceRevision
	destPath    string
	destination integrationdestination.Revision
	registry    string
}

func seedTestSource(t *testing.T, host string, port int, input, archive string, pollSeconds int64) integrationbatch.SourceRevision {
	t.Helper()
	source, err := integrationbatch.NewSourceRevision(integrationbatch.SourceRevisionInput{
		ArtifactID: "sftp-seed", RevisionID: "r1", SourceID: "sftp-seed", Provider: integrationbatch.ProviderSFTP,
		PollSeconds: pollSeconds, LeaseSeconds: 120, ProcessSeconds: 60, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		SFTP: &integrationbatch.SFTPPolicy{
			Host: host, Port: port, Username: seedSFTPUser, InputDirectory: input, ArchiveDirectory: archive,
			KnownHostsBinding: "sftp-seed-known-hosts", PasswordBinding: "sftp-seed-password",
		},
		Workload: &integrationbatch.WorkloadIdentity{Subject: "batch-seed", Grants: []string{"integration:batch"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func seedTestDestination(t *testing.T, artifactID string, bound bool) integrationdestination.Revision {
	t.Helper()
	input := integrationdestination.RevisionInput{
		ArtifactID: artifactID, RevisionID: "r1", DestinationID: artifactID,
		Class: integration.DestinationClassProduction, Transport: integrationdestination.TransportFHIR,
		FHIR: &integrationdestination.FHIRPolicy{
			BaseURL: "https://hospital.example.test/fhir", TokenBinding: artifactID + "-token",
			Interaction: integrationdestination.FHIRInteractionTransaction,
		},
	}
	if bound {
		input.Identity = &integrationdestination.ClientIdentity{
			Subject: artifactID + "-client", Grants: []string{"integration.destination.client"},
		}
	}
	revision, err := integrationdestination.NewRevision(input)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func writeSeedJSON(t *testing.T, dir, name string, value any) string {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newSeedFixture(t *testing.T) seedFixture {
	t.Helper()
	dir := t.TempDir()
	fixture := seedFixture{dir: dir, registry: seedTestRegistryPath(t)}
	fixture.source = seedTestSource(t, "127.0.0.1", 2222, "/inbound", "/archive", 15)
	fixture.sourcePath = writeSeedJSON(t, dir, "source.json", fixture.source)
	fixture.destination = seedTestDestination(t, "fhir-primary", true)
	fixture.destPath = writeSeedJSON(t, dir, "destination.json", fixture.destination)
	return fixture
}

func (f seedFixture) args(extra ...string) []string {
	args := []string{
		"--source", f.sourcePath, "--definition-id", seedTestDefinition,
		"--integration", "adt-east", "--registry", f.registry,
		"--destination", f.destPath, "--principal", seedTestPrincipal,
		"--reason", seedTestReason, "--tenant", seedTestTenant,
	}
	return append(args, extra...)
}

// isolateSeedEnv clears every environment key the seed reads, so a developer's
// shell or a CI job's variables cannot leak into a unit test.
func isolateSeedEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FI_FHIR_DEPLOYMENT_TENANT_ID", "FI_FHIR_INTEGRATION_REGISTRY_PATH",
		"FI_FHIR_DATABASE_HOST", "FI_FHIR_DATABASE_NAME", "FI_FHIR_DATABASE_USERNAME", "FI_FHIR_DATABASE_USER",
		"FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE", "FI_FHIR_BATCH_SFTP_PASSWORD", "FI_FHIR_BATCH_SFTP_PASSWORD_FILE",
		"FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE", "FI_FHIR_BATCH_S3_ACCESS_KEY", "FI_FHIR_BATCH_S3_SECRET_KEY",
	} {
		t.Setenv(key, "")
	}
}

func runSeedForTest(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := runLifecycleSeed(ctx, args, &stdout, strings.NewReader(""), &stderr)
	return stdout.String(), stderr.String(), err
}

func decodeSeedDryRun(t *testing.T, stdout string) lifecycleSeedDryRun {
	t.Helper()
	var output lifecycleSeedDryRun
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, stdout)
	}
	return output
}

func TestLifecycleSeedArgs_RefusesMissingOrInvalidInput(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	without := func(flag string) []string {
		args := fixture.args()
		for index := 0; index < len(args); index += 2 {
			if args[index] == flag {
				return append(append([]string(nil), args[:index]...), args[index+2:]...)
			}
		}
		t.Fatalf("fixture has no %s", flag)
		return nil
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"source", without("--source"), "--source"},
		{"definition", without("--definition-id"), "--definition-id"},
		{"integration", without("--integration"), "--integration"},
		{"registry", without("--registry"), "--registry"},
		{"destination", without("--destination"), "--destination"},
		{"principal", without("--principal"), "--principal"},
		{"tenant", without("--tenant"), "--tenant"},
		{"reason", without("--reason"), "--reason is required"},
		{"non-canonical principal", fixture.args("--principal", " operator"), "--principal"},
		{"empty role", fixture.args("--role", ""), "--role"},
		{"reason with surrounding space", fixture.args("--reason", " padded reason "), "--reason is required"},
		{"reason too long", fixture.args("--reason", strings.Repeat("r", 1025)), "--reason is required"},
		{"reason with control character", fixture.args("--reason", "line one\nline two"), "--reason is required"},
		{"validate mode", fixture.args("--validate", "maybe"), "--validate must be"},
		{"short skip reason", append(without("--reason"), "--reason", "too short", "--validate", "skip"), "at least 16 bytes"},
		{"through", fixture.args("--through", "approved"), "--through must be"},
		{"created at", fixture.args("--created-at", "yesterday"), "--created-at"},
		{"policy integer", fixture.args("--validation-max-age", "0"), "--validation-max-age"},
		{"policy bounds", fixture.args("--validation-timeout", "301"), "deployment policy"},
		{"two stdin destinations", fixture.args("--destination", "-", "--destination", "-"), "at most one --destination"},
		{"stdin source", append(without("--source"), "--source", "-"), "--source must name a file"},
		{"unknown flag", fixture.args("--force", "yes"), "unknown lifecycle seed option"},
		{"positional", fixture.args("extra"), "unexpected lifecycle seed argument"},
		{"missing value", fixture.args("--revision-id"), "--revision-id requires a value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseLifecycleSeedArgs(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parse error = %v, want it to mention %q", err, test.want)
			}
		})
	}
}

func TestLifecycleSeedArgs_Defaults(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", seedTestTenant)
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", fixture.registry)
	args := fixture.args()
	// Drop --tenant and --registry so the environment supplies them.
	trimmed := make([]string, 0, len(args))
	for index := 0; index < len(args); index += 2 {
		if args[index] == "--tenant" || args[index] == "--registry" {
			continue
		}
		trimmed = append(trimmed, args[index], args[index+1])
	}
	parsed, err := parseLifecycleSeedArgs(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.tenantID != seedTestTenant || parsed.registryPath != fixture.registry {
		t.Fatalf("environment defaults = %q, %q", parsed.tenantID, parsed.registryPath)
	}
	if parsed.revisionID != "v1" || parsed.validate != seedValidateReal ||
		parsed.through != integration.DeploymentStatePublished || parsed.dryRun {
		t.Fatalf("defaults = %+v", parsed)
	}
	if !reflect.DeepEqual(parsed.roles, []string{"integration:operator"}) {
		t.Fatalf("default roles = %v", parsed.roles)
	}
	want := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 5, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 10, MaxMessagesPerSecond: 100},
	}
	if parsed.policy != want {
		t.Fatalf("default policy = %+v", parsed.policy)
	}

	overridden, err := parseLifecycleSeedArgs(fixture.args(
		"--tenant", "tenant-b", "--role", "integration.deployment.operator", "--role", "integration:operator",
		"--validation-max-age", "900", "--max-in-flight", "4", "--max-queued", "40",
		"--max-messages-per-second", "50", "--through", "deployed", "--dry-run",
	))
	if err != nil {
		t.Fatal(err)
	}
	if overridden.tenantID != "tenant-b" || len(overridden.roles) != 2 || !overridden.dryRun ||
		overridden.through != integration.DeploymentStateDeployed ||
		overridden.policy.ConnectionValidation.MaxAgeSeconds != 900 ||
		overridden.policy.Capacity != (integration.CapacityPolicy{MaxInFlight: 4, MaxQueued: 40, MaxMessagesPerSecond: 50}) {
		t.Fatalf("overrides = %+v", overridden)
	}
}

func TestLifecycleSeedDryRun_IsDeterministicAndWritesNothing(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	registryOut := filepath.Join(fixture.dir, "destination-registry.json")
	args := fixture.args("--dry-run", "--created-at", seedTestCreatedAt, "--destination-registry-out", registryOut)

	first, _, err := runSeedForTest(t, args)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	second, _, err := runSeedForTest(t, args)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equal inputs produced different dry-run output")
	}
	if _, err := os.Stat(registryOut); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run wrote the destination registry file: %v", err)
	}

	output := decodeSeedDryRun(t, first)
	if !output.DryRun || output.Env["FI_FHIR_BATCH_DEFINITION_ID"] != seedTestDefinition {
		t.Fatalf("dry-run header = %v, env %v", output.DryRun, output.Env)
	}
	definition := output.Definition
	// The printed revision is the exact bytes the catalog stores: it decodes
	// strictly and its digest verifies.
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil || decoded.ValidateForDeployment() != nil {
		t.Fatalf("printed definition does not decode for deployment: %v", err)
	}
	if output.Refs.Definition != definition.Reference() || output.Refs.Source != definition.Source ||
		output.Refs.Profile != definition.Profile || output.Refs.Workflow != definition.Workflow {
		t.Fatalf("refs do not match the definition: %+v", output.Refs)
	}
	if definition.Source.ArtifactRevisionRef != fixture.source.Reference() || definition.Source.SourceID != fixture.source.SourceID {
		t.Fatalf("source ref = %+v", definition.Source)
	}
	if !reflect.DeepEqual(definition.Destinations, []integration.DestinationRevisionRef{fixture.destination.Reference()}) {
		t.Fatalf("destinations = %+v", definition.Destinations)
	}
	wantBindings := []integration.SecretBinding{
		{Name: "fhir-primary-token", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "destinations/fhir-primary-token"}},
		{Name: "sftp-seed-known-hosts", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "batch/sftp-seed-known-hosts"}},
		{Name: "sftp-seed-password", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "batch/sftp-seed-password"}},
	}
	if !reflect.DeepEqual(definition.SecretBindings, wantBindings) {
		t.Fatalf("secret bindings = %+v", definition.SecretBindings)
	}
	if !reflect.DeepEqual(definition.Created.Principal, integration.Principal{
		ID: seedTestPrincipal, Kind: integration.PrincipalKindHuman, AuthMethod: "postgres", Roles: []string{"integration:operator"},
	}) {
		t.Fatalf("created principal = %+v", definition.Created.Principal)
	}
	if definition.Created.Reason != seedTestReason || !definition.Created.OccurredAt.Equal(time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("created audit = %+v", definition.Created)
	}
	if len(output.DestinationRegistry) == 0 {
		t.Fatal("dry run with --destination-registry-out printed no destination registry")
	}

	// The creation audit is digested, so a different time is a different revision.
	later, _, err := runSeedForTest(t, fixture.args("--dry-run", "--created-at", "2026-09-28T12:00:01Z"))
	if err != nil {
		t.Fatal(err)
	}
	if decodeSeedDryRun(t, later).Definition.Digest == definition.Digest {
		t.Fatal("a different creation time produced the same digest")
	}
}

func TestLifecycleSeed_ProfileAndWorkflowRefsResolveAsTheRuntimeResolvesThem(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	stdout, _, err := runSeedForTest(t, fixture.args("--dry-run"))
	if err != nil {
		t.Fatal(err)
	}
	definition := decodeSeedDryRun(t, stdout).Definition
	// Exactly the construction serve performs for the batch runtime
	// (preview_runtime.go loadIntegrationRuntimeFromEnv).
	staticRegistry, err := loadSeedRegistry(fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := processor.NewRevisionResolver(seedTestTenant, staticRegistry)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.Resolve(context.Background(), seedTestTenant, definition.Profile, definition.Workflow)
	if err != nil {
		t.Fatalf("runtime resolver refuses the seeded refs: %v", err)
	}
	if resolved.ProfileReference() != definition.Profile || resolved.WorkflowReference() != definition.Workflow {
		t.Fatalf("resolved refs differ: %+v %+v", resolved.ProfileReference(), resolved.WorkflowReference())
	}
	want := integration.ArtifactRevisionRef{
		ArtifactID: "profile-adt", RevisionID: "1",
		Digest: "sha256:79c8f575ae135f6d6c10d46fcb57c75a6094ac65c8ab6d10912e5050b26315d3",
	}
	if definition.Profile != want {
		t.Fatalf("profile ref = %+v, want the adt-east registry entry's", definition.Profile)
	}
}

func TestLifecycleSeed_DestinationRegistryLoadsWithTheDeliveryLoader(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	stdout, _, err := runSeedForTest(t, fixture.args("--dry-run", "--destination-registry-out", "-"))
	if err != nil {
		t.Fatal(err)
	}
	output := decodeSeedDryRun(t, stdout)
	registry, err := integrationdestination.LoadRegistry(bytes.NewReader(output.DestinationRegistry), integrationdestination.ModeStrict)
	if err != nil {
		t.Fatalf("destination registry does not load strictly: %v\n%s", err, output.DestinationRegistry)
	}
	if registry.TenantID() != seedTestTenant || registry.IntegrationRevision() != output.Definition.Reference() {
		t.Fatalf("registry binds %s %+v", registry.TenantID(), registry.IntegrationRevision())
	}
	resolved, err := registry.Resolve(seedTestTenant, fixture.destination.Reference())
	if err != nil || resolved.Digest != fixture.destination.Digest {
		t.Fatalf("registry resolve = %+v, %v", resolved.Reference(), err)
	}
	if !reflect.DeepEqual(registry.SecretBindings(), []integration.SecretBinding{{
		Name: "fhir-primary-token", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "destinations/fhir-primary-token"},
	}}) {
		t.Fatalf("registry bindings = %+v", registry.SecretBindings())
	}
	// The destination is carried verbatim: its own bytes decode to its digest.
	var document struct {
		Destinations []json.RawMessage `json:"destinations"`
	}
	if err := json.Unmarshal(output.DestinationRegistry, &document); err != nil || len(document.Destinations) != 1 {
		t.Fatalf("registry destinations = %v", err)
	}
	carried, err := integrationdestination.DecodeRevision(bytes.NewReader(document.Destinations[0]))
	if err != nil || carried.Digest != fixture.destination.Digest {
		t.Fatalf("carried destination = %v", err)
	}

	// An unbound destination is valid only in compatibility mode, and the seed
	// still emits a document that loads there.
	unbound := seedTestDestination(t, "fhir-primary", false)
	unboundPath := writeSeedJSON(t, fixture.dir, "unbound.json", unbound)
	args := fixture.args("--dry-run", "--destination-registry-out", "-")
	for index := range args {
		if args[index] == fixture.destPath {
			args[index] = unboundPath
		}
	}
	stdout, _, err = runSeedForTest(t, args)
	if err != nil {
		t.Fatal(err)
	}
	compatibility := decodeSeedDryRun(t, stdout).DestinationRegistry
	if _, err := integrationdestination.LoadRegistry(bytes.NewReader(compatibility), integrationdestination.ModeCompatibility); err != nil {
		t.Fatalf("unbound registry does not load in compatibility mode: %v", err)
	}
}

func TestLifecycleSeed_RefusesAWorkflowDestinationTheDefinitionLacks(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	elsewhere := writeSeedJSON(t, fixture.dir, "st-elsewhere.json", seedTestDestination(t, "st-elsewhere", true))
	args := fixture.args("--dry-run")
	for index := range args {
		if args[index] == fixture.destPath {
			args[index] = elsewhere
		}
	}
	stdout, _, err := runSeedForTest(t, args)
	if err == nil || !strings.Contains(err.Error(), `destination "fhir-primary"`) || !strings.Contains(err.Error(), "st-elsewhere") {
		t.Fatalf("seed accepted a workflow that delivers outside the definition: %v", err)
	}
	if stdout != "" {
		t.Fatalf("refusal printed output: %s", stdout)
	}
}

func TestLifecycleSeed_RefusesRegistryAndInputMismatches(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	notJSON := filepath.Join(fixture.dir, "not-a-source.json")
	if err := os.WriteFile(notJSON, []byte(`{"schema_version":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown integration", fixture.args("--integration", "adt-west", "--dry-run"), `--integration "adt-west" is not in the integration registry`},
		{"tenant mismatch", fixture.args("--tenant", "tenant-b", "--dry-run"), "registry tenant does not match"},
		{"invalid source", replaceSeedArg(fixture.args("--dry-run"), fixture.sourcePath, notJSON), "not a valid batch source revision"},
		{"invalid destination", replaceSeedArg(fixture.args("--dry-run"), fixture.destPath, fixture.sourcePath), "not a valid destination revision"},
		{"duplicate destination", fixture.args("--destination", fixture.destPath, "--dry-run"), "given more than once"},
		{"missing registry file", fixture.args("--registry", filepath.Join(fixture.dir, "absent.json"), "--dry-run"), "open integration registry"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := runSeedForTest(t, test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func replaceSeedArg(args []string, from, to string) []string {
	replaced := append([]string(nil), args...)
	for index := range replaced {
		if replaced[index] == from {
			replaced[index] = to
		}
	}
	return replaced
}

func TestLifecycleSeed_ReadsOneDestinationFromStandardInput(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	raw, err := os.ReadFile(fixture.destPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := replaceSeedArg(fixture.args("--dry-run", "--created-at", seedTestCreatedAt), fixture.destPath, "-")
	if err := runLifecycleSeed(context.Background(), args, &stdout, bytes.NewReader(raw), &stderr); err != nil {
		t.Fatal(err)
	}
	fromFile, _, err := runSeedForTest(t, fixture.args("--dry-run", "--created-at", seedTestCreatedAt))
	if err != nil {
		t.Fatal(err)
	}
	if decodeSeedDryRun(t, stdout.String()).Definition.Digest != decodeSeedDryRun(t, fromFile).Definition.Digest {
		t.Fatal("a destination read from standard input produced a different definition")
	}
}

func TestLifecycleSeed_RealValidationRefusesMissingCredentialsBeforeAnyWrite(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	// No FI_FHIR_DATABASE_* is set: reaching the database would fail with a
	// different error, so this error proves nothing was opened or written.
	_, _, err := runSeedForTest(t, fixture.args())
	if err == nil || !strings.Contains(err.Error(), "FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE") {
		t.Fatalf("missing batch credential was not refused first: %v", err)
	}
	_, _, err = runSeedForTest(t, fixture.args("--validate", "skip", "--reason", "demo source validated out of band"))
	if err == nil || !strings.Contains(err.Error(), "open lifecycle database") {
		t.Fatalf("skip mode should need only the database: %v", err)
	}
}

func TestLifecycleUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "lifecycle", "help")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{
		"--source", "--definition-id", "--revision-id", "--integration", "--registry", "--destination",
		"--principal", "--reason", "--role", "--tenant", "--validate", "--through", "--dry-run",
		"--destination-registry-out", "--created-at", "--validation-timeout", "--validation-max-age",
		"--max-in-flight", "--max-queued", "--max-messages-per-second", "FI_FHIR_DATABASE_",
	} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("lifecycle usage does not mention %s", flag)
		}
	}
	if _, _, err := runCLI(t, "lifecycle", "deploy"); err == nil || !strings.Contains(err.Error(), "unknown lifecycle command") {
		t.Fatalf("unknown lifecycle subcommand = %v", err)
	}
}

// --- Connection validation ---------------------------------------------------

func seedSFTPSecrets(knownHosts, password string) batchProviderSecrets {
	return batchProviderSecrets{sftp: integrationbatch.SFTPSecrets{KnownHostsPath: knownHosts, Password: password}}
}

func seedRevisionFor(t *testing.T, source integrationbatch.SourceRevision) integration.IntegrationDefinitionRevision {
	t.Helper()
	fixture := newSeedFixture(t)
	fixture.sourcePath = writeSeedJSON(t, fixture.dir, "validated-source.json", source)
	parsed, err := parseLifecycleSeedArgs(fixture.args("--validate", "skip", "--reason", "validator unit test fixture"))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := loadLifecycleSeedInputs(context.Background(), parsed, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := buildSeedDefinition(inputs, integration.AuditEnvelope{
		TenantID: seedTestTenant, Principal: inputs.principal, Reason: parsed.reason,
		OccurredAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestBatchConnectionValidator_SFTPOutcomes(t *testing.T) {
	isolateSeedEnv(t)
	server := startSeedSFTPServer(t)
	input := filepath.Join(server.root, "inbound")
	archive := filepath.Join(server.root, "archive")
	for _, directory := range []string{input, archive} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := seedTestSource(t, server.host, server.port, input, archive, 15)
	revision := seedRevisionFor(t, source)
	validate := func(secrets batchProviderSecrets, candidate integrationbatch.SourceRevision) (lifecycle.ConnectionValidationOutcome, string, error) {
		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		outcome, err := batchConnectionValidator(candidate, secrets, newBatchProvider, &stderr)(ctx, revision)
		return outcome, stderr.String(), err
	}

	outcome, detail, err := validate(seedSFTPSecrets(server.knownHosts, seedSFTPPassword), source)
	if err != nil || !outcome.Passed || detail != "" ||
		!reflect.DeepEqual(outcome.Codes, []string{"SOURCE_REACHABLE", "HOST_KEY_VERIFIED", "AUTH_OK", "INPUT_LISTED"}) {
		t.Fatalf("reachable source = %+v, %q, %v", outcome, detail, err)
	}

	for _, test := range []struct {
		name    string
		secrets batchProviderSecrets
	}{
		{"pinned host key differs", seedSFTPSecrets(wrongSeedKnownHosts(t, server.address), seedSFTPPassword)},
		{"password refused", seedSFTPSecrets(server.knownHosts, "not-the-password")},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome, detail, err := validate(test.secrets, source)
			if err != nil || outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{"SOURCE_CONNECT_FAILED"}) {
				t.Fatalf("outcome = %+v, %v", outcome, err)
			}
			if !strings.Contains(detail, "batch provider unavailable") || strings.Contains(detail, "not-the-password") {
				t.Fatalf("detail = %q", detail)
			}
		})
	}

	missingInput := seedTestSource(t, server.host, server.port, filepath.Join(server.root, "absent"), archive, 15)
	outcome, _, err = validate(seedSFTPSecrets(server.knownHosts, seedSFTPPassword), missingInput)
	if err != nil || outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{"SOURCE_REVISION_MISMATCH"}) {
		t.Fatalf("validator accepted a revision for another source: %+v, %v", outcome, err)
	}
	missingRevision := seedRevisionFor(t, missingInput)
	var stderr bytes.Buffer
	outcome, err = batchConnectionValidator(missingInput, seedSFTPSecrets(server.knownHosts, seedSFTPPassword), newBatchProvider, &stderr)(context.Background(), missingRevision)
	if err != nil || outcome.Passed ||
		!reflect.DeepEqual(outcome.Codes, []string{"SOURCE_REACHABLE", "HOST_KEY_VERIFIED", "AUTH_OK", "INPUT_LIST_FAILED"}) {
		t.Fatalf("missing input directory = %+v, %v", outcome, err)
	}
}

func TestBatchConnectionValidator_HonoursTheCatalogDeadline(t *testing.T) {
	isolateSeedEnv(t)
	address := startSilentListener(t)
	host, portText, _ := strings.Cut(address, ":")
	var port int
	_, _ = fmt.Sscan(portText, &port)
	source := seedTestSource(t, host, port, "/inbound", "/archive", 15)
	revision := seedRevisionFor(t, source)
	knownHosts := wrongSeedKnownHosts(t, address)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := batchConnectionValidator(source, seedSFTPSecrets(knownHosts, seedSFTPPassword), newBatchProvider, nil)(ctx, revision)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled handshake = %v, want the catalog deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("validator ignored the deadline for %s", elapsed)
	}
}

func TestSkipConnectionValidator_RecordsItsCode(t *testing.T) {
	outcome, err := skipConnectionValidator()(context.Background(), integration.IntegrationDefinitionRevision{})
	if err != nil || !outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{"VALIDATION_SKIPPED"}) {
		t.Fatalf("skip outcome = %+v, %v", outcome, err)
	}
}

// --- Resume on a fake catalog -------------------------------------------------

// fakeSeedCatalog is an in-memory model of lifecycle.PostgresCatalog's state
// machine: expected versions, validation freshness, one release per publish,
// and one active deployment per definition.
type fakeSeedCatalog struct {
	now         func() time.Time
	validator   lifecycle.ConnectionValidatorFunc
	revisions   map[string][]byte
	snapshots   map[string]lifecycle.Snapshot
	validations map[string]lifecycle.ValidationRecord
	active      map[string]string
	commands    []lifecycle.Command
	sequence    int
}

func newFakeSeedCatalog(now func() time.Time, validator lifecycle.ConnectionValidatorFunc) *fakeSeedCatalog {
	return &fakeSeedCatalog{
		now: now, validator: validator,
		revisions: map[string][]byte{}, snapshots: map[string]lifecycle.Snapshot{},
		validations: map[string]lifecycle.ValidationRecord{}, active: map[string]string{},
	}
}

func fakeSeedKey(definitionID, revisionID string) string { return definitionID + "/" + revisionID }

func (c *fakeSeedCatalog) CreateDraft(_ context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.Snapshot, error) {
	key := fakeSeedKey(revision.DefinitionID, revision.RevisionID)
	if _, exists := c.snapshots[key]; exists {
		return lifecycle.Snapshot{}, lifecycle.ErrAlreadyExists
	}
	if revision.ValidateForDeployment() != nil {
		return lifecycle.Snapshot{}, lifecycle.ErrInvalidCommand
	}
	raw, err := json.Marshal(revision)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	c.revisions[key] = raw
	snapshot := lifecycle.Snapshot{
		TenantID: revision.TenantID, DefinitionRevision: revision.Reference(),
		State: integration.DeploymentStateDraft, Version: 1, Health: integration.DeploymentHealthUnknown,
	}
	c.snapshots[key] = snapshot
	return snapshot, nil
}

func (c *fakeSeedCatalog) GetSnapshot(_ context.Context, _, definitionID, revisionID string) (lifecycle.Snapshot, error) {
	snapshot, found := c.snapshots[fakeSeedKey(definitionID, revisionID)]
	if !found {
		return lifecycle.Snapshot{}, lifecycle.ErrNotFound
	}
	return snapshot, nil
}

func (c *fakeSeedCatalog) LoadDefinitionRevision(_ context.Context, _, definitionID, revisionID string) ([]byte, error) {
	raw, found := c.revisions[fakeSeedKey(definitionID, revisionID)]
	if !found {
		return nil, lifecycle.ErrNotFound
	}
	return bytes.Clone(raw), nil
}

func (c *fakeSeedCatalog) GetValidation(_ context.Context, validationID string) (lifecycle.ValidationRecord, error) {
	record, found := c.validations[validationID]
	if !found {
		return lifecycle.ValidationRecord{}, lifecycle.ErrNotFound
	}
	return record, nil
}

func (c *fakeSeedCatalog) lock(command lifecycle.Command) (string, lifecycle.Snapshot, error) {
	c.commands = append(c.commands, command)
	key := fakeSeedKey(command.DefinitionID, command.RevisionID)
	snapshot, found := c.snapshots[key]
	if !found {
		return key, lifecycle.Snapshot{}, lifecycle.ErrNotFound
	}
	if snapshot.Version != command.ExpectedVersion {
		return key, lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	return key, snapshot, nil
}

func (c *fakeSeedCatalog) ValidateConnection(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	key, snapshot, err := c.lock(command)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	switch snapshot.State {
	case integration.DeploymentStateDeployed, integration.DeploymentStateRetired:
		return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
	}
	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(c.revisions[key]))
	if err != nil {
		return lifecycle.Snapshot{}, lifecycle.ErrImmutableRecord
	}
	outcome, err := c.validator(ctx, revision)
	if err != nil {
		outcome = lifecycle.ConnectionValidationOutcome{Codes: []string{"CONNECTION_CHECK_ERROR"}}
	}
	c.sequence++
	now := c.now().UTC()
	record := lifecycle.ValidationRecord{
		ID: fmt.Sprintf("validation-%d", c.sequence), TenantID: snapshot.TenantID,
		DefinitionRevision: snapshot.DefinitionRevision, SourceRevision: revision.Source.ArtifactRevisionRef,
		Passed: outcome.Passed, Codes: append([]string(nil), outcome.Codes...), CheckedAt: now,
		ExpiresAt: now.Add(time.Duration(revision.Deployment.ConnectionValidation.MaxAgeSeconds) * time.Second),
	}
	c.validations[record.ID] = record
	snapshot.Version++
	snapshot.LastValidationID = record.ID
	snapshot.ValidationPassed = record.Passed
	snapshot.ValidationCheckedAt = record.CheckedAt
	snapshot.ValidationExpiresAt = record.ExpiresAt
	if snapshot.State == integration.DeploymentStateDraft && outcome.Passed {
		snapshot.State = integration.DeploymentStateValidated
	}
	c.snapshots[key] = snapshot
	if !outcome.Passed {
		return snapshot, lifecycle.ErrConnectionValidationFailed
	}
	return snapshot, nil
}

func (c *fakeSeedCatalog) transition(command lifecycle.Command, from, to integration.DeploymentState) (lifecycle.Snapshot, error) {
	key, snapshot, err := c.lock(command)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	if snapshot.State != from {
		return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
	}
	if !snapshot.ValidationPassed || !snapshot.ValidationExpiresAt.After(c.now()) {
		return lifecycle.Snapshot{}, lifecycle.ErrConnectionValidationRequired
	}
	switch to {
	case integration.DeploymentStatePublished:
		c.sequence++
		snapshot.ReleaseID = fmt.Sprintf("release-%d", c.sequence)
	case integration.DeploymentStateDeployed:
		if other, busy := c.active[command.DefinitionID]; busy && other != command.RevisionID {
			return lifecycle.Snapshot{}, lifecycle.ErrActiveDeployment
		}
		c.active[command.DefinitionID] = command.RevisionID
		snapshot.Health = integration.DeploymentHealthStarting
	}
	snapshot.State = to
	snapshot.Version++
	c.snapshots[key] = snapshot
	return snapshot, nil
}

func (c *fakeSeedCatalog) Approve(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.transition(command, integration.DeploymentStateValidated, integration.DeploymentStateApproved)
}

func (c *fakeSeedCatalog) Publish(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.transition(command, integration.DeploymentStateApproved, integration.DeploymentStatePublished)
}

func (c *fakeSeedCatalog) Deploy(_ context.Context, command lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.transition(command, integration.DeploymentStatePublished, integration.DeploymentStateDeployed)
}

// seedCase builds parsed inputs and the candidate revision the way
// runLifecycleSeed does, at a fixed creation time.
func seedCase(t *testing.T, fixture seedFixture, extra ...string) (lifecycleSeedInputs, integration.IntegrationDefinitionRevision) {
	t.Helper()
	parsed, err := parseLifecycleSeedArgs(fixture.args(append([]string{"--validate", "skip", "--reason", "seed the batch demo definition"}, extra...)...))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := loadLifecycleSeedInputs(context.Background(), parsed, nil)
	if err != nil {
		t.Fatal(err)
	}
	created := parsed.createdAt
	if created.IsZero() {
		created = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	}
	candidate, err := buildSeedDefinition(inputs, integration.AuditEnvelope{
		TenantID: parsed.tenantID, Principal: inputs.principal, Reason: parsed.reason, OccurredAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	return inputs, candidate
}

type seedClock struct{ at time.Time }

func (c *seedClock) now() time.Time { return c.at }

func TestSeedLifecycle_CreatesPublishesThenResumesToDeployed(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
	inputs, candidate := seedCase(t, fixture)

	summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !summary.CreatedDraft || summary.State != integration.DeploymentStatePublished ||
		!reflect.DeepEqual(summary.Transitions, []string{"create_draft", "validate_connection", "approve", "publish"}) {
		t.Fatalf("first run = %+v", summary)
	}
	if summary.Definition != candidate.Reference() || summary.ReleaseID == "" ||
		!summary.Validation.Passed || !reflect.DeepEqual(summary.Validation.Codes, []string{"VALIDATION_SKIPPED"}) ||
		summary.Env["FI_FHIR_BATCH_DEFINITION_ID"] != seedTestDefinition || !strings.Contains(summary.Next, summary.ReleaseID) {
		t.Fatalf("first summary = %+v", summary)
	}
	// Every transition names the snapshot version it read, the operator, and
	// the reason.
	for index, command := range catalog.commands {
		if command.ExpectedVersion != int64(index+1) || command.Principal.ID != seedTestPrincipal ||
			command.Principal.AuthMethod != "postgres" || command.Reason != seedTestReason ||
			!reflect.DeepEqual(command.Principal.Roles, []string{"integration:operator"}) {
			t.Fatalf("command %d = %+v", index, command)
		}
	}

	// Same inputs, a later run by the same operator, now through deployed:
	// only the deploy happens.
	clock.at = clock.at.Add(time.Minute)
	inputs.args.through = integration.DeploymentStateDeployed
	again, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if again.CreatedDraft || again.State != integration.DeploymentStateDeployed ||
		!reflect.DeepEqual(again.Transitions, []string{"deploy"}) || again.Definition != summary.Definition {
		t.Fatalf("resume = %+v", again)
	}
	// Deployed is terminal for the seed: a third run changes nothing.
	third, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil || len(third.Transitions) != 0 || third.State != integration.DeploymentStateDeployed {
		t.Fatalf("third run = %+v, %v", third, err)
	}
}

func TestSeedLifecycle_ResumesUnderAnotherOperatorsCreationAudit(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
	inputs, _ := seedCase(t, fixture)
	// Someone else created the identical content earlier and stopped at draft.
	storedInputs := inputs
	storedInputs.principal = integration.Principal{ID: "someone-else", Kind: integration.PrincipalKindHuman, AuthMethod: "postgres", Roles: []string{"integration:operator"}}
	stored, err := buildSeedDefinition(storedInputs, integration.AuditEnvelope{
		TenantID: seedTestTenant, Principal: storedInputs.principal, Reason: "an earlier seed", OccurredAt: clock.at.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.CreateDraft(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	_, candidate := seedCase(t, fixture)
	if candidate.Digest == stored.Digest {
		t.Fatal("fixture error: the two creation audits should differ")
	}
	summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil {
		t.Fatalf("resume of identical content was refused: %v", err)
	}
	if summary.CreatedDraft || summary.Definition.Digest != stored.Digest || summary.State != integration.DeploymentStatePublished ||
		!summary.DefinitionCreatedAt.Equal(clock.at.Add(-time.Hour)) {
		t.Fatalf("resume summary = %+v", summary)
	}
}

func TestSeedLifecycle_RefusesDifferentContentUnderTheSameRevision(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
	inputs, candidate := seedCase(t, fixture)
	if _, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now); err != nil {
		t.Fatal(err)
	}
	before := len(catalog.commands)
	storedBefore := bytes.Clone(catalog.revisions[fakeSeedKey(seedTestDefinition, "v1")])

	mutated := seedTestSource(t, "127.0.0.1", 2222, "/inbound", "/archive", 30)
	fixture.sourcePath = writeSeedJSON(t, fixture.dir, "mutated-source.json", mutated)
	mutatedInputs, mutatedCandidate := seedCase(t, fixture)
	summary, err := seedLifecycle(context.Background(), catalog, mutatedInputs, mutatedCandidate, clock.now)
	if !errors.Is(err, errLifecycleSeedConflict) || !strings.Contains(err.Error(), "new --revision-id") {
		t.Fatalf("mutated source = %v", err)
	}
	if summary.Definition.ArtifactID != "" || len(catalog.commands) != before ||
		!bytes.Equal(catalog.revisions[fakeSeedKey(seedTestDefinition, "v1")], storedBefore) {
		t.Fatal("a refused seed touched the catalog")
	}
	// The same change as a new revision is accepted.
	mutatedInputs, mutatedCandidate = seedCase(t, fixture, "--revision-id", "v2")
	if _, err := seedLifecycle(context.Background(), catalog, mutatedInputs, mutatedCandidate, clock.now); err != nil {
		t.Fatalf("new revision: %v", err)
	}
}

func TestSeedLifecycle_ResumesFromEachIntermediateStateAndRefreshesStaleEvidence(t *testing.T) {
	isolateSeedEnv(t)
	for _, stop := range []integration.DeploymentState{
		integration.DeploymentStateDraft, integration.DeploymentStateValidated, integration.DeploymentStateApproved,
	} {
		t.Run(string(stop), func(t *testing.T) {
			fixture := newSeedFixture(t)
			clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
			catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
			inputs, candidate := seedCase(t, fixture)
			snapshot, err := catalog.CreateDraft(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			command := func() lifecycle.Command {
				return lifecycle.Command{
					TenantID: seedTestTenant, DefinitionID: seedTestDefinition, RevisionID: "v1",
					ExpectedVersion: snapshot.Version, Principal: seedPrincipal(inputs.args), Reason: "partial",
				}
			}
			if stop != integration.DeploymentStateDraft {
				if snapshot, err = catalog.ValidateConnection(context.Background(), command()); err != nil {
					t.Fatal(err)
				}
			}
			if stop == integration.DeploymentStateApproved {
				if snapshot, err = catalog.Approve(context.Background(), command()); err != nil {
					t.Fatal(err)
				}
			}
			// The earlier run stopped long enough ago that its evidence expired.
			clock.at = clock.at.Add(time.Hour)
			summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
			if err != nil {
				t.Fatalf("resume from %s: %v", stop, err)
			}
			if summary.State != integration.DeploymentStatePublished || summary.CreatedDraft ||
				summary.Transitions[0] != "validate_connection" || summary.Transitions[len(summary.Transitions)-1] != "publish" {
				t.Fatalf("resume from %s = %+v", stop, summary)
			}
			if summary.Validation.CheckedAt == nil || !summary.Validation.CheckedAt.Equal(clock.at) {
				t.Fatalf("resume did not refresh stale evidence: %+v", summary.Validation)
			}
		})
	}
}

func TestSeedLifecycle_PublishedRefreshesStaleEvidenceForAStudioDeploy(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
	inputs, candidate := seedCase(t, fixture)
	if _, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now); err != nil {
		t.Fatal(err)
	}
	// Still fresh: nothing to do.
	fresh, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil || len(fresh.Transitions) != 0 {
		t.Fatalf("fresh published re-run = %+v, %v", fresh, err)
	}
	clock.at = clock.at.Add(10 * time.Minute)
	stale, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil || !reflect.DeepEqual(stale.Transitions, []string{"validate_connection"}) ||
		stale.State != integration.DeploymentStatePublished || !stale.Validation.ExpiresAt.After(clock.at) {
		t.Fatalf("stale published re-run = %+v, %v", stale, err)
	}
}

func TestSeedLifecycle_FailedValidationReportsCodesAndResumesAfterTheFix(t *testing.T) {
	isolateSeedEnv(t)
	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	reachable := false
	catalog := newFakeSeedCatalog(clock.now, func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		if !reachable {
			return lifecycle.ConnectionValidationOutcome{Codes: []string{"SOURCE_CONNECT_FAILED"}}, nil
		}
		return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE"}}, nil
	})
	inputs, candidate := seedCase(t, fixture)
	summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err == nil || !strings.Contains(err.Error(), "connection validation failed") {
		t.Fatalf("failed validation = %v", err)
	}
	if summary.State != integration.DeploymentStateDraft || summary.Validation.Passed ||
		!reflect.DeepEqual(summary.Validation.Codes, []string{"SOURCE_CONNECT_FAILED"}) ||
		!reflect.DeepEqual(summary.Transitions, []string{"create_draft", "validate_connection"}) {
		t.Fatalf("failed summary = %+v", summary)
	}
	reachable = true
	summary, err = seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if err != nil || summary.State != integration.DeploymentStatePublished ||
		!reflect.DeepEqual(summary.Validation.Codes, []string{"SOURCE_REACHABLE"}) {
		t.Fatalf("resume after fix = %+v, %v", summary, err)
	}
}

func TestSeedLifecycle_ExplainsStatesItWillNotAdvance(t *testing.T) {
	isolateSeedEnv(t)
	for _, test := range []struct {
		state integration.DeploymentState
		want  string
	}{
		{integration.DeploymentStatePaused, "resumeIntegrationDeployment"},
		{integration.DeploymentStateRetired, "new --revision-id"},
	} {
		t.Run(string(test.state), func(t *testing.T) {
			fixture := newSeedFixture(t)
			clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
			catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
			inputs, candidate := seedCase(t, fixture)
			snapshot, err := catalog.CreateDraft(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.State = test.state
			catalog.snapshots[fakeSeedKey(seedTestDefinition, "v1")] = snapshot
			summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
			if err == nil || !strings.Contains(err.Error(), test.want) || summary.State != test.state || len(summary.Transitions) != 0 {
				t.Fatalf("%s = %+v, %v", test.state, summary, err)
			}
		})
	}

	fixture := newSeedFixture(t)
	clock := &seedClock{at: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	catalog := newFakeSeedCatalog(clock.now, skipConnectionValidator())
	catalog.active[seedTestDefinition] = "v0"
	inputs, candidate := seedCase(t, fixture, "--through", "deployed")
	summary, err := seedLifecycle(context.Background(), catalog, inputs, candidate, clock.now)
	if !errors.Is(err, lifecycle.ErrActiveDeployment) || !strings.Contains(err.Error(), "retire it") ||
		summary.State != integration.DeploymentStatePublished {
		t.Fatalf("active deployment = %+v, %v", summary, err)
	}
}
