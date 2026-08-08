package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	ErrProviderUnavailable = errors.New("batch provider unavailable")
	ErrObjectChanged       = errors.New("batch object changed")
	ErrArchiveCollision    = errors.New("batch archive collision")
	ErrInvalidObject       = errors.New("invalid batch object")
)

// Object is one exact provider object version. Path is used only in memory and
// is never persisted by the checkpoint store.
//
// Trust boundary: Version and ETag are provider-owned exact-version identifiers
// re-verified on every read. RemoteModifiedAtAdvisory is remote-controlled
// metadata. It is retained for operator diagnostics and as a change-detection
// input to the SFTP synthetic version, and it is excluded from every trust,
// provenance, and audit decision. Receipt provenance uses the server-owned
// custody timestamp and the streaming content digest instead.
type Object struct {
	Provider                 ProviderType
	Path                     string
	Version                  string
	ETag                     string
	Size                     int64
	RemoteModifiedAtAdvisory time.Time
}

// Provider supplies exact-version streaming and archive operations.
type Provider interface {
	Type() ProviderType
	List(context.Context, int) ([]Object, error)
	OpenAt(context.Context, Object, int64) (io.ReadCloser, error)
	Digest(context.Context, Object) (string, error)
	PrepareArchive(context.Context, Object, string) (string, error)
	DeleteSource(context.Context, Object, string) error
	Close() error
}

func (o Object) validate() error {
	if (o.Provider != ProviderS3 && o.Provider != ProviderSFTP) ||
		o.Path == "" || strings.TrimSpace(o.Path) != o.Path || strings.ContainsAny(o.Path, "\r\n\x00") ||
		o.Version == "" || len(o.Version) > 2048 || strings.TrimSpace(o.Version) != o.Version ||
		o.Size <= 0 || o.RemoteModifiedAtAdvisory.IsZero() {
		return ErrInvalidObject
	}
	// S3 pins the entity tag alongside the version ID; SFTP has no equivalent
	// server-issued entity tag and must not fabricate one.
	if o.Provider == ProviderS3 && !validETag(o.ETag) {
		return ErrInvalidObject
	}
	if o.Provider == ProviderSFTP && o.ETag != "" {
		return ErrInvalidObject
	}
	return nil
}

func validETag(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < '!' || character > '~' || character == '"' {
			return false
		}
	}
	return true
}

func objectID(source SourceRevision, object Object) (string, error) {
	if source.Validate() != nil || object.validate() != nil || object.Provider != source.Provider {
		return "", ErrInvalidObject
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("fi-fhir/batch-object/v1\x00"))
	for _, value := range []string{source.Digest, string(object.Provider), object.Path, object.Version} {
		_, _ = fmt.Fprintf(hasher, "%d:", len(value))
		_, _ = hasher.Write([]byte(value))
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func digestReader(reader io.Reader) (string, error) {
	if reader == nil {
		return "", ErrProviderUnavailable
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}
