package workflow

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStaticEventGenerator(t *testing.T) {
	event := map[string]interface{}{"type": "test"}
	gen := &StaticEventGenerator{Event: event, EventName: "test_event"}

	// Generate should always return the same event (by pointer)
	for i := 0; i < 10; i++ {
		got := gen.Generate().(map[string]interface{})
		if got["type"] != event["type"] {
			t.Error("StaticEventGenerator should return the same event")
		}
	}

	if gen.Name() != "test_event" {
		t.Errorf("Expected name 'test_event', got %q", gen.Name())
	}
}

func TestRandomEventGenerator(t *testing.T) {
	events := []interface{}{
		map[string]interface{}{"type": "a"},
		map[string]interface{}{"type": "b"},
		map[string]interface{}{"type": "c"},
	}

	gen := NewRandomEventGenerator(events)

	// Generate many events and verify all types are seen
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		e := gen.Generate().(map[string]interface{})
		seen[e["type"].(string)] = true
	}

	if len(seen) < 2 {
		t.Error("RandomEventGenerator should produce varied events")
	}

	if !strings.Contains(gen.Name(), "random") {
		t.Errorf("Expected name to contain 'random', got %q", gen.Name())
	}
}

func TestWeightedEventGenerator(t *testing.T) {
	events := []WeightedEvent{
		{Event: map[string]interface{}{"type": "common"}, Weight: 90},
		{Event: map[string]interface{}{"type": "rare"}, Weight: 10},
	}

	gen := NewWeightedEventGenerator(events)

	// Generate many events and check distribution
	counts := make(map[string]int)
	iterations := 1000
	for i := 0; i < iterations; i++ {
		e := gen.Generate().(map[string]interface{})
		counts[e["type"].(string)]++
	}

	// Common should appear much more than rare
	commonRatio := float64(counts["common"]) / float64(iterations)
	if commonRatio < 0.80 {
		t.Errorf("Expected 'common' to appear ~90%% of time, got %.1f%%", commonRatio*100)
	}

	if !strings.Contains(gen.Name(), "weighted") {
		t.Errorf("Expected name to contain 'weighted', got %q", gen.Name())
	}
}

func TestSequenceEventGenerator(t *testing.T) {
	events := []interface{}{
		map[string]interface{}{"index": 0},
		map[string]interface{}{"index": 1},
		map[string]interface{}{"index": 2},
	}

	gen := &SequenceEventGenerator{Events: events}

	// Should cycle through in order
	for cycle := 0; cycle < 2; cycle++ {
		for i := 0; i < len(events); i++ {
			e := gen.Generate().(map[string]interface{})
			expectedIdx := (cycle*len(events) + i) % len(events)
			if e["index"].(int) != expectedIdx {
				t.Errorf("Expected index %d, got %v", expectedIdx, e["index"])
			}
		}
	}
}

func TestHealthcareEventGenerator(t *testing.T) {
	gen := NewHealthcareEventGenerator()

	// Generate events and verify they have expected structure
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		e := gen.Generate().(map[string]interface{})
		eventType, ok := e["type"].(string)
		if !ok {
			t.Error("Event should have 'type' field")
		}
		seen[eventType] = true
	}

	// Should see at least a few different event types
	if len(seen) < 3 {
		t.Errorf("Expected diverse events, only saw %d types: %v", len(seen), seen)
	}

	if gen.Name() != "healthcare_mix" {
		t.Errorf("Expected name 'healthcare_mix', got %q", gen.Name())
	}
}

func TestLoadTester_Run_Simple(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	tester := NewLoadTester(engine)

	config := &LoadTestConfig{
		Duration:       500 * time.Millisecond,
		TargetRPS:      100,
		Workers:        2,
		WarmupDuration: 100 * time.Millisecond,
		EventGenerator: &StaticEventGenerator{
			Event: map[string]interface{}{"type": "test"},
		},
	}

	result, err := tester.Run(context.Background(), config)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have processed some events
	if result.TotalEvents < 10 {
		t.Errorf("Expected at least 10 events, got %d", result.TotalEvents)
	}

	// Should have latency stats
	if result.LatencyMean == 0 {
		t.Error("Expected non-zero mean latency")
	}

	// Error rate should be low
	if result.ErrorRate > 0.01 {
		t.Errorf("Expected low error rate, got %.2f%%", result.ErrorRate*100)
	}
}

func TestLoadTester_Run_NoRateLimit(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	tester := NewLoadTester(engine)

	config := &LoadTestConfig{
		Duration:       200 * time.Millisecond,
		TargetRPS:      0, // No rate limit
		Workers:        4,
		WarmupDuration: 50 * time.Millisecond,
		EventGenerator: &StaticEventGenerator{
			Event: map[string]interface{}{"type": "test"},
		},
	}

	result, err := tester.Run(context.Background(), config)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// With no rate limit, should process many more events
	if result.TotalEvents < 100 {
		t.Errorf("Expected high throughput without rate limit, got %d events", result.TotalEvents)
	}
}

func TestLoadTester_Run_WithProgress(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	engine, _ := NewEngine(workflow)
	tester := NewLoadTester(engine)

	var progressCount int64
	config := &LoadTestConfig{
		Duration:         300 * time.Millisecond,
		TargetRPS:        50,
		Workers:          2,
		WarmupDuration:   50 * time.Millisecond,
		ProgressInterval: 50 * time.Millisecond,
		EventGenerator: &StaticEventGenerator{
			Event: map[string]interface{}{"type": "test"},
		},
		OnProgress: func(stats LoadTestProgress) {
			atomic.AddInt64(&progressCount, 1)
		},
	}

	result, err := tester.Run(context.Background(), config)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have received progress callbacks
	if atomic.LoadInt64(&progressCount) < 2 {
		t.Errorf("Expected progress callbacks, got %d", progressCount)
	}

	_ = result
}

func TestLoadTester_Run_ContextCancellation(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	engine, _ := NewEngine(workflow)
	tester := NewLoadTester(engine)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	config := &LoadTestConfig{
		Duration:       10 * time.Second, // Long duration
		TargetRPS:      100,
		Workers:        2,
		WarmupDuration: 0,
		EventGenerator: &StaticEventGenerator{
			Event: map[string]interface{}{"type": "test"},
		},
	}

	start := time.Now()
	_, err := tester.Run(ctx, config)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have stopped early due to context cancellation
	if elapsed > 500*time.Millisecond {
		t.Errorf("Expected early termination, took %v", elapsed)
	}
}

func TestLoadTestResult_Summary(t *testing.T) {
	result := &LoadTestResult{
		Config: LoadTestConfig{
			Duration:  30 * time.Second,
			TargetRPS: 1000,
			Workers:   4,
		},
		TotalEvents:   28000,
		EventsPerSec:  933.33,
		AchievedRatio: 0.93,
		LatencyMin:    100 * time.Microsecond,
		LatencyMean:   500 * time.Microsecond,
		LatencyP50:    450 * time.Microsecond,
		LatencyP90:    800 * time.Microsecond,
		LatencyP95:    1 * time.Millisecond,
		LatencyP99:    2 * time.Millisecond,
		LatencyP999:   5 * time.Millisecond,
		LatencyMax:    10 * time.Millisecond,
		ErrorCount:    5,
		ErrorRate:     0.000178,
	}

	summary := result.Summary()

	// Check key elements are present
	checks := []string{
		"Load Test Results",
		"Total Events: 28000",
		"Events/sec:",
		"P99:",
		"Error",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("Summary should contain %q", check)
		}
	}
}

func TestLoadTestResult_Passed(t *testing.T) {
	tests := []struct {
		name             string
		result           *LoadTestResult
		minAchievedRatio float64
		maxErrorRate     float64
		maxP99Latency    time.Duration
		expectedPass     bool
	}{
		{
			name: "all passing",
			result: &LoadTestResult{
				TargetRPS:     1000,
				AchievedRatio: 0.95,
				ErrorRate:     0.001,
				LatencyP99:    1 * time.Millisecond,
			},
			minAchievedRatio: 0.90,
			maxErrorRate:     0.01,
			maxP99Latency:    5 * time.Millisecond,
			expectedPass:     true,
		},
		{
			name: "low throughput",
			result: &LoadTestResult{
				TargetRPS:     1000,
				AchievedRatio: 0.80, // Only 80%
				ErrorRate:     0.001,
				LatencyP99:    1 * time.Millisecond,
			},
			minAchievedRatio: 0.90,
			maxErrorRate:     0.01,
			maxP99Latency:    5 * time.Millisecond,
			expectedPass:     false,
		},
		{
			name: "high error rate",
			result: &LoadTestResult{
				TargetRPS:     1000,
				AchievedRatio: 0.95,
				ErrorRate:     0.05, // 5%
				LatencyP99:    1 * time.Millisecond,
			},
			minAchievedRatio: 0.90,
			maxErrorRate:     0.01,
			maxP99Latency:    5 * time.Millisecond,
			expectedPass:     false,
		},
		{
			name: "high latency",
			result: &LoadTestResult{
				TargetRPS:     1000,
				AchievedRatio: 0.95,
				ErrorRate:     0.001,
				LatencyP99:    10 * time.Millisecond, // Too high
			},
			minAchievedRatio: 0.90,
			maxErrorRate:     0.01,
			maxP99Latency:    5 * time.Millisecond,
			expectedPass:     false,
		},
		{
			name: "no target RPS (burst mode)",
			result: &LoadTestResult{
				TargetRPS:     0,
				AchievedRatio: 0, // Doesn't matter
				ErrorRate:     0.001,
				LatencyP99:    1 * time.Millisecond,
			},
			minAchievedRatio: 0.90,
			maxErrorRate:     0.01,
			maxP99Latency:    5 * time.Millisecond,
			expectedPass:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			passed := tc.result.Passed(tc.minAchievedRatio, tc.maxErrorRate, tc.maxP99Latency)
			if passed != tc.expectedPass {
				t.Errorf("Expected Passed()=%v, got %v", tc.expectedPass, passed)
			}
		})
	}
}

func TestStandardScenarios(t *testing.T) {
	scenarios := StandardScenarios()

	if len(scenarios) < 4 {
		t.Errorf("Expected at least 4 standard scenarios, got %d", len(scenarios))
	}

	// Check smoke test exists
	smoke := GetScenario("smoke")
	if smoke == nil {
		t.Error("Expected 'smoke' scenario to exist")
	}
	if smoke.Config.Duration != 10*time.Second {
		t.Errorf("Smoke test should be 10s, got %v", smoke.Config.Duration)
	}

	// Check standard test exists
	standard := GetScenario("standard")
	if standard == nil {
		t.Error("Expected 'standard' scenario to exist")
	}

	// Non-existent scenario
	if GetScenario("nonexistent") != nil {
		t.Error("Expected nil for non-existent scenario")
	}
}

func TestDefaultLoadTestConfig(t *testing.T) {
	config := DefaultLoadTestConfig()

	if config.Duration < 1*time.Second {
		t.Error("Default duration should be at least 1 second")
	}
	if config.Workers < 1 {
		t.Error("Default workers should be at least 1")
	}
	if config.TargetRPS < 1 {
		t.Error("Default RPS should be at least 1")
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
		epsilon  float64
	}{
		{4, 2, 0.001},
		{9, 3, 0.001},
		{16, 4, 0.001},
		{2, 1.414, 0.01},
		{0, 0, 0.001},
	}

	for _, tc := range tests {
		result := sqrt(tc.input)
		diff := result - tc.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > tc.epsilon {
			t.Errorf("sqrt(%f) = %f, expected ~%f", tc.input, result, tc.expected)
		}
	}
}
