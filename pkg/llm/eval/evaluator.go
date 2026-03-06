// Package eval provides an evaluation framework for LLM prompt and model quality.
//
// It supports golden test cases, multiple scoring strategies, and comparison
// between prompt/model combinations for regression detection.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// EvalCase is a golden test case with input and expected output.
type EvalCase struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	TaskType    string          `json:"task_type"`     // "ranking", "extraction", etc.
	System      string          `json:"system_prompt"` // System prompt text
	Input       string          `json:"input"`         // User prompt text
	Expected    json.RawMessage `json:"expected"`      // Expected output (JSON)
	Tags        []string        `json:"tags,omitempty"`
}

// EvalResult captures the outcome of evaluating a single test case.
type EvalResult struct {
	CaseID        string          `json:"case_id"`
	Passed        bool            `json:"passed"`
	Score         float64         `json:"score"` // 0.0-1.0
	Actual        json.RawMessage `json:"actual,omitempty"`
	Error         string          `json:"error,omitempty"`
	Latency       time.Duration   `json:"latency_ms"`
	TokensUsed    int             `json:"tokens_used"`
	Model         string          `json:"model"`
	PromptVersion string          `json:"prompt_version,omitempty"`
}

// EvalSummary aggregates results across all test cases.
type EvalSummary struct {
	TotalCases  int                       `json:"total_cases"`
	PassCount   int                       `json:"pass_count"`
	FailCount   int                       `json:"fail_count"`
	ErrorCount  int                       `json:"error_count"`
	MeanScore   float64                   `json:"mean_score"`
	MeanLatency time.Duration             `json:"mean_latency_ms"`
	TotalTokens int                       `json:"total_tokens"`
	PassRate    float64                   `json:"pass_rate"`
	Results     []EvalResult              `json:"results"`
	ByTaskType  map[string]*TaskTypeStats `json:"by_task_type"`
}

// TaskTypeStats tracks evaluation metrics for a specific task type.
type TaskTypeStats struct {
	Cases      int     `json:"cases"`
	PassCount  int     `json:"pass_count"`
	MeanScore  float64 `json:"mean_score"`
	totalScore float64
}

// EvalConfig configures the evaluation run.
type EvalConfig struct {
	// PassThreshold is the minimum score for a case to be considered passing.
	PassThreshold float64 `json:"pass_threshold"`

	// Model to use for the evaluation run. If empty, uses client default.
	Model string `json:"model,omitempty"`

	// PromptVersion tag to record in results.
	PromptVersion string `json:"prompt_version,omitempty"`

	// Scorer determines how expected vs actual outputs are compared.
	Scorer Scorer `json:"-"`
}

// DefaultEvalConfig returns sensible defaults.
func DefaultEvalConfig() EvalConfig {
	return EvalConfig{
		PassThreshold: 0.7,
		Scorer:        &JSONMatchScorer{},
	}
}

// Evaluator runs prompt+model combinations against golden test cases.
type Evaluator struct {
	client llm.Client
	config EvalConfig
}

// NewEvaluator creates a new evaluator.
func NewEvaluator(client llm.Client, cfg EvalConfig) *Evaluator {
	if cfg.Scorer == nil {
		cfg.Scorer = &JSONMatchScorer{}
	}
	return &Evaluator{
		client: client,
		config: cfg,
	}
}

// RunCase evaluates a single test case and returns the result.
func (e *Evaluator) RunCase(ctx context.Context, tc EvalCase) EvalResult {
	start := time.Now()

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(tc.System),
			llm.UserMessage(tc.Input),
		},
		Model: e.config.Model,
	}

	jsonResp, err := e.client.CompleteJSON(ctx, req)
	latency := time.Since(start)

	if err != nil {
		return EvalResult{
			CaseID:        tc.ID,
			Passed:        false,
			Score:         0,
			Error:         err.Error(),
			Latency:       latency,
			Model:         e.config.Model,
			PromptVersion: e.config.PromptVersion,
		}
	}

	score := e.config.Scorer.Score(tc.Expected, jsonResp)

	return EvalResult{
		CaseID:        tc.ID,
		Passed:        score >= e.config.PassThreshold,
		Score:         score,
		Actual:        jsonResp,
		Latency:       latency,
		Model:         e.config.Model,
		PromptVersion: e.config.PromptVersion,
	}
}

// RunAll evaluates all test cases and returns an aggregate summary.
func (e *Evaluator) RunAll(ctx context.Context, cases []EvalCase) *EvalSummary {
	summary := &EvalSummary{
		ByTaskType: make(map[string]*TaskTypeStats),
	}

	var totalLatency time.Duration
	var totalScore float64

	for _, tc := range cases {
		select {
		case <-ctx.Done():
			summary.Results = append(summary.Results, EvalResult{
				CaseID: tc.ID,
				Error:  ctx.Err().Error(),
			})
			summary.ErrorCount++
			continue
		default:
		}

		result := e.RunCase(ctx, tc)
		summary.Results = append(summary.Results, result)
		summary.TotalCases++
		summary.TotalTokens += result.TokensUsed
		totalLatency += result.Latency

		if result.Error != "" {
			summary.ErrorCount++
		} else if result.Passed {
			summary.PassCount++
		} else {
			summary.FailCount++
		}

		totalScore += result.Score

		// Track by task type
		ts, ok := summary.ByTaskType[tc.TaskType]
		if !ok {
			ts = &TaskTypeStats{}
			summary.ByTaskType[tc.TaskType] = ts
		}
		ts.Cases++
		ts.totalScore += result.Score
		if result.Passed {
			ts.PassCount++
		}
	}

	if summary.TotalCases > 0 {
		summary.MeanScore = totalScore / float64(summary.TotalCases)
		summary.MeanLatency = totalLatency / time.Duration(summary.TotalCases)
		summary.PassRate = float64(summary.PassCount) / float64(summary.TotalCases)
	}

	// Compute per-task mean scores
	for _, ts := range summary.ByTaskType {
		if ts.Cases > 0 {
			ts.MeanScore = ts.totalScore / float64(ts.Cases)
		}
	}

	return summary
}

// CompareResult holds the comparison between two evaluation runs.
type CompareResult struct {
	BaseLabel  string     `json:"base_label"`
	ChalLabel  string     `json:"challenge_label"`
	BaseScore  float64    `json:"base_score"`
	ChalScore  float64    `json:"challenge_score"`
	ScoreDelta float64    `json:"score_delta"`
	Improved   int        `json:"improved"`
	Regressed  int        `json:"regressed"`
	Unchanged  int        `json:"unchanged"`
	PerCase    []CaseDiff `json:"per_case"`
}

// CaseDiff shows the score delta for a single case between two runs.
type CaseDiff struct {
	CaseID    string  `json:"case_id"`
	BaseScore float64 `json:"base_score"`
	ChalScore float64 `json:"challenge_score"`
	Delta     float64 `json:"delta"`
}

// Compare two evaluation summaries and produce a comparison report.
func Compare(baseLabel string, base *EvalSummary, chalLabel string, chal *EvalSummary) *CompareResult {
	cr := &CompareResult{
		BaseLabel:  baseLabel,
		ChalLabel:  chalLabel,
		BaseScore:  base.MeanScore,
		ChalScore:  chal.MeanScore,
		ScoreDelta: chal.MeanScore - base.MeanScore,
	}

	// Build map of chal results by case ID
	chalMap := make(map[string]EvalResult, len(chal.Results))
	for _, r := range chal.Results {
		chalMap[r.CaseID] = r
	}

	for _, baseR := range base.Results {
		chalR, ok := chalMap[baseR.CaseID]
		if !ok {
			continue
		}

		diff := CaseDiff{
			CaseID:    baseR.CaseID,
			BaseScore: baseR.Score,
			ChalScore: chalR.Score,
			Delta:     chalR.Score - baseR.Score,
		}
		cr.PerCase = append(cr.PerCase, diff)

		switch {
		case diff.Delta > 0.01:
			cr.Improved++
		case diff.Delta < -0.01:
			cr.Regressed++
		default:
			cr.Unchanged++
		}
	}

	return cr
}

// FormatSummary returns a human-readable summary.
func (s *EvalSummary) FormatSummary() string {
	return fmt.Sprintf(
		"Eval: %d cases | %.0f%% pass rate | mean score: %.2f | mean latency: %v | %d tokens",
		s.TotalCases,
		s.PassRate*100,
		s.MeanScore,
		s.MeanLatency.Round(time.Millisecond),
		s.TotalTokens,
	)
}

// FormatComparison returns a human-readable comparison report.
func (cr *CompareResult) FormatComparison() string {
	direction := "→"
	if cr.ScoreDelta > 0 {
		direction = "↑"
	} else if cr.ScoreDelta < 0 {
		direction = "↓"
	}

	return fmt.Sprintf(
		"%s vs %s: %.2f %s %.2f (%+.2f) | ↑%d ↓%d =%d",
		cr.BaseLabel, cr.ChalLabel,
		cr.BaseScore, direction, cr.ChalScore,
		cr.ScoreDelta,
		cr.Improved, cr.Regressed, cr.Unchanged,
	)
}

// Suppress unused import warning.
var _ = math.Abs
