package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const maxDestinationRegistryBytes = 1 << 22

// destinationIdentityRuntime carries the wired decision and the mode to report
// at startup.
type destinationIdentityRuntime struct {
	authorizer *integrationdestination.Authorizer
	mode       integrationdestination.Mode
	registry   *integrationdestination.Registry
}

// loadDestinationIdentityFromEnv wires the Slice 4.1c-a delivery identity
// decision, or reports that it is not configured.
//
// The mode is explicit and the two modes reject each other's configuration:
//   - strict refuses a compatibility subject, and refuses to load a destination
//     set containing any unbound destination, so it fails startup rather than
//     authorizing one later;
//   - compatibility requires a subject to issue its grant under.
//
// Every FI_FHIR_DELIVERY_IDENTITY_* setting without a mode is also refused, so a
// half-applied configuration cannot silently run with the decision disabled.
func loadDestinationIdentityFromEnv(
	ctx context.Context,
	db *sql.DB,
) (*destinationIdentityRuntime, error) {
	mode := strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_MODE"))
	registryPath := strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH"))
	compatibilitySubject := strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT"))
	secretDirectory := strings.TrimSpace(os.Getenv("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR"))

	if mode == "" {
		if registryPath != "" || compatibilitySubject != "" || secretDirectory != "" {
			return nil, fmt.Errorf(
				"FI_FHIR_DELIVERY_IDENTITY_MODE is required with any FI_FHIR_DELIVERY_IDENTITY_* setting")
		}
		return nil, nil
	}
	identityMode := integrationdestination.Mode(mode)
	if identityMode != integrationdestination.ModeStrict &&
		identityMode != integrationdestination.ModeCompatibility {
		return nil, fmt.Errorf("FI_FHIR_DELIVERY_IDENTITY_MODE must be strict or compatibility")
	}
	if registryPath == "" {
		return nil, fmt.Errorf("FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH is required with a delivery identity mode")
	}
	if db == nil {
		return nil, fmt.Errorf("delivery identity requires the PostgreSQL submission database")
	}

	registry, err := loadDestinationRegistry(registryPath, identityMode)
	if err != nil {
		return nil, err
	}
	resolver, err := newDestinationSecretResolver(secretDirectory)
	if err != nil {
		return nil, err
	}
	if err := verifyDestinationSecrets(ctx, registry, resolver); err != nil {
		return nil, err
	}
	provenance, err := integrationdestination.NewPostgresProvenance(db)
	if err != nil {
		return nil, fmt.Errorf("configure delivery identity provenance: %w", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate delivery identity provenance: %w", err)
	}
	authorizer, err := integrationdestination.NewAuthorizer(integrationdestination.AuthorizerConfig{
		Registry:             registry,
		Recorder:             provenance,
		CompatibilitySubject: compatibilitySubject,
	})
	if err != nil {
		return nil, fmt.Errorf("configure delivery identity: %w", err)
	}
	return &destinationIdentityRuntime{
		authorizer: authorizer,
		mode:       identityMode,
		registry:   registry,
	}, nil
}

func loadDestinationRegistry(
	path string,
	mode integrationdestination.Mode,
) (*integrationdestination.Registry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open delivery identity registry: %w", err)
	}
	defer func() { _ = file.Close() }()
	registry, err := integrationdestination.LoadRegistry(
		io.LimitReader(file, maxDestinationRegistryBytes+1), mode,
	)
	if err != nil {
		return nil, fmt.Errorf("load delivery identity registry: %w", err)
	}
	return registry, nil
}

// verifyDestinationSecrets resolves every declared binding once at startup and
// discards the material.
//
// It is the fail-closed half of the secret-reference contract: a deployment
// whose destination names a credential that does not resolve refuses to start,
// instead of discovering the gap on the first dispatch. Nothing is retained —
// the resolved bytes are zeroed and never enter a struct, a record, or a log.
func verifyDestinationSecrets(
	ctx context.Context,
	registry *integrationdestination.Registry,
	resolver integration.SecretResolver,
) error {
	for _, binding := range registry.SecretBindings() {
		material, err := resolver.Resolve(ctx, binding.Reference)
		if err != nil {
			return fmt.Errorf("delivery identity secret binding %q is unresolvable", binding.Name)
		}
		if len(material) == 0 {
			return fmt.Errorf("delivery identity secret binding %q is empty", binding.Name)
		}
		for index := range material {
			material[index] = 0
		}
	}
	return nil
}

// destinationSecretResolver is the file/env implementation of
// integration.SecretResolver.
//
// It lives in cmd/ beside the other single-line secret loaders so no package
// under internal/integration/* can hold resolved material in a struct that is
// later marshaled into a durable record.
type destinationSecretResolver struct {
	root string
}

func newDestinationSecretResolver(root string) (*destinationSecretResolver, error) {
	if root == "" {
		return &destinationSecretResolver{}, nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR is invalid")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR must be an existing directory")
	}
	return &destinationSecretResolver{root: absolute}, nil
}

// Resolve returns bounded secret material for env- and file-backed references.
//
// Every failure collapses to integration.ErrSecretUnresolvable so a caller
// cannot distinguish "no such key" from "unreadable" and enumerate the secret
// inventory.
func (r *destinationSecretResolver) Resolve(
	ctx context.Context,
	reference integration.SecretReference,
) ([]byte, error) {
	if r == nil || ctx == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := integration.ValidateSecretReference(reference); err != nil {
		return nil, integration.ErrSecretUnresolvable
	}
	// Version pinning needs a provider that stores versions. File and env do not,
	// so a pinned reference fails closed rather than silently returning whatever
	// version happens to be on disk.
	if reference.Version != "" {
		return nil, integration.ErrSecretUnresolvable
	}
	switch reference.Provider {
	case integration.SecretProviderEnvironment:
		return r.resolveEnvironment(reference.Key)
	case integration.SecretProviderFile:
		return r.resolveFile(reference.Key)
	default:
		return nil, integration.ErrSecretUnresolvable
	}
}

func (r *destinationSecretResolver) resolveEnvironment(key string) ([]byte, error) {
	if !validEnvironmentKey(key) {
		return nil, integration.ErrSecretUnresolvable
	}
	value := os.Getenv(key)
	if value == "" || len(value) > integration.MaxSecretBytes {
		return nil, integration.ErrSecretUnresolvable
	}
	return []byte(value), nil
}

func (r *destinationSecretResolver) resolveFile(key string) ([]byte, error) {
	if r.root == "" || !validSecretFileKey(key) {
		return nil, integration.ErrSecretUnresolvable
	}
	path := filepath.Join(r.root, filepath.FromSlash(key))
	if path != filepath.Clean(path) || !strings.HasPrefix(path, r.root+string(os.PathSeparator)) {
		return nil, integration.ErrSecretUnresolvable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, integration.ErrSecretUnresolvable
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, integration.MaxSecretBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(raw) == 0 || len(raw) > integration.MaxSecretBytes {
		return nil, integration.ErrSecretUnresolvable
	}
	return raw, nil
}

func validEnvironmentKey(key string) bool {
	for index, character := range key {
		switch {
		case character >= 'A' && character <= 'Z', character == '_':
		case character >= '0' && character <= '9' && index > 0:
		default:
			return false
		}
	}
	return key != ""
}

func validSecretFileKey(key string) bool {
	if key == "" || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return false
	}
	for _, element := range strings.Split(key, "/") {
		if element == "" || element == "." || element == ".." {
			return false
		}
	}
	return true
}
