//go:build fhirday1gate

// This file is behind the `fhirday1gate` tag on purpose.
//
// It is the Slice 5.1a day-1 gate, and on unmodified `main` it FAILS. The
// failure is the finding, not a regression: a gate that fails is the evidence
// the slice exists, and the tag keeps `go test ./...` honest about the rest of
// the suite while the defect is still open. Run it with:
//
//	make fhir-conformance-day1-gate
//
// which asserts the failure and the exact reason for it. When Slice 5.1a
// reconciles the mapper and the checker, this file's assertion moves into the
// untagged conformance table and the Makefile target stops inverting.

package fhir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// TestFHIRConformance_ValidatorRejectsMapperOutputToday is the Slice 5.1a day-1
// gate. It must FAIL on unmodified `main`, with:
//
//	warning value: meta.profile does not include an expected profile for DiagnosticReport
//
// One row of the eventual conformance table: MapLabResult -> marshal ->
// ValidateJSON at the shipped CLI's default mode. The mapper stamps
// `us-core-diagnosticreport-lab` as a bare literal (mapper.go MapLabResult);
// the checker's accepted set for DiagnosticReport contains only
// `us-core-diagnosticreport-note` (validate.go expectedProfilesForResourceType).
// The shipped validator therefore rejects the shipped mapper's own output.
//
// CI cannot see this today: pkg/fhir/validate_golden_test.go feeds the checker a
// hand-written `-note` fixture and testdata/fhir/ holds no lab DiagnosticReport,
// so `go test ./pkg/fhir/...` is green on the one input the mapper never emits.
//
// What this kills: "Slice 5.1 opens by integrating a validator." The repo does
// not need a bigger validator first. It needs the validator it already ships to
// agree with the mapper it already ships, because a structural validator built
// over a disagreeing mapper only certifies the disagreement at higher
// resolution.
//
// Two controls keep the failure attributable:
//
//   - the repo's own `-note` fixture is validated in the same run and must be
//     clean, so the failure is the mapper/checker disagreement and not a broken
//     checker; and
//   - the mapper's bytes are the ones validated — the report is marshalled and
//     re-parsed rather than inspected as a struct — so the gate cannot pass by
//     reading a field the wire never carries.
func TestFHIRConformance_ValidatorRejectsMapperOutputToday(t *testing.T) {
	// Control: the checker is not simply broken. The repo's own fixture passes.
	notePath := filepath.Join("..", "..", "testdata", "fhir", "diagnosticreport_uscore_note.json")
	fixture, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read %s: %v", notePath, err)
	}
	noteOutcome, err := ValidateJSON(fixture, ValidationOptions{Mode: "us-core"})
	if err != nil {
		t.Fatalf("ValidateJSON(note fixture): %v", err)
	}
	if len(noteOutcome.Issue) != 0 {
		t.Fatalf("control failed: the repo's own -note fixture reports %d issues (%s); "+
			"the checker is broken and this gate would be measuring the wrong thing",
			len(noteOutcome.Issue), describeIssues(noteOutcome.Issue))
	}

	// The gate: the mapper's own DiagnosticReport, on the wire, under the
	// shipped CLI's default mode.
	mapper := NewUSCoreMapper()
	report, observations := mapper.MapLabResult(day1GateLabResultEvent())
	if report == nil {
		t.Fatal("MapLabResult produced no DiagnosticReport")
	}
	if len(observations) == 0 {
		t.Fatal("MapLabResult produced no Observations")
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal DiagnosticReport: %v", err)
	}

	outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: "us-core"})
	if err != nil {
		t.Fatalf("ValidateJSON(mapper output): %v", err)
	}
	if len(outcome.Issue) != 0 {
		t.Fatalf("the shipped validator rejects the shipped mapper's own DiagnosticReport: %s\n"+
			"mapper output: %s", describeIssues(outcome.Issue), encoded)
	}
}

// describeIssues renders an OperationOutcome as one line per issue so the
// failure message names the reason rather than a count.
func describeIssues(issues []OperationOutcomeIssue) string {
	rendered := make([]string, 0, len(issues))
	for _, issue := range issues {
		rendered = append(rendered, issue.Severity+" "+issue.Code+": "+issue.Diagnostics)
	}
	return strings.Join(rendered, "; ")
}

// day1GateLabResultEvent is a representative lab result: a patient identified by
// MRN, two LOINC-coded observations, and a final status. It is the shape every
// shipped parser produces for an ORU^R01.
func day1GateLabResultEvent() *events.LabResultEvent {
	return &events.LabResultEvent{
		Patient: events.Patient{MRN: "MRN-000123"},
		Test:    events.LabTest{LOINCCode: "58410-2", Description: "CBC panel"},
		Result:  events.LabValue{Status: "final"},
		Results: []events.LabObservation{
			{
				Test:   events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes"},
				Result: events.LabValue{Value: "12.5", Unit: "10*3/uL", Status: "final"},
			},
			{
				Test:   events.LabTest{LOINCCode: "789-8", Description: "Erythrocytes"},
				Result: events.LabValue{Value: "5.2", Unit: "10*6/uL", Status: "final"},
			},
		},
	}
}
