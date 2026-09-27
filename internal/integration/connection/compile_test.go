package connection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Digests the destination package pins as constants
// (internal/integration/destination/revision_digest_pin_test.go). Restated
// here rather than exported from a test file.
const (
	pinnedHTTPSRevisionDigest = "sha256:9fdfcafa70ea89ffe1ec7aacb4acbf28a918cde02060e35bc2e197004be74f63"
	pinnedKafkaRevisionDigest = "sha256:4ac293d946b0091f08a9589ff3bdad601e68167b2ce54d49ace95f7dcceebfd5"
)

// TestCompiledDocumentIsTheDocumentAnOperatorWouldHandWrite pins .loom/38
// Decision 2: compiling a catalog spec with the existing constructor produces
// exactly the document `serve` already mounts. Given the identifiers the
// checked-in golden documents use, the compiled MLLP and S3 sources reproduce
// those documents member for member and digest for digest, and the https and
// kafka destinations reproduce the digests the destination package pins.
func TestCompiledDocumentIsTheDocumentAnOperatorWouldHandWrite(t *testing.T) {
	golden := func(name string) string {
		return filepath.Join("..", "..", "..", "testdata", "golden", "integration", name, "source-revision.json")
	}
	cases := []struct {
		kind       Kind
		artifactID string
		revisionID string
		goldenPath string
		digest     string
	}{
		{KindMLLP, "source-adt-mllp", "source-v1", golden("adt-mllp"), ""},
		{KindBatchS3, "source-adt-batch-s3", "source-v1", golden("adt-batch-s3"), ""},
		{KindHTTPS, "dest-https-pinned", "destination-1", "", pinnedHTTPSRevisionDigest},
		{KindKafka, "dest-kafka-pinned", "destination-1", "", pinnedKafkaRevisionDigest},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			fixture := loadSpecFixture(t, tc.kind)
			spec, problems := decodeAndCheck(tc.kind, fixture.specJSON(t), fixture.SecretBindings)
			if HasBlocking(problems) {
				t.Fatalf("fixture has problems: %+v", problems)
			}
			document, digest, err := constructDocument(tc.kind, spec, tc.artifactID, tc.revisionID)
			if err != nil {
				t.Fatalf("constructDocument: %v", err)
			}
			want := tc.digest
			if tc.goldenPath != "" {
				raw, err := os.ReadFile(tc.goldenPath)
				if err != nil {
					t.Fatalf("read golden document: %v", err)
				}
				want = decodedDigest(t, tc.kind, raw)
				if !sameJSON(t, raw, document) {
					t.Fatalf("compiled document differs from the golden document:\n got  %s\n want %s", document, raw)
				}
			}
			if digest != want {
				t.Fatalf("compiled digest %s, want %s", digest, want)
			}
		})
	}
}

func decodedDigest(t *testing.T, kind Kind, raw []byte) string {
	t.Helper()
	digest, err := DocumentDigest(kind, raw)
	if err != nil {
		t.Fatalf("decode golden %s document: %v", kind, err)
	}
	return digest
}

func sameJSON(t *testing.T, left, right []byte) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(left, &a); err != nil {
		t.Fatalf("decode left: %v", err)
	}
	if err := json.Unmarshal(right, &b); err != nil {
		t.Fatalf("decode right: %v", err)
	}
	return reflect.DeepEqual(a, b)
}

// TestBuildRevisionStoresTheExactBytesTheDecoderAccepts: each compiled
// document decodes with the kind's existing Decode function — the one serve
// mounts it with — to the digest the revision records, and the revision
// records the draft version it was compiled from.
func TestBuildRevisionStoresTheExactBytesTheDecoderAccepts(t *testing.T) {
	created := integration.AuditEnvelope{
		TenantID: "tenant-a", Reason: "compile fixture",
		Principal:  integration.Principal{ID: "engineer", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{ReadRole, WriteRole}},
		OccurredAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	}
	decoders := map[Kind]func([]byte) (string, error){
		KindMLLP: func(raw []byte) (string, error) {
			revision, err := mllp.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindBatchS3: func(raw []byte) (string, error) {
			revision, err := batch.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindBatchSFTP: func(raw []byte) (string, error) {
			revision, err := batch.DecodeSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindHTTP: func(raw []byte) (string, error) {
			revision, err := DecodeHTTPSourceRevision(bytes.NewReader(raw))
			return revision.Digest, err
		},
		KindHTTPS: destinationDigest, KindFHIR: destinationDigest, KindKafka: destinationDigest,
	}
	for _, kind := range Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			draft := loadSpecFixture(t, kind).draft(t, "fixture-connection")
			draft.Version = 7
			revision, problems := BuildRevision(draft, 3, created)
			if revision == nil {
				t.Fatalf("BuildRevision: %+v", problems)
			}
			if revision.RevisionID != "3" || revision.Number != 3 || revision.ArtifactID != "fixture-connection" ||
				revision.CompiledFromVersion != 7 || revision.Kind != kind || revision.Created.Reason != "compile fixture" {
				t.Fatalf("revision identity = %+v", revision)
			}
			digest, err := decoders[kind](revision.Document)
			if err != nil || digest != revision.Digest {
				t.Fatalf("existing decoder: digest %q err %v, want %q", digest, err, revision.Digest)
			}
			var document map[string]any
			if err := json.Unmarshal(revision.Document, &document); err != nil {
				t.Fatalf("document is not JSON: %v", err)
			}
			if document["artifact_id"] != "fixture-connection" || document["revision_id"] != "3" {
				t.Fatalf("document identifiers = %v/%v", document["artifact_id"], document["revision_id"])
			}
		})
	}
}

func destinationDigest(raw []byte) (string, error) {
	revision, err := destination.DecodeRevision(bytes.NewReader(raw))
	return revision.Digest, err
}

func TestBuildRevisionWritesNothingForABlockingProblem(t *testing.T) {
	draft := loadSpecFixture(t, KindMLLP).draft(t, "adt-mllp")
	draft.Spec = json.RawMessage(`{"source_id":"adt-east"}`)
	revision, problems := BuildRevision(draft, 1, integration.AuditEnvelope{})
	if revision != nil || !HasBlocking(problems) {
		t.Fatalf("revision = %+v, problems = %+v", revision, problems)
	}
	for _, path := range []string{"listen_address", "encoding", "timeouts", "tls", "clients", "acknowledgements",
		"max_message_bytes", "max_connections"} {
		if !hasCode(problems, CodeRequired, path) {
			t.Errorf("no REQUIRED problem at %s: %+v", path, problems)
		}
	}
}

func TestUnusedBindingIsAWarningThatDoesNotBlockCompile(t *testing.T) {
	fixture := loadSpecFixture(t, KindKafka)
	fixture.SecretBindings = []integration.SecretBinding{binding("left-over")}
	draft := fixture.draft(t, "dest-kafka")
	revision, problems := BuildRevision(draft, 1, integration.AuditEnvelope{})
	if revision == nil {
		t.Fatalf("an unused binding blocked compile: %+v", problems)
	}
	if !hasCode(problems, CodeUnusedBinding, "secret_bindings[0].name") || HasBlocking(problems) {
		t.Fatalf("problems = %+v, want exactly one UNUSED_BINDING warning", problems)
	}
}

func TestCheckSpecBindingRules(t *testing.T) {
	fixture := loadSpecFixture(t, KindHTTPS)
	fixture.SecretBindings = []integration.SecretBinding{
		{Name: "pinned-token", Reference: integration.SecretReference{Provider: "keychain", Key: "k"}},
		{Name: "pinned-token", Reference: integration.SecretReference{Provider: integration.SecretProviderFile}},
		{Name: "", Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: "has space"}},
		{Name: "pinned-other", Reference: integration.SecretReference{Provider: integration.SecretProviderVault, Key: "k", Version: "v 1"}},
	}
	problems := CheckSpec(KindHTTPS, fixture.specJSON(t), fixture.SecretBindings)
	for _, want := range []struct{ code, path string }{
		{CodeInvalidEnum, "secret_bindings[0].provider"},
		{CodeDuplicate, "secret_bindings[1].name"},
		{CodeRequired, "secret_bindings[1].key"},
		{CodeRequired, "secret_bindings[2].name"},
		{CodeInvalidValue, "secret_bindings[2].key"},
		{CodeInvalidValue, "secret_bindings[3].version"},
		{CodeUnboundSecret, "https.ca_bundle_binding"},
		{CodeUnusedBinding, "secret_bindings[3].name"},
	} {
		if !hasCode(problems, want.code, want.path) {
			t.Errorf("no %s at %s in %+v", want.code, want.path, problems)
		}
	}
}

// writeGate runs the draft write gate on a spec the way Service.Create does.
func writeGate(t *testing.T, kind Kind, spec string, bindings []integration.SecretBinding) []Problem {
	t.Helper()
	tree, ok := decodeSpecTree(json.RawMessage(spec), &checker{})
	if !ok {
		t.Fatalf("spec is not one JSON object: %s", spec)
	}
	return writeProblems(kind, tree, bindings)
}

// TestCheckSpecRefusesSecretValues: secret material is refused wherever it
// sits, by validate and compile and by the draft write alike — a key the kind
// does not define whose name suggests a value (also spelled with `-` or in
// camelCase, and under a key the kind does not know), a `*_binding` member
// holding more than a name, PEM material in any string, and a URL carrying
// credentials or a key, token, or signature parameter. Each is reported once,
// at its path.
func TestCheckSpecRefusesSecretValues(t *testing.T) {
	httpsSpec := func(url string) string {
		return `{"destination_id":"d","class":"production","https":{"url":"` + url + `","method":"POST","token_binding":"t"}}`
	}
	fhirSpec := func(url string) string {
		return `{"destination_id":"d","class":"production","fhir":{"base_url":"` + url + `","token_binding":"t"}}`
	}
	cases := []struct {
		name string
		kind Kind
		spec string
		path string
	}{
		{"token", KindHTTPS, `{"destination_id":"d","class":"production","https":{"url":"https://x.example","method":"POST","token_binding":"t","token":"abc"}}`, "https.token"},
		{"secret", KindHTTPS, `{"destination_id":"d","secret":"s"}`, "secret"},
		{"password at top level", KindMLLP, `{"password":"hunter2"}`, "password"},
		{"pwd", KindMLLP, `{"pwd":"hunter2"}`, "pwd"},
		{"client secret", KindMLLP, `{"client_secret":"s"}`, "client_secret"},
		{"passphrase", KindMLLP, `{"passphrase":"p"}`, "passphrase"},
		{"private key", KindMLLP, `{"tls":{"private_key":"k"}}`, "tls.private_key"},
		{"api key in camel case", KindMLLP, `{"apiKey":"k"}`, "apiKey"},
		{"api key with hyphens", KindHTTPS, `{"https":{"x-api-key":"k"}}`, "https.x-api-key"},
		{"access key", KindBatchS3, `{"s3":{"access_key":"k"}}`, "s3.access_key"},
		{"access token in camel case", KindHTTPS, `{"https":{"accessToken":"k"}}`, "https.accessToken"},
		{"authorization header", KindHTTPS, `{"https":{"authorization":"Bearer synthetic"}}`, "https.authorization"},
		{"auth", KindKafka, `{"kafka":{"topic":"t","auth":"u:p"}}`, "kafka.auth"},
		{"bearer", KindFHIR, `{"fhir":{"bearer":"synthetic"}}`, "fhir.bearer"},
		{"credential nested under an unknown key", KindMLLP, `{"extra":{"credentials":{"user":"u"}}}`, "extra.credentials"},
		{"inside a list", KindMLLP, `{"clients":{"identities":[{"subject":"s","secret":"x"}]}}`, "clients.identities[0].secret"},
		{"inside a container of the wrong type", KindMLLP, `{"timeouts":[{"password":"x"}]}`, "timeouts[0].password"},
		{"PEM material", KindMLLP, `{"tls":{"server_certificate_binding":"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"}}`, "tls.server_certificate_binding"},
		{"object under a binding field", KindHTTPS, `{"https":{"token_binding":{"value":"synthetic"}}}`, "https.token_binding"},
		{"number under a binding field", KindHTTPS, `{"https":{"token_binding":12345}}`, "https.token_binding"},
		{"object under an unknown binding key", KindHTTPS, `{"https":{"signing_binding":{"value":"synthetic"}}}`, "https.signing_binding"},
		{"token in the url query", KindHTTPS, httpsSpec("https://hooks.example.org/in?token=synthetic"), "https.url"},
		{"access token in the url query", KindHTTPS, httpsSpec("https://hooks.example.org/in?tenant=a&access_token=synthetic"), "https.url"},
		{"signature in the url query", KindHTTPS, httpsSpec("https://hooks.example.org/in?sv=1&sig=synthetic"), "https.url"},
		{"signed url", KindHTTPS, httpsSpec("https://hooks.example.org/in?X-Amz-Signature=synthetic"), "https.url"},
		{"key in the url query", KindHTTPS, httpsSpec("https://hooks.example.org/in?key=synthetic"), "https.url"},
		{"subscription key in the url query", KindHTTPS, httpsSpec("https://hooks.example.org/in?subscription-key=synthetic"), "https.url"},
		{"token in the url fragment", KindHTTPS, httpsSpec("https://hooks.example.org/in#access_token=synthetic"), "https.url"},
		{"token behind a bad escape", KindHTTPS, httpsSpec("https://hooks.example.org/%zz?token=synthetic"), "https.url"},
		{"credentials in the url", KindHTTPS, httpsSpec("https://user:synthetic@hooks.example.org/in"), "https.url"},
		{"token in the fhir base url", KindFHIR, fhirSpec("https://fhir.example.org/r4?_token=synthetic"), "fhir.base_url"},
		{"token in a uri san", KindMLLP, `{"clients":{"identities":[{"subject":"s","uri_san":"spiffe://example.org/a?token=synthetic"}]}}`, "clients.identities[0].uri_san"},
		{"token in a url under an unknown key", KindHTTPS, `{"callback":{"url":"https://x.example/?apikey=synthetic"}}`, "callback.url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckSpec(tc.kind, json.RawMessage(tc.spec), nil)
			if !hasCode(problems, CodeSecretValueForbidden, tc.path) {
				t.Fatalf("no SECRET_VALUE_FORBIDDEN at %s: %+v", tc.path, problems)
			}
			if at := problemsAt(problems, tc.path); len(at) != 1 {
				t.Fatalf("the secret at %s was reported %d times: %+v", tc.path, len(at), at)
			}
			if got := writeGate(t, tc.kind, tc.spec, nil); !hasCode(got, CodeSecretValueForbidden, tc.path) {
				t.Fatalf("the write gate missed %s: %+v", tc.path, got)
			}
			if strings.Contains(fmt.Sprint(problems), "synthetic") {
				t.Fatalf("a problem echoes the secret value: %+v", problems)
			}
		})
	}
	for _, allowed := range []struct {
		kind     Kind
		spec     string
		bindings []integration.SecretBinding
	}{
		{KindHTTP, `{"credential_binding":"c","auth_mode":"bearer"}`, declared("c")},
		{KindHTTP, `{"auth_mode":"oauth2","oauth":{"issuer_url":"https://issuer.example.org/realm","audience":"a"}}`, nil},
		{KindBatchS3, `{"s3":{"secret_access_key_binding":"s","access_key_binding":"a"}}`, declared("s", "a")},
		{KindBatchSFTP, `{"sftp":{"private_key_passphrase_binding":"p","password_binding":"q"}}`, declared("p", "q")},
		{KindHTTPS, `{"https":{"token_binding":"t","url":"https://hooks.example.org/in?tenant=a&format=hl7"}}`, declared("t")},
		{KindHTTPS, `{"https":{"token_binding":null}}`, nil},
		{KindMLLP, `{"clients":{"identities":[{"subject":"s","uri_san":"spiffe://example.org/ns/lab-east"}]}}`, nil},
	} {
		if problems := writeGate(t, allowed.kind, allowed.spec, allowed.bindings); len(problems) != 0 {
			t.Errorf("%s refused at write: %+v", allowed.spec, problems)
		}
		if problems := CheckSpec(allowed.kind, json.RawMessage(allowed.spec), allowed.bindings); hasAnyCode(problems, CodeSecretValueForbidden) {
			t.Errorf("%s reported as secret material: %+v", allowed.spec, problems)
		}
	}
}

// declared returns well-formed environment bindings with the given names.
func declared(names ...string) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, integration.SecretBinding{Name: name, Reference: integration.SecretReference{
			Provider: integration.SecretProviderEnvironment, Key: "FI_FHIR_TEST_" + strings.ToUpper(name),
		}})
	}
	return bindings
}

// TestWriteGateRefusesWhatADraftMayNeverPersist: a draft may be incomplete,
// out of range, or hold a scalar of the wrong type, but it never stores a key
// its kind does not define — even inside a container of the wrong type — a
// malformed secret binding reference, or a binding field that does not name
// one of its declared bindings. Every problem the gate raises is one validate
// reports too.
func TestWriteGateRefusesWhatADraftMayNeverPersist(t *testing.T) {
	ref := func(name, provider, key, version string) integration.SecretBinding {
		return integration.SecretBinding{Name: name, Reference: integration.SecretReference{
			Provider: integration.SecretProviderKind(provider), Key: key, Version: version,
		}}
	}
	cases := []struct {
		name     string
		kind     Kind
		spec     string
		bindings []integration.SecretBinding
		code     string
		path     string
	}{
		{"unknown top-level key", KindKafka, `{"destination_id":"d","colour":"blue"}`, nil, CodeUnknownField, "colour"},
		{"unknown nested key", KindMLLP, `{"tls":{"mode":"disabled","sni":"x"}}`, nil, CodeUnknownField, "tls.sni"},
		{"unknown key in a list element", KindMLLP, `{"clients":{"identities":[{"subject":"s","note":"x"}]}}`, nil, CodeUnknownField, "clients.identities[0].note"},
		{"key the kind defines elsewhere", KindKafka, `{"https":{"url":"https://x.example"}}`, nil, CodeUnknownField, "https"},
		{"object where a string belongs", KindKafka, `{"destination_id":{"note":"x"}}`, nil, CodeInvalidType, "destination_id"},
		{"list where an object belongs", KindMLLP, `{"timeouts":[{"read_seconds":5}]}`, nil, CodeInvalidType, "timeouts"},
		{"object inside a list of strings", KindMLLP, `{"clients":{"allowed_cidrs":[{"note":"x"}]}}`, nil, CodeInvalidType, "clients.allowed_cidrs[0]"},
		{"binding without a name", KindKafka, `{}`, []integration.SecretBinding{ref("", "env", "K", "")}, CodeRequired, "secret_bindings[0].name"},
		{"binding name with whitespace", KindKafka, `{}`, []integration.SecretBinding{ref("a b", "env", "K", "")}, CodeInvalidValue, "secret_bindings[0].name"},
		{"binding names repeated", KindKafka, `{}`, []integration.SecretBinding{ref("a", "env", "K", ""), ref("a", "env", "L", "")}, CodeDuplicate, "secret_bindings[1].name"},
		{"binding provider unknown", KindKafka, `{}`, []integration.SecretBinding{ref("a", "keychain", "K", "")}, CodeInvalidEnum, "secret_bindings[0].provider"},
		{"binding provider missing", KindKafka, `{}`, []integration.SecretBinding{ref("a", "", "K", "")}, CodeRequired, "secret_bindings[0].provider"},
		{"binding key missing", KindKafka, `{}`, []integration.SecretBinding{ref("a", "vault", "", "")}, CodeRequired, "secret_bindings[0].key"},
		{"binding key with whitespace", KindKafka, `{}`, []integration.SecretBinding{ref("a", "vault", "has space", "")}, CodeInvalidValue, "secret_bindings[0].key"},
		{"binding key too long", KindKafka, `{}`, []integration.SecretBinding{ref("a", "vault", strings.Repeat("k", 257), "")}, CodeInvalidValue, "secret_bindings[0].key"},
		{"binding version with a control character", KindKafka, `{}`, []integration.SecretBinding{ref("a", "vault", "k", "v\x01")}, CodeInvalidValue, "secret_bindings[0].version"},
		{"PEM in a binding key", KindKafka, `{}`, []integration.SecretBinding{ref("a", "file", "-----BEGIN", "")}, CodeSecretValueForbidden, "secret_bindings[0].key"},
		{"too many bindings", KindKafka, `{}`, bindingsOf(MaxSecretBindings + 1), CodeOutOfRange, "secret_bindings"},
		{"a credential pasted into a binding field", KindHTTPS, `{"https":{"token_binding":"pasted-credential-value"}}`,
			declared("https-token"), CodeUnboundSecret, "https.token_binding"},
		{"a binding field naming an undeclared binding", KindBatchSFTP,
			`{"sftp":{"known_hosts_binding":"sftp-known-hosts","private_key_binding":"sftp-client-key"}}`,
			declared("sftp-known-hosts"), CodeUnboundSecret, "sftp.private_key_binding"},
		{"a binding field naming a malformed binding", KindHTTPS, `{"https":{"token_binding":"a b"}}`,
			[]integration.SecretBinding{ref("a b", "env", "K", "")}, CodeUnboundSecret, "https.token_binding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refused := writeGate(t, tc.kind, tc.spec, tc.bindings)
			if !hasCode(refused, tc.code, tc.path) {
				t.Fatalf("the write gate did not refuse %s at %s: %+v", tc.code, tc.path, refused)
			}
			checked := CheckSpec(tc.kind, json.RawMessage(tc.spec), tc.bindings)
			for _, problem := range refused {
				if !hasCode(checked, problem.Code, problem.Path) {
					t.Fatalf("validate does not report the write refusal %+v: %+v", problem, checked)
				}
			}
			if strings.Contains(fmt.Sprint(refused, checked), "pasted-credential-value") {
				t.Fatalf("a problem repeats the value of a binding field: %+v", refused)
			}
		})
	}
	for name, spec := range map[string]string{
		"an incomplete draft":             `{"destination_id":"d"}`,
		"an empty draft":                  `{}`,
		"a value out of range":            `{"kafka":{"topic":"` + strings.Repeat("t", 300) + `"}}`,
		"a scalar of the wrong type":      `{"destination_id":42,"kafka":"topic"}`,
		"a JSON null for an optional one": `{"identity":null}`,
	} {
		if problems := writeGate(t, KindKafka, spec, nil); len(problems) != 0 {
			t.Errorf("%s was refused at write: %+v", name, problems)
		}
	}
	for name, tc := range map[string]struct {
		spec     string
		bindings []integration.SecretBinding
	}{
		"a binding field naming a declared binding": {`{"https":{"token_binding":"https-token"}}`, declared("https-token")},
		"an empty binding field":                    {`{"https":{"token_binding":""}}`, nil},
		"a declared binding no field names yet":     {`{"destination_id":"d"}`, declared("https-token")},
	} {
		if problems := writeGate(t, KindHTTPS, tc.spec, tc.bindings); len(problems) != 0 {
			t.Errorf("%s was refused at write: %+v", name, problems)
		}
	}
}

func bindingsOf(count int) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, count)
	for index := range count {
		bindings = append(bindings, integration.SecretBinding{Name: fmt.Sprintf("binding-%d", index),
			Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: fmt.Sprintf("KEY_%d", index)}})
	}
	return bindings
}

func hasAnyCode(problems []Problem, code string) bool {
	for _, problem := range problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}

func TestCheckSpecReportsEveryStructuralProblemWithItsPath(t *testing.T) {
	spec := `{
		"source_id": "adt-east",
		"listen_address": 2575,
		"encoding": "utf-8",
		"timeouts": "fast",
		"tls": {"mode": "disabled", "sni": "x"},
		"clients": {"allowed_cidrs": "10.0.0.0/8"},
		"acknowledgements": {"mode": "commit", "include_error_segment": "yes"},
		"max_message_bytes": 1.5,
		"max_connections": 10,
		"colour": "blue"
	}`
	problems := CheckSpec(KindMLLP, json.RawMessage(spec), nil)
	for _, want := range []struct{ code, path string }{
		{CodeInvalidType, "listen_address"},
		{CodeInvalidType, "timeouts"},
		{CodeUnknownField, "tls.sni"},
		{CodeInvalidType, "clients.allowed_cidrs"},
		{CodeInvalidType, "acknowledgements.include_error_segment"},
		{CodeInvalidType, "max_message_bytes"},
		{CodeUnknownField, "colour"},
	} {
		if !hasCode(problems, want.code, want.path) {
			t.Errorf("no %s at %s in %+v", want.code, want.path, problems)
		}
	}
	// One mistake is one finding: a value of the wrong type is not also
	// reported as a missing field.
	if len(problemsAt(problems, "timeouts")) != 1 || len(problemsAt(problems, "listen_address")) != 1 {
		t.Fatalf("a type error was reported more than once: %+v", problems)
	}
}

func TestCheckSpecRefusesMalformedDocuments(t *testing.T) {
	for name, tc := range map[string]struct {
		spec string
		code string
		path string
	}{
		"empty":           {"", CodeRequired, ""},
		"null":            {"null", CodeRequired, ""},
		"array":           {`[1]`, CodeInvalidJSON, ""},
		"not json":        {`{"a":`, CodeInvalidJSON, ""},
		"duplicate key":   {`{"source_id":"a","source_id":"b"}`, CodeInvalidJSON, "source_id"},
		"nested dup":      {`{"tls":{"mode":"disabled","mode":"mutual"}}`, CodeInvalidJSON, "tls.mode"},
		"trailing value":  {`{} {}`, CodeInvalidJSON, ""},
		"oversized value": {`{"source_id":"` + strings.Repeat("a", MaxSpecBytes) + `"}`, CodeOutOfRange, ""},
	} {
		t.Run(name, func(t *testing.T) {
			problems := CheckSpec(KindMLLP, json.RawMessage(tc.spec), nil)
			if !hasCode(problems, tc.code, tc.path) {
				t.Fatalf("no %s at %q in %+v", tc.code, tc.path, problems)
			}
		})
	}
	if problems := CheckSpec(Kind("smtp"), json.RawMessage(`{}`), nil); !hasCode(problems, CodeInvalidEnum, "") {
		t.Fatalf("unknown kind: %+v", problems)
	}
}
