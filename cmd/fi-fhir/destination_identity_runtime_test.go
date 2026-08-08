package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const destinationSecretSentinel = "DESTINATION-SECRET-SENTINEL-9f3c"

func TestLoadDestinationIdentityIsAbsentUntilAModeIsChosen(t *testing.T) {
	clearDestinationIdentityEnv(t)
	runtime, err := loadDestinationIdentityFromEnv(t.Context(), nil)
	if err != nil || runtime != nil {
		t.Fatalf("unset mode runtime = %#v error = %v", runtime, err)
	}

	// A half-applied configuration must not run with the decision disabled.
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH", "/tmp/registry.json")
	if _, err := loadDestinationIdentityFromEnv(t.Context(), nil); err == nil ||
		!strings.Contains(err.Error(), "FI_FHIR_DELIVERY_IDENTITY_MODE is required") {
		t.Fatalf("registry path without a mode = %v", err)
	}
}

func TestLoadDestinationIdentityModesRejectEachOthersConfiguration(t *testing.T) {
	clearDestinationIdentityEnv(t)
	directory := t.TempDir()
	registryPath := writeDestinationRegistry(t, directory, true)
	writeDestinationSecret(t, directory, "alpha/token", destinationSecretSentinel)
	writeDestinationSecret(t, directory, "alpha/ca.pem", "-----BEGIN CERTIFICATE-----\n")

	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH", registryPath)
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR", directory)

	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_MODE", "audit")
	if _, err := loadDestinationIdentityFromEnv(t.Context(), nil); err == nil ||
		!strings.Contains(err.Error(), "must be strict or compatibility") {
		t.Fatalf("unknown mode = %v", err)
	}

	// strict + a compatibility subject is refused before any database work.
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_MODE", string(integrationdestination.ModeStrict))
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT", "fallback-client")
	if _, err := loadDestinationIdentityFromEnv(t.Context(), nil); err == nil ||
		!strings.Contains(err.Error(), "requires the PostgreSQL submission database") {
		t.Fatalf("strict without a database = %v", err)
	}

	// A strict registry containing an unbound destination fails to load at all.
	unboundPath := writeDestinationRegistry(t, directory, false)
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH", unboundPath)
	t.Setenv("FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT", "")
	if _, err := loadDestinationIdentityFromEnv(t.Context(), openUnusedDB(t)); err == nil ||
		!strings.Contains(err.Error(), "load delivery identity registry") {
		t.Fatalf("strict with an unbound destination = %v", err)
	}
}

func TestDestinationSecretResolverIsBoundedAndFailsClosed(t *testing.T) {
	directory := t.TempDir()
	writeDestinationSecret(t, directory, "alpha/token", destinationSecretSentinel)
	resolver, err := newDestinationSecretResolver(directory)
	if err != nil {
		t.Fatalf("newDestinationSecretResolver: %v", err)
	}

	material, err := resolver.Resolve(t.Context(), integration.SecretReference{
		Provider: integration.SecretProviderFile, Key: "alpha/token",
	})
	if err != nil || string(material) != destinationSecretSentinel {
		t.Fatalf("file resolve = %q error = %v", material, err)
	}

	t.Setenv("DESTINATION_TEST_TOKEN", destinationSecretSentinel)
	material, err = resolver.Resolve(t.Context(), integration.SecretReference{
		Provider: integration.SecretProviderEnvironment, Key: "DESTINATION_TEST_TOKEN",
	})
	if err != nil || string(material) != destinationSecretSentinel {
		t.Fatalf("env resolve = %q error = %v", material, err)
	}

	references := map[string]integration.SecretReference{
		"escape":            {Provider: integration.SecretProviderFile, Key: "../outside"},
		"absolute":          {Provider: integration.SecretProviderFile, Key: "/etc/passwd"},
		"absent file":       {Provider: integration.SecretProviderFile, Key: "alpha/missing"},
		"absent env":        {Provider: integration.SecretProviderEnvironment, Key: "DESTINATION_TEST_ABSENT"},
		"lowercase env":     {Provider: integration.SecretProviderEnvironment, Key: "lowercase"},
		"pinned version":    {Provider: integration.SecretProviderFile, Key: "alpha/token", Version: "2"},
		"vault provider":    {Provider: integration.SecretProviderVault, Key: "alpha/token"},
		"unknown provider":  {Provider: "s3", Key: "alpha/token"},
		"empty key":         {Provider: integration.SecretProviderFile, Key: ""},
		"traversal segment": {Provider: integration.SecretProviderFile, Key: "alpha/./token"},
	}
	for name, reference := range references {
		if _, err := resolver.Resolve(t.Context(), reference); err == nil {
			t.Fatalf("Resolve(%s) succeeded", name)
		}
	}

	// Without a configured directory the file provider resolves nothing at all.
	rootless, err := newDestinationSecretResolver("")
	if err != nil {
		t.Fatalf("newDestinationSecretResolver(\"\"): %v", err)
	}
	if _, err := rootless.Resolve(t.Context(), integration.SecretReference{
		Provider: integration.SecretProviderFile, Key: "alpha/token",
	}); err == nil {
		t.Fatal("file provider resolved without a configured directory")
	}
	if _, err := newDestinationSecretResolver(filepath.Join(directory, "alpha", "token")); err == nil {
		t.Fatal("a file was accepted as the secret directory")
	}
}

// TestVerifyDestinationSecretsFailsClosedAndRetainsNothing proves the startup
// check is load-bearing: a missing credential refuses startup, and a resolved
// credential leaves no copy behind.
func TestVerifyDestinationSecretsFailsClosedAndRetainsNothing(t *testing.T) {
	directory := t.TempDir()
	registryPath := writeDestinationRegistry(t, directory, true)
	registryFile, err := os.Open(registryPath)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer func() { _ = registryFile.Close() }()
	registry, err := integrationdestination.LoadRegistry(registryFile, integrationdestination.ModeStrict)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	resolver, err := newDestinationSecretResolver(directory)
	if err != nil {
		t.Fatalf("newDestinationSecretResolver: %v", err)
	}

	if err := verifyDestinationSecrets(t.Context(), registry, resolver); err == nil ||
		!strings.Contains(err.Error(), "unresolvable") {
		t.Fatalf("missing credential = %v", err)
	}

	writeDestinationSecret(t, directory, "alpha/token", destinationSecretSentinel)
	writeDestinationSecret(t, directory, "alpha/ca.pem", "-----BEGIN CERTIFICATE-----\n")
	t.Setenv("DESTINATION_TEST_BETA_TOKEN", destinationSecretSentinel)
	if err := verifyDestinationSecrets(t.Context(), registry, resolver); err != nil {
		t.Fatalf("verifyDestinationSecrets: %v", err)
	}

	// The registry that survives startup carries binding names and references,
	// never material.
	encoded, err := json.Marshal(registry.SecretBindings())
	if err != nil {
		t.Fatalf("marshal bindings: %v", err)
	}
	if strings.Contains(string(encoded), destinationSecretSentinel) {
		t.Fatalf("resolved material was retained: %s", encoded)
	}
	for _, revision := range registry.Destinations() {
		revisionJSON, err := json.Marshal(revision)
		if err != nil {
			t.Fatalf("marshal revision: %v", err)
		}
		if strings.Contains(string(revisionJSON), destinationSecretSentinel) {
			t.Fatalf("destination revision carries material: %s", revisionJSON)
		}
	}
}

// openUnusedDB provides a handle that is never dialed. Registry loading and mode
// validation must fail before any connection is attempted.
func openUnusedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://unused")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func clearDestinationIdentityEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"FI_FHIR_DELIVERY_IDENTITY_MODE",
		"FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH",
		"FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT",
		"FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR",
	} {
		t.Setenv(name, "")
	}
}

func writeDestinationSecret(t *testing.T, root, key, value string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create secret directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
}

func writeDestinationRegistry(t *testing.T, directory string, bound bool) string {
	t.Helper()
	input := integrationdestination.RevisionInput{
		ArtifactID:    "dest-alpha",
		RevisionID:    "destination-1",
		DestinationID: "alpha",
		Class:         integration.DestinationClassProduction,
		Transport:     integrationdestination.TransportHTTPS,
		HTTPS: &integrationdestination.HTTPSPolicy{
			URL: "https://alpha.example/fhir", Method: "POST",
			TokenBinding: "alpha-token", CABundleBinding: "alpha-ca",
		},
	}
	if bound {
		input.Identity = &integrationdestination.ClientIdentity{
			Subject: "alpha-client", Grants: []string{"integration.destination.client"},
		}
	}
	revision, err := integrationdestination.NewRevision(input)
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	document := map[string]any{
		"schema":    "fi-fhir/destination-registry/v1",
		"tenant_id": "tenant-a",
		"integration_revision": map[string]string{
			"artifact_id": "integration-adt", "revision_id": "revision-1",
			"digest": "sha256:" + strings.Repeat("b", 64),
		},
		"secret_bindings": []map[string]any{
			{"name": "alpha-token", "reference": map[string]string{"provider": "file", "key": "alpha/token"}},
			{"name": "alpha-ca", "reference": map[string]string{"provider": "file", "key": "alpha/ca.pem"}},
		},
		"destinations": []integrationdestination.Revision{revision},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	name := "registry-bound.json"
	if !bound {
		name = "registry-unbound.json"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}
