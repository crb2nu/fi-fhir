package connection

import (
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// expectation is what a mutation must do to the checker and the constructor.
type expectation int

const (
	// rejected: the document constructor refuses it, and the checker catches
	// it first, at the named path.
	rejected expectation = iota
	// stricter: the constructor would accept it, but the catalog refuses it at
	// the named path (an undeclared binding, an implicit TLS default). The
	// checker may be stricter than the constructor, never looser.
	stricter
	// valid: both accept it.
	valid
)

type checkerMutation struct {
	name   string
	apply  func(*testing.T, *specFixture)
	path   string
	expect expectation
}

// TestConnectionChecker_MirrorsConstructorBounds is the table test .loom/38
// names. For every kind: the catalog-valid fixture compiles with the
// document's own constructor, and every mutation the constructor refuses is
// caught by the checker first, with the path of the field that is wrong.
//
// The constructors stay the authority. A "rejected" row that the constructor
// accepts fails here too — it would mean the row no longer exercises a bound,
// and the checker's copy of that bound can drift unseen.
func TestConnectionChecker_MirrorsConstructorBounds(t *testing.T) {
	for _, kindCase := range checkerCases() {
		t.Run(string(kindCase.kind), func(t *testing.T) {
			base := loadSpecFixture(t, kindCase.kind)
			t.Run("fixture compiles and round-trips", func(t *testing.T) {
				draft := base.draft(t, "fixture-"+strings.ReplaceAll(string(kindCase.kind), "_", "-"))
				revision, problems := BuildRevision(draft, 1, integration.AuditEnvelope{})
				if revision == nil || HasBlocking(problems) || len(problems) != 0 {
					t.Fatalf("fixture did not compile cleanly: %+v", problems)
				}
				decoded, err := DocumentDigest(kindCase.kind, revision.Document)
				if err != nil || decoded != revision.Digest {
					t.Fatalf("compiled document decodes to %q (%v), want %q", decoded, err, revision.Digest)
				}
			})
			for _, mutation := range kindCase.mutations {
				t.Run(mutation.name, func(t *testing.T) {
					fixture := base.clone(t)
					mutation.apply(t, &fixture)
					spec, problems := decodeAndCheck(fixture.Kind, fixture.specJSON(t), fixture.SecretBindings)
					if spec == nil {
						t.Fatalf("mutation left no decodable spec: %+v", problems)
					}
					_, _, constructErr := constructDocument(fixture.Kind, spec, "connection-under-test", "1")
					switch mutation.expect {
					case rejected:
						if constructErr == nil {
							t.Fatalf("the constructor accepted this mutation, so the row exercises no constructor bound; "+
								"mark it stricter or valid (checker problems: %+v)", problems)
						}
						if !blockingAt(problems, mutation.path) {
							t.Fatalf("the constructor refused this (%v) but the checker reported no blocking problem at %q: %+v",
								constructErr, mutation.path, problems)
						}
					case stricter:
						if constructErr != nil {
							t.Fatalf("the constructor refused this too (%v); mark it rejected", constructErr)
						}
						if !blockingAt(problems, mutation.path) {
							t.Fatalf("the catalog should refuse this at %q: %+v", mutation.path, problems)
						}
					case valid:
						if constructErr != nil {
							t.Fatalf("the constructor refused a variant the catalog calls valid: %v", constructErr)
						}
						if HasBlocking(problems) {
							t.Fatalf("the checker refused a variant the constructor accepts: %+v", problems)
						}
					}
				})
			}
		})
	}
}

type checkerCase struct {
	kind      Kind
	mutations []checkerMutation
}

func checkerCases() []checkerCase {
	return []checkerCase{
		{KindMLLP, mllpMutations()},
		{KindBatchS3, batchS3Mutations()},
		{KindBatchSFTP, batchSFTPMutations()},
		{KindHTTP, httpMutations()},
		{KindHTTPS, httpsMutations()},
		{KindFHIR, fhirMutations()},
		{KindKafka, kafkaMutations()},
	}
}

func mllpMutations() []checkerMutation {
	disabledTLS := all(
		set("tls", map[string]any{"mode": "disabled"}),
		bindings(),
	)
	identity := map[string]any{
		"subject":     "lab-east",
		"uri_san":     "spiffe://example.org/lab-east",
		"spki_sha256": "sha256:" + strings.Repeat("ab", 32),
		"grants":      []any{"integration:mllp"},
	}
	return []checkerMutation{
		{"source id missing", del("source_id"), "source_id", rejected},
		{"source id with whitespace", set("source_id", "adt east"), "source_id", rejected},
		{"listen address missing", del("listen_address"), "listen_address", rejected},
		{"listen address without port", set("listen_address", "0.0.0.0"), "listen_address", rejected},
		{"listen address port zero", set("listen_address", "0.0.0.0:0"), "listen_address", rejected},
		{"listen address port too large", set("listen_address", "0.0.0.0:65536"), "listen_address", rejected},
		{"listen address non-canonical port", set("listen_address", "0.0.0.0:02575"), "listen_address", rejected},
		{"encoding missing", del("encoding"), "encoding", rejected},
		{"encoding unsupported", set("encoding", "latin-1"), "encoding", rejected},
		{"framing byte zero", set("framing.start_byte", 0), "framing.start_byte", rejected},
		{"framing bytes collide", set("framing.end_byte", 11), "framing.end_byte", rejected},
		{"framing omitted uses the standard bytes", del("framing"), "", valid},
		{"framing partially overridden", del("framing.end_byte", "framing.trailer_byte"), "", valid},
		{"timeouts missing", del("timeouts"), "timeouts", rejected},
		{"read timeout zero", set("timeouts.read_seconds", 0), "timeouts.read_seconds", rejected},
		{"read timeout too long", set("timeouts.read_seconds", 301), "timeouts.read_seconds", rejected},
		{"read timeout missing", del("timeouts.read_seconds"), "timeouts.read_seconds", rejected},
		{"write timeout too long", set("timeouts.write_seconds", 61), "timeouts.write_seconds", rejected},
		{"idle shorter than read", set("timeouts.idle_seconds", 4), "timeouts.idle_seconds", rejected},
		{"idle too long", set("timeouts.idle_seconds", 3601), "timeouts.idle_seconds", rejected},
		{"process timeout zero", set("timeouts.process_seconds", 0), "timeouts.process_seconds", rejected},
		{"process timeout too long", set("timeouts.process_seconds", 301), "timeouts.process_seconds", rejected},
		{"tls missing", del("tls"), "tls", rejected},
		{"tls mode unknown", set("tls.mode", "optional"), "tls.mode", rejected},
		{"mutual tls without a key binding", del("tls.server_private_key_binding"), "tls.server_private_key_binding", rejected},
		{"disabled tls with bindings", set("tls.mode", "disabled"), "tls.server_certificate_binding", rejected},
		{"disabled tls without bindings", disabledTLS, "", valid},
		{"allowlist empty", set("clients.allowed_cidrs", []any{}), "clients.allowed_cidrs", rejected},
		{"allowlist missing", del("clients.allowed_cidrs"), "clients.allowed_cidrs", rejected},
		{"allowlist host bits set", set("clients.allowed_cidrs", []any{"10.0.0.1/8"}), "clients.allowed_cidrs[0]", rejected},
		{"allowlist not a CIDR", set("clients.allowed_cidrs", []any{"ten-dot-zero"}), "clients.allowed_cidrs[0]", rejected},
		{"allowlist duplicate", set("clients.allowed_cidrs", []any{"10.0.0.0/8", "10.0.0.0/8"}), "clients.allowed_cidrs[1]", rejected},
		{"allowlist too long", set("clients.allowed_cidrs", cidrs(129)), "clients.allowed_cidrs", rejected},
		{"clients missing", del("clients"), "clients", rejected},
		{"identity mapped", set("clients.identities", []any{identity}), "", valid},
		{"identity without uri or pin", set("clients.identities", []any{map[string]any{"subject": "lab-east"}}),
			"clients.identities[0]", rejected},
		{"identity uri without scheme", set("clients.identities", []any{map[string]any{"subject": "lab-east", "uri_san": "lab-east"}}),
			"clients.identities[0].uri_san", rejected},
		{"identity pin malformed", set("clients.identities", []any{map[string]any{"subject": "lab-east", "spki_sha256": "sha256:XYZ"}}),
			"clients.identities[0].spki_sha256", rejected},
		{"identity subject repeated", set("clients.identities", []any{
			map[string]any{"subject": "lab-east", "uri_san": "spiffe://example.org/a"},
			map[string]any{"subject": "lab-east", "uri_san": "spiffe://example.org/b"},
		}), "clients.identities[1].subject", rejected},
		{"identity grant repeated", set("clients.identities", []any{map[string]any{
			"subject": "lab-east", "uri_san": "spiffe://example.org/a", "grants": []any{"g", "g"},
		}}), "clients.identities[0].grants[1]", rejected},
		{"identity mapping without mutual tls", all(disabledTLS, set("clients.identities", []any{identity})),
			"clients.identities", rejected},
		{"acknowledgements missing", del("acknowledgements"), "acknowledgements", rejected},
		{"acknowledgement mode unknown", set("acknowledgements.mode", "never"), "acknowledgements.mode", rejected},
		{"error segment flag omitted", del("acknowledgements.include_error_segment"), "", valid},
		{"max message bytes zero", set("max_message_bytes", 0), "max_message_bytes", rejected},
		{"max message bytes too large", set("max_message_bytes", 1048577), "max_message_bytes", rejected},
		{"max connections zero", set("max_connections", 0), "max_connections", rejected},
		{"max connections too many", set("max_connections", 10001), "max_connections", rejected},
		{"max connections missing", del("max_connections"), "max_connections", rejected},
		{"binding named but not declared", bindings(binding("mllp-server-cert"), binding("mllp-server-key")),
			"tls.client_ca_binding", stricter},
	}
}

func batchS3Mutations() []checkerMutation {
	workload := map[string]any{"subject": "batch-adt-east", "grants": []any{"integration:batch"}}
	return []checkerMutation{
		{"source id missing", del("source_id"), "source_id", rejected},
		{"poll zero", set("poll_seconds", 0), "poll_seconds", rejected},
		{"poll too long", set("poll_seconds", 3601), "poll_seconds", rejected},
		{"process zero", set("process_seconds", 0), "process_seconds", rejected},
		{"process too long", set("process_seconds", 301), "process_seconds", rejected},
		{"lease equals process", set("lease_seconds", 60), "lease_seconds", rejected},
		{"lease too long", set("lease_seconds", 3601), "lease_seconds", rejected},
		{"lease missing", del("lease_seconds"), "lease_seconds", rejected},
		{"files per poll zero", set("max_files_per_poll", 0), "max_files_per_poll", rejected},
		{"files per poll too many", set("max_files_per_poll", 1001), "max_files_per_poll", rejected},
		{"message bytes too large", set("max_message_bytes", 1048577), "max_message_bytes", rejected},
		{"s3 missing", del("s3"), "s3", rejected},
		{"endpoint with scheme", set("s3.endpoint", "https://objects.example.com"), "s3.endpoint", rejected},
		{"endpoint with path", set("s3.endpoint", "objects.example.com/bucket"), "s3.endpoint", rejected},
		{"endpoint port zero", set("s3.endpoint", "objects.example.com:0"), "s3.endpoint", rejected},
		{"endpoint bad label", set("s3.endpoint", "-objects.example.com"), "s3.endpoint", rejected},
		{"bucket missing", del("s3.bucket"), "s3.bucket", rejected},
		{"bucket with whitespace", set("s3.bucket", "adt drop"), "s3.bucket", rejected},
		{"plaintext to a remote endpoint", set("s3.use_tls", false), "s3.use_tls", rejected},
		{"plaintext to loopback", all(set("s3.use_tls", false), set("s3.endpoint", "localhost:9000")), "", valid},
		{"tls flag omitted for loopback", all(del("s3.use_tls"), set("s3.endpoint", "127.0.0.1:9000")), "s3.use_tls", stricter},
		{"input prefix absolute", set("s3.input_prefix", "/incoming"), "s3.input_prefix", rejected},
		{"input prefix escapes", set("s3.input_prefix", "../incoming"), "s3.input_prefix", rejected},
		{"input prefix not clean", set("s3.input_prefix", "a/../incoming"), "s3.input_prefix", rejected},
		{"archive inside input", set("s3.archive_prefix", "incoming/archive"), "s3.archive_prefix", rejected},
		{"access key binding missing", del("s3.access_key_binding"), "s3.access_key_binding", rejected},
		{"both bindings the same", all(set("s3.secret_access_key_binding", "batch-s3-access-key"),
			bindings(binding("batch-s3-access-key"))), "s3.secret_access_key_binding", rejected},
		{"region omitted", del("s3.region"), "", valid},
		{"workload bound", set("workload", workload), "", valid},
		{"workload without subject", set("workload", map[string]any{"grants": []any{"integration:batch"}}),
			"workload.subject", rejected},
		{"workload grant repeated", set("workload", map[string]any{"subject": "s", "grants": []any{"g", "g"}}),
			"workload.grants[1]", rejected},
		{"workload grants too many", set("workload", map[string]any{"subject": "s", "grants": repeat("g", 17)}),
			"workload.grants", rejected},
	}
}

func batchSFTPMutations() []checkerMutation {
	return []checkerMutation{
		{"sftp missing", del("sftp"), "sftp", rejected},
		{"host missing", del("sftp.host"), "sftp.host", rejected},
		{"host with space", set("sftp.host", "sftp example"), "sftp.host", rejected},
		{"port zero", set("sftp.port", 0), "sftp.port", rejected},
		{"port too large", set("sftp.port", 65536), "sftp.port", rejected},
		{"username missing", del("sftp.username"), "sftp.username", rejected},
		{"input directory relative", set("sftp.input_directory", "inbound"), "sftp.input_directory", rejected},
		{"input directory root", set("sftp.input_directory", "/"), "sftp.input_directory", rejected},
		{"archive inside input", set("sftp.archive_directory", "/inbound/archive"), "sftp.archive_directory", rejected},
		{"known hosts binding missing", del("sftp.known_hosts_binding"), "sftp.known_hosts_binding", rejected},
		{"password and private key", all(set("sftp.password_binding", "sftp-password"),
			bindings(binding("sftp-known-hosts"), binding("sftp-client-key"), binding("sftp-client-key-passphrase"), binding("sftp-password"))),
			"sftp.private_key_binding", rejected},
		{"neither password nor private key", del("sftp.private_key_binding", "sftp.private_key_passphrase_binding"),
			"sftp.password_binding", rejected},
		{"password with a passphrase", all(del("sftp.private_key_binding"), set("sftp.password_binding", "sftp-password"),
			bindings(binding("sftp-known-hosts"), binding("sftp-client-key-passphrase"), binding("sftp-password"))),
			"sftp.private_key_passphrase_binding", rejected},
		{"password only", all(del("sftp.private_key_binding", "sftp.private_key_passphrase_binding"),
			set("sftp.password_binding", "sftp-password"), bindings(binding("sftp-known-hosts"), binding("sftp-password"))),
			"", valid},
		{"private key without passphrase", all(del("sftp.private_key_passphrase_binding"),
			bindings(binding("sftp-known-hosts"), binding("sftp-client-key"))), "", valid},
		{"workload omitted", del("workload"), "", valid},
	}
}

func httpMutations() []checkerMutation {
	oauth := map[string]any{
		"issuer_url":         "https://issuer.example.org/realms/fi-fhir",
		"audience":           "fi-fhir-ingress",
		"allowed_client_ids": []any{"adt-east-client"},
	}
	oauthMode := all(
		set("auth_mode", "oauth2"), del("principal_id", "credential_binding"),
		set("oauth", oauth), bindings(),
	)
	return []checkerMutation{
		{"source id missing", del("source_id"), "source_id", rejected},
		{"path relative", set("path", "v1/hl7v2"), "path", rejected},
		{"path not clean", set("path", "/v1/../hl7v2"), "path", rejected},
		{"path with space", set("path", "/v1/hl7 v2"), "path", rejected},
		{"path omitted defaults", del("path"), "", valid},
		{"auth mode missing", del("auth_mode"), "auth_mode", rejected},
		{"auth mode unknown", set("auth_mode", "basic"), "auth_mode", rejected},
		{"hmac", set("auth_mode", "hmac-sha256"), "", valid},
		{"bearer without principal", del("principal_id"), "principal_id", rejected},
		{"bearer without credential binding", del("credential_binding"), "credential_binding", rejected},
		{"bearer with an oauth block", set("oauth", oauth), "oauth", rejected},
		{"body limit zero", set("max_body_bytes", 0), "max_body_bytes", rejected},
		{"body limit too large", set("max_body_bytes", 1048577), "max_body_bytes", rejected},
		{"body limit missing", del("max_body_bytes"), "max_body_bytes", rejected},
		{"oauth2 with runtime defaults", oauthMode, "", valid},
		{"oauth2 with every claim", all(oauthMode, set("oauth.tenant_claim", "tid"), set("oauth.roles_claim", "groups"),
			set("oauth.client_id_claim", "azp"), set("oauth.signing_algs", []any{"ES256", "RS256"})), "", valid},
		{"oauth2 with a principal", all(oauthMode, set("principal_id", "sender")), "principal_id", rejected},
		{"oauth2 with a credential binding", all(oauthMode, set("credential_binding", "http-ingress-credential"),
			bindings(binding("http-ingress-credential"))), "credential_binding", rejected},
		{"oauth2 without oauth", all(oauthMode, del("oauth")), "oauth", rejected},
		{"oauth2 issuer over http", all(oauthMode, set("oauth.issuer_url", "http://issuer.example.org")),
			"oauth.issuer_url", rejected},
		{"oauth2 issuer with query", all(oauthMode, set("oauth.issuer_url", "https://issuer.example.org/?a=b")),
			"oauth.issuer_url", rejected},
		{"oauth2 issuer with credentials", all(oauthMode, set("oauth.issuer_url", "https://u:p@issuer.example.org")),
			"oauth.issuer_url", rejected},
		{"oauth2 audience missing", all(oauthMode, del("oauth.audience")), "oauth.audience", rejected},
		{"oauth2 claims collide", all(oauthMode, set("oauth.tenant_claim", "roles")), "oauth.roles_claim", rejected},
		{"oauth2 client claim is sub", all(oauthMode, set("oauth.client_id_claim", "sub")), "oauth.client_id_claim", rejected},
		{"oauth2 unsupported algorithm", all(oauthMode, set("oauth.signing_algs", []any{"HS256"})),
			"oauth.signing_algs[0]", rejected},
		{"oauth2 algorithm repeated", all(oauthMode, set("oauth.signing_algs", []any{"RS256", "RS256"})),
			"oauth.signing_algs[1]", rejected},
		{"oauth2 no allowed clients", all(oauthMode, set("oauth.allowed_client_ids", []any{})),
			"oauth.allowed_client_ids", rejected},
		{"oauth2 client id with space", all(oauthMode, set("oauth.allowed_client_ids", []any{"adt east"})),
			"oauth.allowed_client_ids[0]", rejected},
	}
}

func httpsMutations() []checkerMutation {
	return []checkerMutation{
		{"destination id missing", del("destination_id"), "destination_id", rejected},
		{"class unknown", set("class", "staging"), "class", rejected},
		{"https missing", del("https"), "https", rejected},
		{"url missing", del("https.url"), "https.url", rejected},
		{"url over http", set("https.url", "http://destination.example.org/inbound"), "https.url", rejected},
		{"url with credentials", set("https.url", "https://user:pw@destination.example.org/inbound"), "https.url", rejected},
		{"url with fragment", set("https.url", "https://destination.example.org/inbound#x"), "https.url", rejected},
		{"url without host", set("https.url", "https:///inbound"), "https.url", rejected},
		{"url too long", set("https.url", "https://destination.example.org/"+strings.Repeat("a", 2048)), "https.url", rejected},
		{"url with query", set("https.url", "https://destination.example.org/inbound?tenant=a"), "", valid},
		{"method unknown", set("https.method", "PATCH"), "https.method", rejected},
		{"method put", set("https.method", "PUT"), "", valid},
		{"token binding missing", del("https.token_binding"), "https.token_binding", rejected},
		{"ca bundle equals token", all(set("https.ca_bundle_binding", "pinned-token"), bindings(binding("pinned-token"))),
			"https.ca_bundle_binding", rejected},
		{"ca bundle omitted", all(del("https.ca_bundle_binding"), bindings(binding("pinned-token"))), "", valid},
		{"identity without grants", set("identity.grants", []any{}), "identity.grants", rejected},
		{"identity grants too many", set("identity.grants", repeat("grant", 17)), "identity.grants", rejected},
		{"identity without subject", del("identity.subject"), "identity.subject", rejected},
		{"identity omitted", del("identity"), "", valid},
		{"a second transport policy", set("kafka", map[string]any{"topic": "t"}), "kafka", stricter},
	}
}

func fhirMutations() []checkerMutation {
	return []checkerMutation{
		{"fhir missing", del("fhir"), "fhir", rejected},
		{"base url with query", set("fhir.base_url", "https://fhir.example.org/r4?_format=json"), "fhir.base_url", rejected},
		{"base url over http", set("fhir.base_url", "http://fhir.example.org/r4"), "fhir.base_url", rejected},
		{"interaction unsupported", set("fhir.interaction", "batch"), "fhir.interaction", rejected},
		{"interaction omitted defaults", del("fhir.interaction"), "", valid},
		{"token binding missing", del("fhir.token_binding"), "fhir.token_binding", rejected},
		{"ca bundle equals token", all(set("fhir.ca_bundle_binding", "fhir-token"), bindings(binding("fhir-token"))),
			"fhir.ca_bundle_binding", rejected},
		{"class missing", del("class"), "class", rejected},
	}
}

func kafkaMutations() []checkerMutation {
	return []checkerMutation{
		{"kafka missing", del("kafka"), "kafka", rejected},
		{"topic missing", del("kafka.topic"), "kafka.topic", rejected},
		{"topic with space", set("kafka.topic", "integration delivery"), "kafka.topic", rejected},
		{"topic too long", set("kafka.topic", strings.Repeat("t", 250)), "kafka.topic", rejected},
		{"class unknown", set("class", "prod"), "class", rejected},
		{"destination id with whitespace", set("destination_id", "dest kafka"), "destination_id", rejected},
		{"identity omitted", del("identity"), "", valid},
	}
}
