//nolint:errcheck,gosec // Test file - error checking intentionally relaxed
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/explain"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

// ---------------------------------------------------------------------------
// Helpers for overriding factories in tests
// ---------------------------------------------------------------------------

// withMockLLM overrides llmClientFactory for the duration of a test function.
// Returns a restore function.
func withMockLLM(mock *llm.MockClient) func() {
	orig := llmClientFactory
	llmClientFactory = func() (llm.Client, error) {
		return mock, nil
	}
	return func() { llmClientFactory = orig }
}

// withLLMError overrides llmClientFactory to return an error.
func withLLMError(err error) func() {
	orig := llmClientFactory
	llmClientFactory = func() (llm.Client, error) {
		return nil, err
	}
	return func() { llmClientFactory = orig }
}

// lineReader delivers one line per Read call, then EOF.
type lineReader struct {
	lines []string
	idx   int
}

func (r *lineReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.lines) {
		return 0, io.EOF
	}
	line := r.lines[r.idx] + "\n"
	r.idx++
	n := copy(p, line)
	return n, nil
}

// withStdin overrides stdinReader for interactive session tests.
func withStdin(lines ...string) func() {
	orig := stdinReader
	stdinReader = &lineReader{lines: lines}
	return func() { stdinReader = orig }
}

// stubSearcher is a mock terminologySearcher.
type stubSearcher struct {
	results []semantic.SemanticMatch
	err     error
}

func (s *stubSearcher) Search(_ context.Context, _ string, _ semantic.SearchOptions) ([]semantic.SemanticMatch, error) {
	return s.results, s.err
}

// withMockSearcher overrides terminologySearchFactory for tests.
func withMockSearcher(results []semantic.SemanticMatch, err error) func() {
	orig := terminologySearchFactory
	terminologySearchFactory = func(cfg semantic.SearchConfig) (terminologySearcher, error) {
		return &stubSearcher{results: results, err: err}, nil
	}
	return func() { terminologySearchFactory = orig }
}

// withSearcherFactoryError makes the factory itself return an error.
func withSearcherFactoryError(err error) func() {
	orig := terminologySearchFactory
	terminologySearchFactory = func(cfg semantic.SearchConfig) (terminologySearcher, error) {
		return nil, err
	}
	return func() { terminologySearchFactory = orig }
}

// makeExplainJSON builds a valid WorkflowExplanation JSON response.
func makeExplainJSON(summary, description, diagram string, routes []explain.RouteExplanation, warnings []string) json.RawMessage {
	resp := explain.WorkflowExplanation{
		Summary:           summary,
		Description:       description,
		RouteExplanations: routes,
		Diagram:           diagram,
		Warnings:          warnings,
	}
	b, _ := json.Marshal(resp)
	return b
}

// makeCELGenJSON builds a valid CEL generation JSON response for CLI tests.
func makeCLICELGenJSON(expression, explanation string, alternatives, fieldsUsed []string) json.RawMessage {
	resp := map[string]interface{}{
		"expression":  expression,
		"explanation": explanation,
	}
	if alternatives != nil {
		resp["alternatives"] = alternatives
	}
	if fieldsUsed != nil {
		resp["fields_used"] = fieldsUsed
	}
	b, _ := json.Marshal(resp)
	return b
}

// makeCLIValidationJSON builds a valid CEL validation JSON response.
func makeCLIValidationJSON(valid bool, errMsg, corrected string, suggestions []string) json.RawMessage {
	resp := map[string]interface{}{
		"valid": valid,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	if corrected != "" {
		resp["corrected"] = corrected
	}
	if suggestions != nil {
		resp["suggestions"] = suggestions
	}
	b, _ := json.Marshal(resp)
	return b
}

// ---------------------------------------------------------------------------
// runWorkflowGenerate — deep tests (beyond flag parsing)
// ---------------------------------------------------------------------------

func TestRunWorkflowGenerate_LLMClientError(t *testing.T) {
	restore := withLLMError(fmt.Errorf("no API key"))
	defer restore()

	err := runWorkflowGenerate([]string{"generate a workflow"})
	assertError(t, err)
	assertErrorContains(t, err, "create LLM client")
}

func TestRunWorkflowGenerate_TextOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		resp := map[string]interface{}{
			"yaml":        "name: test\nversion: 1.0.0",
			"explanation": "Routes ADT events to webhook",
			"warnings":    []string{"missing retry config"},
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"Route ADT events to webhook"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Generated Workflow")
	assertContains(t, stdout, "Routes ADT events to webhook")
	assertContains(t, stdout, "name: test")
	assertContains(t, stdout, "Warnings")
	assertContains(t, stdout, "missing retry config")
}

func TestRunWorkflowGenerate_JSONOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		resp := map[string]interface{}{
			"yaml":        "name: test\nversion: 1.0.0",
			"explanation": "Routes events",
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--json", "generate events workflow"})
		assertNoError(t, err)
	})

	// JSON output should be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("expected valid JSON output, got parse error: %v\nOutput: %s", err, stdout)
	}
}

func TestRunWorkflowGenerate_LLMGenerateError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("rate limited"))
	restore := withMockLLM(mock)
	defer restore()

	err := runWorkflowGenerate([]string{"generate something"})
	assertError(t, err)
	assertErrorContains(t, err, "generate workflow")
}

func TestRunWorkflowGenerate_NoWarnings(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		resp := map[string]interface{}{
			"yaml":        "name: clean\nversion: 1.0.0",
			"explanation": "Clean workflow",
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"simple workflow"})
		assertNoError(t, err)
	})

	// Should not have Warnings section
	if strings.Contains(stdout, "Warnings") {
		t.Error("output should not contain Warnings section when no warnings")
	}
}

// ---------------------------------------------------------------------------
// runInteractiveWorkflowSession — tests with injected stdin
// ---------------------------------------------------------------------------

func TestRunWorkflowGenerate_InteractiveQuit(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		// Should not be called - user quits immediately
		resp := map[string]interface{}{
			"yaml":        "name: test\nversion: 1.0.0",
			"explanation": "test",
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Interactive workflow builder started")
}

func TestRunWorkflowGenerate_InteractiveExit(t *testing.T) {
	mock := llm.NewMockClient()
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("exit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"-i"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Interactive workflow builder started")
}

func TestRunWorkflowGenerate_InteractiveShowEmpty(t *testing.T) {
	mock := llm.NewMockClient()
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("show", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "No workflow generated yet")
}

func TestRunWorkflowGenerate_InteractiveYAMLCommand(t *testing.T) {
	mock := llm.NewMockClient()
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("yaml", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	// "yaml" command is alias for "show"
	assertContains(t, stdout, "No workflow generated yet")
}

func TestRunWorkflowGenerate_InteractiveChat(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		resp := map[string]interface{}{
			"message":       "I'll create a workflow for ADT events.",
			"workflow_yaml": "name: adt-router\nversion: 1.0.0",
			"is_complete":   false,
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("create an ADT workflow", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "I'll create a workflow for ADT events.")
	assertContains(t, stdout, "Current workflow:")
	assertContains(t, stdout, "name: adt-router")
}

func TestRunWorkflowGenerate_InteractiveChatComplete(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		resp := map[string]interface{}{
			"message":     "Workflow is done!",
			"is_complete": true,
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("finish", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Workflow is complete!")
}

func TestRunWorkflowGenerate_InteractiveChatError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("timeout"))
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("hello", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err) // errors in chat are printed, not returned
	})

	assertContains(t, stdout, "Error:")
}

func TestRunWorkflowGenerate_InteractiveShowAfterGenerate(t *testing.T) {
	callCount := 0
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		callCount++
		resp := map[string]interface{}{
			"message":       "Generated workflow",
			"workflow_yaml": "name: test-workflow\nversion: 1.0.0",
			"is_complete":   false,
		}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	restoreLLM := withMockLLM(mock)
	defer restoreLLM()
	restoreStdin := withStdin("create workflow", "show", "quit")
	defer restoreStdin()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--interactive"})
		assertNoError(t, err)
	})

	// "show" should display the YAML from the previous chat
	// Count occurrences of the YAML — once from chat, once from show
	count := strings.Count(stdout, "name: test-workflow")
	if count < 2 {
		t.Errorf("expected YAML to appear at least 2 times (chat + show), got %d\nOutput: %s", count, stdout)
	}
}

// ---------------------------------------------------------------------------
// runWorkflowExplain — deep tests
// ---------------------------------------------------------------------------

func TestRunWorkflowExplain_LLMClientError(t *testing.T) {
	restore := withLLMError(fmt.Errorf("no credentials"))
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: test\nversion: 1.0.0")
	err := runWorkflowExplain([]string{tmpFile})
	assertError(t, err)
	assertErrorContains(t, err, "create LLM client")
}

func TestRunWorkflowExplain_TextOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeExplainJSON(
			"Routes ADT events",
			"This workflow processes ADT messages and sends them to FHIR.",
			"",
			[]explain.RouteExplanation{
				{Name: "adt-route", Trigger: "ADT event", Actions: []string{"send to FHIR"}, Description: "Handles admits"},
			},
			[]string{"no retry config"},
		), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: adt\nversion: 1.0.0\nroutes: []")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{tmpFile})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Workflow Explanation")
	assertContains(t, stdout, "Routes ADT events")
	assertContains(t, stdout, "This workflow processes ADT messages")
	assertContains(t, stdout, "adt-route")
	assertContains(t, stdout, "ADT event")
	assertContains(t, stdout, "send to FHIR")
	assertContains(t, stdout, "no retry config")
}

func TestRunWorkflowExplain_JSONOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeExplainJSON("Summary", "Description", "", nil, nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: test\nversion: 1.0.0")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"--json", tmpFile})
		assertNoError(t, err)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("expected valid JSON output, got parse error: %v", err)
	}
}

func TestRunWorkflowExplain_WithAudience(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeExplainJSON("Business summary", "Business description", "", nil, nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: test\nversion: 1.0.0")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"--audience", "business", tmpFile})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Business summary")
}

func TestRunWorkflowExplain_WithDiagram(t *testing.T) {
	callCount := 0
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		callCount++
		// First call is Explain, return without diagram
		return makeExplainJSON("Summary", "Desc", "", nil, nil), nil
	}
	// GenerateDiagram uses Complete, not CompleteStructured
	mock.CompleteFunc = func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
		return &llm.CompletionResponse{
			Choices: []llm.Choice{
				{Message: llm.Message{Role: "assistant", Content: "graph TD\n  A-->B"}},
			},
		}, nil
	}
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: test\nversion: 1.0.0")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"--diagram", tmpFile})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Diagram")
	assertContains(t, stdout, "mermaid")
}

func TestRunWorkflowExplain_LLMExplainError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("service unavailable"))
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: test\nversion: 1.0.0")

	err := runWorkflowExplain([]string{tmpFile})
	assertError(t, err)
	assertErrorContains(t, err, "explain workflow")
}

func TestRunWorkflowExplain_NoRoutes(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeExplainJSON("Empty workflow", "No routes defined", "", nil, nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	tmpFile := createTempFile(t, t.TempDir(), "workflow-*.yaml", "name: empty\nversion: 1.0.0")

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{tmpFile})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Empty workflow")
	if strings.Contains(stdout, "## Routes") {
		t.Error("should not have Routes section when no routes")
	}
}

// ---------------------------------------------------------------------------
// runWorkflowCEL — deep tests
// ---------------------------------------------------------------------------

func TestRunWorkflowCEL_LLMClientError(t *testing.T) {
	restore := withLLMError(fmt.Errorf("no API key"))
	defer restore()

	err := runWorkflowCEL([]string{"patients over 65"})
	assertError(t, err)
	assertErrorContains(t, err, "create LLM client")
}

func TestRunWorkflowCEL_GenerateTextOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLICELGenJSON(
			"event.patient.age >= 65",
			"Matches elderly patients",
			[]string{"event.patient.age > 64"},
			[]string{"event.patient.age"},
		), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"patients over 65"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Expression: event.patient.age >= 65")
	assertContains(t, stdout, "Matches elderly patients")
	assertContains(t, stdout, "Fields used")
	assertContains(t, stdout, "event.patient.age")
	assertContains(t, stdout, "Alternatives")
	assertContains(t, stdout, "event.patient.age > 64")
}

func TestRunWorkflowCEL_GenerateJSONOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLICELGenJSON("event.patient.age >= 65", "Elderly patients", nil, nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--json", "patients over 65"})
		assertNoError(t, err)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("expected valid JSON output, got parse error: %v", err)
	}
}

func TestRunWorkflowCEL_GenerateLLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("rate limited"))
	restore := withMockLLM(mock)
	defer restore()

	err := runWorkflowCEL([]string{"some condition"})
	assertError(t, err)
	assertErrorContains(t, err, "generate CEL")
}

func TestRunWorkflowCEL_ValidateValid(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLIValidationJSON(true, "", "", nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--validate", "event.patient.age >= 65"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Expression is valid")
}

func TestRunWorkflowCEL_ValidateInvalid(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLIValidationJSON(
			false,
			"missing quotes",
			`event.event_type == "ADT"`,
			[]string{"Always quote strings"},
		), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--validate", "event.event_type == ADT"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Expression is invalid: missing quotes")
	assertContains(t, stdout, "Suggestions")
	assertContains(t, stdout, "Always quote strings")
	assertContains(t, stdout, "Corrected expression")
}

func TestRunWorkflowCEL_ValidateJSONOutput(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLIValidationJSON(true, "", "", nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--json", "--validate", "event.patient.age >= 65"})
		assertNoError(t, err)
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("expected valid JSON output, got parse error: %v", err)
	}
}

func TestRunWorkflowCEL_ValidateLLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("timeout"))
	restore := withMockLLM(mock)
	defer restore()

	err := runWorkflowCEL([]string{"--validate", "event.patient.age >= 65"})
	assertError(t, err)
	assertErrorContains(t, err, "validate CEL")
}

func TestRunWorkflowCEL_GenerateNoFieldsNoAlternatives(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLICELGenJSON("true", "Always true", nil, nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"match everything"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Expression: true")
	if strings.Contains(stdout, "Fields used") {
		t.Error("should not show Fields used when none")
	}
	if strings.Contains(stdout, "Alternatives") {
		t.Error("should not show Alternatives when none")
	}
}

func TestRunWorkflowCEL_ValidateNoSuggestionsNoCorrected(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCLIValidationJSON(false, "unknown field", "", nil), nil
	}
	restore := withMockLLM(mock)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--validate", "event.bogus == true"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Expression is invalid: unknown field")
	if strings.Contains(stdout, "Suggestions") {
		t.Error("should not show Suggestions when none")
	}
	if strings.Contains(stdout, "Corrected") {
		t.Error("should not show Corrected when empty")
	}
}

// ---------------------------------------------------------------------------
// runTerminologySearch — deep tests
// ---------------------------------------------------------------------------

func TestRunTerminologySearch_SearcherFactoryError(t *testing.T) {
	restore := withSearcherFactoryError(fmt.Errorf("invalid config"))
	defer restore()

	err := runTerminologySearch([]string{"blood glucose"})
	assertError(t, err)
	assertErrorContains(t, err, "create searcher")
}

func TestRunTerminologySearch_TextOutput(t *testing.T) {
	results := []semantic.SemanticMatch{
		{
			Code:       "2345-7",
			Display:    "Glucose [Mass/volume] in Serum or Plasma",
			System:     "http://loinc.org",
			Vocabulary: index.VocabularyLOINC,
			Score:      0.95,
		},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"blood glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Found 1 results")
	assertContains(t, stdout, "blood glucose")
	assertContains(t, stdout, "2345-7")
	assertContains(t, stdout, "Glucose [Mass/volume]")
	assertContains(t, stdout, "0.950")
}

func TestRunTerminologySearch_JSONOutput(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose", System: "http://loinc.org", Score: 0.9},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"--json", "blood glucose"})
		assertNoError(t, err)
	})

	var parsed []interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("expected valid JSON array output, got parse error: %v", err)
	}
}

func TestRunTerminologySearch_NoResults(t *testing.T) {
	restore := withMockSearcher(nil, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"xyzzy nonexistent"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "No results found")
}

func TestRunTerminologySearch_SearchError(t *testing.T) {
	restore := withMockSearcher(nil, fmt.Errorf("qdrant connection refused"))
	defer restore()

	err := runTerminologySearch([]string{"blood glucose"})
	assertError(t, err)
	assertErrorContains(t, err, "search")
}

func TestRunTerminologySearch_VocabularyLOINC(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose", Vocabulary: index.VocabularyLOINC, Score: 0.9},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"--vocabulary", "loinc", "glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "2345-7")
}

func TestRunTerminologySearch_VocabularySNOMED(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "33747003", Display: "Glucose", Vocabulary: index.VocabularySNOMED, Score: 0.85},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"-v", "snomed", "glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "33747003")
}

func TestRunTerminologySearch_VocabularyICD10(t *testing.T) {
	for _, vocab := range []string{"icd10", "icd-10", "icd10cm"} {
		t.Run(vocab, func(t *testing.T) {
			results := []semantic.SemanticMatch{
				{Code: "E11.9", Display: "Type 2 diabetes", Vocabulary: index.VocabularyICD10CM, Score: 0.8},
			}
			restore := withMockSearcher(results, nil)
			defer restore()

			stdout, _ := captureOutput(t, func() {
				err := runTerminologySearch([]string{"-v", vocab, "diabetes"})
				assertNoError(t, err)
			})

			assertContains(t, stdout, "E11.9")
		})
	}
}

func TestRunTerminologySearch_VocabularyRxNorm(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "161", Display: "Acetaminophen", Vocabulary: index.VocabularyRxNorm, Score: 0.88},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"-v", "rxnorm", "acetaminophen"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "161")
}

func TestRunTerminologySearch_CustomVocabulary(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "CUSTOM-1", Display: "Custom code", Vocabulary: "custom_vocab", Score: 0.7},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"-v", "custom_vocab", "custom search"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "CUSTOM-1")
}

func TestRunTerminologySearch_WithLimitAndMinScore(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose", Score: 0.95},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"--limit", "5", "--min-score", "0.5", "glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "2345-7")
}

func TestRunTerminologySearch_PositionalQuery(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose", Score: 0.9},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"blood glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "2345-7")
}

func TestRunTerminologySearch_MissingFlagValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"limit missing value", []string{"--limit"}, "--limit requires a value"},
		{"min-score missing value", []string{"--min-score"}, "--min-score requires a value"},
		{"qdrant-url missing value", []string{"--qdrant-url"}, "--qdrant-url requires a value"},
		{"embedding-url missing value", []string{"--embedding-url"}, "--embedding-url requires a value"},
		{"embedding-model missing value", []string{"--embedding-model"}, "--embedding-model requires a value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runTerminologySearch(tt.args)
			assertError(t, err)
			assertErrorContains(t, err, tt.want)
		})
	}
}

func TestRunTerminologySearch_MultipleResults(t *testing.T) {
	results := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose [Mass/volume]", Score: 0.95},
		{Code: "2339-0", Display: "Glucose [Mass/volume] in Blood", Score: 0.90},
		{Code: "14749-6", Display: "Glucose [Moles/volume]", Score: 0.85},
	}
	restore := withMockSearcher(results, nil)
	defer restore()

	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"glucose"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "Found 3 results")
	assertContains(t, stdout, "1.")
	assertContains(t, stdout, "2.")
	assertContains(t, stdout, "3.")
}
