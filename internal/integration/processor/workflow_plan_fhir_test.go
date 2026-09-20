package processor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const fhirUnsupportedRouteWorkflow = `dsl_version: "1"
name: documents-to-fhir
version: "1"
routes:
  - name: documents
    filter:
      event_type: document_reference
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
      - id: send-webhook
        type: webhook
        destination: fhir-primary
`

// TestPlanWorkflowReportsAnUnprojectableFHIRActionAtPlanTime is Slice 4.1c-c's
// IDE/runtime parity proof (proof-matrix row 5): a `fhir` action on a route
// whose event type internal/integration/fhirout cannot project produces
// FHIR_PROJECTION_UNSUPPORTED at dry-run and at publish, and queues nothing for
// that action — while every other action on the route is planned as before.
func TestPlanWorkflowReportsAnUnprojectableFHIRActionAtPlanTime(t *testing.T) {
	t.Parallel()

	resolved, revision, _ := workflowPlanFixture(t, fhirUnsupportedRouteWorkflow)
	event, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       revision.TenantID,
		Classification: integration.DataClassificationPHI,
	}, &events.DocumentReferenceEvent{
		EventMeta: events.EventMeta{
			ID: "event-document-1", Type: events.EventType("document_reference"),
			Timestamp: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC), ReceivedAt: time.Date(2026, 7, 13, 12, 0, 1, 0, time.UTC),
			Source: "adt-east", SourceFormat: events.FormatHL7v2, SourceProfileID: "profile-adt",
			SourceMessageID: "control-document-1", CorrelationID: "correlation-123",
		},
		Patient: &events.Patient{MRN: "MRN-123"},
	})
	if err != nil {
		t.Fatalf("NewProcessedEvent(document_reference): %v", err)
	}

	for _, mode := range []integration.ExecutionMode{integration.ExecutionModePreview, integration.ExecutionModeProduction} {
		routes, deliveries, diagnostics, err := planWorkflow(resolved, event, revision, mode)
		if err != nil {
			t.Fatalf("planWorkflow(%s): %v", mode, err)
		}
		if len(routes) != 1 || !routes[0].Matched || !reflectStrings(routes[0].PlannedActions, []string{"send-fhir", "send-webhook"}) {
			t.Fatalf("%s: routes = %+v, want the documents route matched with both actions planned", mode, routes)
		}
		if !reflectStrings(routes[0].DiagnosticCodes, []string{"FHIR_PROJECTION_UNSUPPORTED"}) {
			t.Fatalf("%s: route diagnostic codes = %v, want [FHIR_PROJECTION_UNSUPPORTED]", mode, routes[0].DiagnosticCodes)
		}
		if len(deliveries) != 1 || deliveries[0].Action != "send-webhook" {
			t.Fatalf("%s: deliveries = %+v, want exactly the webhook action — the fhir action must queue nothing", mode, deliveries)
		}
		if len(diagnostics) != 1 {
			t.Fatalf("%s: diagnostics = %+v, want exactly one", mode, diagnostics)
		}
		diagnostic := diagnostics[0]
		if diagnostic.Code != "FHIR_PROJECTION_UNSUPPORTED" || diagnostic.Path != "routes[0].actions[0]" ||
			diagnostic.Severity != integration.DiagnosticSeverityWarning || diagnostic.Stage != "workflow" ||
			diagnostic.Source() != "planner" || diagnostic.TenantID != revision.TenantID {
			t.Fatalf("%s: diagnostic = %+v", mode, diagnostic)
		}
	}
}

// TestPlanWorkflowQueuesAProjectableFHIRAction is the control: the same
// planner over an admit still queues the fhir action with no diagnostic, so the
// gate above is measuring projectability rather than the action type.
func TestPlanWorkflowQueuesAProjectableFHIRAction(t *testing.T) {
	t.Parallel()

	resolved, revision, request := workflowPlanFixture(t, processorPublishedWorkflow)
	event, _, err := projectADTA01(
		projectorParseResult(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)), request, revision, 0,
	)
	if err != nil {
		t.Fatalf("projectADTA01: %v", err)
	}
	routes, deliveries, diagnostics, err := planWorkflow(resolved, event, revision, integration.ExecutionModeProduction)
	if err != nil {
		t.Fatalf("planWorkflow: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Action != "send-fhir" || deliveries[0].Status != integration.DeliveryStatusPlanned {
		t.Fatalf("deliveries = %+v, want the fhir action planned", deliveries)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "FHIR_PROJECTION_UNSUPPORTED" {
			t.Fatalf("an admit produced FHIR_PROJECTION_UNSUPPORTED: %+v", diagnostics)
		}
	}
	for _, route := range routes {
		for _, code := range route.DiagnosticCodes {
			if code == "FHIR_PROJECTION_UNSUPPORTED" {
				t.Fatalf("route %s carries FHIR_PROJECTION_UNSUPPORTED for an admit", route.Route)
			}
		}
	}
}

func TestPlanWorkflowQueuesClinicalFHIRActions(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/fhir/clinical-events.json")
	if err != nil {
		t.Fatal(err)
	}
	var payloads []json.RawMessage
	if err := json.Unmarshal(raw, &payloads); err != nil {
		t.Fatal(err)
	}
	for _, payload := range payloads {
		var meta events.EventMeta
		if err := json.Unmarshal(payload, &meta); err != nil {
			t.Fatal(err)
		}
		t.Run(string(meta.Type), func(t *testing.T) {
			workflow := strings.ReplaceAll(fhirUnsupportedRouteWorkflow, "document_reference", string(meta.Type))
			resolved, revision, _ := workflowPlanFixture(t, workflow)
			canonical, err := integration.DecodeCanonicalEventPayload(meta.Type, payload)
			if err != nil {
				t.Fatal(err)
			}
			event, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{TenantID: revision.TenantID, Classification: integration.DataClassificationPHI}, canonical)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []integration.ExecutionMode{integration.ExecutionModePreview, integration.ExecutionModeProduction} {
				_, deliveries, diagnostics, err := planWorkflow(resolved, event, revision, mode)
				if err != nil {
					t.Fatal(err)
				}
				if len(deliveries) != 2 || deliveries[0].Action != "send-fhir" {
					t.Fatalf("deliveries = %+v", deliveries)
				}
				for _, diagnostic := range diagnostics {
					if diagnostic.Code == "FHIR_PROJECTION_UNSUPPORTED" {
						t.Fatalf("clinical event refused: %+v", diagnostic)
					}
				}
			}
		})
	}
}
