package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// documentedServeKeys reads every environment key `serve --help` documents.
// `X[_FILE]` documents two keys; `FI_FHIR_DATABASE_*` documents a family.
func documentedServeKeys(t *testing.T) (map[string]bool, []string) {
	t.Helper()
	keyPattern := regexp.MustCompile(`\b((?:FI_FHIR|TEMPORAL)_[A-Z0-9_]*[A-Z0-9])(\[_FILE\])?(_\*)?`)
	exact := make(map[string]bool)
	var families []string
	for _, match := range keyPattern.FindAllStringSubmatch(serveUsage, -1) {
		switch {
		case match[3] != "":
			families = append(families, match[1]+"_")
		case match[2] != "":
			exact[match[1]] = true
			exact[match[1]+"_FILE"] = true
		default:
			exact[match[1]] = true
		}
	}
	if len(exact) == 0 || len(families) == 0 {
		t.Fatalf("parsed no keys from serveUsage (exact %d, families %v)", len(exact), families)
	}
	return exact, families
}

// TestServePropertiesAreExactlyTheDocumentedKeys holds the allowlist closed in
// both directions: every key `serve --help` documents is reported, and nothing
// the help does not document is. A property added to the engine runtime
// without documentation, or a documented key the Engine tab would silently
// omit, fails here.
func TestServePropertiesAreExactlyTheDocumentedKeys(t *testing.T) {
	exact, families := documentedServeKeys(t)
	allowlisted := make(map[string]bool)
	for _, property := range serveProperties() {
		if allowlisted[property.key] {
			t.Fatalf("%s is allowlisted twice", property.key)
		}
		allowlisted[property.key] = true
		documented := exact[property.key]
		for _, family := range families {
			documented = documented || strings.HasPrefix(property.key, family)
		}
		if !documented {
			t.Errorf("%s is reported by engineRuntime but not documented by serve --help", property.key)
		}
	}
	var missing []string
	for key := range exact {
		if !allowlisted[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("serve --help documents keys engineRuntime does not report: %v", missing)
	}
	for _, family := range families {
		found := false
		for key := range allowlisted {
			found = found || strings.HasPrefix(key, family)
		}
		if !found {
			t.Errorf("the documented family %s* has no allowlisted member", family)
		}
	}
}

// TestServePropertiesMarkEveryCredentialSecret: a key whose name says it
// carries a credential, or the path to one, can only ever render as set/unset.
func TestServePropertiesMarkEveryCredentialSecret(t *testing.T) {
	credential := regexp.MustCompile(`TOKEN|SECRET|PASSWORD|_KEY(_|$)`)
	for _, property := range serveProperties() {
		if credential.MatchString(property.key) && !property.secret {
			t.Errorf("%s names a credential but is not secret", property.key)
		}
		if property.secret && property.defaultValue != "" {
			t.Errorf("%s is secret but carries a default value", property.key)
		}
	}
}

func TestDescribeServePropertiesNeverRendersASecretValue(t *testing.T) {
	const sentinel = "synthetic-secret-sentinel-value"
	for _, property := range serveProperties() {
		if property.secret {
			t.Setenv(property.key, sentinel)
		}
	}
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", "")
	t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", "https://operator:pw@issuer.example.org/realm")
	properties := describeServeProperties()
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(encoded, []byte(sentinel)) || bytes.Contains(encoded, []byte("operator:pw")) {
		t.Fatalf("a secret reached the properties: %s", encoded)
	}
	byKey := make(map[string]connection.RuntimeProperty, len(properties))
	for _, property := range properties {
		byKey[property.Key] = property
	}
	if got := byKey["FI_FHIR_GRAPHQL_BEARER_TOKEN"]; got.Value != "set" || !got.Secret || got.Source != "env" {
		t.Fatalf("bearer token property = %+v", got)
	}
	if got := byKey["FI_FHIR_GRAPHQL_AUTH_MODE"]; got.Value != "static" || got.Source != "default" {
		t.Fatalf("auth mode property = %+v", got)
	}
	if got := byKey["FI_FHIR_GRAPHQL_OIDC_ISSUER_URL"]; got.Value != "https://issuer.example.org/realm" {
		t.Fatalf("issuer URL property = %+v", got)
	}
}

// TestServePropertiesReportTheKeysServeReadsForSessionsIdentityAndSFTP pins
// the keys serve read before the Engine tab could show them, with their secret
// flags: a path to a credential is as secret as the credential.
func TestServePropertiesReportTheKeysServeReadsForSessionsIdentityAndSFTP(t *testing.T) {
	want := []struct {
		key    string
		secret bool
		family bool
	}{
		{key: "FI_FHIR_INTEGRATION_SESSION_ENABLED"},
		{key: "FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE", secret: true},
		{key: "FI_FHIR_DELIVERY_IDENTITY_MODE"},
		{key: "FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH"},
		{key: "FI_FHIR_DELIVERY_IDENTITY_COMPATIBILITY_SUBJECT"},
		{key: "FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR", secret: true},
		{key: "FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE", secret: true},
		{key: "FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE_FILE", secret: true},
		{key: connectionSecretEnvPrefix, secret: true, family: true},
	}
	allowlisted := make(map[string]serveProperty)
	for _, property := range serveProperties() {
		allowlisted[property.key] = property
	}
	for _, tc := range want {
		t.Run(tc.key, func(t *testing.T) {
			property, ok := allowlisted[tc.key]
			if !ok {
				t.Fatalf("%s is not allowlisted", tc.key)
			}
			if property.secret != tc.secret || property.family != tc.family {
				t.Fatalf("%s: secret=%t family=%t, want secret=%t family=%t",
					tc.key, property.secret, property.family, tc.secret, tc.family)
			}
		})
	}
}

// TestServePropertiesAllowlistExactlyOneFamily: the prefix rule exists for the
// connection secret env family and nothing else, and it is secret.
func TestServePropertiesAllowlistExactlyOneFamily(t *testing.T) {
	var families []string
	for _, property := range serveProperties() {
		if !property.family {
			continue
		}
		families = append(families, property.key)
		if !property.secret || property.defaultValue != "" {
			t.Errorf("family %s must be secret with no default", property.key)
		}
	}
	if len(families) != 1 || families[0] != connectionSecretEnvPrefix {
		t.Fatalf("allowlisted families = %v, want exactly [%s]", families, connectionSecretEnvPrefix)
	}
}

func TestDescribeServePropertiesRendersConnectionSecretsOnlyAsSet(t *testing.T) {
	const sentinel = "synthetic-connection-secret-value"
	t.Setenv("FI_FHIR_CONNECTION_SECRET_X", sentinel)
	t.Setenv("FI_FHIR_CONNECTION_SECRET_A", sentinel+"-a")
	t.Setenv("FI_FHIR_SOMETHING_ELSE", "unrelated-value")
	properties := describeServeProperties()
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(encoded, []byte(sentinel)) {
		t.Fatalf("a connection secret reached the properties: %s", encoded)
	}
	if bytes.Contains(encoded, []byte("FI_FHIR_SOMETHING_ELSE")) || bytes.Contains(encoded, []byte("unrelated-value")) {
		t.Fatalf("an undocumented key reached the properties: %s", encoded)
	}
	var members []connection.RuntimeProperty
	for _, property := range properties {
		if strings.HasPrefix(property.Key, connectionSecretEnvPrefix) {
			members = append(members, property)
		}
	}
	if len(members) != 2 || members[0].Key != "FI_FHIR_CONNECTION_SECRET_A" || members[1].Key != "FI_FHIR_CONNECTION_SECRET_X" {
		t.Fatalf("connection secret rows = %+v, want A then X", members)
	}
	for _, member := range members {
		if !member.Secret || member.Value != connection.PropertyValueSet || member.Source != connection.PropertySourceEnv {
			t.Fatalf("connection secret row = %+v", member)
		}
	}
	description := connection.RuntimeDescription{Adapters: [4]connection.RuntimeAdapter{
		{Kind: connection.AdapterOrder[0]}, {Kind: connection.AdapterOrder[1]},
		{Kind: connection.AdapterOrder[2]}, {Kind: connection.AdapterOrder[3]},
	}, Properties: properties}
	if err := description.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestDescribeServePropertiesRendersAnUnsetConnectionSecretFamily(t *testing.T) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, connectionSecretEnvPrefix) {
			t.Setenv(key, "")
		}
	}
	t.Setenv(connectionSecretEnvPrefix, "no-name-is-not-a-member")
	var members []connection.RuntimeProperty
	for _, property := range describeServeProperties() {
		if strings.HasPrefix(property.Key, connectionSecretEnvPrefix) {
			members = append(members, property)
		}
	}
	if len(members) != 1 || members[0].Key != connectionSecretEnvPrefix+"*" ||
		!members[0].Secret || members[0].Value != connection.PropertyValueUnset || members[0].Source != connection.PropertySourceDefault {
		t.Fatalf("connection secret rows = %+v, want one unset family row", members)
	}
}

func TestSchemaLedgersReportTheConnectionLedger(t *testing.T) {
	ledgers := schemaLedgers()
	if len(ledgers) != 7 {
		t.Fatalf("schemaLedgers() reports %d ledgers, want 7", len(ledgers))
	}
	found := false
	for _, ledger := range ledgers {
		if !observability.KnownSchemaLedger(ledger.Name) {
			t.Errorf("ledger %q is not a known metric label", ledger.Name)
		}
		if ledger.Name == observability.SchemaLedgerConnection {
			found = ledger.Version == connection.SchemaVersion
		}
	}
	if !found {
		t.Fatal("the connection ledger is not reported at its SchemaVersion")
	}
	var output bytes.Buffer
	printVersion(&output, "test")
	if !strings.Contains(output.String(), "connection") {
		t.Fatalf("fi-fhir version does not report the connection ledger:\n%s", output.String())
	}
}

// TestEngineRuntimeDescribesAPreviewOnlyReplica is the e2e stack's shape: the
// registry is loaded and no ingress, listener, runner, or worker is.
func TestEngineRuntimeDescribesAPreviewOnlyReplica(t *testing.T) {
	configureOperatorRuntimeTest(t)
	runtime, err := loadServeIntegrationRuntimeFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	description, err := buildEngineRuntimeDescription(runtime, engineRuntimeFacts{version: "v-test", host: "127.0.0.1", port: 3000})
	if err != nil {
		t.Fatalf("buildEngineRuntimeDescription: %v", err)
	}
	if description.TenantID != "tenant-a" || description.AuthMode != graphqlAuthModeStatic || description.ReplicaID == "" ||
		description.Registry.IntegrationCount != 1 || description.DestinationIdentity != nil {
		t.Fatalf("description = %+v", description)
	}
	for index, adapter := range description.Adapters {
		if adapter.Kind != connection.AdapterOrder[index] || adapter.Enabled || adapter.SourceDigest != "" ||
			adapter.PollSeconds != nil || adapter.RequireClientIdentity != nil {
			t.Fatalf("adapter %d = %+v, want a disabled row with nothing in it", index, adapter)
		}
	}
	if len(description.Ledgers) != 7 || len(description.Properties) != len(serveProperties()) {
		t.Fatalf("ledgers %d, properties %d", len(description.Ledgers), len(description.Properties))
	}
	if len(description.MountedDigests()) != 0 {
		t.Fatal("a replica with no adapter mounts a document")
	}
	encoded, _ := json.Marshal(description)
	if strings.Contains(string(encoded), "correct-horse-battery-staple") {
		t.Fatal("the bearer token reached the description")
	}
}

// TestEngineRuntimeDescribesEveryMountedAdapter drives the adapter rows from
// recorded composition facts. The adapters themselves need a database; the
// description reads only whether each one exists and what it was built from.
func TestEngineRuntimeDescribesEveryMountedAdapter(t *testing.T) {
	configureOperatorRuntimeTest(t)
	runtime, err := loadServeIntegrationRuntimeFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	summaries := runtime.composition.registry.Integrations()
	if len(summaries) != 1 {
		t.Fatalf("registry summaries = %+v", summaries)
	}
	mllpSource := mllp.SourceRevision{
		ArtifactID: "source-adt-mllp", RevisionID: "1", SourceID: "adt-east", ListenAddress: "0.0.0.0:2575",
		TLS: mllp.TLSPolicy{Mode: mllp.TLSModeMutual}, MaxMessageBytes: 1 << 20, MaxConnections: 128,
		Digest: "sha256:" + strings.Repeat("2", 64),
	}
	batchSource := integrationbatch.SourceRevision{
		ArtifactID: "source-adt-batch", RevisionID: "1", SourceID: "adt-west", Provider: integrationbatch.ProviderS3,
		PollSeconds: 15, MaxMessageBytes: 1 << 20, Digest: "sha256:" + strings.Repeat("3", 64),
	}
	runtime.ingressHandler = http.NotFoundHandler()
	runtime.ingressPath = "/v1/hl7v2"
	runtime.mllpServer = &mllp.Server{}
	runtime.batchRunner = &integrationbatch.Runner{}
	runtime.deliveryWorker = &integrationdelivery.Dispatcher{}
	runtime.composition.http = &httpIngressFacts{integrationID: summaries[0].IntegrationID, authMode: "bearer", maxBodyBytes: 4096}
	runtime.composition.mllp = &mllpFacts{source: mllpSource, definitionID: "adt-mllp", requireClientIdentity: true}
	runtime.composition.batch = &batchFacts{source: batchSource, definitionID: "adt-batch", workerID: "host-7", requireWorkloadIdentity: false}
	runtime.composition.delivery = &deliveryFacts{workerID: "host-7", maxAttempts: 5, queueDriver: "kafka"}

	description, err := buildEngineRuntimeDescription(runtime, engineRuntimeFacts{host: "0.0.0.0", port: 8080, controlPlane: true})
	if err != nil {
		t.Fatalf("buildEngineRuntimeDescription: %v", err)
	}
	httpAdapter, mllpAdapter, batchAdapter, deliveryAdapter := description.Adapters[0], description.Adapters[1], description.Adapters[2], description.Adapters[3]
	if !httpAdapter.Enabled || httpAdapter.IntegrationID != summaries[0].IntegrationID ||
		httpAdapter.SourceDigest != summaries[0].Source.Digest || httpAdapter.DefinitionID != summaries[0].IntegrationRevision.ArtifactID ||
		httpAdapter.ListenAddress != "0.0.0.0:8080" || *httpAdapter.MaxBodyBytes != 4096 || httpAdapter.AuthMode != "bearer" {
		t.Fatalf("http adapter = %+v", httpAdapter)
	}
	if !mllpAdapter.Enabled || mllpAdapter.SourceDigest != mllpSource.Digest || mllpAdapter.TLSMode != "mutual" ||
		*mllpAdapter.MaxConnections != 128 || !*mllpAdapter.RequireClientIdentity || mllpAdapter.DefinitionID != "adt-mllp" {
		t.Fatalf("mllp adapter = %+v", mllpAdapter)
	}
	if !batchAdapter.Enabled || batchAdapter.Provider != "s3" || *batchAdapter.PollSeconds != 15 ||
		*batchAdapter.RequireWorkloadIdentity || batchAdapter.WorkerID != "host-7" {
		t.Fatalf("batch adapter = %+v", batchAdapter)
	}
	if !deliveryAdapter.Enabled || deliveryAdapter.QueueDriver != "kafka" || *deliveryAdapter.MaxAttempts != 5 {
		t.Fatalf("delivery adapter = %+v", deliveryAdapter)
	}
	mounted := description.MountedDigests()
	if mounted[mllpSource.Digest].Role != connection.RuntimeRoleMLLPListener ||
		mounted[batchSource.Digest].Role != connection.RuntimeRoleBatchRunner ||
		mounted[summaries[0].Source.Digest].Role != connection.RuntimeRoleHTTPIngress {
		t.Fatalf("mounted digests = %+v", mounted)
	}
	if !description.ControlPlane {
		t.Fatal("control plane fact lost")
	}
}
