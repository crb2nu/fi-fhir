package main

import (
	"strings"
	"testing"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
)

func TestLoadBatchProviderFromEnvFailsClosedAndBuildsS3(t *testing.T) {
	source := testBatchSourceRevision(t, nil)
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

func TestRequireBatchWorkloadIdentityFailsClosed(t *testing.T) {
	compatibility := testBatchSourceRevision(t, nil)
	bound := testBatchSourceRevision(t, &integrationbatch.WorkloadIdentity{
		Subject: "svc-batch-east", Grants: []string{integrationbatch.SubmitRole},
	})

	// Unset keeps the existing compatibility deployment working.
	t.Setenv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY", "")
	if err := requireBatchWorkloadIdentity(compatibility); err != nil {
		t.Fatalf("unset switch rejected compatibility mode: %v", err)
	}

	t.Setenv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY", "true")
	if err := requireBatchWorkloadIdentity(compatibility); err == nil ||
		!strings.Contains(err.Error(), "FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY") {
		t.Fatalf("compatibility source started under a required workload identity: %v", err)
	}
	if err := requireBatchWorkloadIdentity(bound); err != nil {
		t.Fatalf("bound source rejected: %v", err)
	}

	// An unparseable boolean fails closed instead of defaulting to permissive.
	t.Setenv("FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY", "yes-please")
	if err := requireBatchWorkloadIdentity(bound); err == nil ||
		!strings.Contains(err.Error(), "FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY") {
		t.Fatalf("invalid boolean error = %v", err)
	}
}

func testBatchSourceRevision(
	t *testing.T,
	workload *integrationbatch.WorkloadIdentity,
) integrationbatch.SourceRevision {
	t.Helper()
	source, err := integrationbatch.NewSourceRevision(integrationbatch.SourceRevisionInput{
		ArtifactID: "source", RevisionID: "v1", SourceID: "adt-east",
		Provider: integrationbatch.ProviderS3, Workload: workload,
		PollSeconds: 5, LeaseSeconds: 60, ProcessSeconds: 30,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &integrationbatch.S3Policy{
			Endpoint: "objects.example.com:443", Bucket: "drop", InputPrefix: "incoming",
			ArchivePrefix: "archive", UseTLS: true, AccessKeyBinding: "access",
			SecretAccessKeyBinding: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
