//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The lifecycle seed proofs (ci/test-lifecycle-seed.yml) drive
// runLifecycleSeed exactly as `fi-fhir lifecycle seed` does — the
// FI_FHIR_DATABASE_* loader serve uses, real connection validation against an
// in-process SSH/SFTP server with a pinned host key — against a fresh
// PostgreSQL 16 database per test, then hand the result to the batch runtime
// serve builds (loadBatchRuntimeFromEnv) and require it to ingest.

// seedPostgres creates an isolated database on POSTGRES_TEST_URL, points the
// FI_FHIR_DATABASE_* settings at it, and returns a connection to it.
func seedPostgres(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("set POSTGRES_TEST_URL to a disposable PostgreSQL 16 server")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.User == nil {
		t.Fatal("POSTGRES_TEST_URL must be a PostgreSQL URL with host and user")
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal("open POSTGRES_TEST_URL")
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.PingContext(ctx); err != nil {
		t.Fatal("POSTGRES_TEST_URL is unreachable")
	}
	name := fmt.Sprintf("lifecycle_seed_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)); err != nil {
		t.Fatalf("create isolated database: %v", err)
	}
	isolated := *parsed
	isolated.Path = "/" + name
	db, err := sql.Open("postgres", isolated.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+pq.QuoteIdentifier(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated database: %v", err)
		}
	})

	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	password, _ := parsed.User.Password()
	sslMode := parsed.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "require"
	}
	for key, value := range map[string]string{
		"FI_FHIR_DATABASE_DRIVER": "postgres", "FI_FHIR_DATABASE_HOST": parsed.Hostname(),
		"FI_FHIR_DATABASE_PORT": port, "FI_FHIR_DATABASE_NAME": name,
		"FI_FHIR_DATABASE_USERNAME": parsed.User.Username(), "FI_FHIR_DATABASE_PASSWORD": password,
		"FI_FHIR_DATABASE_SSL_MODE": sslMode,
	} {
		t.Setenv(key, value)
	}
	return db
}

type seedProof struct {
	server  *seedSFTPServer
	input   string
	archive string
	fixture seedFixture
	db      *sql.DB
	catalog *lifecycle.PostgresCatalog
}

func newSeedProof(t *testing.T, ctx context.Context) *seedProof {
	t.Helper()
	isolateSeedEnv(t)
	proof := &seedProof{db: seedPostgres(t, ctx), server: startSeedSFTPServer(t)}
	proof.input = filepath.Join(proof.server.root, "inbound")
	proof.archive = filepath.Join(proof.server.root, "archive")
	for _, directory := range []string{proof.input, proof.archive} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	proof.fixture = newSeedFixture(t)
	proof.fixture.source = seedTestSource(t, proof.server.host, proof.server.port, proof.input, proof.archive, 1)
	proof.fixture.sourcePath = writeSeedJSON(t, proof.fixture.dir, "sftp-source.json", proof.fixture.source)
	t.Setenv("FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE", proof.server.knownHosts)
	t.Setenv("FI_FHIR_BATCH_SFTP_PASSWORD", seedSFTPPassword)
	catalog, err := lifecycle.NewPostgresCatalog(proof.db, lifecycle.Config{})
	if err != nil {
		t.Fatal(err)
	}
	proof.catalog = catalog
	return proof
}

func (p *seedProof) seed(t *testing.T, ctx context.Context, extra ...string) (lifecycleSeedSummary, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runLifecycleSeed(ctx, p.fixture.args(extra...), &stdout, strings.NewReader(""), &stderr)
	var summary lifecycleSeedSummary
	if stdout.Len() > 0 {
		if decodeErr := json.Unmarshal(stdout.Bytes(), &summary); decodeErr != nil {
			t.Fatalf("summary is not JSON: %v\n%s", decodeErr, stdout.String())
		}
	}
	return summary, stdout.String(), err
}

func seedProofCount(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func seedBatchFile() []byte {
	message := func(controlID string) string {
		return strings.Join([]string{
			"MSH|^~\\&|SEEDADT|FAC|APP|FAC|20260928120000||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
			"EVN|A01|20260928120000",
			"PID|1||MRN-SEED-001^^^HOSP^MR||Patient^Seed||19800101|F",
			"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-seed-1|||||||||||||||||||||||||20260928120000",
		}, "\r")
	}
	return []byte(message("seed-control-001") + "\r" + message("seed-control-002"))
}

func TestLifecycleSeedPostgres_PublishesThenResumesToDeployedAndTheRunnerIngests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	proof := newSeedProof(t, ctx)
	if err := os.WriteFile(filepath.Join(proof.input, "adt-batch.hl7"), seedBatchFile(), 0o600); err != nil {
		t.Fatal(err)
	}

	published, stdout, err := proof.seed(t, ctx)
	if err != nil {
		t.Fatalf("seed through published: %v", err)
	}
	if !published.CreatedDraft || published.State != integration.DeploymentStatePublished || published.ReleaseID == "" ||
		!published.Validation.Passed || strings.Join(published.Validation.Codes, ",") != "SOURCE_REACHABLE,HOST_KEY_VERIFIED,AUTH_OK,INPUT_LISTED" ||
		strings.Join(published.Transitions, ",") != "create_draft,validate_connection,approve,publish" ||
		published.Env["FI_FHIR_BATCH_DEFINITION_ID"] != seedTestDefinition {
		t.Fatalf("published summary = %s", stdout)
	}
	for _, forbidden := range []string{seedSFTPPassword, "MRN-SEED", os.Getenv("FI_FHIR_DATABASE_PASSWORD")} {
		if forbidden != "" && strings.Contains(stdout, forbidden) {
			t.Fatalf("summary leaks %q", forbidden)
		}
	}
	if _, err := proof.catalog.ResolveRunnable(ctx, seedTestTenant, seedTestDefinition); !errors.Is(err, lifecycle.ErrNotFound) {
		t.Fatalf("a published definition is runnable: %v", err)
	}
	// Validation listed the input and moved nothing.
	if _, err := os.Stat(filepath.Join(proof.input, "adt-batch.hl7")); err != nil {
		t.Fatalf("validation touched the input: %v", err)
	}

	deployed, stdout, err := proof.seed(t, ctx, "--through", "deployed")
	if err != nil {
		t.Fatalf("resume through deployed: %v", err)
	}
	if deployed.CreatedDraft || deployed.State != integration.DeploymentStateDeployed ||
		strings.Join(deployed.Transitions, ",") != "deploy" || deployed.Definition != published.Definition ||
		deployed.ReleaseID != published.ReleaseID {
		t.Fatalf("deployed summary = %s", stdout)
	}

	binding, err := proof.catalog.ResolveRunnable(ctx, seedTestTenant, seedTestDefinition)
	if err != nil {
		t.Fatalf("resolve runnable: %v", err)
	}
	// The runner's own gate (batch service PollOnce) on the resolved binding.
	if err := proof.fixture.source.ValidateAgainst(binding); err != nil {
		t.Fatalf("deployed binding does not match the source: %v", err)
	}
	if binding.IntegrationRevision != deployed.Definition || binding.SourceID != proof.fixture.source.SourceID ||
		binding.ReleaseID != deployed.ReleaseID {
		t.Fatalf("binding = %+v", binding)
	}
	events, err := proof.catalog.ListEvents(ctx, seedTestTenant, seedTestDefinition, "v1")
	if err != nil {
		t.Fatal(err)
	}
	actions := make([]string, 0, len(events))
	for _, event := range events {
		actions = append(actions, event.Action)
		if event.Audit.Principal.ID != seedTestPrincipal || event.Audit.Reason != seedTestReason ||
			event.Audit.Principal.AuthMethod != "postgres" {
			t.Fatalf("lifecycle event %s audit = %+v", event.Action, event.Audit)
		}
	}
	if strings.Join(actions, ",") != "create_draft,validate_connection,approve,publish,deploy" {
		t.Fatalf("lifecycle history = %v", actions)
	}

	// The runtime serve mounts ingests from the seeded definition: the profile
	// and workflow come from the static registry, the definition from the catalog.
	submissions, err := processor.NewPostgresSubmissionStore(proof.db, processor.PostgresSubmissionConfig{})
	if err != nil || submissions.Migrate(ctx) != nil {
		t.Fatalf("submission migrations = %v", err)
	}
	staticRegistry, err := loadSeedRegistry(proof.fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	artifactResolver, err := processor.NewRevisionResolver(seedTestTenant, staticRegistry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FI_FHIR_BATCH_DEFINITION_ID", seedTestDefinition)
	t.Setenv("FI_FHIR_BATCH_PRINCIPAL_ID", "batch-ingest")
	t.Setenv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY", "true")
	t.Setenv("FI_FHIR_BATCH_WORKER_ID", "lifecycle-seed-proof")
	runner, provider, _, err := loadBatchRuntimeFromEnv(ctx, seedTestTenant, proof.fixture.sourcePath, proof.db, artifactResolver)
	if err != nil {
		t.Fatalf("batch runtime refuses the seeded definition: %v", err)
	}
	defer func() { _ = provider.Close() }()
	processed, err := runner.PollOnce(ctx)
	if err != nil || processed != 1 {
		t.Fatalf("poll = %d, %v", processed, err)
	}
	if receipts := seedProofCount(t, ctx, proof.db,
		`SELECT count(*) FROM integration_receipts WHERE tenant_id = $1`, seedTestTenant); receipts != 2 {
		t.Fatalf("receipts = %d, want 2", receipts)
	}
	if outbox := seedProofCount(t, ctx, proof.db,
		`SELECT count(*) FROM integration_delivery_outbox WHERE tenant_id = $1`, seedTestTenant); outbox != 2 {
		t.Fatalf("delivery outbox = %d, want 2", outbox)
	}
	if _, err := os.Stat(filepath.Join(proof.input, "adt-batch.hl7")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("input was not archived: %v", err)
	}
	archived, err := filepath.Glob(filepath.Join(proof.archive, "*", "adt-batch.hl7"))
	if err != nil || len(archived) != 1 {
		t.Fatalf("archive = %v, %v", archived, err)
	}
}

func TestLifecycleSeedPostgres_RefusesAChangedSourceUnderTheSameRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	proof := newSeedProof(t, ctx)
	published, _, err := proof.seed(t, ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	countRows := func() (int, int, int) {
		return seedProofCount(t, ctx, proof.db, `SELECT count(*) FROM integration_definition_revisions`),
			seedProofCount(t, ctx, proof.db, `SELECT count(*) FROM integration_lifecycle_events`),
			seedProofCount(t, ctx, proof.db, `SELECT count(*) FROM integration_connection_validations`)
	}
	revisions, events, validations := countRows()
	snapshotBefore, err := proof.catalog.GetSnapshot(ctx, seedTestTenant, seedTestDefinition, "v1")
	if err != nil {
		t.Fatal(err)
	}

	// Same definition and revision IDs, a source that polls at another interval:
	// a different content address.
	mutated := seedTestSource(t, proof.server.host, proof.server.port, proof.input, proof.archive, 2)
	proof.fixture.sourcePath = writeSeedJSON(t, proof.fixture.dir, "mutated-source.json", mutated)
	summary, stdout, err := proof.seed(t, ctx, "--through", "deployed")
	if !errors.Is(err, errLifecycleSeedConflict) || !strings.Contains(err.Error(), published.Definition.Digest) {
		t.Fatalf("mutated source = %v", err)
	}
	if stdout != "" || summary.Definition.ArtifactID != "" {
		t.Fatalf("refusal printed a summary: %s", stdout)
	}
	if r, e, v := countRows(); r != revisions || e != events || v != validations {
		t.Fatalf("refusal wrote rows: revisions %d->%d events %d->%d validations %d->%d", revisions, r, events, e, validations, v)
	}
	snapshotAfter, err := proof.catalog.GetSnapshot(ctx, seedTestTenant, seedTestDefinition, "v1")
	if err != nil || snapshotAfter.Version != snapshotBefore.Version || snapshotAfter.State != integration.DeploymentStatePublished {
		t.Fatalf("refusal moved the snapshot: %+v, %v", snapshotAfter, err)
	}
	if _, err := proof.catalog.ResolveRunnable(ctx, seedTestTenant, seedTestDefinition); !errors.Is(err, lifecycle.ErrNotFound) {
		t.Fatalf("refused seed left a runnable definition: %v", err)
	}
}

func TestLifecycleSeedPostgres_FailedValidationStaysDraftAndResumesAfterTheFix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	proof := newSeedProof(t, ctx)
	t.Setenv("FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE", wrongSeedKnownHosts(t, proof.server.address))
	failed, _, err := proof.seed(t, ctx)
	if err == nil || !strings.Contains(err.Error(), "connection validation failed") {
		t.Fatalf("pinned-key mismatch = %v", err)
	}
	if failed.State != integration.DeploymentStateDraft || failed.Validation.Passed ||
		strings.Join(failed.Validation.Codes, ",") != "SOURCE_CONNECT_FAILED" {
		t.Fatalf("failed summary = %+v", failed)
	}
	var stored []byte
	if err := proof.db.QueryRowContext(ctx,
		`SELECT codes::text FROM integration_connection_validations ORDER BY checked_at DESC LIMIT 1`,
	).Scan(&stored); err != nil || !bytes.Contains(stored, []byte("SOURCE_CONNECT_FAILED")) {
		t.Fatalf("stored failure evidence = %s, %v", stored, err)
	}

	t.Setenv("FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE", proof.server.knownHosts)
	resumed, _, err := proof.seed(t, ctx)
	if err != nil || resumed.CreatedDraft || resumed.State != integration.DeploymentStatePublished ||
		resumed.Definition != failed.Definition || !resumed.Validation.Passed {
		t.Fatalf("resume after fixing the host key = %+v, %v", resumed, err)
	}
	if strings.Join(resumed.Transitions, ",") != "validate_connection,approve,publish" {
		t.Fatalf("resume transitions = %v", resumed.Transitions)
	}
}
