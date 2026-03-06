package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRouter_TaskRouting(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()

	router := NewRouter(fast, quality, DefaultRouterConfig())

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	// Ranking should go to fast tier by default
	_, err := router.Complete(ctx, TaskRanking, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fast.CallCount() != 1 {
		t.Errorf("expected fast to receive 1 call, got %d", fast.CallCount())
	}
	if quality.CallCount() != 0 {
		t.Errorf("expected quality to receive 0 calls, got %d", quality.CallCount())
	}

	fast.Reset()
	quality.Reset()

	// Extraction should go to quality tier
	_, err = router.Complete(ctx, TaskExtraction, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fast.CallCount() != 0 {
		t.Errorf("expected fast to receive 0 calls, got %d", fast.CallCount())
	}
	if quality.CallCount() != 1 {
		t.Errorf("expected quality to receive 1 call, got %d", quality.CallCount())
	}
}

func TestRouter_FallbackOnError(t *testing.T) {
	testErr := errors.New("fast model unavailable")
	fast := NewMockClient().WithError(testErr)
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	// Ranking goes to fast, fails, should fallback to quality
	resp, err := router.Complete(ctx, TaskRanking, req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response from fallback")
	}
	if fast.CallCount() != 1 {
		t.Errorf("expected fast to receive 1 call, got %d", fast.CallCount())
	}
	if quality.CallCount() != 1 {
		t.Errorf("expected quality to receive 1 fallback call, got %d", quality.CallCount())
	}
}

func TestRouter_NoFallbackWhenDisabled(t *testing.T) {
	testErr := errors.New("fast model unavailable")
	fast := NewMockClient().WithError(testErr)
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = false
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	_, err := router.Complete(ctx, TaskRanking, req)
	if err == nil {
		t.Fatal("expected error when fallback is disabled")
	}
	if quality.CallCount() != 0 {
		t.Errorf("expected quality to receive 0 calls, got %d", quality.CallCount())
	}
}

func TestRouter_NoFallbackFromQualityTier(t *testing.T) {
	testErr := errors.New("quality model unavailable")
	fast := NewMockClient()
	quality := NewMockClient().WithError(testErr)

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	// Extraction routes to quality, fails — should NOT fallback to fast
	_, err := router.Complete(ctx, TaskExtraction, req)
	if err == nil {
		t.Fatal("expected error, quality tier should not fallback to fast")
	}
}

func TestRouter_NilQualityClient(t *testing.T) {
	fast := NewMockClient()

	router := NewRouter(fast, nil, DefaultRouterConfig())

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	// Even extraction should go to fast when quality is nil
	_, err := router.Complete(ctx, TaskExtraction, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fast.CallCount() != 1 {
		t.Errorf("expected fast to receive 1 call, got %d", fast.CallCount())
	}
}

func TestRouter_MaxTokensApplied(t *testing.T) {
	fast := NewMockClient()

	cfg := RouterConfig{
		Routes: []RouteRule{
			{TaskType: TaskRanking, Tier: TierFast, MaxTokens: 512},
		},
	}
	router := NewRouter(fast, nil, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")
	// req.MaxTokens is 0, should be overridden by route rule

	_, _ = router.Complete(ctx, TaskRanking, req)

	lastCall := fast.LastCall()
	if lastCall == nil {
		t.Fatal("expected at least one call")
	}
	if lastCall.MaxTokens != 512 {
		t.Errorf("expected MaxTokens=512, got %d", lastCall.MaxTokens)
	}
}

func TestRouter_MaxTokensNotOverridden(t *testing.T) {
	fast := NewMockClient()

	cfg := RouterConfig{
		Routes: []RouteRule{
			{TaskType: TaskRanking, Tier: TierFast, MaxTokens: 512},
		},
	}
	router := NewRouter(fast, nil, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")
	req.MaxTokens = 1024 // Explicitly set

	_, _ = router.Complete(ctx, TaskRanking, req)

	lastCall := fast.LastCall()
	if lastCall == nil {
		t.Fatal("expected at least one call")
	}
	if lastCall.MaxTokens != 1024 {
		t.Errorf("expected MaxTokens=1024 (not overridden), got %d", lastCall.MaxTokens)
	}
}

func TestRouter_CostTracking(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableCostTracking = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	// Make a few requests
	_, _ = router.Complete(ctx, TaskRanking, req)
	_, _ = router.Complete(ctx, TaskRanking, req)
	_, _ = router.Complete(ctx, TaskExtraction, req)

	summary := router.GetCostSummary()
	if summary.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", summary.TotalRequests)
	}

	rankingCost, ok := summary.ByTaskType[TaskRanking]
	if !ok {
		t.Fatal("expected ranking task type in summary")
	}
	if rankingCost.Requests != 2 {
		t.Errorf("expected 2 ranking requests, got %d", rankingCost.Requests)
	}

	extractionCost, ok := summary.ByTaskType[TaskExtraction]
	if !ok {
		t.Fatal("expected extraction task type in summary")
	}
	if extractionCost.Requests != 1 {
		t.Errorf("expected 1 extraction request, got %d", extractionCost.Requests)
	}
}

func TestRouter_CostTrackingDisabled(t *testing.T) {
	fast := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableCostTracking = false
	router := NewRouter(fast, nil, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	_, _ = router.Complete(ctx, TaskRanking, req)

	summary := router.GetCostSummary()
	if summary.TotalRequests != 0 {
		t.Errorf("expected 0 tracked requests when disabled, got %d", summary.TotalRequests)
	}
}

func TestRouter_FallbackCostTracking(t *testing.T) {
	testErr := errors.New("fast model unavailable")
	fast := NewMockClient().WithError(testErr)
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = true
	cfg.EnableCostTracking = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	_, _ = router.Complete(ctx, TaskRanking, req)

	summary := router.GetCostSummary()
	if summary.FallbackCount != 1 {
		t.Errorf("expected 1 fallback, got %d", summary.FallbackCount)
	}
}

func TestRouter_ResetCosts(t *testing.T) {
	fast := NewMockClient()
	router := NewRouter(fast, nil, DefaultRouterConfig())

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	_, _ = router.Complete(ctx, TaskRanking, req)
	router.ResetCosts()

	summary := router.GetCostSummary()
	if summary.TotalRequests != 0 {
		t.Errorf("expected 0 after reset, got %d", summary.TotalRequests)
	}
}

func TestRouter_CompleteJSON(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()

	router := NewRouter(fast, quality, DefaultRouterConfig())

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	result, err := router.CompleteJSON(ctx, TaskRanking, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if fast.CallCount() != 1 {
		t.Errorf("expected fast to receive 1 call, got %d", fast.CallCount())
	}
}

func TestRouter_CompleteStructured(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()

	router := NewRouter(fast, quality, DefaultRouterConfig())

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")
	schema := map[string]interface{}{"type": "object"}

	result, err := router.CompleteStructured(ctx, TaskRanking, req, "test_schema", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRouter_CompleteJSON_Fallback(t *testing.T) {
	testErr := errors.New("fast model unavailable")
	fast := NewMockClient().WithError(testErr)
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	result, err := router.CompleteJSON(ctx, TaskRanking, req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from fallback")
	}
}

func TestRouter_CompleteStructured_Fallback(t *testing.T) {
	testErr := errors.New("fast model unavailable")
	fast := NewMockClient().WithError(testErr)
	quality := NewMockClient()

	cfg := DefaultRouterConfig()
	cfg.EnableFallback = true
	router := NewRouter(fast, quality, cfg)

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")
	schema := map[string]interface{}{"type": "object"}

	result, err := router.CompleteStructured(ctx, TaskRanking, req, "test_schema", schema)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from fallback")
	}
}

func TestRouter_UnknownTaskDefaultsToFast(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()

	router := NewRouter(fast, quality, RouterConfig{
		Routes: []RouteRule{}, // No rules
	})

	ctx := context.Background()
	req := NewCompletionRequest("system", "hello")

	_, err := router.Complete(ctx, "unknown_task", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fast.CallCount() != 1 {
		t.Errorf("expected fast to receive 1 call for unknown task, got %d", fast.CallCount())
	}
}

func TestRouter_ClientForTask(t *testing.T) {
	fast := NewMockClient()
	quality := NewMockClient()
	router := NewRouter(fast, quality, DefaultRouterConfig())

	if router.ClientForTask(TaskRanking) != fast {
		t.Error("expected fast client for ranking task")
	}
	if router.ClientForTask(TaskExtraction) != quality {
		t.Error("expected quality client for extraction task")
	}
}

func TestModelTier_String(t *testing.T) {
	if TierFast.String() != "fast" {
		t.Errorf("expected 'fast', got '%s'", TierFast)
	}
	if TierQuality.String() != "quality" {
		t.Errorf("expected 'quality', got '%s'", TierQuality)
	}
}

func TestTaskType_String(t *testing.T) {
	if TaskRanking.String() != "ranking" {
		t.Errorf("expected 'ranking', got '%s'", TaskRanking)
	}
}

// Suppress unused import warning
var _ = json.RawMessage{}
