package authoring

import (
	"context"
	"errors"
	"io"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type fakeProvider struct {
	closed *atomic.Int32
	list   func(context.Context) error
}

func (p fakeProvider) Type() batch.ProviderType { return batch.ProviderSFTP }
func (p fakeProvider) List(ctx context.Context, _ int) ([]batch.Object, error) {
	if p.list != nil {
		return nil, p.list(ctx)
	}
	return nil, nil
}
func (p fakeProvider) OpenAt(context.Context, batch.Object, int64) (io.ReadCloser, error) {
	return nil, errors.New("unused")
}
func (p fakeProvider) Digest(context.Context, batch.Object) (string, error) { return "", nil }
func (p fakeProvider) PrepareArchive(context.Context, batch.Object, string) (string, error) {
	return "", nil
}
func (p fakeProvider) DeleteSource(context.Context, batch.Object, string) error { return nil }
func (p fakeProvider) Close() error {
	p.closed.Add(1)
	return nil
}

func testBatchSource(t *testing.T) batch.SourceRevision {
	t.Helper()
	source, err := batch.NewSourceRevision(batch.SourceRevisionInput{
		ArtifactID: "sftp-test", RevisionID: "1", SourceID: "sftp-test", Provider: batch.ProviderSFTP,
		PollSeconds: 15, LeaseSeconds: 120, ProcessSeconds: 60, MaxFilesPerPoll: 10, MaxMessageBytes: 1 << 20,
		SFTP: &batch.SFTPPolicy{
			Host: "sftp.example.test", Port: 22, Username: "fhir", InputDirectory: "/in", ArchiveDirectory: "/archive",
			KnownHostsBinding: "sftp-known-hosts", PasswordBinding: "sftp-password",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func batchRevisionFor(source batch.SourceRevision) integration.IntegrationDefinitionRevision {
	return integration.IntegrationDefinitionRevision{
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
	}
}

// Disconfirming search for .loom/42's kill-test: hosting the batch validator
// in the API process must not leak the probe goroutine or its provider when
// the catalog's deadline fires first. The probe finishes on its own (the SFTP
// dial is bounded at 10 s and List honours the context), closes its
// provider, and exits.
func TestBatchValidator_DeadlineLeaksNoGoroutineOrProvider(t *testing.T) {
	source := testBatchSource(t)
	release := make(chan struct{})
	var closed atomic.Int32
	build := func(batch.SourceRevision, BatchSecrets) (batch.Provider, error) {
		<-release // a dial that outlives the catalog's deadline
		return fakeProvider{closed: &closed}, nil
	}
	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := BatchValidator(source, BatchSecrets{}, build, nil)(ctx, batchRevisionFor(source))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("validator error = %v, want the deadline", err)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline && closed.Load() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines %d (baseline %d), provider closes %d: the probe leaked", runtime.NumGoroutine(), baseline, closed.Load())
}

func TestBatchValidator_RecordsCodesAndRefusesAnotherSource(t *testing.T) {
	source := testBatchSource(t)
	var closed atomic.Int32
	ok := func(batch.SourceRevision, BatchSecrets) (batch.Provider, error) {
		return fakeProvider{closed: &closed}, nil
	}
	outcome, err := BatchValidator(source, BatchSecrets{}, ok, nil)(context.Background(), batchRevisionFor(source))
	if err != nil || !outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{CodeReachable, CodeHostKeyVerified, CodeAuthOK, CodeInputListed}) {
		t.Fatalf("outcome = %#v, %v", outcome, err)
	}
	other := batchRevisionFor(source)
	other.Source.SourceID = "another-source"
	outcome, err = BatchValidator(source, BatchSecrets{}, ok, nil)(context.Background(), other)
	if err != nil || outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{CodeSourceMismatch}) {
		t.Fatalf("mismatch outcome = %#v, %v", outcome, err)
	}
	failing := func(batch.SourceRevision, BatchSecrets) (batch.Provider, error) {
		return nil, errors.New("dial refused")
	}
	outcome, _ = BatchValidator(source, BatchSecrets{}, failing, nil)(context.Background(), batchRevisionFor(source))
	if outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{CodeConnectFailed}) {
		t.Fatalf("connect-failed outcome = %#v", outcome)
	}
}

func TestStaticValidator_ClaimsOnlyWhatItChecked(t *testing.T) {
	revision := testDefinition(t, "static", "mllp-static")
	revision.SecretBindings = []integration.SecretBinding{{
		Name: "tls-cert", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "connections/tls-cert"},
	}}
	mounted := func(context.Context, integration.ArtifactRevisionRef) (bool, error) { return true, nil }
	names := func(context.Context, integration.ArtifactRevisionRef) ([]string, bool, error) {
		return []string{"tls-cert"}, true, nil
	}
	for _, test := range []struct {
		name   string
		checks StaticChecks
		passed bool
		codes  []string
	}{
		{name: "no hooks", checks: StaticChecks{}, codes: []string{CodeStatic, CodeSourceNotMounted, CodeBindingsNotChecked}},
		{name: "mounted, no resolver", checks: StaticChecks{SourceMounted: mounted, SourceBindingNames: names}, passed: true,
			codes: []string{CodeStatic, CodeSourceMounted, CodeBindingsNotChecked}},
		{name: "mounted, resolved", checks: StaticChecks{SourceMounted: mounted, SourceBindingNames: names,
			ResolveSecret: func(context.Context, integration.SecretReference) error { return nil }}, passed: true,
			codes: []string{CodeStatic, CodeSourceMounted, CodeBindingsResolved}},
		{name: "mounted, unresolvable", checks: StaticChecks{SourceMounted: mounted, SourceBindingNames: names,
			ResolveSecret: func(context.Context, integration.SecretReference) error { return errors.New("missing") }},
			codes: []string{CodeStatic, CodeSourceMounted, CodeBindingUnresolvable}},
		{name: "source not in catalog", checks: StaticChecks{SourceMounted: mounted,
			SourceBindingNames: func(context.Context, integration.ArtifactRevisionRef) ([]string, bool, error) { return nil, false, nil }},
			passed: true, codes: []string{CodeStatic, CodeSourceMounted, CodeBindingsNotChecked}},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome, err := StaticValidator(test.checks)(context.Background(), revision)
			if err != nil || outcome.Passed != test.passed || !reflect.DeepEqual(outcome.Codes, test.codes) {
				t.Fatalf("outcome = %#v, %v; want passed=%v %v", outcome, err, test.passed, test.codes)
			}
		})
	}
}

func TestValidators_DispatchOnModeAndRefuseAMissingOne(t *testing.T) {
	validators := Validators{Skip: SkipValidator()}
	revision := testDefinition(t, "dispatch", "mllp-dispatch")
	if _, err := validators.Func()(context.Background(), revision); !errors.Is(err, ErrModeMissing) {
		t.Fatalf("no mode = %v", err)
	}
	if _, err := validators.Func()(WithMode(context.Background(), ModeReal), revision); !errors.Is(err, ErrModeUnavailable) {
		t.Fatalf("real without a batch validator = %v", err)
	}
	outcome, err := validators.Func()(WithMode(context.Background(), ModeSkip), revision)
	if err != nil || !outcome.Passed || !reflect.DeepEqual(outcome.Codes, []string{CodeSkipped}) {
		t.Fatalf("skip = %#v, %v", outcome, err)
	}
}
