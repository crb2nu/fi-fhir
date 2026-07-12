package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkSuite_AddResult(t *testing.T) {
	suite := NewBenchmarkSuite("test")

	suite.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		Iterations:  1000,
		NsPerOp:     100.5,
		BytesPerOp:  256,
		AllocsPerOp: 5,
	})

	if len(suite.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(suite.Results))
	}

	result := suite.GetResult("BenchmarkTest")
	if result == nil {
		t.Fatal("Expected to find result")
	}
	if result.NsPerOp != 100.5 {
		t.Errorf("Expected NsPerOp 100.5, got %f", result.NsPerOp)
	}
}

func TestBenchmarkSuite_GetResult_NotFound(t *testing.T) {
	suite := NewBenchmarkSuite("test")

	result := suite.GetResult("NonExistent")
	if result != nil {
		t.Error("Expected nil for non-existent result")
	}
}

func TestBenchmarkSuite_Summary(t *testing.T) {
	suite := NewBenchmarkSuite("test-suite")

	suite.AddResult(BenchmarkResult{
		Name:         "BenchmarkA",
		Iterations:   1000,
		NsPerOp:      100.0,
		BytesPerOp:   256,
		AllocsPerOp:  5,
		EventsPerSec: 10000000,
	})

	suite.AddResult(BenchmarkResult{
		Name:        "BenchmarkB",
		Iterations:  500,
		NsPerOp:     200.0,
		BytesPerOp:  512,
		AllocsPerOp: 10,
	})

	summary := suite.Summary()

	// Check that key elements are present
	if !strings.Contains(summary, "test-suite") {
		t.Error("Summary should contain suite name")
	}
	if !strings.Contains(summary, "BenchmarkA") {
		t.Error("Summary should contain BenchmarkA")
	}
	if !strings.Contains(summary, "BenchmarkB") {
		t.Error("Summary should contain BenchmarkB")
	}
	if !strings.Contains(summary, "10000000") || !strings.Contains(summary, "events/sec") {
		t.Error("Summary should contain throughput")
	}
}

func TestBenchmarkComparison(t *testing.T) {
	baseline := NewBenchmarkSuite("baseline")
	baseline.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		NsPerOp:     100.0,
		AllocsPerOp: 5,
	})

	current := NewBenchmarkSuite("current")
	current.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		NsPerOp:     80.0, // 20% faster
		AllocsPerOp: 5,
	})

	comparison := Compare(baseline, current)

	if len(comparison.Diffs) != 1 {
		t.Fatalf("Expected 1 diff, got %d", len(comparison.Diffs))
	}

	diff := comparison.Diffs[0]
	if !diff.Improved {
		t.Error("Expected benchmark to be marked as improved")
	}
	if diff.Regressed {
		t.Error("Expected benchmark NOT to be marked as regressed")
	}

	// 80 is 20% less than 100, so delta should be -20
	expectedDelta := -20.0
	if diff.NsPerOpDelta < expectedDelta-0.1 || diff.NsPerOpDelta > expectedDelta+0.1 {
		t.Errorf("Expected delta around -20%%, got %.1f%%", diff.NsPerOpDelta)
	}
}

func TestBenchmarkComparison_Regression(t *testing.T) {
	baseline := NewBenchmarkSuite("baseline")
	baseline.AddResult(BenchmarkResult{
		Name:    "BenchmarkTest",
		NsPerOp: 100.0,
	})

	current := NewBenchmarkSuite("current")
	current.AddResult(BenchmarkResult{
		Name:    "BenchmarkTest",
		NsPerOp: 120.0, // 20% slower
	})

	comparison := Compare(baseline, current)

	if !comparison.HasRegressions() {
		t.Error("Expected regression to be detected")
	}

	diff := comparison.Diffs[0]
	if !diff.Regressed {
		t.Error("Expected benchmark to be marked as regressed")
	}
}

func TestBenchmarkComparison_Summary(t *testing.T) {
	baseline := NewBenchmarkSuite("v1.0")
	baseline.AddResult(BenchmarkResult{Name: "BenchmarkA", NsPerOp: 100.0})
	baseline.AddResult(BenchmarkResult{Name: "BenchmarkB", NsPerOp: 200.0})

	current := NewBenchmarkSuite("v1.1")
	current.AddResult(BenchmarkResult{Name: "BenchmarkA", NsPerOp: 80.0})  // Improved
	current.AddResult(BenchmarkResult{Name: "BenchmarkB", NsPerOp: 250.0}) // Regressed

	comparison := Compare(baseline, current)
	summary := comparison.Summary()

	if !strings.Contains(summary, "FASTER") {
		t.Error("Summary should indicate improved benchmark")
	}
	if !strings.Contains(summary, "SLOWER") {
		t.Error("Summary should indicate regressed benchmark")
	}
	if !strings.Contains(summary, "1 improved") {
		t.Error("Summary should show 1 improved")
	}
	if !strings.Contains(summary, "1 regressed") {
		t.Error("Summary should show 1 regressed")
	}
}

func TestPerformanceThresholds_Validate(t *testing.T) {
	// Test passing case
	t.Run("passing", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkFast": 100,
			},
			MaxAllocsPerOp: map[string]int64{
				"BenchmarkFast": 5,
			},
			MinThroughput: map[string]float64{
				"BenchmarkFast": 1000000,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:         "BenchmarkFast",
			NsPerOp:      50,
			AllocsPerOp:  3,
			EventsPerSec: 2000000,
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 0 {
			t.Errorf("Expected no violations, got: %v", violations)
		}
	})

	// Test ns/op violation (only checking ns/op threshold)
	t.Run("ns_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkSlow": 100,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:    "BenchmarkSlow",
			NsPerOp: 200, // Exceeds 100
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "exceeds threshold") {
			t.Errorf("Expected threshold violation message, got: %s", violations[0])
		}
	})

	// Test allocs violation (only checking allocs threshold)
	t.Run("allocs_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxAllocsPerOp: map[string]int64{
				"BenchmarkAllocy": 5,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:        "BenchmarkAllocy",
			AllocsPerOp: 10, // Exceeds 5
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "allocs/op exceeds") {
			t.Errorf("Expected allocs violation message, got: %s", violations[0])
		}
	})

	// Test throughput violation (only checking throughput threshold)
	t.Run("throughput_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MinThroughput: map[string]float64{
				"BenchmarkThroughput": 1000000,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:         "BenchmarkThroughput",
			EventsPerSec: 500000, // Below 1000000
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "below threshold") {
			t.Errorf("Expected throughput violation message, got: %s", violations[0])
		}
	})

	t.Run("missing_required_benchmark", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkRequired": 100,
			},
		}

		violations := thresholds.Validate(NewBenchmarkSuite("test"))
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "required benchmark result missing") {
			t.Errorf("Expected missing benchmark violation, got: %s", violations[0])
		}
	})

	t.Run("parsed_result_missing_required_metric", func(t *testing.T) {
		result, err := parseBenchLine("BenchmarkRequired-8 1000 50 ns/op")
		if err != nil {
			t.Fatalf("parseBenchLine returned error: %v", err)
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(result)
		thresholds := &PerformanceThresholds{
			MaxNsPerOp:     map[string]float64{"BenchmarkRequired": 100},
			MaxAllocsPerOp: map[string]int64{"BenchmarkRequired": 1},
		}

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "required allocs/op metric missing") {
			t.Errorf("Expected missing metric violation, got: %s", violations[0])
		}
	})
}

func TestDefaultWorkflowThresholds(t *testing.T) {
	thresholds := DefaultWorkflowThresholds()

	if thresholds.MinThroughput["BenchmarkThroughput_Simple"] != 100000 {
		t.Error("Expected default throughput threshold")
	}
	if thresholds.MaxNsPerOp["BenchmarkCELEvaluate_Simple"] != 2000 {
		t.Error("Expected shared-CI CEL threshold")
	}
	if thresholds.MaxNsPerOp["BenchmarkEngineProcess"] != 12000 {
		t.Error("Expected shared-CI engine threshold")
	}
	if thresholds.MaxNsPerOp["BenchmarkFilterMatch_EventType"] != 5500 {
		t.Error("Expected shared-CI filter threshold")
	}
	if thresholds.MaxNsPerOp["BenchmarkTransform_SetField"] != 3000 {
		t.Error("Expected shared-CI transform threshold")
	}
	if _, ok := thresholds.MaxNsPerOp["BenchmarkFilterMatch_EventType"]; !ok {
		t.Error("Expected threshold for the concrete filter benchmark")
	}
	if _, ok := thresholds.MaxNsPerOp["BenchmarkFilterMatch"]; ok {
		t.Error("Unexpected threshold for nonexistent BenchmarkFilterMatch")
	}
}

func TestQuickBenchmark(t *testing.T) {
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

	event := map[string]interface{}{"type": "test"}

	// Run a quick benchmark (10ms)
	opsPerSec := QuickBenchmark(engine, event, 10*time.Millisecond)

	// Should be able to process at least 1000 events/sec
	if opsPerSec < 1000 {
		t.Errorf("Expected at least 1000 ops/sec, got %.0f", opsPerSec)
	}
}

func TestTruncateName(t *testing.T) {
	tests := []struct {
		name     string
		maxLen   int
		expected string
	}{
		{"Short", 10, "Short"},
		{"ExactlyTen", 10, "ExactlyTen"},
		{"ThisNameIsWayTooLong", 10, "ThisNam..."},
		{"BenchmarkCELEvaluate_ConditionComplexity", 30, "BenchmarkCELEvaluate_Condit..."},
	}

	for _, tc := range tests {
		result := truncateName(tc.name, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateName(%q, %d) = %q, want %q",
				tc.name, tc.maxLen, result, tc.expected)
		}
	}
}
