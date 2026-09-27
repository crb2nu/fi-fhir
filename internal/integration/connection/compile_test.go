package connection

import (
	"bytes"
	"encoding/json"
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

// TestCheckSpecRefusesSecretValues: a key whose name suggests a value is
// refused wherever it sits — including under a key the kind does not know,
// because a draft persists those — and so is PEM material in any string.
// Binding names are the one allowed spelling.
func TestCheckSpecRefusesSecretValues(t *testing.T) {
	cases := []struct {
		name string
		spec string
		path string
	}{
		{"token", `{"destination_id":"d","class":"production","https":{"url":"https://x.example","method":"POST","token_binding":"t","token":"abc"}}`, "https.token"},
		{"password at top level", `{"password":"hunter2"}`, "password"},
		{"client secret", `{"client_secret":"s"}`, "client_secret"},
		{"passphrase", `{"passphrase":"p"}`, "passphrase"},
		{"private key", `{"tls":{"private_key":"k"}}`, "tls.private_key"},
		{"api key", `{"apiKey":"k"}`, "apiKey"},
		{"credential nested under an unknown key", `{"extra":{"credentials":{"user":"u"}}}`, "extra.credentials"},
		{"inside a list", `{"clients":{"identities":[{"subject":"s","secret":"x"}]}}`, "clients.identities[0].secret"},
		{"PEM material", `{"tls":{"server_certificate_binding":"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"}}`, "tls.server_certificate_binding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind := KindMLLP
			if strings.Contains(tc.spec, "destination_id") {
				kind = KindHTTPS
			}
			problems := CheckSpec(kind, json.RawMessage(tc.spec), nil)
			if !hasCode(problems, CodeSecretValueForbidden, tc.path) {
				t.Fatalf("no SECRET_VALUE_FORBIDDEN at %s: %+v", tc.path, problems)
			}
			if hasCode(problems, CodeUnknownField, tc.path) {
				t.Fatalf("a secret-looking key was also reported as UNKNOWN_FIELD: %+v", problems)
			}
			if got := SecretValueProblems(json.RawMessage(tc.spec)); !hasCode(got, CodeSecretValueForbidden, tc.path) {
				t.Fatalf("the write-time scan missed %s: %+v", tc.path, got)
			}
		})
	}
	for _, allowed := range []string{
		`{"credential_binding":"c"}`,
		`{"s3":{"secret_access_key_binding":"s","access_key_binding":"a"}}`,
		`{"sftp":{"private_key_passphrase_binding":"p","password_binding":"q"}}`,
		`{"https":{"token_binding":"t"}}`,
	} {
		if problems := SecretValueProblems(json.RawMessage(allowed)); len(problems) != 0 {
			t.Errorf("binding names refused in %s: %+v", allowed, problems)
		}
	}
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
