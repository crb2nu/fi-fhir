package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// loadBatchRuntimeFromEnv builds the lifecycle-gated batch runner and reports
// what it mounted for the engine runtime description.
func loadBatchRuntimeFromEnv(
	ctx context.Context,
	tenantID string,
	sourcePath string,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
) (*integrationbatch.Runner, integrationbatch.Provider, batchFacts, error) {
	if ctx == nil || tenantID == "" || sourcePath == "" || db == nil || artifactResolver == nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure batch runtime: invalid dependencies")
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("open batch source revision: %w", err)
	}
	source, decodeErr := integrationbatch.DecodeSourceRevision(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("load batch source revision: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("close batch source revision: %w", closeErr)
	}
	if err := requireBatchWorkloadIdentity(source); err != nil {
		return nil, nil, batchFacts{}, err
	}
	// requireBatchWorkloadIdentity just refused a malformed value.
	requireWorkloadIdentity, _ := optionalBoolEnv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY")
	definitionID, err := requiredEnv("FI_FHIR_BATCH_DEFINITION_ID")
	if err != nil {
		return nil, nil, batchFacts{}, err
	}
	principalID, err := requiredEnv("FI_FHIR_BATCH_PRINCIPAL_ID")
	if err != nil {
		return nil, nil, batchFacts{}, err
	}
	workerID, err := resolveBatchWorkerID(observability.ModeFromEnv())
	if err != nil {
		return nil, nil, batchFacts{}, err
	}
	facts := batchFacts{
		source: source, definitionID: definitionID, workerID: workerID,
		requireWorkloadIdentity: requireWorkloadIdentity,
	}

	provider, err := loadBatchProviderFromEnv(source)
	if err != nil {
		return nil, nil, batchFacts{}, err
	}
	closeProvider := true
	defer func() {
		if closeProvider {
			_ = provider.Close()
		}
	}()

	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{})
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure batch lifecycle catalog: %w", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("migrate batch lifecycle catalog: %w", err)
	}
	checkpointStore, err := integrationbatch.NewPostgresStore(db, nil)
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure batch checkpoint store: %w", err)
	}
	if err := checkpointStore.Migrate(ctx); err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("migrate batch checkpoint store: %w", err)
	}
	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: catalog.AuthorizeRunnableSubmission,
	})
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure durable batch submission store: %w", err)
	}
	definitionResolver, err := processor.NewDefinitionRevisionResolver(tenantID, catalog)
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure batch definition resolver: %w", err)
	}
	messageProcessor, err := processor.NewDurableMessageProcessor(definitionResolver, artifactResolver, submissionStore)
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure durable batch message processor: %w", err)
	}
	runner, err := integrationbatch.NewRunner(integrationbatch.RunnerConfig{
		TenantID: tenantID, DefinitionID: definitionID, PrincipalID: principalID,
		WorkerID: workerID, Source: source, Resolver: catalog,
		Processor: messageProcessor, Store: checkpointStore, Provider: provider,
	})
	if err != nil {
		return nil, nil, batchFacts{}, fmt.Errorf("configure batch ingestion runner: %w", err)
	}
	closeProvider = false
	return runner, provider, facts, nil
}

// requireBatchWorkloadIdentity enforces the deployment-owned switch that
// refuses compatibility mode. Workload identity lives in the immutable source
// revision, so this is what stops a swapped source document from silently
// downgrading a bound source to the shared connector principal.
func requireBatchWorkloadIdentity(source integrationbatch.SourceRevision) error {
	required, err := optionalBoolEnv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY")
	if err != nil {
		return err
	}
	if required && !source.WorkloadIdentityEnabled() {
		return fmt.Errorf(
			"FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY requires a workload block in the batch source revision",
		)
	}
	return nil
}

// batchProviderSecrets is the credential material one batch source needs, read
// from the FI_FHIR_BATCH_* keys. It exists only to construct a provider and is
// never logged, marshaled, or persisted. The type is shared with the definition
// editor's in-process batch validator.
type batchProviderSecrets = authoring.BatchSecrets

func loadBatchProviderFromEnv(source integrationbatch.SourceRevision) (integrationbatch.Provider, error) {
	secrets, err := loadBatchProviderSecretsFromEnv(source)
	if err != nil {
		return nil, err
	}
	return newBatchProvider(source, secrets)
}

// loadBatchProviderSecretsFromEnv reads exactly the keys the source's provider
// and auth mode declare. It performs no network I/O, so `lifecycle seed` can
// refuse a missing credential before it writes anything and still build the
// provider (which dials, for SFTP) inside the bounded connection validation.
func loadBatchProviderSecretsFromEnv(source integrationbatch.SourceRevision) (batchProviderSecrets, error) {
	switch source.Provider {
	case integrationbatch.ProviderS3:
		accessKey, err := loadSingleLineSecret(
			"FI_FHIR_BATCH_S3_ACCESS_KEY", "FI_FHIR_BATCH_S3_ACCESS_KEY_FILE", "batch S3 access key",
		)
		if err != nil {
			return batchProviderSecrets{}, err
		}
		secretKey, err := loadSingleLineSecret(
			"FI_FHIR_BATCH_S3_SECRET_KEY", "FI_FHIR_BATCH_S3_SECRET_KEY_FILE", "batch S3 secret key",
		)
		if err != nil {
			return batchProviderSecrets{}, err
		}
		return batchProviderSecrets{S3: integrationbatch.S3Secrets{
			AccessKeyID: accessKey, SecretAccessKey: secretKey,
		}}, nil
	case integrationbatch.ProviderSFTP:
		knownHostsPath, err := requiredEnv("FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE")
		if err != nil {
			return batchProviderSecrets{}, err
		}
		secrets := integrationbatch.SFTPSecrets{KnownHostsPath: knownHostsPath}
		if source.SFTP.PasswordBinding != "" {
			secrets.Password, err = loadSingleLineSecret(
				"FI_FHIR_BATCH_SFTP_PASSWORD", "FI_FHIR_BATCH_SFTP_PASSWORD_FILE", "batch SFTP password",
			)
		} else {
			secrets.PrivateKey, err = loadBoundedRuntimeFile("FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE", "batch SFTP private key")
			if err == nil && source.SFTP.PrivateKeyPassBinding != "" {
				var passphrase string
				passphrase, err = loadSingleLineSecret(
					"FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE",
					"FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE_FILE",
					"batch SFTP private key passphrase",
				)
				secrets.PrivateKeyPassphrase = []byte(passphrase)
			}
		}
		if err != nil {
			return batchProviderSecrets{}, err
		}
		return batchProviderSecrets{SFTP: secrets}, nil
	default:
		return batchProviderSecrets{}, fmt.Errorf("configure batch provider: unsupported provider")
	}
}

func newBatchProvider(source integrationbatch.SourceRevision, secrets batchProviderSecrets) (integrationbatch.Provider, error) {
	return authoring.NewBatchProvider(source, secrets)
}

// resolveBatchWorkerID derives a per-process batch worker identity.
//
// FI_FHIR_BATCH_WORKER_ID used to be a required environment variable, and both
// `.env.example` and `docs/operations/BATCH-INGESTION.md` handed out the same
// literal value, `fi-fhir-batch-1`. The batch store treats a *matching* owner as
// a re-lease (internal/integration/batch/store.go), which is correct for a
// restarted worker reclaiming its own lease and catastrophic for two replicas
// sharing an identity: they steal each other's live leases and process the same
// object concurrently. The existing CI replica-exclusion proof cannot see it,
// because that proof uses distinct IDs worker-a/worker-b/worker-c.
//
// The derivation mirrors the delivery worker, which has always done this
// (cmd/fi-fhir/delivery_runtime.go). The environment variable remains an
// override for deployments that mint their own identities, and the
// documentation now states the uniqueness requirement instead of publishing a
// value to copy.
func resolveBatchWorkerID(mode observability.Mode) (string, error) {
	configured := strings.TrimSpace(os.Getenv("FI_FHIR_BATCH_WORKER_ID"))
	if configured != "" {
		return configured, nil
	}
	if mode.Legacy() {
		// The negative control keeps the pre-Slice-4.3 contract: no derivation,
		// so the documented shared literal is the only way to start.
		return "", fmt.Errorf("FI_FHIR_BATCH_WORKER_ID is required")
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "", fmt.Errorf("FI_FHIR_BATCH_WORKER_ID is required when hostname is unavailable")
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(hostname), os.Getpid()), nil
}
