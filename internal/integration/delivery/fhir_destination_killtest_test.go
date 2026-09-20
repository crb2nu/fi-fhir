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

// History: this file held TestFHIRDestination_RedeliveryDuplicatesToday, Slice
// 4.1c-c's second day-1 kill-test, which PASSED on `main` @ a3335a71f by
// asserting the pre-slice fact: the mapper's Patient and Encounter through
// pkg/fhir.CreateTransactionBundle (every entry `POST <Type>`), delivered
// TWICE under the SAME attempt id through the real destination.Transport to an
// identifier-keyed in-test FHIR server, left **2 Patients and 2 Encounters**.
// The outbox is at-least-once and no FHIR server honours `Idempotency-Key`, so
// reusing the bundle builder would have duplicated every patient on the first
// lease reclaim (`.loom/34` correction 5).
//
// TestFHIRDestination_RedeliveryIsIdempotent below is that test inverted: the
// same two deliveries through the `fhir` transport leave exactly one of each.

// TestFHIRDestination_RedeliveryIsIdempotent proves the second half of the
// sprint's riskiest assumption: a FHIR write built from the stored payload can
// be redelivered without creating duplicates.
//
// The stored payload of one admit is delivered twice under one attempt id
// through the real `fhir` transport. Every entry the destination receives is a
// conditional `PUT <Type>?identifier=<system>|<value>`; the first delivery
// creates (201) and the second updates in place (200); the store holds one
// Patient and one Encounter.
//
// Negative control (`make fhir-destination-negative-control`): built with
// `-tags fhirpostbundle`, fhirout's entry builder reverts to `POST <Type>` and
// this test MUST fail on exactly `want 1 Patient, got 2`. A control that still
// passes means the server is not keyed the way this test assumes and the count
// proves nothing.
//
// The tail keeps two rules exercised against the same server: a projection
// with no usable identifier is refused before any request is made (the lane's
// riskiest assumption — never a POST), and the server's refusal of a bare
// `?identifier=<value>` stays live so the "no system" rule is enforced by the
// destination, not assumed by the client.
func TestFHIRDestination_RedeliveryIsIdempotent(t *testing.T) {
	server := newFHIRTestServer(t)
	transport, revision := fhirKillTestFHIRTransport(t, server)
	payload := fhirKillTestPayload(t, fhirKillTestAdmitEvent())

	const attemptID = "attempt-fhir-redelivery"
	for redelivery := 1; redelivery <= 2; redelivery++ {
		handled, err := transport.DeliverDestination(
			context.Background(), "tenant-a", attemptID, revision.Reference(), nil, payload,
		)
		if !handled || err != nil {
			t.Fatalf("delivery %d: handled=%v err=%v — the fhir transport did not deliver the projection",
				redelivery, handled, err)
		}
	}

	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("the FHIR server served %d requests, want exactly 2 (one per redelivery)", len(requests))
	}

	// The inverted fact, asserted first so the negative control fails on the
	// count and not on a request-shape check further down. The control must
	// turn both of these into 2.
	if got := server.Count("Patient"); got != 1 {
		t.Fatalf("want 1 Patient, got %d after two deliveries of the same attempt — redelivery is not idempotent", got)
	}
	if got := server.Count("Encounter"); got != 1 {
		t.Fatalf("want 1 Encounter, got %d after two deliveries of the same attempt — redelivery is not idempotent", got)
	}

	for index, request := range requests {
		if request.IdempotencyKey != attemptID {
			t.Fatalf("request %d Idempotency-Key = %q, want %q — the two deliveries are not the same attempt",
				index, request.IdempotencyKey, attemptID)
		}
		if request.Status != http.StatusOK {
			t.Fatalf("request %d answered %d, want 200: %s", index, request.Status, request.Body)
		}
		if request.ContentType != "application/fhir+json; charset=utf-8" {
			t.Fatalf("request %d Content-Type = %q, want application/fhir+json; charset=utf-8", index, request.ContentType)
		}
		if len(request.EntryMethods) != 2 {
			t.Fatalf("request %d carried %d entries, want 2 (Patient, Encounter)", index, len(request.EntryMethods))
		}
		for entryIndex, method := range request.EntryMethods {
			if method != http.MethodPut {
				t.Fatalf("request %d entry %d method = %q, want PUT (a conditional update)", index, entryIndex, method)
			}
			if _, _, ok := fhirGateConditionalKey(request.EntryURLs[entryIndex], fhirKillTestEntryType(entryIndex)); !ok {
				t.Fatalf("request %d entry %d url = %q, want <Type>?identifier=<system>|<value>",
					index, entryIndex, request.EntryURLs[entryIndex])
			}
		}
	}

	for entryIndex, status := range requests[0].EntryStatuses {
		if status != "201 Created" {
			t.Fatalf("first delivery entry %d status = %q, want 201 Created", entryIndex, status)
		}
	}
	for entryIndex, status := range requests[1].EntryStatuses {
		if status != "200 OK" {
			t.Fatalf("second delivery entry %d status = %q, want 200 OK (updated in place)", entryIndex, status)
		}
	}

	// A projection with no usable identifier is refused before any request:
	// an admit whose PV1 carried no visit number has an Encounter nothing can
	// key on, and the transport must not fall back to a POST.
	orphan := fhirKillTestAdmitEvent()
	orphan.Encounter = events.Encounter{Class: "I"}
	handled, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-fhir-orphan-encounter", revision.Reference(),
		nil, fhirKillTestPayload(t, orphan),
	)
	var failure *destination.TransportError
	if !handled || !errors.As(err, &failure) || failure.Code != destination.FailureProjection || failure.Retryable {
		t.Fatalf("orphan Encounter: handled=%v err=%v, want a terminal %s with no request made",
			handled, err, destination.FailureProjection)
	}
	if got := len(server.Requests()); got != 2 {
		t.Fatalf("the refused projection reached the destination (%d requests, want 2 unchanged)", got)
	}
	if got := server.Count("Patient"); got != 1 {
		t.Fatalf("Patient count after the refused projection = %d, want 1", got)
	}

	// The server's own rule: a conditional write without a system is refused
	// with 400, surfaced through the https transport as a terminal rejection.
	httpsTransport, httpsRevision := fhirKillTestHTTPSTransport(t, server)
	patient := fhir.NewUSCoreMapper().MapPatient(&fhirKillTestAdmitEvent().Patient)
	bareBundle := fhir.Bundle{
		ResourceType: "Bundle",
		Type:         "transaction",
		Entry: []fhir.BundleEntry{{
			Resource: mustMarshalFHIR(t, patient),
			Request:  &fhir.BundleEntryRequest{Method: http.MethodPut, URL: "Patient?identifier=MRN-000123"},
		}},
	}
	handled, err = httpsTransport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-fhir-bare-identifier", httpsRevision.Reference(),
		mustMarshalFHIR(t, &bareBundle), nil,
	)
	if !handled || !errors.As(err, &failure) || failure.Code != destination.FailureRejected || failure.Retryable {
		t.Fatalf("bare-value conditional write: handled=%v err=%v, want a terminal %s — the in-test server "+
			"must refuse `?identifier=<value>` with 400 so the no-system rule is enforced, not assumed",
			handled, err, destination.FailureRejected)
	}
	if got := server.Count("Patient"); got != 1 {
		t.Fatalf("Patient count after the refused bare-identifier write = %d, want 1 (transaction is all-or-nothing)", got)
	}
}

func fhirKillTestEntryType(index int) string {
	if index == 0 {
		return "Patient"
	}
	return "Encounter"
}

const fhirKillTestMRNSystem = "urn:oid:1.2.3"

// fhirKillTestAdmitEvent is a representative admit in the shape the executable
// A01 subset produces: the MRN carries the assigning-authority system the
// golden profile maps for HOSP; the visit number is bare, because strict
// validation caps PV1.19 at one component.
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
			ID:            "VISIT-000123",
			Class:         "I",
			Status:        "admitted",
			AdmitDateTime: time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		},
	}
}

// fhirKillTestPayload is PayloadJSON() of NewProcessedEvent over the event —
// the bytes the outbox row holds.
func fhirKillTestPayload(t *testing.T, event *events.PatientAdmitEvent) json.RawMessage {
	t.Helper()
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "tenant-a",
		Classification: integration.DataClassificationPHI,
	}, event)
	if err != nil {
		t.Fatalf("NewProcessedEvent: %v", err)
	}
	return processed.PayloadJSON()
}

// fhirKillTestFHIRTransport deploys one `fhir`-transport destination pointing
// at the in-test FHIR server.
func fhirKillTestFHIRTransport(t *testing.T, server *fhirTestServer) (*destination.Transport, destination.Revision) {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "dest-fhir-killtest", RevisionID: "destination-1",
		DestinationID: "dest-fhir-killtest",
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
	return fhirKillTestTransport(t, server, revision), revision
}

// fhirKillTestHTTPSTransport deploys one `https`-transport destination pointing
// at the same server, for the checks that need to send a hand-built bundle.
func fhirKillTestHTTPSTransport(t *testing.T, server *fhirTestServer) (*destination.Transport, destination.Revision) {
	t.Helper()
	revision, err := destination.NewRevision(destination.RevisionInput{
		ArtifactID: "dest-https-killtest", RevisionID: "destination-1",
		DestinationID: "dest-https-killtest",
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
	return fhirKillTestTransport(t, server, revision), revision
}

func fhirKillTestTransport(t *testing.T, server *fhirTestServer, revision destination.Revision) *destination.Transport {
	t.Helper()
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
	return transport
}

func mustMarshalFHIR(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	return encoded
}
