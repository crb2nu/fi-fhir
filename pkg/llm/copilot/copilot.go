// Package copilot provides LLM-powered workflow generation and assistance.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// WorkflowCopilot generates workflows from natural language descriptions.
type WorkflowCopilot struct {
	client llm.Client
	model  string
}

// CopilotConfig configures the workflow copilot.
type CopilotConfig struct {
	// Client is the LLM client to use.
	Client llm.Client

	// Model is the model to use (defaults to client's default).
	Model string
}

// NewWorkflowCopilot creates a new workflow copilot.
func NewWorkflowCopilot(cfg CopilotConfig) (*WorkflowCopilot, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}

	return &WorkflowCopilot{
		client: cfg.Client,
		model:  cfg.Model,
	}, nil
}

// GenerateRequest contains parameters for workflow generation.
type GenerateRequest struct {
	// Description is the natural language description of the workflow.
	Description string `json:"description"`

	// EventTypes are the available event types (optional, for context).
	EventTypes []string `json:"event_types,omitempty"`

	// ActionTypes are the available action types (optional, for context).
	ActionTypes []string `json:"action_types,omitempty"`

	// Examples are example workflows for few-shot learning (optional).
	Examples []WorkflowExample `json:"examples,omitempty"`
}

// WorkflowExample provides an example for few-shot learning.
type WorkflowExample struct {
	Description string `json:"description"`
	YAML        string `json:"yaml"`
}

// GenerateResult contains the generated workflow.
type GenerateResult struct {
	// YAML is the generated workflow YAML.
	YAML string `json:"yaml"`

	// Explanation describes what the workflow does.
	Explanation string `json:"explanation"`

	// Warnings contains any warnings about the generated workflow.
	Warnings []string `json:"warnings,omitempty"`
}

// Generate generates a workflow from a natural language description.
func (c *WorkflowCopilot) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	systemPrompt := buildWorkflowSystemPrompt(req.EventTypes, req.ActionTypes)
	userPrompt := buildWorkflowUserPrompt(req)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       c.model,
		Temperature: 0.2,
		MaxTokens:   2048,
	}

	rawJSON, err := c.client.CompleteStructured(ctx, llmReq, "workflow_generation", workflowGenSchema)
	if err != nil {
		return nil, fmt.Errorf("generate workflow: %w", err)
	}

	var resp workflowGenResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &GenerateResult{
		YAML:        resp.YAML,
		Explanation: resp.Explanation,
		Warnings:    resp.Warnings,
	}, nil
}

// InteractiveSession manages an interactive workflow building session.
type InteractiveSession struct {
	copilot      *WorkflowCopilot
	conversation []llm.Message
	currentYAML  string
}

// NewInteractiveSession starts a new interactive session.
func (c *WorkflowCopilot) NewInteractiveSession() *InteractiveSession {
	return &InteractiveSession{
		copilot: c,
		conversation: []llm.Message{
			llm.SystemMessage(interactiveSystemPrompt),
		},
	}
}

// Chat sends a message to the interactive session and gets a response.
func (s *InteractiveSession) Chat(ctx context.Context, message string) (*InteractiveResponse, error) {
	s.conversation = append(s.conversation, llm.UserMessage(message))

	llmReq := llm.CompletionRequest{
		Messages:    s.conversation,
		Model:       s.copilot.model,
		Temperature: 0.3,
		MaxTokens:   2048,
	}

	rawJSON, err := s.copilot.client.CompleteStructured(ctx, llmReq, "interactive_response", interactiveSchema)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	var resp InteractiveResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Update current YAML if provided
	if resp.WorkflowYAML != "" {
		s.currentYAML = resp.WorkflowYAML
	}

	// Add assistant response to conversation
	s.conversation = append(s.conversation, llm.AssistantMessage(resp.Message))

	return &resp, nil
}

// GetCurrentYAML returns the current workflow YAML.
func (s *InteractiveSession) GetCurrentYAML() string {
	return s.currentYAML
}

// InteractiveResponse is the response from an interactive chat.
type InteractiveResponse struct {
	Message      string   `json:"message"`
	WorkflowYAML string   `json:"workflow_yaml,omitempty"`
	Questions    []string `json:"questions,omitempty"`
	IsComplete   bool     `json:"is_complete"`
}

// workflowGenResponse is the LLM response structure.
type workflowGenResponse struct {
	YAML        string   `json:"yaml"`
	Explanation string   `json:"explanation"`
	Warnings    []string `json:"warnings,omitempty"`
}

var workflowGenSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"yaml": map[string]interface{}{
			"type":        "string",
			"description": "The generated workflow YAML",
		},
		"explanation": map[string]interface{}{
			"type":        "string",
			"description": "Explanation of what the workflow does",
		},
		"warnings": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Warnings about the generated workflow",
		},
	},
	"required": []string{"yaml", "explanation"},
}

var interactiveSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Response message to the user",
		},
		"workflow_yaml": map[string]interface{}{
			"type":        "string",
			"description": "Updated workflow YAML if changes were made",
		},
		"questions": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
			"description": "Clarifying questions for the user",
		},
		"is_complete": map[string]interface{}{
			"type":        "boolean",
			"description": "Whether the workflow is complete",
		},
	},
	"required": []string{"message", "is_complete"},
}

func buildWorkflowSystemPrompt(eventTypes, actionTypes []string) string {
	var sb strings.Builder
	sb.WriteString(`You are a healthcare workflow automation expert. Your task is to generate fi-fhir workflow YAML configurations from natural language descriptions.

## Workflow YAML Format

A workflow has:
- name: Unique workflow name
- version: Semantic version
- routes: Array of event processing routes

Each route has:
- name: Route name
- filter: Event matching criteria
  - event_type: Event type(s) to match (string or array)
  - source: Source system(s) to match (string or array)
  - condition: CEL expression for complex filtering
- transform: Optional array of transformations
  - set_field: Set a field value
  - map_terminology: Map codes between systems
  - redact: Remove sensitive fields
- actions: Array of actions to execute

## Available Event Types
`)

	if len(eventTypes) > 0 {
		for _, et := range eventTypes {
			sb.WriteString("- " + et + "\n")
		}
	} else {
		sb.WriteString(`- ADT (Admit/Discharge/Transfer)
- ORU (Observation Result - Lab)
- ORM (Order Message)
- MDM (Medical Document)
- DFT (Financial Transaction)
- SIU (Scheduling)
- RDE (Pharmacy)
`)
	}

	sb.WriteString("\n## Available Action Types\n")
	if len(actionTypes) > 0 {
		for _, at := range actionTypes {
			sb.WriteString("- " + at + "\n")
		}
	} else {
		sb.WriteString(`- webhook: Send HTTP POST to URL
  - url: Target URL
  - headers: Optional HTTP headers
- fhir: Convert to FHIR and send
  - server: FHIR server URL
  - resource_type: Target resource type
- log: Log the event
  - level: info, warn, error
  - message: Log message template
- email: Send email notification
  - to: Recipient address
  - subject: Email subject
  - body: Email body template
- file: Write to file
  - path: File path
  - format: json, csv, hl7
`)
	}

	sb.WriteString(`
## CEL Expression Examples
- event.event_type == "ADT"
- event.patient.age >= 65
- event.observation.interpretation == "critical"
- event.patient.mrn != "" && event.patient.ssn != ""

## Guidelines
1. Always include workflow name and version
2. Use descriptive route names
3. Include appropriate filters for the use case
4. Add comments in YAML for clarity
5. Warn about potential issues or missing information
`)

	return sb.String()
}

func buildWorkflowUserPrompt(req GenerateRequest) string {
	var sb strings.Builder
	sb.WriteString("Generate a workflow for the following requirement:\n\n")
	sb.WriteString(req.Description)

	if len(req.Examples) > 0 {
		sb.WriteString("\n\n## Examples for Reference\n")
		for _, ex := range req.Examples {
			sb.WriteString(fmt.Sprintf("\nDescription: %s\n```yaml\n%s\n```\n", ex.Description, ex.YAML))
		}
	}

	sb.WriteString("\n\nGenerate valid YAML with explanation.")
	return sb.String()
}

const interactiveSystemPrompt = `You are a healthcare workflow automation assistant. Help users build fi-fhir workflow configurations through interactive conversation.

Your capabilities:
1. Ask clarifying questions about requirements
2. Generate and refine workflow YAML incrementally
3. Explain what the workflow does
4. Suggest improvements and best practices

Workflow YAML Format:
- name, version, routes
- routes have: name, filter (event_type, source, condition), transform, actions
- actions have: type and type-specific config

Available action types: webhook, fhir, log, email, file

Guide the user step by step:
1. Understand what events they want to process
2. Determine what actions to take
3. Ask about any conditions or filters
4. Generate the YAML incrementally
5. Refine based on feedback`
