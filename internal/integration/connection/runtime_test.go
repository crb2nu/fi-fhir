package connection

import (
	"strings"
	"testing"
)

func int64Pointer(value int64) *int64 { return &value }
func boolPointer(value bool) *bool    { return &value }

func describedRuntime() *RuntimeDescription {
	description := &RuntimeDescription{
		Version: "test", TenantID: "tenant-a", ReplicaID: "host-1",
		Registry: RuntimeRegistry{IntegrationCount: 1, Integrations: []RuntimeRegistryIntegration{{
			IntegrationID: "adt-east", DefinitionID: "integration-adt", RevisionID: "definition-revision-1",
			Digest: "sha256:" + strings.Repeat("d", 64), SourceID: "adt-east", Format: "hl7v2",
		}}},
		Ledgers:    []RuntimeLedger{{Name: "connection", Version: SchemaVersion}},
		Properties: []RuntimeProperty{NewRuntimeProperty("FI_FHIR_DEPLOYMENT_TENANT_ID", false, "tenant-a", true, "")},
	}
	description.Adapters = [4]RuntimeAdapter{
		{Kind: AdapterHTTP, Enabled: true, DefinitionID: "integration-adt", IntegrationID: "adt-east",
			SourceDigest: "sha256:" + strings.Repeat("1", 64), Path: DefaultHTTPSourcePath},
		{Kind: AdapterMLLP, Enabled: true, DefinitionID: "adt-mllp", SourceDigest: "sha256:" + strings.Repeat("2", 64),
			ListenAddress: "0.0.0.0:2575", MaxConnections: int64Pointer(128), RequireClientIdentity: boolPointer(false)},
		{Kind: AdapterBatch, Enabled: false, SourceDigest: "sha256:" + strings.Repeat("3", 64)},
		{Kind: AdapterDelivery, Enabled: true, QueueDriver: "kafka", MaxAttempts: int64Pointer(5)},
	}
	description.DestinationIdentity = &RuntimeDestinationIdentity{Mode: "strict", Destinations: []RuntimeDestination{{
		ArtifactID: "dest-fhir", RevisionID: "1", Digest: "sha256:" + strings.Repeat("4", 64),
		Transport: "fhir", Class: "production", EndpointAdvisory: "https://fhir.example.org/r4",
	}}}
	return description
}

func TestNewRuntimePropertyRendersSecretsAsSetOrUnset(t *testing.T) {
	cases := []struct {
		name         string
		secret       bool
		value        string
		present      bool
		defaultValue string
		want         RuntimeProperty
	}{
		{"secret set", true, "a-real-looking-token-value", true, "", RuntimeProperty{Value: "set", Secret: true, Source: "env"}},
		{"secret unset", true, "", false, "", RuntimeProperty{Value: "unset", Secret: true, Source: "default"}},
		{"plain set", false, "oidc", true, "static", RuntimeProperty{Value: "oidc", Source: "env"}},
		{"plain default", false, "", false, "static", RuntimeProperty{Value: "static", Source: "default"}},
		{"plain without default", false, "", false, "", RuntimeProperty{Value: "", Source: "default"}},
		{"url credentials stripped", false, "https://user:pw@issuer.example.org/realm", true, "",
			RuntimeProperty{Value: "https://issuer.example.org/realm", Source: "env"}},
		{"url query and fragment stripped", false, "https://collector.example.org/v1?api_key=synthetic#sig=synthetic", true, "",
			RuntimeProperty{Value: "https://collector.example.org/v1", Source: "env"}},
		{"a value that is not a url is kept", false, "localhost:4317", true, "",
			RuntimeProperty{Value: "localhost:4317", Source: "env"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NewRuntimeProperty("KEY", tc.secret, tc.value, tc.present, tc.defaultValue)
			tc.want.Key = "KEY"
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if tc.secret && strings.Contains(got.Value, tc.value) && tc.value != "" {
				t.Fatal("a secret value reached the property")
			}
		})
	}
	long := NewRuntimeProperty("KEY", false, strings.Repeat("x", 600), true, "")
	if len(long.Value) > maxPropertyValueBytes+len("…") {
		t.Fatalf("value not bounded: %d bytes", len(long.Value))
	}
}

func TestEndpointAdvisoryDropsCredentialsQueryAndFragment(t *testing.T) {
	for raw, want := range map[string]string{
		"https://user:pw@destination.example.org/inbound?token=abc#x": "https://destination.example.org/inbound",
		"https://fhir.example.org/r4":                                 "https://fhir.example.org/r4",
		"integration.delivery.v1":                                     "integration.delivery.v1",
	} {
		if got := EndpointAdvisory(raw); got != want {
			t.Errorf("EndpointAdvisory(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestRuntimeDescriptionValidate(t *testing.T) {
	if err := describedRuntime().Validate(); err != nil {
		t.Fatalf("valid description refused: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeDescription){
		"adapters out of order": func(d *RuntimeDescription) { d.Adapters[0], d.Adapters[1] = d.Adapters[1], d.Adapters[0] },
		"secret with a value": func(d *RuntimeDescription) {
			d.Properties = append(d.Properties, RuntimeProperty{Key: "S", Value: "hunter2", Secret: true, Source: "env"})
		},
		"repeated property": func(d *RuntimeDescription) { d.Properties = append(d.Properties, d.Properties[0]) },
		"unknown source": func(d *RuntimeDescription) {
			d.Properties = append(d.Properties, RuntimeProperty{Key: "X", Source: "file"})
		},
		"registry count drift": func(d *RuntimeDescription) { d.Registry.IntegrationCount = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			description := describedRuntime()
			mutate(description)
			if description.Validate() == nil {
				t.Fatal("Validate accepted a broken description")
			}
		})
	}
	var missing *RuntimeDescription
	if missing.Validate() == nil {
		t.Fatal("Validate accepted a nil description")
	}
}

func TestRuntimeDescriptionCloneIsDeep(t *testing.T) {
	original := describedRuntime()
	clone := original.Clone()
	*clone.Adapters[1].MaxConnections = 1
	clone.Registry.Integrations[0].IntegrationID = "mutated"
	clone.DestinationIdentity.Destinations[0].Digest = "mutated"
	clone.Properties[0].Value = "mutated"
	if *original.Adapters[1].MaxConnections != 128 || original.Registry.Integrations[0].IntegrationID != "adt-east" ||
		original.DestinationIdentity.Destinations[0].Digest == "mutated" || original.Properties[0].Value == "mutated" {
		t.Fatal("Clone shares memory with the description it copied")
	}
}

func TestMountedDigestsAndRuntimeState(t *testing.T) {
	description := describedRuntime()
	mounted := description.MountedDigests()
	for digest, role := range map[string]string{
		"sha256:" + strings.Repeat("1", 64): RuntimeRoleHTTPIngress,
		"sha256:" + strings.Repeat("2", 64): RuntimeRoleMLLPListener,
		"sha256:" + strings.Repeat("4", 64): RuntimeRoleDeliveryRegistry,
	} {
		if mounted[digest].Role != role {
			t.Errorf("digest %s… role = %q, want %q", digest[:12], mounted[digest].Role, role)
		}
	}
	if _, ok := mounted["sha256:"+strings.Repeat("3", 64)]; ok {
		t.Fatal("a disabled adapter's digest reads as mounted")
	}

	revisions := []RevisionDigest{
		{ArtifactID: "adt-mllp", RevisionID: "3", Number: 3, Digest: "sha256:" + strings.Repeat("9", 64)},
		{ArtifactID: "adt-mllp", RevisionID: "2", Number: 2, Digest: "sha256:" + strings.Repeat("2", 64)},
	}
	state := runtimeStateFor(revisions, mounted)
	if !state.Mounted || state.Role != RuntimeRoleMLLPListener || !strings.HasPrefix(state.Detail, "revision 2: ") {
		t.Fatalf("state = %+v", state)
	}
	if state := runtimeStateFor(revisions[:1], mounted); state.Mounted || state.Role != "" || state.Detail != "" {
		t.Fatalf("an unmounted revision reads as mounted: %+v", state)
	}
	var none *RuntimeDescription
	if len(none.MountedDigests()) != 0 {
		t.Fatal("a nil description mounts something")
	}
}
