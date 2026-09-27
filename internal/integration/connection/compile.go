package connection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// errConstructorRejected is compile failing closed: the document constructor
// refused a spec the checker passed.
var errConstructorRejected = errors.New("document constructor rejected the spec")

// RevisionIDForNumber is the catalog's revision ID scheme: "1", "2", … per
// connection, like Source Profile revisions.
func RevisionIDForNumber(number int64) string {
	return strconv.FormatInt(number, 10)
}

// BuildRevision is the pure half of compile. It checks the draft's spec and
// bindings and, when no problem blocks, runs the kind's own constructor and
// returns the exact bytes the constructor's revision marshals to. It writes
// nothing and reads nothing but its arguments.
//
// The returned bytes are re-read with the kind's existing Decode function
// before they are returned, so a revision the catalog stores is, by
// construction, one `serve` can mount.
func BuildRevision(draft Draft, number int64, created integration.AuditEnvelope) (*Revision, []Problem) {
	spec, problems := decodeAndCheck(draft.Kind, draft.Spec, draft.SecretBindings)
	if HasBlocking(problems) {
		return nil, problems
	}
	// decodeAndCheck reported an unknown kind or an undecodable spec as a
	// blocking problem, so both are known good here.
	direction, _ := draft.Kind.Direction()
	revisionID := RevisionIDForNumber(number)
	document, digest, err := constructDocument(draft.Kind, spec, draft.ID, revisionID)
	if err != nil || number < 1 {
		return nil, append(problems, Problem{
			Code: CodeConstructorRejected, Path: "",
			Message: "the " + string(draft.Kind) + " document constructor refused this spec; no revision was written",
		})
	}
	return &Revision{
		TenantID:            draft.TenantID,
		ArtifactID:          draft.ID,
		RevisionID:          revisionID,
		Number:              number,
		Digest:              digest,
		Direction:           direction,
		Kind:                draft.Kind,
		Document:            document,
		CompiledFromVersion: draft.Version,
		Created:             created,
	}, problems
}

// constructDocument runs the constructor for one decoded spec and returns the
// document bytes and digest, verified by a round trip through the kind's
// Decode function.
func constructDocument(kind Kind, spec kindSpec, artifactID, revisionID string) ([]byte, string, error) {
	var (
		revision any
		digest   string
	)
	switch typed := spec.(type) {
	case *MLLPSpec:
		built, err := mllp.NewSourceRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *BatchS3Spec:
		built, err := batch.NewSourceRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *BatchSFTPSpec:
		built, err := batch.NewSourceRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *HTTPSpec:
		built, err := NewHTTPSourceRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *HTTPSSpec:
		built, err := destination.NewRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *FHIRSpec:
		built, err := destination.NewRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	case *KafkaSpec:
		built, err := destination.NewRevision(typed.input(artifactID, revisionID))
		if err != nil {
			return nil, "", fmt.Errorf("%w: %w", errConstructorRejected, err)
		}
		revision, digest = built, built.Digest
	default:
		return nil, "", errConstructorRejected
	}
	document, err := json.Marshal(revision)
	if err != nil {
		return nil, "", fmt.Errorf("%w: marshal document", errConstructorRejected)
	}
	decoded, err := DocumentDigest(kind, document)
	if err != nil || decoded != digest {
		return nil, "", fmt.Errorf("%w: document does not round-trip", errConstructorRejected)
	}
	return document, digest, nil
}

// DocumentDigest decodes one compiled document with the kind's existing
// Decode function — the same one `serve` mounts it with — and returns the
// digest that decoder verified.
func DocumentDigest(kind Kind, document []byte) (string, error) {
	reader := bytes.NewReader(document)
	switch kind {
	case KindMLLP:
		revision, err := mllp.DecodeSourceRevision(reader)
		if err != nil {
			return "", err
		}
		return revision.Digest, nil
	case KindBatchS3, KindBatchSFTP:
		revision, err := batch.DecodeSourceRevision(reader)
		if err != nil {
			return "", err
		}
		if (kind == KindBatchS3) != (revision.Provider == batch.ProviderS3) {
			return "", batch.ErrInvalidSourceRevision
		}
		return revision.Digest, nil
	case KindHTTP:
		revision, err := DecodeHTTPSourceRevision(reader)
		if err != nil {
			return "", err
		}
		return revision.Digest, nil
	case KindHTTPS, KindFHIR, KindKafka:
		revision, err := destination.DecodeRevision(reader)
		if err != nil {
			return "", err
		}
		if string(revision.Transport) != string(kind) {
			return "", destination.ErrInvalidRevision
		}
		return revision.Digest, nil
	default:
		return "", fmt.Errorf("connection kind %q is not supported", kind)
	}
}
