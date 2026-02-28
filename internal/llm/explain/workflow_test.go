package explain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

const sampleWorkflowYAML = `name: ADT Router
routes:
  - name: patient-admit
    filter:
      event_type: ADT
      cel: "event.patient_class == 'I'"
    actions:
      - type: webhook
        url: https://ehr.example.com/admit
  - name: lab-result
    filter:
      event_type: ORU
    transform:
      terminology_map: true
    actions:
      - type: fhir
        endpoint: https://fhir.example.com/Observation
`

func TestNewWorkflowExplainer(t *testing.T) {
	client := llm.NewMockClient()
	e := NewWorkflowExplainer(client, "test-model")
	if e == nil {
		t.Fatal("expected non-nil explainer")
	}
	if e.model != "test-model" {
		t.Fatalf("model=%q want test-model", e.model)
	}
}

func TestWorkflowExplainer_Explain(t *testing.T) {
	ctx := context.Background()

	t.Run("returns explanation for valid YAML", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"summary":     "Routes ADT and ORU messages",
			"description": "This workflow routes patient admissions to a webhook and lab results to a FHIR server.",
			"route_explanations": []map[string]interface{}{
				{
					"name":        "patient-admit",
					"trigger":     "ADT events for inpatients",
					"actions":     []string{"Send to EHR webhook"},
					"description": "Routes inpatient admissions",
				},
			},
			"warnings": []string{"No error handling configured"},
		})

		e := NewWorkflowExplainer(client, "test-model")
		result, err := e.Explain(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("Explain error: %v", err)
		}

		if result.Summary != "Routes ADT and ORU messages" {
			t.Errorf("Summary=%q", result.Summary)
		}
		if result.Description == "" {
			t.Error("expected non-empty Description")
		}
		if len(result.RouteExplanations) != 1 {
			t.Errorf("RouteExplanations len=%d want 1", len(result.RouteExplanations))
		}
		if len(result.Warnings) != 1 {
			t.Errorf("Warnings len=%d want 1", len(result.Warnings))
		}
		if client.CallCount() != 1 {
			t.Errorf("CallCount=%d want 1", client.CallCount())
		}
	})

	t.Run("returns error for empty YAML", func(t *testing.T) {
		client := llm.NewMockClient()
		e := NewWorkflowExplainer(client, "test-model")

		_, err := e.Explain(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty YAML")
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM unavailable"))

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.Explain(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error on LLM failure")
		}
	})

	t.Run("returns error on invalid JSON response", func(t *testing.T) {
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{invalid json`), nil
		}

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.Explain(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error on invalid JSON")
		}
	})
}

func TestWorkflowExplainer_ExplainForAudience(t *testing.T) {
	ctx := context.Background()

	validResponse := map[string]interface{}{
		"summary":            "Routes ADT messages",
		"description":        "Detailed explanation",
		"route_explanations": []map[string]interface{}{},
	}

	audiences := []struct {
		name     string
		audience string
	}{
		{"technical", "technical"},
		{"developer", "developer"},
		{"business", "business"},
		{"analyst", "analyst"},
		{"compliance", "compliance"},
		{"audit", "audit"},
		{"operations", "operations"},
		{"ops", "ops"},
		{"unknown audience", "executive"},
	}

	for _, tc := range audiences {
		t.Run(tc.name, func(t *testing.T) {
			client := llm.NewMockClient()
			client.WithJSONResponse(validResponse)

			e := NewWorkflowExplainer(client, "test-model")
			result, err := e.ExplainForAudience(ctx, sampleWorkflowYAML, tc.audience)
			if err != nil {
				t.Fatalf("ExplainForAudience(%q) error: %v", tc.audience, err)
			}
			if result.Summary == "" {
				t.Error("expected non-empty Summary")
			}
		})
	}

	t.Run("returns error for empty YAML", func(t *testing.T) {
		client := llm.NewMockClient()
		e := NewWorkflowExplainer(client, "test-model")

		_, err := e.ExplainForAudience(ctx, "", "technical")
		if err == nil {
			t.Fatal("expected error for empty YAML")
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM error"))

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.ExplainForAudience(ctx, sampleWorkflowYAML, "business")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			return json.RawMessage(`not json`), nil
		}

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.ExplainForAudience(ctx, sampleWorkflowYAML, "technical")
		if err == nil {
			t.Fatal("expected error on bad JSON")
		}
	})
}

func TestWorkflowExplainer_GenerateDiagram(t *testing.T) {
	ctx := context.Background()

	t.Run("returns Mermaid diagram", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithCompleteResponse("```mermaid\ngraph TD\n  A-->B\n```")

		e := NewWorkflowExplainer(client, "test-model")
		diagram, err := e.GenerateDiagram(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("GenerateDiagram error: %v", err)
		}
		if diagram == "" {
			t.Error("expected non-empty diagram")
		}
		// Should strip mermaid code fences
		if diagram != "graph TD\n  A-->B" {
			t.Errorf("diagram=%q", diagram)
		}
	})

	t.Run("handles raw diagram without code fences", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithCompleteResponse("graph LR\n  Start-->End")

		e := NewWorkflowExplainer(client, "test-model")
		diagram, err := e.GenerateDiagram(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("GenerateDiagram error: %v", err)
		}
		if diagram != "graph LR\n  Start-->End" {
			t.Errorf("diagram=%q", diagram)
		}
	})

	t.Run("returns error for empty YAML", func(t *testing.T) {
		client := llm.NewMockClient()
		e := NewWorkflowExplainer(client, "test-model")

		_, err := e.GenerateDiagram(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty YAML")
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM error"))

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.GenerateDiagram(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error on LLM failure")
		}
	})
}

func TestWorkflowExplainer_GenerateDocumentation(t *testing.T) {
	ctx := context.Background()

	t.Run("generates full documentation", func(t *testing.T) {
		callNum := 0
		client := llm.NewMockClient()
		// Explain and generateUseCasesAndLimitations use CompleteStructured
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			callNum++
			switch schemaName {
			case "workflow_explanation":
				return json.Marshal(map[string]interface{}{
					"summary":            "ADT routing workflow",
					"description":        "Routes patient events",
					"route_explanations": []map[string]interface{}{},
				})
			case "use_cases_limitations":
				return json.Marshal(map[string]interface{}{
					"use_cases":   []string{"Inpatient routing"},
					"limitations": []string{"No retry logic"},
				})
			default:
				return json.RawMessage(`{}`), nil
			}
		}
		// GenerateDiagram uses Complete
		client.CompleteFunc = func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Choices: []llm.Choice{{Message: llm.Message{Content: "graph TD\n  A-->B"}}},
			}, nil
		}

		e := NewWorkflowExplainer(client, "test-model")
		doc, err := e.GenerateDocumentation(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("GenerateDocumentation error: %v", err)
		}

		if doc.Title != "ADT Router" {
			t.Errorf("Title=%q want ADT Router", doc.Title)
		}
		if doc.Overview != "ADT routing workflow" {
			t.Errorf("Overview=%q", doc.Overview)
		}
		if doc.Diagram == "" {
			t.Error("expected non-empty Diagram")
		}
		if len(doc.UseCases) != 1 {
			t.Errorf("UseCases len=%d want 1", len(doc.UseCases))
		}
		if len(doc.Limitations) != 1 {
			t.Errorf("Limitations len=%d want 1", len(doc.Limitations))
		}
	})

	t.Run("continues without diagram on error", func(t *testing.T) {
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			switch schemaName {
			case "workflow_explanation":
				return json.Marshal(map[string]interface{}{
					"summary":            "Workflow summary",
					"description":        "Workflow description",
					"route_explanations": []map[string]interface{}{},
				})
			case "use_cases_limitations":
				return json.Marshal(map[string]interface{}{
					"use_cases":   []string{"Use case"},
					"limitations": []string{"Limitation"},
				})
			default:
				return json.RawMessage(`{}`), nil
			}
		}
		// Diagram generation fails
		client.CompleteFunc = func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return nil, errors.New("diagram generation failed")
		}

		e := NewWorkflowExplainer(client, "test-model")
		doc, err := e.GenerateDocumentation(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("GenerateDocumentation should not fail: %v", err)
		}
		if doc.Diagram != "" {
			t.Errorf("Diagram should be empty on error, got %q", doc.Diagram)
		}
	})

	t.Run("continues without use cases on error", func(t *testing.T) {
		callNum := 0
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			callNum++
			switch schemaName {
			case "workflow_explanation":
				return json.Marshal(map[string]interface{}{
					"summary":            "Workflow summary",
					"description":        "Workflow description",
					"route_explanations": []map[string]interface{}{},
				})
			case "use_cases_limitations":
				return nil, errors.New("use cases failed")
			default:
				return json.RawMessage(`{}`), nil
			}
		}
		client.WithCompleteResponse("graph TD\n  A-->B")

		e := NewWorkflowExplainer(client, "test-model")
		doc, err := e.GenerateDocumentation(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("GenerateDocumentation should not fail: %v", err)
		}
		if doc.UseCases != nil {
			t.Errorf("UseCases should be nil on error, got %v", doc.UseCases)
		}
		if doc.Limitations != nil {
			t.Errorf("Limitations should be nil on error, got %v", doc.Limitations)
		}
	})

	t.Run("returns error when Explain fails", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("explain failed"))

		e := NewWorkflowExplainer(client, "test-model")
		_, err := e.GenerateDocumentation(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error when Explain fails")
		}
	})
}

func TestExtractWorkflowName(t *testing.T) {
	t.Run("extracts name from YAML", func(t *testing.T) {
		name := extractWorkflowName("name: ADT Router\nroutes:\n  - name: foo")
		if name != "ADT Router" {
			t.Errorf("name=%q want ADT Router", name)
		}
	})

	t.Run("returns default for missing name", func(t *testing.T) {
		name := extractWorkflowName("routes:\n  - name: foo")
		if name != "Workflow" {
			t.Errorf("name=%q want Workflow", name)
		}
	})

	t.Run("returns default for empty YAML", func(t *testing.T) {
		name := extractWorkflowName("")
		if name != "Workflow" {
			t.Errorf("name=%q want Workflow", name)
		}
	})
}

func TestGenerateUseCasesAndLimitations(t *testing.T) {
	ctx := context.Background()

	t.Run("returns use cases and limitations", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"use_cases":   []string{"Route ADT messages", "Filter by patient class"},
			"limitations": []string{"No retry logic", "No DLQ"},
		})

		e := NewWorkflowExplainer(client, "test-model")
		useCases, limitations, err := e.generateUseCasesAndLimitations(ctx, sampleWorkflowYAML)
		if err != nil {
			t.Fatalf("generateUseCasesAndLimitations error: %v", err)
		}
		if len(useCases) != 2 {
			t.Errorf("useCases len=%d want 2", len(useCases))
		}
		if len(limitations) != 2 {
			t.Errorf("limitations len=%d want 2", len(limitations))
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM error"))

		e := NewWorkflowExplainer(client, "test-model")
		_, _, err := e.generateUseCasesAndLimitations(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{bad json`), nil
		}

		e := NewWorkflowExplainer(client, "test-model")
		_, _, err := e.generateUseCasesAndLimitations(ctx, sampleWorkflowYAML)
		if err == nil {
			t.Fatal("expected error on bad JSON")
		}
	})
}
