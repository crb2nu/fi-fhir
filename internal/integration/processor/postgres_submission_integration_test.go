//go:build integration

package processor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestPostgresProductionSubmission_64WayDuplicateFaultRestart(t *testing.T) {
	ctx := t.Context()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for durable submission integration tests")
	}

	schema := fmt.Sprintf("production_submission_%d", time.Now().UnixNano())
	createSubmissionSchema(t, dsn, schema)
	schemaDSN := submissionSchemaDSN(t, dsn, schema)
	fixedClock := func() time.Time { return time.Date(2026, 7, 14, 12, 34, 56, 789000000, time.UTC) }

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	request := fixture.request
	request.Mode = integration.ExecutionModeProduction
	request.IdempotencyKey = "explicit-submission-key-123"

	initialDB := openSubmissionDB(t, schemaDSN)
	initialStore := newSubmissionStore(t, initialDB, fixedClock)
	if err := initialStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(first): %v", err)
	}
	if err := initialStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(idempotent): %v", err)
	}
	initialProcessor := newDurableFixtureProcessor(t, fixture, initialStore)

	preCommitCheckpoints := []submissionCheckpoint{
		checkpointAfterReceipt,
		checkpointAfterEvent,
		checkpointAfterLineage,
		checkpointAfterAttempt,
		checkpointAfterOutbox,
		checkpointBeforeCommit,
	}
	for _, checkpoint := range preCommitCheckpoints {
		t.Run("rollback_"+string(checkpoint), func(t *testing.T) {
			initialStore.faultHook = func(actual submissionCheckpoint) error {
				if actual == checkpoint {
					return fmt.Errorf("injected fault at %s", actual)
				}
				return nil
			}
			if _, err := initialProcessor.Process(ctx, request); !errors.Is(err, ErrDurableSubmissionFailed) {
				t.Fatalf("Process fault %s error = %v", checkpoint, err)
			}
			assertSubmissionCounts(t, initialDB, submissionCounts{})
		})
	}
	initialStore.faultHook = nil
	if err := initialDB.Close(); err != nil {
		t.Fatalf("close pre-fault database handle: %v", err)
	}

	const handleCount = 8
	handles := make([]*sql.DB, 0, handleCount)
	processors := make([]*MessageProcessor, 0, handleCount)
	var commitOutcomeUnknownInjected atomic.Bool
	commitOutcomeUnknownFault := func(checkpoint submissionCheckpoint) error {
		if checkpoint == checkpointAfterCommit && commitOutcomeUnknownInjected.CompareAndSwap(false, true) {
			return ErrCommitOutcomeUnknown
		}
		return nil
	}
	for index := 0; index < handleCount; index++ {
		db := openSubmissionDB(t, schemaDSN)
		store := newSubmissionStore(t, db, fixedClock)
		store.faultHook = commitOutcomeUnknownFault
		handles = append(handles, db)
		processors = append(processors, newDurableFixtureProcessor(t, fixture, store))
	}

	const callers = 64
	requests := make([]integration.ProcessRequest, callers)
	results := make([]integration.ProcessResult, callers)
	errorsByCaller := make([]error, callers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for caller := 0; caller < callers; caller++ {
		requests[caller] = request
		requests[caller].CorrelationID = fmt.Sprintf("submission-correlation-%02d", caller)
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsByCaller[index] = processors[index%len(processors)].Process(ctx, requests[index])
		}(caller)
	}
	close(start)
	wait.Wait()

	unknownCaller := -1
	for caller, err := range errorsByCaller {
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrDurableSubmissionFailed) {
			t.Fatalf("caller %d unexpected error: %v", caller, err)
		}
		if !errors.Is(err, ErrCommitOutcomeUnknown) {
			t.Fatalf("caller %d unexpected durable failure: %v", caller, durableSubmissionCause(err))
		}
		if unknownCaller != -1 {
			t.Fatalf("multiple commit-unknown callers: first=%d second=%d", unknownCaller, caller)
		}
		unknownCaller = caller
	}
	if unknownCaller == -1 {
		t.Fatal("64-way gate did not exercise the post-COMMIT unknown outcome")
	}
	if !commitOutcomeUnknownInjected.Load() {
		t.Fatal("64-way gate did not inject the post-COMMIT unknown outcome")
	}
	assertSubmissionCounts(t, handles[0], submissionCounts{receipts: 1, events: 1, lineage: 1, attempts: 1, outbox: 1})

	for index, db := range handles {
		if err := db.Close(); err != nil {
			t.Fatalf("close concurrent database handle %d: %v", index, err)
		}
	}

	restartedDB := openSubmissionDB(t, schemaDSN)
	restartedStore := newSubmissionStore(t, restartedDB, fixedClock)
	if err := restartedStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(after restart): %v", err)
	}
	restartedProcessor := newDurableFixtureProcessor(t, fixture, restartedStore)
	retried, err := restartedProcessor.Process(ctx, requests[unknownCaller])
	if err != nil {
		t.Fatalf("retry commit-unknown caller %d: %v", unknownCaller, err)
	}
	results[unknownCaller] = retried
	errorsByCaller[unknownCaller] = nil

	var expectedJSON []byte
	for caller, result := range results {
		if err := result.ValidateProductionAgainst(fixture.revision); err != nil {
			t.Fatalf("caller %d strict result: %v", caller, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal caller %d: %v", caller, err)
		}
		if expectedJSON == nil {
			expectedJSON = encoded
			continue
		}
		if !bytes.Equal(encoded, expectedJSON) {
			t.Fatalf("caller %d received a different durable result:\nwant: %s\n got: %s", caller, expectedJSON, encoded)
		}
	}
	if results[0].Receipt == nil || results[0].Receipt.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("explicit idempotency key did not win: %#v", results[0].Receipt)
	}
	derived, err := effectiveIdempotencyKey(integration.ProcessRequest{
		Mode:                request.Mode,
		IntegrationRevision: request.IntegrationRevision,
		Security:            request.Security,
		Envelope:            request.Envelope,
		CorrelationID:       request.CorrelationID,
	}, fixture.revision, results[0].Correlations.SourceMessageID)
	if err != nil || derived == request.IdempotencyKey {
		t.Fatalf("derived precedence proof failed: derived=%q err=%v", derived, err)
	}

	conflict := request
	conflict.Envelope = messageProcessorEnvelope(
		t,
		fixture.revision,
		bytes.Replace(processorA01Message(true), []byte("Patient^Test"), []byte("Patient^Conflict"), 1),
		fixture.revision.TenantID,
	)
	if _, err := restartedProcessor.Process(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed request reused explicit key: %v", err)
	}
	assertSubmissionCounts(t, restartedDB, submissionCounts{receipts: 1, events: 1, lineage: 1, attempts: 1, outbox: 1})
	assertSubmissionPersistenceIsRawFree(t, restartedDB)

	var migrationCount int
	if err := restartedDB.QueryRow(`SELECT COUNT(*) FROM integration_submission_schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count submission migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration rows = %d, want 1", migrationCount)
	}
	if err := restartedDB.Close(); err != nil {
		t.Fatalf("close restarted database handle: %v", err)
	}
}

type submissionCounts struct {
	receipts int
	events   int
	lineage  int
	attempts int
	outbox   int
}

func assertSubmissionCounts(t *testing.T, db *sql.DB, want submissionCounts) {
	t.Helper()
	tables := []struct {
		name string
		want int
	}{
		{name: "integration_receipts", want: want.receipts},
		{name: "integration_canonical_events", want: want.events},
		{name: "integration_message_lineage", want: want.lineage},
		{name: "integration_delivery_attempts", want: want.attempts},
		{name: "integration_delivery_outbox", want: want.outbox},
	}
	for _, table := range tables {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + pq.QuoteIdentifier(table.name)).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table.name, err)
		}
		if count != table.want {
			t.Fatalf("%s rows = %d, want %d", table.name, count, table.want)
		}
	}
}

func assertSubmissionPersistenceIsRawFree(t *testing.T, db *sql.DB) {
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
		t.Fatalf("query persisted JSON: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var persisted string
		if err := rows.Scan(&persisted); err != nil {
			t.Fatalf("scan persisted JSON: %v", err)
		}
		for _, forbidden := range []string{"RAW-POSTGRES-SENTINEL", `"raw_payload"`, "event.???", "secret"} {
			if strings.Contains(persisted, forbidden) {
				t.Fatalf("persisted JSON contains forbidden %q: %s", forbidden, persisted)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate persisted JSON: %v", err)
	}
}

func newDurableFixtureProcessor(
	t *testing.T,
	fixture messageProcessorFixture,
	store *PostgresSubmissionStore,
) *MessageProcessor {
	t.Helper()
	messageProcessor, err := NewDurableMessageProcessor(
		fixture.processor.definitions,
		fixture.processor.artifacts,
		store,
	)
	if err != nil {
		t.Fatalf("NewDurableMessageProcessor: %v", err)
	}
	return messageProcessor
}

func newSubmissionStore(
	t *testing.T,
	db *sql.DB,
	clock func() time.Time,
) *PostgresSubmissionStore {
	t.Helper()
	store, err := NewPostgresSubmissionStore(db, PostgresSubmissionConfig{Clock: clock})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	return store
}

func createSubmissionSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres for submission schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		_ = db.Close()
		t.Fatalf("create submission schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close submission schema database: %v", err)
	}
	t.Cleanup(func() {
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanupDB.Close() }()
		_, _ = cleanupDB.ExecContext(context.Background(), `DROP SCHEMA `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
}

func submissionSchemaDSN(t *testing.T, dsn, schema string) string {
	t.Helper()
	connectionString := dsn
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			t.Fatalf("parse PostgreSQL test URL: %v", err)
		}
		connectionString = parsed
	}
	return connectionString + " search_path=" + schema
}

func openSubmissionDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open submission PostgreSQL: %v", err)
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(12)
	if err := db.PingContext(t.Context()); err != nil {
		_ = db.Close()
		t.Fatalf("ping submission PostgreSQL: %v", err)
	}
	return db
}
