package integration

import (
	"context"
	"errors"
)

// MaxSecretBytes bounds any single resolved secret. Secret material is credential
// sized; anything larger is a misconfiguration or an attempt to stream a file
// through the credential path.
const MaxSecretBytes = 1 << 16

var (
	// ErrSecretResolverUnavailable means no resolver is configured for the
	// deployment. It is never a signal to proceed without the credential.
	ErrSecretResolverUnavailable = errors.New("secret resolver unavailable")
	// ErrSecretUnresolvable is the single catalog-safe failure for every
	// resolution problem: unknown provider, absent key, unreadable material,
	// oversized material, or a version that does not exist. It deliberately does
	// not distinguish them, so a caller cannot probe the secret inventory.
	ErrSecretUnresolvable = errors.New("secret reference is unresolvable")
)

// SecretResolver turns a SecretReference into its material.
//
// The Slice 1.0 contract carries references, never values: a revision names a
// binding, the binding names a provider and key, and the value stays out of
// band. Until this interface existed there was no resolver of any kind — every
// runtime read a fixed process environment variable or file instead, so the
// reference was in practice a name-presence check.
//
// Implementations must:
//   - live outside internal/integration/*, so no domain package can hold secret
//     material in a struct that is later marshaled into a record;
//   - bound every read by MaxSecretBytes;
//   - return only ErrSecretUnresolvable to callers, so failures cannot be used
//     to enumerate which references exist.
//
// Resolved material must never enter a struct that is JSON-marshaled into a
// durable record, a log line, a metric label, or a broker payload.
type SecretResolver interface {
	Resolve(ctx context.Context, reference SecretReference) ([]byte, error)
}

// ValidateSecretReference reports whether a reference is well formed enough to
// be resolvable. It inspects only the reference, never the material.
func ValidateSecretReference(reference SecretReference) error {
	switch reference.Provider {
	case SecretProviderEnvironment, SecretProviderFile, SecretProviderVault,
		SecretProviderAWSSSM, SecretProviderKubernetes:
	default:
		return ErrSecretUnresolvable
	}
	if !validSecretToken(reference.Key) {
		return ErrSecretUnresolvable
	}
	if reference.Version != "" && !validSecretToken(reference.Version) {
		return ErrSecretUnresolvable
	}
	return nil
}

func validSecretToken(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
