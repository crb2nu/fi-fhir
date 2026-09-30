package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// `fi-fhir lifecycle seed` is the supported way to put a batch integration
// definition into the PostgreSQL lifecycle catalog outside tests. It runs the
// sequence the batch proof's deployBatchRevision helper runs
// (internal/integration/batch/batch_integration_test.go): build the revision
// with integration.NewIntegrationDefinitionRevision, then CreateDraft ->
// ValidateConnection -> Approve -> Publish [-> Deploy], each transition at the
// snapshot's expected version under the operator's principal and reason.
//
// Where the profile and workflow refs come from, and why.
//
// The batch runner never loads profile or workflow bytes from the lifecycle
// catalog. `serve` builds one processor.RevisionResolver over the static
// integration registry (FI_FHIR_INTEGRATION_REGISTRY_PATH; preview_runtime.go,
// loadIntegrationRuntimeFromEnv) and hands that same resolver to the batch
// runtime (batch_runtime.go, loadBatchRuntimeFromEnv). At admission the durable
// processor resolves the deployed definition from the catalog, then calls
// RevisionResolver.Resolve with the definition's Profile and Workflow refs. That
// looks the bytes up in the static registry by (artifact_id, revision_id) only,
// recomputes the domain-separated digest (canonical JSON for a profile, the exact
// YAML bytes for a workflow), and refuses unless the recomputed ref equals the
// definition's ref byte for byte. A profile or workflow the Studio published into
// its own stores is invisible to this path. So the seed takes both refs from one
// registry entry (--integration) and proves them with the very resolver the
// runtime uses before it writes anything.
//
// The workflow planner then requires every non-log action's destination to be
// one of the definition's destination artifact IDs (processor/workflow_plan.go);
// a miss fails every matching message with ErrInvalidWorkflowPlan. The seed
// refuses such a definition instead of deploying one that cannot ingest.
//
// The seed and the Studio's definition editor (.loom/42 E-1) share one
// implementation: internal/integration/lifecycle/authoring builds the
// revision, proves the registry refs, applies the planner rule, and owns the
// skip and batch validators. This file only assembles authoring's inputs from
// files and flags, so the CLI and the API cannot author different bytes for
// the same inputs (TestDefinitionAuthoringParity_SeedAndEditorAuthorTheSameBytes).

const (
	lifecycleSeedDefaultRevisionID = "v1"
	lifecycleSeedDefaultRole       = "integration:operator"
	// The PostgreSQL connection authenticates the operator, as it does for
	// `fi-fhir delivery replay`.
	lifecycleSeedAuthMethod        = "postgres"
	lifecycleSeedDestKeyPrefix     = authoring.DestinationFileKeyPrefix
	lifecycleSeedMaxReasonBytes    = 1024
	lifecycleSeedMinSkipReason     = authoring.MinSkipReasonBytes
	lifecycleSeedRunTimeout        = 2 * time.Minute
	lifecycleSeedValidationMargin  = 30 * time.Second
	lifecycleSeedDestinationSchema = "fi-fhir/destination-registry/v1"
	lifecycleSeedStdinPath         = "-"

	seedValidateReal = string(authoring.ModeReal)
	seedValidateSkip = string(authoring.ModeSkip)

	// The validation outcome code a skipped validation records.
	seedCodeSkipped = authoring.CodeSkipped
)

var errLifecycleSeedConflict = errors.New("definition revision already exists with different content")

type lifecycleSeedArgs struct {
	sourcePath       string
	definitionID     string
	revisionID       string
	integrationID    string
	registryPath     string
	destinationPaths []string
	principalID      string
	reason           string
	roles            []string
	tenantID         string
	validate         string
	through          integration.DeploymentState
	dryRun           bool
	registryOut      string
	createdAt        time.Time
	policy           integration.IntegrationDeploymentPolicy
}

// seedDestination keeps the decoded revision and its exact file bytes, so the
// destination registry document carries the revision verbatim.
type seedDestination struct {
	revision integrationdestination.Revision
	raw      json.RawMessage
}

type lifecycleSeedInputs struct {
	args         lifecycleSeedArgs
	source       integrationbatch.SourceRevision
	profile      integration.ArtifactRevisionRef
	workflow     integration.ArtifactRevisionRef
	destinations []seedDestination
	principal    integration.Principal
}

// seedCatalog is the part of lifecycle.PostgresCatalog the seed drives. It
// exists so the resume logic is tested against a fake without a database.
type seedCatalog interface {
	CreateDraft(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.Snapshot, error)
	GetSnapshot(ctx context.Context, tenantID, definitionID, revisionID string) (lifecycle.Snapshot, error)
	LoadDefinitionRevision(ctx context.Context, tenantID, definitionID, revisionID string) ([]byte, error)
	GetValidation(ctx context.Context, validationID string) (lifecycle.ValidationRecord, error)
	ValidateConnection(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Approve(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Publish(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Deploy(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
}

type seedValidationSummary struct {
	Passed    bool       `json:"passed"`
	Codes     []string   `json:"codes"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type lifecycleSeedSummary struct {
	Definition          integration.ArtifactRevisionRef `json:"definition"`
	DefinitionCreatedAt time.Time                       `json:"definition_created_at"`
	TenantID            string                          `json:"tenant_id"`
	State               integration.DeploymentState     `json:"state"`
	SnapshotVersion     int64                           `json:"snapshot_version"`
	CreatedDraft        bool                            `json:"created_draft"`
	Transitions         []string                        `json:"transitions"`
	Validation          seedValidationSummary           `json:"validation"`
	ReleaseID           string                          `json:"release_id,omitempty"`
	Env                 map[string]string               `json:"env"`
	DestinationRegistry json.RawMessage                 `json:"destination_registry,omitempty"`
	DestinationRegPath  string                          `json:"destination_registry_path,omitempty"`
	Next                string                          `json:"next,omitempty"`
}

type lifecycleSeedRefs struct {
	Definition   integration.ArtifactRevisionRef      `json:"definition"`
	Source       integration.SourceRevisionRef        `json:"source"`
	Profile      integration.ArtifactRevisionRef      `json:"profile"`
	Workflow     integration.ArtifactRevisionRef      `json:"workflow"`
	Destinations []integration.DestinationRevisionRef `json:"destinations"`
}

type lifecycleSeedDryRun struct {
	DryRun              bool                                      `json:"dry_run"`
	Definition          integration.IntegrationDefinitionRevision `json:"definition"`
	Refs                lifecycleSeedRefs                         `json:"refs"`
	Env                 map[string]string                         `json:"env"`
	DestinationRegistry json.RawMessage                           `json:"destination_registry,omitempty"`
}

type seedDestinationRegistryDocument struct {
	Schema              string                          `json:"schema"`
	TenantID            string                          `json:"tenant_id"`
	IntegrationRevision integration.ArtifactRevisionRef `json:"integration_revision"`
	SecretBindings      []integration.SecretBinding     `json:"secret_bindings,omitempty"`
	Destinations        []json.RawMessage               `json:"destinations"`
}

func runLifecycle(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printLifecycleUsage(os.Stdout)
		return nil
	}
	if args[0] != "seed" {
		return fmt.Errorf("unknown lifecycle command %q", args[0])
	}
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			printLifecycleUsage(os.Stdout)
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleSeedRunTimeout)
	defer cancel()
	return runLifecycleSeed(ctx, args[1:], os.Stdout, os.Stdin, os.Stderr)
}

func runLifecycleSeed(ctx context.Context, args []string, stdout io.Writer, stdin io.Reader, stderr io.Writer) error {
	parsed, err := parseLifecycleSeedArgs(args)
	if err != nil {
		return err
	}
	inputs, err := loadLifecycleSeedInputs(ctx, parsed, stdin)
	if err != nil {
		return err
	}
	created := integration.AuditEnvelope{
		TenantID: parsed.tenantID, Principal: inputs.principal,
		Reason: parsed.reason, OccurredAt: parsed.createdAt,
	}
	if created.OccurredAt.IsZero() {
		created.OccurredAt = time.Now().UTC().Truncate(time.Second)
	}
	candidate, err := buildSeedDefinition(inputs, created)
	if err != nil {
		return err
	}
	if parsed.dryRun {
		return writeLifecycleSeedDryRun(stdout, inputs, candidate)
	}

	var secrets batchProviderSecrets
	if parsed.validate == seedValidateReal {
		// Refuse a missing or unreadable credential before anything is written.
		secrets, err = loadBatchProviderSecretsFromEnv(inputs.source)
		if err != nil {
			return fmt.Errorf("--validate real: %w", err)
		}
	}
	db, err := openSubmissionDatabaseFromEnv(ctx)
	if err != nil {
		return fmt.Errorf("open lifecycle database: %w", err)
	}
	defer func() { _ = db.Close() }()
	validator := skipConnectionValidator()
	if parsed.validate == seedValidateReal {
		validator = batchConnectionValidator(inputs.source, secrets, newBatchProvider, stderr)
	}
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{ValidateConnection: validator})
	if err != nil {
		return fmt.Errorf("configure lifecycle catalog: %w", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate lifecycle catalog: %w", err)
	}
	summary, runErr := seedLifecycle(ctx, catalog, inputs, candidate, time.Now)
	if runErr == nil && parsed.registryOut != "" {
		document, err := seedDestinationRegistry(inputs, summary.Definition)
		if err != nil {
			return err
		}
		if parsed.registryOut == lifecycleSeedStdinPath {
			summary.DestinationRegistry = document
		} else {
			if err := os.WriteFile(parsed.registryOut, append(document, '\n'), 0o600); err != nil {
				return fmt.Errorf("write destination registry: %w", err)
			}
			summary.DestinationRegPath = parsed.registryOut
		}
	}
	if summary.Definition.ArtifactID != "" {
		if err := writeIndentedJSON(stdout, summary); err != nil {
			return err
		}
	}
	return runErr
}

func parseLifecycleSeedArgs(args []string) (lifecycleSeedArgs, error) {
	parsed := lifecycleSeedArgs{
		revisionID: lifecycleSeedDefaultRevisionID,
		validate:   seedValidateReal,
		through:    integration.DeploymentStatePublished,
		tenantID:   strings.TrimSpace(os.Getenv("FI_FHIR_DEPLOYMENT_TENANT_ID")),
		registryPath: strings.TrimSpace(
			os.Getenv("FI_FHIR_INTEGRATION_REGISTRY_PATH"),
		),
		policy: defaultSeedDeploymentPolicy(),
	}
	for index := 0; index < len(args); index++ {
		name := args[index]
		if name == "--dry-run" {
			parsed.dryRun = true
			continue
		}
		if !strings.HasPrefix(name, "--") {
			return lifecycleSeedArgs{}, fmt.Errorf("unexpected lifecycle seed argument %q", name)
		}
		if index+1 >= len(args) {
			return lifecycleSeedArgs{}, fmt.Errorf("%s requires a value", name)
		}
		index++
		value := args[index]
		var err error
		switch name {
		case "--source":
			parsed.sourcePath = value
		case "--definition-id":
			parsed.definitionID = value
		case "--revision-id":
			parsed.revisionID = value
		case "--integration":
			parsed.integrationID = value
		case "--registry":
			parsed.registryPath = value
		case "--destination":
			parsed.destinationPaths = append(parsed.destinationPaths, value)
		case "--principal":
			parsed.principalID = value
		case "--reason":
			parsed.reason = value
		case "--role":
			parsed.roles = append(parsed.roles, value)
		case "--tenant":
			parsed.tenantID = value
		case "--validate":
			parsed.validate = value
		case "--through":
			parsed.through = integration.DeploymentState(value)
		case "--destination-registry-out":
			parsed.registryOut = value
		case "--created-at":
			parsed.createdAt, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return lifecycleSeedArgs{}, fmt.Errorf("--created-at must be an RFC 3339 timestamp")
			}
			parsed.createdAt = parsed.createdAt.UTC()
		case "--validation-timeout":
			parsed.policy.ConnectionValidation.TimeoutSeconds, err = parseSeedPositiveInt(name, value)
		case "--validation-max-age":
			parsed.policy.ConnectionValidation.MaxAgeSeconds, err = parseSeedPositiveInt(name, value)
		case "--max-in-flight":
			parsed.policy.Capacity.MaxInFlight, err = parseSeedPositiveIntAsInt(name, value)
		case "--max-queued":
			parsed.policy.Capacity.MaxQueued, err = parseSeedPositiveIntAsInt(name, value)
		case "--max-messages-per-second":
			parsed.policy.Capacity.MaxMessagesPerSecond, err = parseSeedPositiveIntAsInt(name, value)
		default:
			return lifecycleSeedArgs{}, fmt.Errorf("unknown lifecycle seed option %q", name)
		}
		if err != nil {
			return lifecycleSeedArgs{}, err
		}
	}
	if len(parsed.roles) == 0 {
		parsed.roles = []string{lifecycleSeedDefaultRole}
	}

	missing := make([]string, 0, 8)
	for _, required := range []struct{ name, value string }{
		{"--source", parsed.sourcePath},
		{"--definition-id", parsed.definitionID},
		{"--revision-id", parsed.revisionID},
		{"--integration", parsed.integrationID},
		{"--registry (or FI_FHIR_INTEGRATION_REGISTRY_PATH)", parsed.registryPath},
		{"--principal", parsed.principalID},
		{"--tenant (or FI_FHIR_DEPLOYMENT_TENANT_ID)", parsed.tenantID},
	} {
		if !canonicalSeedIdentity(required.value) {
			missing = append(missing, required.name)
		}
	}
	if len(parsed.destinationPaths) == 0 {
		missing = append(missing, "--destination")
	}
	for _, role := range parsed.roles {
		if !canonicalSeedIdentity(role) {
			missing = append(missing, "--role")
			break
		}
	}
	if len(missing) > 0 {
		return lifecycleSeedArgs{}, fmt.Errorf("required canonical lifecycle seed options are missing or invalid: %s", strings.Join(missing, ", "))
	}
	if strings.TrimSpace(parsed.reason) == "" || strings.TrimSpace(parsed.reason) != parsed.reason ||
		len(parsed.reason) > lifecycleSeedMaxReasonBytes || strings.ContainsFunc(parsed.reason, isSeedControl) {
		return lifecycleSeedArgs{}, fmt.Errorf("--reason is required: 1 to %d bytes of text without surrounding whitespace or control characters", lifecycleSeedMaxReasonBytes)
	}
	switch parsed.validate {
	case seedValidateReal:
	case seedValidateSkip:
		if len(parsed.reason) < lifecycleSeedMinSkipReason {
			return lifecycleSeedArgs{}, fmt.Errorf("--validate skip records %s instead of a connection check and requires a --reason of at least %d bytes", seedCodeSkipped, lifecycleSeedMinSkipReason)
		}
	default:
		return lifecycleSeedArgs{}, fmt.Errorf("--validate must be %q or %q", seedValidateReal, seedValidateSkip)
	}
	if parsed.through != integration.DeploymentStatePublished && parsed.through != integration.DeploymentStateDeployed {
		return lifecycleSeedArgs{}, fmt.Errorf("--through must be %q or %q", integration.DeploymentStatePublished, integration.DeploymentStateDeployed)
	}
	stdinDestinations := 0
	for _, path := range parsed.destinationPaths {
		if path == lifecycleSeedStdinPath {
			stdinDestinations++
		}
	}
	if stdinDestinations > 1 {
		return lifecycleSeedArgs{}, fmt.Errorf("at most one --destination may be read from standard input")
	}
	if parsed.sourcePath == lifecycleSeedStdinPath {
		return lifecycleSeedArgs{}, fmt.Errorf("--source must name a file; the runner mounts the same file")
	}
	if err := parsed.policy.Validate(); err != nil {
		return lifecycleSeedArgs{}, fmt.Errorf("deployment policy: %w", err)
	}
	return parsed, nil
}

func defaultSeedDeploymentPolicy() integration.IntegrationDeploymentPolicy {
	return authoring.DefaultDeploymentPolicy(authoring.DefaultValidationMaxAgeSeconds)
}

func parseSeedPositiveInt(name, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return 0, fmt.Errorf("%s must be a positive decimal integer", name)
	}
	return parsed, nil
}

func parseSeedPositiveIntAsInt(name, value string) (int, error) {
	parsed, err := parseSeedPositiveInt(name, value)
	if err != nil || parsed > 1<<31-1 {
		return 0, fmt.Errorf("%s must be a positive decimal integer", name)
	}
	return int(parsed), nil
}

func canonicalSeedIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsFunc(value, isSeedControl)
}

func isSeedControl(character rune) bool {
	return character < 0x20 || character == 0x7f
}

func seedPrincipal(args lifecycleSeedArgs) integration.Principal {
	return integration.Principal{
		ID: args.principalID, Kind: integration.PrincipalKindHuman,
		AuthMethod: lifecycleSeedAuthMethod, Roles: append([]string(nil), args.roles...),
	}
}

func loadLifecycleSeedInputs(ctx context.Context, parsed lifecycleSeedArgs, stdin io.Reader) (lifecycleSeedInputs, error) {
	inputs := lifecycleSeedInputs{args: parsed, principal: seedPrincipal(parsed)}
	sourceRaw, err := readSeedFile(parsed.sourcePath, stdin, "batch source revision")
	if err != nil {
		return lifecycleSeedInputs{}, err
	}
	inputs.source, err = integrationbatch.DecodeSourceRevision(bytes.NewReader(sourceRaw))
	if err != nil {
		return lifecycleSeedInputs{}, fmt.Errorf("--source is not a valid batch source revision (DecodeSourceRevision): %w", err)
	}
	var workflowYAML []byte
	inputs.profile, inputs.workflow, workflowYAML, err = resolveSeedRegistryArtifacts(ctx, parsed)
	if err != nil {
		return lifecycleSeedInputs{}, err
	}
	seen := make(map[string]struct{}, len(parsed.destinationPaths))
	for _, path := range parsed.destinationPaths {
		raw, err := readSeedFile(path, stdin, "destination revision")
		if err != nil {
			return lifecycleSeedInputs{}, err
		}
		revision, err := integrationdestination.DecodeRevision(bytes.NewReader(raw))
		if err != nil {
			return lifecycleSeedInputs{}, fmt.Errorf("--destination %s is not a valid destination revision (DecodeRevision): %w", seedPathLabel(path), err)
		}
		if _, duplicate := seen[revision.ArtifactID]; duplicate {
			return lifecycleSeedInputs{}, fmt.Errorf("destination artifact %q is given more than once", revision.ArtifactID)
		}
		seen[revision.ArtifactID] = struct{}{}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return lifecycleSeedInputs{}, fmt.Errorf("--destination %s: %w", seedPathLabel(path), err)
		}
		inputs.destinations = append(inputs.destinations, seedDestination{revision: revision, raw: compact.Bytes()})
	}
	if err := requireWorkflowDestinations(inputs.workflow, workflowYAML, seen); err != nil {
		return lifecycleSeedInputs{}, err
	}
	return inputs, nil
}

// resolveSeedRegistryArtifacts returns the profile and workflow refs of one
// static-registry entry, and the workflow bytes, after resolving them with the
// same processor.RevisionResolver construction the batch runtime uses.
func resolveSeedRegistryArtifacts(
	ctx context.Context,
	parsed lifecycleSeedArgs,
) (integration.ArtifactRevisionRef, integration.ArtifactRevisionRef, []byte, error) {
	none := integration.ArtifactRevisionRef{}
	staticRegistry, err := loadSeedRegistry(parsed.registryPath)
	if err != nil {
		return none, none, nil, err
	}
	proven, err := authoring.NewRegistry(parsed.tenantID, staticRegistry)
	if err != nil {
		return none, none, nil, err
	}
	artifact, err := proven.Artifact(ctx, parsed.integrationID)
	if errors.Is(err, authoring.ErrUnknownIntegration) {
		return none, none, nil, fmt.Errorf("--integration %q is not in the integration registry", parsed.integrationID)
	}
	if err != nil {
		return none, none, nil, err
	}
	return artifact.Profile, artifact.Workflow, artifact.WorkflowYAML, nil
}

func loadSeedRegistry(path string) (*registry.StaticRegistry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open integration registry: %w", err)
	}
	staticRegistry, decodeErr := registry.DecodeStaticRegistry(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, fmt.Errorf("load integration registry: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close integration registry: %w", closeErr)
	}
	return staticRegistry, nil
}

// requireWorkflowDestinations is the planner's binding rule, shared with the
// definition editor (authoring.RequireWorkflowDestinations).
func requireWorkflowDestinations(
	workflowRef integration.ArtifactRevisionRef,
	workflowYAML []byte,
	destinations map[string]struct{},
) error {
	return authoring.RequireWorkflowDestinations(workflowRef, workflowYAML, destinations)
}

func readSeedFile(path string, stdin io.Reader, label string) ([]byte, error) {
	const maxSeedFileBytes = 1 << 20
	var reader io.Reader
	if path == lifecycleSeedStdinPath {
		if stdin == nil {
			return nil, fmt.Errorf("read %s: standard input is unavailable", label)
		}
		reader = stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", label, err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSeedFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if len(raw) == 0 || len(raw) > maxSeedFileBytes {
		return nil, fmt.Errorf("read %s: must contain between 1 and %d bytes", label, maxSeedFileBytes)
	}
	return raw, nil
}

func seedPathLabel(path string) string {
	if path == lifecycleSeedStdinPath {
		return "(standard input)"
	}
	return path
}

// buildSeedDefinition is deterministic: equal inputs and an equal creation
// audit produce an equal digest. The creation audit is part of the digest, so
// resume rebuilds the candidate under the stored revision's audit to compare.
func buildSeedDefinition(inputs lifecycleSeedInputs, created integration.AuditEnvelope) (integration.IntegrationDefinitionRevision, error) {
	source := authoring.BatchSource(inputs.source)
	destinations := make([]authoring.Destination, 0, len(inputs.destinations))
	for _, destination := range inputs.destinations {
		destinations = append(destinations, authoring.DestinationOf(destination.revision))
	}
	bindings, err := seedDefinitionSecretBindings(source, destinations)
	if err != nil {
		return integration.IntegrationDefinitionRevision{}, err
	}
	return authoring.BuildDefinition(authoring.Draft{
		DefinitionID: inputs.args.definitionID, RevisionID: inputs.args.revisionID, TenantID: inputs.args.tenantID,
		Source: source, Profile: inputs.profile, Workflow: inputs.workflow,
		Destinations: destinations, SecretBindings: bindings,
		Policy:     authoring.DefaultPolicy(),
		Deployment: inputs.args.policy,
		Created:    created,
	})
}

// seedDefinitionSecretBindings binds every name the source declares (file key
// batch/<name>, as the batch proof does) and every name a destination declares
// (file key destinations/<name>, the same reference the destination registry
// carries), because destination.Revision.ValidateAgainst requires a deployed
// release to name each of them too.
func seedDefinitionSecretBindings(source authoring.Source, destinations []authoring.Destination) ([]integration.SecretBinding, error) {
	return authoring.FileBindings(source, destinations)
}

func sortedSeedBindings(byName map[string]integration.SecretBinding) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, len(byName))
	for _, binding := range byName {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Name < bindings[j].Name })
	return bindings
}

func seedRefs(revision integration.IntegrationDefinitionRevision) lifecycleSeedRefs {
	return lifecycleSeedRefs{
		Definition: revision.Reference(), Source: revision.Source,
		Profile: revision.Profile, Workflow: revision.Workflow,
		Destinations: append([]integration.DestinationRevisionRef(nil), revision.Destinations...),
	}
}

func seedEnv(definitionID string) map[string]string {
	return map[string]string{"FI_FHIR_BATCH_DEFINITION_ID": definitionID}
}

func writeLifecycleSeedDryRun(stdout io.Writer, inputs lifecycleSeedInputs, revision integration.IntegrationDefinitionRevision) error {
	output := lifecycleSeedDryRun{
		DryRun: true, Definition: revision, Refs: seedRefs(revision),
		Env: seedEnv(revision.DefinitionID),
	}
	if inputs.args.registryOut != "" {
		document, err := seedDestinationRegistry(inputs, revision.Reference())
		if err != nil {
			return err
		}
		output.DestinationRegistry = document
	}
	return writeIndentedJSON(stdout, output)
}

// seedDestinationRegistry builds the delivery worker's registry document for
// this definition (internal/integration/destination/registry.go) and proves it
// loads with the worker's own loader before returning it.
func seedDestinationRegistry(inputs lifecycleSeedInputs, definition integration.ArtifactRevisionRef) (json.RawMessage, error) {
	byName := make(map[string]integration.SecretBinding)
	document := seedDestinationRegistryDocument{
		Schema: lifecycleSeedDestinationSchema, TenantID: inputs.args.tenantID,
		IntegrationRevision: definition,
	}
	allBound := true
	for _, destination := range inputs.destinations {
		document.Destinations = append(document.Destinations, destination.raw)
		allBound = allBound && destination.revision.IdentityBound()
		for _, name := range destination.revision.SecretBindingNames() {
			byName[name] = integration.SecretBinding{
				Name:      name,
				Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: lifecycleSeedDestKeyPrefix + name},
			}
		}
	}
	document.SecretBindings = sortedSeedBindings(byName)
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal destination registry: %w", err)
	}
	mode := integrationdestination.ModeCompatibility
	if allBound {
		mode = integrationdestination.ModeStrict
	}
	if _, err := integrationdestination.LoadRegistry(bytes.NewReader(raw), mode); err != nil {
		return nil, fmt.Errorf("destination registry does not load in %s mode: %w", mode, err)
	}
	return raw, nil
}

func writeIndentedJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write lifecycle seed output: %w", err)
	}
	return nil
}

// seedLifecycle creates the draft or resumes an identical one, then advances it
// to --through. It returns a summary whenever the definition exists, including
// when a transition is refused, so a failed validation still reports its codes.
func seedLifecycle(
	ctx context.Context,
	catalog seedCatalog,
	inputs lifecycleSeedInputs,
	candidate integration.IntegrationDefinitionRevision,
	now func() time.Time,
) (lifecycleSeedSummary, error) {
	args := inputs.args
	summary := lifecycleSeedSummary{
		TenantID: args.tenantID, Transitions: []string{}, Env: seedEnv(args.definitionID),
		Validation: seedValidationSummary{Codes: []string{}},
	}
	snapshot, err := catalog.GetSnapshot(ctx, args.tenantID, args.definitionID, args.revisionID)
	switch {
	case errors.Is(err, lifecycle.ErrNotFound):
		snapshot, err = catalog.CreateDraft(ctx, candidate)
		if err == nil {
			summary.CreatedDraft = true
			summary.Transitions = append(summary.Transitions, "create_draft")
			summary.DefinitionCreatedAt = candidate.Created.OccurredAt
			break
		}
		if !errors.Is(err, lifecycle.ErrAlreadyExists) {
			return summary, fmt.Errorf("create draft: %w", err)
		}
		// Another writer created it between the read and the insert; compare.
		snapshot, err = catalog.GetSnapshot(ctx, args.tenantID, args.definitionID, args.revisionID)
		if err != nil {
			return summary, fmt.Errorf("load lifecycle snapshot: %w", err)
		}
		fallthrough
	case err == nil:
		createdAt, err := verifyExistingSeedDefinition(ctx, catalog, inputs, snapshot)
		if err != nil {
			return summary, err
		}
		summary.DefinitionCreatedAt = createdAt
	default:
		return summary, fmt.Errorf("load lifecycle snapshot: %w", err)
	}
	summary.Definition = snapshot.DefinitionRevision

	run := seedRun{catalog: catalog, now: now, args: args, summary: &summary}
	snapshot, runErr := run.advance(ctx, snapshot)
	summary.State = snapshot.State
	summary.SnapshotVersion = snapshot.Version
	summary.ReleaseID = snapshot.ReleaseID
	if snapshot.LastValidationID != "" {
		if record, err := catalog.GetValidation(ctx, snapshot.LastValidationID); err == nil {
			summary.Validation = seedValidationSummary{
				Passed: record.Passed, Codes: append([]string{}, record.Codes...),
				CheckedAt: seedTime(record.CheckedAt), ExpiresAt: seedTime(record.ExpiresAt),
			}
		} else if runErr == nil {
			runErr = fmt.Errorf("load validation evidence: %w", err)
		}
	}
	if runErr == nil {
		summary.Next = seedNextStep(snapshot, args)
	}
	return summary, runErr
}

func seedTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func seedNextStep(snapshot lifecycle.Snapshot, args lifecycleSeedArgs) string {
	switch snapshot.State {
	case integration.DeploymentStatePublished:
		return fmt.Sprintf(
			"deploy release %s from the Studio Operator page (deployIntegrationRelease) before validation.expires_at, or re-run this command with --through deployed; after expiry, re-run this command to refresh validation",
			snapshot.ReleaseID,
		)
	case integration.DeploymentStateDeployed:
		return fmt.Sprintf("the batch runner polling as FI_FHIR_BATCH_DEFINITION_ID=%s ingests from its next poll", args.definitionID)
	default:
		return ""
	}
}

// verifyExistingSeedDefinition refuses to reuse a revision whose content differs
// from these inputs. The tables are append-only, so a different revision needs a
// new --revision-id; nothing is ever overwritten.
func verifyExistingSeedDefinition(
	ctx context.Context,
	catalog seedCatalog,
	inputs lifecycleSeedInputs,
	snapshot lifecycle.Snapshot,
) (time.Time, error) {
	args := inputs.args
	raw, err := catalog.LoadDefinitionRevision(ctx, args.tenantID, args.definitionID, args.revisionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load existing definition revision: %w", err)
	}
	stored, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("existing definition revision %s/%s does not decode: %w", args.definitionID, args.revisionID, err)
	}
	expected, err := buildSeedDefinition(inputs, stored.Created)
	if err != nil {
		return time.Time{}, err
	}
	if expected.Digest != stored.Digest || snapshot.DefinitionRevision.Digest != stored.Digest {
		return time.Time{}, fmt.Errorf(
			"%w: %s/%s is stored with digest %s, and these inputs produce %s under its creation audit; revisions are append-only, so seed the change as a new --revision-id",
			errLifecycleSeedConflict, args.definitionID, args.revisionID, stored.Digest, expected.Digest,
		)
	}
	return stored.Created.OccurredAt.UTC(), nil
}

type seedRun struct {
	catalog seedCatalog
	now     func() time.Time
	args    lifecycleSeedArgs
	summary *lifecycleSeedSummary
}

func (r seedRun) command(snapshot lifecycle.Snapshot) lifecycle.Command {
	return lifecycle.Command{
		TenantID: r.args.tenantID, DefinitionID: r.args.definitionID, RevisionID: r.args.revisionID,
		ExpectedVersion: snapshot.Version, Principal: seedPrincipal(r.args), Reason: r.args.reason,
	}
}

func (r seedRun) advance(ctx context.Context, snapshot lifecycle.Snapshot) (lifecycle.Snapshot, error) {
	// Each iteration performs at most one transition; the bound is the longest
	// path draft -> validated -> approved -> published -> deployed plus a
	// validation refresh before each gated step.
	for step := 0; step < 10; step++ {
		var err error
		switch snapshot.State {
		case integration.DeploymentStateDraft:
			snapshot, err = r.validate(ctx, snapshot)
		case integration.DeploymentStateValidated:
			snapshot, err = r.gated(ctx, snapshot, "approve", r.catalog.Approve)
		case integration.DeploymentStateApproved:
			snapshot, err = r.gated(ctx, snapshot, "publish", r.catalog.Publish)
		case integration.DeploymentStatePublished:
			if r.args.through == integration.DeploymentStatePublished {
				// Refresh stale evidence so a Studio deploy can follow within the
				// policy's max age; the deploy itself stays an operator action.
				if !r.validationCurrent(snapshot) {
					return r.validate(ctx, snapshot)
				}
				return snapshot, nil
			}
			snapshot, err = r.gated(ctx, snapshot, "deploy", r.catalog.Deploy)
		case integration.DeploymentStateDeployed:
			return snapshot, nil
		case integration.DeploymentStatePaused:
			return snapshot, fmt.Errorf("%s/%s is paused; resume it from the Studio Operator page (resumeIntegrationDeployment)", r.args.definitionID, r.args.revisionID)
		case integration.DeploymentStateRetired:
			return snapshot, fmt.Errorf("%s/%s is retired and can never run again; seed a new --revision-id", r.args.definitionID, r.args.revisionID)
		default:
			return snapshot, fmt.Errorf("%s/%s is in unknown lifecycle state %q", r.args.definitionID, r.args.revisionID, snapshot.State)
		}
		if err != nil {
			return snapshot, err
		}
	}
	return snapshot, fmt.Errorf("lifecycle seed did not converge; re-run the same command to resume")
}

// validationCurrent is the catalog's own freshness rule with a margin, so the
// seed re-validates rather than racing an expiry between its read and the
// transition. The margin never exceeds half the policy's max age.
func (r seedRun) validationCurrent(snapshot lifecycle.Snapshot) bool {
	margin := lifecycleSeedValidationMargin
	if half := time.Duration(r.args.policy.ConnectionValidation.MaxAgeSeconds) * time.Second / 2; half < margin {
		margin = half
	}
	return snapshot.ValidationPassed && snapshot.LastValidationID != "" &&
		snapshot.ValidationExpiresAt.After(r.now().Add(margin))
}

func (r seedRun) validate(ctx context.Context, snapshot lifecycle.Snapshot) (lifecycle.Snapshot, error) {
	next, err := r.catalog.ValidateConnection(ctx, r.command(snapshot))
	if next.Version > snapshot.Version {
		// A failed check is still recorded evidence and advances the version.
		snapshot = next
		r.summary.Transitions = append(r.summary.Transitions, "validate_connection")
	}
	if errors.Is(err, lifecycle.ErrConnectionValidationFailed) {
		return snapshot, fmt.Errorf("connection validation failed for source %s; see validation.codes, fix the source, and re-run the same command to resume", r.args.sourcePath)
	}
	if err != nil {
		return snapshot, seedTransitionError("validate connection", r.args, err)
	}
	return snapshot, nil
}

func (r seedRun) gated(
	ctx context.Context,
	snapshot lifecycle.Snapshot,
	action string,
	transition func(context.Context, lifecycle.Command) (lifecycle.Snapshot, error),
) (lifecycle.Snapshot, error) {
	if !r.validationCurrent(snapshot) {
		return r.validate(ctx, snapshot)
	}
	next, err := transition(ctx, r.command(snapshot))
	if err != nil {
		return snapshot, seedTransitionError(action, r.args, err)
	}
	r.summary.Transitions = append(r.summary.Transitions, action)
	return next, nil
}

func seedTransitionError(action string, args lifecycleSeedArgs, err error) error {
	switch {
	case errors.Is(err, lifecycle.ErrActiveDeployment):
		return fmt.Errorf("%s: another revision of definition %s is deployed or paused; retire it on the Studio Operator page first: %w", action, args.definitionID, err)
	case errors.Is(err, lifecycle.ErrVersionConflict):
		return fmt.Errorf("%s: another writer advanced %s/%s; re-run the same command to resume: %w", action, args.definitionID, args.revisionID, err)
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func skipConnectionValidator() lifecycle.ConnectionValidatorFunc {
	return authoring.SkipValidator()
}

type batchProviderFactory = authoring.BatchProviderFactory

// batchConnectionValidator is the shared batch validator
// (authoring.BatchValidator) the definition editor hosts in serve too.
func batchConnectionValidator(
	source integrationbatch.SourceRevision,
	secrets batchProviderSecrets,
	build batchProviderFactory,
	stderr io.Writer,
) lifecycle.ConnectionValidatorFunc {
	return authoring.BatchValidator(source, secrets, build, stderr)
}

func printLifecycleUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `fi-fhir lifecycle - Seed an integration definition into the lifecycle catalog

Usage:
  fi-fhir lifecycle seed \
    --source FILE --definition-id ID [--revision-id ID] \
    --integration ID [--registry FILE] \
    --destination FILE [--destination FILE ...] \
    --principal ID --reason TEXT [--role ROLE ...] \
    [--tenant ID] [--validate real|skip] [--through published|deployed] \
    [--dry-run] [--destination-registry-out FILE|-] [--created-at RFC3339] \
    [--validation-timeout SECONDS] [--validation-max-age SECONDS] \
    [--max-in-flight N] [--max-queued N] [--max-messages-per-second N]

Builds one batch integration definition revision and runs CreateDraft ->
ValidateConnection -> Approve -> Publish [-> Deploy] against the PostgreSQL
lifecycle catalog, each at the expected snapshot version under --principal and
--reason. Re-running with the same inputs resumes from the stored state; a
revision stored with different content is refused, never overwritten.

Inputs:
  --source FILE          Batch source revision JSON, the file the runner mounts
                         (FI_FHIR_BATCH_SOURCE_CONFIG_PATH)
  --definition-id ID     Definition the runner polls (FI_FHIR_BATCH_DEFINITION_ID)
  --revision-id ID       Definition revision (default v1)
  --integration ID       Static-registry entry whose profile and workflow refs
                         the definition binds; the batch runner loads those
                         bytes from this registry, not from the Studio's stores
  --registry FILE        Static registry (default FI_FHIR_INTEGRATION_REGISTRY_PATH)
  --destination FILE     Destination revision JSON, repeatable; "-" reads one
                         from standard input. Every non-log workflow action
                         must deliver to one of these
  --principal ID         Operator identity recorded on every transition
  --reason TEXT          Audit reason, 1-1024 bytes
  --role ROLE            Principal role, repeatable (default integration:operator)
  --tenant ID            Deployment tenant (default FI_FHIR_DEPLOYMENT_TENANT_ID)

Behaviour:
  --validate real|skip   real (default) builds the batch provider from the
                         FI_FHIR_BATCH_* keys serve reads and lists one object
                         of the input location; skip records VALIDATION_SKIPPED
                         and requires a --reason of at least 16 bytes
  --through STATE        published (default): stop after Publish and deploy
                         from the Studio Operator page within the validation
                         max age; deployed: continue through Deploy
  --dry-run              Print the definition revision and its refs; write nothing
  --destination-registry-out FILE
                         Also write the delivery worker's destination registry
                         (fi-fhir/destination-registry/v1) for this definition;
                         "-" puts it in the stdout summary instead
  --created-at RFC3339   Creation time recorded in (and digested by) a new
                         revision (default now); fix it to reproduce a digest

Deployment policy (defaults): validation timeout 5 s, max age 300 s; schedule
continuous; health startup grace 5 s, interval 30 s, timeout 5 s, failure
threshold 3; capacity in-flight 2, queued 10, 100 messages/s.

Requires FI_FHIR_DATABASE_* PostgreSQL settings (the ones serve reads) unless
--dry-run. Prints one JSON summary on stdout: the definition ref, final state,
validation codes, and the FI_FHIR_BATCH_DEFINITION_ID value.`)
}
