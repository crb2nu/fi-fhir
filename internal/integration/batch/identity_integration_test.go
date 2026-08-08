//go:build integration

package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

// spoofedRemoteModifiedAt is what a remote sender can freely claim. Slice 2.4
// used exactly this value as the receipt's received-at; Slice 4.1b3 must not.
var spoofedRemoteModifiedAt = time.Date(1994, 3, 2, 4, 5, 6, 0, time.UTC)

// durableRecordTables are every durable record class a denied source must leave
// untouched.
var durableRecordTables = []string{
	"integration_batch_objects",
	"integration_batch_audit",
	"integration_receipts",
	"integration_canonical_events",
	"integration_delivery_outbox",
}

// TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance is the Slice
// 4.1b3 kill test. It drives real MinIO and a real SSH/SFTP server against
// PostgreSQL 16 with the production durable processor and transaction-scoped
// runnable admission, and proves that
//
//  1. two sources bound to two workload subjects stay distinct at admission
//     even when object keys and MSH content impersonate each other,
//  2. an ungranted subject stops before artifact loading with zero durable
//     records and no poisoned checkpoint,
//  3. compatibility mode still admits under the deployment-fixed principal,
//  4. receipt provenance is the server-owned custody timestamp plus verified
//     content identity, never the remote modification time.
func TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	db := openBatchPostgres(t, ctx)
	artifactResolver := batchArtifactResolver(t)
	submissionMigrations, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil || submissionMigrations.Migrate(ctx) != nil {
		t.Fatalf("submission migrations = %v", err)
	}
	checkpointStore, err := NewPostgresStore(db, nil)
	if err != nil || checkpointStore.Migrate(ctx) != nil {
		t.Fatalf("batch migrations = %v", err)
	}
	// Migration is idempotent across both ledger entries.
	if err := checkpointStore.Migrate(ctx); err != nil {
		t.Fatalf("repeat batch migrations = %v", err)
	}

	endpoint, accessKey, secretKey := batchMinIO(t, ctx)
	server := startBatchSFTPServer(t)

	// Both objects carry identical MSH sending application/facility naming the
	// SFTP subject, so any identity that follows message content collapses.
	impersonating := batchIdentityFixture(t, "svc-batch-west")
	fixtureDigest := sha256Digest(impersonating)

	// ---------------------------------------------------------------- S3 ---
	bucket := fmt.Sprintf("batch-identity-%d", time.Now().UnixNano())
	s3Source := boundS3Source(t, endpoint, bucket, &WorkloadIdentity{
		Subject: "svc-batch-east", Grants: []string{SubmitRole},
	})
	s3Provider := newIdentityS3Bucket(t, ctx, s3Source, accessKey, secretKey)
	// The object key also names the other subject.
	putS3Object(t, ctx, s3Provider, "incoming/svc-batch-west/adt.hl7", impersonating)
	runIdentityBatchSource(t, ctx, db, artifactResolver, checkpointStore, s3Source, s3Provider, "batch-identity-s3")

	// -------------------------------------------------------------- SFTP ---
	sftpInput := filepath.Join(server.root, "west-in")
	sftpArchive := filepath.Join(server.root, "west-archive")
	writeSFTPFixture(t, sftpInput, "adt.hl7", impersonating, spoofedRemoteModifiedAt)
	sftpSource := boundSFTPSource(t, server.address, sftpInput, sftpArchive, &WorkloadIdentity{
		Subject: "svc-batch-west", Grants: []string{SubmitRole},
	})
	sftpProvider, err := NewSFTPProvider(sftpSource, SFTPSecrets{KnownHostsPath: server.knownHosts, Password: "batch-pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sftpProvider.Close() }()
	proveAdvisoryModifiedTime(t, ctx, sftpProvider)
	runIdentityBatchSource(t, ctx, db, artifactResolver, checkpointStore, sftpSource, sftpProvider, "batch-identity-sftp")

	// 1. Two subjects, no crossover, despite identical MSH and a crossed key.
	subjects := admittedSubjects(t, ctx, db)
	if len(subjects) != 2 || subjects["svc-batch-east"] != 2 || subjects["svc-batch-west"] != 2 {
		t.Fatalf("admitted subjects = %v, want two messages each for east and west", subjects)
	}
	assertAuthMethods(t, ctx, db, AuthMethodWorkloadIdentity, 4)

	// 4. Provenance is server-owned custody time plus verified content identity.
	proveTrustedProvenance(t, ctx, db, fixtureDigest)

	// 2. An ungranted subject halts before any durable record exists.
	proveUngrantedWorkloadIsInert(t, ctx, db, artifactResolver, checkpointStore, server)

	// 3. Compatibility mode is unchanged.
	proveCompatibilityMode(t, ctx, db, artifactResolver, checkpointStore, endpoint, accessKey, secretKey)

	// Upgrade path: rows admitted before the provenance migration must not be
	// permanently rejected by the new exact-version check.
	proveLegacyRowAdoption(t, ctx, db, checkpointStore, endpoint)
}

// proveLegacyRowAdoption simulates a row written before migration 0002, which
// carries empty exact-version columns, and asserts a later claim adopts the
// observed version and entity tag instead of failing closed forever.
func proveLegacyRowAdoption(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	store *PostgresStore,
	endpoint string,
) {
	t.Helper()
	source := boundS3Source(t, endpoint, fmt.Sprintf("batch-legacy-%d", time.Now().UnixNano()), nil)
	object := Object{
		Provider: ProviderS3, Path: "incoming/legacy.hl7", Version: "version:legacy-1",
		ETag: "5d41402abc4b2a76b9719d911017c592", Size: 42,
		RemoteModifiedAtAdvisory: spoofedRemoteModifiedAt,
	}
	id, err := objectID(source, object)
	if err != nil {
		t.Fatal(err)
	}
	integrationDigest := testDigest('e')
	// Reproduce the real upgrade order: the row is written while the provenance
	// constraint does not yet exist, then the NOT VALID constraint is added on
	// top of it. A row seeded any other way would be blocked, which is itself
	// the proof that the constraint governs every write from here forward.
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE integration_batch_objects
			DROP CONSTRAINT integration_batch_objects_provenance_chk
	`); err != nil {
		t.Fatalf("drop provenance constraint: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_batch_objects (
			tenant_id, source_id, source_revision_digest, integration_revision_digest,
			object_id, provider, object_size, remote_modified_at_advisory,
			phase, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 's3', $6, $7, 'processing', now(), now())
	`, "tenant-a", source.SourceID, source.Digest, integrationDigest, id,
		object.Size, object.RemoteModifiedAtAdvisory); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE integration_batch_objects
			ADD CONSTRAINT integration_batch_objects_provenance_chk CHECK (
				(provider = 's3' AND object_version <> '' AND object_etag <> '')
				OR
				(provider = 'sftp' AND object_version <> '' AND object_etag = '')
			) NOT VALID
	`); err != nil {
		t.Fatalf("re-add provenance constraint over the legacy row: %v", err)
	}

	item, err := store.Claim(ctx, "tenant-a", source, integrationDigest, object, "legacy-worker", time.Minute)
	if err != nil || item == nil {
		t.Fatalf("legacy claim = %#v, %v", item, err)
	}
	if item.ObjectVersion != object.Version || item.ObjectETag != object.ETag {
		t.Fatalf("legacy row did not adopt provenance: %#v", item)
	}
	if item.ReceivedAt.IsZero() {
		t.Fatal("legacy row lost its custody timestamp")
	}

	// Once adopted, a different exact version under the same key is rejected.
	changed := object
	changed.ETag = "7d793037a0760186574b0282f2f435e7"
	if _, err := store.Claim(ctx, "tenant-a", source, integrationDigest, changed, "legacy-worker", time.Minute); err == nil {
		t.Fatal("adopted provenance accepted a different entity tag")
	}
}

// proveUngrantedWorkloadIsInert denies a source whose declared subject holds no
// recognized submit grant, asserts every durable record class is untouched,
// then repairs the grant in a new source revision and admits the same object.
func proveUngrantedWorkloadIsInert(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
	checkpointStore *PostgresStore,
	server *batchSFTPServer,
) {
	t.Helper()
	input := filepath.Join(server.root, "denied-in")
	archive := filepath.Join(server.root, "denied-archive")
	raw := batchIdentityFixture(t, "svc-batch-denied")
	writeSFTPFixture(t, input, "denied.hl7", raw, spoofedRemoteModifiedAt)

	denied := boundSFTPSource(t, server.address, input, archive, &WorkloadIdentity{
		Subject: "svc-batch-denied", Grants: []string{"integration:observe"},
	})
	provider, err := NewSFTPProvider(denied, SFTPSecrets{KnownHostsPath: server.knownHosts, Password: "batch-pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provider.Close() }()

	before := durableRecordCounts(t, ctx, db)
	runner := newIdentityRunner(t, ctx, db, artifactResolver, checkpointStore, denied, provider, "batch-identity-denied")
	if processed, err := runner.PollOnce(ctx); processed != 0 || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ungranted poll = %d, %v; want 0, ErrUnavailable", processed, err)
	}
	after := durableRecordCounts(t, ctx, db)
	for table, want := range before {
		if after[table] != want {
			t.Fatalf("%s = %d after denial, want %d", table, after[table], want)
		}
	}
	if _, err := os.Stat(filepath.Join(input, "denied.hl7")); err != nil {
		t.Fatalf("denied source object was disturbed: %v", err)
	}

	// Repairing the grant requires a new source revision, definition revision,
	// and deploy. The previously denied object must then admit cleanly.
	repaired := boundSFTPSource(t, server.address, input, archive, &WorkloadIdentity{
		Subject: "svc-batch-denied", Grants: []string{"integration:observe", SubmitRole},
	})
	if repaired.Digest == denied.Digest {
		t.Fatal("repairing a grant must produce a new source digest")
	}
	repairedProvider, err := NewSFTPProvider(repaired, SFTPSecrets{KnownHostsPath: server.knownHosts, Password: "batch-pass"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repairedProvider.Close() }()
	runIdentityBatchSource(t, ctx, db, artifactResolver, checkpointStore, repaired, repairedProvider, "batch-identity-repaired")

	subjects := admittedSubjects(t, ctx, db)
	if subjects["svc-batch-denied"] != 2 {
		t.Fatalf("repaired subject admissions = %d, want 2 (%v)", subjects["svc-batch-denied"], subjects)
	}
}

// proveCompatibilityMode admits a source with no workload block and asserts the
// Slice 2.4 principal and server-issued grant are unchanged.
func proveCompatibilityMode(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
	checkpointStore *PostgresStore,
	endpoint, accessKey, secretKey string,
) {
	t.Helper()
	bucket := fmt.Sprintf("batch-compat-%d", time.Now().UnixNano())
	source := boundS3Source(t, endpoint, bucket, nil)
	if source.WorkloadIdentityEnabled() {
		t.Fatal("a source without a workload block must stay in compatibility mode")
	}
	provider := newIdentityS3Bucket(t, ctx, source, accessKey, secretKey)
	putS3Object(t, ctx, provider, "incoming/compat.hl7", batchIdentityFixture(t, "svc-batch-east"))
	runIdentityBatchSource(t, ctx, db, artifactResolver, checkpointStore, source, provider, "batch-identity-compat")

	subjects := admittedSubjects(t, ctx, db)
	if subjects["batch-identity-compat-principal"] != 2 {
		t.Fatalf("compatibility admissions = %v", subjects)
	}
	assertAuthMethods(t, ctx, db, AuthMethodConnectorPrefix+string(ProviderS3), 2)
	var grants int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM integration_receipts
		WHERE principal_json->>'id' = 'batch-identity-compat-principal'
		  AND principal_json->'roles' @> $1::jsonb
	`, fmt.Sprintf(`[%q]`, SubmitRole)).Scan(&grants); err != nil || grants != 2 {
		t.Fatalf("compatibility server-issued grants = %d, %v", grants, err)
	}
}

// proveTrustedProvenance asserts the durable trust boundary: canonical
// received-at equals the server-owned custody timestamp, S3 rows pin version ID
// and entity tag, and both providers record the streaming content digest.
func proveTrustedProvenance(t *testing.T, ctx context.Context, db *sql.DB, fixtureDigest string) {
	t.Helper()
	// Every canonical received-at must be exactly some object's server-owned
	// custody timestamp. No other clock may produce it.
	var mismatched int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM integration_canonical_events e
		WHERE NOT EXISTS (
			SELECT 1 FROM integration_batch_objects o
			WHERE o.tenant_id = e.tenant_id
			  AND o.created_at = (e.payload_json->>'received_at')::timestamptz
		)
	`).Scan(&mismatched); err != nil || mismatched != 0 {
		t.Fatalf("canonical received_at not aligned with custody time: %d, %v", mismatched, err)
	}
	var spoofedTrusted int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM integration_canonical_events
		WHERE (payload_json->>'received_at')::timestamptz = $1
	`, spoofedRemoteModifiedAt).Scan(&spoofedTrusted); err != nil || spoofedTrusted != 0 {
		t.Fatalf("spoofed remote modification time reached %d canonical events (%v)", spoofedTrusted, err)
	}
	var advisoryRows int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM integration_batch_objects
		WHERE provider = 'sftp' AND remote_modified_at_advisory = $1
	`, spoofedRemoteModifiedAt).Scan(&advisoryRows); err != nil || advisoryRows != 1 {
		t.Fatalf("advisory remote modification time rows = %d, %v", advisoryRows, err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT provider, object_version, object_etag, content_digest, digest_state, created_at
		FROM integration_batch_objects ORDER BY provider
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]int{}
	for rows.Next() {
		var provider, version, etag, digest, digestState string
		var createdAt time.Time
		if err := rows.Scan(&provider, &version, &etag, &digest, &digestState, &createdAt); err != nil {
			t.Fatal(err)
		}
		seen[provider]++
		if digest != fixtureDigest {
			t.Fatalf("%s content digest = %q, want streaming digest %q", provider, digest, fixtureDigest)
		}
		if digestState == "" {
			t.Fatalf("%s recorded no streaming digest continuation state", provider)
		}
		if createdAt.IsZero() || !createdAt.After(spoofedRemoteModifiedAt) {
			t.Fatalf("%s custody timestamp = %s", provider, createdAt)
		}
		switch provider {
		case string(ProviderS3):
			if !strings.HasPrefix(version, "version:") || len(version) <= len("version:") {
				t.Fatalf("S3 object_version = %q, want an exact version ID", version)
			}
			if etag == "" || strings.Contains(etag, `"`) {
				t.Fatalf("S3 object_etag = %q, want a normalized entity tag", etag)
			}
		case string(ProviderSFTP):
			if etag != "" {
				t.Fatalf("SFTP object_etag = %q, want empty", etag)
			}
			if !strings.HasPrefix(version, "sha256:") {
				t.Fatalf("SFTP object_version = %q", version)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen[string(ProviderS3)] != 1 || seen[string(ProviderSFTP)] != 1 {
		t.Fatalf("provenance rows = %v", seen)
	}
}

// proveAdvisoryModifiedTime confirms the spoofed remote value really does reach
// the connector, so the trust assertions above are not vacuous.
func proveAdvisoryModifiedTime(t *testing.T, ctx context.Context, provider *SFTPProvider) {
	t.Helper()
	objects, err := provider.List(ctx, 10)
	if err != nil || len(objects) != 1 {
		t.Fatalf("SFTP list = %v, %v", objects, err)
	}
	if !objects[0].RemoteModifiedAtAdvisory.Equal(spoofedRemoteModifiedAt) {
		t.Fatalf("advisory modification time = %s, want the spoofed %s",
			objects[0].RemoteModifiedAtAdvisory, spoofedRemoteModifiedAt)
	}
	if objects[0].ETag != "" {
		t.Fatalf("SFTP object fabricated an entity tag: %q", objects[0].ETag)
	}
}

func admittedSubjects(t *testing.T, ctx context.Context, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT principal_json->>'id', count(*) FROM integration_receipts
		WHERE status = 'accepted' GROUP BY 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	subjects := map[string]int{}
	for rows.Next() {
		var subject string
		var count int
		if err := rows.Scan(&subject, &count); err != nil {
			t.Fatal(err)
		}
		subjects[subject] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return subjects
}

func assertAuthMethods(t *testing.T, ctx context.Context, db *sql.DB, method string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM integration_receipts WHERE principal_json->>'auth_method' = $1
	`, method).Scan(&count); err != nil || count != want {
		t.Fatalf("receipts with auth method %q = %d, %v; want %d", method, count, err, want)
	}
}

func durableRecordCounts(t *testing.T, ctx context.Context, db *sql.DB) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range durableRecordTables {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s = %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func newIdentityRunner(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
	checkpointStore *PostgresStore,
	source SourceRevision,
	provider Provider,
	definitionID string,
) *Runner {
	t.Helper()
	catalog := deployBatchRevision(t, ctx, db, source, definitionID)
	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: catalog.AuthorizeRunnableSubmission,
	})
	if err != nil {
		t.Fatal(err)
	}
	definitionResolver, err := processor.NewDefinitionRevisionResolver("tenant-a", catalog)
	if err != nil {
		t.Fatal(err)
	}
	messageProcessor, err := processor.NewDurableMessageProcessor(definitionResolver, artifactResolver, submissionStore)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		TenantID: "tenant-a", DefinitionID: definitionID,
		PrincipalID: definitionID + "-principal", WorkerID: definitionID + "-worker",
		Source: source, Resolver: catalog, Processor: messageProcessor,
		Store: checkpointStore, Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func runIdentityBatchSource(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
	checkpointStore *PostgresStore,
	source SourceRevision,
	provider Provider,
	definitionID string,
) {
	t.Helper()
	runner := newIdentityRunner(t, ctx, db, artifactResolver, checkpointStore, source, provider, definitionID)
	if processed, err := runner.PollOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("%s poll = %d, %v", definitionID, processed, err)
	}
}

func newIdentityS3Bucket(
	t *testing.T,
	ctx context.Context,
	source SourceRevision,
	accessKey, secretKey string,
) *S3Provider {
	t.Helper()
	provider, err := NewS3Provider(source, S3Secrets{AccessKeyID: accessKey, SecretAccessKey: secretKey})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	if err := provider.client.MakeBucket(ctx, source.S3.Bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := provider.client.SetBucketVersioning(
		ctx, source.S3.Bucket, minio.BucketVersioningConfiguration{Status: "Enabled"},
	); err != nil {
		t.Fatal(err)
	}
	return provider
}

func putS3Object(t *testing.T, ctx context.Context, provider *S3Provider, key string, raw []byte) {
	t.Helper()
	if _, err := provider.client.PutObject(
		ctx, provider.policy.Bucket, key, bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{},
	); err != nil {
		t.Fatal(err)
	}
}

func writeSFTPFixture(t *testing.T, directory, name string, raw []byte, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

// batchIdentityFixture builds two HL7v2 messages whose MSH sending application
// and facility name the supplied subject. In-band provenance must never select
// an identity.
func batchIdentityFixture(t *testing.T, impersonated string) []byte {
	t.Helper()
	message := func(controlID string) string {
		return strings.Join([]string{
			"MSH|^~\\&|" + impersonated + "|" + impersonated + "|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|" +
				controlID + "|P|2.5.1",
			"EVN|A01|20260713120000||||20260713115900-0400",
			"PID|1||MRN-GOLDEN-001^^^HOSP^MR||Patient^Golden||19800101|F",
			"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-123|||||||||||||||||||||||||20260713120000",
		}, "\r")
	}
	return []byte(message("identity-control-001") + "\r" + message("identity-control-002"))
}

func boundS3Source(t *testing.T, endpoint, bucket string, workload *WorkloadIdentity) SourceRevision {
	t.Helper()
	source, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "source-identity-s3", RevisionID: "v1", SourceID: "adt-east",
		Provider: ProviderS3, Workload: workload,
		PollSeconds: 1, LeaseSeconds: 60, ProcessSeconds: 30,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &S3Policy{
			Endpoint: endpoint, Bucket: bucket, InputPrefix: "incoming", ArchivePrefix: "archive",
			UseTLS: false, AccessKeyBinding: "s3-access", SecretAccessKeyBinding: "s3-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func boundSFTPSource(
	t *testing.T,
	address, input, archive string,
	workload *WorkloadIdentity,
) SourceRevision {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscan(portText, &port); err != nil {
		t.Fatal(err)
	}
	source, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "source-identity-sftp", RevisionID: "v1", SourceID: "adt-west",
		Provider: ProviderSFTP, Workload: workload,
		PollSeconds: 1, LeaseSeconds: 60, ProcessSeconds: 30,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		SFTP: &SFTPPolicy{
			Host: host, Port: port, Username: "batch-user", InputDirectory: input,
			ArchiveDirectory: archive, KnownHostsBinding: "sftp-known-hosts",
			PasswordBinding: "sftp-password",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func sha256Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
