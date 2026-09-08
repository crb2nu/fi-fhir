package destination

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The digests below were computed at `main` @ a3335a71f, before Slice 4.1c-c
// added anything to the revision. They are string constants on purpose: a test
// that recomputed them would prove nothing.
const (
	pinnedHTTPSRevisionDigest = "sha256:9fdfcafa70ea89ffe1ec7aacb4acbf28a918cde02060e35bc2e197004be74f63"
	pinnedKafkaRevisionDigest = "sha256:4ac293d946b0091f08a9589ff3bdad601e68167b2ce54d49ace95f7dcceebfd5"
)

// TestRevisionDigest_DeployedTransportsArePinned is Slice 4.1c-c's
// digest-stability gate. It must PASS on unmodified `main` and must STILL pass
// after the `fhir` policy lands on Revision.
//
// `.loom/34` correction 8: the digest is
// `sha256(domain ‖ JSON(revision-without-digest))` (revision.go semanticDigest)
// and both existing policies are `omitempty` pointers. A nil `fhir` policy is
// therefore elided from the JSON and every deployed `kafka` and `https` digest
// is byte-stable across the slice. Every destination registry in every
// deployment carries these digests, and Registry.Resolve refuses an attempt
// whose reference does not match byte for byte — so a digest that moved would
// dead-letter every in-flight delivery on the first rollout. That has to be
// asserted, not assumed, which is what pinning one fixed revision of each
// transport does.
//
// If this test goes red after adding a field to Revision, the field is not
// `omitempty` or the JSON encoding of an existing field changed. Neither is
// acceptable; fix the struct, never the constant.
func TestRevisionDigest_DeployedTransportsArePinned(t *testing.T) {
	t.Parallel()

	https, err := NewRevision(RevisionInput{
		ArtifactID: "dest-https-pinned", RevisionID: "destination-1",
		DestinationID: "dest-https-pinned",
		Class:         integration.DestinationClassProduction,
		Transport:     TransportHTTPS,
		HTTPS: &HTTPSPolicy{
			URL: "https://destination.example.org/inbound", Method: "POST",
			TokenBinding: "pinned-token", CABundleBinding: "pinned-ca",
		},
		Identity: &ClientIdentity{
			Subject: "pinned-client",
			Grants:  []string{"integration.destination.client"},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(https): %v", err)
	}
	kafka, err := NewRevision(RevisionInput{
		ArtifactID: "dest-kafka-pinned", RevisionID: "destination-1",
		DestinationID: "dest-kafka-pinned",
		Class:         integration.DestinationClassSandbox,
		Transport:     TransportKafka,
		Kafka:         &KafkaPolicy{Topic: "integration.delivery.v1"},
		Identity: &ClientIdentity{
			Subject: "pinned-client",
			Grants:  []string{"integration.destination.client"},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(kafka): %v", err)
	}

	if https.Digest != pinnedHTTPSRevisionDigest {
		t.Fatalf("https revision digest moved:\n got  %s\n want %s\n"+
			"Every deployed https destination would fail Registry.Resolve on rollout.",
			https.Digest, pinnedHTTPSRevisionDigest)
	}
	if kafka.Digest != pinnedKafkaRevisionDigest {
		t.Fatalf("kafka revision digest moved:\n got  %s\n want %s\n"+
			"Every deployed kafka destination would fail Registry.Resolve on rollout.",
			kafka.Digest, pinnedKafkaRevisionDigest)
	}

	// The mechanism behind the stability: an absent policy is absent from the
	// encoded bytes, not present as null.
	for name, revision := range map[string]Revision{"https": https, "kafka": kafka} {
		encoded, err := json.Marshal(revision)
		if err != nil {
			t.Fatalf("marshal %s revision: %v", name, err)
		}
		if strings.Contains(string(encoded), "null") {
			t.Fatalf("%s revision encodes a null member; an absent policy must be elided, "+
				"not serialised, or the digest depends on fields the revision does not use: %s",
				name, encoded)
		}
		if strings.Contains(string(encoded), `"fhir"`) {
			t.Fatalf("%s revision encodes a fhir member it does not declare: %s", name, encoded)
		}
	}
}
