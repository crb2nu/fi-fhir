package main

import (
	"testing"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
)

func TestLoadBatchProviderFromEnvFailsClosedAndBuildsS3(t *testing.T) {
	source, err := integrationbatch.NewSourceRevision(integrationbatch.SourceRevisionInput{
		ArtifactID: "source", RevisionID: "v1", SourceID: "adt-east",
		Provider: integrationbatch.ProviderS3, PollSeconds: 5, LeaseSeconds: 60,
		ProcessSeconds: 30, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &integrationbatch.S3Policy{
			Endpoint: "objects.example.com:443", Bucket: "drop", InputPrefix: "incoming",
			ArchivePrefix: "archive", UseTLS: true, AccessKeyBinding: "access",
			SecretAccessKeyBinding: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"FI_FHIR_BATCH_S3_ACCESS_KEY", "FI_FHIR_BATCH_S3_ACCESS_KEY_FILE",
		"FI_FHIR_BATCH_S3_SECRET_KEY", "FI_FHIR_BATCH_S3_SECRET_KEY_FILE",
	} {
		t.Setenv(name, "")
	}
	if _, err := loadBatchProviderFromEnv(source); err == nil {
		t.Fatal("missing batch S3 credentials were accepted")
	}
	t.Setenv("FI_FHIR_BATCH_S3_ACCESS_KEY", "access-value")
	t.Setenv("FI_FHIR_BATCH_S3_SECRET_KEY", "secret-value")
	provider, err := loadBatchProviderFromEnv(source)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Type() != integrationbatch.ProviderS3 {
		t.Fatalf("provider type = %s", provider.Type())
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
}
