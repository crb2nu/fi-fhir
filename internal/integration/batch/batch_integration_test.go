//go:build integration

package batch

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/sftp"
	"github.com/testcontainers/testcontainers-go"
	miniocontainer "github.com/testcontainers/testcontainers-go/modules/minio"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestBatchIngestion_PostgresS3SFTPKillResumeArchive(t *testing.T) {
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
	proveBatchLeaseAndMutation(t, ctx, db)

	raw := batchFixture(t)
	t.Run("S3", func(t *testing.T) {
		endpoint, accessKey, secretKey := batchMinIO(t, ctx)
		source := batchS3Source(t, endpoint)
		provider, err := NewS3Provider(source, S3Secrets{AccessKeyID: accessKey, SecretAccessKey: secretKey})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = provider.Close() }()
		if err := provider.client.MakeBucket(ctx, source.S3.Bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := provider.List(ctx, 10); err == nil {
			t.Fatal("S3 polling accepted a bucket without versioning")
		}
		if err := provider.client.SetBucketVersioning(ctx, source.S3.Bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
			t.Fatal(err)
		}
		proveS3ExactVersionDelete(t, ctx, provider)
		if _, err := provider.client.PutObject(
			ctx, source.S3.Bucket, "incoming/batch.hl7", bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{},
		); err != nil {
			t.Fatal(err)
		}
		runBatchRecoveryProof(t, ctx, db, artifactResolver, checkpointStore, source, provider, raw, "batch-s3")
		objects, err := provider.List(ctx, 10)
		if err != nil || len(objects) != 0 {
			t.Fatalf("source list after archive = %v, %v", objects, err)
		}
	})

	t.Run("SFTP", func(t *testing.T) {
		server := startBatchSFTPServer(t)
		inputDirectory := filepath.Join(server.root, "incoming")
		archiveDirectory := filepath.Join(server.root, "archive")
		if err := os.MkdirAll(inputDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(inputDirectory, "batch.hl7"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		source := batchSFTPSource(t, server.address, inputDirectory, archiveDirectory)
		wrongKnownHosts := filepath.Join(t.TempDir(), "known_hosts")
		_, wrongPrivate, _ := ed25519.GenerateKey(rand.Reader)
		wrongSigner, _ := ssh.NewSignerFromKey(wrongPrivate)
		if err := os.WriteFile(wrongKnownHosts, []byte(knownhosts.Line([]string{knownhosts.Normalize(server.address)}, wrongSigner.PublicKey())+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if insecure, err := NewSFTPProvider(source, SFTPSecrets{KnownHostsPath: wrongKnownHosts, Password: "batch-pass"}); err == nil {
			_ = insecure.Close()
			t.Fatal("mismatched SFTP host key was accepted")
		}
		provider, err := NewSFTPProvider(source, SFTPSecrets{KnownHostsPath: server.knownHosts, Password: "batch-pass"})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = provider.Close() }()
		runBatchRecoveryProof(t, ctx, db, artifactResolver, checkpointStore, source, provider, raw, "batch-sftp")
		if _, err := os.Stat(filepath.Join(inputDirectory, "batch.hl7")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("SFTP source still exists: %v", err)
		}
		archives, err := filepath.Glob(filepath.Join(archiveDirectory, "*", "batch.hl7"))
		if err != nil || len(archives) != 1 {
			t.Fatalf("SFTP archives = %v, %v", archives, err)
		}
		archived, err := os.ReadFile(archives[0])
		if err != nil || !bytes.Equal(archived, raw) {
			t.Fatalf("SFTP archive mismatch: %v", err)
		}
	})

	var rawLeakCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM integration_batch_objects
		WHERE row_to_json(integration_batch_objects)::text LIKE '%RAW-GOLDEN-PHI-SENTINEL%'
	`).Scan(&rawLeakCount); err != nil || rawLeakCount != 0 {
		t.Fatalf("batch checkpoint raw leakage count = %d, %v", rawLeakCount, err)
	}
}

func proveS3ExactVersionDelete(t *testing.T, ctx context.Context, provider *S3Provider) {
	t.Helper()
	key := "incoming/version-race.hl7"
	first := []byte("first-version")
	second := []byte("second-version")
	if _, err := provider.client.PutObject(
		ctx, provider.policy.Bucket, key, bytes.NewReader(first), int64(len(first)), minio.PutObjectOptions{},
	); err != nil {
		t.Fatal(err)
	}
	objects, err := provider.List(ctx, 10)
	if err != nil || len(objects) != 1 {
		t.Fatalf("first S3 version = %v, %v", objects, err)
	}
	oldVersion := objects[0]
	if _, err := provider.client.PutObject(
		ctx, provider.policy.Bucket, key, bytes.NewReader(second), int64(len(second)), minio.PutObjectOptions{},
	); err != nil {
		t.Fatal(err)
	}
	oldDigest, err := digestReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.DeleteSource(ctx, oldVersion, oldDigest); err != nil {
		t.Fatalf("delete exact old S3 version: %v", err)
	}
	objects, err = provider.List(ctx, 10)
	if err != nil || len(objects) != 1 || objects[0].Version == oldVersion.Version {
		t.Fatalf("current S3 version after old delete = %v, %v", objects, err)
	}
	wantDigest, err := digestReader(bytes.NewReader(second))
	if err != nil {
		t.Fatal(err)
	}
	gotDigest, err := provider.Digest(ctx, objects[0])
	if err != nil || gotDigest != wantDigest {
		t.Fatalf("new S3 version digest = %q, %v; want %q", gotDigest, err, wantDigest)
	}
	if err := provider.DeleteSource(ctx, objects[0], wantDigest); err != nil {
		t.Fatal(err)
	}
}

func proveBatchLeaseAndMutation(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store, err := NewPostgresStore(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	source := testS3Source(t)
	object := Object{
		Provider: ProviderS3, Path: "incoming/lease.hl7", Version: "etag:v1",
		ETag: "0cc175b9c0f1b6a831c399e269772661", Size: 10,
		RemoteModifiedAtAdvisory: now.Add(-time.Minute),
	}
	integrationDigest := testDigest('a')
	first, err := store.Claim(ctx, "tenant-a", source, integrationDigest, object, "worker-a", time.Minute)
	if err != nil || first == nil {
		t.Fatalf("first lease = %#v, %v", first, err)
	}
	busy, err := store.Claim(ctx, "tenant-a", source, integrationDigest, object, "worker-b", time.Minute)
	if err != nil || busy != nil {
		t.Fatalf("live competing lease = %#v, %v", busy, err)
	}
	now = now.Add(2 * time.Minute)
	reclaimed, err := store.Claim(ctx, "tenant-a", source, integrationDigest, object, "worker-b", time.Minute)
	if err != nil || reclaimed == nil || reclaimed.LeaseOwner != "worker-b" || reclaimed.ObjectID != first.ObjectID {
		t.Fatalf("reclaimed lease = %#v, %v", reclaimed, err)
	}
	mutated := object
	mutated.Version = "etag:v2"
	mutated.ETag = "92eb5ffee6ae2fec3ad71c777531578f"
	mutated.RemoteModifiedAtAdvisory = now
	newVersion, err := store.Claim(ctx, "tenant-a", source, integrationDigest, mutated, "worker-c", time.Minute)
	if err != nil || newVersion == nil || newVersion.ObjectID == first.ObjectID || newVersion.CheckpointOffset != 0 {
		t.Fatalf("mutated object lease = %#v, %v", newVersion, err)
	}
	if changed, err := store.Claim(ctx, "tenant-a", source, testDigest('b'), object, "worker-d", time.Minute); !errors.Is(err, ErrObjectChanged) || changed != nil {
		t.Fatalf("changed integration revision claim = %#v, %v", changed, err)
	}
}

func runBatchRecoveryProof(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
	checkpointStore *PostgresStore,
	source SourceRevision,
	provider Provider,
	raw []byte,
	definitionID string,
) {
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
		TenantID: "tenant-a", DefinitionID: definitionID, PrincipalID: definitionID + "-principal",
		WorkerID: definitionID + "-worker", Source: source, Resolver: catalog,
		Processor: messageProcessor, Store: checkpointStore, Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	crashed := false
	runner.faultHook = func(checkpoint string) error {
		if checkpoint == "after_admission" && !crashed {
			crashed = true
			return errors.New("simulated process kill")
		}
		return nil
	}
	if _, err := runner.PollOnce(ctx); err == nil || !crashed {
		t.Fatalf("kill-window poll = %v", err)
	}
	runner.faultHook = nil
	if processed, err := runner.PollOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("restart poll = %d, %v", processed, err)
	}
	for table, wanted := range map[string]int{
		"integration_receipts":         2,
		"integration_canonical_events": 2,
		"integration_delivery_outbox":  2,
	} {
		var count int
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE tenant_id = $1`, pq.QuoteIdentifier(table))
		if err := db.QueryRowContext(ctx, query, "tenant-a").Scan(&count); err != nil {
			t.Fatal(err)
		}
		// SFTP runs after S3 in the parent test and therefore doubles the total.
		if definitionID == "batch-sftp" {
			wanted *= 2
		}
		if count != wanted {
			t.Fatalf("%s count = %d, want %d (raw bytes %d)", table, count, wanted, len(raw))
		}
	}
}

func deployBatchRevision(t *testing.T, ctx context.Context, db *sql.DB, source SourceRevision, definitionID string) *lifecycle.PostgresCatalog {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{
		Clock: func() time.Time { return now },
		ValidateConnection: func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
			return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}}, nil
		},
	})
	if err != nil || catalog.Migrate(ctx) != nil {
		t.Fatalf("lifecycle catalog = %v", err)
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: definitionID, RevisionID: "v1", TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
		Format: events.FormatHL7v2,
		Profile: integration.ArtifactRevisionRef{
			ArtifactID: "profile-adt", RevisionID: "1",
			Digest: "sha256:79c8f575ae135f6d6c10d46fcb57c75a6094ac65c8ab6d10912e5050b26315d3",
		},
		Workflow: integration.ArtifactRevisionRef{
			ArtifactID: "workflow-adt", RevisionID: "workflow-version-1",
			Digest: "sha256:7e20cd2b8e591c507a313eb9a422318384d7e7b7c895798c2a264182dad688dc",
		},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: testDigest('d'),
			},
			Class: integration.DestinationClassProduction,
		}},
		SecretBindings: batchSecretBindings(source),
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: &integration.IntegrationDeploymentPolicy{
			ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
			Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
			Health: integration.HealthPolicy{
				StartupGraceSeconds: 1, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3,
			},
			Capacity: integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 10, MaxMessagesPerSecond: 100},
		},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "operator", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:operator"},
			},
			Reason: "batch integration proof", OccurredAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := catalog.CreateDraft(ctx, revision)
	if err != nil {
		t.Fatal(err)
	}
	command := func(reason string) lifecycle.Command {
		return lifecycle.Command{
			TenantID: revision.TenantID, DefinitionID: revision.DefinitionID, RevisionID: revision.RevisionID,
			ExpectedVersion: snapshot.Version,
			Principal: integration.Principal{
				ID: "operator", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:operator"},
			},
			Reason: reason,
		}
	}
	snapshot, err = catalog.ValidateConnection(ctx, command("validate batch source"))
	if err == nil {
		snapshot, err = catalog.Approve(ctx, command("approve batch source"))
	}
	if err == nil {
		snapshot, err = catalog.Publish(ctx, command("publish batch source"))
	}
	if err == nil {
		snapshot, err = catalog.Deploy(ctx, command("deploy batch source"))
	}
	if err != nil || snapshot.State != integration.DeploymentStateDeployed {
		t.Fatalf("deploy batch revision = %#v, %v", snapshot, err)
	}
	return catalog
}

func batchSecretBindings(source SourceRevision) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, len(source.secretBindingNames()))
	for _, name := range source.secretBindingNames() {
		bindings = append(bindings, integration.SecretBinding{
			Name:      name,
			Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "batch/" + name},
		})
	}
	return bindings
}

func batchArtifactResolver(t *testing.T) *processor.RevisionResolver {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	staticRegistry, decodeErr := registry.DecodeStaticRegistry(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		t.Fatalf("registry = %v, close = %v", decodeErr, closeErr)
	}
	resolver, err := processor.NewRevisionResolver("tenant-a", staticRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func batchFixture(t *testing.T) []byte {
	t.Helper()
	message := func(controlID string) []byte {
		segments := []string{
			"MSH|^~\\&|RAW-GOLDEN-PHI-SENTINEL|FAC|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
			"EVN|A01|20260713120000||||20260713115900-0400",
			"PID|1||MRN-GOLDEN-001^^^HOSP^MR||Patient^Golden||19800101|F",
			"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-123|||||||||||||||||||||||||20260713120000",
		}
		return []byte(strings.Join(segments, "\r"))
	}
	first := message("golden-control-001")
	second := message("golden-control-002")
	return append(append(append([]byte(nil), first...), '\r'), second...)
}

func batchS3Source(t *testing.T, endpoint string) SourceRevision {
	t.Helper()
	source, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "source-batch-s3", RevisionID: "v1", SourceID: "adt-east", Provider: ProviderS3,
		PollSeconds: 1, LeaseSeconds: 60, ProcessSeconds: 30, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &S3Policy{
			Endpoint: endpoint, Bucket: "batch-proof", InputPrefix: "incoming", ArchivePrefix: "archive",
			UseTLS: false, AccessKeyBinding: "s3-access", SecretAccessKeyBinding: "s3-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func batchSFTPSource(t *testing.T, address, inputDirectory, archiveDirectory string) SourceRevision {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	_, _ = fmt.Sscan(portText, &port)
	source, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "source-batch-sftp", RevisionID: "v1", SourceID: "adt-east", Provider: ProviderSFTP,
		PollSeconds: 1, LeaseSeconds: 60, ProcessSeconds: 30, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		SFTP: &SFTPPolicy{
			Host: host, Port: port, Username: "batch-user", InputDirectory: inputDirectory,
			ArchiveDirectory: archiveDirectory, KnownHostsBinding: "sftp-known-hosts", PasswordBinding: "sftp-password",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func openBatchPostgres(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		container, err := postgrescontainer.Run(ctx, "postgres:16-alpine",
			postgrescontainer.WithDatabase("fi_fhir_batch"), postgrescontainer.WithUsername("testuser"),
			postgrescontainer.WithPassword("testpass"), postgrescontainer.BasicWaitStrategies(),
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
	schema := fmt.Sprintf("batch_%d", time.Now().UnixNano())
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
		t.Fatalf("open batch database = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
		_ = admin.Close()
	})
	return db
}

func batchMinIO(t *testing.T, ctx context.Context) (string, string, string) {
	t.Helper()
	endpoint := os.Getenv("BATCH_S3_ENDPOINT")
	accessKey := os.Getenv("BATCH_S3_ACCESS_KEY")
	secretKey := os.Getenv("BATCH_S3_SECRET_KEY")
	if endpoint != "" || accessKey != "" || secretKey != "" {
		if endpoint == "" || accessKey == "" || secretKey == "" {
			t.Fatal("BATCH_S3_ENDPOINT, BATCH_S3_ACCESS_KEY, and BATCH_S3_SECRET_KEY must be set together")
		}
		return endpoint, accessKey, secretKey
	}
	if os.Getenv("CI") != "" {
		t.Fatal("batch S3 environment is required in CI")
	}
	container, err := miniocontainer.Run(ctx, "minio/minio:RELEASE.2025-04-22T22-12-26Z",
		miniocontainer.WithUsername("batchaccess"), miniocontainer.WithPassword("batchsecret123"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	endpoint, err = container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint, container.Username, container.Password
}

type batchSFTPServer struct {
	root       string
	address    string
	knownHosts string
	listener   net.Listener
	wait       sync.WaitGroup
}

func startBatchSFTPServer(t *testing.T) *batchSFTPServer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &batchSFTPServer{root: t.TempDir(), address: listener.Addr().String(), listener: listener}
	server.knownHosts = filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(server.address)}, signer.PublicKey()) + "\n"
	if err := os.WriteFile(server.knownHosts, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if metadata.User() != "batch-user" || string(password) != "batch-pass" {
			return nil, errors.New("denied")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	server.wait.Add(1)
	go func() {
		defer server.wait.Done()
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			server.wait.Add(1)
			go func() {
				defer server.wait.Done()
				sshConnection, channels, requests, handshakeErr := ssh.NewServerConn(connection, config)
				if handshakeErr != nil {
					_ = connection.Close()
					return
				}
				defer func() { _ = sshConnection.Close() }()
				go ssh.DiscardRequests(requests)
				for channelRequest := range channels {
					if channelRequest.ChannelType() != "session" {
						_ = channelRequest.Reject(ssh.UnknownChannelType, "session required")
						continue
					}
					channel, channelRequests, channelErr := channelRequest.Accept()
					if channelErr != nil {
						continue
					}
					for request := range channelRequests {
						accepted := request.Type == "subsystem" && len(request.Payload) >= 4 && string(request.Payload[4:]) == "sftp"
						_ = request.Reply(accepted, nil)
						if !accepted {
							continue
						}
						sftpServer, serverErr := sftp.NewServer(channel)
						if serverErr == nil {
							_ = sftpServer.Serve()
							_ = sftpServer.Close()
						}
						break
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		server.wait.Wait()
	})
	return server
}
