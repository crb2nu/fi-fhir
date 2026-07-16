package batch

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestSourceRevisionRoundTripAndLifecycleBinding(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSourceRevision(bytes.NewReader(encoded))
	if err != nil || !reflect.DeepEqual(decoded, source) {
		t.Fatalf("DecodeSourceRevision() = %#v, %v", decoded, err)
	}
	binding := testBatchBinding(source)
	if err := source.ValidateAgainst(binding); err != nil {
		t.Fatalf("ValidateAgainst() = %v", err)
	}
	binding.SecretBindings = binding.SecretBindings[:1]
	if err := source.ValidateAgainst(binding); !errors.Is(err, ErrSourceMismatch) {
		t.Fatalf("missing binding error = %v", err)
	}
}

func TestSourceRevisionFailsClosed(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	encoded, _ := json.Marshal(source)
	cases := map[string][]byte{
		"tampered":  bytes.Replace(encoded, []byte(`"poll_seconds":5`), []byte(`"poll_seconds":6`), 1),
		"duplicate": bytes.Replace(encoded, []byte(`"source_id":"batch-a"`), []byte(`"source_id":"batch-a","source_id":"batch-b"`), 1),
		"unknown":   bytes.Replace(encoded, []byte(`"source_id":"batch-a"`), []byte(`"source_id":"batch-a","secret":"value"`), 1),
		"trailing":  append(append([]byte(nil), encoded...), []byte(` {}`)...),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSourceRevision(bytes.NewReader(raw)); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSourceRevisionSecurityBoundaries(t *testing.T) {
	t.Parallel()
	base := SourceRevisionInput{
		ArtifactID: "source-batch", RevisionID: "v1", SourceID: "batch-a",
		Provider: ProviderS3, PollSeconds: 5, LeaseSeconds: 60, ProcessSeconds: 10,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &S3Policy{
			Endpoint: "objects.example.com:443", Bucket: "drop", InputPrefix: "incoming",
			ArchivePrefix: "archive", UseTLS: true, AccessKeyBinding: "access",
			SecretAccessKeyBinding: "secret",
		},
	}
	cases := map[string]func(*SourceRevisionInput){
		"plaintext remote S3": func(v *SourceRevisionInput) { v.S3.UseTLS = false },
		"archive overlap":     func(v *SourceRevisionInput) { v.S3.ArchivePrefix = "incoming/archive" },
		"lease too short":     func(v *SourceRevisionInput) { v.LeaseSeconds = v.ProcessSeconds },
		"oversize message":    func(v *SourceRevisionInput) { v.MaxMessageBytes = 1<<20 + 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := base
			policy := *base.S3
			input.S3 = &policy
			mutate(&input)
			if _, err := NewSourceRevision(input); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	base.S3.Endpoint = "127.0.0.1:9000"
	base.S3.UseTLS = false
	if _, err := NewSourceRevision(base); err != nil {
		t.Fatalf("loopback S3 should be allowed: %v", err)
	}
}

func TestCheckedInBatchSourceRevision(t *testing.T) {
	t.Parallel()
	file, err := os.Open(filepath.Join("..", "..", "..", "testdata", "golden", "integration", "adt-batch-s3", "source-revision.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	source, err := DecodeSourceRevision(file)
	if err != nil {
		t.Fatal(err)
	}
	if source.Provider != ProviderS3 || source.Digest != "sha256:d8b2381b638b3efec433cdda4a851df08c748643429231dd0cfd2939762961c3" {
		t.Fatalf("checked-in source = %#v", source)
	}
}

func testS3Source(t *testing.T) SourceRevision {
	t.Helper()
	source, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "source-batch", RevisionID: "v1", SourceID: "batch-a",
		Provider: ProviderS3, PollSeconds: 5, LeaseSeconds: 60, ProcessSeconds: 10,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		S3: &S3Policy{
			Endpoint: "objects.example.com:443", Region: "us-east-1", Bucket: "drop",
			InputPrefix: "incoming", ArchivePrefix: "archive", UseTLS: true,
			AccessKeyBinding: "access", SecretAccessKeyBinding: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func testBatchBinding(source SourceRevision) lifecycle.RunnableBinding {
	return lifecycle.RunnableBinding{
		ReleaseID: "release", SnapshotVersion: 1,
		IntegrationRevision: integration.ArtifactRevisionRef{ArtifactID: "definition", RevisionID: "v1", Digest: testDigest('d')},
		SourceRevision:      source.Reference(), SourceID: source.SourceID, Format: events.FormatHL7v2,
		Classification: integration.DataClassificationPHI,
		Deployment: integration.IntegrationDeploymentPolicy{
			ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 60},
			Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
			Health: integration.HealthPolicy{
				StartupGraceSeconds: 0, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3,
			},
			Capacity: integration.CapacityPolicy{MaxInFlight: 1, MaxQueued: 10, MaxMessagesPerSecond: 100},
		},
		SecretBindings: []integration.SecretBinding{
			{Name: "access", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "access"}},
			{Name: "secret", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "secret"}},
		},
	}
}

func testDigest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
}
