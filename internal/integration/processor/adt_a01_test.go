package processor

import (
	"bytes"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestProjectADTA01RebuildsDeterministicRawFreeMetadata(t *testing.T) {
	t.Parallel()

	revision, request := projectorRevisionAndRequest(t, []byte("message-one"))
	occurredAt := time.Date(2026, 7, 13, 12, 30, 45, 123000000, time.FixedZone("source", -4*60*60))
	parsed := projectorParseResult(occurredAt)
	parsedEvent := parsed.Event.(*events.PatientAdmitEvent)
	parsedEvent.RawPayload = json.RawMessage(`"RAW-SENTINEL"`)
	parsedEvent.ParseWarnings = []events.ParseWarning{{Code: "RAW-SENTINEL", Message: "RAW-SENTINEL"}}
	parsedEvent.ExtractedEntities = &events.ExtractedEntities{SourceText: "RAW-SENTINEL"}
	parsedEvent.QualityScore = &events.DataQualityScore{Model: "RAW-SENTINEL"}
	before, err := json.Marshal(parsedEvent)
	if err != nil {
		t.Fatalf("marshal parser event: %v", err)
	}

	projected, diagnostics, err := projectADTA01(parsed, request, revision, 0)
	if err != nil {
		t.Fatalf("projectADTA01: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if projected.TenantID != revision.TenantID || projected.Type != events.EventPatientAdmit {
		t.Fatalf("unexpected processed event wrapper: %#v", projected)
	}
	if projected.SourceMessageID != "control-123" || projected.CorrelationID != request.CorrelationID {
		t.Fatalf("correlation metadata drifted: %#v", projected)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(projected.ID) {
		t.Fatalf("event ID is not an RFC 9562 UUIDv8: %q", projected.ID)
	}
	payload := projected.PayloadJSON()
	for _, forbidden := range []string{"RAW-SENTINEL", "raw_payload", "parse_warnings", "extracted_entities", "quality_score"} {
		if bytes.Contains(payload, []byte(forbidden)) {
			t.Fatalf("processed payload contains %q: %s", forbidden, payload)
		}
	}
	var wire struct {
		ID              string              `json:"id"`
		Timestamp       time.Time           `json:"timestamp"`
		ReceivedAt      time.Time           `json:"received_at"`
		Source          string              `json:"source"`
		SourceFormat    events.SourceFormat `json:"source_format"`
		SourceProfileID string              `json:"source_profile_id"`
		SourceMessageID string              `json:"source_message_id"`
		CorrelationID   string              `json:"correlation_id"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("decode processed payload: %v", err)
	}
	if wire.ID != projected.ID || !wire.Timestamp.Equal(occurredAt) || !wire.ReceivedAt.Equal(request.Envelope.ReceivedAt.UTC()) {
		t.Fatalf("deterministic times/ID mismatch: %#v", wire)
	}
	if wire.Source != revision.Source.SourceID || wire.SourceFormat != events.FormatHL7v2 || wire.SourceProfileID != revision.Profile.ArtifactID || wire.SourceMessageID != "control-123" || wire.CorrelationID != request.CorrelationID {
		t.Fatalf("deterministic provenance mismatch: %#v", wire)
	}
	after, err := json.Marshal(parsedEvent)
	if err != nil {
		t.Fatalf("marshal parser event after projection: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("projector mutated parser-owned event")
	}

	again, _, err := projectADTA01(projectorParseResult(occurredAt), request, revision, 0)
	if err != nil {
		t.Fatalf("projectADTA01(second): %v", err)
	}
	if again.ID != projected.ID || !bytes.Equal(again.PayloadJSON(), projected.PayloadJSON()) {
		t.Fatalf("same request was not deterministic:\nfirst:  %s\nsecond: %s", projected.PayloadJSON(), again.PayloadJSON())
	}
}

func TestProjectADTA01EventIdentityUsesBytesAndRevisionNotRequestTiming(t *testing.T) {
	t.Parallel()

	revision, request := projectorRevisionAndRequest(t, []byte("message-one"))
	parsed := projectorParseResult(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	first, _, err := projectADTA01(parsed, request, revision, 0)
	if err != nil {
		t.Fatalf("project first: %v", err)
	}

	changedRequest := request
	changedRequest.CorrelationID = "correlation-other"
	changedRequest.Envelope = projectorEnvelope(t, revision, []byte("message-one"), request.Envelope.ReceivedAt.Add(24*time.Hour))
	changedRequest.Security = request.Security
	second, _, err := projectADTA01(parsed, changedRequest, revision, 0)
	if err != nil {
		t.Fatalf("project request metadata change: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("correlation/received-at changed event identity: %q != %q", first.ID, second.ID)
	}

	changedBytes := request
	changedBytes.Envelope = projectorEnvelope(t, revision, []byte("message-two"), request.Envelope.ReceivedAt)
	third, _, err := projectADTA01(parsed, changedBytes, revision, 0)
	if err != nil {
		t.Fatalf("project byte change: %v", err)
	}
	if first.ID == third.ID {
		t.Fatal("payload mutation did not change event identity")
	}

	changedRevision, changedRevisionRequest := projectorRevisionAndRequestWithID(t, []byte("message-one"), "revision-2")
	fourth, _, err := projectADTA01(parsed, changedRevisionRequest, changedRevision, 0)
	if err != nil {
		t.Fatalf("project revision change: %v", err)
	}
	if first.ID == fourth.ID {
		t.Fatal("integration revision mutation did not change event identity")
	}
}

func TestProjectADTA01RejectsUnsupportedOrAmbiguousParserOutput(t *testing.T) {
	t.Parallel()

	revision, request := projectorRevisionAndRequest(t, []byte("message-one"))
	base := projectorParseResult(time.Now())
	tests := []struct {
		name   string
		mutate func(*hl7v2.ParseResult)
	}{
		{name: "nil result", mutate: nil},
		{name: "nil event", mutate: func(result *hl7v2.ParseResult) { result.Event = nil }},
		{name: "value event", mutate: func(result *hl7v2.ParseResult) { result.Event = *result.Event.(*events.PatientAdmitEvent) }},
		{name: "wrong concrete event", mutate: func(result *hl7v2.ParseResult) { result.Event = &events.PatientDischargeEvent{} }},
		{name: "A04", mutate: func(result *hl7v2.ParseResult) { result.MessageType = "ADT^A04" }},
		{name: "prefix smuggling", mutate: func(result *hl7v2.ParseResult) { result.MessageType = "ADT^A010" }},
		{name: "wrong structure", mutate: func(result *hl7v2.ParseResult) { result.MessageType = "ADT^A01^OTHER" }},
		{name: "blank control", mutate: func(result *hl7v2.ParseResult) { result.ControlID = "" }},
		{name: "control mismatch", mutate: func(result *hl7v2.ParseResult) { result.ControlID = "other" }},
		{name: "wrong event type", mutate: func(result *hl7v2.ParseResult) {
			result.Event.(*events.PatientAdmitEvent).Type = events.EventPatientUpdate
		}},
		{name: "wrong source", mutate: func(result *hl7v2.ParseResult) { result.Event.(*events.PatientAdmitEvent).Source = "other" }},
		{name: "wrong format", mutate: func(result *hl7v2.ParseResult) {
			result.Event.(*events.PatientAdmitEvent).SourceFormat = events.FormatFHIR
		}},
		{name: "wrong profile", mutate: func(result *hl7v2.ParseResult) { result.ProfileID = "other" }},
		{name: "patient extension", mutate: func(result *hl7v2.ParseResult) {
			result.Event.(*events.PatientAdmitEvent).Patient.Extensions = map[string]any{"raw_ZPD": "sentinel"}
		}},
		{name: "encounter extension", mutate: func(result *hl7v2.ParseResult) {
			result.Event.(*events.PatientAdmitEvent).Encounter.Extensions = map[string]any{"x": true}
		}},
		{name: "provider extension", mutate: func(result *hl7v2.ParseResult) {
			result.Event.(*events.PatientAdmitEvent).Encounter.AttendingProvider = &events.Provider{Extensions: map[string]any{"x": true}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var candidate *hl7v2.ParseResult
			if tt.mutate != nil {
				candidate = cloneProjectorParseResult(t, base)
				tt.mutate(candidate)
			}
			if _, _, err := projectADTA01(candidate, request, revision, 0); err == nil {
				t.Fatal("expected projector to fail closed")
			} else if strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), "ADT^A010") {
				t.Fatalf("projector error leaked source content: %v", err)
			}
		})
	}
}

func TestProjectADTA01CatalogsWarningsWithoutRawText(t *testing.T) {
	t.Parallel()

	revision, request := projectorRevisionAndRequest(t, []byte("message-one"))
	parsed := projectorParseResult(time.Time{})
	parsed.Warnings = []events.ParseWarning{
		{Phase: "semantic", Code: "MISSING_PV1", Message: "RAW-SENTINEL", Path: "PV1", Severity: "warning", Explanation: "RAW-SENTINEL", FixSuggestion: "RAW-SENTINEL"},
		{Phase: "semantic", Code: "INVALID_NPI_LENGTH", Message: "RAW-SENTINEL", Path: "PID.3[0]", Severity: "warning"},
		{Phase: "bad\nphase", Code: "RAW_SENTINEL", Message: "RAW-SENTINEL", Path: "bad\npath", Severity: "error"},
	}

	_, diagnostics, err := projectADTA01(parsed, request, revision, 0)
	if err != nil {
		t.Fatalf("projectADTA01: %v", err)
	}
	gotCodes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		gotCodes = append(gotCodes, diagnostic.Code)
		encoded, err := json.Marshal(diagnostic)
		if err != nil {
			t.Fatalf("marshal diagnostic: %v", err)
		}
		if bytes.Contains(encoded, []byte("RAW-SENTINEL")) || bytes.Contains(encoded, []byte("bad\\n")) {
			t.Fatalf("diagnostic leaked parser content: %s", encoded)
		}
	}
	wantCodes := []string{"MISSING_PV1", "INVALID_NPI_LENGTH", "PARSE_WARNING", "EVENT_TIME_FALLBACK"}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("diagnostic codes = %#v, want %#v", gotCodes, wantCodes)
	}
}

func projectorParseResult(occurredAt time.Time) *hl7v2.ParseResult {
	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:              "parser-random-id",
			Type:            events.EventPatientAdmit,
			Timestamp:       time.Now(),
			ReceivedAt:      time.Now(),
			Source:          "adt-east",
			SourceFormat:    events.FormatHL7v2,
			SourceProfileID: "profile-adt",
			SourceMessageID: "control-123",
			CorrelationID:   "parser-correlation",
		},
		Patient:   events.Patient{MRN: "MRN-123", FamilyName: "Patient", GivenName: "Test"},
		Encounter: events.Encounter{ID: "visit-123", Class: "I", ClassifiedEventType: "inpatient_admit"},
	}
	return &hl7v2.ParseResult{
		Event:          event,
		ProfileID:      "profile-adt",
		MessageType:    "ADT^A01",
		ControlID:      "control-123",
		MessageVersion: "2.5.1",
		OccurredAt:     occurredAt,
	}
}

func cloneProjectorParseResult(t *testing.T, input *hl7v2.ParseResult) *hl7v2.ParseResult {
	t.Helper()
	raw, err := json.Marshal(input.Event)
	if err != nil {
		t.Fatalf("marshal parse result event: %v", err)
	}
	var event events.PatientAdmitEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("unmarshal parse result event: %v", err)
	}
	clone := *input
	clone.Event = &event
	clone.Warnings = append([]events.ParseWarning(nil), input.Warnings...)
	return &clone
}

func projectorRevisionAndRequest(t *testing.T, payload []byte) (integration.IntegrationDefinitionRevision, integration.ProcessRequest) {
	return projectorRevisionAndRequestWithID(t, payload, "revision-1")
}

func projectorRevisionAndRequestWithID(t *testing.T, payload []byte, revisionID string) (integration.IntegrationDefinitionRevision, integration.ProcessRequest) {
	t.Helper()
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   revisionID,
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "7", Digest: digest("b")},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest("c")},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d")},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID:   "tenant-a",
			Principal:  integration.Principal{ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	envelope := projectorEnvelope(t, revision, payload, time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC))
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID:         "source-service",
				Kind:       integration.PrincipalKindService,
				AuthMethod: "mtls",
				SourceID:   "adt-east",
			},
		},
		Envelope:      envelope,
		CorrelationID: "correlation-123",
	}
	if err := request.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid projector request: %v", err)
	}
	return revision, request
}

func projectorEnvelope(t *testing.T, revision integration.IntegrationDefinitionRevision, payload []byte, receivedAt time.Time) integration.RawEnvelope {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       revision.TenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     receivedAt,
		Classification: integration.DataClassificationPHI,
	}, payload)
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	return envelope
}
