//go:build integration

package mllp

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const integrationProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":1},{"message_type":"ADT^A01","event_type":"patient_admit","priority":2}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const integrationWorkflowYAML = `dsl_version: "1"
name: mllp-admission
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
`

func TestPostgresMLLPRuntime_DurableACKPauseRestart(t *testing.T) {
	ctx := t.Context()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for MLLP runtime integration tests")
	}
	schema := fmt.Sprintf("mllp_runtime_%d", time.Now().UnixNano())
	createMLLPSchema(t, dsn, schema)
	schemaDSN := mllpSchemaDSN(t, dsn, schema)
	db := openMLLPDB(t, schemaDSN)

	source := testSource(t)
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(integrationProfileJSON))
	if err != nil {
		t.Fatal(err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(integrationWorkflowYAML))
	if err != nil {
		t.Fatal(err)
	}
	revision := integrationMLLPRevision(t, source, profileRef, workflowRef)
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{
		ValidateConnection: func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
			return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := deployMLLPRevision(t, catalog, revision)

	artifacts, err := processor.NewRevisionResolver("tenant-a", integrationArtifactLoader{
		profile: []byte(integrationProfileJSON), workflow: []byte(integrationWorkflowYAML),
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := processor.NewDefinitionRevisionResolver("tenant-a", catalog)
	if err != nil {
		t.Fatal(err)
	}
	admitted := make(chan struct{})
	var admittedOnce sync.Once
	store, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: func(ctx context.Context, tx *sql.Tx, request integration.ProcessRequest, exact integration.IntegrationDefinitionRevision) error {
			err := catalog.AuthorizeRunnableSubmission(ctx, tx, request, exact)
			if err == nil {
				admittedOnce.Do(func() { close(admitted) })
			}
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	durableProcessor, err := processor.NewDurableMessageProcessor(definitions, artifacts, store)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{Service: ServiceConfig{
		TenantID: "tenant-a", DefinitionID: revision.DefinitionID, PrincipalID: "mllp-listener",
		Source: source, Resolver: catalog, Processor: durableProcessor,
	}})
	if err != nil {
		t.Fatal(err)
	}
	address, stop := serveTestServer(t, server)

	lockTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockTx.ExecContext(ctx, `LOCK TABLE integration_receipts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	connection := dialMLLP(t, address)
	firstMessage := integrationHL7("control-1", "RAW-MLLP-SENTINEL")
	writeMLLPMessage(t, connection, source, firstMessage)
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("submission did not reach transaction-scoped admission")
	}
	assertNoMLLPAcknowledgement(t, connection)

	pauseResult := make(chan struct {
		snapshot lifecycle.Snapshot
		err      error
	}, 1)
	go func() {
		paused, pauseErr := catalog.Pause(ctx, integrationLifecycleCommand(revision, snapshot.Version, "concurrent operator pause"))
		pauseResult <- struct {
			snapshot lifecycle.Snapshot
			err      error
		}{paused, pauseErr}
	}()
	select {
	case result := <-pauseResult:
		t.Fatalf("pause crossed admitted transaction: %#v, %v", result.snapshot, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if code := readMLLPAcknowledgement(t, connection, source); code != "AA" {
		t.Fatalf("first ACK = %s", code)
	}
	_ = connection.Close()
	paused := <-pauseResult
	if paused.err != nil || paused.snapshot.State != integration.DeploymentStatePaused {
		t.Fatalf("pause result = %#v, %v", paused.snapshot, paused.err)
	}
	snapshot = paused.snapshot
	assertMLLPCounts(t, db, 1)

	if code := roundTripMLLP(t, address, source, firstMessage); code != "AE" {
		t.Fatalf("paused duplicate ACK = %s", code)
	}
	snapshot, err = catalog.Resume(ctx, integrationLifecycleCommand(revision, snapshot.Version, "resume validated listener"))
	if err != nil {
		t.Fatal(err)
	}
	if code := roundTripMLLP(t, address, source, firstMessage); code != "AA" {
		t.Fatalf("resumed duplicate ACK = %s", code)
	}
	const duplicateClients = 32
	type duplicateResult struct {
		code string
		err  error
	}
	duplicateCodes := make(chan duplicateResult, duplicateClients)
	var duplicateWait sync.WaitGroup
	for index := 0; index < duplicateClients; index++ {
		duplicateWait.Add(1)
		go func() {
			defer duplicateWait.Done()
			code, roundTripErr := mllpRoundTripResult(address, source, firstMessage)
			duplicateCodes <- duplicateResult{code: code, err: roundTripErr}
		}()
	}
	duplicateWait.Wait()
	close(duplicateCodes)
	for result := range duplicateCodes {
		if result.err != nil || result.code != "AA" {
			t.Fatalf("concurrent duplicate ACK = %s, %v", result.code, result.err)
		}
	}
	assertMLLPCounts(t, db, 1)
	if code := roundTripMLLP(t, address, source, integrationHL7("control-2", "SECOND-MESSAGE")); code != "AA" {
		t.Fatalf("new message ACK = %s", code)
	}
	assertMLLPCounts(t, db, 2)
	assertMLLPPersistenceSafe(t, db)

	snapshot, err = catalog.Retire(ctx, integrationLifecycleCommand(revision, snapshot.Version, "retire listener"))
	if err != nil {
		t.Fatal(err)
	}
	if code := roundTripMLLP(t, address, source, integrationHL7("control-3", "RETIRED")); code != "AE" {
		t.Fatalf("retired ACK = %s", code)
	}
	stop()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedDB := openMLLPDB(t, schemaDSN)
	restartedCatalog, err := lifecycle.NewPostgresCatalog(restartedDB, lifecycle.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := restartedCatalog.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	restartedStore, err := processor.NewPostgresSubmissionStore(restartedDB, processor.PostgresSubmissionConfig{Authorize: restartedCatalog.AuthorizeRunnableSubmission})
	if err != nil {
		t.Fatal(err)
	}
	restartedDefinitions, err := processor.NewDefinitionRevisionResolver("tenant-a", restartedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	restartedProcessor, err := processor.NewDurableMessageProcessor(restartedDefinitions, artifacts, restartedStore)
	if err != nil {
		t.Fatal(err)
	}
	restartedServer, err := NewServer(ServerConfig{Service: ServiceConfig{
		TenantID: "tenant-a", DefinitionID: revision.DefinitionID, PrincipalID: "mllp-listener",
		Source: source, Resolver: restartedCatalog, Processor: restartedProcessor,
	}})
	if err != nil {
		t.Fatal(err)
	}
	restartedAddress, stopRestarted := serveTestServer(t, restartedServer)
	defer stopRestarted()
	if code := roundTripMLLP(t, restartedAddress, source, integrationHL7("control-4", "RESTARTED")); code != "AE" {
		t.Fatalf("restart after retirement ACK = %s", code)
	}
	assertMLLPCounts(t, restartedDB, 2)
	if snapshot.State != integration.DeploymentStateRetired {
		t.Fatalf("final lifecycle state = %s", snapshot.State)
	}
}

type integrationArtifactLoader struct {
	profile  []byte
	workflow []byte
}

func (l integrationArtifactLoader) LoadProfileRevision(context.Context, string, string) ([]byte, error) {
	return append([]byte(nil), l.profile...), nil
}

func (l integrationArtifactLoader) LoadWorkflowRevision(context.Context, string, string) ([]byte, error) {
	return append([]byte(nil), l.workflow...), nil
}

func integrationMLLPRevision(t *testing.T, source SourceRevision, profile, workflow integration.ArtifactRevisionRef) integration.IntegrationDefinitionRevision {
	t.Helper()
	destinationDigest := "sha256:" + strings.Repeat("d", 64)
	deployment := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 5, CheckIntervalSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 8, MaxMessagesPerSecond: 100},
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-mllp", RevisionID: "definition-v1", TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
		Format: events.FormatHL7v2, Profile: profile, Workflow: workflow,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "destination-v1", Digest: destinationDigest},
			Class:               integration.DestinationClassProduction,
		}},
		Policy:     integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Deployment: &deployment,
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a", Principal: integration.Principal{ID: "engineer", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:engineer"}},
			Reason: "create MLLP integration fixture", OccurredAt: time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func deployMLLPRevision(t *testing.T, catalog *lifecycle.PostgresCatalog, revision integration.IntegrationDefinitionRevision) lifecycle.Snapshot {
	t.Helper()
	ctx := t.Context()
	snapshot, err := catalog.CreateDraft(ctx, revision)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = catalog.ValidateConnection(ctx, integrationLifecycleCommand(revision, snapshot.Version, "validate MLLP source"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = catalog.Approve(ctx, integrationLifecycleCommand(revision, snapshot.Version, "approve MLLP source"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = catalog.Publish(ctx, integrationLifecycleCommand(revision, snapshot.Version, "publish MLLP source"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = catalog.Deploy(ctx, integrationLifecycleCommand(revision, snapshot.Version, "deploy MLLP source"))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func integrationLifecycleCommand(revision integration.IntegrationDefinitionRevision, version int64, reason string) lifecycle.Command {
	return lifecycle.Command{
		TenantID: revision.TenantID, DefinitionID: revision.DefinitionID, RevisionID: revision.RevisionID, ExpectedVersion: version,
		Principal: integration.Principal{ID: "operator", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:operator"}},
		Reason:    reason,
	}
}

func integrationHL7(controlID, sentinel string) []byte {
	segments := []string{
		"MSH|^~\\&|SENDER|FAC|FI-FHIR|FAC|20260715120000-0400|" + sentinel + "|ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
		"EVN|A01|20260715120000||||20260715115900-0400",
		"PID|1||MRN-123^^^HOSP^MR||Patient^Test||19800101|F",
		"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-123|||||||||||||||||||||||||20260715120000",
	}
	return []byte(strings.Join(segments, "\r") + "\r")
}

func dialMLLP(t *testing.T, address string) net.Conn {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func writeMLLPMessage(t *testing.T, connection net.Conn, source SourceRevision, payload []byte) {
	t.Helper()
	framed, err := framePayload(payload, source.Framing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write(framed); err != nil {
		t.Fatal(err)
	}
}

func readMLLPAcknowledgement(t *testing.T, connection net.Conn, source SourceRevision) string {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	payload, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("RAW-MLLP-SENTINEL")) {
		t.Fatalf("acknowledgement reflected raw sentinel: %q", payload)
	}
	return acknowledgementCodeFromPayload(t, payload)
}

func assertNoMLLPAcknowledgement(t *testing.T, connection net.Conn) {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("received acknowledgement before durable commit")
	} else {
		var networkError net.Error
		if !errors.As(err, &networkError) || !networkError.Timeout() {
			t.Fatalf("expected read timeout, got %v", err)
		}
	}
}

func roundTripMLLP(t *testing.T, address string, source SourceRevision, payload []byte) string {
	t.Helper()
	code, err := mllpRoundTripResult(address, source, payload)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func mllpRoundTripResult(address string, source SourceRevision, payload []byte) (string, error) {
	connection, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer connection.Close()
	framed, err := framePayload(payload, source.Framing)
	if err != nil {
		return "", err
	}
	if _, err := connection.Write(framed); err != nil {
		return "", err
	}
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	acknowledgement, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
	if err != nil {
		return "", err
	}
	segments := strings.Split(string(acknowledgement), "\r")
	if len(segments) < 2 {
		return "", ErrResponseEncoding
	}
	fields := strings.Split(segments[1], "|")
	if len(fields) < 2 || fields[0] != "MSA" {
		return "", ErrResponseEncoding
	}
	return fields[1], nil
}

func assertMLLPCounts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"integration_receipts", "integration_canonical_events", "integration_message_lineage", "integration_delivery_attempts", "integration_delivery_outbox"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + pq.QuoteIdentifier(table)).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%s rows = %d, want %d", table, count, want)
		}
	}
}

func assertMLLPPersistenceSafe(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`
		SELECT result_json::text FROM integration_receipts
		UNION ALL SELECT payload_json::text FROM integration_canonical_events
		UNION ALL SELECT artifact_revisions_json::text FROM integration_message_lineage
		UNION ALL SELECT routes_json::text FROM integration_message_lineage
		UNION ALL SELECT diagnostics_json::text FROM integration_message_lineage
		UNION ALL SELECT destination_revision_json::text FROM integration_delivery_attempts
		UNION ALL SELECT payload_json::text FROM integration_delivery_outbox
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"RAW-MLLP-SENTINEL", "MSH|^~\\&", `"raw_payload"`} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("persisted data leaked %q: %s", forbidden, value)
			}
		}
	}
}

func createMLLPSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", dsn)
		if err == nil {
			defer cleanup.Close()
			_, _ = cleanup.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		}
	})
}

func mllpSchemaDSN(t *testing.T, dsn, schema string) string {
	t.Helper()
	connectionString := dsn
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
		connectionString = parsed
	}
	return connectionString + " search_path=" + schema
}

func openMLLPDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	if err := db.PingContext(t.Context()); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}
