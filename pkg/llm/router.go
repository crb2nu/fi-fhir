package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TaskType identifies the kind of LLM work being performed.
// The router uses this to pick the right model tier.
type TaskType string

const (
	TaskRanking     TaskType = "ranking"
	TaskExtraction  TaskType = "extraction"
	TaskExplanation TaskType = "explanation"
	TaskQuality     TaskType = "quality"
	TaskGeneral     TaskType = "general"
)

// ModelTier selects which model pool to use.
type ModelTier string

const (
	TierFast    ModelTier = "fast"
	TierQuality ModelTier = "quality"
)

// RouteRule maps a task type to a model tier with optional token limits.
type RouteRule struct {
	TaskType  TaskType  `yaml:"task_type" json:"task_type"`
	Tier      ModelTier `yaml:"tier" json:"tier"`
	MaxTokens int       `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
}

// RouterConfig configures the multi-model router.
type RouterConfig struct {
	// Routes maps task types to model tiers.
	// If a task type is not listed, it defaults to the fast tier.
	Routes []RouteRule `yaml:"routes" json:"routes"`

	// EnableFallback enables automatic escalation from fast to quality
	// tier when the fast tier returns an error.
	EnableFallback bool `yaml:"enable_fallback" json:"enable_fallback"`

	// EnableCostTracking enables per-request token usage aggregation.
	EnableCostTracking bool `yaml:"enable_cost_tracking" json:"enable_cost_tracking"`
}

// DefaultRouterConfig returns sensible default routing rules.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		Routes: []RouteRule{
			{TaskType: TaskRanking, Tier: TierFast, MaxTokens: 1024},
			{TaskType: TaskExtraction, Tier: TierQuality, MaxTokens: 4096},
			{TaskType: TaskExplanation, Tier: TierFast, MaxTokens: 2048},
			{TaskType: TaskQuality, Tier: TierQuality, MaxTokens: 2048},
			{TaskType: TaskGeneral, Tier: TierFast},
		},
		EnableFallback:     true,
		EnableCostTracking: true,
	}
}

// CostRecord tracks token usage for a single request.
type CostRecord struct {
	TaskType     TaskType      `json:"task_type"`
	Tier         ModelTier     `json:"tier"`
	Model        string        `json:"model"`
	PromptTokens int           `json:"prompt_tokens"`
	OutputTokens int           `json:"output_tokens"`
	TotalTokens  int           `json:"total_tokens"`
	Latency      time.Duration `json:"latency_ms"`
	Fallback     bool          `json:"fallback"`
	Timestamp    time.Time     `json:"timestamp"`
}

// CostSummary aggregates cost records by task type.
type CostSummary struct {
	TotalRequests int                        `json:"total_requests"`
	TotalTokens   int                        `json:"total_tokens"`
	FallbackCount int                        `json:"fallback_count"`
	ByTaskType    map[TaskType]*TaskTypeCost `json:"by_task_type"`
}

// TaskTypeCost tracks cost metrics for a specific task type.
type TaskTypeCost struct {
	Requests     int           `json:"requests"`
	TotalTokens  int           `json:"total_tokens"`
	PromptTokens int           `json:"prompt_tokens"`
	OutputTokens int           `json:"output_tokens"`
	MeanLatency  time.Duration `json:"mean_latency_ms"`
	totalLatency time.Duration
}

// Router wraps two Client backends (fast/quality) and selects the
// right one based on task type, with optional fallback cascading.
type Router struct {
	fast    Client
	quality Client
	config  RouterConfig
	rules   map[TaskType]RouteRule

	// cost tracking
	mu       sync.Mutex
	records  []CostRecord
	reqCount atomic.Int64
}

// NewRouter creates a multi-model router.
// fast is the default low-latency model, quality is the high-accuracy model.
// If quality is nil, all requests go to the fast model and fallback is disabled.
func NewRouter(fast, quality Client, cfg RouterConfig) *Router {
	rules := make(map[TaskType]RouteRule, len(cfg.Routes))
	for _, r := range cfg.Routes {
		rules[r.TaskType] = r
	}

	return &Router{
		fast:    fast,
		quality: quality,
		config:  cfg,
		rules:   rules,
	}
}

// ClientForTask returns the appropriate Client for a task type,
// after applying any route-level overrides (e.g. MaxTokens).
func (r *Router) ClientForTask(taskType TaskType) Client {
	tier := r.tierForTask(taskType)
	if tier == TierQuality && r.quality != nil {
		return r.quality
	}
	return r.fast
}

// Complete routes a completion request based on task type.
func (r *Router) Complete(ctx context.Context, taskType TaskType, req CompletionRequest) (*CompletionResponse, error) {
	rule := r.ruleForTask(taskType)
	if rule.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = rule.MaxTokens
	}

	tier := r.tierForTask(taskType)
	client := r.clientForTier(tier)
	start := time.Now()

	resp, err := client.Complete(ctx, req)
	if err != nil && r.shouldFallback(tier) {
		// Escalate to quality tier
		client = r.quality
		tier = TierQuality
		resp, err = client.Complete(ctx, req)
		r.trackCost(taskType, tier, resp, time.Since(start), true)
		return resp, err
	}

	r.trackCost(taskType, tier, resp, time.Since(start), false)
	return resp, err
}

// CompleteJSON routes a JSON completion request based on task type.
func (r *Router) CompleteJSON(ctx context.Context, taskType TaskType, req CompletionRequest) (json.RawMessage, error) {
	rule := r.ruleForTask(taskType)
	if rule.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = rule.MaxTokens
	}

	tier := r.tierForTask(taskType)
	client := r.clientForTier(tier)
	start := time.Now()

	resp, err := client.CompleteJSON(ctx, req)
	if err != nil && r.shouldFallback(tier) {
		client = r.quality
		tier = TierQuality
		resp, err = client.CompleteJSON(ctx, req)
		r.trackCostSimple(taskType, tier, time.Since(start), true)
		return resp, err
	}

	r.trackCostSimple(taskType, tier, time.Since(start), false)
	return resp, err
}

// CompleteStructured routes a structured completion request based on task type.
func (r *Router) CompleteStructured(ctx context.Context, taskType TaskType, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
	rule := r.ruleForTask(taskType)
	if rule.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = rule.MaxTokens
	}

	tier := r.tierForTask(taskType)
	client := r.clientForTier(tier)
	start := time.Now()

	resp, err := client.CompleteStructured(ctx, req, schemaName, schema)
	if err != nil && r.shouldFallback(tier) {
		client = r.quality
		tier = TierQuality
		resp, err = client.CompleteStructured(ctx, req, schemaName, schema)
		r.trackCostSimple(taskType, tier, time.Since(start), true)
		return resp, err
	}

	r.trackCostSimple(taskType, tier, time.Since(start), false)
	return resp, err
}

// GetCostSummary returns aggregated cost metrics.
func (r *Router) GetCostSummary() CostSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	summary := CostSummary{
		ByTaskType: make(map[TaskType]*TaskTypeCost),
	}

	for _, rec := range r.records {
		summary.TotalRequests++
		summary.TotalTokens += rec.TotalTokens
		if rec.Fallback {
			summary.FallbackCount++
		}

		tc, ok := summary.ByTaskType[rec.TaskType]
		if !ok {
			tc = &TaskTypeCost{}
			summary.ByTaskType[rec.TaskType] = tc
		}
		tc.Requests++
		tc.TotalTokens += rec.TotalTokens
		tc.PromptTokens += rec.PromptTokens
		tc.OutputTokens += rec.OutputTokens
		tc.totalLatency += rec.Latency
		tc.MeanLatency = tc.totalLatency / time.Duration(tc.Requests)
	}

	return summary
}

// ResetCosts clears all cost tracking records.
func (r *Router) ResetCosts() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = nil
}

// --- internal helpers ---

func (r *Router) tierForTask(taskType TaskType) ModelTier {
	if rule, ok := r.rules[taskType]; ok {
		return rule.Tier
	}
	return TierFast // default
}

func (r *Router) ruleForTask(taskType TaskType) RouteRule {
	if rule, ok := r.rules[taskType]; ok {
		return rule
	}
	return RouteRule{TaskType: taskType, Tier: TierFast}
}

func (r *Router) clientForTier(tier ModelTier) Client {
	if tier == TierQuality && r.quality != nil {
		return r.quality
	}
	return r.fast
}

func (r *Router) shouldFallback(currentTier ModelTier) bool {
	return r.config.EnableFallback && currentTier == TierFast && r.quality != nil
}

func (r *Router) trackCost(taskType TaskType, tier ModelTier, resp *CompletionResponse, latency time.Duration, fallback bool) {
	if !r.config.EnableCostTracking {
		return
	}

	rec := CostRecord{
		TaskType:  taskType,
		Tier:      tier,
		Latency:   latency,
		Fallback:  fallback,
		Timestamp: time.Now(),
	}
	if resp != nil {
		rec.Model = resp.Model
		rec.PromptTokens = resp.Usage.PromptTokens
		rec.OutputTokens = resp.Usage.CompletionTokens
		rec.TotalTokens = resp.Usage.TotalTokens
	}

	r.mu.Lock()
	r.records = append(r.records, rec)
	r.mu.Unlock()
	r.reqCount.Add(1)
}

func (r *Router) trackCostSimple(taskType TaskType, tier ModelTier, latency time.Duration, fallback bool) {
	if !r.config.EnableCostTracking {
		return
	}

	rec := CostRecord{
		TaskType:  taskType,
		Tier:      tier,
		Latency:   latency,
		Fallback:  fallback,
		Timestamp: time.Now(),
	}

	r.mu.Lock()
	r.records = append(r.records, rec)
	r.mu.Unlock()
	r.reqCount.Add(1)
}

// Ensure Router is documented as not implementing Client directly
// (it takes taskType as an extra parameter for routing decisions).
var _ fmt.Stringer = ModelTier("")

// String implements fmt.Stringer for ModelTier.
func (m ModelTier) String() string { return string(m) }

// String implements fmt.Stringer for TaskType.
func (t TaskType) String() string { return string(t) }
