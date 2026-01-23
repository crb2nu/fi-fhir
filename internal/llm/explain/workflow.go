package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// WorkflowExplainer provides human-readable explanations for workflows.
type WorkflowExplainer struct {
	client llm.Client
	model  string
}

// NewWorkflowExplainer creates a new workflow explainer.
func NewWorkflowExplainer(client llm.Client, model string) *WorkflowExplainer {
	return &WorkflowExplainer{
		client: client,
		model:  model,
	}
}

// WorkflowExplanation contains the explanation of a workflow.
type WorkflowExplanation struct {
	// Summary is a brief overview of what the workflow does.
	Summary string `json:"summary"`

	// Description is a detailed explanation.
	Description string `json:"description"`

	// RouteExplanations explains each route.
	RouteExplanations []RouteExplanation `json:"route_explanations"`

	// Diagram is an optional Mermaid diagram of the workflow.
	Diagram string `json:"diagram,omitempty"`

	// Warnings contains any potential issues or concerns.
	Warnings []string `json:"warnings,omitempty"`
}

// RouteExplanation explains a single route.
type RouteExplanation struct {
	Name        string   `json:"name"`
	Trigger     string   `json:"trigger"`
	Actions     []string `json:"actions"`
	Description string   `json:"description"`
}

// Explain generates a human-readable explanation of a workflow YAML.
func (e *WorkflowExplainer) Explain(ctx context.Context, workflowYAML string) (*WorkflowExplanation, error) {
	if workflowYAML == "" {
		return nil, fmt.Errorf("workflow YAML is required")
	}

	systemPrompt := workflowExplainerSystemPrompt
	userPrompt := fmt.Sprintf("Explain this workflow in plain English:\n\n```yaml\n%s\n```", workflowYAML)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       e.model,
		Temperature: 0.2,
		MaxTokens:   2048,
	}

	rawJSON, err := e.client.CompleteStructured(ctx, llmReq, "workflow_explanation", workflowExplainSchema)
	if err != nil {
		return nil, fmt.Errorf("explain workflow: %w", err)
	}

	var resp WorkflowExplanation
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}

// ExplainForAudience generates an explanation targeted at a specific audience.
func (e *WorkflowExplainer) ExplainForAudience(ctx context.Context, workflowYAML, audience string) (*WorkflowExplanation, error) {
	if workflowYAML == "" {
		return nil, fmt.Errorf("workflow YAML is required")
	}

	audienceContext := ""
	switch strings.ToLower(audience) {
	case "technical", "developer":
		audienceContext = "This explanation is for technical developers. Include implementation details, CEL expression explanations, and integration considerations."
	case "business", "analyst":
		audienceContext = "This explanation is for business analysts. Focus on business logic, data flows, and operational impact without technical jargon."
	case "compliance", "audit":
		audienceContext = "This explanation is for compliance/audit review. Emphasize data handling, PHI exposure, security considerations, and regulatory implications."
	case "operations", "ops":
		audienceContext = "This explanation is for operations staff. Focus on monitoring, alerting, failure modes, and operational procedures."
	default:
		audienceContext = "Provide a general explanation suitable for mixed audiences."
	}

	systemPrompt := workflowExplainerSystemPrompt + "\n\n" + audienceContext
	userPrompt := fmt.Sprintf("Explain this workflow:\n\n```yaml\n%s\n```", workflowYAML)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       e.model,
		Temperature: 0.2,
		MaxTokens:   2048,
	}

	rawJSON, err := e.client.CompleteStructured(ctx, llmReq, "workflow_explanation", workflowExplainSchema)
	if err != nil {
		return nil, fmt.Errorf("explain workflow: %w", err)
	}

	var resp WorkflowExplanation
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}

// GenerateDiagram generates a Mermaid flowchart diagram for a workflow.
func (e *WorkflowExplainer) GenerateDiagram(ctx context.Context, workflowYAML string) (string, error) {
	if workflowYAML == "" {
		return "", fmt.Errorf("workflow YAML is required")
	}

	prompt := fmt.Sprintf(`Generate a Mermaid flowchart diagram for this workflow:

%s

Create a clear flow diagram showing:
1. Event triggers
2. Filters/conditions
3. Transformations
4. Actions

Use proper Mermaid flowchart syntax (graph TD or graph LR).
Only output the Mermaid diagram code, nothing else.`, workflowYAML)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage("You are a diagram generation expert. Create clear, accurate Mermaid flowchart diagrams."),
			llm.UserMessage(prompt),
		},
		Model:       e.model,
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	resp, err := e.client.Complete(ctx, llmReq)
	if err != nil {
		return "", fmt.Errorf("generate diagram: %w", err)
	}

	// Extract Mermaid code if wrapped in code block
	content := resp.Content()
	content = strings.TrimPrefix(content, "```mermaid")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// GenerateDocumentation generates full documentation for a workflow.
type WorkflowDocumentation struct {
	Title       string              `json:"title"`
	Overview    string              `json:"overview"`
	Explanation WorkflowExplanation `json:"explanation"`
	Diagram     string              `json:"diagram"`
	UseCases    []string            `json:"use_cases"`
	Limitations []string            `json:"limitations"`
}

// GenerateDocumentation generates comprehensive documentation.
func (e *WorkflowExplainer) GenerateDocumentation(ctx context.Context, workflowYAML string) (*WorkflowDocumentation, error) {
	// Get explanation
	explanation, err := e.Explain(ctx, workflowYAML)
	if err != nil {
		return nil, fmt.Errorf("explain workflow: %w", err)
	}

	// Get diagram
	diagram, err := e.GenerateDiagram(ctx, workflowYAML)
	if err != nil {
		// Non-fatal, continue without diagram
		diagram = ""
	}

	// Generate use cases and limitations
	useCases, limitations, err := e.generateUseCasesAndLimitations(ctx, workflowYAML)
	if err != nil {
		useCases = nil
		limitations = nil
	}

	return &WorkflowDocumentation{
		Title:       extractWorkflowName(workflowYAML),
		Overview:    explanation.Summary,
		Explanation: *explanation,
		Diagram:     diagram,
		UseCases:    useCases,
		Limitations: limitations,
	}, nil
}

func (e *WorkflowExplainer) generateUseCasesAndLimitations(ctx context.Context, workflowYAML string) ([]string, []string, error) {
	prompt := fmt.Sprintf(`Analyze this workflow and provide:
1. Common use cases (when/why you would use this workflow)
2. Limitations (what this workflow cannot do or potential issues)

Workflow:
%s

Provide 2-4 use cases and 2-4 limitations.`, workflowYAML)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage("You are a healthcare integration expert. Analyze workflows for use cases and limitations."),
			llm.UserMessage(prompt),
		},
		Model:       e.model,
		Temperature: 0.2,
		MaxTokens:   1024,
	}

	rawJSON, err := e.client.CompleteStructured(ctx, llmReq, "use_cases_limitations", useCasesSchema)
	if err != nil {
		return nil, nil, err
	}

	var resp struct {
		UseCases    []string `json:"use_cases"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, nil, err
	}

	return resp.UseCases, resp.Limitations, nil
}

func extractWorkflowName(yaml string) string {
	// Simple extraction - look for "name:" line
	for _, line := range strings.Split(yaml, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
	}
	return "Workflow"
}

var workflowExplainSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"summary": map[string]interface{}{
			"type":        "string",
			"description": "Brief 1-2 sentence overview of the workflow",
		},
		"description": map[string]interface{}{
			"type":        "string",
			"description": "Detailed explanation of the workflow",
		},
		"route_explanations": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"trigger":     map[string]interface{}{"type": "string"},
					"actions":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"description": map[string]interface{}{"type": "string"},
				},
			},
			"description": "Explanation of each route",
		},
		"diagram": map[string]interface{}{
			"type":        "string",
			"description": "Optional Mermaid diagram",
		},
		"warnings": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Potential issues or concerns",
		},
	},
	"required": []string{"summary", "description", "route_explanations"},
}

var useCasesSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"use_cases": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		},
		"limitations": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		},
	},
	"required": []string{"use_cases", "limitations"},
}

const workflowExplainerSystemPrompt = `You are a healthcare data integration expert. Your task is to explain fi-fhir workflow configurations in plain English.

## Workflow Components

**Routes**: Define how events are processed
- Filter: Matches events by type, source, or CEL condition
- Transform: Modifies events (terminology mapping, field setting, redaction)
- Actions: Execute on matched events

**Event Types**:
- ADT: Admit/Discharge/Transfer (patient movement)
- ORU: Observation Result (lab results)
- ORM: Order Message (orders)
- MDM: Medical Document (clinical documents)
- DFT: Financial Transaction (billing)
- SIU: Scheduling (appointments)
- RDE: Pharmacy (prescriptions)

**Action Types**:
- webhook: Send HTTP POST to external system
- fhir: Convert to FHIR and send to FHIR server
- log: Log the event
- email: Send email notification
- file: Write to file

## Guidelines for Explanations

1. Start with a high-level summary
2. Explain each route in business terms
3. Identify what triggers each route
4. Describe what happens when triggered
5. Note any transformations applied
6. Highlight potential concerns or issues
7. Avoid excessive technical jargon
8. Use healthcare domain terminology appropriately`
