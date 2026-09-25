package fhirout

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestProjectSupportsExactlyTheRegisteredTypes pins the agreement between the
// closed projectable set and pkg/integration's canonical registry: every
// supported type is registered and decodes into a struct ProjectEvent handles,
// and a registered type outside the set is refused as unsupported rather than
// failing somewhere deeper.
func TestProjectSupportsExactlyTheRegisteredTypes(t *testing.T) {
	t.Parallel()

	if want := []events.EventType{
		events.EventAllergyIntolerance, events.EventCondition, events.EventImmunization,
		events.EventLabResult, events.EventMedicationRequest, events.EventPatientAdmit, events.EventPatientDischarge,
		events.EventPatientTransfer, events.EventPatientUpdate, events.EventProcedure, events.EventVitalSign,
	}; strings.Join(eventTypeStrings(SupportedEventTypes()), ",") != strings.Join(eventTypeStrings(want), ",") {
		t.Fatalf("SupportedEventTypes = %v, want %v", SupportedEventTypes(), want)
	}
	for _, eventType := range SupportedEventTypes() {
		if !integration.CanonicalEventRegistered(eventType) {
			t.Fatalf("%s is projectable but has no canonical schema", eventType)
		}
		payload := projectTestPayload(t, projectTestEventFor(t, eventType))
		projection, err := Project(eventType, payload)
		if err != nil {
			t.Fatalf("Project(%s): %v", eventType, err)
		}
		if projection.EventType != eventType || len(projection.Resources) == 0 {
			t.Fatalf("Project(%s) = %+v", eventType, projection)
		}
	}

	document := projectTestPayload(t, &events.DocumentReferenceEvent{
		EventMeta: projectTestMeta(events.EventType("document_reference")),
		Patient:   &events.Patient{MRN: "MRN-000123"},
	})
	if _, err := Project(events.EventType("document_reference"), document); !errors.Is(err, ErrUnsupportedEventType) {
		t.Fatalf("Project(document_sign) = %v, want ErrUnsupportedEventType", err)
	}
	if Supports(events.EventType("document_reference")) {
		t.Fatal("Supports(document_sign) = true")
	}
}

// TestProjectAdmissionKeysAndConditionalBundle is the projection's own proof
// over the shape the executable A01 subset produces: a system-qualified MRN and
// a bare visit number.
func TestProjectAdmissionKeysAndConditionalBundle(t *testing.T) {
	t.Parallel()

	event := projectTestAdmit()
	payload := projectTestPayload(t, event)
	projection, err := Project(events.EventPatientAdmit, payload)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	again, err := Project(events.EventPatientAdmit, payload)
	if err != nil {
		t.Fatalf("Project (second): %v", err)
	}

	if got := projection.ResourceTypes(); strings.Join(got, ",") != "Patient,Encounter" {
		t.Fatalf("ResourceTypes = %v", got)
	}
	patient, encounter := projection.Resources[0], projection.Resources[1]
	if patient.Key != (fhir.Identifier{System: "urn:oid:1.2.3", Value: "MRN-000123"}) {
		t.Fatalf("Patient key = %+v, want the profile-mapped system and the MRN", patient.Key)
	}
	if encounter.Key != (fhir.Identifier{System: "urn:fi-fhir:source:adt-east", Value: "VISIT-000123"}) {
		t.Fatalf("Encounter key = %+v, want the source-owned system over the bare visit number", encounter.Key)
	}
	for index := range projection.Resources {
		if projection.Resources[index].FullURL != again.Resources[index].FullURL ||
			!strings.HasPrefix(projection.Resources[index].FullURL, "urn:uuid:") {
			t.Fatalf("fullUrl %d is not deterministic: %q vs %q",
				index, projection.Resources[index].FullURL, again.Resources[index].FullURL)
		}
	}
	if patient.FullURL == encounter.FullURL {
		t.Fatal("Patient and Encounter share a fullUrl")
	}

	// The mapper's output is untouched: told the event's source, as mapEvent
	// tells it since Sprint 7, the mapper alone produces byte for byte what the
	// projection carries — the legacy engine gets exactly the mapper's output.
	mapper := fhir.NewUSCoreMapper()
	mapper.Source = event.Source
	wantEncounter, _ := json.Marshal(mapper.MapEncounter(&event.Encounter, "Patient/MRN-000123"))
	gotEncounter, _ := json.Marshal(encounter.Resource)
	if !bytes.Equal(wantEncounter, gotEncounter) {
		t.Fatalf("projection altered the mapper's Encounter:\nwant %s\ngot  %s", wantEncounter, gotEncounter)
	}

	bundle, err := CreateConditionalTransactionBundle(projection)
	if err != nil {
		t.Fatalf("CreateConditionalTransactionBundle: %v", err)
	}
	if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" || len(bundle.Entry) != 2 {
		t.Fatalf("bundle = %+v", bundle)
	}
	for index, entry := range bundle.Entry {
		resource := projection.Resources[index]
		if entry.FullURL != resource.FullURL {
			t.Fatalf("entry %d fullUrl = %q, want %q", index, entry.FullURL, resource.FullURL)
		}
		if entry.Request == nil || entry.Request.Method != http.MethodPut ||
			entry.Request.URL != resource.Type+"?identifier="+url.QueryEscape(resource.Key.System+"|"+resource.Key.Value) {
			t.Fatalf("entry %d request = %+v", index, entry.Request)
		}
		if !bytes.HasPrefix(entry.Resource, []byte(`{"resourceType":"`+resource.Type+`"`)) {
			t.Fatalf("entry %d resource does not lead with resourceType: %s", index, entry.Resource)
		}
		var body map[string]any
		if err := json.Unmarshal(entry.Resource, &body); err != nil {
			t.Fatalf("entry %d: %v", index, err)
		}
		if _, present := body["id"]; present {
			t.Fatalf("entry %d carries an id", index)
		}
		if !projectTestHasIdentifier(body, resource.Key) {
			t.Fatalf("entry %d does not carry its key %+v: %s", index, resource.Key, entry.Resource)
		}
		outcome, err := fhir.ValidateJSON(entry.Resource, fhir.ValidationOptions{Mode: string(fhir.ModeUSCore)})
		if err != nil || len(outcome.Issue) != 0 {
			t.Fatalf("entry %d does not validate at us-core: err=%v issues=%+v\n%s", index, err, outcome, entry.Resource)
		}
	}

	// The bare visit number gained the system rather than a duplicate, and the
	// Encounter points at the Patient entry.
	var encounterBody map[string]any
	_ = json.Unmarshal(bundle.Entry[1].Resource, &encounterBody)
	identifiers, _ := encounterBody["identifier"].([]any)
	if len(identifiers) != 1 {
		t.Fatalf("Encounter identifiers = %v, want exactly the qualified visit number", identifiers)
	}
	subject, _ := encounterBody["subject"].(map[string]any)
	if reference, _ := subject["reference"].(string); reference != patient.FullURL {
		t.Fatalf("Encounter.subject.reference = %q, want the Patient entry fullUrl %q", reference, patient.FullURL)
	}
}

// TestProjectLabResultReferences proves the lab projection keys its report on
// the order number, keys each observation under the report, points the
// observations at their report entry, and references the Patient — which is
// not in the bundle — conditionally.
func TestProjectLabResultReferences(t *testing.T) {
	t.Parallel()

	event := &events.LabResultEvent{
		EventMeta: projectTestMeta(events.EventLabResult),
		Patient:   events.Patient{MRN: "MRN-000123"},
		Test:      events.LabTest{LOINCCode: "58410-2", Description: "CBC panel", OrderID: "ORD-123"},
		Result:    events.LabValue{Status: "final"},
		Results: []events.LabObservation{
			{Test: events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes"}, Result: events.LabValue{Value: "12.5", Unit: "10*3/uL", Status: "final"}},
			{Test: events.LabTest{LOINCCode: "789-8", Description: "Erythrocytes"}, Result: events.LabValue{Value: "5.2", Unit: "10*6/uL", Status: "final"}},
		},
	}
	projection, err := Project(events.EventLabResult, projectTestPayload(t, event))
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got := projection.ResourceTypes(); strings.Join(got, ",") != "DiagnosticReport,Observation" {
		t.Fatalf("ResourceTypes = %v", got)
	}
	if len(projection.Resources) != 3 {
		t.Fatalf("resources = %d, want report + 2 observations", len(projection.Resources))
	}
	report := projection.Resources[0]
	if report.Key != (fhir.Identifier{System: "urn:fi-fhir:source:lab-east", Value: "ORD-123"}) {
		t.Fatalf("report key = %+v", report.Key)
	}
	if projection.Resources[1].Key.Value != "ORD-123#obs-1" || projection.Resources[2].Key.Value != "ORD-123#obs-2" {
		t.Fatalf("observation keys = %+v / %+v", projection.Resources[1].Key, projection.Resources[2].Key)
	}

	bundle, err := CreateConditionalTransactionBundle(projection)
	if err != nil {
		t.Fatalf("CreateConditionalTransactionBundle: %v", err)
	}
	var reportBody map[string]any
	_ = json.Unmarshal(bundle.Entry[0].Resource, &reportBody)
	subject, _ := reportBody["subject"].(map[string]any)
	if reference, _ := subject["reference"].(string); reference != "Patient?identifier="+url.QueryEscape("urn:fi-fhir:source:lab-east|MRN-000123") {
		t.Fatalf("report subject = %q, want a conditional Patient reference", reference)
	}
	results, _ := reportBody["result"].([]any)
	if len(results) != 2 {
		t.Fatalf("report.result = %v", results)
	}
	for index, raw := range results {
		result, _ := raw.(map[string]any)
		if reference, _ := result["reference"].(string); reference != projection.Resources[index+1].FullURL {
			t.Fatalf("report.result[%d] = %q, want observation fullUrl %q", index, reference, projection.Resources[index+1].FullURL)
		}
	}
	for index := 1; index < 3; index++ {
		var body map[string]any
		_ = json.Unmarshal(bundle.Entry[index].Resource, &body)
		if _, present := body["id"]; present {
			t.Fatalf("observation %d still carries the mapper's positional id", index)
		}
		subject, _ := body["subject"].(map[string]any)
		if reference, _ := subject["reference"].(string); !strings.HasPrefix(reference, "Patient?identifier=") {
			t.Fatalf("observation %d subject = %q", index, reference)
		}
	}
}

// TestProjectRefusesResourcesWithoutAUsableKey is the lane's riskiest
// assumption as code: nothing is ever downgraded to a POST.
func TestProjectRefusesResourcesWithoutAUsableKey(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate func(*events.PatientAdmitEvent)
		want   error
	}{
		"no visit number": {
			mutate: func(e *events.PatientAdmitEvent) {
				e.Encounter.ID = ""
				e.Encounter.Identifiers = events.IdentifierSet{}
			},
			want: ErrNoUsableIdentifier,
		},
		"no medical record number": {
			mutate: func(e *events.PatientAdmitEvent) { e.Patient.MRN = ""; e.Patient.Identifiers = events.IdentifierSet{} },
			want:   ErrNoUsableIdentifier,
		},
		"only an SSN": {
			mutate: func(e *events.PatientAdmitEvent) {
				e.Patient.MRN = ""
				e.Patient.Identifiers = events.IdentifierSet{Identifiers: []events.Identifier{
					{Value: "123456789", Type: "SS", System: "http://hl7.org/fhir/sid/us-ssn"},
				}}
			},
			want: ErrNoUsableIdentifier,
		},
		"bare visit number and no source": {
			mutate: func(e *events.PatientAdmitEvent) { e.Source = "" },
			want:   ErrNoUsableIdentifier,
		},
		"visit number with the token separator": {
			mutate: func(e *events.PatientAdmitEvent) { e.Encounter.ID = "VISIT|1" },
			want:   ErrNoUsableIdentifier,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			event := projectTestAdmit()
			testCase.mutate(event)
			_, err := ProjectEvent(event)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("ProjectEvent = %v, want %v", err, testCase.want)
			}
		})
	}

	if _, err := ProjectEvent((*events.PatientAdmitEvent)(nil)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("ProjectEvent(nil admit) = %v, want ErrInvalidPayload", err)
	}
	if _, err := ProjectEvent(&events.AppointmentEvent{}); !errors.Is(err, ErrUnsupportedEventType) {
		t.Fatalf("ProjectEvent(appointment) = %v, want ErrUnsupportedEventType", err)
	}
	if _, err := Project(events.EventPatientAdmit, json.RawMessage(`{"type":"patient_admit","raw":"x"}`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("Project(raw payload) = %v, want ErrInvalidPayload", err)
	}
}

func TestPayloadEventType(t *testing.T) {
	t.Parallel()
	if got, err := PayloadEventType(json.RawMessage(`{"id":"e","type":"patient_admit"}`)); err != nil || got != events.EventPatientAdmit {
		t.Fatalf("PayloadEventType = %q, %v", got, err)
	}
	for _, payload := range []string{`{}`, `{"type":""}`, `not json`, ``} {
		if _, err := PayloadEventType(json.RawMessage(payload)); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("PayloadEventType(%q) = %v, want ErrInvalidPayload", payload, err)
		}
	}
}

// TestMapEventQualifiesBareEncounterIdentifierUnderTheSource is Lane S7-A's
// banked finding (Sprint 7): Slice 5.1c-α gave USCoreMapper a Source, but
// fhirout never set it, so the legacy workflow `fhir` action's raw Encounter
// kept a systemless visit number while the durable path only looked right
// because ensureIdentifier added the key afterwards. The raw resource must
// carry the deployment-owned system itself, for the pointer and value forms
// the switch accepts alike.
func TestMapEventQualifiesBareEncounterIdentifierUnderTheSource(t *testing.T) {
	want := fhir.Identifier{System: "urn:fi-fhir:source:adt-east", Value: "VISIT-000123"}
	for name, event := range map[string]any{
		"pointer": projectTestAdmit(),
		"value":   *projectTestAdmit(),
	} {
		t.Run(name, func(t *testing.T) {
			resources, err := MapEvent(event)
			if err != nil {
				t.Fatalf("MapEvent: %v", err)
			}
			var encounter *fhir.Encounter
			for _, resource := range resources {
				if typed, ok := resource.(*fhir.Encounter); ok {
					encounter = typed
				}
			}
			if encounter == nil {
				t.Fatalf("MapEvent produced no Encounter among %d resources", len(resources))
			}
			if len(encounter.Identifier) != 1 || encounter.Identifier[0].System != want.System || encounter.Identifier[0].Value != want.Value {
				t.Fatalf("raw Encounter.identifier = %+v, want exactly %+v", encounter.Identifier, want)
			}
		})
	}
}

func TestSourceIdentifierSystemEscapesTheSource(t *testing.T) {
	t.Parallel()
	got, err := SourceIdentifierSystem("adt east/1")
	if err != nil || got != "urn:fi-fhir:source:adt%20east%2F1" {
		t.Fatalf("SourceIdentifierSystem = %q, %v", got, err)
	}
	if _, err := SourceIdentifierSystem("  "); !errors.Is(err, ErrNoUsableIdentifier) {
		t.Fatalf("SourceIdentifierSystem(blank) = %v", err)
	}
}

// --- fixtures ---------------------------------------------------------------

func projectTestMeta(eventType events.EventType) events.EventMeta {
	source := "adt-east"
	if eventType == events.EventLabResult {
		source = "lab-east"
	}
	return events.EventMeta{
		ID:              "event-" + string(eventType),
		Type:            eventType,
		Timestamp:       time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		ReceivedAt:      time.Date(2026, 7, 14, 16, 0, 1, 0, time.UTC),
		Source:          source,
		SourceFormat:    events.FormatHL7v2,
		SourceProfileID: "strict-adt-profile",
		SourceMessageID: "control-" + string(eventType),
		CorrelationID:   "correlation-1",
	}
}

func projectTestAdmit() *events.PatientAdmitEvent {
	return &events.PatientAdmitEvent{
		EventMeta: projectTestMeta(events.EventPatientAdmit),
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
			ID:            "VISIT-000123",
			Class:         "I",
			Status:        "admitted",
			AdmitDateTime: time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		},
	}
}

func projectTestEventFor(t *testing.T, eventType events.EventType) any {
	for _, event := range clinicalTestEvents(t) {
		if event["type"] == string(eventType) {
			factory := supportedEventTypes[eventType]
			value := factory()
			raw, _ := json.Marshal(event)
			if err := json.Unmarshal(raw, value); err != nil {
				t.Fatal(err)
			}
			return value
		}
	}
	switch eventType {
	case events.EventPatientDischarge:
		admit := projectTestAdmit()
		return &events.PatientDischargeEvent{
			EventMeta: projectTestMeta(eventType),
			Patient:   admit.Patient,
			Encounter: admit.Encounter,
		}
	case events.EventLabResult:
		return &events.LabResultEvent{
			EventMeta: projectTestMeta(eventType),
			Patient:   events.Patient{MRN: "MRN-000123"},
			Test:      events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes", OrderID: "ORD-1"},
			Result:    events.LabValue{Value: "12.5", Unit: "10*3/uL", Status: "final"},
		}
	default:
		admit := projectTestAdmit()
		admit.EventMeta = projectTestMeta(eventType)
		return admit
	}
}

func projectTestPayload(t *testing.T, event any) json.RawMessage {
	t.Helper()
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "tenant-a",
		Classification: integration.DataClassificationPHI,
	}, event)
	if err != nil {
		t.Fatalf("NewProcessedEvent(%T): %v", event, err)
	}
	return processed.PayloadJSON()
}

func projectTestHasIdentifier(body map[string]any, key fhir.Identifier) bool {
	identifiers, _ := body["identifier"].([]any)
	for _, raw := range identifiers {
		identifier, _ := raw.(map[string]any)
		system, _ := identifier["system"].(string)
		value, _ := identifier["value"].(string)
		if system == key.System && value == key.Value {
			return true
		}
	}
	return false
}

func eventTypeStrings(types []events.EventType) []string {
	out := make([]string, 0, len(types))
	for _, eventType := range types {
		out = append(out, string(eventType))
	}
	return out
}
