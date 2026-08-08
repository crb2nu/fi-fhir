package batch

import (
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
)

// ErrDigestState rejects unusable streaming-digest continuation state.
var ErrDigestState = errors.New("batch streaming digest state unusable")

const maxDigestStateBytes = 256

// streamDigest is a SHA-256 digest over the exact object bytes admitted so far.
//
// Receipt provenance must be grounded in bytes fi-fhir actually admitted, not in
// a later re-read of whatever the remote side happens to serve. Because durable
// admission checkpoints mid-object and can resume in a different process, the
// intermediate hash state is marshaled at every checkpoint and restored on
// resume. The result therefore covers the whole object exactly once, in order,
// across restarts.
type streamDigest struct {
	hasher hash.Hash
}

// newStreamDigest resumes an in-progress digest, or starts a fresh one when the
// persisted state is empty.
func newStreamDigest(state string) (*streamDigest, error) {
	hasher := sha256.New()
	if state == "" {
		return &streamDigest{hasher: hasher}, nil
	}
	if len(state) > maxDigestStateBytes {
		return nil, ErrDigestState
	}
	raw, err := base64.StdEncoding.DecodeString(state)
	if err != nil {
		return nil, ErrDigestState
	}
	unmarshaler, ok := hasher.(encoding.BinaryUnmarshaler)
	if !ok {
		return nil, ErrDigestState
	}
	if err := unmarshaler.UnmarshalBinary(raw); err != nil {
		return nil, ErrDigestState
	}
	return &streamDigest{hasher: hasher}, nil
}

func (d *streamDigest) write(raw []byte) error {
	if d == nil || d.hasher == nil {
		return ErrDigestState
	}
	if _, err := d.hasher.Write(raw); err != nil {
		return ErrDigestState
	}
	return nil
}

// state marshals the continuation point so a later poll resumes this exact
// hash rather than starting over.
func (d *streamDigest) state() (string, error) {
	if d == nil || d.hasher == nil {
		return "", ErrDigestState
	}
	marshaler, ok := d.hasher.(encoding.BinaryMarshaler)
	if !ok {
		return "", ErrDigestState
	}
	raw, err := marshaler.MarshalBinary()
	if err != nil || len(raw) == 0 {
		return "", ErrDigestState
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	if len(encoded) > maxDigestStateBytes {
		return "", ErrDigestState
	}
	return encoded, nil
}

func (d *streamDigest) sum() (string, error) {
	if d == nil || d.hasher == nil {
		return "", ErrDigestState
	}
	return "sha256:" + hex.EncodeToString(d.hasher.Sum(nil)), nil
}
