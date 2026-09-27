package connection

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The peek's read half without a database or a bucket: a recording provider
// stands in for S3 and fails the test on any call a peek must never make.

type recordingProvider struct {
	t       *testing.T
	objects []batch.Object
	content map[string][]byte
	lists   []int
	opens   []string
	closed  bool
}

func (p *recordingProvider) Type() batch.ProviderType { return batch.ProviderS3 }

func (p *recordingProvider) List(_ context.Context, limit int) ([]batch.Object, error) {
	p.lists = append(p.lists, limit)
	if len(p.objects) > limit {
		return p.objects[:limit], nil
	}
	return p.objects, nil
}

func (p *recordingProvider) OpenAt(_ context.Context, object batch.Object, offset int64) (io.ReadCloser, error) {
	if offset != 0 {
		p.t.Errorf("a peek opened %s at offset %d; it has no checkpoint to resume from", object.Path, offset)
	}
	p.opens = append(p.opens, object.Path)
	return io.NopCloser(bytes.NewReader(p.content[object.Path])), nil
}

func (p *recordingProvider) Digest(context.Context, batch.Object) (string, error) {
	p.t.Error("a peek hashed an object")
	return "", errInjected
}

func (p *recordingProvider) PrepareArchive(context.Context, batch.Object, string) (string, error) {
	p.t.Error("a peek archived an object")
	return "", errInjected
}

func (p *recordingProvider) DeleteSource(context.Context, batch.Object, string) error {
	p.t.Error("a peek deleted an object")
	return errInjected
}

func (p *recordingProvider) Close() error {
	p.closed = true
	return nil
}

// staticResolver resolves env references from a map; anything else fails.
type staticResolver map[string]string

func (r staticResolver) Resolve(_ context.Context, reference integration.SecretReference) ([]byte, error) {
	value, ok := r[reference.Key]
	if !ok || reference.Provider != integration.SecretProviderEnvironment {
		return nil, integration.ErrSecretUnresolvable
	}
	return []byte(value), nil
}

func peekSource(t *testing.T) batch.SourceRevision {
	t.Helper()
	source, err := batch.NewSourceRevision(batch.SourceRevisionInput{
		ArtifactID: "adt-drop", RevisionID: "1", SourceID: "adt-east", Provider: batch.ProviderS3,
		PollSeconds: 15, LeaseSeconds: 120, ProcessSeconds: 60, MaxFilesPerPoll: 100, MaxMessageBytes: 1 << 20,
		S3: &batch.S3Policy{
			Endpoint: "127.0.0.1:9000", Bucket: "adt-drop", InputPrefix: "incoming", ArchivePrefix: "archive",
			AccessKeyBinding: "s3-access", SecretAccessKeyBinding: "s3-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewSourceRevision: %v", err)
	}
	return source
}

func peekBindingsFor(keys ...string) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, len(keys))
	for _, key := range keys {
		bindings = append(bindings, integration.SecretBinding{
			Name: key, Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: strings.ToUpper(strings.ReplaceAll(key, "-", "_"))},
		})
	}
	return bindings
}

func batchObject(path string, size int) batch.Object {
	return batch.Object{
		Provider: batch.ProviderS3, Path: path, Version: "version:v1", ETag: "0cc175b9c0f1b6a831c399e269772661",
		Size: int64(size), RemoteModifiedAtAdvisory: time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC),
	}
}

func syntheticBatch(messages int) []byte {
	var buffer bytes.Buffer
	for index := 1; index <= messages; index++ {
		buffer.WriteString("MSH|^~\\&|S|F|R|F|20260926||ADT^A01|control-")
		buffer.WriteByte(byte('0' + index))
		buffer.WriteString("|P|2.5.1\rPID|1||MRN-SYNTH||Synthetic^Batch\rNK1|1|Synthetic^Kin\r")
	}
	return buffer.Bytes()
}

func newPeekRun(t *testing.T, provider *recordingProvider, sessions *fakeSessions, request PeekRequest, resolver integration.SecretResolver) (*peekRun, *int) {
	t.Helper()
	built := 0
	return &peekRun{
		intake: &sampleIntake{
			sessions: sessions, secrets: resolver,
			providers: func(_ context.Context, source batch.SourceRevision, material map[string][]byte) (batch.Provider, error) {
				built++
				if string(material[source.S3.AccessKeyBinding]) != "synthetic-access" ||
					string(material[source.S3.SecretAccessKeyBinding]) != "synthetic-secret" {
					t.Errorf("the provider was built with material %v", material)
				}
				return provider, nil
			},
		},
		source: peekSource(t), audit: Capture{ID: "peek-1"}, request: request,
		maxObjects: 2, maxMessages: 2,
	}, &built
}

func TestPeekRun_ReadsTheFirstMessagesWithoutLeaseCheckpointArchiveOrDelete(t *testing.T) {
	provider := &recordingProvider{t: t,
		objects: []batch.Object{batchObject("incoming/a.hl7", 10), batchObject("incoming/b.hl7", 10), batchObject("incoming/c.hl7", 10)},
		content: map[string][]byte{"incoming/c.hl7": syntheticBatch(3)},
	}
	sessions := &fakeSessions{}
	resolver := staticResolver{"S3_ACCESS": "synthetic-access\n", "S3_SECRET": "synthetic-secret"}
	run, built := newPeekRun(t, provider, sessions, PeekRequest{ObjectPath: "incoming/c.hl7", SessionID: "sess-1"}, resolver)
	run.read(context.Background(), peekBindingsFor("s3-access", "s3-secret"))

	if len(run.problems) != 0 || *built != 1 {
		t.Fatalf("problems = %+v, provider built %d times", run.problems, *built)
	}
	if !reflect.DeepEqual(provider.lists, []int{peekLookupObjects}) || !reflect.DeepEqual(provider.opens, []string{"incoming/c.hl7"}) || !provider.closed {
		t.Fatalf("lists %v, opens %v, closed %v", provider.lists, provider.opens, provider.closed)
	}
	if len(run.objects) != 2 || run.objects[0].Path != "incoming/a.hl7" {
		t.Fatalf("objects = %+v, want the first maxObjects of the listing", run.objects)
	}
	added := sessions.added()
	if len(added) != 2 || len(run.samples) != 2 {
		t.Fatalf("added %d samples, want maxMessages 2 of the object's 3", len(added))
	}
	for index, request := range added {
		number := index + 1
		// The sample names its audit row only; the object path stays on the
		// row (review S4).
		if request.Source != "peek:peek-1" || request.Name != "peek peek-1 #"+string(rune('0'+number)) ||
			request.Redaction != session.SampleRedactionCapture || request.PHIPolicy != session.PHIPolicyRedact ||
			!strings.HasPrefix(request.Raw, "MSH|") || strings.Contains(request.Source+request.Name, "incoming/") {
			t.Fatalf("sample %d = %+v", number, request)
		}
	}
}

func TestPeekRun_ListOnlyReadsNothing(t *testing.T) {
	provider := &recordingProvider{t: t, objects: []batch.Object{batchObject("incoming/a.hl7", 10)}}
	sessions := &fakeSessions{}
	resolver := staticResolver{"S3_ACCESS": "synthetic-access", "S3_SECRET": "synthetic-secret"}
	run, _ := newPeekRun(t, provider, sessions, PeekRequest{SessionID: "sess-1"}, resolver)
	run.read(context.Background(), peekBindingsFor("s3-access", "s3-secret"))
	if len(run.problems) != 0 || len(provider.opens) != 0 || len(sessions.added()) != 0 || !reflect.DeepEqual(provider.lists, []int{2}) {
		t.Fatalf("list-only peek: problems %+v, opens %v, samples %d, lists %v",
			run.problems, provider.opens, len(sessions.added()), provider.lists)
	}
}

func TestPeekRun_UnresolvableBindingContactsNothing(t *testing.T) {
	for name, fixture := range map[string]struct {
		bindings []integration.SecretBinding
		resolver integration.SecretResolver
		path     string
	}{
		"undeclared on the draft":     {peekBindingsFor("s3-access"), staticResolver{"S3_ACCESS": "synthetic-access"}, "s3.secret_access_key_binding"},
		"no resolver on this replica": {peekBindingsFor("s3-access", "s3-secret"), nil, "s3.access_key_binding"},
		"unresolvable reference":      {peekBindingsFor("s3-access", "s3-secret"), staticResolver{"S3_ACCESS": "synthetic-access"}, "s3.secret_access_key_binding"},
		"multi-line credential":       {peekBindingsFor("s3-access", "s3-secret"), staticResolver{"S3_ACCESS": "synthetic\naccess", "S3_SECRET": "synthetic-secret"}, "s3.access_key_binding"},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &recordingProvider{t: t}
			run, built := newPeekRun(t, provider, &fakeSessions{}, PeekRequest{ObjectPath: "incoming/a.hl7"}, fixture.resolver)
			run.read(context.Background(), fixture.bindings)
			if *built != 0 || len(provider.lists) != 0 {
				t.Fatal("a peek with an unresolvable binding contacted the source")
			}
			if len(run.problems) == 0 || run.problems[0].Code != CodeSecretUnresolvable || run.problems[0].Path != fixture.path {
				t.Fatalf("problems = %+v, want SECRET_UNRESOLVABLE at %s", run.problems, fixture.path)
			}
			if strings.Contains(run.problems[0].Message, "synthetic") {
				t.Fatal("a problem message carried resolved material")
			}
		})
	}
}

func TestPeekRun_MissingObjectAndUnreadableStream(t *testing.T) {
	resolver := staticResolver{"S3_ACCESS": "synthetic-access", "S3_SECRET": "synthetic-secret"}
	provider := &recordingProvider{t: t, objects: []batch.Object{batchObject("incoming/a.hl7", 10)},
		content: map[string][]byte{"incoming/a.hl7": []byte("EVN|A01\rPID|1\r")}}

	missing, _ := newPeekRun(t, provider, &fakeSessions{}, PeekRequest{ObjectPath: "incoming/gone.hl7"}, resolver)
	missing.read(context.Background(), peekBindingsFor("s3-access", "s3-secret"))
	if len(missing.problems) != 1 || missing.problems[0].Code != CodeObjectNotFound || len(provider.opens) != 0 {
		t.Fatalf("missing object: problems %+v, opens %v", missing.problems, provider.opens)
	}

	unreadable, _ := newPeekRun(t, provider, &fakeSessions{}, PeekRequest{ObjectPath: "incoming/a.hl7"}, resolver)
	unreadable.read(context.Background(), peekBindingsFor("s3-access", "s3-secret"))
	if len(unreadable.problems) != 1 || unreadable.problems[0].Code != CodeMessageUnreadable || len(unreadable.samples) != 0 {
		t.Fatalf("unreadable stream: problems %+v, samples %d", unreadable.problems, len(unreadable.samples))
	}
	if strings.Contains(unreadable.problems[0].Message, "EVN") {
		t.Fatal("a problem message carried object content")
	}
}

func TestPeekRun_SessionRefusalStopsThePeek(t *testing.T) {
	resolver := staticResolver{"S3_ACCESS": "synthetic-access", "S3_SECRET": "synthetic-secret"}
	provider := &recordingProvider{t: t, objects: []batch.Object{batchObject("incoming/a.hl7", 10)},
		content: map[string][]byte{"incoming/a.hl7": syntheticBatch(2)}}
	run, _ := newPeekRun(t, provider, &fakeSessions{addErr: errInjected}, PeekRequest{ObjectPath: "incoming/a.hl7"}, resolver)
	run.read(context.Background(), peekBindingsFor("s3-access", "s3-secret"))
	if len(run.problems) != 1 || run.problems[0].Code != CodeSampleWriteFailed || len(run.samples) != 0 {
		t.Fatalf("problems %+v, samples %d", run.problems, len(run.samples))
	}
}

func TestPeekBindings_NameEveryCredentialTheConstructorReads(t *testing.T) {
	sftp := batch.SourceRevision{Provider: batch.ProviderSFTP, SFTP: &batch.SFTPPolicy{
		KnownHostsBinding: "known-hosts", PrivateKeyBinding: "key", PrivateKeyPassBinding: "passphrase",
	}}
	got := peekBindings(sftp)
	want := []peekBinding{
		{path: "sftp.known_hosts_binding", name: "known-hosts"},
		{path: "sftp.private_key_binding", name: "key"},
		{path: "sftp.private_key_passphrase_binding", name: "passphrase", singleLine: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SFTP key bindings = %+v", got)
	}
	password := batch.SourceRevision{Provider: batch.ProviderSFTP, SFTP: &batch.SFTPPolicy{
		KnownHostsBinding: "known-hosts", PasswordBinding: "password",
	}}
	if got := peekBindings(password); len(got) != 2 || got[1] != (peekBinding{path: "sftp.password_binding", name: "password", singleLine: true}) {
		t.Fatalf("SFTP password bindings = %+v", got)
	}
}

func TestResolveSecret_SingleLineCredentials(t *testing.T) {
	reference := integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: "KEY"}
	for value, want := range map[string]string{"synthetic": "synthetic", "synthetic\n": "synthetic", "synthetic\r\n": "synthetic"} {
		got, err := resolveSecret(context.Background(), staticResolver{"KEY": value}, reference, true)
		if err != nil || string(got) != want {
			t.Fatalf("resolveSecret(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"\n", "two\nlines"} {
		if _, err := resolveSecret(context.Background(), staticResolver{"KEY": value}, reference, true); !errors.Is(err, integration.ErrSecretUnresolvable) {
			t.Fatalf("resolveSecret(%q) error = %v", value, err)
		}
	}
	multiLine := "synthetic-known-host-line-1\nsynthetic-known-host-line-2\n"
	if got, err := resolveSecret(context.Background(), staticResolver{"KEY": multiLine}, reference, false); err != nil || string(got) != multiLine {
		t.Fatalf("a multi-line binding was altered: %q, %v", got, err)
	}
}
