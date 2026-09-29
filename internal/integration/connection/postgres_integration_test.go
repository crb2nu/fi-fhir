//go:build integration

package connection

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

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The .loom/38 C-0 PostgreSQL proofs. Each runs in its own PostgreSQL schema
// (the lifecycle suite's idiom), so they share a database without sharing
// state. `make connection-catalog` runs exactly these; ci/test-connection-catalog.yml
// asserts every name exists before it runs them, because a missing
// POSTGRES_TEST_URL makes each one skip rather than fail.

const raiseExceptionCode = pq.ErrorCode("P0001")

func requireConnectionDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for connection catalog integration tests")
	}
	return dsn
}

// newConnectionSchema creates an empty schema and returns a DSN whose
// search_path selects it.
func newConnectionSchema(t *testing.T, dsn string) string {
	t.Helper()
	schema := fmt.Sprintf("connection_catalog_%d", time.Now().UnixNano())
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	if _, err := admin.ExecContext(context.Background(), `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()
	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.ExecContext(context.Background(), `DROP SCHEMA `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
	connectionString := dsn
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		if err != nil {
			t.Fatalf("parse PostgreSQL URL: %v", err)
		}
		connectionString = parsed
	}
	return connectionString + " search_path=" + schema
}

func openConnectionDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	db.SetMaxOpenConns(8)
	if err := db.PingContext(t.Context()); err != nil {
		_ = db.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// fixedClock advances one second per reading, so audit rows are ordered and
// reproducible.
func fixedClock() func() time.Time {
	var ticks atomic.Int64
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return base.Add(time.Duration(ticks.Add(1)) * time.Second) }
}

// catalogUnderTest is one migrated schema with a service bound to tenant-a.
type catalogUnderTest struct {
	dsn     string
	db      *sql.DB
	store   *PostgresStore
	service *Service
}

func newCatalogUnderTest(t *testing.T) catalogUnderTest {
	t.Helper()
	dsn := newConnectionSchema(t, requireConnectionDSN(t))
	db := openConnectionDB(t, dsn)
	store, err := NewPostgresStore(db, fixedClock())
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	service, err := NewService(store, nil, nil, "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return catalogUnderTest{dsn: dsn, db: db, store: store, service: service}
}

func writerContext(tenantID string) context.Context {
	return callerContext(tenantID, ReadRole, WriteRole)
}

func createFromFixture(t *testing.T, service *Service, ctx context.Context, kind Kind, id string) Connection {
	t.Helper()
	fixture := loadSpecFixture(t, kind)
	direction, _ := kind.Direction()
	connection, err := service.Create(ctx, CreateRequest{
		ID: id, Direction: direction, Kind: kind, Name: "fixture " + string(kind),
		Spec: fixture.specJSON(t), SecretBindings: fixture.SecretBindings, Reason: "declare " + id,
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", id, err)
	}
	return connection
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// TestConnectionCatalog_TwoReplicasMigrateConcurrently: two replicas starting
// together against a fresh schema both succeed, the ledger holds one row per
// version, and it is at SchemaVersion.
func TestConnectionCatalog_TwoReplicasMigrateConcurrently(t *testing.T) {
	dsn := newConnectionSchema(t, requireConnectionDSN(t))
	const replicas = 2
	errs := make([]error, replicas)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range replicas {
		db := openConnectionDB(t, dsn)
		store, err := NewPostgresStore(db, nil)
		if err != nil {
			t.Fatalf("NewPostgresStore: %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[index] = store.Migrate(t.Context())
		}()
	}
	close(start)
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("replica %d failed to migrate concurrently: %v", index, err)
		}
	}
	db := openConnectionDB(t, dsn)
	var rows, head int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*), coalesce(max(version), 0) FROM integration_connection_schema_migrations`,
	).Scan(&rows, &head); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if rows != SchemaVersion || head != SchemaVersion {
		t.Fatalf("ledger holds %d rows at head %d, want %d at %d", rows, head, SchemaVersion, SchemaVersion)
	}
	// A restart against the migrated schema is a no-op, not a reapply.
	store, _ := NewPostgresStore(db, nil)
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

// TestConnectionCatalog_CreateUpdateCompileDecodesToTheSameDigest walks the
// draft lifecycle for every kind: create, a stale update refused, an update
// accepted, compile, and the stored bytes decoded with the kind's existing
// Decode function — the one serve mounts them with — to the stored digest.
func TestConnectionCatalog_CreateUpdateCompileDecodesToTheSameDigest(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	decode := map[Kind]func([]byte) (string, error){
		KindMLLP: func(raw []byte) (string, error) {
			revision, err := mllp.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindBatchS3: func(raw []byte) (string, error) {
			revision, err := batch.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindBatchSFTP: func(raw []byte) (string, error) {
			revision, err := batch.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindHTTP: func(raw []byte) (string, error) {
			revision, err := DecodeHTTPSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindHTTPS: storedDestinationDigest, KindFHIR: storedDestinationDigest, KindKafka: storedDestinationDigest,
	}
	for _, kind := range Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			id := "conn-" + strings.ReplaceAll(string(kind), "_", "-")
			created := createFromFixture(t, catalog.service, ctx, kind, id)
			if created.Version != 1 || created.LatestRevision != nil || created.Runtime.Mounted || len(created.References) != 0 {
				t.Fatalf("created = %+v", created)
			}

			renamed := "renamed " + string(kind)
			if _, err := catalog.service.Update(ctx, UpdateRequest{
				ID: id, ExpectedVersion: 7, Name: &renamed, Reason: "stale edit",
			}); !errors.Is(err, ErrVersionConflict) {
				t.Fatalf("stale update error = %v, want ErrVersionConflict", err)
			}
			updated, err := catalog.service.Update(ctx, UpdateRequest{
				ID: id, ExpectedVersion: 1, Name: &renamed, Reason: "rename",
			})
			if err != nil || updated.Version != 2 || updated.Name != renamed || updated.Updated.Reason != "rename" {
				t.Fatalf("update = %+v, %v", updated, err)
			}

			if _, err := catalog.service.Compile(ctx, CommandRequest{ID: id, ExpectedVersion: 1, Reason: "stale compile"}); !errors.Is(err, ErrVersionConflict) {
				t.Fatalf("stale compile error = %v", err)
			}
			result, err := catalog.service.Compile(ctx, CommandRequest{ID: id, ExpectedVersion: 2, Reason: "first compile"})
			if err != nil || result.Revision == nil || HasBlocking(result.Problems) {
				t.Fatalf("compile = %+v, %v", result, err)
			}
			revision := *result.Revision
			if revision.RevisionID != "1" || revision.CompiledFromVersion != 2 || revision.Created.Reason != "first compile" {
				t.Fatalf("revision = %+v", revision)
			}
			stored, err := catalog.service.GetRevision(ctx, id, "1")
			if err != nil || !bytes.Equal(stored.Document, revision.Document) {
				t.Fatalf("stored revision differs: %v", err)
			}
			digest, err := decode[kind](stored.Document)
			if err != nil || digest != stored.Digest {
				t.Fatalf("the existing decoder read the stored bytes as %q (%v), want %q", digest, err, stored.Digest)
			}
			if result.Connection.LatestRevision == nil || result.Connection.LatestRevision.Digest != stored.Digest {
				t.Fatalf("the compile result's connection does not carry the new revision: %+v", result.Connection.LatestRevision)
			}

			// Compiling the same draft version again writes nothing.
			again, err := catalog.service.Compile(ctx, CommandRequest{ID: id, ExpectedVersion: 2, Reason: "double click"})
			if err != nil || again.Revision == nil || again.Revision.RevisionID != "1" {
				t.Fatalf("recompile = %+v, %v", again, err)
			}
			revisions, err := catalog.service.ListRevisions(ctx, id)
			if err != nil || len(revisions) != 1 {
				t.Fatalf("revisions after a no-op recompile = %d, %v", len(revisions), err)
			}
		})
	}

	t.Run("a blocking problem writes nothing", func(t *testing.T) {
		broken := json.RawMessage(`{"destination_id":"broken","class":"production"}`)
		connection, err := catalog.service.Create(ctx, CreateRequest{
			ID: "broken-kafka", Direction: DirectionDestination, Kind: KindKafka, Name: "broken",
			Spec: broken, Reason: "incomplete draft",
		})
		if err != nil {
			t.Fatalf("an incomplete draft was refused: %v", err)
		}
		before := countRows(t, catalog.db, "integration_connection_revisions")
		result, err := catalog.service.Compile(ctx, CommandRequest{ID: "broken-kafka", ExpectedVersion: connection.Version, Reason: "try"})
		if err != nil || result.Revision != nil || !hasCode(result.Problems, CodeRequired, "kafka") {
			t.Fatalf("compile of an incomplete draft = %+v, %v", result, err)
		}
		if after := countRows(t, catalog.db, "integration_connection_revisions"); after != before {
			t.Fatalf("a failed compile wrote %d revision rows", after-before)
		}
	})

	t.Run("a second compile after an edit is revision 2", func(t *testing.T) {
		description := "moved to the new broker topic"
		fixture := loadSpecFixture(t, KindKafka)
		fixture.Spec["kafka"] = map[string]any{"topic": "integration.delivery.v2"}
		updated, err := catalog.service.Update(ctx, UpdateRequest{
			ID: "conn-kafka", ExpectedVersion: 2, Description: &description, Spec: fixture.specJSON(t), Reason: "new topic",
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		result, err := catalog.service.Compile(ctx, CommandRequest{ID: "conn-kafka", ExpectedVersion: updated.Version, Reason: "recompile"})
		if err != nil || result.Revision == nil || result.Revision.RevisionID != "2" || result.Revision.CompiledFromVersion != 3 {
			t.Fatalf("second compile = %+v, %v", result, err)
		}
		revisions, err := catalog.service.ListRevisions(ctx, "conn-kafka")
		if err != nil || len(revisions) != 2 || revisions[0].RevisionID != "2" || revisions[0].Digest == revisions[1].Digest {
			t.Fatalf("revisions = %+v, %v", revisions, err)
		}
	})

	t.Run("archive freezes the draft and keeps its revisions", func(t *testing.T) {
		archived, err := catalog.service.Archive(ctx, CommandRequest{ID: "conn-mllp", ExpectedVersion: 2, Reason: "retired listener"})
		if err != nil || !archived.Archived() || archived.Version != 3 {
			t.Fatalf("archive = %+v, %v", archived, err)
		}
		if _, err := catalog.service.Compile(ctx, CommandRequest{ID: "conn-mllp", ExpectedVersion: 3, Reason: "x"}); !errors.Is(err, ErrArchived) {
			t.Fatalf("compile of an archived draft: %v", err)
		}
		name := "x"
		if _, err := catalog.service.Update(ctx, UpdateRequest{ID: "conn-mllp", ExpectedVersion: 3, Name: &name, Reason: "x"}); !errors.Is(err, ErrArchived) {
			t.Fatalf("update of an archived draft: %v", err)
		}
		listed, err := catalog.service.List(ctx, ListFilter{Direction: DirectionSource})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, connection := range listed {
			if connection.ID == "conn-mllp" {
				t.Fatal("an archived connection is listed without includeArchived")
			}
		}
		withArchived, err := catalog.service.List(ctx, ListFilter{Direction: DirectionSource, IncludeArchived: true})
		if err != nil || len(withArchived) != len(listed)+1 {
			t.Fatalf("includeArchived listed %d, want %d (%v)", len(withArchived), len(listed)+1, err)
		}
		if _, err := catalog.service.GetRevision(ctx, "conn-mllp", "1"); err != nil {
			t.Fatalf("an archived connection's revision is gone: %v", err)
		}
	})
}

func storedDestinationDigest(raw []byte) (string, error) {
	revision, err := destination.DecodeRevision(bytes.NewReader(raw))
	return revision.Digest, err
}

// TestConnectionCatalog_LifecycleReferenceAndRuntimeState: a definition
// draft created in the lifecycle catalog that names the compiled source and
// destination digests appears in each connection's references, and a runtime
// description that mounts the source digest marks it mounted here.
func TestConnectionCatalog_LifecycleReferenceAndRuntimeState(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	createFromFixture(t, catalog.service, ctx, KindMLLP, "adt-mllp")
	createFromFixture(t, catalog.service, ctx, KindHTTPS, "dest-https")
	source, err := catalog.service.Compile(ctx, CommandRequest{ID: "adt-mllp", ExpectedVersion: 1, Reason: "compile source"})
	if err != nil || source.Revision == nil {
		t.Fatalf("compile source: %+v, %v", source, err)
	}
	target, err := catalog.service.Compile(ctx, CommandRequest{ID: "dest-https", ExpectedVersion: 1, Reason: "compile destination"})
	if err != nil || target.Revision == nil {
		t.Fatalf("compile destination: %+v, %v", target, err)
	}

	lifecycleCatalog, err := lifecycle.NewPostgresCatalog(catalog.db, lifecycle.Config{})
	if err != nil {
		t.Fatalf("NewPostgresCatalog: %v", err)
	}
	if err := lifecycleCatalog.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate lifecycle: %v", err)
	}
	definition := referencingDefinition(t, "adt-mllp-definition", *source.Revision, target.Revision.ArtifactID,
		target.Revision.RevisionID, target.Revision.Digest)
	// A second definition names the same source and a destination the catalog
	// does not hold: the one reference query returns it for the source only.
	second := referencingDefinition(t, "adt-mllp-definition-b", *source.Revision, "dest-elsewhere", "1",
		"sha256:"+strings.Repeat("9", 64))
	for _, created := range []integration.IntegrationDefinitionRevision{definition, second} {
		if _, err := lifecycleCatalog.CreateDraft(t.Context(), created); err != nil {
			t.Fatalf("lifecycle CreateDraft(%s): %v", created.DefinitionID, err)
		}
	}

	mounted := describedRuntime()
	mounted.Adapters[1] = RuntimeAdapter{
		Kind: AdapterMLLP, Enabled: true, DefinitionID: "adt-mllp-definition",
		SourceDigest: source.Revision.Digest, ListenAddress: "0.0.0.0:2575",
	}
	service, err := NewService(catalog.store, lifecycleCatalog, mounted, "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	reader := callerContext("tenant-a", ReadRole)
	reference := func(definitionID, digest string) Reference {
		return Reference{DefinitionID: definitionID, RevisionID: "rev-1", Digest: digest, State: "draft", Health: "unknown"}
	}
	for id, want := range map[string][]Reference{
		"adt-mllp": {
			reference("adt-mllp-definition", source.Revision.Digest),
			reference("adt-mllp-definition-b", source.Revision.Digest),
		},
		"dest-https": {reference("adt-mllp-definition", target.Revision.Digest)},
	} {
		connection, err := service.Get(reader, id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if len(connection.References) != len(want) {
			t.Fatalf("%s references = %+v, want %+v", id, connection.References, want)
		}
		for index := range want {
			if connection.References[index] != want[index] {
				t.Fatalf("%s references = %+v, want %+v", id, connection.References, want)
			}
		}
	}
	listed, err := service.List(reader, ListFilter{})
	if err != nil || len(listed) != 2 {
		t.Fatalf("List = %d, %v", len(listed), err)
	}
	for _, connection := range listed {
		switch connection.ID {
		case "adt-mllp":
			if !connection.Runtime.Mounted || connection.Runtime.Role != RuntimeRoleMLLPListener || len(connection.References) != 2 {
				t.Fatalf("adt-mllp = %+v", connection)
			}
		case "dest-https":
			if connection.Runtime.Mounted {
				t.Fatalf("dest-https reads as mounted: %+v", connection.Runtime)
			}
		}
	}

	// The lifecycle dependency is optional: without it there are no
	// references and no error.
	bare, _ := NewService(catalog.store, nil, nil, "tenant-a")
	connection, err := bare.Get(reader, "adt-mllp")
	if err != nil || len(connection.References) != 0 || connection.Runtime.Mounted {
		t.Fatalf("without lifecycle or runtime: %+v, %v", connection, err)
	}
}

func referencingDefinition(t *testing.T, definitionID string, source Revision, targetID, targetRevisionID, targetDigest string) integration.IntegrationDefinitionRevision {
	t.Helper()
	target := Revision{ArtifactID: targetID, RevisionID: targetRevisionID, Digest: targetDigest}
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	policy := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 30, CheckIntervalSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 8, MaxQueued: 64, MaxMessagesPerSecond: 50},
	}
	definition, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: definitionID, RevisionID: "rev-1", TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: source.ArtifactID, RevisionID: source.RevisionID, Digest: source.Digest},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: digest('2')},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest('3')},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Digest: target.Digest},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: &policy,
		Created: integration.AuditEnvelope{
			TenantID:   "tenant-a",
			Principal:  integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{ReadRole}},
			Reason:     "bind the catalog connections",
			OccurredAt: time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return definition
}

// TestConnectionCatalog_RestartPreservesEveryRow: a new store and service over
// the same database — a replica restart — read back every draft and every
// revision byte for byte.
func TestConnectionCatalog_RestartPreservesEveryRow(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	for _, kind := range Kinds() {
		id := "restart-" + strings.ReplaceAll(string(kind), "_", "-")
		createFromFixture(t, catalog.service, ctx, kind, id)
		if _, err := catalog.service.Compile(ctx, CommandRequest{ID: id, ExpectedVersion: 1, Reason: "compile before restart"}); err != nil {
			t.Fatalf("compile %s: %v", id, err)
		}
	}
	before, err := catalog.service.List(ctx, ListFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("List before restart: %v", err)
	}

	restartedDB := openConnectionDB(t, catalog.dsn)
	restartedStore, err := NewPostgresStore(restartedDB, nil)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := restartedStore.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate on restart: %v", err)
	}
	restarted, err := NewService(restartedStore, nil, nil, "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	after, err := restarted.List(ctx, ListFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("List after restart: %v", err)
	}
	if len(after) != len(before) || len(after) != len(Kinds()) {
		t.Fatalf("restart read %d connections, want %d", len(after), len(before))
	}
	for index := range before {
		b, a := before[index], after[index]
		if a.ID != b.ID || a.Version != b.Version || a.Name != b.Name || !sameJSON(t, a.Spec, b.Spec) ||
			a.LatestRevision == nil || b.LatestRevision == nil ||
			!bytes.Equal(a.LatestRevision.Document, b.LatestRevision.Document) ||
			a.LatestRevision.Digest != b.LatestRevision.Digest ||
			!a.Created.OccurredAt.Equal(b.Created.OccurredAt) || a.Created.Reason != b.Created.Reason ||
			len(a.SecretBindings) != len(b.SecretBindings) {
			t.Fatalf("connection %s changed across a restart:\n before %+v\n after  %+v", b.ID, b, a)
		}
	}
}

// TestConnectionCatalog_StoredRecordsAreGuardedBySchemaTriggers: the schema,
// not convention, refuses every mutation the catalog's write paths never make.
func TestConnectionCatalog_StoredRecordsAreGuardedBySchemaTriggers(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	createFromFixture(t, catalog.service, ctx, KindKafka, "guarded-kafka")
	if _, err := catalog.service.Compile(ctx, CommandRequest{ID: "guarded-kafka", ExpectedVersion: 1, Reason: "compile"}); err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err := catalog.db.ExecContext(t.Context(), `
		INSERT INTO integration_connection_captures (
			tenant_id, capture_id, session_id, mode, source_id, status, max_messages,
			principal_json, reason, requested_at, expires_at
		) VALUES ('tenant-a', 'capture-1', 'session-1', 'stream', 'adt-east', 'armed', 5,
			'{"id":"engineer-1","kind":"human","auth_method":"oidc"}', 'sample the feed',
			'2026-09-26T12:00:00Z', '2026-09-26T12:05:00Z')
	`)
	if err != nil {
		t.Fatalf("insert capture: %v", err)
	}

	refused := []struct {
		name  string
		query string
	}{
		{"rewrite a revision's bytes", `UPDATE integration_connection_revisions SET revision_text = revision_text WHERE artifact_id = 'guarded-kafka'`},
		{"delete a revision", `DELETE FROM integration_connection_revisions WHERE artifact_id = 'guarded-kafka'`},
		{"delete a draft", `DELETE FROM integration_connection_drafts WHERE artifact_id = 'guarded-kafka'`},
		{"update a draft without advancing its version", `UPDATE integration_connection_drafts SET name = 'renamed' WHERE artifact_id = 'guarded-kafka'`},
		{"rewrite a draft's creation audit", `UPDATE integration_connection_drafts SET created_json = '{}'::jsonb, version = version + 1 WHERE artifact_id = 'guarded-kafka'`},
		{"change a draft's kind", `UPDATE integration_connection_drafts SET kind = 'fhir', version = version + 1 WHERE artifact_id = 'guarded-kafka'`},
		{"delete a capture", `DELETE FROM integration_connection_captures WHERE capture_id = 'capture-1'`},
		{"rewrite a capture's reason", `UPDATE integration_connection_captures SET reason = 'other', version = version + 1 WHERE capture_id = 'capture-1'`},
		{"advance a capture without its version", `UPDATE integration_connection_captures SET captured = 1 WHERE capture_id = 'capture-1'`},
	}
	for _, mutation := range refused {
		t.Run(mutation.name, func(t *testing.T) {
			_, err := catalog.db.ExecContext(t.Context(), mutation.query)
			var pqErr *pq.Error
			if !errors.As(err, &pqErr) || pqErr.Code != raiseExceptionCode {
				t.Fatalf("error = %v, want the trigger's SQLSTATE P0001", err)
			}
		})
	}

	// The legitimate paths still work: a draft advances under its version, and
	// an armed capture advances and then finishes, after which it is frozen.
	if _, err := catalog.db.ExecContext(t.Context(),
		`UPDATE integration_connection_drafts SET name = 'renamed', version = version + 1 WHERE artifact_id = 'guarded-kafka'`); err != nil {
		t.Fatalf("an expected-version draft update was refused: %v", err)
	}
	if _, err := catalog.db.ExecContext(t.Context(),
		`UPDATE integration_connection_captures SET captured = 2, version = version + 1 WHERE capture_id = 'capture-1'`); err != nil {
		t.Fatalf("an armed capture could not advance: %v", err)
	}
	if _, err := catalog.db.ExecContext(t.Context(), `
		UPDATE integration_connection_captures
		SET status = 'complete', completed_at = '2026-09-26T12:01:00Z', version = version + 1
		WHERE capture_id = 'capture-1'`); err != nil {
		t.Fatalf("an armed capture could not complete: %v", err)
	}
	for _, query := range []string{
		`UPDATE integration_connection_captures SET status = 'armed', completed_at = NULL, version = version + 1 WHERE capture_id = 'capture-1'`,
		`UPDATE integration_connection_captures SET captured = 3, version = version + 1 WHERE capture_id = 'capture-1'`,
	} {
		var pqErr *pq.Error
		if _, err := catalog.db.ExecContext(t.Context(), query); !errors.As(err, &pqErr) || pqErr.Code != raiseExceptionCode {
			t.Fatalf("a finished capture changed (%s): %v", query, err)
		}
	}
	// And the database refuses a revision whose text and JSON disagree even
	// before any trigger could see it.
	_, err = catalog.db.ExecContext(t.Context(), `
		INSERT INTO integration_connection_revisions (
			tenant_id, artifact_id, revision_id, revision_number, digest, direction, kind,
			revision_json, revision_text, compiled_from_version, created_json, created_at
		) VALUES ('tenant-a', 'guarded-kafka', '9', 9, 'sha256:`+strings.Repeat("e", 64)+`', 'destination', 'kafka',
			'{"a":1}', '{"a":2}', 2, '{}', now())`)
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code.Class() != "23" {
		t.Fatalf("a revision whose text and JSON disagree was stored: %v", err)
	}
}

// TestConnectionCatalog_SecretValueIsRefusedAndNotPersisted: a draft write —
// create or update — that would persist secret material, a key the kind does
// not define, a malformed secret binding reference, or a binding field that
// names no declared binding (a credential pasted into it) is refused with the
// problem's code and path, and no row changes. Validate reports the same
// problem in its result instead of refusing. A binding field that names a
// declared binding saves.
func TestConnectionCatalog_SecretValueIsRefusedAndNotPersisted(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	const marker = "synthetic-refused-value"
	createFromFixture(t, catalog.service, ctx, KindHTTPS, "clean-https")
	cases := []struct {
		name     string
		spec     string
		bindings []integration.SecretBinding
		code     string
		path     string
	}{
		{"token", `{"destination_id":"leaky","class":"production",` +
			`"https":{"url":"https://destination.example.org","method":"POST","token_binding":"t","token":"` + marker + `"}}`,
			nil, CodeSecretValueForbidden, "https.token"},
		{"secret", `{"destination_id":"leaky","secret":"` + marker + `"}`, nil, CodeSecretValueForbidden, "secret"},
		{"password", `{"destination_id":"leaky","password":"` + marker + `"}`, nil, CodeSecretValueForbidden, "password"},
		{"authorization header", `{"https":{"authorization":"Bearer ` + marker + `"}}`, nil, CodeSecretValueForbidden, "https.authorization"},
		{"object under a binding field", `{"https":{"token_binding":{"value":"` + marker + `"}}}`, nil,
			CodeSecretValueForbidden, "https.token_binding"},
		{"token in the url query", `{"https":{"url":"https://hooks.example.org/in?tenant=a&token=` + marker + `"}}`, nil,
			CodeSecretValueForbidden, "https.url"},
		{"unknown key", `{"destination_id":"leaky","headers":{"x-trace":"` + marker + `"}}`, nil, CodeUnknownField, "headers"},
		{"unknown nested key", `{"https":{"url":"https://hooks.example.org/in","note":"` + marker + `"}}`, nil,
			CodeUnknownField, "https.note"},
		{"malformed binding reference", `{"destination_id":"leaky"}`, []integration.SecretBinding{{
			Name: "t", Reference: integration.SecretReference{Provider: "keychain", Key: marker},
		}}, CodeInvalidEnum, "secret_bindings[0].provider"},
		{"credential pasted into a binding field", `{"destination_id":"leaky","https":{"token_binding":"` + marker + `"}}`, nil,
			CodeUnboundSecret, "https.token_binding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drafts := countRows(t, catalog.db, "integration_connection_drafts")
			_, err := catalog.service.Create(ctx, CreateRequest{
				ID: "refused-https", Direction: DirectionDestination, Kind: KindHTTPS, Name: "refused",
				Spec: json.RawMessage(tc.spec), SecretBindings: tc.bindings, Reason: "paste " + tc.name,
			})
			var specErr *SpecError
			if !errors.As(err, &specErr) || !hasCode(specErr.Problems, tc.code, tc.path) {
				t.Fatalf("create error = %v, want %s at %s", err, tc.code, tc.path)
			}
			if strings.Contains(err.Error(), marker) || strings.Contains(fmt.Sprint(specErr.Problems), marker) {
				t.Fatal("the refusal echoes the refused value")
			}
			if after := countRows(t, catalog.db, "integration_connection_drafts"); after != drafts {
				t.Fatalf("a refused create wrote %d draft rows", after-drafts)
			}

			current, err := catalog.service.Get(ctx, "clean-https")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			bindings := tc.bindings
			update := UpdateRequest{ID: "clean-https", ExpectedVersion: current.Version, Spec: json.RawMessage(tc.spec),
				Reason: "paste " + tc.name}
			if bindings != nil {
				update.SecretBindings = &bindings
			}
			if _, err := catalog.service.Update(ctx, update); !errors.As(err, &specErr) || !hasCode(specErr.Problems, tc.code, tc.path) {
				t.Fatalf("update error = %v, want %s at %s", err, tc.code, tc.path)
			}
			var version int64
			var stored string
			if err := catalog.db.QueryRowContext(t.Context(), `
				SELECT version, spec_json::text || secret_bindings_json::text
				FROM integration_connection_drafts WHERE artifact_id = 'clean-https'`).Scan(&version, &stored); err != nil {
				t.Fatalf("read draft: %v", err)
			}
			if version != current.Version || strings.Contains(stored, marker) {
				t.Fatalf("a refused update changed the draft: version %d -> %d, stored %s", current.Version, version, stored)
			}

			problems, err := catalog.service.ValidateSpec(ctx, ValidateRequest{
				Kind: KindHTTPS, Spec: json.RawMessage(tc.spec), SecretBindings: tc.bindings,
			})
			if err != nil || !hasCode(problems, tc.code, tc.path) {
				t.Fatalf("validate = %+v, %v; want %s at %s in the result", problems, err, tc.code, tc.path)
			}
		})
	}
	t.Run("a binding field naming a declared binding saves", func(t *testing.T) {
		envBinding := func(name string) []integration.SecretBinding {
			return []integration.SecretBinding{{Name: name, Reference: integration.SecretReference{
				Provider: integration.SecretProviderEnvironment, Key: "FI_FHIR_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
			}}}
		}
		created, err := catalog.service.Create(ctx, CreateRequest{
			ID: "declared-https", Direction: DirectionDestination, Kind: KindHTTPS, Name: "declared",
			Spec:           json.RawMessage(`{"destination_id":"declared","https":{"token_binding":"declared-token"}}`),
			SecretBindings: envBinding("declared-token"), Reason: "name a declared binding",
		})
		if err != nil || created.Version != 1 {
			t.Fatalf("create naming a declared binding = %+v, %v", created, err)
		}
		// One update may rename the binding and point the field at the new name.
		rotated := envBinding("rotated-token")
		updated, err := catalog.service.Update(ctx, UpdateRequest{
			ID: "declared-https", ExpectedVersion: 1, SecretBindings: &rotated, Reason: "rotate the binding",
			Spec: json.RawMessage(`{"destination_id":"declared","https":{"token_binding":"rotated-token"}}`),
		})
		if err != nil || updated.Version != 2 || len(updated.SecretBindings) != 1 || updated.SecretBindings[0].Name != "rotated-token" {
			t.Fatalf("update renaming the binding = %+v, %v", updated, err)
		}
		// Dropping the binding the field still names leaves the field unbound.
		none := []integration.SecretBinding{}
		var specErr *SpecError
		if _, err := catalog.service.Update(ctx, UpdateRequest{
			ID: "declared-https", ExpectedVersion: 2, SecretBindings: &none, Reason: "drop the binding",
		}); !errors.As(err, &specErr) || !hasCode(specErr.Problems, CodeUnboundSecret, "https.token_binding") {
			t.Fatalf("dropping a named binding = %v, want UNBOUND_SECRET at https.token_binding", err)
		}
		if stored, err := catalog.service.Get(ctx, "declared-https"); err != nil || stored.Version != 2 {
			t.Fatalf("a refused update changed the draft: %+v, %v", stored, err)
		}
	})

	var leaked int
	if err := catalog.db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM integration_connection_drafts
		WHERE position($1 in spec_json::text || secret_bindings_json::text || updated_json::text) > 0`, marker).Scan(&leaked); err != nil {
		t.Fatalf("scan drafts: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("%d stored drafts carry a refused value", leaked)
	}
}

// TestConnectionCatalog_CrossTenantReadIsNotFound: another tenant's connection
// is indistinguishable from one that does not exist.
func TestConnectionCatalog_CrossTenantReadIsNotFound(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	createFromFixture(t, catalog.service, writerContext("tenant-a"), KindKafka, "tenant-a-kafka")
	if _, err := catalog.service.Compile(writerContext("tenant-a"), CommandRequest{ID: "tenant-a-kafka", ExpectedVersion: 1, Reason: "compile"}); err != nil {
		t.Fatalf("compile: %v", err)
	}
	other, err := NewService(catalog.store, nil, nil, "tenant-b")
	if err != nil {
		t.Fatalf("NewService(tenant-b): %v", err)
	}
	ctx := writerContext("tenant-b")
	if _, err := other.Get(ctx, "tenant-a-kafka"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Get = %v, want ErrNotFound", err)
	}
	if _, err := other.Get(ctx, "no-such-connection"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent Get = %v, want ErrNotFound", err)
	}
	if _, err := other.GetRevision(ctx, "tenant-a-kafka", "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant GetRevision = %v", err)
	}
	if revisions, err := other.ListRevisions(ctx, "tenant-a-kafka"); err != nil || len(revisions) != 0 {
		t.Fatalf("cross-tenant ListRevisions = %d, %v", len(revisions), err)
	}
	if listed, err := other.List(ctx, ListFilter{IncludeArchived: true}); err != nil || len(listed) != 0 {
		t.Fatalf("cross-tenant List = %d, %v", len(listed), err)
	}
	name := "hijack"
	if _, err := other.Update(ctx, UpdateRequest{ID: "tenant-a-kafka", ExpectedVersion: 1, Name: &name, Reason: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Update = %v", err)
	}
	if _, err := other.Compile(ctx, CommandRequest{ID: "tenant-a-kafka", ExpectedVersion: 1, Reason: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Compile = %v", err)
	}
	// The same ID is free in the other tenant.
	createFromFixture(t, other, ctx, KindKafka, "tenant-a-kafka")
}

// TestConnectionCatalog_WriteWithoutDeploymentRoleIsForbidden: the service
// re-checks the transport gate's roles, so a reader cannot create, update,
// compile, archive, or validate — validate included, because it is a write
// tool even though it writes nothing — and a refused write changes no row.
func TestConnectionCatalog_WriteWithoutDeploymentRoleIsForbidden(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	reader := callerContext("tenant-a", ReadRole)
	fixture := loadSpecFixture(t, KindKafka)
	if _, err := catalog.service.Create(reader, CreateRequest{
		ID: "reader-kafka", Direction: DirectionDestination, Kind: KindKafka, Name: "reader",
		Spec: fixture.specJSON(t), Reason: "try",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Create without %s = %v, want ErrForbidden", WriteRole, err)
	}
	if rows := countRows(t, catalog.db, "integration_connection_drafts"); rows != 0 {
		t.Fatalf("a forbidden write stored %d rows", rows)
	}
	createFromFixture(t, catalog.service, writerContext("tenant-a"), KindKafka, "writer-kafka")
	renamed := "renamed by a reader"
	if _, err := catalog.service.Update(reader, UpdateRequest{
		ID: "writer-kafka", ExpectedVersion: 1, Name: &renamed, Reason: "x",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Update without %s = %v", WriteRole, err)
	}
	if _, err := catalog.service.Compile(reader, CommandRequest{ID: "writer-kafka", ExpectedVersion: 1, Reason: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Compile without %s = %v", WriteRole, err)
	}
	if _, err := catalog.service.Archive(reader, CommandRequest{ID: "writer-kafka", ExpectedVersion: 1, Reason: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Archive without %s = %v", WriteRole, err)
	}
	if problems, err := catalog.service.ValidateSpec(reader, ValidateRequest{
		Kind: KindKafka, Spec: fixture.specJSON(t),
	}); !errors.Is(err, ErrForbidden) || problems != nil {
		t.Fatalf("ValidateSpec without %s = %+v, %v", WriteRole, problems, err)
	}
	stored, err := catalog.service.Get(reader, "writer-kafka")
	if err != nil {
		t.Fatalf("the reader cannot read: %v", err)
	}
	if stored.Version != 1 || stored.Name == renamed || stored.Archived() || stored.LatestRevision != nil {
		t.Fatalf("a forbidden write changed the draft: %+v", stored)
	}
	if rows := countRows(t, catalog.db, "integration_connection_revisions"); rows != 0 {
		t.Fatalf("a forbidden compile stored %d revisions", rows)
	}
}

// TestConnectionCatalog_ConcurrentCompilesClaimOneRevision: two compiles of the
// same draft version race; exactly one revision row results. And a compile
// racing an archive of the same version waits for it and reports archived,
// rather than appending a revision of a version that was archived under it.
func TestConnectionCatalog_ConcurrentCompilesClaimOneRevision(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := writerContext("tenant-a")
	createFromFixture(t, catalog.service, ctx, KindFHIR, "race-fhir")
	const racers = 4
	errs := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[index] = catalog.service.Compile(ctx, CommandRequest{ID: "race-fhir", ExpectedVersion: 1, Reason: "race"})
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("a racing compile failed with %v", err)
		}
	}
	if rows := countRows(t, catalog.db, "integration_connection_revisions"); rows != 1 {
		t.Fatalf("racing compiles stored %d revisions, want 1", rows)
	}

	t.Run("a compile racing an archive reports archived", func(t *testing.T) {
		createFromFixture(t, catalog.service, ctx, KindKafka, "race-archive")
		draft, err := catalog.store.GetDraft(t.Context(), "tenant-a", "race-archive")
		if err != nil {
			t.Fatalf("GetDraft: %v", err)
		}
		// Compile has read the draft at version 1 and built its revision: the
		// window in which an archive can land before the insert.
		revision, problems := BuildRevision(draft, 1, integration.AuditEnvelope{
			TenantID:   "tenant-a",
			Principal:  integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{ReadRole, WriteRole}},
			Reason:     "compile racing an archive",
			OccurredAt: time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC),
		})
		if revision == nil {
			t.Fatalf("BuildRevision: %+v", problems)
		}
		// The archive of version 1 is in flight: its row lock is held and it
		// has not committed.
		archive, err := catalog.db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatalf("begin archive: %v", err)
		}
		defer func() { _ = archive.Rollback() }()
		if _, err := archive.ExecContext(t.Context(), `
			UPDATE integration_connection_drafts
			SET archived_at = '2026-09-26T14:00:01Z', version = version + 1,
				updated_json = '{"reason":"archive racing a compile"}', updated_at = '2026-09-26T14:00:01Z'
			WHERE tenant_id = 'tenant-a' AND artifact_id = 'race-archive' AND version = 1 AND archived_at IS NULL`); err != nil {
			t.Fatalf("archive: %v", err)
		}

		inserted := make(chan error, 1)
		go func() {
			_, err := catalog.store.InsertRevision(context.Background(), *revision)
			inserted <- err
		}()
		deadline := time.Now().Add(10 * time.Second)
		for waiting := false; !waiting; {
			select {
			case err := <-inserted:
				t.Fatalf("the compile's insert did not wait for the in-flight archive (err = %v)", err)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("the compile's insert never blocked on the draft row")
			}
			var blocked int
			if err := catalog.db.QueryRowContext(t.Context(), `
				SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'
				  AND query LIKE '%INSERT INTO integration_connection_revisions%'`).Scan(&blocked); err != nil {
				t.Fatalf("read pg_stat_activity: %v", err)
			}
			waiting = blocked > 0
			if !waiting {
				time.Sleep(20 * time.Millisecond)
			}
		}
		if err := archive.Commit(); err != nil {
			t.Fatalf("commit archive: %v", err)
		}
		select {
		case err := <-inserted:
			if !errors.Is(err, ErrArchived) {
				t.Fatalf("a compile racing an archive = %v, want ErrArchived", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the compile's insert did not finish after the archive committed")
		}
		var stored int
		if err := catalog.db.QueryRowContext(t.Context(),
			`SELECT count(*) FROM integration_connection_revisions WHERE artifact_id = 'race-archive'`).Scan(&stored); err != nil {
			t.Fatalf("count revisions: %v", err)
		}
		if stored != 0 {
			t.Fatalf("a revision of an archived version was stored (%d rows)", stored)
		}
	})
}

// TestConnectionCatalog_RuntimeObservations: a replica's heartbeat upserts one
// row per adapter in place (no immutability trigger), observed_at moves only
// when the mounted digest changes while heartbeat_at moves every tick, two
// replicas keep separate rows, and another tenant's rows are never listed.
func TestConnectionCatalog_RuntimeObservations(t *testing.T) {
	catalog := newCatalogUnderTest(t)
	ctx := t.Context()
	digestA := "sha256:" + strings.Repeat("a", 64)
	digestB := "sha256:" + strings.Repeat("b", 64)
	upsert := func(observation Observation) {
		t.Helper()
		if err := catalog.store.UpsertObservation(ctx, observation); err != nil {
			t.Fatalf("UpsertObservation(%+v): %v", observation, err)
		}
	}
	upsert(Observation{TenantID: "tenant-a", ReplicaID: "host-1-10", Adapter: AdapterMLLP,
		DefinitionID: "adt-mllp", ArtifactID: "adt-east", RevisionID: "r1", Digest: digestA})
	upsert(Observation{TenantID: "tenant-a", ReplicaID: "host-1-10", Adapter: AdapterBatch})
	upsert(Observation{TenantID: "tenant-a", ReplicaID: "host-2-20", Adapter: AdapterMLLP,
		DefinitionID: "adt-mllp", ArtifactID: "adt-east", RevisionID: "r1", Digest: digestA})
	upsert(Observation{TenantID: "tenant-b", ReplicaID: "host-9-90", Adapter: AdapterMLLP, Digest: digestA})

	first, err := catalog.store.ListObservations(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("tenant-a has %d observations, want 3: %+v", len(first), first)
	}
	if first[0].Adapter != AdapterBatch || first[0].Digest != "" || first[0].ArtifactID != "" {
		t.Fatalf("disabled adapter row = %+v, want no document", first[0])
	}

	// Same digest again: heartbeat moves, observed_at does not.
	upsert(Observation{TenantID: "tenant-a", ReplicaID: "host-1-10", Adapter: AdapterMLLP,
		DefinitionID: "adt-mllp", ArtifactID: "adt-east", RevisionID: "r1", Digest: digestA})
	second, err := catalog.store.ListObservations(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}
	before, after := first[1], second[1]
	if after.ReplicaID != "host-1-10" || after.Adapter != AdapterMLLP {
		t.Fatalf("unexpected row order: %+v", second)
	}
	if !after.HeartbeatAt.After(before.HeartbeatAt) || !after.ObservedAt.Equal(before.ObservedAt) {
		t.Fatalf("same-digest heartbeat: before %+v after %+v", before, after)
	}

	// A new digest moves observed_at too.
	upsert(Observation{TenantID: "tenant-a", ReplicaID: "host-1-10", Adapter: AdapterMLLP,
		DefinitionID: "adt-mllp", ArtifactID: "adt-east", RevisionID: "r2", Digest: digestB})
	third, err := catalog.store.ListObservations(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}
	if third[1].Digest != digestB || third[1].RevisionID != "r2" || !third[1].ObservedAt.Equal(third[1].HeartbeatAt) {
		t.Fatalf("changed digest row = %+v", third[1])
	}
	if got := countRows(t, catalog.db, "integration_runtime_observations"); got != 4 {
		t.Fatalf("table holds %d rows, want 4 (upserts must not append)", got)
	}
	if err := catalog.store.UpsertObservation(ctx, Observation{TenantID: "tenant-a", Adapter: AdapterMLLP}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("observation without replica id: %v, want ErrInvalidRequest", err)
	}
}
