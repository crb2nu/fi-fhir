package delivery

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// History: the gate this file inverts.
//
// Until Slice 4.1c-c this file held TestFHIRConformance_DurableEngineProducesNoFHIRResource,
// the Slice 5.1a second day-1 gate, which PASSED on unmodified `main` from
// 2026-08-09 to 2026-09-08 and whose doc comment read:
//
//	`.loom/28-spec-fhir-ig-bulk-smart.md:206-212` wrote the kill-test for the
//	moment Slice 4.1c-b merged:
//
//	    "If no resource is captured — because the destination consumer
//	    delivers a canonical event rather than a FHIR resource — 5.1 is still
//	    blocked and the blocker is 4.1c-b's scope, not the validator. Say so
//	    and stop."
//
//	This is that kill-test executed rather than argued. It stands a live TLS
//	endpoint, deploys an `https`-transport destination that points at it, and
//	runs the real dispatcher — real messageForWorkItem, real
//	destination.Transport, real net/http client — for one claimed durable work
//	item. Then it reads the bytes that actually crossed the wire and asserts
//	four things:
//
//	 1. The request body is the delivery-command envelope
//	    (`integration.delivery.v1`), whose `event` member is the canonical
//	    event verbatim. Neither the envelope nor its event carries
//	    `resourceType`, so nothing on this path is a FHIR resource.
//	 2. The content type is `application/json`, not `application/fhir+json`.
//	 3. The deployed transport vocabulary is exactly {kafka, https} and the
//	    destination-class vocabulary is exactly {production, sandbox}. No value
//	    in either denotes FHIR, so a destination cannot even declare that it
//	    wants resources.
//	 4. No package under `internal/integration/**` imports `pkg/fhir`. The
//	    mapper is not reachable from the delivery path at all.
//
//	Consequence: Slice 5.1 is not unblocked by 4.1c-b. Its real prerequisite is
//	a slice nobody has written — a FHIR destination class (4.1c-c) — and the
//	FHIR work that *is* unblocked is reconciling `pkg/fhir` with `pkg/fhir`'s
//	own checker (Slice 5.1a). See `.loom/33-sprint5-execution-specs.md`
//	correction 40.
//
//	When 4.1c-c lands, this gate is the assertion that must be deliberately
//	inverted, not deleted: it is the record of what the engine delivered before
//	a FHIR destination class existed.
//
// Its fixture hand-wrote `{"event_id","event_type":"patient.admitted",…}`; that
// was never the stored wire shape (`.loom/34` correction 3), which is why the
// inverted gate below is driven by NewProcessedEvent output instead.

// TestFHIRDestination_DurableEngineDeliversFHIRResource is the 5.1a gate
// inverted by Slice 4.1c-c: the same real dispatcher, over a `fhir`-transport
// destination, and every one of the four assertions above turned around.
//
//  1. The request body is a FHIR R4 transaction Bundle of a US Core Patient and
//     Encounter — projected from the exact payload NewProcessedEvent stores —
//     whose every entry is a conditional `PUT <Type>?identifier=<system>|<value>`
//     with a `urn:uuid:` fullUrl, and whose Encounter references the Patient
//     entry rather than an id the destination never issued. Every resource
//     validates at `us-core` with zero issues.
//  2. The content type is `application/fhir+json; charset=utf-8`, with
//     `Accept: application/fhir+json`, `Prefer: return=minimal`, and the same
//     server-owned `Idempotency-Key` the https transport sends.
//  3. The deployed transport vocabulary is exactly {fhir, https, kafka}; the
//     destination-class vocabulary is still {production, sandbox}, because the
//     FHIR class is a transport of the server-owned revision, never a class or
//     a workflow flag (DESTINATION-IDENTITY.md).
//  4. `internal/integration/fhirout` is the only non-test package under
//     `internal/integration/**` that imports `pkg/fhir`: the mapper is reachable
//     from the delivery path through exactly one door.
//
// And two things the old gate could not ask: the delivery ledger records the
// act as `transport = fhir` with the resource types and entry count, and a
// destination with referential-integrity checking on accepts the bundle and
// holds exactly one Patient and one Encounter afterwards.
func TestFHIRDestination_DurableEngineDeliversFHIRResource(t *testing.T) {
	server := newFHIRTestServer(t)
	server.strictReferences = true

	processed := fhirGateProcessedEvent(t)
	served, recorder := fhirGateDispatchFHIR(t, server, processed.PayloadJSON())

	// 1. The wire body is a conditional transaction Bundle of Patient + Encounter.
	var bundle struct {
		ResourceType string `json:"resourceType"`
		Type         string `json:"type"`
		Schema       string `json:"schema"`
		Entry        []struct {
			FullURL  string          `json:"fullUrl"`
			Resource json.RawMessage `json:"resource"`
			Request  *struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"request"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(served.Body, &bundle); err != nil {
		t.Fatalf("the delivered body is not a JSON object: %v\nbody: %s", err, served.Body)
	}
	if bundle.Schema != "" {
		t.Fatalf("the delivered body carries schema=%q: it is still the delivery-command envelope", bundle.Schema)
	}
	if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" {
		t.Fatalf("delivered resourceType=%q type=%q, want a transaction Bundle\nbody: %s",
			bundle.ResourceType, bundle.Type, served.Body)
	}
	if len(bundle.Entry) != 2 {
		t.Fatalf("delivered bundle has %d entries, want 2 (Patient, Encounter)\nbody: %s", len(bundle.Entry), served.Body)
	}
	types := make([]string, 0, len(bundle.Entry))
	fullURLs := make(map[string]string, len(bundle.Entry))
	var encounterSubject string
	for index, entry := range bundle.Entry {
		var resource map[string]any
		if err := json.Unmarshal(entry.Resource, &resource); err != nil {
			t.Fatalf("entry %d resource is not a JSON object: %v", index, err)
		}
		resourceType, _ := resource["resourceType"].(string)
		types = append(types, resourceType)
		if entry.Request == nil || entry.Request.Method != http.MethodPut {
			t.Fatalf("entry %d (%s) request method = %v, want PUT — a POST creates a duplicate on every redelivery",
				index, resourceType, entry.Request)
		}
		system, value, conditional := fhirGateConditionalKey(entry.Request.URL, resourceType)
		if !conditional || system == "" || value == "" {
			t.Fatalf("entry %d (%s) request url = %q, want %s?identifier=<system>|<value>",
				index, resourceType, entry.Request.URL, resourceType)
		}
		if !strings.HasPrefix(entry.FullURL, "urn:uuid:") {
			t.Fatalf("entry %d (%s) fullUrl = %q, want urn:uuid:", index, resourceType, entry.FullURL)
		}
		fullURLs[resourceType] = entry.FullURL
		if _, present := resource["id"]; present {
			t.Fatalf("entry %d (%s) carries an id; a conditional update must not name an id the destination did not issue",
				index, resourceType)
		}
		if !fhirGateResourceHasIdentifier(resource, system, value) {
			t.Fatalf("entry %d (%s) does not carry its own conditional key %s|%s among its identifiers, "+
				"so the destination's next match could not find this write", index, resourceType, system, value)
		}
		if resourceType == "Encounter" {
			subject, _ := resource["subject"].(map[string]any)
			encounterSubject, _ = subject["reference"].(string)
		}
		outcome, err := fhir.ValidateJSON(entry.Resource, fhir.ValidationOptions{Mode: string(fhir.ModeUSCore)})
		if err != nil {
			t.Fatalf("ValidateJSON(entry %d): %v", index, err)
		}
		if len(outcome.Issue) != 0 {
			t.Fatalf("entry %d (%s) does not validate at us-core --strict: %s\nresource: %s",
				index, resourceType, fhirGateDescribeIssues(outcome.Issue), entry.Resource)
		}
	}
	if want := []string{"Patient", "Encounter"}; strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("delivered resource types = %v, want %v", types, want)
	}
	if encounterSubject != fullURLs["Patient"] {
		t.Fatalf("Encounter.subject.reference = %q, want the Patient entry's fullUrl %q — a literal "+
			"Patient/<mrn> names an id the destination never issued", encounterSubject, fullURLs["Patient"])
	}

	// 2. FHIR content negotiation and the shared idempotency key.
	if served.ContentType != "application/fhir+json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/fhir+json; charset=utf-8", served.ContentType)
	}
	if served.Accept != "application/fhir+json" {
		t.Fatalf("Accept = %q, want application/fhir+json", served.Accept)
	}
	if served.Prefer != "return=minimal" {
		t.Fatalf("Prefer = %q, want return=minimal", served.Prefer)
	}
	if served.IdempotencyKey != "attempt-a" {
		t.Fatalf("Idempotency-Key = %q, want the durable attempt id", served.IdempotencyKey)
	}

	// 3. The vocabulary admits exactly one new value.
	assertTransportVocabulary(t)

	// 4. The mapper is reachable through exactly one door.
	assertFHIROutIsTheOnlyMapperImporter(t)

	// 5. The ledger records the act as a fhir delivery with what crossed the wire.
	if len(recorder.records) != 1 {
		t.Fatalf("delivery ledger holds %d records, want 1", len(recorder.records))
	}
	record := recorder.records[0]
	if record.Transport != destination.TransportFHIR || record.Outcome != "delivered" ||
		record.HTTPStatusClass != "2xx" || record.FailureCode != "" {
		t.Fatalf("delivery record = %+v, want transport fhir, outcome delivered, 2xx", record)
	}
	if record.FHIRResourceTypes != "Patient,Encounter" || record.FHIREntryCount != 2 || record.FHIROutcomeCodesAdvisory != "" {
		t.Fatalf("delivery record FHIR facts = %q/%d/%q, want Patient,Encounter / 2 / \"\"",
			record.FHIRResourceTypes, record.FHIREntryCount, record.FHIROutcomeCodesAdvisory)
	}

	// 6. A destination with referential integrity on accepted it and holds one
	// of each, with the Encounter now pointing at the Patient it issued.
	if got := server.Count("Patient"); got != 1 {
		t.Fatalf("destination holds %d Patients, want 1", got)
	}
	if got := server.Count("Encounter"); got != 1 {
		t.Fatalf("destination holds %d Encounters, want 1", got)
	}
	stored := server.Resources("Encounter")[0]
	subject, _ := stored["subject"].(map[string]any)
	if reference, _ := subject["reference"].(string); !strings.HasPrefix(reference, "Patient/") ||
		reference != "Patient/"+server.Resources("Patient")[0]["id"].(string) {
		t.Fatalf("stored Encounter.subject.reference = %q, want the stored Patient's Type/id", reference)
	}
}

// fhirGateConditionalKey parses `<Type>?identifier=<system>|<value>`.
func fhirGateConditionalKey(requestURL, resourceType string) (system, value string, ok bool) {
	parsed, err := url.Parse(requestURL)
	if err != nil || parsed.Path != resourceType || parsed.Fragment != "" {
		return "", "", false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 1 || len(query["identifier"]) != 1 {
		return "", "", false
	}
	return fhirSplitIdentifierToken(query.Get("identifier"))
}

func fhirGateResourceHasIdentifier(resource map[string]any, system, value string) bool {
	return fhirResourceHasIdentifier(resource, system, value)
}

func fhirGateDescribeIssues(issues []fhir.OperationOutcomeIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Severity+" "+issue.Code+": "+issue.Diagnostics)
	}
	return strings.Join(parts, "; ")
}

// assertTransportVocabulary proves the deployed vocabulary admits exactly
// {fhir, https, kafka} and that no destination class denotes FHIR.
//
// Both sets are asserted exhaustively against the validator that admits them
// rather than against a copy of the constant block, so a new kind added without
// updating this gate turns the gate red instead of leaving it stale. Each
// candidate is retried with each policy shape, because a kind may be refused
// for carrying the wrong policy rather than for the kind itself.
func assertTransportVocabulary(t *testing.T) {
	t.Helper()

	https := &destination.HTTPSPolicy{
		URL: "https://destination.example.org/inbound", Method: "POST",
		TokenBinding: "token", CABundleBinding: "ca",
	}
	kafka := &destination.KafkaPolicy{Topic: deliveryCommandSchema}
	fhirPolicy := &destination.FHIRPolicy{
		BaseURL: "https://destination.example.org/fhir", TokenBinding: "token",
		CABundleBinding: "ca", Interaction: destination.FHIRInteractionTransaction,
	}
	admitted := make([]string, 0, 3)
	for _, candidate := range []string{
		"kafka", "https", "fhir", "fhir+json", "fhir-rest", "fhir-r4", "http", "mllp", "",
	} {
		base := destination.RevisionInput{
			ArtifactID: "dest-vocabulary", RevisionID: "destination-1",
			DestinationID: "dest-vocabulary",
			Class:         integration.DestinationClassProduction,
			Transport:     destination.TransportKind(candidate),
			Identity: &destination.ClientIdentity{
				Subject: "vocabulary-client",
				Grants:  []string{authorization.DestinationClientGrant},
			},
		}
		shapes := []destination.RevisionInput{base, base, base}
		shapes[0].HTTPS = https
		shapes[1].Kafka = kafka
		shapes[2].FHIR = fhirPolicy
		for _, input := range shapes {
			if revision, err := destination.NewRevision(input); err == nil && string(revision.Transport) == candidate {
				admitted = append(admitted, candidate)
				break
			}
		}
	}
	sort.Strings(admitted)
	if want := []string{"fhir", "https", "kafka"}; strings.Join(admitted, ",") != strings.Join(want, ",") {
		t.Fatalf("destination transport vocabulary = %v, want %v", admitted, want)
	}

	classes := map[integration.DestinationClass]bool{
		integration.DestinationClassProduction: true,
		integration.DestinationClassSandbox:    true,
	}
	for class := range classes {
		if strings.Contains(strings.ToLower(string(class)), "fhir") {
			t.Fatalf("destination class %q denotes FHIR; the FHIR class is a transport, not an environment class", class)
		}
	}
	for _, candidate := range []integration.DestinationClass{"fhir", "fhir-r4", "us-core"} {
		if classes[candidate] {
			t.Fatalf("destination class vocabulary admits %q", candidate)
		}
	}
}

// assertFHIROutIsTheOnlyMapperImporter proves pkg/fhir is reachable from the
// durable engine through exactly one package: internal/integration/fhirout.
// Before 4.1c-c the same walk asserted zero importers.
func assertFHIROutIsTheOnlyMapperImporter(t *testing.T) {
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
	var importers []string
	scanned := 0
	walkErr := filepath.WalkDir(integrationRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			if strings.Contains(strings.Trim(spec.Path.Value, `"`), "fi-fhir/pkg/fhir") {
				relative, _ := filepath.Rel(root, path)
				importers = append(importers, filepath.ToSlash(relative))
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
	if len(importers) == 0 {
		t.Fatal("no package under internal/integration imports pkg/fhir; the FHIR destination " +
			"class has not been built on this branch")
	}
	for _, importer := range importers {
		if !strings.HasPrefix(importer, "internal/integration/fhirout/") {
			t.Fatalf("pkg/fhir is imported by %s; internal/integration/fhirout must stay the only door "+
				"(all importers: %v)", importer, importers)
		}
	}
}

// fhirGateProcessedEvent is NewProcessedEvent output over an admit in the exact
// shape the executable ADT A01 v1 subset produces (`.loom/34` correction 3 and
// the day-1 worklog): the MRN carries the assigning-authority system the
// profile maps for HOSP, the visit number is bare because strict validation
// caps PV1.19 at one component, and the attending provider has an id only.
// PayloadJSON() of this value is byte-for-byte what the outbox row holds.
func fhirGateProcessedEvent(t *testing.T) integration.ProcessedEvent {
	t.Helper()
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "tenant-a",
		Classification: integration.DataClassificationPHI,
	}, fhirGateAdmitEvent())
	if err != nil {
		t.Fatalf("NewProcessedEvent: %v", err)
	}
	return processed
}

func fhirGateAdmitEvent() *events.PatientAdmitEvent {
	return &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:              "event-a",
			Type:            events.EventPatientAdmit,
			Timestamp:       time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
			ReceivedAt:      time.Date(2026, 7, 14, 16, 0, 1, 0, time.UTC),
			Source:          "adt-east",
			SourceFormat:    events.FormatHL7v2,
			SourceProfileID: "strict-adt-profile",
			SourceMessageID: "control-gate-001",
			CorrelationID:   "correlation-gate",
		},
		Patient: events.Patient{
			MRN: "MRN-000123",
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Value: "MRN-000123", Type: "MR", System: "urn:oid:1.2.3", Assigner: "HOSP"},
			}},
			FamilyName:  "Alpha",
			GivenName:   "Ada",
			DateOfBirth: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
			Gender:      "F",
		},
		Encounter: events.Encounter{
			ID:                  "VISIT-000123",
			Class:               "I",
			ClassifiedEventType: "patient_admit",
			AdmitDateTime:       time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
			AttendingProvider:   &events.Provider{ID: "1234567893", FamilyName: "Attending", GivenName: "Amy"},
		},
	}
}

// fhirGateDispatchFHIR runs one real dispatch of one durable work item to the
// in-test FHIR destination and returns what the destination was sent and what
// the ledger recorded.
//
// Everything on the production path is real: the dispatcher, destination.Transport
// resolving the credential and the trust roots from the deployed revision,
// fhirout projecting the stored payload, and net/http performing the call. Only
// the durable store and the broker are faked, and the broker is faked precisely
// so that a Kafka publish would be visible as a test failure rather than as a
// silent second path.
func fhirGateDispatchFHIR(
	t *testing.T, server *fhirTestServer, payload json.RawMessage,
) (fhirServedRequest, *fhirGateDeliveryRecorder) {
	t.Helper()

	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "dest-fhir-conformance", RevisionID: "destination-1",
		DestinationID: "dest-fhir-conformance",
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportFHIR,
		FHIR: &destination.FHIRPolicy{
			BaseURL: server.URL(), TokenBinding: "conformance-token", CABundleBinding: "conformance-ca",
			Interaction: destination.FHIRInteractionTransaction,
		},
		Identity: &destination.ClientIdentity{
			Subject: "conformance-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}

	recorder := &fhirGateDeliveryRecorder{}
	transport, err := destination.NewTransport(destination.TransportConfig{
		Registry: fhirGateRegistry(t, revision),
		Resolver: fhirGateSecretResolver{values: map[string]string{
			"conformance/token": "conformance-token-material",
			"conformance/ca":    server.CAPEM(),
		}},
		Recorder: recorder,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	item := testWorkItem()
	item.Action = "send-fhir"
	item.Destination = revision.Reference()
	item.EventPayload = payload

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
		t.Fatalf("outcome = %q, published = %d, failed = %d (%+v) — the destination was not delivered to",
			outcome, store.published, store.failed, store.failure)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("the destination served %d requests, want exactly 1", len(requests))
	}
	if len(publisher.message.Value) != 0 {
		t.Fatal("a fhir-class destination also published to the broker; this gate is reading the wrong path")
	}
	served := requests[0]
	if served.Method != http.MethodPost || served.Path != "/" && served.Path != "" {
		t.Fatalf("method/path = %s %q, want POST at the FHIR base", served.Method, served.Path)
	}
	if served.Status != http.StatusOK {
		t.Fatalf("destination answered %d: %s", served.Status, served.Body)
	}
	return served, recorder
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
