package resolvers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func TestParseFHIRSubscriptionAllowedHosts(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{name: "exact and suffix normalised", raw: " FHIR.Example.org , *.Partner.Example.COM. ", want: []string{"fhir.example.org", "*.partner.example.com"}},
		{name: "ip literals and localhost", raw: "127.0.0.1,::1,localhost", want: []string{"127.0.0.1", "::1", "localhost"}},
		{name: "duplicates collapse", raw: "a.example.org,A.example.org", want: []string{"a.example.org"}},
		{name: "scheme refused", raw: "https://fhir.example.org", wantErr: true},
		{name: "port refused", raw: "fhir.example.org:443", wantErr: true},
		{name: "path refused", raw: "fhir.example.org/r4", wantErr: true},
		{name: "userinfo refused", raw: "u@fhir.example.org", wantErr: true},
		{name: "bare star refused", raw: "*", wantErr: true},
		{name: "tld wildcard refused", raw: "*.org", wantErr: true},
		{name: "numeric suffix parses but never matches an ip (see CheckDestination)", raw: "*.0.0.1", want: []string{"*.0.0.1"}},
		{name: "ip literal wildcard refused", raw: "*.10.0.0.1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFHIRSubscriptionAllowedHosts(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFHIRSubscriptionPolicyCheckDestination(t *testing.T) {
	policy := FHIRSubscriptionPolicy{AllowedHosts: []string{
		"fhir.example.org", "*.partner.example.com", "*.0.0.1", "127.0.0.1",
	}}
	tests := []struct {
		name   string
		policy FHIRSubscriptionPolicy
		url    string
		allow  bool
	}{
		{name: "empty allowlist refuses", policy: DefaultFHIRSubscriptionPolicy(), url: "https://fhir.example.org/r4", allow: false},
		{name: "exact https", policy: policy, url: "https://fhir.example.org/r4", allow: true},
		{name: "exact is case-insensitive", policy: policy, url: "https://FHIR.example.org./r4", allow: true},
		{name: "exact with port", policy: policy, url: "https://fhir.example.org:8443/r4", allow: true},
		{name: "exact does not cover subdomain", policy: policy, url: "https://evil.fhir.example.org/r4", allow: false},
		{name: "suffix matches subdomain", policy: policy, url: "https://ehr.partner.example.com/fhir", allow: true},
		{name: "suffix matches deep subdomain", policy: policy, url: "https://a.b.partner.example.com/fhir", allow: true},
		{name: "suffix excludes apex", policy: policy, url: "https://partner.example.com/fhir", allow: false},
		{name: "suffix excludes lookalike", policy: policy, url: "https://evilpartner.example.com/fhir", allow: false},
		{name: "wildcard never matches ip literal", policy: policy, url: "https://10.0.0.1/fhir", allow: false},
		{name: "unlisted ip literal", policy: policy, url: "https://10.43.0.10/fhir", allow: false},
		{name: "localhost not listed", policy: policy, url: "https://localhost/fhir", allow: false},
		{name: "listed loopback over http", policy: policy, url: "http://127.0.0.1:8080/fhir", allow: true},
		{name: "http to non-loopback", policy: policy, url: "http://fhir.example.org/r4", allow: false},
		{name: "unsupported scheme", policy: policy, url: "ftp://fhir.example.org/r4", allow: false},
		{name: "user info", policy: policy, url: "https://u:p@fhir.example.org/r4", allow: false},
		{name: "userinfo host confusion", policy: policy, url: "https://fhir.example.org@10.0.0.1/r4", allow: false},
		{name: "relative", policy: policy, url: "/r4", allow: false},
		{name: "garbage", policy: policy, url: "::not a url", allow: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.policy.CheckDestination(tt.url)
			if tt.allow && err != nil {
				t.Fatalf("CheckDestination(%q) refused: %v", tt.url, err)
			}
			if !tt.allow && !errors.Is(err, ErrFHIRSubscriptionDestinationNotAllowed) {
				t.Fatalf("CheckDestination(%q) = %v, want refusal", tt.url, err)
			}
		})
	}
}

func fakeFHIRSubscriptionServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"resourceType":"Subscription","id":"sub-1","status":"requested","criteria":"Patient?","channel":{"type":"rest-hook","endpoint":"https://hook.example.org"}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCreateFhirSubscriptionRefusesUnlistedDestination is the regression test
// for SEC-2026-09-27-2: the default policy makes no request and returns the
// inventory-safe message verbatim.
func TestCreateFhirSubscriptionRefusesUnlistedDestination(t *testing.T) {
	var hits atomic.Int32
	server := fakeFHIRSubscriptionServer(t, &hits)
	mutation := &mutationResolver{NewResolver()}

	_, err := mutation.CreateFhirSubscription(context.Background(), model.CreateSubscriptionInput{
		Name: "refused", Server: server.URL, Criteria: "Patient?", Endpoint: "https://hook.example.org",
	})
	if err == nil || err.Error() != "FHIR subscription destination not allowed" {
		t.Fatalf("err = %v, want the inventory-safe refusal", err)
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Fatalf("refusal echoes the destination: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("refused destination received %d requests", hits.Load())
	}
}

func TestCreateFhirSubscriptionAllowsListedDestination(t *testing.T) {
	var hits atomic.Int32
	server := fakeFHIRSubscriptionServer(t, &hits)
	mutation := &mutationResolver{NewResolver(WithFHIRSubscriptionPolicy(FHIRSubscriptionPolicy{
		AllowedHosts: []string{"127.0.0.1"},
	}))}

	created, err := mutation.CreateFhirSubscription(context.Background(), model.CreateSubscriptionInput{
		Name: "allowed", Server: server.URL, Criteria: "Patient?", Endpoint: "https://hook.example.org",
	})
	if err != nil {
		t.Fatalf("CreateFhirSubscription: %v", err)
	}
	if created.ID != "sub-1" || hits.Load() != 1 {
		t.Fatalf("created %+v with %d requests", created, hits.Load())
	}
}

func TestSubscriptionClientRefusesRedirectOffAllowlist(t *testing.T) {
	var targetHits atomic.Int32
	target := fakeFHIRSubscriptionServer(t, &targetHits)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	// Same listener, but addressed as "localhost", which is not allowlisted.
	offList := "http://localhost:" + targetURL.Port()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, offList+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	mutation := &mutationResolver{NewResolver(WithFHIRSubscriptionPolicy(FHIRSubscriptionPolicy{
		AllowedHosts: []string{"127.0.0.1"},
	}))}
	_, err = mutation.CreateFhirSubscription(context.Background(), model.CreateSubscriptionInput{
		Name: "redirect", Server: redirector.URL, Criteria: "Patient?", Endpoint: "https://hook.example.org",
	})
	if !errors.Is(err, ErrFHIRSubscriptionDestinationNotAllowed) {
		t.Fatalf("err = %v, want the redirect refused by the destination policy", err)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("redirect target received %d requests", targetHits.Load())
	}
}

func TestSubscriptionClientCacheIsBoundedLRU(t *testing.T) {
	resolver := NewResolver(WithFHIRSubscriptionPolicy(FHIRSubscriptionPolicy{
		AllowedHosts: []string{"fhir.example.org"},
		MaxClients:   2,
	}))
	a, err := resolver.getOrCreateSubscriptionClient("https://fhir.example.org/a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.getOrCreateSubscriptionClient("https://fhir.example.org/b"); err != nil {
		t.Fatal(err)
	}
	// Touch /a so /b is the least recently used.
	if again, _ := resolver.getOrCreateSubscriptionClient("https://fhir.example.org/a"); again != a {
		t.Fatal("expected the cached client for /a")
	}
	if _, err := resolver.getOrCreateSubscriptionClient("https://fhir.example.org/c"); err != nil {
		t.Fatal(err)
	}
	if got := resolver.subscriptionClients.len(); got != 2 {
		t.Fatalf("cache holds %d clients, want 2", got)
	}
	if _, ok := resolver.subscriptionClients.get("https://fhir.example.org/b"); ok {
		t.Fatal("least recently used client /b was not evicted")
	}
	if _, ok := resolver.subscriptionClients.get("https://fhir.example.org/a"); !ok {
		t.Fatal("recently used client /a was evicted")
	}

	// Refused destinations never occupy a slot.
	for i := 0; i < 10; i++ {
		if _, err := resolver.getOrCreateSubscriptionClient("https://other.example.org/x"); err == nil {
			t.Fatal("unlisted destination accepted")
		}
	}
	if got := resolver.subscriptionClients.len(); got != 2 {
		t.Fatalf("cache holds %d clients after refusals, want 2", got)
	}

	if DefaultFHIRSubscriptionPolicy().maxClients() != DefaultFHIRSubscriptionMaxClients {
		t.Fatal("default client bound drifted")
	}
}
