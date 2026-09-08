package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

// TestFHIRDestination_DurablePayloadRoundTripsToMapperInput is Slice 4.1c-c's
// first day-1 kill-test, and it must PASS on unmodified `main`.
//
// `.loom/34-sprint6-execution-specs.md` names the sprint's riskiest assumption:
//
//	"The stored canonical payload round-trips into the exact mapper input, and
//	a FHIR write built from it can be redelivered without creating duplicates."
//
// This test is the first half. The outbox row's `payload_json` is
// `ProcessedEvent.PayloadJSON()` (processor/postgres_submission.go), which
// NewProcessedEvent builds by marshalling a concrete pkg/events struct after
// zeroing only the eight forbiddenRawPayloadKeys. Correction 2 claims that
// `decodeCanonicalEventPayload` already reverses that projection into the exact
// type `pkg/fhir.USCoreMapper` consumes, so 4.1c-c needs no new mapper and no
// new dependency. That claim is what this test executes rather than argues:
//
//  1. Parse a fixture through the real path — the golden ADT A01 through
//     processor.MessageProcessor (the same parse, projection, and
//     NewProcessedEvent the durable submission commits), and the discharge and
//     lab-result fixtures through the real hl7v2 parser and NewProcessedEvent.
//  2. Take PayloadJSON(), the bytes the outbox stores.
//  3. Decode them with decodeCanonicalEventPayload for the event's type.
//  4. Map the decoded event AND the parser's original with the same
//     USCoreMapper entry points the legacy engine's eventToFHIRResources uses.
//  5. Assert the two resource sets are byte-equal after marshalling, and that
//     every resource validates at `us-core` with zero issues.
//
// If this fails, the blocker is the redaction list or the registry — not the
// transport — and S6-A's first task changes from "build the transport" to
// "widen the decoder". Say so and stop.
//
// It also records correction 3 in executable form: the stored wire shape
// carries `id` and `type` with the value `patient_admit`, not the
// `event_id`/`event_type: patient.admitted` literal the 5.1a gate hand-wrote.
func TestFHIRDestination_DurablePayloadRoundTripsToMapperInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  func(t *testing.T) roundTripFixture
	}{
		{name: "golden ADT A01 through the processor", run: roundTripGoldenADTA01},
		{name: "ADT A01 with PV1 through the processor", run: roundTripADTA01WithPV1},
		{name: "ADT A03 through the parser", run: roundTripADTA03},
		{name: "ORU R01 through the parser", run: roundTripORUR01},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := testCase.run(t)
			assertDurablePayloadRoundTrips(t, fixture)
		})
	}
}

// roundTripFixture is one event as the parser produced it and as the durable
// engine stored it.
type roundTripFixture struct {
	eventType events.EventType
	original  any
	processed integration.ProcessedEvent
}

func assertDurablePayloadRoundTrips(t *testing.T, fixture roundTripFixture) {
	t.Helper()

	payload := fixture.processed.PayloadJSON()
	if len(payload) == 0 {
		t.Fatal("PayloadJSON() is empty; nothing was stored")
	}

	// Correction 3: the wire shape is the pkg/events struct's own metadata.
	var wire struct {
		ID        string           `json:"id"`
		Type      events.EventType `json:"type"`
		EventID   json.RawMessage  `json:"event_id"`
		EventType json.RawMessage  `json:"event_type"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("stored payload is not a JSON object: %v", err)
	}
	if wire.Type != fixture.eventType || wire.ID == "" {
		t.Fatalf("stored payload carries id=%q type=%q, want a non-empty id and type %q",
			wire.ID, wire.Type, fixture.eventType)
	}
	if wire.EventID != nil || wire.EventType != nil {
		t.Fatalf("stored payload carries event_id/event_type; that is the 5.1a gate's "+
			"hand-written literal, not the engine's wire shape\npayload: %s", payload)
	}

	decoded, err := integration.DecodeCanonicalEventPayload(fixture.eventType, payload)
	if err != nil {
		t.Fatalf("decodeCanonicalEventPayload(%s) failed — the durable payload does not "+
			"round-trip into the mapper's input, so the blocker is the decoder or the "+
			"redaction list, not the transport: %v", fixture.eventType, err)
	}

	mapper := fhir.NewUSCoreMapper()
	wantResources := roundTripMap(t, mapper, fixture.original)
	gotResources := roundTripMap(t, mapper, decoded)
	if len(wantResources) == 0 {
		t.Fatalf("the mapper produced no resources from the original %T", fixture.original)
	}
	if len(gotResources) != len(wantResources) {
		t.Fatalf("decoded event maps to %d resources, original maps to %d",
			len(gotResources), len(wantResources))
	}
	for index := range wantResources {
		want, err := json.Marshal(wantResources[index])
		if err != nil {
			t.Fatalf("marshal original resource %d: %v", index, err)
		}
		got, err := json.Marshal(gotResources[index])
		if err != nil {
			t.Fatalf("marshal decoded resource %d: %v", index, err)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("resource %d differs after the durable round-trip\noriginal: %s\ndecoded:  %s",
				index, want, got)
		}
		outcome, err := fhir.ValidateJSON(got, fhir.ValidationOptions{Mode: string(fhir.ModeUSCore)})
		if err != nil {
			t.Fatalf("ValidateJSON(resource %d): %v", index, err)
		}
		if len(outcome.Issue) != 0 {
			t.Fatalf("resource %d does not validate at us-core --strict: %s\nresource: %s",
				index, roundTripDescribeIssues(outcome.Issue), got)
		}
	}
}

// roundTripMap is the legacy engine's three-case switch
// (internal/workflow/actions.go eventToFHIRResources), applied to whichever
// concrete type the fixture carries. It is deliberately a copy of the dispatch
// shape rather than a call into the legacy engine: the point is to prove the
// decoded value is the *same concrete type* the mapper entry points take.
func roundTripMap(t *testing.T, mapper *fhir.USCoreMapper, event any) []fhir.Resource {
	t.Helper()
	var resources []fhir.Resource
	switch typed := event.(type) {
	case *events.PatientAdmitEvent:
		resources = append(resources, mapper.MapPatient(&typed.Patient))
		resources = append(resources, mapper.MapEncounter(&typed.Encounter, "Patient/"+typed.Patient.MRN))
	case *events.PatientDischargeEvent:
		resources = append(resources, mapper.MapPatient(&typed.Patient))
		resources = append(resources, mapper.MapEncounter(&typed.Encounter, "Patient/"+typed.Patient.MRN))
	case *events.LabResultEvent:
		report, observations := mapper.MapLabResult(typed)
		resources = append(resources, report)
		for _, observation := range observations {
			resources = append(resources, observation)
		}
	default:
		t.Fatalf("%T is not a type the mapper's admit/discharge/lab entry points accept", event)
	}
	return resources
}

func roundTripDescribeIssues(issues []fhir.OperationOutcomeIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Severity+" "+issue.Code+": "+issue.Diagnostics)
	}
	return strings.Join(parts, "; ")
}

// --- Fixtures -----------------------------------------------------------------

const (
	roundTripTenantID = "tenant-a"
	roundTripSourceID = "adt-east"
	roundTripProfile  = "strict-adt-profile"
)

// roundTripGoldenADTA01 drives the golden ADT A01 fixture through the real
// processor: DefinitionRevisionResolver, RevisionResolver, profile compilation,
// hl7v2.Parser, projectADTA01, NewProcessedEvent. Preview mode is used so the
// test needs no PostgreSQL; the ProcessedEvent it returns is the same value the
// production path hands to PostgresSubmissionStore.commit, whose outbox row is
// PayloadJSON() of that event.
func roundTripGoldenADTA01(t *testing.T) roundTripFixture {
	t.Helper()
	root := roundTripRepositoryRoot(t)
	raw := roundTripReadFile(t, filepath.Join(root, "testdata", "golden", "integration", "adt-http", "input.hl7"))
	profileJSON := roundTripReadFile(t, filepath.Join(root, "testdata", "golden", "integration", "adt-http", "tolerant-profile.json"))
	workflowYAML := roundTripReadFile(t, filepath.Join(root, "testdata", "golden", "integration", "adt-http", "workflow.yaml"))
	return roundTripThroughProcessor(t, profileJSON, workflowYAML, raw)
}

// roundTripADTA01WithPV1 is the same processor path over an admit that carries
// a PV1, so the Encounter half of the projection has an identifier, a class,
// and an admit time to lose.
//
// The PV1 is the widest one the executable A01 v1 subset admits: strict
// validation caps PV1.19 at ONE component (hl7v2/strict_validation.go
// strictA01FieldComponentLimits), so a durable visit number can never carry an
// assigning authority and the canonical Encounter's identifier never has a
// system. 4.1c-c's conditional-write design has to start from that fact.
func roundTripADTA01WithPV1(t *testing.T) roundTripFixture {
	t.Helper()
	root := roundTripRepositoryRoot(t)
	profileJSON := roundTripReadFile(t, filepath.Join(root, "testdata", "golden", "integration", "adt-http", "tolerant-profile.json"))
	workflowYAML := roundTripReadFile(t, filepath.Join(root, "testdata", "golden", "integration", "adt-http", "workflow.yaml"))
	raw := []byte(strings.Join([]string{
		`MSH|^~\&|RAW-ROUNDTRIP-SENTINEL|FAC|APP|FAC|20260714120000-0400||ADT^A01^ADT_A01|roundtrip-a01-001|P|2.5.1`,
		`EVN|A01|20260714120000||||20260714115900-0400`,
		`PID|1||MRN-RT-001^^^HOSP^MR||Patient^Round^Trip||19800101|F|||1 Main St^^Springfield^IL^62701||5551234567`,
		`PV1|1|I|UNIT^101^A^FAC||||1234567893^Attending^Amy|||MED||||||||VISIT-RT-001|||||||||||||||||||||||||20260714120000`,
	}, "\r"))
	return roundTripThroughProcessor(t, profileJSON, workflowYAML, raw)
}

// roundTripADTA03 parses a discharge through the real hl7v2 parser under the
// golden profile and projects it exactly as projectADTA01 projects an admit.
// There is no durable projector for A03 today (the processor's executable v1
// subset is ADT A01), so this row proves the decoder over the discharge type
// the registry and the legacy switch both already admit.
func roundTripADTA03(t *testing.T) roundTripFixture {
	t.Helper()
	raw := strings.Join([]string{
		`MSH|^~\&|RAW-ROUNDTRIP-SENTINEL|FAC|APP|FAC|20260714180000-0400||ADT^A03^ADT_A03|roundtrip-a03-001|P|2.5.1`,
		`EVN|A03|20260714180000`,
		`PID|1||MRN-RT-001^^^HOSP^MR||Patient^Round^Trip||19800101|F`,
		`PV1|1|I|UNIT^101^A^FAC||||1234567893^Attending^Amy^^^MD|||MED||||||||VISIT-RT-001^^^HOSP^VN||||||||||||||||||||||||01|20260714120000|20260714180000`,
	}, "\r")
	parsed := roundTripParse(t, raw, false)
	original, ok := parsed.Event.(*events.PatientDischargeEvent)
	if !ok {
		t.Fatalf("parser produced %T for ADT A03, want *events.PatientDischargeEvent", parsed.Event)
	}
	clone := *original
	clone.EventMeta = roundTripEventMeta(events.EventPatientDischarge, parsed)
	clone.RawPayload = nil
	return roundTripFixture{
		eventType: events.EventPatientDischarge,
		original:  original,
		processed: roundTripProcessedEvent(t, &clone),
	}
}

// roundTripORUR01 parses the repository's ORU R01 sample through the real
// hl7v2 parser and projects it the same way.
func roundTripORUR01(t *testing.T) roundTripFixture {
	t.Helper()
	root := roundTripRepositoryRoot(t)
	raw := string(roundTripReadFile(t, filepath.Join(root, "testdata", "oru_r01_sample.hl7")))
	parsed := roundTripParse(t, raw, false)
	original, ok := parsed.Event.(*events.LabResultEvent)
	if !ok {
		t.Fatalf("parser produced %T for ORU R01, want *events.LabResultEvent", parsed.Event)
	}
	clone := *original
	clone.EventMeta = roundTripEventMeta(events.EventLabResult, parsed)
	clone.RawPayload = nil
	return roundTripFixture{
		eventType: events.EventLabResult,
		original:  original,
		processed: roundTripProcessedEvent(t, &clone),
	}
}

func roundTripThroughProcessor(t *testing.T, profileJSON, workflowYAML, raw []byte) roundTripFixture {
	t.Helper()

	profileRef, err := processor.NewProfileRevisionReference(roundTripProfile, 1, profileJSON)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("adt-fhir-workflow", "workflow-1", workflowYAML)
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "adt-to-fhir",
		RevisionID:   "revision-1",
		TenantID:     roundTripTenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt", RevisionID: "source-1", Digest: "sha256:" + strings.Repeat("a", 64),
			},
			SourceID: roundTripSourceID,
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: "sha256:" + strings.Repeat("d", 64),
			},
			Class: integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID:   roundTripTenantID,
			Principal:  integration.Principal{ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition revision: %v", err)
	}
	definitions, err := processor.NewDefinitionRevisionResolver(roundTripTenantID, roundTripDefinitionLoader(definitionJSON))
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifacts, err := processor.NewRevisionResolver(roundTripTenantID, roundTripArtifactLoader{profile: profileJSON, workflow: workflowYAML})
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	engine, err := processor.NewMessageProcessor(definitions, artifacts)
	if err != nil {
		t.Fatalf("NewMessageProcessor: %v", err)
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       roundTripTenantID,
		SourceID:       roundTripSourceID,
		Format:         events.FormatHL7v2,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		Classification: integration.DataClassificationPHI,
	}, raw)
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	result, err := engine.Process(context.Background(), integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  roundTripTenantID,
			Principal: integration.Principal{ID: "source-service", Kind: integration.PrincipalKindService, AuthMethod: "mtls", SourceID: roundTripSourceID},
		},
		Envelope:      envelope,
		CorrelationID: "correlation-roundtrip",
	})
	if err != nil {
		t.Fatalf("MessageProcessor.Process: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("Process produced %d events, want 1", len(result.Events))
	}

	// The original is the same bytes through the same parser under the same
	// profile. The processor compiles its profile from the stored JSON; this is
	// the golden tolerant profile expressed as the parser's own configuration.
	parsed := roundTripParse(t, string(raw), true)
	original, ok := parsed.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("parser produced %T for ADT A01, want *events.PatientAdmitEvent", parsed.Event)
	}
	return roundTripFixture{
		eventType: events.EventPatientAdmit,
		original:  original,
		processed: result.Events[0],
	}
}

// roundTripParse runs the real hl7v2 parser under the same configuration the
// processor builds from the golden tolerant profile: UTC, no Z-segments, PV1
// tolerated as missing, HOSP → urn:oid:1.2.3. Strict validation is the
// processor's setting; it admits only the ADT A01 v1 subset, so the discharge
// and lab rows — which the durable processor cannot project today — parse
// without it.
func roundTripParse(t *testing.T, raw string, strict bool) *hl7v2.ParseResult {
	t.Helper()
	parser := hl7v2.NewParser(roundTripSourceID, hl7v2.ParserConfig{
		DefaultTimezone:  time.UTC,
		ExtractZSegments: false,
		StrictValidation: strict,
	})
	parser.SetProfile(&profile.SourceProfile{
		ID:      roundTripProfile,
		Name:    roundTripProfile,
		Version: "1",
		HL7v2: &profile.HL7v2Config{
			DefaultVersion: "2.5.1",
			Timezone:       "UTC",
			Encoding: &profile.EncodingConfig{
				CharsetDefault:   "UTF-8",
				CharsetDetection: false,
				LineEndingMode:   "strict",
			},
			Tolerate: &profile.ToleranceConfig{MissingSegments: []string{"PV1"}},
			EventRules: &profile.EventRulesConfig{
				ADTA01: &profile.EventRule{Default: string(events.EventPatientAdmit)},
			},
		},
		ZSegments: &profile.ZSegmentConfig{Mappings: map[string][]profile.ZFieldMapping{}},
		Identifiers: &profile.IdentifierConfig{
			AssigningAuthorityMap: map[string]string{"HOSP": "urn:oid:1.2.3"},
		},
	})
	parsed, err := parser.ParseWithResult(raw)
	if err != nil {
		t.Fatalf("hl7v2 ParseWithResult: %v", err)
	}
	return parsed
}

// roundTripEventMeta is the metadata projectADTA01 stamps before
// NewProcessedEvent: a deterministic ID, the exact type, source-derived times,
// and no parse warnings or extracted entities.
func roundTripEventMeta(eventType events.EventType, parsed *hl7v2.ParseResult) events.EventMeta {
	occurred := parsed.OccurredAt
	if occurred.IsZero() {
		occurred = time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)
	}
	return events.EventMeta{
		ID:              "roundtrip-" + string(eventType) + "-" + parsed.ControlID,
		Type:            eventType,
		Timestamp:       occurred.UTC(),
		ReceivedAt:      time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC),
		Source:          roundTripSourceID,
		SourceFormat:    events.FormatHL7v2,
		SourceProfileID: roundTripProfile,
		SourceMessageID: parsed.ControlID,
		CorrelationID:   "correlation-roundtrip",
	}
}

func roundTripProcessedEvent(t *testing.T, canonicalEvent any) integration.ProcessedEvent {
	t.Helper()
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       roundTripTenantID,
		Classification: integration.DataClassificationPHI,
	}, canonicalEvent)
	if err != nil {
		t.Fatalf("NewProcessedEvent(%T): %v", canonicalEvent, err)
	}
	return processed
}

type roundTripDefinitionLoader []byte

func (l roundTripDefinitionLoader) LoadDefinitionRevision(ctx context.Context, _, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l...), nil
}

type roundTripArtifactLoader struct {
	profile  []byte
	workflow []byte
}

func (l roundTripArtifactLoader) LoadProfileRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.profile...), nil
}

func (l roundTripArtifactLoader) LoadWorkflowRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.workflow...), nil
}

func roundTripRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root %s has no go.mod: %v", root, err)
	}
	return root
}

func roundTripReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
