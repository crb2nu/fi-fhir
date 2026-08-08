package destination

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestLoadRegistryResolvesOnlyExactDeployedDestinations(t *testing.T) {
	t.Parallel()

	alpha := mustHTTPSRevision(t)
	beta := mustRevision(t, httpsInput(func(input *RevisionInput) {
		input.ArtifactID = "dest-beta"
		input.DestinationID = "beta"
		input.HTTPS.URL = "https://beta.example/fhir"
		input.HTTPS.TokenBinding = "beta-token"
		input.HTTPS.CABundleBinding = ""
		input.Identity.Subject = "beta-client"
	}))
	registry := mustRegistry(t, registryJSON(t, "tenant-a", []Revision{alpha, beta}), ModeStrict)

	resolved, err := registry.Resolve("tenant-a", alpha.Reference())
	if err != nil || resolved.Identity.Subject != "alpha-client" {
		t.Fatalf("Resolve(alpha) = %#v error=%v", resolved, err)
	}

	// A reference for beta carrying alpha's digest must not resolve. This is the
	// exact substitution the dispatch path has to refuse.
	crossed := beta.Reference()
	crossed.Digest = alpha.Digest
	if _, err := registry.Resolve("tenant-a", crossed); !errors.Is(err, ErrDestinationUnverified) {
		t.Fatalf("Resolve(crossed digest) = %v, want unverified", err)
	}

	wrongClass := alpha.Reference()
	wrongClass.Class = integration.DestinationClassSandbox
	if _, err := registry.Resolve("tenant-a", wrongClass); !errors.Is(err, ErrDestinationUnverified) {
		t.Fatalf("Resolve(wrong class) = %v, want unverified", err)
	}

	orphan := alpha.Reference()
	orphan.ArtifactID = "dest-orphan"
	if _, err := registry.Resolve("tenant-a", orphan); !errors.Is(err, ErrDestinationUnknown) {
		t.Fatalf("Resolve(orphan) = %v, want unknown", err)
	}
	if _, err := registry.Resolve("tenant-b", alpha.Reference()); !errors.Is(err, ErrDestinationUnknown) {
		t.Fatalf("Resolve(other tenant) = %v, want unknown", err)
	}

	destinations := registry.Destinations()
	if len(destinations) != 2 || destinations[0].ArtifactID != "dest-alpha" ||
		destinations[1].ArtifactID != "dest-beta" {
		t.Fatalf("Destinations = %#v", destinations)
	}
}

func TestLoadRegistryStrictModeRefusesAnUnboundDestination(t *testing.T) {
	t.Parallel()

	unbound := mustRevision(t, RevisionInput{
		ArtifactID: "queue-primary", RevisionID: "destination-1", DestinationID: "queue-primary",
		Class: integration.DestinationClassProduction, Transport: TransportKafka,
		Kafka: &KafkaPolicy{Topic: "integration.delivery.v1"},
	})
	document := registryJSON(t, "tenant-a", []Revision{unbound})

	if _, err := LoadRegistry(strings.NewReader(document), ModeStrict); !errors.Is(err, ErrInvalidRegistry) {
		t.Fatalf("strict load with an unbound destination = %v, want rejected", err)
	}
	registry, err := LoadRegistry(strings.NewReader(document), ModeCompatibility)
	if err != nil {
		t.Fatalf("compatibility load: %v", err)
	}
	if registry.Mode() != ModeCompatibility || registry.TenantID() != "tenant-a" {
		t.Fatalf("registry = %#v", registry)
	}
	if registry.IntegrationRevision().ArtifactID != "integration-adt" {
		t.Fatalf("IntegrationRevision = %#v", registry.IntegrationRevision())
	}
}

func TestLoadRegistryFailsClosedOnMalformedDocuments(t *testing.T) {
	t.Parallel()

	alpha := mustHTTPSRevision(t)
	valid := registryJSON(t, "tenant-a", []Revision{alpha})
	documents := map[string]string{
		"empty":            "",
		"wrong schema":     strings.Replace(valid, registryDocumentSchema, "other/v1", 1),
		"no tenant":        strings.Replace(valid, `"tenant_id":"tenant-a"`, `"tenant_id":""`, 1),
		"unknown field":    strings.Replace(valid, `"destinations"`, `"surprise":1,"destinations"`, 1),
		"duplicate key":    strings.Replace(valid, `"destinations"`, `"tenant_id":"tenant-b","destinations"`, 1),
		"no destinations":  strings.Replace(valid, `"destinations":[`, `"destinations":[],"unused":[`, 1),
		"missing binding":  strings.Replace(valid, `{"name":"alpha-ca"`, `{"name":"unused-ca"`, 1),
		"mutated revision": strings.Replace(valid, `"alpha-client"`, `"impostor"`, 1),
	}
	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadRegistry(strings.NewReader(document), ModeStrict); !errors.Is(err, ErrInvalidRegistry) {
				t.Fatalf("LoadRegistry(%s) = %v", name, err)
			}
		})
	}
	if _, err := LoadRegistry(strings.NewReader(valid), "audit"); !errors.Is(err, ErrInvalidRegistry) {
		t.Fatalf("LoadRegistry(unknown mode) = %v", err)
	}
	if _, err := LoadRegistry(nil, ModeStrict); !errors.Is(err, ErrInvalidRegistry) {
		t.Fatalf("LoadRegistry(nil) = %v", err)
	}
}

func TestRegistryDeclaredSecretBindingsCarryReferencesOnly(t *testing.T) {
	t.Parallel()

	registry := mustRegistry(t, registryJSON(t, "tenant-a", []Revision{mustHTTPSRevision(t)}), ModeStrict)
	bindings := registry.SecretBindings()
	if len(bindings) != 3 {
		t.Fatalf("SecretBindings = %#v", bindings)
	}
	encoded, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("marshal bindings: %v", err)
	}
	for _, forbidden := range []string{"DESTINATION-SECRET-SENTINEL", "password", "bearer "} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("secret bindings carry %q: %s", forbidden, encoded)
		}
	}
	for _, binding := range bindings {
		if integration.ValidateSecretReference(binding.Reference) != nil {
			t.Fatalf("binding %q carries an unresolvable reference", binding.Name)
		}
	}
}

func mustRegistry(t *testing.T, document string, mode Mode) *Registry {
	t.Helper()
	registry, err := LoadRegistry(strings.NewReader(document), mode)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return registry
}

func registryJSON(t *testing.T, tenantID string, revisions []Revision) string {
	t.Helper()
	bindings := []integration.SecretBinding{
		{Name: "alpha-token", Reference: integration.SecretReference{
			Provider: integration.SecretProviderFile, Key: "alpha/token",
		}},
		{Name: "alpha-ca", Reference: integration.SecretReference{
			Provider: integration.SecretProviderFile, Key: "alpha/ca.pem",
		}},
		{Name: "beta-token", Reference: integration.SecretReference{
			Provider: integration.SecretProviderEnvironment, Key: "DESTINATION_TEST_BETA_TOKEN",
		}},
	}
	document := registryDocument{
		Schema:   registryDocumentSchema,
		TenantID: tenantID,
		IntegrationRevision: integration.ArtifactRevisionRef{
			ArtifactID: "integration-adt", RevisionID: "revision-1",
			Digest: "sha256:" + strings.Repeat("b", 64),
		},
		SecretBindings: bindings,
		Destinations:   revisions,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	return string(encoded)
}
