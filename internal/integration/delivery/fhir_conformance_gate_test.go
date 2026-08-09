package delivery

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// fhirGateCanonicalEvent is the exact byte shape `integration_canonical_events.
// payload_json` holds and the dispatcher hands to the destination transport
// (store.go Claim selects `e.payload_json` into WorkItem.EventPayload). It is a
// canonical integration event. It is not, and has never been, a FHIR resource.
const fhirGateCanonicalEvent = `{` +
	`"event_id":"event-a",` +
	`"event_type":"patient.admitted",` +
	`"tenant_id":"tenant-a",` +
	`"occurred_at":"2026-08-09T09:00:00Z",` +
	`"patient":{"mrn":"MRN-000123","family_name":"Alpha","given_name":"Ada"}` +
	`}`

// TestFHIRConformance_DurableEngineProducesNoFHIRResource is the Slice 5.1
// second day-1 gate, and it must PASS on unmodified `main`.
//
// `.loom/28-spec-fhir-ig-bulk-smart.md:206-212` wrote the kill-test for the
// moment Slice 4.1c-b merged:
//
//	"If no resource is captured — because the destination consumer delivers a
//	canonical event rather than a FHIR resource — 5.1 is still blocked and the
//	blocker is 4.1c-b's scope, not the validator. Say so and stop."
//
// This is that kill-test executed rather than argued. It stands a live TLS
// endpoint, deploys an `https`-transport destination that points at it, and runs
// the real dispatcher — real messageForWorkItem, real destination.Transport,
// real net/http client — for one claimed durable work item. Then it reads the
// bytes that actually crossed the wire and asserts four things:
//
//  1. The request body is the delivery-command envelope
//     (`integration.delivery.v1`), whose `event` member is the canonical event
//     verbatim. Neither the envelope nor its event carries `resourceType`, so
//     nothing on this path is a FHIR resource.
//  2. The content type is `application/json`, not `application/fhir+json`.
//  3. The deployed transport vocabulary is exactly {kafka, https} and the
//     destination-class vocabulary is exactly {production, sandbox}. No value in
//     either denotes FHIR, so a destination cannot even declare that it wants
//     resources.
//  4. No package under `internal/integration/**` imports `pkg/fhir`. The mapper
//     is not reachable from the delivery path at all.
//
// Consequence: Slice 5.1 is not unblocked by 4.1c-b. Its real prerequisite is a
// slice nobody has written — a FHIR destination class (4.1c-c) — and the FHIR
// work that *is* unblocked is reconciling `pkg/fhir` with `pkg/fhir`'s own
// checker (Slice 5.1a). See `.loom/33-sprint5-execution-specs.md` correction 40.
//
// When 4.1c-c lands, this gate is the assertion that must be deliberately
// inverted, not deleted: it is the record of what the engine delivered before a
// FHIR destination class existed.
func TestFHIRConformance_DurableEngineProducesNoFHIRResource(t *testing.T) {
	served := fhirGateDispatchOnce(t)

	// 1. The wire body is a delivery command carrying a canonical event.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(served.body, &envelope); err != nil {
		t.Fatalf("the delivered body is not a JSON object: %v\nbody: %s", err, served.body)
	}
	var schema string
	if err := json.Unmarshal(envelope["schema"], &schema); err != nil {
		t.Fatalf("the delivered body has no schema member: %v\nbody: %s", err, served.body)
	}
	if schema != deliveryCommandSchema {
		t.Fatalf("delivered schema = %q, want the delivery-command envelope %q",
			schema, deliveryCommandSchema)
	}
	for _, member := range []string{"tenant_id", "outbox_id", "attempt_id", "receipt_id",
		"event_id", "trace_id", "destination", "route", "action", "attempt_count", "event"} {
		if _, present := envelope[member]; !present {
			t.Fatalf("the delivered body is missing delivery-command member %q; "+
				"it is not the Kafka command envelope this test claims it is", member)
		}
	}
	if _, present := envelope["resourceType"]; present {
		t.Fatalf("the delivered body declares resourceType; it is a FHIR resource, "+
			"not a delivery command\nbody: %s", served.body)
	}

	var event map[string]json.RawMessage
	if err := json.Unmarshal(envelope["event"], &event); err != nil {
		t.Fatalf("the delivery command's event member is not a JSON object: %v", err)
	}
	if _, present := event["resourceType"]; present {
		t.Fatalf("the canonical event carries resourceType; the payload is a FHIR "+
			"resource after all\nevent: %s", envelope["event"])
	}
	if _, present := event["event_type"]; !present {
		t.Fatalf("the delivered event is not the canonical event the outbox stored\nevent: %s",
			envelope["event"])
	}

	// 2. The content type is generic JSON.
	if served.contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q — a FHIR destination would send "+
			"application/fhir+json", served.contentType, "application/json")
	}

	// 3. No transport kind and no destination class denotes FHIR.
	assertNoFHIRTransportVocabulary(t)

	// 4. The mapper is not reachable from the delivery engine.
	assertNoFHIRImportsUnderIntegration(t)
}

// assertNoFHIRTransportVocabulary proves the deployed vocabulary cannot express
// "this destination receives FHIR resources".
//
// Both sets are asserted exhaustively against the validator that admits them
// rather than against a copy of the constant block, so a new kind added without
// updating this gate turns the gate red instead of leaving it stale.
func assertNoFHIRTransportVocabulary(t *testing.T) {
	t.Helper()

	admitted := make([]string, 0, 2)
	for _, candidate := range []string{
		"kafka", "https", "fhir", "fhir+json", "fhir-rest", "fhir-r4", "http", "mllp", "",
	} {
		revision, err := destination.NewRevision(destination.RevisionInput{
			ArtifactID: "dest-vocabulary", RevisionID: "destination-1",
			DestinationID: "dest-vocabulary",
			Class:         integration.DestinationClassProduction,
			Transport:     destination.TransportKind(candidate),
			HTTPS: &destination.HTTPSPolicy{
				URL: "https://destination.example.org/inbound", Method: "POST",
				TokenBinding: "token", CABundleBinding: "ca",
			},
			Kafka: &destination.KafkaPolicy{Topic: deliveryCommandSchema},
			Identity: &destination.ClientIdentity{
				Subject: "vocabulary-client",
				Grants:  []string{authorization.DestinationClientGrant},
			},
		})
		if err == nil && string(revision.Transport) == candidate {
			admitted = append(admitted, candidate)
			continue
		}
		// A kind may be refused for carrying both policies rather than for the
		// kind itself; retry it with only the policy that kind would use.
		input := destination.RevisionInput{
			ArtifactID: "dest-vocabulary", RevisionID: "destination-1",
			DestinationID: "dest-vocabulary",
			Class:         integration.DestinationClassProduction,
			Transport:     destination.TransportKind(candidate),
			Identity: &destination.ClientIdentity{
				Subject: "vocabulary-client",
				Grants:  []string{authorization.DestinationClientGrant},
			},
		}
		input.HTTPS = &destination.HTTPSPolicy{
			URL: "https://destination.example.org/inbound", Method: "POST",
			TokenBinding: "token", CABundleBinding: "ca",
		}
		if _, err := destination.NewRevision(input); err == nil {
			admitted = append(admitted, candidate)
			continue
		}
		input.HTTPS = nil
		input.Kafka = &destination.KafkaPolicy{Topic: deliveryCommandSchema}
		if _, err := destination.NewRevision(input); err == nil {
			admitted = append(admitted, candidate)
		}
	}
	sort.Strings(admitted)
	if want := []string{"https", "kafka"}; strings.Join(admitted, ",") != strings.Join(want, ",") {
		t.Fatalf("destination transport vocabulary = %v, want %v — a FHIR class "+
			"would change this gate's answer", admitted, want)
	}

	classes := map[integration.DestinationClass]bool{
		integration.DestinationClassProduction: true,
		integration.DestinationClassSandbox:    true,
	}
	for class := range classes {
		if strings.Contains(strings.ToLower(string(class)), "fhir") {
			t.Fatalf("destination class %q denotes FHIR", class)
		}
	}
	for _, candidate := range []integration.DestinationClass{"fhir", "fhir-r4", "us-core"} {
		if classes[candidate] {
			t.Fatalf("destination class vocabulary admits %q", candidate)
		}
	}
}

// assertNoFHIRImportsUnderIntegration proves the mapper is not on any path the
// durable engine executes. `pkg/fhir` is reachable from exactly two non-test
// files in the repository, and neither of them is in the integration engine.
func assertNoFHIRImportsUnderIntegration(t *testing.T) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	integrationRoot := filepath.Join(root, "internal", "integration")
	if info, err := os.Stat(integrationRoot); err != nil || !info.IsDir() {
		t.Fatalf("internal/integration is not where this gate expects it (%s): %v",
			integrationRoot, err)
	}

	fileSet := token.NewFileSet()
	var offenders []string
	scanned := 0
	walkErr := filepath.WalkDir(integrationRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if strings.Contains(imported, "fi-fhir/pkg/fhir") {
				relative, _ := filepath.Rel(root, path)
				offenders = append(offenders, relative)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/integration: %v", walkErr)
	}
	if scanned == 0 {
		t.Fatal("scanned zero non-test Go files under internal/integration; the walk is broken " +
			"and a zero here would mean nothing")
	}
	if len(offenders) != 0 {
		t.Fatalf("internal/integration imports pkg/fhir in %v — the delivery path can now "+
			"produce FHIR resources and this gate's premise has changed", offenders)
	}
}

// fhirGateServedRequest is the one request the destination actually received.
type fhirGateServedRequest struct {
	method      string
	contentType string
	body        []byte
}

// fhirGateDispatchOnce runs one real dispatch of one durable work item to a live
// TLS `https`-class destination and returns what the destination was sent.
//
// Everything on the production path is real: messageForWorkItem builds the
// envelope, destination.Transport resolves the credential and the trust roots
// from the deployed revision, and net/http performs the call. Only the durable
// store and the broker are faked, and the broker is faked precisely so that a
// Kafka publish would be visible as a test failure rather than as a silent
// second path.
func fhirGateDispatchOnce(t *testing.T) fhirGateServedRequest {
	t.Helper()

	var served fhirGateServedRequest
	var serves int
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			serves++
			served = fhirGateServedRequest{
				method:      r.Method,
				contentType: r.Header.Get("Content-Type"),
				body:        body,
			}
			w.WriteHeader(http.StatusOK)
		}))
	server.StartTLS()
	t.Cleanup(server.Close)

	caPEM := string(pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
	}))

	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "dest-https-conformance", RevisionID: "destination-1",
		DestinationID: "dest-https-conformance",
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportHTTPS,
		HTTPS: &destination.HTTPSPolicy{
			URL: server.URL, Method: "POST",
			TokenBinding: "conformance-token", CABundleBinding: "conformance-ca",
		},
		Identity: &destination.ClientIdentity{
			Subject: "conformance-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}

	registry := fhirGateRegistry(t, revision)
	transport, err := destination.NewTransport(destination.TransportConfig{
		Registry: registry,
		Resolver: fhirGateSecretResolver{values: map[string]string{
			"conformance/token": "conformance-token-material",
			"conformance/ca":    caPEM,
		}},
		Recorder: &fhirGateDeliveryRecorder{},
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	item := testWorkItem()
	item.Action = "send-https"
	item.Destination = revision.Reference()
	item.EventPayload = json.RawMessage(fhirGateCanonicalEvent)

	store := &fakeStore{item: &item}
	publisher := &fakePublisher{}
	dispatcher, err := NewDispatcherWithDestination(
		store, publisher, "worker-fhir-gate", testConfig(), nil, transport,
	)
	if err != nil {
		t.Fatalf("NewDispatcherWithDestination: %v", err)
	}

	outcome, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if outcome != OutcomePublished || store.published != 1 {
		t.Fatalf("outcome = %q, published = %d — the destination was not delivered to, "+
			"so there is nothing to inspect", outcome, store.published)
	}
	if serves != 1 {
		t.Fatalf("the destination served %d requests, want exactly 1", serves)
	}
	if len(publisher.message.Value) != 0 {
		t.Fatalf("an https-class destination also published to the broker; this gate " +
			"is reading the wrong path")
	}
	if served.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", served.method)
	}
	if len(served.body) == 0 {
		t.Fatal("the destination received an empty body")
	}
	return served
}

func fhirGateRegistry(t *testing.T, revisions ...destination.Revision) *destination.Registry {
	t.Helper()
	document := map[string]any{
		"schema":    "fi-fhir/destination-registry/v1",
		"tenant_id": "tenant-a",
		"integration_revision": map[string]string{
			"artifact_id": "integration-adt", "revision_id": "revision-1",
			"digest": "sha256:" + strings.Repeat("b", 64),
		},
		"secret_bindings": []map[string]any{
			{"name": "conformance-token", "reference": map[string]string{
				"provider": "file", "key": "conformance/token"}},
			{"name": "conformance-ca", "reference": map[string]string{
				"provider": "file", "key": "conformance/ca"}},
		},
		"destinations": revisions,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	registry, err := destination.LoadRegistry(strings.NewReader(string(encoded)), destination.ModeStrict)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return registry
}

type fhirGateSecretResolver struct {
	values map[string]string
}

func (r fhirGateSecretResolver) Resolve(
	ctx context.Context, reference integration.SecretReference,
) ([]byte, error) {
	if ctx == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	if integration.ValidateSecretReference(reference) != nil {
		return nil, integration.ErrSecretUnresolvable
	}
	value, found := r.values[reference.Key]
	if !found || value == "" {
		return nil, integration.ErrSecretUnresolvable
	}
	return []byte(value), nil
}

type fhirGateDeliveryRecorder struct {
	records []destination.DeliveryRecord
}

func (r *fhirGateDeliveryRecorder) RecordDelivery(
	_ context.Context, record destination.DeliveryRecord,
) error {
	r.records = append(r.records, record)
	return nil
}
