package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// CELAssistant helps generate CEL expressions from natural language.
type CELAssistant struct {
	client llm.Client
	model  string
}

// NewCELAssistant creates a new CEL expression assistant.
func NewCELAssistant(client llm.Client, model string) *CELAssistant {
	return &CELAssistant{
		client: client,
		model:  model,
	}
}

// GenerateCELRequest contains parameters for CEL generation.
type GenerateCELRequest struct {
	// Description is the natural language description of the condition.
	Description string `json:"description"`

	// EventSchema describes the structure of events (optional).
	EventSchema *EventSchema `json:"event_schema,omitempty"`

	// Examples are example CEL expressions for few-shot learning.
	Examples []CELExample `json:"examples,omitempty"`
}

// EventSchema describes the structure of events for CEL generation.
type EventSchema struct {
	EventTypes    []string          `json:"event_types,omitempty"`
	PatientFields []string          `json:"patient_fields,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"` // field path -> description
}

// CELExample provides an example for few-shot learning.
type CELExample struct {
	Description string `json:"description"`
	Expression  string `json:"expression"`
}

// GenerateCELResult contains the generated CEL expression.
type GenerateCELResult struct {
	// Expression is the generated CEL expression.
	Expression string `json:"expression"`

	// Explanation describes what the expression does.
	Explanation string `json:"explanation"`

	// Alternatives are alternative expressions for the same condition.
	Alternatives []string `json:"alternatives,omitempty"`

	// FieldsUsed lists the event fields used in the expression.
	FieldsUsed []string `json:"fields_used,omitempty"`
}

// Generate generates a CEL expression from a natural language description.
func (a *CELAssistant) Generate(ctx context.Context, req GenerateCELRequest) (*GenerateCELResult, error) {
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	systemPrompt := buildCELSystemPrompt(req.EventSchema)
	userPrompt := buildCELUserPrompt(req)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       a.model,
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	rawJSON, err := a.client.CompleteStructured(ctx, llmReq, "cel_generation", celGenSchema)
	if err != nil {
		return nil, fmt.Errorf("generate cel: %w", err)
	}

	var resp celGenResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &GenerateCELResult{
		Expression:   resp.Expression,
		Explanation:  resp.Explanation,
		Alternatives: resp.Alternatives,
		FieldsUsed:   resp.FieldsUsed,
	}, nil
}

// Explain generates an explanation for a CEL expression.
func (a *CELAssistant) Explain(ctx context.Context, expression string) (string, error) {
	if expression == "" {
		return "", fmt.Errorf("expression is required")
	}

	prompt := fmt.Sprintf(`Explain what this CEL expression does in plain English:

%s

Provide a clear, concise explanation that a non-technical person could understand.`, expression)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage("You are a CEL (Common Expression Language) expert. Explain expressions in plain English."),
			llm.UserMessage(prompt),
		},
		Model:       a.model,
		Temperature: 0.2,
		MaxTokens:   512,
	}

	resp, err := a.client.Complete(ctx, llmReq)
	if err != nil {
		return "", fmt.Errorf("explain cel: %w", err)
	}

	return resp.Content(), nil
}

// Validate validates a CEL expression and suggests corrections.
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	Error       string   `json:"error,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Corrected   string   `json:"corrected,omitempty"`
}

// ValidateAndCorrect validates a CEL expression and suggests corrections if invalid.
func (a *CELAssistant) ValidateAndCorrect(ctx context.Context, expression string) (*ValidationResult, error) {
	if expression == "" {
		return &ValidationResult{Valid: false, Error: "empty expression"}, nil
	}

	prompt := fmt.Sprintf(`Analyze this CEL expression for healthcare event filtering:

%s

Check for:
1. Syntax errors
2. Common mistakes
3. Potential improvements

If there are issues, suggest corrections.`, expression)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(celValidationSystemPrompt),
			llm.UserMessage(prompt),
		},
		Model:       a.model,
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	rawJSON, err := a.client.CompleteStructured(ctx, llmReq, "cel_validation", celValidationSchema)
	if err != nil {
		return nil, fmt.Errorf("validate cel: %w", err)
	}

	var resp ValidationResult
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}

// celGenResponse is the LLM response structure for CEL generation.
type celGenResponse struct {
	Expression   string   `json:"expression"`
	Explanation  string   `json:"explanation"`
	Alternatives []string `json:"alternatives,omitempty"`
	FieldsUsed   []string `json:"fields_used,omitempty"`
}

var celGenSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"expression": map[string]interface{}{
			"type":        "string",
			"description": "The generated CEL expression",
		},
		"explanation": map[string]interface{}{
			"type":        "string",
			"description": "Explanation of what the expression does",
		},
		"alternatives": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Alternative expressions for the same condition",
		},
		"fields_used": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Event fields used in the expression",
		},
	},
	"required": []string{"expression", "explanation"},
}

var celValidationSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"valid": map[string]interface{}{
			"type":        "boolean",
			"description": "Whether the expression is valid",
		},
		"error": map[string]interface{}{
			"type":        "string",
			"description": "Error message if invalid",
		},
		"suggestions": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Suggestions for improvement",
		},
		"corrected": map[string]interface{}{
			"type":        "string",
			"description": "Corrected expression if there were issues",
		},
	},
	"required": []string{"valid"},
}

func buildCELSystemPrompt(schema *EventSchema) string {
	var sb strings.Builder
	sb.WriteString(`You are a CEL (Common Expression Language) expert for healthcare event filtering. Generate CEL expressions for the fi-fhir workflow system.

## CEL Basics
- Access event fields with dot notation: event.patient.age
- String comparison: event.event_type == "ADT"
- Numeric comparison: event.patient.age >= 65
- Boolean operators: && (and), || (or), ! (not)
- String functions: contains(), startsWith(), endsWith()
- List operations: size(), has()
- Null-safe access: has(event.patient) && event.patient.age > 0

## Common Event Fields
- event.event_type: ADT, ORU, ORM, MDM, etc.
- event.event_id: Unique event identifier
- event.timestamp: Event timestamp
- event.source: Source system identifier

## Patient Fields
- event.patient.mrn: Medical Record Number
- event.patient.name: Patient name
- event.patient.dob: Date of birth
- event.patient.age: Patient age (computed)
- event.patient.gender: Patient gender
- event.patient.ssn: Social Security Number

## Observation Fields (ORU)
- event.observation.code: Observation code
- event.observation.value: Observation value
- event.observation.interpretation: Interpretation (normal, abnormal, critical)
- event.observation.status: Result status

## Encounter Fields
- event.encounter.type: Encounter type
- event.encounter.location: Location
- event.encounter.status: Encounter status

`)

	if schema != nil {
		if len(schema.Fields) > 0 {
			sb.WriteString("\n## Custom Fields\n")
			for path, desc := range schema.Fields {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", path, desc))
			}
		}
	}

	sb.WriteString(`
## Example Expressions
- Patient over 65: event.patient.age >= 65
- Critical lab: event.observation.interpretation == "critical"
- ADT admits: event.event_type == "ADT" && event.encounter.type == "I"
- Has MRN: has(event.patient) && event.patient.mrn != ""
- Multiple conditions: event.patient.age >= 65 && event.observation.interpretation == "abnormal"

## Guidelines
1. Use proper CEL syntax
2. Prefer explicit comparisons over truthy checks
3. Use has() for optional field access
4. Keep expressions readable
`)

	return sb.String()
}

func buildCELUserPrompt(req GenerateCELRequest) string {
	var sb strings.Builder
	sb.WriteString("Generate a CEL expression for:\n\n")
	sb.WriteString(req.Description)

	if len(req.Examples) > 0 {
		sb.WriteString("\n\n## Examples for Reference\n")
		for _, ex := range req.Examples {
			sb.WriteString(fmt.Sprintf("- \"%s\": `%s`\n", ex.Description, ex.Expression))
		}
	}

	return sb.String()
}

const celValidationSystemPrompt = `You are a CEL (Common Expression Language) validator for healthcare event filtering.

Your task is to:
1. Check for syntax errors
2. Identify common mistakes
3. Suggest improvements for clarity and correctness
4. Provide a corrected version if needed

Common issues to check:
- Missing quotes around strings
- Wrong comparison operators
- Incorrect field paths
- Missing null checks for optional fields
- Type mismatches`
