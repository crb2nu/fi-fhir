package destination

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// fhirScriptedServer answers every request with one scripted status and body
// and records what it was sent.
type fhirScriptedServer struct {
	mu       sync.Mutex
	server   *httptest.Server
	status   int
	body     string
	requests []*http.Request
	bodies   [][]byte
}

func newFHIRScriptedServer(t *testing.T, status int, body string) *fhirScriptedServer {
	t.Helper()
	scripted := &fhirScriptedServer{status: status, body: body}
	scripted.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		scripted.mu.Lock()
		scripted.requests = append(scripted.requests, r.Clone(context.Background()))
		scripted.bodies = append(scripted.bodies, payload)
		scripted.mu.Unlock()
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(scripted.status)
		_, _ = io.WriteString(w, scripted.body)
	}))
	scripted.server.StartTLS()
	t.Cleanup(scripted.server.Close)
	return scripted
}

func (s *fhirScriptedServer) caPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.server.Certificate().Raw}))
}

func (s *fhirScriptedServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func transportTestFHIRRevision(t *testing.T, artifactID, baseURL, token, caBundle string) Revision {
	t.Helper()
	revision, err := NewRevision(RevisionInput{
		ArtifactID: artifactID, RevisionID: "destination-1", DestinationID: artifactID,
		Class: integration.DestinationClassProduction, Transport: TransportFHIR,
		FHIR: &FHIRPolicy{
			BaseURL: baseURL, TokenBinding: token, CABundleBinding: caBundle,
			Interaction: FHIRInteractionTransaction,
		},
		Identity: &ClientIdentity{
			Subject: artifactID + "-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(%s): %v", artifactID, err)
	}
	return revision
}

// fhirTransportFixture stands one fhir destination at a scripted server.
type fhirTransportFixture struct {
	server    *fhirScriptedServer
	transport *Transport
	revision  Revision
	recorder  *recordingDeliveryRecorder
}

func newFHIRTransportFixture(t *testing.T, status int, body string) fhirTransportFixture {
	t.Helper()
	server := newFHIRScriptedServer(t, status, body)
	revision := transportTestFHIRRevision(t, "dest-fhir", server.server.URL, "fhir-token", "fhir-ca")
	registry := newTransportTestRegistry(t, map[string]string{"fhir-token": "token", "fhir-ca": "ca"}, revision)
	recorder := &recordingDeliveryRecorder{}
	transport := newTransportForTest(t, registry,
		&mapSecretResolver{values: map[string]string{"token": "fhir-bearer-material\n", "ca": server.caPEM()}},
		recorder)
	return fhirTransportFixture{server: server, transport: transport, revision: revision, recorder: recorder}
}

func (f fhirTransportFixture) deliver(t *testing.T, attemptID string, payload []byte) (bool, error) {
	t.Helper()
	return f.transport.DeliverDestination(
		context.Background(), "tenant-a", attemptID, f.revision.Reference(), nil, payload,
	)
}

func fhirTransportTestPayload(t *testing.T, mutate func(*events.PatientAdmitEvent)) []byte {
	t.Helper()
	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID: "event-fhir", Type: events.EventPatientAdmit,
			Timestamp: time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC), ReceivedAt: time.Date(2026, 7, 14, 16, 0, 1, 0, time.UTC),
			Source: "adt-east", SourceFormat: events.FormatHL7v2, SourceProfileID: "strict-adt-profile",
			SourceMessageID: "control-fhir", CorrelationID: "correlation-fhir",
		},
		Patient: events.Patient{
			MRN: "MRN-000123",
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Value: "MRN-000123", Type: "MR", System: "urn:oid:1.2.3"},
			}},
			FamilyName: "Alpha", GivenName: "Ada",
			DateOfBirth: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC), Gender: "F",
		},
		Encounter: events.Encounter{ID: "VISIT-000123", Class: "I", Status: "admitted"},
	}
	if mutate != nil {
		mutate(event)
	}
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID: "tenant-a", Classification: integration.DataClassificationPHI,
	}, event)
	if err != nil {
		t.Fatalf("NewProcessedEvent: %v", err)
	}
	return processed.PayloadJSON()
}

const fhirTransactionResponseTwoCreated = `{"resourceType":"Bundle","type":"transaction-response","entry":[` +
	`{"response":{"status":"201 Created"}},{"response":{"status":"201 Created"}}]}`

func TestFHIRTransportDeliversAConditionalBundleUnderTheDeclaredIdentity(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusOK, fhirTransactionResponseTwoCreated)

	owned, err := fixture.deliver(t, "attempt-fhir-1", fhirTransportTestPayload(t, nil))
	if !owned || err != nil {
		t.Fatalf("DeliverDestination = %v, %v; want owned with no error", owned, err)
	}
	if fixture.server.count() != 1 {
		t.Fatalf("server saw %d requests, want 1", fixture.server.count())
	}
	request := fixture.server.requests[0]
	if request.Method != http.MethodPost || request.URL.Path != "/" {
		t.Fatalf("request = %s %s, want POST at the base", request.Method, request.URL.Path)
	}
	for header, want := range map[string]string{
		"Content-Type":    "application/fhir+json; charset=utf-8",
		"Accept":          "application/fhir+json",
		"Prefer":          "return=minimal",
		"Idempotency-Key": "attempt-fhir-1",
		"Authorization":   "Bearer fhir-bearer-material",
	} {
		if got := request.Header.Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
	var bundle struct {
		Type  string `json:"type"`
		Entry []struct {
			Request struct {
				Method, URL string
			} `json:"request"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(fixture.server.bodies[0], &bundle); err != nil || bundle.Type != "transaction" || len(bundle.Entry) != 2 {
		t.Fatalf("body is not a two-entry transaction bundle: %v %s", err, fixture.server.bodies[0])
	}
	for index, entry := range bundle.Entry {
		if entry.Request.Method != http.MethodPut || !strings.Contains(entry.Request.URL, "?identifier=") {
			t.Fatalf("entry %d request = %+v, want a conditional PUT", index, entry.Request)
		}
	}

	if len(fixture.recorder.records) != 1 {
		t.Fatalf("recorded %d deliveries, want 1", len(fixture.recorder.records))
	}
	record := fixture.recorder.records[0]
	if record.Transport != TransportFHIR || record.Outcome != outcomeDelivered || record.HTTPStatusClass != "2xx" ||
		record.FHIRResourceTypes != "Patient,Encounter" || record.FHIREntryCount != 2 || record.FHIROutcomeCodesAdvisory != "" ||
		record.EndpointAdvisory != fixture.server.server.URL || record.ServedCertificateSubjectAdvisory == "" {
		t.Fatalf("record = %+v", record)
	}
}

func TestFHIRTransportRecordsIssueCodesAndNeverDiagnostics(t *testing.T) {
	const diagnostics = "Patient MRN-000123 Alpha, Ada failed validation at Patient.birthDate"
	fixture := newFHIRTransportFixture(t, http.StatusBadRequest,
		`{"resourceType":"OperationOutcome","issue":[`+
			`{"severity":"error","code":"Invalid!!","diagnostics":"`+diagnostics+`"},`+
			`{"severity":"error","code":"NOT-found","details":{"text":"`+diagnostics+`"}},`+
			`{"severity":"error","code":"invalid"}]}`)

	owned, err := fixture.deliver(t, "attempt-fhir-refused", fhirTransportTestPayload(t, nil))
	var failure *TransportError
	if !owned || !errors.As(err, &failure) || failure.Code != FailureRejected || failure.Retryable {
		t.Fatalf("DeliverDestination = %v, %v; want a terminal %s", owned, err, FailureRejected)
	}
	record := fixture.recorder.records[0]
	if record.Outcome != outcomeRefused || record.FailureCode != FailureRejected || record.HTTPStatusClass != "4xx" {
		t.Fatalf("record = %+v", record)
	}
	if record.FHIROutcomeCodesAdvisory != "invalid,not-found" {
		t.Fatalf("outcome codes = %q, want the sanitised, sorted, deduplicated codes", record.FHIROutcomeCodesAdvisory)
	}
	encoded, _ := json.Marshal(record)
	for _, forbidden := range []string{"MRN-000123", "Alpha", "birthDate", "failed validation"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("the delivery record carries destination diagnostics text %q: %s", forbidden, encoded)
		}
	}
	if strings.Contains(failure.Detail, "MRN-000123") || len(failure.Detail) > 512 {
		t.Fatalf("failure detail carries response content: %q", failure.Detail)
	}
	if record.FHIRResourceTypes != "Patient,Encounter" || record.FHIREntryCount != 2 {
		t.Fatalf("a refused delivery still records what was sent: %+v", record)
	}
}

func TestFHIRTransportRetriesAnUnavailableDestination(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusServiceUnavailable, `busy`)

	owned, err := fixture.deliver(t, "attempt-fhir-503", fhirTransportTestPayload(t, nil))
	var failure *TransportError
	if !owned || !errors.As(err, &failure) || failure.Code != FailureUnavailable || !failure.Retryable {
		t.Fatalf("DeliverDestination = %v, %v; want a retryable %s", owned, err, FailureUnavailable)
	}
	record := fixture.recorder.records[0]
	if record.Outcome != outcomeRetryable || record.HTTPStatusClass != "5xx" ||
		record.FHIROutcomeCodesAdvisory != outcomeCodeResponseUnparsed {
		t.Fatalf("record = %+v", record)
	}
}

func TestFHIRTransportRefusesACommittedTransactionWithAFailingEntry(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusOK,
		`{"resourceType":"Bundle","type":"transaction-response","entry":[`+
			`{"response":{"status":"200 OK"}},{"response":{"status":"422 Unprocessable Entity"}}]}`)

	owned, err := fixture.deliver(t, "attempt-fhir-entry", fhirTransportTestPayload(t, nil))
	var failure *TransportError
	if !owned || !errors.As(err, &failure) || failure.Code != FailureRejected || failure.Retryable {
		t.Fatalf("DeliverDestination = %v, %v; want a terminal %s", owned, err, FailureRejected)
	}
	if got := fixture.recorder.records[0].FHIROutcomeCodesAdvisory; got != "entry-4xx" {
		t.Fatalf("outcome codes = %q, want entry-4xx", got)
	}
}

func TestFHIRTransportTreatsAnUnreadable2xxAsDeliveredButSaysSo(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusOK, `<html>not fhir</html>`)

	owned, err := fixture.deliver(t, "attempt-fhir-unparsed", fhirTransportTestPayload(t, nil))
	if !owned || err != nil {
		t.Fatalf("DeliverDestination = %v, %v", owned, err)
	}
	record := fixture.recorder.records[0]
	if record.Outcome != outcomeDelivered || record.FHIROutcomeCodesAdvisory != outcomeCodeResponseUnparsed {
		t.Fatalf("record = %+v", record)
	}
}

func TestFHIRTransportRefusesAProjectionItCannotKeyWithoutARequest(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusOK, fhirTransactionResponseTwoCreated)

	cases := map[string][]byte{
		"no visit number": fhirTransportTestPayload(t, func(e *events.PatientAdmitEvent) {
			e.Encounter = events.Encounter{Class: "I"}
		}),
		"unsupported event type": func() []byte {
			processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
				TenantID: "tenant-a", Classification: integration.DataClassificationPHI,
			}, &events.DocumentReferenceEvent{
				EventMeta: events.EventMeta{
					ID: "event-document", Type: events.EventType("document_reference"), Timestamp: time.Now(), ReceivedAt: time.Now(),
					Source: "adt-east", SourceFormat: events.FormatHL7v2, SourceMessageID: "control-document", CorrelationID: "c",
				},
				Patient: &events.Patient{MRN: "MRN-000123"},
			})
			if err != nil {
				t.Fatalf("NewProcessedEvent(vital): %v", err)
			}
			return processed.PayloadJSON()
		}(),
		"not a canonical payload": []byte(`{"schema":"integration.delivery.v1"}`),
	}
	for name, payload := range cases {
		owned, err := fixture.deliver(t, "attempt-"+strings.ReplaceAll(name, " ", "-"), payload)
		var failure *TransportError
		if !owned || !errors.As(err, &failure) || failure.Code != FailureProjection || failure.Retryable {
			t.Fatalf("%s: DeliverDestination = %v, %v; want a terminal %s", name, owned, err, FailureProjection)
		}
		if strings.Contains(failure.Detail, "MRN") || strings.Contains(failure.Detail, "VISIT") {
			t.Fatalf("%s: failure detail carries event content: %q", name, failure.Detail)
		}
	}
	if fixture.server.count() != 0 {
		t.Fatalf("a refused projection reached the destination (%d requests)", fixture.server.count())
	}
	if len(fixture.recorder.records) != len(cases) {
		t.Fatalf("recorded %d refusals, want %d — a refused projection is still provenance", len(fixture.recorder.records), len(cases))
	}
	for _, record := range fixture.recorder.records {
		if record.Outcome != outcomeRefused || record.FailureCode != FailureProjection || record.Transport != TransportFHIR {
			t.Fatalf("record = %+v", record)
		}
	}
}

func TestFHIRTransportIgnoresAnEmptyEventPayload(t *testing.T) {
	fixture := newFHIRTransportFixture(t, http.StatusOK, fhirTransactionResponseTwoCreated)
	owned, err := fixture.transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-empty", fixture.revision.Reference(),
		[]byte(`{"schema":"integration.delivery.v1"}`), nil,
	)
	var failure *TransportError
	if !owned || !errors.As(err, &failure) || failure.Code != FailureUnconfigured {
		t.Fatalf("DeliverDestination = %v, %v; want %s", owned, err, FailureUnconfigured)
	}
	if fixture.server.count() != 0 {
		t.Fatal("an empty event payload reached the destination")
	}
}

func TestOperationOutcomeCodesAreSanitisedAndBounded(t *testing.T) {
	t.Parallel()
	if got := operationOutcomeCodes(nil); got != "" {
		t.Fatalf("empty body = %q", got)
	}
	if got := operationOutcomeCodes([]byte(`{"resourceType":"Bundle"}`)); got != outcomeCodeResponseUnparsed {
		t.Fatalf("non-outcome body = %q", got)
	}
	long := strings.Repeat(`{"code":"`+strings.Repeat("a", 40)+`"},`, 40)
	got := operationOutcomeCodes([]byte(`{"resourceType":"OperationOutcome","issue":[` + strings.TrimSuffix(long, ",") + `]}`))
	if len(got) > maxFHIRLedgerBytes || strings.Count(got, ",") > maxOutcomeCodes-1 {
		t.Fatalf("codes not bounded: %d bytes, %q", len(got), got)
	}
	for _, code := range strings.Split(got, ",") {
		if len(code) > maxOutcomeCodeBytes {
			t.Fatalf("code %q exceeds %d bytes", code, maxOutcomeCodeBytes)
		}
	}
	if got := entryStatusClass("201 Created"); got != "2xx" {
		t.Fatalf("entryStatusClass = %q", got)
	}
	if got := entryStatusClass("nope"); got != "" {
		t.Fatalf("entryStatusClass(nope) = %q", got)
	}
}
