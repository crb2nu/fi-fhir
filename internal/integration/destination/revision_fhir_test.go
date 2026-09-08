package destination

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func fhirRevisionInput(mutate func(*RevisionInput)) RevisionInput {
	input := RevisionInput{
		ArtifactID: "dest-fhir", RevisionID: "destination-1", DestinationID: "dest-fhir",
		Class: integration.DestinationClassProduction, Transport: TransportFHIR,
		FHIR: &FHIRPolicy{
			BaseURL: "https://fhir.example.org/r4", TokenBinding: "fhir-token",
			CABundleBinding: "fhir-ca", Interaction: FHIRInteractionTransaction,
		},
		Identity: &ClientIdentity{
			Subject: "fhir-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	}
	if mutate != nil {
		mutate(&input)
	}
	return input
}

func TestFHIRRevisionIsAdmittedWithExactlyItsOwnPolicy(t *testing.T) {
	t.Parallel()

	revision, err := NewRevision(fhirRevisionInput(nil))
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	if revision.Transport != TransportFHIR || revision.FHIR == nil || revision.HTTPS != nil || revision.Kafka != nil {
		t.Fatalf("revision = %+v", revision)
	}
	if got := revision.SecretBindingNames(); strings.Join(got, ",") != "fhir-token,fhir-ca" {
		t.Fatalf("SecretBindingNames = %v", got)
	}
	if revision.EndpointAdvisory() != "https://fhir.example.org/r4" {
		t.Fatalf("EndpointAdvisory = %q", revision.EndpointAdvisory())
	}
	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"fhir":{"base_url":"https://fhir.example.org/r4","token_binding":"fhir-token","ca_bundle_binding":"fhir-ca","interaction":"transaction"}`) ||
		strings.Contains(string(encoded), `"https"`) || strings.Contains(string(encoded), `"kafka"`) {
		t.Fatalf("encoded revision = %s", encoded)
	}
	decoded, err := DecodeRevision(strings.NewReader(string(encoded)))
	if err != nil || decoded.Digest != revision.Digest {
		t.Fatalf("DecodeRevision = %+v, %v", decoded, err)
	}

	// The policy is digest-covered.
	moved, err := NewRevision(fhirRevisionInput(func(input *RevisionInput) {
		input.FHIR.BaseURL = "https://fhir.example.org/other"
	}))
	if err != nil || moved.Digest == revision.Digest {
		t.Fatalf("base_url is not covered by the digest: %v", err)
	}
	mutated := revision
	mutated.FHIR = cloneFHIR(revision.FHIR)
	mutated.FHIR.TokenBinding = "other"
	if err := mutated.Validate(); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("Validate after token_binding mutation = %v", err)
	}

	registry := newTransportTestRegistry(t, map[string]string{"fhir-token": "token", "fhir-ca": "ca"}, revision)
	if !registry.HasTransport(TransportFHIR) || registry.HasTransport(TransportHTTPS) {
		t.Fatal("HasTransport does not report the fhir destination")
	}
}

func TestFHIRRevisionRejectsEveryMalformedPolicy(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*RevisionInput){
		"http scheme":       func(i *RevisionInput) { i.FHIR.BaseURL = "http://fhir.example.org/r4" },
		"userinfo":          func(i *RevisionInput) { i.FHIR.BaseURL = "https://user:pw@fhir.example.org/r4" },
		"fragment":          func(i *RevisionInput) { i.FHIR.BaseURL = "https://fhir.example.org/r4#frag" },
		"query":             func(i *RevisionInput) { i.FHIR.BaseURL = "https://fhir.example.org/r4?_format=json" },
		"no host":           func(i *RevisionInput) { i.FHIR.BaseURL = "https:///r4" },
		"too long":          func(i *RevisionInput) { i.FHIR.BaseURL = "https://fhir.example.org/" + strings.Repeat("a", 2048) },
		"batch interaction": func(i *RevisionInput) { i.FHIR.Interaction = "batch" },
		"empty interaction": func(i *RevisionInput) { i.FHIR.Interaction = "" },
		"no token binding":  func(i *RevisionInput) { i.FHIR.TokenBinding = "" },
		"ca equals token":   func(i *RevisionInput) { i.FHIR.CABundleBinding = i.FHIR.TokenBinding },
		"fhir and https": func(i *RevisionInput) {
			i.HTTPS = &HTTPSPolicy{URL: "https://x.example", Method: "POST", TokenBinding: "t"}
		},
		"fhir and kafka": func(i *RevisionInput) { i.Kafka = &KafkaPolicy{Topic: "integration.delivery.v1"} },
		"fhir transport no policy": func(i *RevisionInput) {
			i.FHIR = nil
		},
		"https transport with fhir policy": func(i *RevisionInput) {
			i.Transport = TransportHTTPS
			i.HTTPS = &HTTPSPolicy{URL: "https://x.example", Method: "POST", TokenBinding: "t"}
		},
		"kafka transport with fhir policy": func(i *RevisionInput) {
			i.Transport = TransportKafka
			i.Kafka = &KafkaPolicy{Topic: "integration.delivery.v1"}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRevision(fhirRevisionInput(mutate)); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("NewRevision = %v, want ErrInvalidRevision", err)
			}
		})
	}
}

func TestFHIRRevisionWithoutACABundleUsesTheSystemPool(t *testing.T) {
	t.Parallel()
	revision, err := NewRevision(fhirRevisionInput(func(input *RevisionInput) { input.FHIR.CABundleBinding = "" }))
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	if got := revision.SecretBindingNames(); strings.Join(got, ",") != "fhir-token" {
		t.Fatalf("SecretBindingNames = %v", got)
	}
}
