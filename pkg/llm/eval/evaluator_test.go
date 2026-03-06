package eval

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

func TestEvaluator_RunCase_Success(t *testing.T) {
	expected := json.RawMessage(`{"best_match":{"code":"E11.9","confidence":0.95}}`)
	mock := llm.NewMockClient()
	mock.CompleteJSONFunc = func(ctx context.Context, req llm.CompletionRequest) (json.RawMessage, error) {
		return expected, nil
	}

	cfg := DefaultEvalConfig()
	cfg.PassThreshold = 0.5
	evaluator := NewEvaluator(mock, cfg)

	tc := EvalCase{
		ID:       "test-1",
		TaskType: "ranking",
		System:   "You are a medical coder.",
		Input:    "Map diabetes to ICD-10",
		Expected: expected,
	}

	result := evaluator.RunCase(context.Background(), tc)
	if !result.Passed {
		t.Errorf("expected pass, got fail with score %.2f", result.Score)
	}
	if result.Score < 0.99 {
		t.Errorf("expected high score for identical output, got %.2f", result.Score)
	}
}

func TestEvaluator_RunCase_Error(t *testing.T) {
	mock := llm.NewMockClient().WithError(errors.New("model offline"))

	evaluator := NewEvaluator(mock, DefaultEvalConfig())

	tc := EvalCase{
		ID:       "test-err",
		TaskType: "ranking",
		System:   "system",
		Input:    "input",
		Expected: json.RawMessage(`{}`),
	}

	result := evaluator.RunCase(context.Background(), tc)
	if result.Passed {
		t.Error("expected fail when model errors")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestEvaluator_RunAll(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteJSONFunc = func(ctx context.Context, req llm.CompletionRequest) (json.RawMessage, error) {
		return json.RawMessage(`{"code":"E11.9","confidence":0.9}`), nil
	}

	cfg := DefaultEvalConfig()
	cfg.PassThreshold = 0.5
	evaluator := NewEvaluator(mock, cfg)

	cases := []EvalCase{
		{ID: "1", TaskType: "ranking", System: "s", Input: "i", Expected: json.RawMessage(`{"code":"E11.9","confidence":0.9}`)},
		{ID: "2", TaskType: "ranking", System: "s", Input: "i", Expected: json.RawMessage(`{"code":"E11.9","confidence":0.9}`)},
		{ID: "3", TaskType: "extraction", System: "s", Input: "i", Expected: json.RawMessage(`{"code":"J45.0","confidence":0.8}`)},
	}

	summary := evaluator.RunAll(context.Background(), cases)
	if summary.TotalCases != 3 {
		t.Errorf("expected 3 total cases, got %d", summary.TotalCases)
	}
	if summary.PassCount < 2 {
		t.Errorf("expected at least 2 passes, got %d", summary.PassCount)
	}

	rankingStats, ok := summary.ByTaskType["ranking"]
	if !ok {
		t.Fatal("expected ranking task type stats")
	}
	if rankingStats.Cases != 2 {
		t.Errorf("expected 2 ranking cases, got %d", rankingStats.Cases)
	}
}

func TestEvaluator_RunAll_ContextCancelled(t *testing.T) {
	mock := llm.NewMockClient()
	evaluator := NewEvaluator(mock, DefaultEvalConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cases := []EvalCase{
		{ID: "1", TaskType: "ranking", System: "s", Input: "i", Expected: json.RawMessage(`{}`)},
	}

	summary := evaluator.RunAll(ctx, cases)
	if summary.ErrorCount != 1 {
		t.Errorf("expected 1 error from cancelled context, got %d", summary.ErrorCount)
	}
}

func TestCompare(t *testing.T) {
	base := &EvalSummary{
		MeanScore: 0.8,
		Results: []EvalResult{
			{CaseID: "1", Score: 0.9},
			{CaseID: "2", Score: 0.7},
			{CaseID: "3", Score: 0.8},
		},
	}
	chal := &EvalSummary{
		MeanScore: 0.85,
		Results: []EvalResult{
			{CaseID: "1", Score: 0.95},
			{CaseID: "2", Score: 0.75},
			{CaseID: "3", Score: 0.8},
		},
	}

	cr := Compare("v1", base, "v2", chal)
	if cr.Improved != 2 {
		t.Errorf("expected 2 improved, got %d", cr.Improved)
	}
	if cr.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", cr.Unchanged)
	}
	if cr.ScoreDelta < 0 {
		t.Error("expected positive score delta")
	}
}

func TestFormatSummary(t *testing.T) {
	s := &EvalSummary{
		TotalCases: 10,
		PassRate:   0.8,
		MeanScore:  0.85,
	}
	result := s.FormatSummary()
	if result == "" {
		t.Error("expected non-empty format")
	}
}

func TestFormatComparison(t *testing.T) {
	cr := &CompareResult{
		BaseLabel:  "v1",
		ChalLabel:  "v2",
		BaseScore:  0.8,
		ChalScore:  0.85,
		ScoreDelta: 0.05,
		Improved:   3,
		Regressed:  1,
		Unchanged:  2,
	}
	result := cr.FormatComparison()
	if result == "" {
		t.Error("expected non-empty comparison format")
	}
}
