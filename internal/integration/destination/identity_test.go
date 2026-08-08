package destination

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestNewAuthorizerRejectsEachModesOppositeConfiguration(t *testing.T) {
	t.Parallel()

	bound := mustRegistry(t, registryJSON(t, "tenant-a", []Revision{mustHTTPSRevision(t)}), ModeStrict)
	unbound := mustRegistry(t, registryJSON(t, "tenant-a", []Revision{unboundKafkaRevision(t)}), ModeCompatibility)

	if _, err := NewAuthorizer(AuthorizerConfig{
		Registry: bound, Recorder: &recordingProvenance{}, CompatibilitySubject: "fallback-client",
	}); err == nil || !strings.Contains(err.Error(), "strict rejects") {
		t.Fatalf("strict with a compatibility subject = %v", err)
	}
	if _, err := NewAuthorizer(AuthorizerConfig{
		Registry: unbound, Recorder: &recordingProvenance{},
	}); err == nil || !strings.Contains(err.Error(), "compatibility requires") {
		t.Fatalf("compatibility without a subject = %v", err)
	}
	if _, err := NewAuthorizer(AuthorizerConfig{Registry: bound}); !errors.Is(err, ErrAuthorizerUnavailable) {
		t.Fatalf("missing recorder = %v", err)
	}
	if _, err := NewAuthorizer(AuthorizerConfig{Recorder: &recordingProvenance{}}); !errors.Is(err, ErrAuthorizerUnavailable) {
		t.Fatalf("missing registry = %v", err)
	}
}

func TestAuthorizeDeliveryDerivesIdentityFromTheDestinationAlone(t *testing.T) {
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
	recorder := &recordingProvenance{}
	authorizer := mustAuthorizer(t, AuthorizerConfig{
		Registry: mustRegistry(t, registryJSON(t, "tenant-a", []Revision{alpha, beta}), ModeStrict),
		Recorder: recorder,
		Clock:    func() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) },
	})
	if authorizer.Mode() != ModeStrict {
		t.Fatalf("Mode = %q", authorizer.Mode())
	}

	for _, revision := range []Revision{alpha, beta} {
		if err := decideFor(authorizer, revision); err != nil {
			t.Fatalf("Decide(%s): %v", revision.ArtifactID, err)
		}
	}
	decisions := recorder.all()
	if len(decisions) != 2 {
		t.Fatalf("recorded %d decisions, want 2", len(decisions))
	}
	if decisions[0].Subject != "alpha-client" || decisions[1].Subject != "beta-client" {
		t.Fatalf("subjects = %q, %q", decisions[0].Subject, decisions[1].Subject)
	}
	for index, decision := range decisions {
		if !decision.Authorized || decision.DenialCode != "" {
			t.Fatalf("decision %d = %#v", index, decision)
		}
		if decision.GrantedRole != authorization.DestinationClientGrant {
			t.Fatalf("decision %d granted role = %q", index, decision.GrantedRole)
		}
		if decision.AuthMethod != AuthMethodClientIdentity {
			t.Fatalf("decision %d auth method = %q", index, decision.AuthMethod)
		}
		if decision.DecidedAt.IsZero() || decision.Mode != ModeStrict {
			t.Fatalf("decision %d provenance = %#v", index, decision)
		}
	}
	if decisions[0].DestinationDigestVerified != alpha.Digest ||
		decisions[1].DestinationDigestVerified != beta.Digest {
		t.Fatal("recorded digests do not name the destination each dispatch resolved")
	}
	// Neither decision may name the other identity.
	if strings.Contains(decisions[0].Subject, "beta") || strings.Contains(decisions[1].Subject, "alpha") {
		t.Fatal("a destination was authorized under another destination's identity")
	}
}

func TestAuthorizeDeliveryDeniesUnknownUnverifiedAndUnboundDestinations(t *testing.T) {
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
	recorder := &recordingProvenance{}
	authorizer := mustAuthorizer(t, AuthorizerConfig{
		Registry: mustRegistry(t, registryJSON(t, "tenant-a", []Revision{alpha, beta}), ModeStrict),
		Recorder: recorder,
	})

	orphan := requestFor(alpha)
	orphan.reference.ArtifactID = "dest-orphan"
	crossed := requestFor(beta)
	crossed.reference.Digest = alpha.Digest
	otherTenant := requestFor(alpha)
	otherTenant.tenantID = "tenant-b"

	cases := map[string]struct {
		request decideRequest
		code    string
	}{
		"orphan":       {request: orphan, code: RefusalForbidden},
		"crossed":      {request: crossed, code: RefusalUnverified},
		"other tenant": {request: otherTenant, code: RefusalForbidden},
	}
	for name, test := range cases {
		err := authorizer.Decide(
			context.Background(), test.request.tenantID, test.request.attemptID, test.request.reference)
		if !errors.Is(err, ErrDeliveryRefused) {
			t.Fatalf("Decide(%s) = %v, want refused", name, err)
		}
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.DeliveryRefusalCode() != test.code {
			t.Fatalf("Decide(%s) refusal = %#v, want code %q", name, refusal, test.code)
		}
		if refusal.DeliveryRefusalDetail() == "" {
			t.Fatalf("Decide(%s) refusal detail is empty", name)
		}
	}
	for _, decision := range recorder.all() {
		if decision.Authorized || decision.DenialCode == "" || decision.GrantedRole != "" {
			t.Fatalf("denied decision = %#v", decision)
		}
	}
	if len(recorder.all()) != len(cases) {
		t.Fatalf("recorded %d denials, want %d", len(recorder.all()), len(cases))
	}
}

func TestAuthorizeDeliveryCompatibilityModeIssuesItsOwnGrant(t *testing.T) {
	t.Parallel()

	unbound := unboundKafkaRevision(t)
	recorder := &recordingProvenance{}
	authorizer := mustAuthorizer(t, AuthorizerConfig{
		Registry:             mustRegistry(t, registryJSON(t, "tenant-a", []Revision{unbound}), ModeCompatibility),
		Recorder:             recorder,
		CompatibilitySubject: "fallback-client",
	})
	if err := decideFor(authorizer, unbound); err != nil {
		t.Fatalf("compatibility Decide: %v", err)
	}
	decision := recorder.all()[0]
	if !decision.Authorized || decision.Mode != ModeCompatibility ||
		decision.Subject != "fallback-client" ||
		decision.AuthMethod != AuthMethodCompatibility ||
		decision.GrantedRole != authorization.DestinationCompatibilityGrant {
		t.Fatalf("compatibility decision = %#v", decision)
	}
	if decision.EndpointAdvisory != "integration.delivery.v1" {
		t.Fatalf("advisory endpoint = %q", decision.EndpointAdvisory)
	}
}

func TestAuthorizeDeliverySurfacesRecorderFailuresAsRetryable(t *testing.T) {
	t.Parallel()

	recorder := &recordingProvenance{err: errors.New("provenance write failed")}
	authorizer := mustAuthorizer(t, AuthorizerConfig{
		Registry: mustRegistry(t, registryJSON(t, "tenant-a", []Revision{mustHTTPSRevision(t)}), ModeStrict),
		Recorder: recorder,
	})
	request := requestFor(mustHTTPSRevision(t))
	request.reference.ArtifactID = "dest-orphan"
	err := authorizer.Decide(context.Background(), request.tenantID, request.attemptID, request.reference)
	if errors.Is(err, ErrDeliveryRefused) || err == nil {
		t.Fatalf("recorder failure = %v, want an infrastructure error rather than a refusal", err)
	}
}

func unboundKafkaRevision(t *testing.T) Revision {
	t.Helper()
	return mustRevision(t, RevisionInput{
		ArtifactID: "queue-primary", RevisionID: "destination-1", DestinationID: "queue-primary",
		Class: integration.DestinationClassProduction, Transport: TransportKafka,
		Kafka: &KafkaPolicy{Topic: "integration.delivery.v1"},
	})
}

func mustAuthorizer(t *testing.T, config AuthorizerConfig) *Authorizer {
	t.Helper()
	authorizer, err := NewAuthorizer(config)
	if err != nil {
		t.Fatalf("NewAuthorizer: %v", err)
	}
	return authorizer
}

// decideRequest mirrors the primitives the dispatch worker passes for one
// claimed work item.
type decideRequest struct {
	tenantID  string
	attemptID string
	reference integration.DestinationRevisionRef
}

func requestFor(revision Revision) decideRequest {
	return decideRequest{
		tenantID:  "tenant-a",
		attemptID: "attempt-" + revision.ArtifactID,
		reference: revision.Reference(),
	}
}

func decideFor(authorizer *Authorizer, revision Revision) error {
	request := requestFor(revision)
	return authorizer.Decide(context.Background(), request.tenantID, request.attemptID, request.reference)
}

type recordingProvenance struct {
	mu        sync.Mutex
	decisions []Decision
	err       error
}

func (p *recordingProvenance) RecordDecision(_ context.Context, decision Decision) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.decisions = append(p.decisions, decision)
	return nil
}

func (p *recordingProvenance) all() []Decision {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Decision(nil), p.decisions...)
}
