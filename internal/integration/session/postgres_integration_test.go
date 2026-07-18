//go:build integration

package session

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
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	strictSessionProfile   = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`
	tolerantSessionProfile = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`
	rawSessionPHI          = "MSH|^~\\&|APP|FAC|EHR|HOSPITAL|20240115120000||ADT^A01|MSG00001|P|2.5\n" +
		"PID|1||RAW-PHI-SENTINEL^^^HOSPITAL^MRN||SENTINEL^PATIENT||19800315|M"
)

func TestPostgresSessionWorkspace_RestartExactProfilesAndRawPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db := openSessionPostgres(t, ctx)
	protector, err := NewAESGCMProtector(bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	store := newMigratedSessionStore(t, ctx, db, protector)

	workspace, err := store.CreateSession(ctx, CreateSessionRequest{Name: "Restart-safe ADT workspace"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	redacted, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "missing PV1", Format: events.FormatHL7v2, Source: "adt-feed", Raw: rawSessionPHI,
	})
	if err != nil {
		t.Fatalf("AddSample(redacted default): %v", err)
	}
	if redacted.PHIPolicy != PHIPolicyRedact || !redacted.PHIRedacted || strings.Contains(redacted.Raw, "RAW-PHI-SENTINEL") {
		t.Fatalf("redacted sample = %#v", redacted)
	}
	retained, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "explicit retained sample", Format: events.FormatHL7v2, Raw: rawSessionPHI,
		PHIPolicy: PHIPolicyRetain,
	})
	if err != nil {
		t.Fatalf("AddSample(explicit retain): %v", err)
	}
	if retained.Raw != rawSessionPHI {
		t.Fatal("retained sample did not round-trip through protector")
	}

	strict, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{
		Kind: ArtifactKindMappingProfile, Name: "adt-profile", Content: json.RawMessage(strictSessionProfile),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(strict): %v", err)
	}
	strictRun, err := NewRunner(store, NewHub()).RunHL7v2(ctx, RunRequest{
		SessionID: workspace.ID, SampleID: redacted.ID, ProfileRevisionID: strict.RevisionID,
	})
	if err == nil || strictRun == nil || strictRun.Status != RunStatusFailed {
		t.Fatalf("strict run = %#v, err = %v", strictRun, err)
	}
	assertExactRunProfile(t, strictRun, strict)
	if _, err := store.UpdateRun(ctx, *strictRun); !errors.Is(err, ErrImmutable) {
		t.Fatalf("terminal UpdateRun error = %v, want ErrImmutable", err)
	}

	// Complete restart: discard every service/store/runner object and reconstruct
	// from only the database and deployment retention key.
	store = newMigratedSessionStore(t, ctx, db, protector)
	reopened, err := store.GetSession(ctx, workspace.ID)
	if err != nil || reopened.ID != workspace.ID {
		t.Fatalf("GetSession after restart = %#v, %v", reopened, err)
	}
	retainedAfterRestart, err := store.GetSample(ctx, workspace.ID, retained.ID)
	if err != nil || retainedAfterRestart.Raw != rawSessionPHI {
		t.Fatalf("retained sample after restart = %#v, %v", retainedAfterRestart, err)
	}

	tolerant, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{
		ID: strict.ID, Kind: ArtifactKindMappingProfile, Name: "adt-profile",
		Content: json.RawMessage(tolerantSessionProfile),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(tolerant): %v", err)
	}
	if tolerant.Version != 2 || tolerant.RevisionID == strict.RevisionID || tolerant.Digest == strict.Digest {
		t.Fatalf("tolerant revision = %#v, strict = %#v", tolerant, strict)
	}
	tolerantRun, err := NewRunner(store, NewHub()).RunHL7v2(ctx, RunRequest{
		SessionID: workspace.ID, SampleID: redacted.ID, ProfileRevisionID: tolerant.RevisionID,
	})
	if err != nil || tolerantRun.Status != RunStatusSucceeded || len(tolerantRun.Events) != 1 {
		t.Fatalf("tolerant run = %#v, err = %v", tolerantRun, err)
	}
	assertExactRunProfile(t, tolerantRun, tolerant)
	if !diagnosticCode(tolerantRun.Diagnostics, "MISSING_PV1") {
		t.Fatalf("tolerant diagnostics = %#v", tolerantRun.Diagnostics)
	}

	baselineWorkflow, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{
		Kind: ArtifactKindWorkflowDraft, Name: "adt-routing", Content: json.RawMessage(`name: baseline
version: "1"
routes:
  - name: labs
    filter: {event_type: lab_result}
    actions:
      - id: log-lab
        type: log
`),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(baseline workflow): %v", err)
	}
	simulator := NewWorkflowSimulator(store)
	baselineSimulation, err := simulator.Simulate(ctx, SimulateWorkflowRequest{
		SessionID: workspace.ID, WorkflowRevisionID: baselineWorkflow.RevisionID,
		SourceRunIDs: []string{tolerantRun.ID},
	})
	if err != nil {
		t.Fatalf("Simulate(baseline workflow): %v", err)
	}
	trapPath := filepath.Join(t.TempDir(), "simulation-must-not-execute")
	candidateWorkflowYAML := fmt.Sprintf(`name: candidate
version: "2"
routes:
  - name: admits
    filter: {event_type: patient_admit}
    transform:
      - redact: {fields: [patient.identifiers]}
    actions:
      - id: notify
        type: exec
        destination: notification-sandbox
        command: touch %s
        secret: ACTION-CONFIG-SENTINEL
`, trapPath)
	candidateWorkflow, err := store.SaveArtifactDraft(ctx, workspace.ID, SaveArtifactDraftRequest{
		ID: baselineWorkflow.ID, Kind: ArtifactKindWorkflowDraft, Name: "adt-routing",
		Content: json.RawMessage(candidateWorkflowYAML),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(candidate workflow): %v", err)
	}
	candidateSimulation, err := simulator.Simulate(ctx, SimulateWorkflowRequest{
		SessionID: workspace.ID, WorkflowRevisionID: candidateWorkflow.RevisionID,
		SourceRunIDs: []string{tolerantRun.ID},
	})
	if err != nil {
		t.Fatalf("Simulate(candidate workflow): %v", err)
	}
	if _, err := os.Stat(trapPath); !os.IsNotExist(err) {
		t.Fatalf("workflow simulation executed side effect at %s: %v", trapPath, err)
	}

	// Reconstruct the store before reading/comparing simulation evidence. The
	// exact revision/run binding and PHI-minimal trace must survive restart.
	store = newMigratedSessionStore(t, ctx, db, protector)
	baselineAfterRestart, err := store.GetWorkflowSimulation(ctx, workspace.ID, baselineSimulation.ID)
	if err != nil {
		t.Fatalf("GetWorkflowSimulation(baseline after restart): %v", err)
	}
	candidateAfterRestart, err := store.GetWorkflowSimulation(ctx, workspace.ID, candidateSimulation.ID)
	if err != nil {
		t.Fatalf("GetWorkflowSimulation(candidate after restart): %v", err)
	}
	delta, err := CompareWorkflowSimulations(*baselineAfterRestart, *candidateAfterRestart)
	if err != nil || len(delta.AddedMatchedRoutes) != 1 || len(delta.AddedTransforms) != 1 || len(delta.AddedActions) != 1 {
		t.Fatalf("simulation delta = %#v, %v", delta, err)
	}
	simulationJSON, err := json.Marshal(candidateAfterRestart)
	if err != nil {
		t.Fatalf("marshal restored simulation: %v", err)
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", "ACTION-CONFIG-SENTINEL", trapPath, "SENTINEL^PATIENT"} {
		if strings.Contains(string(simulationJSON), forbidden) {
			t.Fatalf("restored simulation leaked %q: %s", forbidden, simulationJSON)
		}
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	publication, err := store.CreatePublication(ctx, workspace.ID, CreatePublicationRequest{
		ID: "publication-restart-fixture", ProfileArtifactID: tolerant.ID,
		ProfileRevisionID: tolerant.RevisionID, ProfileRevisionDigest: tolerant.Digest,
		WorkflowArtifactID: candidateWorkflow.ID, WorkflowRevisionID: candidateWorkflow.RevisionID,
		WorkflowRevisionDigest: candidateWorkflow.Digest, WorkflowSimulationID: candidateSimulation.ID,
		DefinitionRevision: integration.ArtifactRevisionRef{ArtifactID: "adt-http", RevisionID: "definition-1", Digest: digest},
		DefinitionVersion:  2,
		ProductionProfile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: digest},
		ProductionWorkflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest},
		SourceRunIDs:       []string{tolerantRun.ID}, Manifest: []byte(`{"schema":"storage-test"}`),
		ManifestDigest: digest, Signature: bytes.Repeat([]byte{0x42}, 64), SignatureAlgorithm: publicationAlgorithm,
		SigningKeyID: "release-key", PublishedBy: "engineer-1", Reason: "verify restart-safe publication storage", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreatePublication: %v", err)
	}
	store = newMigratedSessionStore(t, ctx, db, protector)
	reopenedPublication, err := store.GetPublication(ctx, workspace.ID, publication.ID)
	if err != nil || reopenedPublication.ManifestDigest != publication.ManifestDigest || !bytes.Equal(reopenedPublication.Signature, publication.Signature) {
		t.Fatalf("GetPublication after restart = %#v, %v", reopenedPublication, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE integration_session_publications SET version = version + 1 WHERE publication_id = $1`, publication.ID); err == nil {
		t.Fatal("append-only publication table accepted an update")
	}

	decision, err := store.AcceptDecision(ctx, AcceptDecisionRequest{
		SessionID: workspace.ID, RunID: tolerantRun.ID,
		DiagnosticID: tolerantRun.Diagnostics[0].ID, AcceptedBy: "engineer-1",
		Reason: "expected source omission",
	})
	if err != nil || decision.ID == "" {
		t.Fatalf("AcceptDecision = %#v, %v", decision, err)
	}
	if _, err := store.ArchiveSession(ctx, workspace.ID); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	active, err := store.ListSessions(ctx, ListSessionsOptions{})
	if err != nil || len(active) != 0 {
		t.Fatalf("active sessions = %#v, %v", active, err)
	}
	all, err := store.ListSessions(ctx, ListSessionsOptions{IncludeArchived: true})
	if err != nil || len(all) != 1 || all[0].ID != workspace.ID {
		t.Fatalf("all sessions = %#v, %v", all, err)
	}
	bundle, err := store.ExportBundle(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if bundle.ID == "" || len(bundle.Runs) != 2 || len(bundle.Simulations) != 2 || len(bundle.Publications) != 1 || len(bundle.Decisions) != 1 {
		t.Fatalf("bundle = %#v", bundle)
	}
	for _, sample := range bundle.Samples {
		if sample.PHIPolicy == PHIPolicyRetain && sample.Raw != "" {
			t.Fatal("default export included retained raw payload")
		}
	}

	store = newMigratedSessionStore(t, ctx, db, protector)
	reopenedExport, err := store.GetExport(ctx, workspace.ID, bundle.ID)
	if err != nil || reopenedExport.ID != bundle.ID || len(reopenedExport.Decisions) != 1 {
		t.Fatalf("GetExport after restart = %#v, %v", reopenedExport, err)
	}
	var exportedWorkflow *ArtifactDraft
	for index := range reopenedExport.Drafts {
		if reopenedExport.Drafts[index].RevisionID == candidateWorkflow.RevisionID {
			exportedWorkflow = &reopenedExport.Drafts[index]
			break
		}
	}
	if exportedWorkflow == nil || string(exportedWorkflow.Content) != candidateWorkflowYAML || exportedWorkflow.Digest != candidateWorkflow.Digest {
		t.Fatalf("workflow YAML export did not round-trip exactly: %#v", exportedWorkflow)
	}
	exports, err := store.ListExports(ctx, workspace.ID)
	if err != nil || len(exports) != 1 || exports[0].ID != bundle.ID {
		t.Fatalf("ListExports after restart = %#v, %v", exports, err)
	}

	assertNoRawPHIInSessionTables(t, ctx, db)
	var exportCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM integration_session_exports`).Scan(&exportCount); err != nil || exportCount != 1 {
		t.Fatalf("durable exports = %d, %v", exportCount, err)
	}
}

func assertExactRunProfile(t *testing.T, run *Run, revision *ArtifactDraft) {
	t.Helper()
	if run.ProfileID != revision.ID || run.ProfileRevisionID != revision.RevisionID || run.ProfileRevisionDigest != revision.Digest {
		t.Fatalf("run profile = %q/%q/%q, want %q/%q/%q", run.ProfileID, run.ProfileRevisionID,
			run.ProfileRevisionDigest, revision.ID, revision.RevisionID, revision.Digest)
	}
}

func diagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func assertNoRawPHIInSessionTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, table := range []string{
		"integration_sessions", "integration_session_samples", "integration_session_artifact_revisions",
		"integration_session_runs", "integration_session_decisions", "integration_session_exports",
		"integration_session_publications",
	} {
		var leaked bool
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE record_json::text LIKE '%%RAW-PHI-SENTINEL%%')`, pq.QuoteIdentifier(table))
		if err := db.QueryRowContext(ctx, query).Scan(&leaked); err != nil {
			t.Fatalf("scan %s for raw PHI: %v", table, err)
		}
		if leaked {
			t.Fatalf("raw PHI leaked into %s", table)
		}
	}
	var cipherContainsPlaintext bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM integration_session_samples
			WHERE raw_cipher IS NOT NULL AND position(convert_to('RAW-PHI-SENTINEL', 'UTF8') in raw_cipher) > 0
		)
	`).Scan(&cipherContainsPlaintext); err != nil {
		t.Fatalf("scan retained ciphertext: %v", err)
	}
	if cipherContainsPlaintext {
		t.Fatal("retained ciphertext contains raw PHI sentinel")
	}
}

func newMigratedSessionStore(t *testing.T, ctx context.Context, db *sql.DB, protector PayloadProtector) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(db, PostgresConfig{TenantID: "tenant-a", Protector: protector})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store
}

func openSessionPostgres(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		container, err := postgrescontainer.Run(ctx, "postgres:16-alpine",
			postgrescontainer.WithDatabase("fi_fhir_session"),
			postgrescontainer.WithUsername("testuser"),
			postgrescontainer.WithPassword("testpass"),
			postgrescontainer.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
		base, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("session_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil || db.PingContext(ctx) != nil {
		t.Fatalf("open session database = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
		_ = admin.Close()
	})
	return db
}
