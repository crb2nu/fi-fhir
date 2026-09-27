package connection

import (
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Destination spec bounds, restated from destination.Revision.validateSemanticFields.
const (
	destinationMaxIdentityGrants = 16
	destinationMaxTopicBytes     = 249
)

// HTTPSSpec is the `https` connection spec: destination.RevisionInput with
// transport https.
type HTTPSSpec struct {
	DestinationID string           `json:"destination_id"`
	Class         string           `json:"class"`
	HTTPS         *HTTPSPolicySpec `json:"https"`
	Identity      *IdentitySpec    `json:"identity,omitempty"`
}

// HTTPSPolicySpec is destination.HTTPSPolicy.
type HTTPSPolicySpec struct {
	URL             string `json:"url"`
	Method          string `json:"method"`
	TokenBinding    string `json:"token_binding"`
	CABundleBinding string `json:"ca_bundle_binding,omitempty"`
}

// FHIRSpec is the `fhir` connection spec: destination.RevisionInput with
// transport fhir. interaction defaults to "transaction", the only value.
type FHIRSpec struct {
	DestinationID string          `json:"destination_id"`
	Class         string          `json:"class"`
	FHIR          *FHIRPolicySpec `json:"fhir"`
	Identity      *IdentitySpec   `json:"identity,omitempty"`
}

// FHIRPolicySpec is destination.FHIRPolicy.
type FHIRPolicySpec struct {
	BaseURL         string `json:"base_url"`
	TokenBinding    string `json:"token_binding"`
	CABundleBinding string `json:"ca_bundle_binding,omitempty"`
	Interaction     string `json:"interaction,omitempty"`
}

// KafkaSpec is the `kafka` connection spec: destination.RevisionInput with
// transport kafka.
type KafkaSpec struct {
	DestinationID string           `json:"destination_id"`
	Class         string           `json:"class"`
	Kafka         *KafkaPolicySpec `json:"kafka"`
	Identity      *IdentitySpec    `json:"identity,omitempty"`
}

// KafkaPolicySpec is destination.KafkaPolicy.
type KafkaPolicySpec struct {
	Topic string `json:"topic"`
}

// IdentitySpec is destination.ClientIdentity: a subject and one to sixteen grants.
type IdentitySpec struct {
	Subject string   `json:"subject"`
	Grants  []string `json:"grants"`
}

func checkDestinationCommon(c *checker, destinationID, class string, identity *IdentitySpec) {
	c.identity("destination_id", destinationID)
	c.enum("class", class, string(integration.DestinationClassProduction), string(integration.DestinationClassSandbox))
	if identity != nil {
		c.identity("identity.subject", identity.Subject)
		c.grants("identity.grants", identity.Grants, 1, destinationMaxIdentityGrants)
	}
}

// checkTokenAndCA applies the shared token/CA-bundle binding rule of the
// https and fhir policies.
func checkTokenAndCA(c *checker, prefix, token, caBundle string) {
	tokenOK := c.binding(prefix+".token_binding", token)
	if caBundle == "" {
		return
	}
	if c.binding(prefix+".ca_bundle_binding", caBundle) && tokenOK && caBundle == token {
		c.add(CodeConflict, prefix+".ca_bundle_binding", "must name a different binding than token_binding")
	}
}

func (s *HTTPSSpec) check(c *checker) {
	checkDestinationCommon(c, s.DestinationID, s.Class, s.Identity)
	if s.HTTPS == nil {
		c.add(CodeRequired, "https", "is required")
		return
	}
	if reason := httpsURLProblem(s.HTTPS.URL, true); reason == "required" {
		c.add(CodeRequired, "https.url", "is required")
	} else if reason != "" {
		c.add(CodeInvalidURL, "https.url", reason)
	}
	c.enum("https.method", s.HTTPS.Method, "POST", "PUT")
	checkTokenAndCA(c, "https", s.HTTPS.TokenBinding, s.HTTPS.CABundleBinding)
}

func (s *FHIRSpec) check(c *checker) {
	checkDestinationCommon(c, s.DestinationID, s.Class, s.Identity)
	if s.FHIR == nil {
		c.add(CodeRequired, "fhir", "is required")
		return
	}
	if reason := httpsURLProblem(s.FHIR.BaseURL, false); reason == "required" {
		c.add(CodeRequired, "fhir.base_url", "is required")
	} else if reason != "" {
		c.add(CodeInvalidURL, "fhir.base_url", reason)
	}
	if s.FHIR.Interaction != "" && s.FHIR.Interaction != destination.FHIRInteractionTransaction {
		c.add(CodeInvalidEnum, "fhir.interaction", "must be \"transaction\"")
	}
	checkTokenAndCA(c, "fhir", s.FHIR.TokenBinding, s.FHIR.CABundleBinding)
}

func (s *KafkaSpec) check(c *checker) {
	checkDestinationCommon(c, s.DestinationID, s.Class, s.Identity)
	if s.Kafka == nil {
		c.add(CodeRequired, "kafka", "is required")
		return
	}
	if c.identity("kafka.topic", s.Kafka.Topic) && len(s.Kafka.Topic) > destinationMaxTopicBytes {
		c.add(CodeOutOfRange, "kafka.topic", "must be at most 249 characters")
	}
}

func destinationIdentity(identity *IdentitySpec) *destination.ClientIdentity {
	if identity == nil {
		return nil
	}
	return &destination.ClientIdentity{
		Subject: identity.Subject,
		Grants:  append([]string(nil), identity.Grants...),
	}
}

func (s *HTTPSSpec) input(artifactID, revisionID string) destination.RevisionInput {
	input := destination.RevisionInput{
		ArtifactID: artifactID, RevisionID: revisionID, DestinationID: s.DestinationID,
		Class: integration.DestinationClass(s.Class), Transport: destination.TransportHTTPS,
		Identity: destinationIdentity(s.Identity),
	}
	if s.HTTPS != nil {
		input.HTTPS = &destination.HTTPSPolicy{
			URL: s.HTTPS.URL, Method: s.HTTPS.Method,
			TokenBinding: s.HTTPS.TokenBinding, CABundleBinding: s.HTTPS.CABundleBinding,
		}
	}
	return input
}

func (s *FHIRSpec) input(artifactID, revisionID string) destination.RevisionInput {
	input := destination.RevisionInput{
		ArtifactID: artifactID, RevisionID: revisionID, DestinationID: s.DestinationID,
		Class: integration.DestinationClass(s.Class), Transport: destination.TransportFHIR,
		Identity: destinationIdentity(s.Identity),
	}
	if s.FHIR != nil {
		interaction := s.FHIR.Interaction
		if interaction == "" {
			interaction = destination.FHIRInteractionTransaction
		}
		input.FHIR = &destination.FHIRPolicy{
			BaseURL: s.FHIR.BaseURL, TokenBinding: s.FHIR.TokenBinding,
			CABundleBinding: s.FHIR.CABundleBinding, Interaction: interaction,
		}
	}
	return input
}

func (s *KafkaSpec) input(artifactID, revisionID string) destination.RevisionInput {
	input := destination.RevisionInput{
		ArtifactID: artifactID, RevisionID: revisionID, DestinationID: s.DestinationID,
		Class: integration.DestinationClass(s.Class), Transport: destination.TransportKafka,
		Identity: destinationIdentity(s.Identity),
	}
	if s.Kafka != nil {
		input.Kafka = &destination.KafkaPolicy{Topic: s.Kafka.Topic}
	}
	return input
}
