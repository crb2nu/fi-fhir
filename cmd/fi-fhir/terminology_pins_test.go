package main

import (
	"context"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

type dummyEvent struct {
	events.EventMeta
}

type dummyOutput struct {
	Events []*dummyEvent
}

func TestAppendParseWarningsToEvent_Appends(t *testing.T) {
	evt := &dummyEvent{
		EventMeta: events.EventMeta{
			ParseWarnings: []events.ParseWarning{{Phase: "p1", Code: "C1"}},
		},
	}
	extra := []events.ParseWarning{{Phase: "p2", Code: "C2"}}

	if ok := appendParseWarningsToEvent(evt, extra); !ok {
		t.Fatalf("expected appendParseWarningsToEvent to return true")
	}
	if len(evt.ParseWarnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(evt.ParseWarnings))
	}
}

func TestAppendParseWarningsToEvent_RejectsNonPointers(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	if ok := appendParseWarningsToEvent(dummyEvent{}, extra); ok {
		t.Fatalf("expected false for non-pointer input")
	}
}

func TestAppendParseWarningsToOutputData_StructEventsField(t *testing.T) {
	out := &dummyOutput{
		Events: []*dummyEvent{
			{EventMeta: events.EventMeta{}},
			{EventMeta: events.EventMeta{ParseWarnings: []events.ParseWarning{{Phase: "x", Code: "Y"}}}},
		},
	}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(out, extra)

	for i, e := range out.Events {
		if len(e.ParseWarnings) == 0 {
			t.Fatalf("expected parse warnings appended for event %d", i)
		}
	}
}

func TestAppendParseWarningsToOutputData_MapEvent(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	out := map[string]interface{}{"event": evt}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(out, extra)
	if len(evt.ParseWarnings) != 1 {
		t.Fatalf("expected parse warnings appended, got %d", len(evt.ParseWarnings))
	}
}

func TestCheckTerminologyPins_InvalidPolicy(t *testing.T) {
	_, err := checkTerminologyPins(context.Background(), "", map[string]string{"loinc": "2.77"}, "nope")
	assertError(t, err)
	assertErrorContains(t, err, "invalid terminology pin policy")
}

func TestCheckTerminologyPins_PassPolicySkips(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://example.invalid/db", map[string]string{"loinc": "2.77"}, "pass")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_EmptyDBURLSkips(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "", map[string]string{"loinc": "2.77"}, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}
