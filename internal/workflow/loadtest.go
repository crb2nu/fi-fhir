package workflow

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LoadTestConfig configures a load test run.
type LoadTestConfig struct {
	// Duration is how long to run the load test.
	Duration time.Duration

	// TargetRPS is the target requests (events) per second.
	// Use 0 for maximum throughput (no rate limiting).
	TargetRPS int

	// Workers is the number of concurrent workers processing events.
	Workers int

	// WarmupDuration is an optional warmup period before measurements start.
	WarmupDuration time.Duration

	// EventGenerator produces events for the load test.
	EventGenerator EventGenerator

	// ProgressInterval controls how often progress is reported.
	// Use 0 to disable progress reporting.
	ProgressInterval time.Duration

	// OnProgress is called periodically with progress updates.
	OnProgress func(stats LoadTestProgress)
}

// DefaultLoadTestConfig returns a sensible default configuration.
func DefaultLoadTestConfig() *LoadTestConfig {
	return &LoadTestConfig{
		Duration:         30 * time.Second,
		TargetRPS:        1000,
		Workers:          4,
		WarmupDuration:   5 * time.Second,
		ProgressInterval: 1 * time.Second,
	}
}

// LoadTestProgress contains progress information during a load test.
type LoadTestProgress struct {
	Elapsed       time.Duration
	EventsTotal   int64
	EventsPerSec  float64
	ErrorCount    int64
	P50Latency    time.Duration
	P99Latency    time.Duration
	WorkersActive int
}

// LoadTestResult contains the complete results of a load test.
type LoadTestResult struct {
	// Configuration
	Config LoadTestConfig

	// Timing
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	WarmupEvents int64

	// Throughput
	TotalEvents   int64
	EventsPerSec  float64
	TargetRPS     int
	AchievedRatio float64 // Achieved RPS / Target RPS

	// Latency (in nanoseconds for precision)
	LatencyMin    time.Duration
	LatencyMax    time.Duration
	LatencyMean   time.Duration
	LatencyP50    time.Duration
	LatencyP90    time.Duration
	LatencyP95    time.Duration
	LatencyP99    time.Duration
	LatencyP999   time.Duration
	LatencyStdDev time.Duration

	// Errors
	ErrorCount int64
	ErrorRate  float64
	Errors     []LoadTestError

	// Resource usage
	PeakWorkers int
}

// LoadTestError records an error during load testing.
type LoadTestError struct {
	Timestamp time.Time
	EventType string
	Error     string
}

// Summary returns a formatted summary of load test results.
func (r *LoadTestResult) Summary() string {
	var sb string

	sb += "Load Test Results\n"
	sb += "================\n\n"

	sb += "Configuration:\n"
	sb += fmt.Sprintf("  Duration:    %v\n", r.Config.Duration)
	sb += fmt.Sprintf("  Target RPS:  %d\n", r.Config.TargetRPS)
	sb += fmt.Sprintf("  Workers:     %d\n", r.Config.Workers)
	sb += fmt.Sprintf("  Warmup:      %v\n", r.Config.WarmupDuration)
	sb += "\n"

	sb += "Throughput:\n"
	sb += fmt.Sprintf("  Total Events: %d\n", r.TotalEvents)
	sb += fmt.Sprintf("  Events/sec:   %.2f\n", r.EventsPerSec)
	if r.TargetRPS > 0 {
		sb += fmt.Sprintf("  Achievement:  %.1f%% of target\n", r.AchievedRatio*100)
	}
	sb += "\n"

	sb += "Latency:\n"
	sb += fmt.Sprintf("  Min:    %v\n", r.LatencyMin)
	sb += fmt.Sprintf("  Mean:   %v\n", r.LatencyMean)
	sb += fmt.Sprintf("  P50:    %v\n", r.LatencyP50)
	sb += fmt.Sprintf("  P90:    %v\n", r.LatencyP90)
	sb += fmt.Sprintf("  P95:    %v\n", r.LatencyP95)
	sb += fmt.Sprintf("  P99:    %v\n", r.LatencyP99)
	sb += fmt.Sprintf("  P99.9:  %v\n", r.LatencyP999)
	sb += fmt.Sprintf("  Max:    %v\n", r.LatencyMax)
	sb += "\n"

	sb += "Errors:\n"
	sb += fmt.Sprintf("  Count:  %d\n", r.ErrorCount)
	sb += fmt.Sprintf("  Rate:   %.4f%%\n", r.ErrorRate*100)

	return sb
}

// Passed returns true if the load test met its targets.
func (r *LoadTestResult) Passed(minAchievedRatio, maxErrorRate float64, maxP99Latency time.Duration) bool {
	if r.TargetRPS > 0 && r.AchievedRatio < minAchievedRatio {
		return false
	}
	if r.ErrorRate > maxErrorRate {
		return false
	}
	if r.LatencyP99 > maxP99Latency {
		return false
	}
	return true
}

// EventGenerator produces events for load testing.
type EventGenerator interface {
	// Generate produces a new event for load testing.
	Generate() interface{}

	// Name returns the generator name for reporting.
	Name() string
}

// StaticEventGenerator always returns the same event.
type StaticEventGenerator struct {
	Event     interface{}
	EventName string
}

// Generate returns the static event.
func (g *StaticEventGenerator) Generate() interface{} {
	return g.Event
}

// Name returns the generator name.
func (g *StaticEventGenerator) Name() string {
	if g.EventName != "" {
		return g.EventName
	}
	return "static"
}

// RandomEventGenerator picks from a pool of event templates.
type RandomEventGenerator struct {
	Events    []interface{}
	EventName string
	rng       *rand.Rand
	mu        sync.Mutex
}

// NewRandomEventGenerator creates a generator that randomly selects from events.
func NewRandomEventGenerator(events []interface{}) *RandomEventGenerator {
	return &RandomEventGenerator{
		Events: events,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Generate returns a random event from the pool.
func (g *RandomEventGenerator) Generate() interface{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Events[g.rng.Intn(len(g.Events))]
}

// Name returns the generator name.
func (g *RandomEventGenerator) Name() string {
	if g.EventName != "" {
		return g.EventName
	}
	return fmt.Sprintf("random(%d events)", len(g.Events))
}

// WeightedEventGenerator picks events based on weights.
type WeightedEventGenerator struct {
	Events      []WeightedEvent
	EventName   string
	totalWeight int
	rng         *rand.Rand
	mu          sync.Mutex
}

// WeightedEvent pairs an event with a selection weight.
type WeightedEvent struct {
	Event  interface{}
	Weight int
}

// NewWeightedEventGenerator creates a generator with weighted event selection.
func NewWeightedEventGenerator(events []WeightedEvent) *WeightedEventGenerator {
	total := 0
	for _, e := range events {
		total += e.Weight
	}
	return &WeightedEventGenerator{
		Events:      events,
		totalWeight: total,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Generate returns an event based on weights.
func (g *WeightedEventGenerator) Generate() interface{} {
	g.mu.Lock()
	defer g.mu.Unlock()

	r := g.rng.Intn(g.totalWeight)
	cumulative := 0
	for _, e := range g.Events {
		cumulative += e.Weight
		if r < cumulative {
			return e.Event
		}
	}
	return g.Events[len(g.Events)-1].Event
}

// Name returns the generator name.
func (g *WeightedEventGenerator) Name() string {
	if g.EventName != "" {
		return g.EventName
	}
	return fmt.Sprintf("weighted(%d events)", len(g.Events))
}

// SequenceEventGenerator cycles through events in order.
type SequenceEventGenerator struct {
	Events    []interface{}
	EventName string
	index     int64
}

// Generate returns the next event in sequence.
func (g *SequenceEventGenerator) Generate() interface{} {
	idx := atomic.AddInt64(&g.index, 1) - 1
	return g.Events[idx%int64(len(g.Events))]
}

// Name returns the generator name.
func (g *SequenceEventGenerator) Name() string {
	if g.EventName != "" {
		return g.EventName
	}
	return fmt.Sprintf("sequence(%d events)", len(g.Events))
}

// LoadTester runs load tests against a workflow engine.
type LoadTester struct {
	engine *Engine
}

// NewLoadTester creates a new load tester for the given engine.
func NewLoadTester(engine *Engine) *LoadTester {
	return &LoadTester{engine: engine}
}

// Run executes a load test with the given configuration.
func (lt *LoadTester) Run(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	if config.EventGenerator == nil {
		return nil, fmt.Errorf("EventGenerator is required")
	}
	if config.Workers < 1 {
		config.Workers = 1
	}
	if config.Duration < 1*time.Second {
		config.Duration = 1 * time.Second
	}

	result := &LoadTestResult{
		Config:      *config,
		TargetRPS:   config.TargetRPS,
		PeakWorkers: config.Workers,
		Errors:      make([]LoadTestError, 0),
	}

	// Channels for coordination
	eventCh := make(chan interface{}, config.Workers*10)
	latencyCh := make(chan time.Duration, 10000)
	errorCh := make(chan LoadTestError, 1000)
	doneCh := make(chan struct{})

	// Counters
	var totalEvents int64
	var warmupEvents int64
	var errorCount int64
	var isWarmup int32 = 1

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for event := range eventCh {
				start := time.Now()
				result := lt.engine.Process(event)
				elapsed := time.Since(start)

				if atomic.LoadInt32(&isWarmup) == 0 {
					latencyCh <- elapsed

					if result.HasErrors() {
						atomic.AddInt64(&errorCount, 1)
						for _, err := range result.AllErrors() {
							select {
							case errorCh <- LoadTestError{
								Timestamp: time.Now(),
								Error:     err.Error(),
							}:
							default:
								// Error buffer full, skip
							}
						}
					}
				} else {
					atomic.AddInt64(&warmupEvents, 1)
				}

				atomic.AddInt64(&totalEvents, 1)
			}
		}()
	}

	// Collect latencies
	latencies := make([]time.Duration, 0, 100000)
	var latencyMu sync.Mutex

	go func() {
		for {
			select {
			case l := <-latencyCh:
				latencyMu.Lock()
				latencies = append(latencies, l)
				latencyMu.Unlock()
			case <-doneCh:
				// Drain remaining
				for len(latencyCh) > 0 {
					l := <-latencyCh
					latencyMu.Lock()
					latencies = append(latencies, l)
					latencyMu.Unlock()
				}
				return
			}
		}
	}()

	// Collect errors
	go func() {
		for {
			select {
			case err := <-errorCh:
				result.Errors = append(result.Errors, err)
			case <-doneCh:
				for len(errorCh) > 0 {
					err := <-errorCh
					result.Errors = append(result.Errors, err)
				}
				return
			}
		}
	}()

	// Rate limiter
	var ticker *time.Ticker
	if config.TargetRPS > 0 {
		interval := time.Second / time.Duration(config.TargetRPS)
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	// Progress reporter
	var progressTicker *time.Ticker
	if config.ProgressInterval > 0 && config.OnProgress != nil {
		progressTicker = time.NewTicker(config.ProgressInterval)
		defer progressTicker.Stop()
	}

	// Run warmup
	if config.WarmupDuration > 0 {
		warmupCtx, warmupCancel := context.WithTimeout(ctx, config.WarmupDuration)
		lt.generateEvents(warmupCtx, eventCh, config, ticker)
		warmupCancel()
	}

	// Start measurement
	atomic.StoreInt32(&isWarmup, 0)
	result.StartTime = time.Now()
	result.WarmupEvents = atomic.LoadInt64(&warmupEvents)

	// Create measurement context
	measureCtx, measureCancel := context.WithTimeout(ctx, config.Duration)
	defer measureCancel()

	// Generate events with progress reporting
	go func() {
		lt.generateEvents(measureCtx, eventCh, config, ticker)
		close(eventCh)
	}()

	// Progress reporting loop
	if progressTicker != nil {
		go func() {
			for {
				select {
				case <-progressTicker.C:
					elapsed := time.Since(result.StartTime)
					events := atomic.LoadInt64(&totalEvents) - result.WarmupEvents
					errors := atomic.LoadInt64(&errorCount)

					latencyMu.Lock()
					var p50, p99 time.Duration
					if len(latencies) > 0 {
						sorted := make([]time.Duration, len(latencies))
						copy(sorted, latencies)
						sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
						p50 = sorted[len(sorted)/2]
						p99 = sorted[int(float64(len(sorted))*0.99)]
					}
					latencyMu.Unlock()

					config.OnProgress(LoadTestProgress{
						Elapsed:       elapsed,
						EventsTotal:   events,
						EventsPerSec:  float64(events) / elapsed.Seconds(),
						ErrorCount:    errors,
						P50Latency:    p50,
						P99Latency:    p99,
						WorkersActive: config.Workers,
					})
				case <-measureCtx.Done():
					return
				}
			}
		}()
	}

	// Wait for workers to finish
	wg.Wait()
	close(doneCh)
	result.EndTime = time.Now()

	// Calculate results
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalEvents = atomic.LoadInt64(&totalEvents) - result.WarmupEvents
	result.EventsPerSec = float64(result.TotalEvents) / result.Duration.Seconds()
	result.ErrorCount = atomic.LoadInt64(&errorCount)

	if result.TotalEvents > 0 {
		result.ErrorRate = float64(result.ErrorCount) / float64(result.TotalEvents)
	}

	if config.TargetRPS > 0 {
		result.AchievedRatio = result.EventsPerSec / float64(config.TargetRPS)
	}

	// Calculate latency statistics
	latencyMu.Lock()
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

		result.LatencyMin = latencies[0]
		result.LatencyMax = latencies[len(latencies)-1]
		result.LatencyP50 = latencies[len(latencies)/2]
		result.LatencyP90 = latencies[int(float64(len(latencies))*0.90)]
		result.LatencyP95 = latencies[int(float64(len(latencies))*0.95)]
		result.LatencyP99 = latencies[int(float64(len(latencies))*0.99)]
		p999Idx := int(float64(len(latencies)) * 0.999)
		if p999Idx >= len(latencies) {
			p999Idx = len(latencies) - 1
		}
		result.LatencyP999 = latencies[p999Idx]

		// Calculate mean
		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		result.LatencyMean = sum / time.Duration(len(latencies))

		// Calculate standard deviation
		var variance float64
		meanNs := float64(result.LatencyMean.Nanoseconds())
		for _, l := range latencies {
			diff := float64(l.Nanoseconds()) - meanNs
			variance += diff * diff
		}
		variance /= float64(len(latencies))
		result.LatencyStdDev = time.Duration(sqrt(variance))
	}
	latencyMu.Unlock()

	return result, nil
}

// generateEvents sends events to the channel until context is done.
func (lt *LoadTester) generateEvents(ctx context.Context, eventCh chan<- interface{}, config *LoadTestConfig, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if ticker != nil {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
			}

			event := config.EventGenerator.Generate()
			select {
			case eventCh <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

// sqrt calculates square root using Newton's method (avoids math import).
func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 100; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// HealthcareEventGenerator generates realistic healthcare event mixes.
type HealthcareEventGenerator struct {
	gen *WeightedEventGenerator
}

// NewHealthcareEventGenerator creates a generator with realistic healthcare event distribution.
func NewHealthcareEventGenerator() *HealthcareEventGenerator {
	events := []WeightedEvent{
		// ADT events (most common)
		{Weight: 25, Event: map[string]interface{}{
			"type": "patient_admit", "source": "epic_adt",
			"patient": map[string]interface{}{"mrn": "12345", "name": "John Doe"},
		}},
		{Weight: 10, Event: map[string]interface{}{
			"type": "patient_discharge", "source": "epic_adt",
			"patient": map[string]interface{}{"mrn": "12345"},
		}},
		{Weight: 15, Event: map[string]interface{}{
			"type": "patient_update", "source": "epic_adt",
			"patient": map[string]interface{}{"mrn": "12345"},
		}},
		{Weight: 5, Event: map[string]interface{}{
			"type": "patient_transfer", "source": "epic_adt",
		}},

		// Lab results (very common)
		{Weight: 30, Event: map[string]interface{}{
			"type": "lab_result", "source": "lab_system",
			"test":   map[string]interface{}{"code": "2345-7", "name": "Glucose"},
			"result": map[string]interface{}{"value": 95, "unit": "mg/dL"},
		}},

		// Scheduling
		{Weight: 10, Event: map[string]interface{}{
			"type": "appointment_scheduled", "source": "scheduling",
		}},

		// Claims (less frequent)
		{Weight: 5, Event: map[string]interface{}{
			"type": "claim_submitted", "source": "billing",
		}},
	}

	return &HealthcareEventGenerator{
		gen: NewWeightedEventGenerator(events),
	}
}

// Generate produces a realistic healthcare event.
func (g *HealthcareEventGenerator) Generate() interface{} {
	return g.gen.Generate()
}

// Name returns the generator name.
func (g *HealthcareEventGenerator) Name() string {
	return "healthcare_mix"
}

// LoadTestScenario defines a pre-configured load test scenario.
type LoadTestScenario struct {
	Name        string
	Description string
	Config      *LoadTestConfig
}

// StandardScenarios returns pre-defined load test scenarios.
func StandardScenarios() []LoadTestScenario {
	return []LoadTestScenario{
		{
			Name:        "smoke",
			Description: "Quick smoke test (10s, 100 RPS)",
			Config: &LoadTestConfig{
				Duration:         10 * time.Second,
				TargetRPS:        100,
				Workers:          2,
				WarmupDuration:   2 * time.Second,
				ProgressInterval: 1 * time.Second,
				EventGenerator:   NewHealthcareEventGenerator(),
			},
		},
		{
			Name:        "standard",
			Description: "Standard load test (60s, 1000 RPS)",
			Config: &LoadTestConfig{
				Duration:         60 * time.Second,
				TargetRPS:        1000,
				Workers:          4,
				WarmupDuration:   5 * time.Second,
				ProgressInterval: 5 * time.Second,
				EventGenerator:   NewHealthcareEventGenerator(),
			},
		},
		{
			Name:        "stress",
			Description: "Stress test (120s, 5000 RPS)",
			Config: &LoadTestConfig{
				Duration:         120 * time.Second,
				TargetRPS:        5000,
				Workers:          8,
				WarmupDuration:   10 * time.Second,
				ProgressInterval: 10 * time.Second,
				EventGenerator:   NewHealthcareEventGenerator(),
			},
		},
		{
			Name:        "burst",
			Description: "Burst test (30s, max throughput)",
			Config: &LoadTestConfig{
				Duration:         30 * time.Second,
				TargetRPS:        0, // No rate limiting
				Workers:          16,
				WarmupDuration:   5 * time.Second,
				ProgressInterval: 5 * time.Second,
				EventGenerator:   NewHealthcareEventGenerator(),
			},
		},
		{
			Name:        "soak",
			Description: "Soak test (5min, 500 RPS)",
			Config: &LoadTestConfig{
				Duration:         5 * time.Minute,
				TargetRPS:        500,
				Workers:          4,
				WarmupDuration:   10 * time.Second,
				ProgressInterval: 30 * time.Second,
				EventGenerator:   NewHealthcareEventGenerator(),
			},
		},
	}
}

// GetScenario returns a scenario by name.
func GetScenario(name string) *LoadTestScenario {
	for _, s := range StandardScenarios() {
		if s.Name == name {
			return &s
		}
	}
	return nil
}
