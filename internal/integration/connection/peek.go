package connection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The batch peek (.loom/38, "Sample intake from connections"). A peek reads a
// compiled batch connection's source the way an operator would look at it:
// it lists the input prefix or directory and, when asked, reads the first few
// messages of one object into a session. It takes no lease, writes no
// checkpoint, archives nothing, and deletes nothing — the only provider calls
// it makes are List, OpenAt, and Close — so the object is still there, and
// still unclaimed, for the runner that will ingest it.

// Peek bounds.
const (
	// DefaultPeekObjects is maxObjects when a request leaves it out.
	DefaultPeekObjects = 10
	// MaxPeekObjects bounds the listing a peek returns.
	MaxPeekObjects = 50
	// DefaultPeekMessages is maxMessages when a request leaves it out.
	DefaultPeekMessages = 5
	// MaxPeekMessages bounds the messages one peek reads.
	MaxPeekMessages = 50
	// peekLookupObjects bounds the listing a peek searches for objectPath: the
	// batch source's own per-poll bound.
	peekLookupObjects = batchMaxFilesPerPoll
	// peekTimeout bounds a peek's read, counted from before its audit row is
	// inserted; the row's expires_at is the same instant.
	peekTimeout = 60 * time.Second
	// peekFinishTimeout bounds writing the peek's outcome to its audit row.
	peekFinishTimeout = 5 * time.Second
	// peekOrphanGrace is how long after its expires_at a peek row still armed
	// is taken for orphaned — its replica died between insert and finish — and
	// expired (ExpireCaptures). A live peek has finished by peekFinishTimeout
	// after expires_at; the minute is margin for clock skew between replicas.
	peekOrphanGrace = peekFinishTimeout + time.Minute
	// maxObjectPathBytes bounds the objectPath a caller names.
	maxObjectPathBytes = 1024
)

// ErrPeekUnsupported means the connection is not a batch source with a
// compiled revision, so there is nothing a peek could read.
var ErrPeekUnsupported = errors.New("peek requires a compiled batch source connection")

// BatchProviderFactory builds the provider a peek reads through, from the
// revision's exact document and the resolved material of each binding it
// names, keyed by binding name. The material is the caller's to zero.
type BatchProviderFactory func(ctx context.Context, source batch.SourceRevision, material map[string][]byte) (batch.Provider, error)

// PeekRequest reads one batch connection's source into a session.
type PeekRequest struct {
	ConnectionID string
	SessionID    string
	// ObjectPath selects the object to read; empty lists and reads nothing.
	ObjectPath string
	// MaxObjects is 0 for DefaultPeekObjects, otherwise 1..MaxPeekObjects.
	MaxObjects int
	// MaxMessages is 0 for DefaultPeekMessages, otherwise 1..MaxPeekMessages.
	MaxMessages int
	Reason      string
}

// PeekObject is one listed object. ModifiedAt is the provider's advisory
// timestamp, never a trust input.
type PeekObject struct {
	Path       string
	Size       int64
	Version    string
	ModifiedAt time.Time
}

// PeekResult is what one peek saw and wrote. Capture is its audit row;
// Problems says what stopped it, and is empty when it did all it was asked.
type PeekResult struct {
	Objects  []PeekObject
	Samples  []session.Sample
	Capture  Capture
	Problems []Problem
}

// PeekBatch lists a compiled batch connection's source and, with ObjectPath,
// adds the first MaxMessages messages of that object to an active session
// under the capture redactor. The source is the latest revision's exact
// document — the bytes `serve` would mount — and its credentials are the
// draft's bindings resolved on this replica. A binding that does not resolve
// is a SECRET_UNRESOLVABLE problem, and then nothing is contacted.
func (s *Service) PeekBatch(ctx context.Context, request PeekRequest) (PeekResult, error) {
	security, intake, err := s.authorizeIntake(ctx)
	if err != nil {
		return PeekResult{}, err
	}
	reason, err := intakeReason(request.Reason)
	if err != nil {
		return PeekResult{}, err
	}
	maxObjects, objectsOK := boundedDefault(request.MaxObjects, DefaultPeekObjects, MaxPeekObjects)
	maxMessages, messagesOK := boundedDefault(request.MaxMessages, DefaultPeekMessages, MaxPeekMessages)
	if !objectsOK || !messagesOK || !validConnectionID(request.ConnectionID) ||
		!validIdentity(request.SessionID) || (request.ObjectPath != "" && !validObjectPath(request.ObjectPath)) {
		return PeekResult{}, ErrInvalidRequest
	}
	if err := intake.activeSession(ctx, request.SessionID); err != nil {
		return PeekResult{}, err
	}
	draft, revision, source, err := s.peekableSource(ctx, security.TenantID, request.ConnectionID)
	if err != nil {
		return PeekResult{}, err
	}

	// The audit row is written before anything is resolved or contacted, so an
	// attempt that fails part-way is on record as failed rather than absent.
	// The read's deadline is fixed first, so the read ends by the row's
	// expires_at however long the insert takes.
	peekCtx, cancel := context.WithDeadline(ctx, time.Now().Add(peekTimeout))
	defer cancel()
	requestedAt := s.store.Now()
	audit, err := s.store.InsertCapture(ctx, Capture{
		TenantID: security.TenantID, ID: uuid.NewString(), SessionID: request.SessionID,
		Mode: CaptureModePeek, SourceID: source.SourceID, ConnectionArtifactID: draft.ID,
		ConnectionDigest: revision.Digest, ObjectPath: request.ObjectPath, Status: CaptureStatusArmed,
		MaxMessages: maxMessages, Version: 1, RequestedBy: auditPrincipal(security), Reason: reason,
		RequestedAt: requestedAt, ExpiresAt: requestedAt.Add(peekTimeout),
	})
	if err != nil {
		return PeekResult{}, err
	}
	run := peekRun{
		intake: intake, source: source, audit: audit, request: request,
		maxObjects: maxObjects, maxMessages: maxMessages,
	}
	run.read(peekCtx, draft.SecretBindings)

	status := CaptureStatusComplete
	if len(run.problems) > 0 {
		status = CaptureStatusFailed
	}
	captured := len(run.samples)
	// The finish is detached from the caller's cancellation, so a client that
	// goes away mid-peek still leaves the row saying what the peek wrote. The
	// row is this peek's alone until peekOrphanGrace after expires_at (a peek
	// cannot be cancelled, and ExpireCaptures waits that long), so a finish
	// that does not apply is a fault, not a race.
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), peekFinishTimeout)
	defer cancelFinish()
	finished, applied, err := s.store.FinishCapture(finishCtx, security.TenantID, audit.ID, audit.Version, captureFinish{
		mode: CaptureModePeek, status: status, captured: &captured, problems: run.problems, at: s.store.Now(),
	})
	if err != nil {
		return PeekResult{}, err
	}
	if !applied {
		return PeekResult{}, fmt.Errorf("%w: %s is %s at version %d", ErrPeekUnrecorded, audit.ID, finished.Status, finished.Version)
	}
	intake.observer.captured(CaptureModePeek, captured)
	problems := run.problems
	if problems == nil {
		problems = []Problem{}
	}
	return PeekResult{Objects: run.objects, Samples: run.samples, Capture: finished, Problems: problems}, nil
}

// peekableSource loads the connection and decodes its latest revision's exact
// bytes, refusing anything a peek cannot read.
func (s *Service) peekableSource(ctx context.Context, tenantID, connectionID string) (Draft, *Revision, batch.SourceRevision, error) {
	draft, err := s.store.GetDraft(ctx, tenantID, connectionID)
	if err != nil {
		return Draft{}, nil, batch.SourceRevision{}, err
	}
	if draft.Archived() {
		return Draft{}, nil, batch.SourceRevision{}, ErrArchived
	}
	if draft.Kind != KindBatchS3 && draft.Kind != KindBatchSFTP {
		return Draft{}, nil, batch.SourceRevision{}, ErrPeekUnsupported
	}
	revision, err := s.store.LatestRevision(ctx, tenantID, draft.ID)
	if err != nil {
		return Draft{}, nil, batch.SourceRevision{}, err
	}
	if revision == nil {
		return Draft{}, nil, batch.SourceRevision{}, ErrPeekUnsupported
	}
	source, err := batch.DecodeSourceRevision(bytes.NewReader(revision.Document))
	if err != nil || source.Digest != revision.Digest || (draft.Kind == KindBatchS3) != (source.Provider == batch.ProviderS3) {
		return Draft{}, nil, batch.SourceRevision{}, ErrPeekUnsupported
	}
	return draft, revision, source, nil
}

// peekRun is one peek between its audit row's insert and its finish.
type peekRun struct {
	intake      *sampleIntake
	source      batch.SourceRevision
	audit       Capture
	request     PeekRequest
	maxObjects  int
	maxMessages int

	objects  []PeekObject
	samples  []session.Sample
	problems []Problem
}

func (p *peekRun) fail(code, path, message string) {
	p.problems = append(p.problems, Problem{Code: code, Path: path, Message: message})
}

// read resolves, lists, and — with an object path — reads. It records every
// outcome on p and returns nothing, because every outcome is the caller's to
// write to the audit row.
func (p *peekRun) read(ctx context.Context, bindings []integration.SecretBinding) {
	p.objects = []PeekObject{}
	p.samples = []session.Sample{}
	material, ok := p.resolve(ctx, bindings)
	defer zeroMaterial(material)
	if !ok {
		return
	}
	provider, err := p.intake.providers(ctx, p.source, material)
	if err != nil {
		p.fail(CodeSourceUnavailable, "", "the "+string(p.source.Provider)+" source could not be reached with this revision's settings and credentials")
		return
	}
	defer func() { _ = provider.Close() }()

	limit := p.maxObjects
	if p.request.ObjectPath != "" {
		limit = peekLookupObjects
	}
	listed, err := provider.List(ctx, limit)
	if err != nil {
		p.fail(CodeSourceUnavailable, "", "the "+string(p.source.Provider)+" source could not be listed")
		return
	}
	for index, object := range listed {
		if index == p.maxObjects {
			break
		}
		p.objects = append(p.objects, PeekObject{
			Path: object.Path, Size: object.Size, Version: object.Version, ModifiedAt: object.RemoteModifiedAtAdvisory,
		})
	}
	if p.request.ObjectPath == "" {
		return
	}
	for _, object := range listed {
		if object.Path == p.request.ObjectPath {
			p.readObject(ctx, provider, object)
			return
		}
	}
	p.fail(CodeObjectNotFound, "objectPath", "no object with this path is listed under the source's input location")
}

// readObject streams one object through the bounded batch reader and adds its
// first maxMessages messages to the session. It stops early: nothing past the
// last message it needs is read, and no digest, checkpoint, or archive is made.
func (p *peekRun) readObject(ctx context.Context, provider batch.Provider, object batch.Object) {
	stream, err := provider.OpenAt(ctx, object, 0)
	if err != nil {
		p.fail(CodeSourceUnavailable, "objectPath", "the object could not be opened")
		return
	}
	defer func() { _ = stream.Close() }()
	reader, err := batch.NewMessageReader(stream, 0, p.source.MaxMessageBytes)
	if err != nil {
		p.fail(CodeMessageUnreadable, "objectPath", "the object could not be read as HL7v2")
		return
	}
	for len(p.samples) < p.maxMessages {
		message, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			p.fail(CodeMessageUnreadable, "objectPath",
				fmt.Sprintf("message %d of the object is not readable HL7v2 within the source's max_message_bytes", len(p.samples)+1))
			return
		}
		number := len(p.samples) + 1
		// The sample names its audit row and nothing else; which connection,
		// revision, and object it came from is recorded on that row.
		sample, err := p.intake.sessions.AddSample(ctx, p.request.SessionID, session.AddSampleRequest{
			Name:      fmt.Sprintf("peek %s #%d", p.audit.ID, number),
			Format:    events.FormatHL7v2,
			Source:    "peek:" + p.audit.ID,
			Raw:       string(message.Payload),
			PHIPolicy: session.PHIPolicyRedact,
			Redaction: session.SampleRedactionCapture,
		})
		if err != nil {
			p.fail(CodeSampleWriteFailed, "sessionId", fmt.Sprintf("message %d could not be written to the session", number))
			return
		}
		p.samples = append(p.samples, *sample)
	}
}

// peekBinding is one binding a batch revision names, at the spec path that
// names it.
type peekBinding struct {
	path       string
	name       string
	singleLine bool
}

// peekBindings lists the bindings the provider constructor will need.
func peekBindings(source batch.SourceRevision) []peekBinding {
	switch source.Provider {
	case batch.ProviderS3:
		return []peekBinding{
			{path: "s3.access_key_binding", name: source.S3.AccessKeyBinding, singleLine: true},
			{path: "s3.secret_access_key_binding", name: source.S3.SecretAccessKeyBinding, singleLine: true},
		}
	case batch.ProviderSFTP:
		bindings := []peekBinding{{path: "sftp.known_hosts_binding", name: source.SFTP.KnownHostsBinding}}
		if source.SFTP.PasswordBinding != "" {
			return append(bindings, peekBinding{path: "sftp.password_binding", name: source.SFTP.PasswordBinding, singleLine: true})
		}
		bindings = append(bindings, peekBinding{path: "sftp.private_key_binding", name: source.SFTP.PrivateKeyBinding})
		if source.SFTP.PrivateKeyPassBinding != "" {
			bindings = append(bindings, peekBinding{
				path: "sftp.private_key_passphrase_binding", name: source.SFTP.PrivateKeyPassBinding, singleLine: true,
			})
		}
		return bindings
	default:
		return nil
	}
}

// resolve turns every binding the revision names into material through the
// injected resolver. Every failure is SECRET_UNRESOLVABLE, reported per
// binding, and a peek with any is contacted with nothing.
func (p *peekRun) resolve(ctx context.Context, bindings []integration.SecretBinding) (map[string][]byte, bool) {
	declared := make(map[string]integration.SecretReference, len(bindings))
	for _, binding := range bindings {
		declared[binding.Name] = binding.Reference
	}
	material := make(map[string][]byte)
	for _, needed := range peekBindings(p.source) {
		if _, done := material[needed.name]; done {
			continue
		}
		reference, ok := declared[needed.name]
		if !ok {
			p.fail(CodeSecretUnresolvable, needed.path,
				fmt.Sprintf("binding %q is not declared on this connection's draft", needed.name))
			continue
		}
		resolved, err := resolveSecret(ctx, p.intake.secrets, reference, needed.singleLine)
		if err != nil {
			p.fail(CodeSecretUnresolvable, needed.path,
				fmt.Sprintf("binding %q (%s) did not resolve on this replica", needed.name, reference.Provider))
			continue
		}
		material[needed.name] = resolved
	}
	return material, len(p.problems) == 0
}

// resolveSecret resolves one reference and, for a single-line credential,
// drops the one trailing newline a secret file conventionally ends with, as
// the batch runtime's own loader does; a credential with another line break is
// refused rather than sent.
func resolveSecret(ctx context.Context, resolver integration.SecretResolver, reference integration.SecretReference, singleLine bool) ([]byte, error) {
	if resolver == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	material, err := resolver.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	if len(material) == 0 {
		return nil, integration.ErrSecretUnresolvable
	}
	if singleLine {
		material = bytes.TrimSuffix(material, []byte("\n"))
		material = bytes.TrimSuffix(material, []byte("\r"))
		if len(material) == 0 || bytes.ContainsAny(material, "\r\n") {
			return nil, integration.ErrSecretUnresolvable
		}
	}
	return material, nil
}

func zeroMaterial(material map[string][]byte) {
	for _, value := range material {
		for index := range value {
			value[index] = 0
		}
	}
}

// openBatchProvider is the default BatchProviderFactory: the batch package's
// own constructors, fed exactly as cmd/fi-fhir's batch runtime feeds them.
func openBatchProvider(_ context.Context, source batch.SourceRevision, material map[string][]byte) (batch.Provider, error) {
	switch source.Provider {
	case batch.ProviderS3:
		provider, err := batch.NewS3Provider(source, batch.S3Secrets{
			AccessKeyID:     string(material[source.S3.AccessKeyBinding]),
			SecretAccessKey: string(material[source.S3.SecretAccessKeyBinding]),
		})
		if err != nil {
			return nil, err
		}
		return provider, nil
	case batch.ProviderSFTP:
		// The SFTP provider reads known_hosts from a path when it is built and
		// never again, so the resolved lines live in a private temporary file
		// only for the constructor's duration.
		knownHosts, err := os.CreateTemp("", "fi-fhir-peek-known-hosts-*")
		if err != nil {
			return nil, batch.ErrProviderUnavailable
		}
		defer func() { _ = os.Remove(knownHosts.Name()) }()
		_, writeErr := knownHosts.Write(material[source.SFTP.KnownHostsBinding])
		closeErr := knownHosts.Close()
		if writeErr != nil || closeErr != nil {
			return nil, batch.ErrProviderUnavailable
		}
		provider, err := batch.NewSFTPProvider(source, batch.SFTPSecrets{
			KnownHostsPath:       knownHosts.Name(),
			Password:             string(material[source.SFTP.PasswordBinding]),
			PrivateKey:           material[source.SFTP.PrivateKeyBinding],
			PrivateKeyPassphrase: material[source.SFTP.PrivateKeyPassBinding],
		})
		if err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, batch.ErrProviderUnavailable
	}
}

// validObjectPath accepts a provider object path a caller copied from a
// listing: bounded, canonical, and free of control characters.
func validObjectPath(value string) bool {
	if value == "" || len(value) > maxObjectPathBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
