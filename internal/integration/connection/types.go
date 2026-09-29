// Package connection is the durable catalog of source and destination
// connections (.loom/38, Lane C-0).
//
// A connection is a named, editable declaration of one endpoint the engine
// listens on (a source: mllp, http, batch_s3, batch_sftp) or delivers to (a
// destination: https, fhir, kafka). Editing changes a mutable Draft guarded by
// an optimistic version. Compiling a draft runs the document's own constructor
// — mllp.NewSourceRevision, batch.NewSourceRevision, destination.NewRevision,
// or NewHTTPSourceRevision — and stores the exact bytes it produces as an
// immutable, content-addressed Revision. Those bytes are what `serve` mounts
// today; the catalog never hot-loads anything.
//
// Secrets stay references end to end. A spec names bindings (`*_binding`
// fields), the draft carries binding references ({provider, key, version}),
// and no field anywhere in this package can hold a secret value. A spec key
// that looks like a value is refused before it is persisted.
package connection

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// ReadRole authorizes every catalog read and engineRuntime. It is the
	// operator control plane's read role on purpose: the production operator
	// bundle already carries it (.loom/38 Decision 4).
	ReadRole = operator.ReadRole
	// WriteRole is required, beside ReadRole, for every catalog write.
	WriteRole = operator.DeploymentOperatorRole

	// MaxReasonBytes bounds the reason every write records.
	MaxReasonBytes = 1024
	// MaxSpecBytes bounds one draft's spec document.
	MaxSpecBytes = 64 << 10
	// MaxSecretBindings bounds one draft's binding list.
	MaxSecretBindings = 32
	// MaxConnections bounds one List call.
	MaxConnections = 500
	// MaxRevisionsPerConnection bounds one ListRevisions call.
	MaxRevisionsPerConnection = 500
	// maxNameBytes and maxDescriptionBytes bound the human-facing labels.
	maxNameBytes        = 256
	maxDescriptionBytes = 4096
)

// Direction says whether a connection is an ingress or an egress endpoint.
type Direction string

const (
	DirectionSource      Direction = "source"
	DirectionDestination Direction = "destination"
)

// Valid reports whether d is one of the two directions.
func (d Direction) Valid() bool {
	return d == DirectionSource || d == DirectionDestination
}

// Kind names the endpoint type and therefore the spec shape and the document
// a revision compiles to.
type Kind string

const (
	KindMLLP      Kind = "mllp"
	KindHTTP      Kind = "http"
	KindBatchS3   Kind = "batch_s3"
	KindBatchSFTP Kind = "batch_sftp"
	KindHTTPS     Kind = "https"
	KindFHIR      Kind = "fhir"
	KindKafka     Kind = "kafka"
)

// Kinds lists every kind in a stable order.
func Kinds() []Kind {
	return []Kind{KindMLLP, KindHTTP, KindBatchS3, KindBatchSFTP, KindHTTPS, KindFHIR, KindKafka}
}

// Direction returns the only direction a kind may have.
func (k Kind) Direction() (Direction, bool) {
	switch k {
	case KindMLLP, KindHTTP, KindBatchS3, KindBatchSFTP:
		return DirectionSource, true
	case KindHTTPS, KindFHIR, KindKafka:
		return DirectionDestination, true
	default:
		return "", false
	}
}

// Valid reports whether k is a supported kind.
func (k Kind) Valid() bool {
	_, ok := k.Direction()
	return ok
}

var (
	// ErrUnavailable means the connection catalog is not configured. The
	// GraphQL layer keeps it indistinguishable from a missing capability and
	// /api/auth/status reports the deployment fact honestly instead.
	ErrUnavailable = errors.New("connection catalog unavailable")
	// ErrUnauthenticated means no verified caller identity reached the service.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden means the verified caller lacks a required role.
	ErrForbidden = errors.New("connection catalog action forbidden")
	// ErrInvalidRequest means an identifier, label, reason, or version is
	// malformed. Spec content is never an ErrInvalidRequest on its own; it is
	// reported as Problems, except for secret material (see SpecError).
	ErrInvalidRequest = errors.New("invalid connection catalog request")
	// ErrNotFound hides inventory: another tenant's connection and a connection
	// that does not exist are indistinguishable.
	ErrNotFound = errors.New("connection not found")
	// ErrAlreadyExists means the connection ID is already registered for the
	// caller's tenant.
	ErrAlreadyExists = errors.New("connection already exists")
	// ErrVersionConflict means another writer advanced the draft.
	ErrVersionConflict = errors.New("connection version conflict")
	// ErrArchived means the connection is archived and accepts no change.
	ErrArchived = errors.New("connection is archived")
)

// Problem codes. A problem's code is stable; its message is guidance.
// Every code except CodeUnusedBinding blocks compile.
const (
	CodeRequired             = "REQUIRED"
	CodeOutOfRange           = "OUT_OF_RANGE"
	CodeInvalidEnum          = "INVALID_ENUM"
	CodeUnknownField         = "UNKNOWN_FIELD"
	CodeUnboundSecret        = "UNBOUND_SECRET"
	CodeUnusedBinding        = "UNUSED_BINDING"
	CodeSecretValueForbidden = "SECRET_VALUE_FORBIDDEN"
	CodeInvalidURL           = "INVALID_URL"
	CodeInvalidCIDR          = "INVALID_CIDR"
	CodeInvalidAddress       = "INVALID_ADDRESS"
	// CodeInvalidValue is a malformed value that no narrower code describes: an
	// identifier with whitespace, a non-canonical path, a malformed pin.
	CodeInvalidValue = "INVALID_VALUE"
	// CodeInvalidType is a JSON value of the wrong type for its field.
	CodeInvalidType = "INVALID_TYPE"
	// CodeInvalidJSON is a spec that is not one JSON object, or that repeats a key.
	CodeInvalidJSON = "INVALID_JSON"
	// CodeDuplicate is a repeated list member or binding name.
	CodeDuplicate = "DUPLICATE"
	// CodeConflict is two individually valid fields that cannot hold together.
	CodeConflict = "CONFLICT"
	// CodeForbidden is a field that the rest of the spec rules out.
	CodeForbidden = "FORBIDDEN"
	// CodeConstructorRejected means the document constructor refused a spec
	// the checker accepted. TestConnectionChecker_MirrorsConstructorBounds
	// exists so this is never produced; it is here so compile fails closed if
	// it ever is.
	CodeConstructorRejected = "CONSTRUCTOR_REJECTED"
)

// Problem is one field-level finding about a draft spec or its bindings.
// Path is JSON dot form relative to the spec (`timeouts.read_seconds`,
// `s3.bucket`, `clients.allowed_cidrs[0]`), or `secret_bindings[i].<field>`
// for a binding, or empty for the document as a whole.
type Problem struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Blocking reports whether the problem prevents compile. Only an unused
// binding is a warning.
func (p Problem) Blocking() bool {
	return p.Code != CodeUnusedBinding
}

// HasBlocking reports whether any problem blocks compile.
func HasBlocking(problems []Problem) bool {
	for _, problem := range problems {
		if problem.Blocking() {
			return true
		}
	}
	return false
}

// SpecError refuses a draft write whose spec would persist secret material.
// It wraps ErrInvalidRequest so callers that only care about the class can
// use errors.Is; the problems are derived from the caller's own input only.
type SpecError struct {
	Problems []Problem
}

func (e *SpecError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return "connection spec rejected"
	}
	parts := make([]string, 0, len(e.Problems))
	for _, problem := range e.Problems {
		parts = append(parts, problem.Code+" at "+problem.Path)
	}
	return "connection spec rejected: " + strings.Join(parts, "; ")
}

// Unwrap classifies a SpecError as an invalid request.
func (e *SpecError) Unwrap() error { return ErrInvalidRequest }

// Draft is the mutable, versioned declaration of one connection. Its ID is
// the artifact ID every compiled revision carries.
type Draft struct {
	TenantID    string
	ID          string
	Direction   Direction
	Kind        Kind
	Name        string
	Description string
	// Spec is the kind's JSON document as the author saved it. It may be
	// incomplete; compile reports what is missing. It never carries a secret
	// value (SpecError).
	Spec json.RawMessage
	// SecretBindings are references only: a name, a provider, a key, and an
	// optional version. Never material.
	SecretBindings []integration.SecretBinding
	Version        int64
	ArchivedAt     *time.Time
	Created        integration.AuditEnvelope
	Updated        integration.AuditEnvelope
}

// Archived reports whether the draft accepts no further change.
func (d Draft) Archived() bool { return d.ArchivedAt != nil }

// Revision is one immutable compile of a draft. Document is the exact byte
// sequence the constructor's revision marshals to — what `serve` mounts —
// and Digest is that document's own content address.
type Revision struct {
	TenantID            string
	ArtifactID          string
	RevisionID          string
	Number              int64
	Digest              string
	Direction           Direction
	Kind                Kind
	Document            []byte
	CompiledFromVersion int64
	Created             integration.AuditEnvelope
}

// Reference is one lifecycle definition revision that names a revision of
// this connection. DefinitionID and RevisionID identify the definition
// revision; Digest is the digest of THIS connection's revision that the
// definition names (as its source, or as one of its destinations), so a
// caller can tell which of the connection's revisions is referenced.
type Reference struct {
	DefinitionID string
	RevisionID   string
	Digest       string
	State        string
	Health       string
}

// RuntimeState reports whether this replica mounts a revision of the
// connection. Role is one of the RuntimeRole* constants when Mounted.
// RevisionID and Digest name the mounted revision; both are empty when not
// mounted and for the HTTP ingress, which is bound by definition id.
type RuntimeState struct {
	Mounted    bool
	Role       string
	Detail     string
	RevisionID string
	Digest     string
}

// Connection is the read projection of one draft: the draft itself, its
// latest revision, the lifecycle definitions that reference any of its
// revisions, and this replica's runtime state for it.
type Connection struct {
	Draft
	LatestRevision *Revision
	References     []Reference
	Runtime        RuntimeState
}

// CompileResult is the outcome of one compile. Revision is nil exactly when a
// blocking problem was found, and then nothing was written.
type CompileResult struct {
	Connection Connection
	Revision   *Revision
	Problems   []Problem
}

// CaptureMode distinguishes a batch peek from a stream capture (.loom/38,
// "Sample intake from connections"; Lane C-2 implements both).
type CaptureMode string

const (
	CaptureModePeek   CaptureMode = "peek"
	CaptureModeStream CaptureMode = "stream"
)

// CaptureStatus is the closed status vocabulary of a capture row.
type CaptureStatus string

const (
	CaptureStatusArmed     CaptureStatus = "armed"
	CaptureStatusComplete  CaptureStatus = "complete"
	CaptureStatusExpired   CaptureStatus = "expired"
	CaptureStatusCancelled CaptureStatus = "cancelled"
	CaptureStatusFailed    CaptureStatus = "failed"
)

// Terminal reports whether a status accepts no further transition. The
// integration_connection_captures trigger enforces the same rule.
func (s CaptureStatus) Terminal() bool {
	switch s {
	case CaptureStatusComplete, CaptureStatusExpired, CaptureStatusCancelled, CaptureStatusFailed:
		return true
	default:
		return false
	}
}

// MaxCaptureMessages bounds one capture or peek (the table enforces it too).
const MaxCaptureMessages = 100

// Capture is one audited peek or stream capture: the row
// integration_connection_captures holds. Lane C-0 ships the table and this
// type; Lane C-2 owns the behaviour that writes and advances it.
//
// A stream capture names the runtime SourceID it taps; a peek names the
// catalog connection and the digest of the revision whose bindings it used.
// Only Status, Captured, CompletedAt, and Version ever change after insert,
// and only under an expected-version update.
type Capture struct {
	TenantID             string
	ID                   string
	SessionID            string
	Mode                 CaptureMode
	SourceID             string
	ConnectionArtifactID string
	ConnectionDigest     string
	Status               CaptureStatus
	Captured             int
	MaxMessages          int
	Version              int64
	RequestedBy          integration.Principal
	Reason               string
	RequestedAt          time.Time
	ExpiresAt            time.Time
	CompletedAt          *time.Time
	// Problems says why the capture finished as it did (Lane C-2,
	// problems_json); empty while it is armed and when it completed cleanly.
	Problems []Problem
	// ObjectPath is the object a peek read (Lane C-2, object_path); empty for
	// a stream capture and for a peek that only listed.
	ObjectPath string
}
