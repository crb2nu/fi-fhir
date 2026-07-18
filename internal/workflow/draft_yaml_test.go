package workflow

import (
	"strings"
	"testing"
)

func TestParseDraftWorkflowBoundsAuthoringShape(t *testing.T) {
	draft := []byte(`name: adt-draft
version: "1"
routes:
  - name: admit
    filter:
      event_type: patient_admit
    transform:
      - redact:
          fields: [patient.ssn]
      - explain_warnings:
          model: must-not-call
    actions:
      - type: webhook
        url: https://example.invalid
        secret: ACTION-CONFIG-SENTINEL
`)
	definition, err := ParseDraftWorkflow(draft)
	if err != nil {
		t.Fatalf("ParseDraftWorkflow() error = %v", err)
	}
	planner, err := NewPlanner(definition)
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]any{"type": "patient_admit"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Routes) != 1 || !plan.Routes[0].Matched {
		t.Fatalf("routes = %#v", plan.Routes)
	}
	if got := plan.Routes[0].Transforms; len(got) != 2 || got[0].Type != "redact" || got[1].Type != "explain_warnings" {
		t.Fatalf("transforms = %#v", got)
	}
	encoded := plan.Routes[0].Actions[0]
	if encoded.Type != "webhook" || encoded.ID != "legacy-action-0001" {
		t.Fatalf("action = %#v", encoded)
	}
	if strings.Contains(encoded.DestinationArtifactID, "ACTION-CONFIG-SENTINEL") {
		t.Fatalf("action exposed configuration: %#v", encoded)
	}
}

func TestParseDraftWorkflowRejectsUnsafeOrUnboundedYAML(t *testing.T) {
	tests := map[string][]byte{
		"multiple documents": []byte("name: one\nversion: 1\nroutes: []\n---\nname: two\n"),
		"explicit tag":       []byte("name: !!str tagged\nversion: 1\nroutes: []\n"),
		"oversized":          []byte(strings.Repeat("x", MaxPublishedWorkflowYAMLBytes+1)),
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDraftWorkflow(document); err == nil {
				t.Fatal("ParseDraftWorkflow() unexpectedly succeeded")
			}
		})
	}
}
