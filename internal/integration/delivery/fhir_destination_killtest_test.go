package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestFHIRDestination_RedeliveryDuplicatesToday is Slice 4.1c-c's second
// day-1 kill-test. It PASSES on unmodified `main` by asserting today's
// behaviour, and the implementation MUST invert it.
//
// `.loom/34` correction 5: `pkg/fhir.CreateTransactionBundle` emits every entry
// as `POST <ResourceType>` (mapper.go:1288-1305). The outbox is at-least-once —
// a lease reclaim redelivers the same attempt — and the HTTPS transport's only
// duplicate defence is `Idempotency-Key: <attemptID>`, a header no FHIR server
// honours. So reusing the bundle builder as-is creates a second Patient and a
// second Encounter on every redelivery. This is the sprint's riskiest
// assumption, second half.
//
// The spec asks for a test that FAILS on main with `want 1 Patient, got 2`. A
// red test cannot merge under pipeline-must-succeed, so the same fact is
// encoded as a passing assertion of today's behaviour: the mapper's Patient and
// Encounter go through CreateTransactionBundle, the bundle is delivered TWICE
// under the SAME attempt id through the real destination.Transport to an
// in-test FHIR server whose store is keyed on identifier, and the store must
// hold exactly 2 Patients and 2 Encounters.
//
// After 4.1c-c the `fhir` transport emits conditional
// `PUT <Type>?identifier=<system>|<value>` entries, the count must be 1, and
// this test is renamed TestFHIRDestination_RedeliveryIsIdempotent with the
// inverted assertion. Its negative control keeps the POST builder behind a
// build tag and requires the count to return to 2 — a control that stays at 1
// means the server is not counting what the test thinks it is.
//
// The second half exercises the in-test server's refusal of a bare-value
// conditional write (`?identifier=<value>` with no system). That rule must be
// live on day 1 so the projection's "no usable identifier → projection error,
// never a POST" refusal (lane riskiest assumption) can be tested against a
// server that enforces it rather than assumed.
func TestFHIRDestination_RedeliveryDuplicatesToday(t *testing.T) {
	server := newFHIRTestServer(t)
	transport, revision := fhirKillTestHTTPSTransport(t, server)

	mapper := fhir.NewUSCoreMapper()
	event := fhirKillTestAdmitEvent()
	patient := mapper.MapPatient(&event.Patient)
	encounter := mapper.MapEncounter(&event.Encounter, "Patient/"+event.Patient.MRN)
	bundle := fhir.CreateTransactionBundle([]fhir.Resource{patient, encounter})
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal transaction bundle: %v", err)
	}

	const attemptID = "attempt-fhir-redelivery"
	for redelivery := 1; redelivery <= 2; redelivery++ {
		handled, err := transport.DeliverDestination(
			context.Background(), "tenant-a", attemptID, revision.Reference(), body,
		)
		if !handled || err != nil {
			t.Fatalf("delivery %d: handled=%v err=%v — the https transport did not deliver the bundle",
				redelivery, handled, err)
		}
	}

	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("the FHIR server served %d requests, want exactly 2 (one per redelivery)", len(requests))
	}
	for index, request := range requests {
		if request.IdempotencyKey != attemptID {
			t.Fatalf("request %d Idempotency-Key = %q, want %q — the two deliveries are not the same attempt",
				index, request.IdempotencyKey, attemptID)
		}
		if request.Status != http.StatusOK {
			t.Fatalf("request %d answered %d, want 200: %s", index, request.Status, request.Body)
		}
		for entryIndex, method := range request.EntryMethods {
			if method != http.MethodPost {
				t.Fatalf("request %d entry %d method = %q; CreateTransactionBundle emits POST today, "+
					"and a non-POST here means the builder changed without this test being inverted",
					index, entryIndex, method)
			}
		}
	}

	// Today's fact. The implementation must make both of these 1.
	if got := server.Count("Patient"); got != 2 {
		t.Fatalf("Patient count after two deliveries of the same attempt = %d; today's POST "+
			"builder produces 2, and 4.1c-c's conditional writes must produce 1. A value that is "+
			"neither means the in-test server is not keyed the way this test assumes", got)
	}
	if got := server.Count("Encounter"); got != 2 {
		t.Fatalf("Encounter count after two deliveries of the same attempt = %d, want 2 today (1 after 4.1c-c)", got)
	}

	// The server refuses a conditional write whose identifier has no system, and
	// the transport surfaces that 400 as a terminal rejection.
	bareBundle := fhir.Bundle{
		ResourceType: "Bundle",
		Type:         "transaction",
		Entry: []fhir.BundleEntry{{
			Resource: mustMarshalFHIR(t, patient),
			Request:  &fhir.BundleEntryRequest{Method: http.MethodPut, URL: "Patient?identifier=" + event.Patient.MRN},
		}},
	}
	bareBody := mustMarshalFHIR(t, &bareBundle)
	handled, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-fhir-bare-identifier", revision.Reference(), bareBody,
	)
	if !handled {
		t.Fatal("the https transport did not own the bare-identifier delivery")
	}
	var failure *destination.TransportError
	if !errors.As(err, &failure) || failure.Code != destination.FailureRejected || failure.Retryable {
		t.Fatalf("bare-value conditional write: err = %v, want a terminal %s — the in-test server "+
			"must refuse `?identifier=<value>` with 400 so the no-system rule is exercised, not assumed",
			err, destination.FailureRejected)
	}
	if got := server.Count("Patient"); got != 2 {
		t.Fatalf("Patient count after the refused bare-identifier write = %d, want 2 (transaction is all-or-nothing)", got)
	}

	// A conditional write WITH a system is what the server accepts and what
	// makes the second delivery a no-op. This is the shape 4.1c-c must emit, and
	// proving the server honours it today is what makes the count-1 assertion
	// meaningful tomorrow. A second patient is used so the first conditional
	// write is a create and the second an in-place update.
	other := fhirKillTestAdmitEvent()
	other.Patient.MRN = "MRN-000999"
	other.Patient.Identifiers.Identifiers[0].Value = "MRN-000999"
	otherPatient := mapper.MapPatient(&other.Patient)
	systemBundle := fhir.Bundle{
		ResourceType: "Bundle",
		Type:         "transaction",
		Entry: []fhir.BundleEntry{{
			Resource: mustMarshalFHIR(t, otherPatient),
			Request: &fhir.BundleEntryRequest{
				Method: http.MethodPut,
				URL:    "Patient?identifier=" + fhirKillTestMRNSystem + "|" + other.Patient.MRN,
			},
		}},
	}
	systemBody := mustMarshalFHIR(t, &systemBundle)
	for redelivery := 1; redelivery <= 2; redelivery++ {
		handled, err := transport.DeliverDestination(
			context.Background(), "tenant-a", "attempt-fhir-conditional", revision.Reference(), systemBody,
		)
		if !handled || err != nil {
			t.Fatalf("conditional delivery %d: handled=%v err=%v", redelivery, handled, err)
		}
	}
	// 2 from the POST deliveries + 1 created by the first conditional write; the
	// second conditional write updated it in place.
	if got := server.Count("Patient"); got != 3 {
		t.Fatalf("Patient count after two conditional deliveries = %d, want 3 — the in-test server "+
			"does not update in place on `PUT ?identifier=system|value`, so it cannot prove idempotency", got)
	}
	requests = server.Requests()
	if len(requests) != 5 {
		t.Fatalf("served %d requests, want 5 (2 POST deliveries, 1 refused, 2 conditional)", len(requests))
	}
	if got := requests[3].EntryStatuses; len(got) != 1 || got[0] != "201 Created" {
		t.Fatalf("first conditional write entry status = %v, want [201 Created]", got)
	}
	if got := requests[4].EntryStatuses; len(got) != 1 || got[0] != "200 OK" {
		t.Fatalf("second conditional write entry status = %v, want [200 OK] (updated in place)", got)
	}
}

const fhirKillTestMRNSystem = "urn:oid:1.2.3"

// fhirKillTestAdmitEvent is a representative admit: the MRN carries an
// assigning-authority system (the shape the golden tolerant profile produces
// for HOSP), and the encounter carries a visit number with a system.
func fhirKillTestAdmitEvent() *events.PatientAdmitEvent {
	return &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:              "event-fhir-killtest",
			Type:            events.EventPatientAdmit,
			Timestamp:       time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
			ReceivedAt:      time.Date(2026, 7, 14, 16, 0, 1, 0, time.UTC),
			Source:          "adt-east",
			SourceFormat:    events.FormatHL7v2,
			SourceProfileID: "strict-adt-profile",
			SourceMessageID: "control-fhir-killtest",
			CorrelationID:   "correlation-fhir-killtest",
		},
		Patient: events.Patient{
			MRN: "MRN-000123",
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Value: "MRN-000123", Type: "MR", System: fhirKillTestMRNSystem, Assigner: "HOSP"},
			}},
			FamilyName:  "Alpha",
			GivenName:   "Ada",
			DateOfBirth: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
			Gender:      "F",
		},
		Encounter: events.Encounter{
			ID: "VISIT-000123",
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Value: "VISIT-000123", Type: "VN", System: "urn:oid:1.2.3.4", Assigner: "HOSP"},
			}},
			Class:         "I",
			Status:        "admitted",
			AdmitDateTime: time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		},
	}
}

// fhirKillTestHTTPSTransport deploys one `https`-transport destination pointing
// at the in-test FHIR server, under the same registry, resolver, and recorder
// shape the 5.1a gate uses.
func fhirKillTestHTTPSTransport(t *testing.T, server *fhirTestServer) (*destination.Transport, destination.Revision) {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "dest-fhir-killtest", RevisionID: "destination-1",
		DestinationID: "dest-fhir-killtest",
		Class:         integration.DestinationClassProduction,
		Transport:     destination.TransportHTTPS,
		HTTPS: &destination.HTTPSPolicy{
			URL: server.URL(), Method: http.MethodPost,
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
	transport, err := destination.NewTransport(destination.TransportConfig{
		Registry: fhirGateRegistry(t, revision),
		Resolver: fhirGateSecretResolver{values: map[string]string{
			"conformance/token": "conformance-token-material",
			"conformance/ca":    server.CAPEM(),
		}},
		Recorder: &fhirGateDeliveryRecorder{},
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return transport, revision
}

func mustMarshalFHIR(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	return encoded
}
