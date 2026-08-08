package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWorkloadIdentityKeepsCompatibilityDigestsStable(t *testing.T) {
	t.Parallel()
	compatibility := testS3Source(t)
	if compatibility.WorkloadIdentityEnabled() {
		t.Fatal("a source without a workload block must select compatibility mode")
	}
	encoded, err := json.Marshal(compatibility)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"workload"`)) {
		t.Fatalf("compatibility source encoded a workload key: %s", encoded)
	}

	bound := testWorkloadSource(t, "svc-batch-east", []string{SubmitRole})
	if !bound.WorkloadIdentityEnabled() || bound.Digest == compatibility.Digest {
		t.Fatalf("bound digest = %q, compatibility digest = %q", bound.Digest, compatibility.Digest)
	}
	// Grant order is presentation, not identity: it must not move the digest.
	reordered := testWorkloadSource(t, "svc-batch-east", []string{"integration:observe", SubmitRole})
	sorted := testWorkloadSource(t, "svc-batch-east", []string{SubmitRole, "integration:observe"})
	if reordered.Digest != sorted.Digest {
		t.Fatalf("grant order changed digest: %q vs %q", reordered.Digest, sorted.Digest)
	}
	decoded, err := DecodeSourceRevision(bytes.NewReader(mustMarshal(t, bound)))
	if err != nil || decoded.Workload == nil || decoded.Workload.Subject != "svc-batch-east" {
		t.Fatalf("DecodeSourceRevision() = %#v, %v", decoded, err)
	}
}

func TestWorkloadIdentityValidationFailsClosed(t *testing.T) {
	t.Parallel()
	cases := map[string]*WorkloadIdentity{
		"empty subject":     {Subject: ""},
		"spaced subject":    {Subject: "svc batch"},
		"control subject":   {Subject: "svc\nbatch"},
		"duplicate grants":  {Subject: "svc-batch", Grants: []string{SubmitRole, SubmitRole}},
		"malformed grant":   {Subject: "svc-batch", Grants: []string{"integration submit"}},
		"too many grants":   {Subject: "svc-batch", Grants: manyGrants(maxWorkloadGrants + 1)},
		"oversized subject": {Subject: strings.Repeat("s", 257)},
	}
	for name, workload := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := testWorkloadInput(workload)
			if _, err := NewSourceRevision(input); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	// A declared subject with zero grants is well-formed but unauthorized. It
	// must decode so the fail-closed submit decision — not the schema — is what
	// stops it.
	if _, err := NewSourceRevision(testWorkloadInput(&WorkloadIdentity{Subject: "svc-batch"})); err != nil {
		t.Fatalf("ungranted subject must decode: %v", err)
	}
}

func TestRunnerPrincipalComesOnlyFromTheSourceRevision(t *testing.T) {
	t.Parallel()
	compatibility := testS3Source(t)
	runner := newTestRunner(t, compatibility, newMemoryProvider(nil), newMemoryCheckpointStore(), &recordingProcessor{})
	principal, err := runner.resolvePrincipal("batch-a")
	if err != nil || principal.ID != "batch-worker" ||
		principal.AuthMethod != AuthMethodConnectorPrefix+string(ProviderS3) ||
		len(principal.Roles) != 1 || principal.Roles[0] != SubmitRole {
		t.Fatalf("compatibility principal = %#v, %v", principal, err)
	}

	bound := testWorkloadSource(t, "svc-batch-west", []string{SubmitRole})
	boundRunner := newTestRunner(t, bound, newMemoryProvider(nil), newMemoryCheckpointStore(), &recordingProcessor{})
	principal, err = boundRunner.resolvePrincipal("batch-a")
	if err != nil || principal.ID != "svc-batch-west" ||
		principal.AuthMethod != AuthMethodWorkloadIdentity || principal.SourceID != "batch-a" {
		t.Fatalf("bound principal = %#v, %v", principal, err)
	}
	// The deployment-fixed principal is never reachable once a source declares
	// a workload subject.
	if principal.ID == "batch-worker" {
		t.Fatal("bound source fell back to the deployment-fixed principal")
	}
}

func TestRunnerDeniesUngrantedWorkloadBeforeAnySideEffect(t *testing.T) {
	t.Parallel()
	source := testWorkloadSource(t, "svc-batch-observer", []string{"integration:observe"})
	provider := newMemoryProvider(testBatchRaw)
	store := newMemoryCheckpointStore()
	messageProcessor := &recordingProcessor{}
	runner := newTestRunner(t, source, provider, store, messageProcessor)

	processed, err := runner.PollOnce(context.Background())
	if processed != 0 || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ungranted poll = %d, %v", processed, err)
	}
	if provider.listCalls != 0 {
		t.Fatalf("denied source listed the remote side %d times", provider.listCalls)
	}
	if len(store.items) != 0 {
		t.Fatalf("denied source created %d durable rows", len(store.items))
	}
	if len(messageProcessor.recorded()) != 0 {
		t.Fatal("denied source reached the processor")
	}

	// Repairing the grant in a new source revision admits the same object with
	// no residue from the denial.
	repaired := testWorkloadSource(t, "svc-batch-observer", []string{"integration:observe", SubmitRole})
	repairedRunner := newTestRunner(t, repaired, provider, store, messageProcessor)
	if processed, err := repairedRunner.PollOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("repaired poll = %d, %v", processed, err)
	}
	requests := messageProcessor.recorded()
	if len(requests) != 2 {
		t.Fatalf("admitted requests = %d", len(requests))
	}
	for _, request := range requests {
		if request.Security.Principal.ID != "svc-batch-observer" {
			t.Fatalf("admitted principal = %#v", request.Security.Principal)
		}
	}
}

func TestObjectContentCannotSelectAnIdentity(t *testing.T) {
	t.Parallel()
	// The object key names one subject and the MSH sending application names
	// another. Neither may reach the principal.
	source := testWorkloadSource(t, "svc-batch-east", []string{SubmitRole})
	raw := []byte(
		"MSH|^~\\&|svc-batch-west|FAC|APP|FAC|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r",
	)
	provider := newMemoryProvider(raw)
	provider.object.Path = "incoming/svc-batch-west/claim.hl7"
	messageProcessor := &recordingProcessor{}
	runner := newTestRunner(t, source, provider, newMemoryCheckpointStore(), messageProcessor)
	if processed, err := runner.PollOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("poll = %d, %v", processed, err)
	}
	requests := messageProcessor.recorded()
	if len(requests) != 1 || requests[0].Security.Principal.ID != "svc-batch-east" {
		t.Fatalf("principal = %#v", requests)
	}
}

func TestReceiptProvenanceUsesServerCustodyTimeNotRemoteModifiedAt(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	provider := newMemoryProvider(testBatchRaw)
	// A remote side claiming the object was written in 1994.
	spoofed := memoryCustodyTime.AddDate(-32, 0, 0)
	provider.object.RemoteModifiedAtAdvisory = spoofed
	messageProcessor := &recordingProcessor{}
	runner := newTestRunner(t, source, provider, newMemoryCheckpointStore(), messageProcessor)
	if processed, err := runner.PollOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("poll = %d, %v", processed, err)
	}
	requests := messageProcessor.recorded()
	if len(requests) == 0 {
		t.Fatal("no admitted request")
	}
	for _, request := range requests {
		if !request.Envelope.ReceivedAt.Equal(memoryCustodyTime) {
			t.Fatalf("received_at = %s, want server custody time %s",
				request.Envelope.ReceivedAt, memoryCustodyTime)
		}
		if request.Envelope.ReceivedAt.Equal(spoofed) {
			t.Fatal("receipt provenance trusted the remote modification time")
		}
	}
}

func TestStreamingDigestResumesAcrossCheckpoints(t *testing.T) {
	t.Parallel()
	raw := []byte("first-chunk|second-chunk|third-chunk")
	whole := sha256.Sum256(raw)
	want := "sha256:" + hex.EncodeToString(whole[:])

	single, err := newStreamDigest("")
	if err != nil || single.write(raw) != nil {
		t.Fatalf("single-pass digest = %v", err)
	}
	got, err := single.sum()
	if err != nil || got != want {
		t.Fatalf("single-pass sum = %q, %v; want %q", got, err, want)
	}

	// Hash the same bytes across three processes, carrying only the marshaled
	// continuation state between them.
	state := ""
	for _, chunk := range [][]byte{raw[:5], raw[5:20], raw[20:]} {
		resumed, resumeErr := newStreamDigest(state)
		if resumeErr != nil {
			t.Fatalf("resume = %v", resumeErr)
		}
		if err := resumed.write(chunk); err != nil {
			t.Fatal(err)
		}
		state, err = resumed.state()
		if err != nil || state == "" {
			t.Fatalf("state = %q, %v", state, err)
		}
	}
	final, err := newStreamDigest(state)
	if err != nil {
		t.Fatal(err)
	}
	got, err = final.sum()
	if err != nil || got != want {
		t.Fatalf("resumed sum = %q, %v; want %q", got, err, want)
	}

	for name, corrupt := range map[string]string{
		"not base64": "!!!!",
		"truncated":  state[:8],
		"oversized":  strings.Repeat("A", maxDigestStateBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newStreamDigest(corrupt); !errors.Is(err, ErrDigestState) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMessageRawCoversTheExactByteInterval(t *testing.T) {
	t.Parallel()
	reader, err := NewMessageReader(bytes.NewReader(testBatchRaw), 0, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var replayed []byte
	var offset int64
	for {
		message, readErr := reader.Next()
		if readErr != nil {
			break
		}
		if message.StartOffset != offset {
			t.Fatalf("start offset = %d, want %d", message.StartOffset, offset)
		}
		if int64(len(message.Raw)) != message.EndOffset-message.StartOffset {
			t.Fatalf("raw length = %d, interval = %d",
				len(message.Raw), message.EndOffset-message.StartOffset)
		}
		replayed = append(replayed, message.Raw...)
		offset = message.EndOffset
	}
	if !bytes.Equal(replayed, testBatchRaw) {
		t.Fatalf("replayed raw bytes differ from the source object")
	}
}

func TestObjectProvenanceFieldsAreProviderScoped(t *testing.T) {
	t.Parallel()
	base := Object{
		Provider: ProviderS3, Path: "incoming/a.hl7", Version: "version:v1",
		ETag: "0cc175b9c0f1b6a831c399e269772661", Size: 4,
		RemoteModifiedAtAdvisory: memoryCustodyTime,
	}
	if err := base.validate(); err != nil {
		t.Fatalf("valid S3 object = %v", err)
	}
	missingETag := base
	missingETag.ETag = ""
	if err := missingETag.validate(); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("S3 object without an entity tag = %v", err)
	}
	quotedETag := base
	quotedETag.ETag = `"0cc175b9"`
	if err := quotedETag.validate(); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("quoted entity tag = %v", err)
	}
	sftp := base
	sftp.Provider = ProviderSFTP
	if err := sftp.validate(); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("SFTP object with a fabricated entity tag = %v", err)
	}
	sftp.ETag = ""
	if err := sftp.validate(); err != nil {
		t.Fatalf("valid SFTP object = %v", err)
	}
}

func TestNormalizeETagStripsTransportQuoting(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		`"abc"`:   "abc",
		`W/"abc"`: "abc",
		` abc `:   "abc",
		"abc":     "abc",
	} {
		if got := normalizeETag(input); got != want {
			t.Fatalf("normalizeETag(%q) = %q, want %q", input, got, want)
		}
	}
}

var testBatchRaw = []byte(
	"MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r" +
		"MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|TWO|P|2.5\rPID|1||456\r",
)

func testWorkloadInput(workload *WorkloadIdentity) SourceRevisionInput {
	return SourceRevisionInput{
		ArtifactID: "source-batch", RevisionID: "v1", SourceID: "batch-a",
		Provider: ProviderS3, PollSeconds: 5, LeaseSeconds: 60, ProcessSeconds: 10,
		MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20, Workload: workload,
		S3: &S3Policy{
			Endpoint: "objects.example.com:443", Region: "us-east-1", Bucket: "drop",
			InputPrefix: "incoming", ArchivePrefix: "archive", UseTLS: true,
			AccessKeyBinding: "access", SecretAccessKeyBinding: "secret",
		},
	}
}

func testWorkloadSource(t *testing.T, subject string, grants []string) SourceRevision {
	t.Helper()
	source, err := NewSourceRevision(testWorkloadInput(&WorkloadIdentity{Subject: subject, Grants: grants}))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func manyGrants(count int) []string {
	grants := make([]string, 0, count)
	for index := 0; index < count; index++ {
		grants = append(grants, fmt.Sprintf("integration:grant-%d", index))
	}
	return grants
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
