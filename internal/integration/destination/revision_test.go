package destination

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestNewRevisionContentAddressesEverySemanticField(t *testing.T) {
	t.Parallel()

	revision := mustHTTPSRevision(t)
	if !strings.HasPrefix(revision.Digest, "sha256:") || len(revision.Digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q", revision.Digest)
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	reference := revision.Reference()
	if reference.ArtifactID != revision.ArtifactID || reference.RevisionID != revision.RevisionID ||
		reference.Digest != revision.Digest || reference.Class != revision.Class {
		t.Fatalf("Reference = %#v", reference)
	}

	// Grant order is canonicalized, so a reordered identity is the same artifact.
	reordered := mustRevision(t, httpsInput(func(input *RevisionInput) {
		input.Identity.Grants = []string{"integration.destination.client", "alpha.extra"}
	}))
	sorted := mustRevision(t, httpsInput(func(input *RevisionInput) {
		input.Identity.Grants = []string{"alpha.extra", "integration.destination.client"}
	}))
	if reordered.Digest != sorted.Digest {
		t.Fatal("grant ordering changed the destination digest")
	}
}

func TestRevisionRejectsEverySemanticMutation(t *testing.T) {
	t.Parallel()

	base := mustHTTPSRevision(t)
	mutations := map[string]func(*Revision){
		"artifact":        func(r *Revision) { r.ArtifactID = "other-destination" },
		"revision":        func(r *Revision) { r.RevisionID = "destination-2" },
		"destination":     func(r *Revision) { r.DestinationID = "other" },
		"class":           func(r *Revision) { r.Class = integration.DestinationClassSandbox },
		"url":             func(r *Revision) { r.HTTPS.URL = "https://elsewhere.example/fhir" },
		"method":          func(r *Revision) { r.HTTPS.Method = "PUT" },
		"token binding":   func(r *Revision) { r.HTTPS.TokenBinding = "other-token" },
		"identity":        func(r *Revision) { r.Identity.Subject = "impostor" },
		"identity grants": func(r *Revision) { r.Identity.Grants = []string{"integration.destination.client", "extra"} },
		"schema":          func(r *Revision) { r.SchemaVersion = "2" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mutated := base
			mutated.HTTPS = cloneHTTPS(base.HTTPS)
			mutated.Identity = cloneIdentity(base.Identity)
			mutate(&mutated)
			if err := mutated.Validate(); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("Validate after %s mutation = %v", name, err)
			}
		})
	}
}

func TestDecodeRevisionRejectsMalformedDocuments(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(mustHTTPSRevision(t))
	if err != nil {
		t.Fatalf("marshal revision: %v", err)
	}
	if _, err := DecodeRevision(bytes.NewReader(valid)); err != nil {
		t.Fatalf("DecodeRevision(valid): %v", err)
	}

	documents := map[string][]byte{
		"empty":          nil,
		"unknown field":  append(bytes.TrimSuffix(valid, []byte("}")), []byte(`,"extra":1}`)...),
		"duplicate key":  append(bytes.TrimSuffix(valid, []byte("}")), []byte(`,"class":"sandbox"}`)...),
		"trailing value": append(append([]byte(nil), valid...), []byte(`{}`)...),
		"not an object":  []byte(`[]`),
	}
	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeRevision(bytes.NewReader(document)); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("DecodeRevision(%s) error = %v", name, err)
			}
		})
	}
	if _, err := DecodeRevision(nil); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("DecodeRevision(nil) error = %v", err)
	}
}

func TestNewRevisionFailsClosedOnTransportAndIdentity(t *testing.T) {
	t.Parallel()

	inputs := map[string]RevisionInput{
		"no transport": httpsInput(func(input *RevisionInput) { input.Transport = "" }),
		"both transports": httpsInput(func(input *RevisionInput) {
			input.Kafka = &KafkaPolicy{Topic: "integration.delivery.v1"}
		}),
		"plaintext url":  httpsInput(func(input *RevisionInput) { input.HTTPS.URL = "http://insecure.example/fhir" }),
		"url with user":  httpsInput(func(input *RevisionInput) { input.HTTPS.URL = "https://user:pass@example/fhir" }),
		"unsafe method":  httpsInput(func(input *RevisionInput) { input.HTTPS.Method = "GET" }),
		"no token":       httpsInput(func(input *RevisionInput) { input.HTTPS.TokenBinding = "" }),
		"binding collis": httpsInput(func(input *RevisionInput) { input.HTTPS.CABundleBinding = input.HTTPS.TokenBinding }),
		"empty identity": httpsInput(func(input *RevisionInput) { input.Identity = &ClientIdentity{Subject: "s"} }),
		"blank subject":  httpsInput(func(input *RevisionInput) { input.Identity.Subject = " " }),
		"dup grants": httpsInput(func(input *RevisionInput) {
			input.Identity.Grants = []string{"integration.destination.client", "integration.destination.client"}
		}),
		"bad class": httpsInput(func(input *RevisionInput) { input.Class = "internal" }),
		"kafka without topic": {
			ArtifactID: "dest", RevisionID: "1", DestinationID: "dest",
			Class: integration.DestinationClassProduction, Transport: TransportKafka,
			Kafka: &KafkaPolicy{},
		},
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRevision(input); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("NewRevision(%s) error = %v", name, err)
			}
		})
	}
}

func TestRevisionSecretBindingNamesAndEndpointAdvisory(t *testing.T) {
	t.Parallel()

	https := mustHTTPSRevision(t)
	names := https.SecretBindingNames()
	if len(names) != 2 || names[0] != "alpha-token" || names[1] != "alpha-ca" {
		t.Fatalf("SecretBindingNames = %#v", names)
	}
	if https.EndpointAdvisory() != "https://alpha.example/fhir" {
		t.Fatalf("EndpointAdvisory = %q", https.EndpointAdvisory())
	}

	kafka := mustRevision(t, RevisionInput{
		ArtifactID: "queue-primary", RevisionID: "destination-1", DestinationID: "queue-primary",
		Class: integration.DestinationClassProduction, Transport: TransportKafka,
		Kafka: &KafkaPolicy{Topic: "integration.delivery.v1"},
	})
	if kafka.SecretBindingNames() != nil {
		t.Fatalf("Kafka SecretBindingNames = %#v", kafka.SecretBindingNames())
	}
	if kafka.IdentityBound() {
		t.Fatal("unbound destination reported an identity")
	}
	if kafka.EndpointAdvisory() != "integration.delivery.v1" {
		t.Fatalf("Kafka EndpointAdvisory = %q", kafka.EndpointAdvisory())
	}
}

func TestValidateAgainstRequiresEveryNamedBindingInTheDeployedRelease(t *testing.T) {
	t.Parallel()

	revision := mustHTTPSRevision(t)
	binding := lifecycle.RunnableBinding{
		IntegrationRevision: integration.ArtifactRevisionRef{
			ArtifactID: "integration-adt", RevisionID: "revision-1",
			Digest: "sha256:" + strings.Repeat("b", 64),
		},
		SourceID:       "adt-east",
		Format:         events.FormatHL7v2,
		Classification: integration.DataClassificationPHI,
		Deployment:     validDeploymentPolicy(),
		SecretBindings: []integration.SecretBinding{
			{Name: "alpha-token", Reference: integration.SecretReference{
				Provider: integration.SecretProviderFile, Key: "alpha/token",
			}},
			{Name: "alpha-ca", Reference: integration.SecretReference{
				Provider: integration.SecretProviderFile, Key: "alpha/ca.pem",
			}},
		},
	}
	if err := revision.ValidateAgainst(binding); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}

	missing := binding
	missing.SecretBindings = binding.SecretBindings[:1]
	if err := revision.ValidateAgainst(missing); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("ValidateAgainst(missing binding) = %v", err)
	}
	unbound := binding
	unbound.IntegrationRevision = integration.ArtifactRevisionRef{}
	if err := revision.ValidateAgainst(unbound); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("ValidateAgainst(no deployed revision) = %v", err)
	}
	mutated := revision
	mutated.Digest = "sha256:" + strings.Repeat("0", 64)
	if err := mutated.ValidateAgainst(binding); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("ValidateAgainst(mutated digest) = %v", err)
	}
}

func httpsInput(mutate func(*RevisionInput)) RevisionInput {
	input := RevisionInput{
		ArtifactID:    "dest-alpha",
		RevisionID:    "destination-1",
		DestinationID: "alpha",
		Class:         integration.DestinationClassProduction,
		Transport:     TransportHTTPS,
		HTTPS: &HTTPSPolicy{
			URL: "https://alpha.example/fhir", Method: "POST",
			TokenBinding: "alpha-token", CABundleBinding: "alpha-ca",
		},
		Identity: &ClientIdentity{
			Subject: "alpha-client",
			Grants:  []string{"integration.destination.client"},
		},
	}
	if mutate != nil {
		mutate(&input)
	}
	return input
}

func mustHTTPSRevision(t *testing.T) Revision {
	t.Helper()
	return mustRevision(t, httpsInput(nil))
}

func mustRevision(t *testing.T, input RevisionInput) Revision {
	t.Helper()
	revision, err := NewRevision(input)
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	return revision
}

func validDeploymentPolicy() integration.IntegrationDeploymentPolicy {
	return integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health: integration.HealthPolicy{
			StartupGraceSeconds: 30, CheckIntervalSeconds: 15,
			TimeoutSeconds: 5, FailureThreshold: 3,
		},
		Capacity: integration.CapacityPolicy{
			MaxInFlight: 32, MaxQueued: 1024, MaxMessagesPerSecond: 250,
		},
	}
}
