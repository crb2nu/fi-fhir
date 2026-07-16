package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

func loadBatchRuntimeFromEnv(
	ctx context.Context,
	tenantID string,
	sourcePath string,
	db *sql.DB,
	artifactResolver *processor.RevisionResolver,
) (*integrationbatch.Runner, integrationbatch.Provider, error) {
	if ctx == nil || tenantID == "" || sourcePath == "" || db == nil || artifactResolver == nil {
		return nil, nil, fmt.Errorf("configure batch runtime: invalid dependencies")
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open batch source revision: %w", err)
	}
	source, decodeErr := integrationbatch.DecodeSourceRevision(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, nil, fmt.Errorf("load batch source revision: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close batch source revision: %w", closeErr)
	}
	definitionID, err := requiredEnv("FI_FHIR_BATCH_DEFINITION_ID")
	if err != nil {
		return nil, nil, err
	}
	principalID, err := requiredEnv("FI_FHIR_BATCH_PRINCIPAL_ID")
	if err != nil {
		return nil, nil, err
	}
	workerID, err := requiredEnv("FI_FHIR_BATCH_WORKER_ID")
	if err != nil {
		return nil, nil, err
	}

	provider, err := loadBatchProviderFromEnv(source)
	if err != nil {
		return nil, nil, err
	}
	closeProvider := true
	defer func() {
		if closeProvider {
			_ = provider.Close()
		}
	}()

	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("configure batch lifecycle catalog: %w", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		return nil, nil, fmt.Errorf("migrate batch lifecycle catalog: %w", err)
	}
	checkpointStore, err := integrationbatch.NewPostgresStore(db, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("configure batch checkpoint store: %w", err)
	}
	if err := checkpointStore.Migrate(ctx); err != nil {
		return nil, nil, fmt.Errorf("migrate batch checkpoint store: %w", err)
	}
	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: catalog.AuthorizeRunnableSubmission,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure durable batch submission store: %w", err)
	}
	definitionResolver, err := processor.NewDefinitionRevisionResolver(tenantID, catalog)
	if err != nil {
		return nil, nil, fmt.Errorf("configure batch definition resolver: %w", err)
	}
	messageProcessor, err := processor.NewDurableMessageProcessor(definitionResolver, artifactResolver, submissionStore)
	if err != nil {
		return nil, nil, fmt.Errorf("configure durable batch message processor: %w", err)
	}
	runner, err := integrationbatch.NewRunner(integrationbatch.RunnerConfig{
		TenantID: tenantID, DefinitionID: definitionID, PrincipalID: principalID,
		WorkerID: workerID, Source: source, Resolver: catalog,
		Processor: messageProcessor, Store: checkpointStore, Provider: provider,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure batch ingestion runner: %w", err)
	}
	closeProvider = false
	return runner, provider, nil
}

func loadBatchProviderFromEnv(source integrationbatch.SourceRevision) (integrationbatch.Provider, error) {
	switch source.Provider {
	case integrationbatch.ProviderS3:
		accessKey, err := loadSingleLineSecret(
			"FI_FHIR_BATCH_S3_ACCESS_KEY", "FI_FHIR_BATCH_S3_ACCESS_KEY_FILE", "batch S3 access key",
		)
		if err != nil {
			return nil, err
		}
		secretKey, err := loadSingleLineSecret(
			"FI_FHIR_BATCH_S3_SECRET_KEY", "FI_FHIR_BATCH_S3_SECRET_KEY_FILE", "batch S3 secret key",
		)
		if err != nil {
			return nil, err
		}
		provider, err := integrationbatch.NewS3Provider(source, integrationbatch.S3Secrets{
			AccessKeyID: accessKey, SecretAccessKey: secretKey,
		})
		if err != nil {
			return nil, fmt.Errorf("configure batch S3 provider: %w", err)
		}
		return provider, nil
	case integrationbatch.ProviderSFTP:
		knownHostsPath, err := requiredEnv("FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE")
		if err != nil {
			return nil, err
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
			return nil, err
		}
		provider, err := integrationbatch.NewSFTPProvider(source, secrets)
		if err != nil {
			return nil, fmt.Errorf("configure batch SFTP provider: %w", err)
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("configure batch provider: unsupported provider")
	}
}
