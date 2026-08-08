package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const processorPublishedWorkflow = `dsl_version: "1"
name: adt-preview
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
      source: adt-east
      condition: 'event.patient.mrn == "MRN-123"'
    actions:
      - id: audit-only
        type: log
      - id: send-fhir
        type: fhir
        destination: fhir-primary
  - name: unmatched
    filter:
      event_type: lab_result
    actions:
      - id: never
        type: log
  - name: invalid-cel
    filter:
      condition: "event.???"
    actions:
      - id: never-cel
        type: log
`

func TestPlanPreviewWorkflowBindsExactSuppressedDestinations(t *testing.T) {
	t.Parallel()

	resolved, revision, request := workflowPlanFixture(t, processorPublishedWorkflow)
	event, _, err := projectADTA01(
		projectorParseResult(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)),
		request,
		revision,
		0,
	)
	if err != nil {
		t.Fatalf("projectADTA01: %v", err)
	}

	routes, deliveries, diagnostics, err := planWorkflow(resolved, event, revision, integration.ExecutionModePreview)
	if err != nil {
		t.Fatalf("planPreviewWorkflow: %v", err)
	}
	if len(routes) != 3 {
		t.Fatalf("routes = %d, want 3", len(routes))
	}
	if !routes[0].Matched || routes[0].Skipped || routes[0].EventID != event.ID || !reflectStrings(routes[0].PlannedActions, []string{"audit-only", "send-fhir"}) {
		t.Fatalf("matched route = %#v", routes[0])
	}
	if routes[1].Matched || routes[1].TransformCount != 0 || len(routes[1].PlannedActions) != 0 {
		t.Fatalf("unmatched route = %#v", routes[1])
	}
	if routes[2].Matched || !reflectStrings(routes[2].DiagnosticCodes, []string{"INVALID_CEL"}) {
		t.Fatalf("invalid CEL route = %#v", routes[2])
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1: %#v", len(deliveries), deliveries)
	}
	delivery := deliveries[0]
	if delivery.EventID != event.ID || delivery.Route != "matched" || delivery.Action != "send-fhir" || delivery.Status != integration.DeliveryStatusSuppressed {
		t.Fatalf("delivery lineage/status = %#v", delivery)
	}
	if delivery.Destination != revision.Destinations[0] || delivery.AttemptID != "" || delivery.AttemptCount != 0 {
		t.Fatalf("delivery destination/attempt state = %#v", delivery)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "INVALID_CEL" || diagnostics[0].Path != "routes[2].filter.condition" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}

	result := integration.ProcessResult{
		Mode:                integration.ExecutionModePreview,
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		ArtifactRevisions: &integration.ExecutionArtifactRevisions{
			Source:   revision.Source.ArtifactRevisionRef,
			Profile:  resolved.ProfileReference(),
			Workflow: resolved.WorkflowReference(),
		},
		Security:    request.Security,
		Events:      []integration.ProcessedEvent{event},
		Diagnostics: diagnostics,
		Routes:      routes,
		Deliveries:  deliveries,
		Correlations: integration.CorrelationIDs{
			TenantID:        revision.TenantID,
			CorrelationID:   request.CorrelationID,
			SourceMessageID: event.SourceMessageID,
			EventIDs:        []string{event.ID},
		},
	}
	if err := result.ValidatePreviewFor(request, revision); err != nil {
		t.Fatalf("strict preview plan does not satisfy public contract: %v", err)
	}
}

func TestPlanPreviewWorkflowIsDeterministicAndNeverExposesArtifactBytes(t *testing.T) {
	t.Parallel()

	resolved, revision, request := workflowPlanFixture(t, processorPublishedWorkflow)
	event, _, err := projectADTA01(projectorParseResult(time.Now()), request, revision, 0)
	if err != nil {
		t.Fatalf("projectADTA01: %v", err)
	}
	firstRoutes, firstDeliveries, firstDiagnostics, err := planWorkflow(resolved, event, revision, integration.ExecutionModePreview)
	if err != nil {
		t.Fatalf("plan first: %v", err)
	}
	secondRoutes, secondDeliveries, secondDiagnostics, err := planWorkflow(resolved, event, revision, integration.ExecutionModePreview)
	if err != nil {
		t.Fatalf("plan second: %v", err)
	}
	first := marshalPlanParts(t, firstRoutes, firstDeliveries, firstDiagnostics)
	second := marshalPlanParts(t, secondRoutes, secondDeliveries, secondDiagnostics)
	if !bytes.Equal(first, second) {
		t.Fatalf("planning is nondeterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
	for _, forbidden := range []string{"event.???", "dsl_version", "destination: fhir-primary", "secret"} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("plan exposed workflow bytes/config %q: %s", forbidden, first)
		}
	}
}

// TestPlanPreviewWorkflowFailsClosedOnDestinationBinding is a regression guard,
// not a new feature: unmapped destinations have failed closed since Slice 2.3.
// Slice 4.1c-a's destination identity decision assumes it, because a destination
// absent from the deployed revision must never reach a delivery attempt in the
// first place. Both execution modes are covered so the durable path is pinned
// alongside preview.
func TestPlanPreviewWorkflowFailsClosedOnDestinationBinding(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing destination": strings.Replace(processorPublishedWorkflow, "        destination: fhir-primary\n", "", 1),
		"unknown destination": strings.Replace(processorPublishedWorkflow, "destination: fhir-primary", "destination: missing", 1),
		"log destination":     strings.Replace(processorPublishedWorkflow, "      - id: audit-only\n        type: log", "      - id: audit-only\n        type: log\n        destination: fhir-primary", 1),
	}
	for name, workflowYAML := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolved, revision, request := workflowPlanFixture(t, workflowYAML)
			event, _, err := projectADTA01(projectorParseResult(time.Now()), request, revision, 0)
			if err != nil {
				t.Fatalf("projectADTA01: %v", err)
			}
			for _, mode := range []integration.ExecutionMode{
				integration.ExecutionModePreview, integration.ExecutionModeProduction,
			} {
				if _, _, _, err := planWorkflow(resolved, event, revision, mode); !errors.Is(err, ErrInvalidWorkflowPlan) {
					t.Fatalf("plan error in %s mode = %v, want invalid workflow plan", mode, err)
				}
			}
		})
	}
}

func workflowPlanFixture(t *testing.T, workflowYAML string) (ResolvedArtifactRevisions, integration.IntegrationDefinitionRevision, integration.ProcessRequest) {
	t.Helper()
	workflowRef, err := NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(workflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	profileJSON := []byte(`{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`)
	profileRef, err := NewProfileRevisionReference("profile-adt", 7, profileJSON)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "revision-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
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
	envelope := projectorEnvelope(t, revision, []byte("message-one"), time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC))
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  "tenant-a",
			Principal: integration.Principal{ID: "source-service", Kind: integration.PrincipalKindService, AuthMethod: "mtls", SourceID: "adt-east"},
		},
		Envelope:      envelope,
		CorrelationID: "correlation-123",
	}
	resolved := ResolvedArtifactRevisions{
		profileRef:   profileRef,
		workflowRef:  workflowRef,
		profileJSON:  profileJSON,
		workflowYAML: []byte(workflowYAML),
	}
	return resolved, revision, request
}

func marshalPlanParts(t *testing.T, routes []integration.RouteResult, deliveries []integration.DeliveryResult, diagnostics []integration.Diagnostic) []byte {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Routes      []integration.RouteResult    `json:"routes"`
		Deliveries  []integration.DeliveryResult `json:"deliveries"`
		Diagnostics []integration.Diagnostic     `json:"diagnostics"`
	}{Routes: routes, Deliveries: deliveries, Diagnostics: diagnostics})
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	return encoded
}

func reflectStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
